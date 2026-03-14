#!/bin/bash
# Plot Bot — Autonomous Agent Runner
# Runs claude in headless mode in a loop with cooldown, rate control, and logging.
# Designed to be supervised by launchd (macOS) or systemd (Linux).
#
# Usage:
#   ./tools/scripts/runner.sh              # foreground
#   launchd / systemd → auto-managed       # production
#
# Safety features:
#   P0: PID lock, signal handlers, atomic state writes, crash alerting
#   P1: Feedback gate every cycle, repetition detection, git commit verification
#   P2: Heartbeat watchdog, token projection, JSONL rotation
#
set -euo pipefail

export PATH="/opt/homebrew/bin:/usr/local/bin:$PATH"

REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
LOG_DIR="${REPO_DIR}/logs"
STATE_FILE="${REPO_DIR}/context/state.json"
COOLDOWN_MIN=10                                  # minimum pause between cycles (seconds)
MAX_CONSECUTIVE_FAILURES=5
KEYCHAIN_PASSWORD="${PLOT_BOT_KEYCHAIN_PW:-}"
RATE_LIMIT_PAUSE=3600                            # 1 hour if rate-limited

HEARTBEAT_FILE="${REPO_DIR}/context/heartbeat.json"
PAUSE_FILE="${REPO_DIR}/context/agent_paused"
PID_FILE="${REPO_DIR}/context/runner.pid"
FEEDBACK_DIR="${REPO_DIR}/work/feedback"
CYCLE_REPORTS_DIR="${REPO_DIR}/work/cycle_reports"
JSONL_DIR="$HOME/.claude/projects/-Users-polansk-Developer-mantissa-code-plot-bot"
MAX_JSONL_FILES=100
REPETITION_WINDOW=3                              # alert if last N titles are identical

mkdir -p "$LOG_DIR" "${REPO_DIR}/context"

# ═══════════════════════════════════════════════════════════════════════════════
# P0-1: PID LOCK — prevent double-start
# ═══════════════════════════════════════════════════════════════════════════════
if [ -f "$PID_FILE" ]; then
  OLD_PID=$(cat "$PID_FILE" 2>/dev/null || echo "0")
  if kill -0 "$OLD_PID" 2>/dev/null; then
    echo "Runner already running (PID $OLD_PID). Exiting."
    exit 1
  fi
  # Stale PID file — process is dead, clean up
  rm -f "$PID_FILE"
fi
echo $$ > "$PID_FILE"

# ═══════════════════════════════════════════════════════════════════════════════
# P0-2: SIGNAL HANDLERS — clean shutdown on SIGTERM/SIGINT/EXIT
# ═══════════════════════════════════════════════════════════════════════════════
cleanup() {
  local sig="${1:-EXIT}"
  log "Signal $sig received. Cleaning up..."
  # Kill child claude process if running
  if [ -n "${CLAUDE_PID:-}" ] && [ "${CLAUDE_PID:-0}" -ne 0 ]; then
    kill "$CLAUDE_PID" 2>/dev/null || true
  fi
  if [ -n "${WATCHDOG_PID:-}" ] && [ "${WATCHDOG_PID:-0}" -ne 0 ]; then
    kill "$WATCHDOG_PID" 2>/dev/null || true
  fi
  write_heartbeat "stopped" "\"reason\": \"signal_${sig}\""
  rm -f "$PID_FILE"
  # P0-4: Alert Vadim on unexpected shutdown
  if [ "$sig" != "EXIT" ] && [ "$sig" != "clean" ]; then
    escalate "Plot Bot: Runner stopped" "Runner received $sig. PID=$$. Last cycle: ${NEXT_CYCLE:-?}."
  fi
  exit 0
}
trap 'cleanup TERM' SIGTERM
trap 'cleanup INT' SIGINT
trap 'cleanup EXIT' EXIT

