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
STATE_FILE="${REPO_DIR}/context/state.json"
COOLDOWN_SECONDS="${PLOT_BOT_COOLDOWN:-300}"     # 5 min between cycles
MAX_CONSECUTIVE_FAILURES=5
KEYCHAIN_PASSWORD="${PLOT_BOT_KEYCHAIN_PW:-}"
RATE_LIMIT_PAUSE=3600                            # 1 hour if rate-limited

HEARTBEAT_FILE="${REPO_DIR}/context/heartbeat.json"
PAUSE_FILE="${REPO_DIR}/context/agent_paused"

mkdir -p "$LOG_DIR" "${REPO_DIR}/context"

write_heartbeat() {
  # Args: status, [extra fields as JSON fragment]
  local status="$1"
  local extra="${2:-}"
  cat > "$HEARTBEAT_FILE" << HBJSON
{
  "status": "${status}",
  "cycle": ${NEXT_CYCLE:-0},
  "cycle_position": ${CYCLE_POS:-0},
  "cycle_type": "${CYCLE_TYPE:-unknown}",
  "mode": "${RATE_MODE:-FULL}",
  "rate_pct": ${rate_pct:-0},
  "timestamp": "$(date -u '+%Y-%m-%dT%H:%M:%SZ')",
  "pid": ${CLAUDE_PID:-0}${extra:+,
  $extra}
}
HBJSON
}

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_DIR/runner.log"
}

check_rate_control() {
  local pct
  pct=$(node ~/.claude/rate-control/claude_limits.mjs --json 2>/dev/null \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('windows',{}).get('7d',{}).get('pct',0))" 2>/dev/null || echo "0")
  echo "$pct"
}

read_state() {
  if [ -f "$STATE_FILE" ]; then
    CYCLE_COUNT=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('cycle_count',0))" 2>/dev/null || echo "0")
    CYCLE_POS=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('cycle_position',0))" 2>/dev/null || echo "0")
    AVG_IMPACT=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('avg_impact',0))" 2>/dev/null || echo "0")
  else
    CYCLE_COUNT=0
    CYCLE_POS=0
    AVG_IMPACT=0
    # Create initial state
    echo '{"cycle_count":0,"cycle_position":0,"mega_cycle":0,"last_impact":null,"impact_history":[],"avg_impact":0,"mode":"full","last_cycle_date":"","north_star_counts":{"V":0,"E":0,"R":0,"A":0}}' > "$STATE_FILE"
  fi
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
  # Pause gate: if pause file exists, wait until it's removed
  if [ -f "$PAUSE_FILE" ]; then
    log "PAUSED by operator (${PAUSE_FILE} exists). Waiting..."
    write_heartbeat "paused"
    while [ -f "$PAUSE_FILE" ]; do
      sleep 10
    done
    log "RESUMED — pause file removed."
  fi

  # Ensure keychain is unlocked (needed for Claude OAuth on macOS)
  unlock_keychain

  # Read loop state
  read_state
  NEXT_CYCLE=$((CYCLE_COUNT + 1))

  # Rate control gate
  rate_pct=$(check_rate_control)
  if (( $(echo "$rate_pct > 95" | bc -l 2>/dev/null || echo 0) )); then
    log "STOP mode: rate ${rate_pct}% > 95%. Pausing ${RATE_LIMIT_PAUSE}s."
    sleep "$RATE_LIMIT_PAUSE"
    continue
  fi

  # Determine mode
  if (( $(echo "$rate_pct > 80" | bc -l 2>/dev/null || echo 0) )); then
    log "ECO mode: rate ${rate_pct}% > 80%. Only escalations."
    RATE_MODE="ECO"
  elif (( $(echo "$rate_pct > 60" | bc -l 2>/dev/null || echo 0) )); then
    log "LIGHT mode: rate ${rate_pct}% > 60%. P0 tasks only."
    RATE_MODE="LIGHT"
  else
    log "FULL mode: rate ${rate_pct}%."
    RATE_MODE="FULL"
  fi

  # Determine cycle type
  if [ "$CYCLE_POS" -eq 0 ]; then
    CYCLE_TYPE="META"
  else
    CYCLE_TYPE="REGULAR"
  fi

  log "Cycle #${NEXT_CYCLE} (position ${CYCLE_POS}/4, type=${CYCLE_TYPE}, avg_impact=${AVG_IMPACT})"

  # Build prompt with state context
  CYCLE_PROMPT="Autonomous cycle #${NEXT_CYCLE}. Mode: ${RATE_MODE}. Rate: ${rate_pct}%.
Cycle position: ${CYCLE_POS} of 4 (0=META).
Type: ${CYCLE_TYPE}.
Avg impact (last 4): ${AVG_IMPACT}.

Follow the Autonomous Loop Protocol in CLAUDE.md exactly:
1. Read context/state.json for full state
2. Execute Session Start sequence
3. $([ "$CYCLE_TYPE" = "META" ] && echo "This is a META cycle: retrospective + quality check + research + plan next 4 cycles." || echo "This is a regular cycle: pick task #${CYCLE_POS} from context/cycle_plan.md and execute it.")
4. Execute Session End sequence: self-score, append to work/CYCLE_PROGRESS.md, update context/state.json, commit, log to Hive
5. Build dashboard: python3 tools/scripts/build_cycle_dashboard.py"

  TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
  CYCLE_LOG="$LOG_DIR/cycle_${TIMESTAMP}.log"

  log "Starting cycle → $CYCLE_LOG"

  # Write heartbeat: running
  CLAUDE_PID=0
  write_heartbeat "running"

  # Run one autonomous cycle (with 30-min watchdog)
  set +e
  cd "$REPO_DIR"
  claude -p "$CYCLE_PROMPT" \
    --dangerously-skip-permissions \
    --output-format text \
    --max-turns 30 \
    > "$CYCLE_LOG" 2>&1 &
  CLAUDE_PID=$!

  # Update heartbeat with actual PID
  write_heartbeat "running"

  # Watchdog: kill if running longer than 30 minutes
  ( sleep 1800 && kill "$CLAUDE_PID" 2>/dev/null ) &
  WATCHDOG_PID=$!

  wait "$CLAUDE_PID" 2>/dev/null
  EXIT_CODE=$?
  kill "$WATCHDOG_PID" 2>/dev/null || true
  wait "$WATCHDOG_PID" 2>/dev/null || true
  set -e

  if [ $EXIT_CODE -eq 0 ]; then
    log "Cycle #${NEXT_CYCLE} completed successfully."
    write_heartbeat "completed" "\"exit_code\": 0"
    failures=0
  elif [ $EXIT_CODE -eq 137 ] || [ $EXIT_CODE -eq 143 ]; then
    log "Cycle #${NEXT_CYCLE} timed out (30 min). Will retry."
    write_heartbeat "timeout" "\"exit_code\": $EXIT_CODE"
    failures=$((failures + 1))
  else
    log "Cycle #${NEXT_CYCLE} failed (exit=$EXIT_CODE). Failure $((failures + 1))/${MAX_CONSECUTIVE_FAILURES}."
    write_heartbeat "failed" "\"exit_code\": $EXIT_CODE, \"failures\": $((failures + 1))"
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
  write_heartbeat "cooldown" "\"cooldown_seconds\": $COOLDOWN_SECONDS"
  sleep "$COOLDOWN_SECONDS"
done
