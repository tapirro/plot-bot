#!/bin/bash
# Plot Bot — Heartbeat Watchdog
# Monitors heartbeat.json and alerts if bot is stale or dead.
# Run via launchd every 5 minutes, or as a cron job.
#
# Usage:
#   ./tools/scripts/watchdog.sh              # check once, alert if stale
#   WATCHDOG_THRESHOLD=600 ./watchdog.sh     # custom threshold (seconds)
#
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
HEARTBEAT_FILE="${REPO_DIR}/context/heartbeat.json"
PID_FILE="${REPO_DIR}/context/runner.pid"
THRESHOLD="${WATCHDOG_THRESHOLD:-600}"  # 10 minutes default

escalate() {
  curl -s -X POST "https://spora.live/api/v1/escalate" \
    -H "Authorization: Bearer $(cat "$REPO_DIR/.claude/api_token" 2>/dev/null)" \
    -H "X-Agent-Id: $(cat "$REPO_DIR/.claude/agent_id" 2>/dev/null)" \
    -H "Content-Type: application/json; charset=utf-8" \
    -d "{\"subject\": \"$1\", \"body\": \"$2\"}" \
    2>/dev/null || true
}

# Check 1: Is runner process alive?
if [ -f "$PID_FILE" ]; then
  PID=$(cat "$PID_FILE" 2>/dev/null || echo "0")
  if ! kill -0 "$PID" 2>/dev/null; then
    escalate "Plot Bot: Runner Dead" "PID file exists ($PID) but process is not running. Runner crashed without cleanup."
    exit 1
  fi
else
  # No PID file — runner not started or cleaned up normally
  exit 0
fi

# Check 2: Is heartbeat fresh?
if [ ! -f "$HEARTBEAT_FILE" ]; then
  escalate "Plot Bot: No Heartbeat" "Runner is running (PID $PID) but heartbeat.json does not exist."
  exit 1
fi

# Get heartbeat age in seconds
HB_TIMESTAMP=$(python3 -c "
import json, datetime
d = json.load(open('$HEARTBEAT_FILE'))
ts = d.get('timestamp', '')
if ts:
    dt = datetime.datetime.fromisoformat(ts.replace('Z', '+00:00'))
    now = datetime.datetime.now(datetime.timezone.utc)
    print(int((now - dt).total_seconds()))
else:
    print(99999)
" 2>/dev/null || echo "99999")

if [ "$HB_TIMESTAMP" -gt "$THRESHOLD" ]; then
  STATUS=$(python3 -c "import json; print(json.load(open('$HEARTBEAT_FILE')).get('status','?'))" 2>/dev/null || echo "?")
  escalate "Plot Bot: Stale Heartbeat" "Last heartbeat ${HB_TIMESTAMP}s ago (threshold: ${THRESHOLD}s). Status: ${STATUS}. PID: ${PID}."
  exit 1
fi

exit 0