# ═══════════════════════════════════════════════════════════════════════════════
# UTILITY FUNCTIONS
# ═══════════════════════════════════════════════════════════════════════════════
write_heartbeat() {
  local status="$1"
  local extra="${2:-}"
  # Atomic write: tmp → mv
  local tmp="${HEARTBEAT_FILE}.tmp"
  cat > "$tmp" << HBJSON
{
  "status": "${status}",
  "cycle": ${NEXT_CYCLE:-0},
  "cycle_position": ${CYCLE_POS:-0},
  "cycle_type": "${CYCLE_TYPE:-unknown}",
  "mode": "${RATE_MODE:-FULL}",
  "rate_pct": ${rate_pct:-0},
  "timestamp": "$(date -u '+%Y-%m-%dT%H:%M:%SZ')",
  "pid": $$${extra:+,
  $extra}
}
HBJSON
  mv -f "$tmp" "$HEARTBEAT_FILE"
}

log() {
  echo "[$(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_DIR/runner.log"
}

escalate() {
  local subject="$1"
  local body="$2"
  log "ESCALATION: $subject — $body"
  curl -s -X POST "https://spora.live/api/v1/escalate" \
    -H "Authorization: Bearer $(cat "$REPO_DIR/.claude/api_token" 2>/dev/null)" \
    -H "X-Agent-Id: $(cat "$REPO_DIR/.claude/agent_id" 2>/dev/null)" \
    -H "Content-Type: application/json; charset=utf-8" \
    -d "{\"subject\": \"$subject\", \"body\": \"$body\"}" \
    2>/dev/null || log "WARNING: escalation POST failed"
}

check_rate_control() {
  local pct
  pct=$(node ~/.claude/rate-control/claude_limits.mjs --json 2>/dev/null \
    | python3 -c "import sys,json; print(json.load(sys.stdin).get('windows',{}).get('7d',{}).get('pct',0))" 2>/dev/null || echo "0")
  echo "$pct"
}

# P0-3: Atomic state file operations with backup
read_state() {
  if [ -f "$STATE_FILE" ]; then
    # Validate JSON before reading
    if python3 -c "import json; json.load(open('$STATE_FILE'))" 2>/dev/null; then
      CYCLE_COUNT=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('cycle_count',0))" 2>/dev/null || echo "0")
      CYCLE_POS=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('cycle_position',0))" 2>/dev/null || echo "0")
      AVG_IMPACT=$(python3 -c "import json; d=json.load(open('$STATE_FILE')); print(d.get('avg_impact',0))" 2>/dev/null || echo "0")
    else
      log "ERROR: state.json corrupted! Attempting recovery from backup..."
      if [ -f "${STATE_FILE}.bak" ] && python3 -c "import json; json.load(open('${STATE_FILE}.bak'))" 2>/dev/null; then
        cp "${STATE_FILE}.bak" "$STATE_FILE"
        log "Recovered state.json from backup."
        read_state  # re-read from restored file
        return
      else
        log "CRITICAL: No valid backup. Recovering from git..."
        cd "$REPO_DIR"
        git show HEAD:context/state.json > "$STATE_FILE" 2>/dev/null || true
        if python3 -c "import json; json.load(open('$STATE_FILE'))" 2>/dev/null; then
          log "Recovered state.json from git HEAD."
          read_state
          return
        fi
        log "CRITICAL: All recovery failed. Escalating and stopping."
        escalate "Plot Bot: State Corrupted" "state.json corrupted, backup missing, git recovery failed. Manual intervention needed."
        sleep "$RATE_LIMIT_PAUSE"
        CYCLE_COUNT=0; CYCLE_POS=0; AVG_IMPACT=0
      fi
    fi
  else
    CYCLE_COUNT=0
    CYCLE_POS=0
    AVG_IMPACT=0
    echo '{"cycle_count":0,"cycle_position":0,"mega_cycle":0,"last_impact":null,"impact_history":[],"avg_impact":0,"mode":"full","last_cycle_date":"","north_star_counts":{"V":0,"E":0,"R":0,"A":0}}' > "$STATE_FILE"
  fi
  # Always keep a backup after successful read
  cp "$STATE_FILE" "${STATE_FILE}.bak" 2>/dev/null || true
}

# P1-1: Check for pending feedback
check_pending_feedback() {
  if [ ! -d "$FEEDBACK_DIR" ]; then
    echo "0"
    return
  fi
  local count
  count=$(grep -rl 'status: pending' "$FEEDBACK_DIR"/FEEDBACK_*.md 2>/dev/null | wc -l | tr -d ' ')
  echo "$count"
}

