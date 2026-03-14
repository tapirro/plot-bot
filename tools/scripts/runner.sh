#!/bin/bash
# Plot Bot — Autonomous Agent Runner
# Runs claude in headless mode in a loop with cooldown, rate control, and logging.
# Designed to be supervised by launchd (macOS) or systemd (Linux).
#
# Usage:
#   ./tools/scripts/runner.sh              # foreground
#   launchd / systemd → auto-managed       # production
#
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
LOG_DIR="${REPO_DIR}/logs"
COOLDOWN_SECONDS="${PLOT_BOT_COOLDOWN:-300}"     # 5 min between cycles
MAX_CONSECUTIVE_FAILURES=5
KEYCHAIN_PASSWORD="${PLOT_BOT_KEYCHAIN_PW:-}"
RATE_LIMIT_PAUSE=3600                            # 1 hour if rate-limited

mkdir -p "$LOG_DIR"

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_DIR/runner.log"
}

check_rate_control() {
  local pct
  pct=$(node ~/.claude/rate-control/claude_limits.mjs --json 2>/dev/null \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('windows',{}).get('7d',{}).get('pct',0))" 2>/dev/null || echo "0")
  echo "$pct"
}

failures=0

unlock_keychain() {
  if [ -n "$KEYCHAIN_PASSWORD" ]; then
    security unlock-keychain -p "$KEYCHAIN_PASSWORD" \
      "$HOME/Library/Keychains/login.keychain-db" 2>/dev/null || true
  fi
}

log "=== Plot Bot Runner started ==="
log "Repo: $REPO_DIR"
log "Cooldown: ${COOLDOWN_SECONDS}s"

while true; do
  # Ensure keychain is unlocked (needed for Claude OAuth on macOS)
  unlock_keychain
  # Rate control gate
  rate_pct=$(check_rate_control)
  if (( $(echo "$rate_pct > 95" | bc -l 2>/dev/null || echo 0) )); then
    log "STOP mode: rate ${rate_pct}% > 95%. Pausing ${RATE_LIMIT_PAUSE}s."
    sleep "$RATE_LIMIT_PAUSE"
    continue
  fi

  if (( $(echo "$rate_pct > 80" | bc -l 2>/dev/null || echo 0) )); then
    log "ECO mode: rate ${rate_pct}% > 80%. Only escalations."
    CYCLE_PROMPT="You are in ECO mode (${rate_pct}% budget used). Only process escalations and log status. Do NOT start new tasks."
  elif (( $(echo "$rate_pct > 60" | bc -l 2>/dev/null || echo 0) )); then
    log "LIGHT mode: rate ${rate_pct}% > 60%. P0 tasks only."
    CYCLE_PROMPT="You are in LIGHT mode (${rate_pct}% budget used). Only work on P0 priority tasks. Use Gemini for all research."
  else
    log "FULL mode: rate ${rate_pct}%."
    CYCLE_PROMPT="Full mode. Execute your autonomous loop: bootstrap, check inbox, pick highest priority task, execute, complete, repeat."
  fi

  TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
  CYCLE_LOG="$LOG_DIR/cycle_${TIMESTAMP}.log"

  log "Starting cycle → $CYCLE_LOG"

  # Run one autonomous cycle (with 30-min watchdog)
  set +e
  cd "$REPO_DIR"
  claude -p "$CYCLE_PROMPT" \
    --dangerously-skip-permissions \
    --output-format text \
    --max-turns 30 \
    > "$CYCLE_LOG" 2>&1 &
  CLAUDE_PID=$!

  # Watchdog: kill if running longer than 30 minutes
  ( sleep 1800 && kill "$CLAUDE_PID" 2>/dev/null ) &
  WATCHDOG_PID=$!

  wait "$CLAUDE_PID" 2>/dev/null
  EXIT_CODE=$?
  kill "$WATCHDOG_PID" 2>/dev/null || true
  wait "$WATCHDOG_PID" 2>/dev/null || true
  set -e

  if [ $EXIT_CODE -eq 0 ]; then
    log "Cycle completed successfully."
    failures=0
  elif [ $EXIT_CODE -eq 137 ] || [ $EXIT_CODE -eq 143 ]; then
    log "Cycle timed out (30 min). Will retry."
    failures=$((failures + 1))
  else
    log "Cycle failed (exit=$EXIT_CODE). Failure $((failures + 1))/${MAX_CONSECUTIVE_FAILURES}."
    failures=$((failures + 1))
  fi

  # Circuit breaker
  if [ $failures -ge $MAX_CONSECUTIVE_FAILURES ]; then
    log "CIRCUIT BREAKER: ${failures} consecutive failures. Pausing ${RATE_LIMIT_PAUSE}s."
    # Escalate to Vadim
    curl -s -X POST "https://spora.live/api/v1/escalate" \
      -H "Authorization: Bearer $(cat "$REPO_DIR/.claude/api_token")" \
      -H "X-Agent-Id: $(cat "$REPO_DIR/.claude/agent_id")" \
      -H "Content-Type: application/json; charset=utf-8" \
      -d "{\"subject\": \"Plot Bot: Circuit Breaker\", \"body\": \"${failures} consecutive failures. Last exit code: ${EXIT_CODE}. Pausing for 1 hour.\"}" \
      2>/dev/null || true
    sleep "$RATE_LIMIT_PAUSE"
    failures=0
  fi

  # Rotate logs (keep last 50 cycle logs)
  ls -1t "$LOG_DIR"/cycle_*.log 2>/dev/null | tail -n +51 | xargs rm -f 2>/dev/null || true

  log "Cooldown ${COOLDOWN_SECONDS}s..."
  sleep "$COOLDOWN_SECONDS"
done