# P1-2: Detect repetitive cycle titles
check_repetition() {
  if [ ! -d "$CYCLE_REPORTS_DIR" ]; then
    return 0
  fi
  # Get last N cycle titles from filenames
  local recent_titles
  recent_titles=$(ls -1t "$CYCLE_REPORTS_DIR"/CYCLE_*.md 2>/dev/null \
    | head -"$REPETITION_WINDOW" \
    | xargs -I{} basename {} .md \
    | sed 's/CYCLE_[0-9]*_//' \
    | sort -u)
  local unique_count
  unique_count=$(echo "$recent_titles" | grep -c . || echo "0")
  if [ "$unique_count" -le 1 ] && [ "$(ls -1 "$CYCLE_REPORTS_DIR"/CYCLE_*.md 2>/dev/null | wc -l | tr -d ' ')" -ge "$REPETITION_WINDOW" ]; then
    return 1  # repetition detected
  fi
  return 0
}

# P1-3: Verify git commit succeeded
verify_git_state() {
  cd "$REPO_DIR"
  local dirty
  dirty=$(git status --porcelain 2>/dev/null | grep -v '^??' | wc -l | tr -d ' ')
  if [ "$dirty" -gt 0 ]; then
    log "WARNING: Git working tree dirty after cycle (${dirty} modified files). Bot may have failed to commit."
    return 1
  fi
  return 0
}

# P2-1: Token budget projection
project_token_budget() {
  python3 -c "
import json, sys
try:
    data = json.load(sys.stdin)
    w = data.get('windows', {}).get('7d', {})
    pct = float(w.get('pct', 0))
    days_left = float(w.get('days_left', 7))
    if pct > 0 and days_left > 0:
        rate_per_day = pct / max(7 - days_left, 0.5)
        projected = pct + rate_per_day * days_left
        hours_to_limit = (100 - pct) / (rate_per_day / 24) if rate_per_day > 0 else 999
        print(json.dumps({'projected_pct': round(projected, 1), 'hours_to_limit': round(hours_to_limit, 1)}))
    else:
        print(json.dumps({'projected_pct': 0, 'hours_to_limit': 999}))
except:
    print(json.dumps({'projected_pct': 0, 'hours_to_limit': 999}))
" 2>/dev/null || echo '{"projected_pct": 0, "hours_to_limit": 999}'
}

# P2-2: Rotate JSONL session logs
rotate_jsonl() {
  if [ -d "$JSONL_DIR" ]; then
    local count
    count=$(ls -1 "$JSONL_DIR"/*.jsonl 2>/dev/null | wc -l | tr -d ' ')
    if [ "$count" -gt "$MAX_JSONL_FILES" ]; then
      local to_remove=$((count - MAX_JSONL_FILES))
      log "Rotating JSONL: removing ${to_remove} old session files (keeping ${MAX_JSONL_FILES})"
      ls -1t "$JSONL_DIR"/*.jsonl 2>/dev/null | tail -n +"$((MAX_JSONL_FILES + 1))" | xargs rm -f 2>/dev/null || true
    fi
  fi
}

failures=0

unlock_keychain() {
  if [ -n "$KEYCHAIN_PASSWORD" ]; then
    security unlock-keychain -p "$KEYCHAIN_PASSWORD" \
      "$HOME/Library/Keychains/login.keychain-db" 2>/dev/null || true
  fi
}

log "=== Plot Bot Runner started (PID $$) ==="
log "Repo: $REPO_DIR"
log "Rate control mode (no fixed cooldown)"

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

  # Read loop state (with corruption protection)
  read_state
  NEXT_CYCLE=$((CYCLE_COUNT + 1))

  # Rate control gate
  rate_pct=$(check_rate_control)
  if (( $(echo "$rate_pct > 95" | bc -l 2>/dev/null || echo 0) )); then
    log "STOP mode: rate ${rate_pct}% > 95%. Pausing ${RATE_LIMIT_PAUSE}s."
    write_heartbeat "rate-limited" "\"rate_pct\": ${rate_pct}"
    sleep "$RATE_LIMIT_PAUSE"
    continue
  fi

  # P2-1: Token budget projection
  PROJECTION=$(node ~/.claude/rate-control/claude_limits.mjs --json 2>/dev/null | project_token_budget)
  PROJ_PCT=$(echo "$PROJECTION" | python3 -c "import sys,json; print(json.load(sys.stdin).get('projected_pct',0))" 2>/dev/null || echo "0")
  HOURS_LEFT=$(echo "$PROJECTION" | python3 -c "import sys,json; print(json.load(sys.stdin).get('hours_to_limit',999))" 2>/dev/null || echo "999")

  # Determine mode
  if (( $(echo "$rate_pct > 80" | bc -l 2>/dev/null || echo 0) )); then
    log "ECO mode: rate ${rate_pct}% > 80%. Only escalations. (~${HOURS_LEFT}h to limit)"
    RATE_MODE="ECO"
  elif (( $(echo "$rate_pct > 60" | bc -l 2>/dev/null || echo 0) )); then
    log "LIGHT mode: rate ${rate_pct}% > 60%. P0 tasks only. (~${HOURS_LEFT}h to limit)"
    RATE_MODE="LIGHT"
  else
    log "FULL mode: rate ${rate_pct}%. Projected: ${PROJ_PCT}% by reset. (~${HOURS_LEFT}h to limit)"
    RATE_MODE="FULL"
  fi

  # Alert if projected to hit limit
  if (( $(echo "$PROJ_PCT > 90" | bc -l 2>/dev/null || echo 0) )) && (( $(echo "$rate_pct < 80" | bc -l 2>/dev/null || echo 0) )); then
    log "WARNING: Projected ${PROJ_PCT}% by weekly reset. Consider reducing cycle frequency."
  fi

  # Determine cycle type and class
  if [ "$CYCLE_POS" -eq 0 ]; then
    CYCLE_TYPE="META"
    CYCLE_CLASS="META"
    MAX_TURNS=15
    WATCHDOG_TIMEOUT=1200  # 20 min
  elif [ "$CYCLE_POS" -eq 1 ] || [ "$CYCLE_POS" -eq 3 ]; then
    CYCLE_CLASS="DATA"
    MAX_TURNS=10
    WATCHDOG_TIMEOUT=600   # 10 min
    # Try to read subtype from cycle_plan.md
    CYCLE_TYPE=$(python3 -c "
import re, sys
try:
    text = open('${REPO_DIR}/context/cycle_plan.md').read()
    # Find line for this position: '## Position N' or '- N.' or '#N'
    pattern = r'(?:^|\n).*?(?:position\s*${CYCLE_POS}|^${CYCLE_POS}[\.\):]|#\s*${CYCLE_POS}\b).*?(?:type|subtype|class)?[:\s]*(RESEARCH|ANALYSIS|BUILD|ESCALATION|BOLD|DATA|WORK)'
    m = re.search(pattern, text, re.IGNORECASE)
    print(m.group(1).upper() if m else 'DATA')
except: print('DATA')
" 2>/dev/null || echo "DATA")
  else
    CYCLE_CLASS="WORK"
    MAX_TURNS=20
    WATCHDOG_TIMEOUT=1500  # 25 min
    CYCLE_TYPE=$(python3 -c "
import re, sys
try:
    text = open('${REPO_DIR}/context/cycle_plan.md').read()
    pattern = r'(?:^|\n).*?(?:position\s*${CYCLE_POS}|^${CYCLE_POS}[\.\):]|#\s*${CYCLE_POS}\b).*?(?:type|subtype|class)?[:\s]*(RESEARCH|ANALYSIS|BUILD|ESCALATION|BOLD|DATA|WORK)'
    m = re.search(pattern, text, re.IGNORECASE)
    print(m.group(1).upper() if m else 'WORK')
except: print('WORK')
" 2>/dev/null || echo "WORK")
  fi

  # P1-1: Feedback gate — check for pending feedback EVERY cycle
  PENDING_FB=$(check_pending_feedback)
  FB_INSTRUCTION=""
  if [ "$PENDING_FB" -gt 0 ]; then
    log "FEEDBACK GATE: ${PENDING_FB} pending feedback file(s) in work/feedback/"
    FB_INSTRUCTION="
CRITICAL: There are ${PENDING_FB} PENDING feedback file(s) in work/feedback/ with status: pending.
These contain operator decisions that MUST be processed BEFORE any other work.
Read each pending file, apply decisions per the Escalation Response Processing rules in CLAUDE.md,
update affected artifacts, then set status to resolved. This takes priority over your planned task."
  fi

  # P1-2: Repetition detection
  REPETITION_WARNING=""
  if ! check_repetition; then
    log "WARNING: Last ${REPETITION_WINDOW} cycles have identical titles — possible loop detected."
    REPETITION_WARNING="
WARNING: The last ${REPETITION_WINDOW} cycles appear to have done the same work.
Before proceeding, review work/CYCLE_PROGRESS.md for the last ${REPETITION_WINDOW} entries.
If you're repeating yourself, STOP and pick a DIFFERENT task from the roadmap or research tier.
Write a brief note in your cycle report explaining what was different this time."
  fi

  # Pre-flight: verify cycle_plan.md exists for DATA/WORK cycles
  PLAN_WARNING=""
  if [ "$CYCLE_CLASS" != "META" ] && [ ! -f "${REPO_DIR}/context/cycle_plan.md" ]; then
    log "WARNING: context/cycle_plan.md missing. ${CYCLE_CLASS} cycle has no task plan."
    PLAN_WARNING="
WARNING: context/cycle_plan.md does not exist. You MUST create it before executing a ${CYCLE_CLASS} cycle.
Run a mini-META: read the roadmap, pick a task for this position, write cycle_plan.md, then execute."
  fi

  log "Cycle #${NEXT_CYCLE} (pos ${CYCLE_POS}/4, class=${CYCLE_CLASS}, type=${CYCLE_TYPE}, turns=${MAX_TURNS}, avg_impact=${AVG_IMPACT})"

  # Build cycle-class-specific instruction
  if [ "$CYCLE_CLASS" = "META" ]; then
    CLASS_INSTRUCTION="This is a META cycle (15 turns): retrospective + quality check + research + plan next 4 cycles (DATA/WORK slots). CHECK work/feedback/ for pending items FIRST. If backlog is thin (<2 tasks with impact ≥3), generate hypotheses per CLAUDE.md §Hypothesis Generation."
  elif [ "$CYCLE_CLASS" = "DATA" ]; then
    CLASS_INSTRUCTION="This is a DATA cycle (10 turns): mechanical data collection. Pick task #${CYCLE_POS} from context/cycle_plan.md. Run scripts, save outputs, write minimal stats. NO Gemini required. NO ./ask scan. Be fast and cheap."
  else
    CLASS_INSTRUCTION="This is a WORK cycle (20 turns): full analytical protocol. Pick task #${CYCLE_POS} from context/cycle_plan.md. Gemini offload for research >200 lines. Run ./ask scan. Full cycle report with Gemini Log."
  fi

  # Build prompt with state context
  CYCLE_PROMPT="Autonomous cycle #${NEXT_CYCLE}. Mode: ${RATE_MODE}. Rate: ${rate_pct}%.
Cycle position: ${CYCLE_POS} of 4 (0=META). Class: ${CYCLE_CLASS}. Subtype: ${CYCLE_TYPE}.
Avg impact (last 4): ${AVG_IMPACT}.
Token budget: ${HOURS_LEFT}h to weekly limit (projected ${PROJ_PCT}% at reset).
Turn limit: ${MAX_TURNS}.

Follow the Autonomous Loop Protocol in CLAUDE.md exactly:
1. Read context/state.json for full state
2. Execute Session Start sequence
3. ${CLASS_INSTRUCTION}
4. Execute Session End sequence: self-score, append to work/CYCLE_PROGRESS.md, update context/state.json, update roadmap checkboxes if task completed, commit, log to Hive
5. Build dashboard: python3 tools/scripts/build_cycle_dashboard.py
${FB_INSTRUCTION}${REPETITION_WARNING}${PLAN_WARNING}
MANDATORY REMINDERS:
- For WORK cycles: Gemini offload is REQUIRED for research >200 lines. Cycle report MUST include ## Gemini Log.
- For DATA cycles: skip Gemini and compliance scan. Write minimal stats report. Be fast.
- NEVER use git add -A. Stage specific files only.
- Check work/feedback/ for pending operator feedback — process before new work if any exist.
- After git commit, verify it succeeded. If it fails, do NOT advance state.json."

  TIMESTAMP=$(date '+%Y%m%d_%H%M%S')
  CYCLE_LOG="$LOG_DIR/cycle_${TIMESTAMP}.log"

  # Record git state before cycle (for P1-3 verification)
  GIT_HEAD_BEFORE=$(cd "$REPO_DIR" && git rev-parse HEAD 2>/dev/null || echo "unknown")

  log "Starting cycle → $CYCLE_LOG"

  CYCLE_START=$(date +%s)

  # Write heartbeat: running
  CLAUDE_PID=0
  write_heartbeat "running"

  # Run one autonomous cycle (with class-appropriate watchdog)
  set +e
  cd "$REPO_DIR"
  claude -p "$CYCLE_PROMPT" \
    --dangerously-skip-permissions \
    --output-format json \
    --max-turns "$MAX_TURNS" \
    > "$CYCLE_LOG" 2>&1 &
  CLAUDE_PID=$!

  # Update heartbeat with actual PID
  write_heartbeat "running"

  # Watchdog: kill if exceeds timeout (DATA=10m, META=20m, WORK=25m)
  ( sleep "$WATCHDOG_TIMEOUT" && kill "$CLAUDE_PID" 2>/dev/null ) &
  WATCHDOG_PID=$!

  wait "$CLAUDE_PID" 2>/dev/null
  EXIT_CODE=$?
  kill "$WATCHDOG_PID" 2>/dev/null || true
  wait "$WATCHDOG_PID" 2>/dev/null || true
  set -e

  # Calculate cycle duration
  CYCLE_END=$(date +%s)
  CYCLE_DURATION=$((CYCLE_END - CYCLE_START))

  # Get token usage from latest JSONL session
  CYCLE_TOKENS=$(python3 -c "
import json, glob, os
sessions_dir = os.path.expanduser('~/.claude/projects/-Users-polansk-Developer-mantissa-code-plot-bot')
files = sorted(glob.glob(os.path.join(sessions_dir, '*.jsonl')), key=os.path.getmtime)
if not files:
    print(0)
else:
    total = 0
    with open(files[-1]) as f:
        for line in f:
            try:
                u = json.loads(line).get('message', {}).get('usage', {})
                total += u.get('input_tokens', 0) + u.get('output_tokens', 0)
            except: pass
    print(total)
" 2>/dev/null || echo "0")

  # Refresh rate control
  rate_pct=$(check_rate_control)

  # Detect max-turns exhaustion (exit 0 but actually degraded)
  CYCLE_OUTCOME="success"
  if [ $EXIT_CODE -eq 0 ] && grep -q "Reached max turns" "$CYCLE_LOG" 2>/dev/null; then
    log "WARNING: Cycle #${NEXT_CYCLE} hit max-turns limit (30). Marking as degraded."
    CYCLE_OUTCOME="max-turns"
  fi

  # P1-3: Verify git commit happened
  GIT_HEAD_AFTER=$(cd "$REPO_DIR" && git rev-parse HEAD 2>/dev/null || echo "unknown")
  GIT_DIRTY=0
  if ! verify_git_state; then
    GIT_DIRTY=1
  fi
  GIT_COMMITTED=0
  if [ "$GIT_HEAD_BEFORE" != "$GIT_HEAD_AFTER" ]; then
    GIT_COMMITTED=1
  fi

  if [ $EXIT_CODE -eq 0 ]; then
    # Check for git issues
    if [ $GIT_COMMITTED -eq 0 ] && [ "$CYCLE_OUTCOME" = "success" ]; then
      log "WARNING: Cycle completed but no git commit was made."
      CYCLE_OUTCOME="no-commit"
    fi
    if [ $GIT_DIRTY -eq 1 ]; then
      log "WARNING: Working tree dirty after cycle. Uncommitted changes exist."
      CYCLE_OUTCOME="${CYCLE_OUTCOME}+dirty"
    fi

    log "Cycle #${NEXT_CYCLE} completed (${CYCLE_OUTCOME}). Tokens: ${CYCLE_TOKENS}, Duration: ${CYCLE_DURATION}s"
    write_heartbeat "completed" "\"exit_code\": 0, \"outcome\": \"${CYCLE_OUTCOME}\", \"committed\": ${GIT_COMMITTED}"
    # Log to Hive with rate + tokens
    HIVE_EXTRA="{\"rate_pct\": ${rate_pct}, \"cycle_type\": \"${CYCLE_TYPE}\", \"mode\": \"${RATE_MODE}\", \"outcome\": \"${CYCLE_OUTCOME}\", \"projected_pct\": ${PROJ_PCT}, \"hours_left\": ${HOURS_LEFT}}"
    bash "$REPO_DIR/tools/scripts/hive_log.sh" \
      "cycle ${NEXT_CYCLE}: ${CYCLE_TYPE} (${RATE_MODE}) [${CYCLE_OUTCOME}]" \
      "$CYCLE_TOKENS" "$CYCLE_DURATION" "$CYCLE_OUTCOME" "$HIVE_EXTRA" \
      2>/dev/null || log "WARNING: Hive log failed"
    failures=0
  elif [ $EXIT_CODE -eq 137 ] || [ $EXIT_CODE -eq 143 ]; then
    log "Cycle #${NEXT_CYCLE} timed out (30 min). Will retry."
    write_heartbeat "timeout" "\"exit_code\": $EXIT_CODE"
    bash "$REPO_DIR/tools/scripts/hive_log.sh" \
      "cycle ${NEXT_CYCLE}: TIMEOUT after 30m" \
      "$CYCLE_TOKENS" "$CYCLE_DURATION" "timeout" \
      2>/dev/null || true
    failures=$((failures + 1))
  else
    log "Cycle #${NEXT_CYCLE} failed (exit=$EXIT_CODE). Failure $((failures + 1))/${MAX_CONSECUTIVE_FAILURES}."
    write_heartbeat "failed" "\"exit_code\": $EXIT_CODE, \"failures\": $((failures + 1))"
    bash "$REPO_DIR/tools/scripts/hive_log.sh" \
      "cycle ${NEXT_CYCLE}: FAILED exit=${EXIT_CODE}" \
      "$CYCLE_TOKENS" "$CYCLE_DURATION" "failure" \
      2>/dev/null || true
    failures=$((failures + 1))
  fi

  # Circuit breaker
  if [ $failures -ge $MAX_CONSECUTIVE_FAILURES ]; then
    log "CIRCUIT BREAKER: ${failures} consecutive failures. Pausing ${RATE_LIMIT_PAUSE}s."
    escalate "Plot Bot: Circuit Breaker" "${failures} consecutive failures. Last exit code: ${EXIT_CODE}. Pausing for 1 hour."
    sleep "$RATE_LIMIT_PAUSE"
    failures=0
  fi

  # Rotate logs (keep last 50 cycle logs)
  ls -1t "$LOG_DIR"/cycle_*.log 2>/dev/null | tail -n +51 | xargs rm -f 2>/dev/null || true

  # P2-2: Rotate JSONL session logs
  rotate_jsonl

  # Dynamic cooldown based on rate control
  rate_now=$(check_rate_control)
  if (( $(echo "$rate_now > 80" | bc -l 2>/dev/null || echo 0) )); then
    COOLDOWN=600  # 10 min — eco mode, slow down
    log "Rate ${rate_now}% > 80% → cooldown ${COOLDOWN}s (eco)"
  elif (( $(echo "$rate_now > 60" | bc -l 2>/dev/null || echo 0) )); then
    COOLDOWN=120  # 2 min — light mode
    log "Rate ${rate_now}% > 60% → cooldown ${COOLDOWN}s (light)"
  else
    COOLDOWN=$COOLDOWN_MIN  # 10s — full speed
    log "Rate ${rate_now}% → cooldown ${COOLDOWN}s (full)"
  fi

  write_heartbeat "cooldown" "\"cooldown_seconds\": $COOLDOWN, \"rate_pct\": $rate_now"
  sleep "$COOLDOWN"
done
