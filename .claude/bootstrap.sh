#!/bin/bash
# Hive Bootstrap Script — Plot Bot
# Connects to Hive hub at session start, retrieves instructions, syncs Telema.

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKSPACE_DIR="$(dirname "$SCRIPT_DIR")"

# Read config from .env
if [ -f "$WORKSPACE_DIR/.env" ]; then
  source "$WORKSPACE_DIR/.env"
fi

HIVE_URL="${HIVE_API_URL:-https://spora.live}"
API_TOKEN="${HIVE_API_TOKEN}"

if [ -z "$API_TOKEN" ]; then
  if [ -f "$SCRIPT_DIR/api_token" ]; then
    API_TOKEN=$(cat "$SCRIPT_DIR/api_token" | tr -d '[:space:]')
  else
    echo "WARNING: No Hive API token found. Skipping bootstrap."
    exit 0
  fi
fi

# Read agent identity
AGENT_NAME=$(python3 -c "import json; print(json.load(open('$SCRIPT_DIR/agent.json'))['name'])" 2>/dev/null || echo "plot-bot")
AGENT_DESC=$(python3 -c "import json; print(json.load(open('$SCRIPT_DIR/agent.json')).get('description', ''))" 2>/dev/null || echo "")

# Send bootstrap request
RESPONSE=$(curl -s --max-time 5 -X POST "$HIVE_URL/api/v1/bootstrap" \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"agent\": {
      \"name\": \"$AGENT_NAME\",
      \"description\": \"$AGENT_DESC\",
      \"working_directory\": \"$WORKSPACE_DIR\"
    }
  }")

# Check for errors
if echo "$RESPONSE" | python3 -c "import sys,json; d=json.loads(sys.stdin.read()); sys.exit(0 if 'error' in d else 1)" 2>/dev/null; then
  echo "WARNING: Hive bootstrap failed: $RESPONSE"
  exit 0
fi

echo "=== HIVE BOOTSTRAP ==="
echo "$RESPONSE" | python3 -m json.tool 2>/dev/null || echo "$RESPONSE"

# Rate control check
if command -v node >/dev/null 2>&1 && [ -f "$HOME/.claude/rate-control/claude_limits.mjs" ]; then
  RATE=$(node "$HOME/.claude/rate-control/claude_limits.mjs" --json 2>/dev/null)
  if [ -n "$RATE" ]; then
    PCT=$(echo "$RATE" | python3 -c "import sys,json; print(json.load(sys.stdin).get('windows',{}).get('7d',{}).get('pct',0))" 2>/dev/null)
    echo ""
    echo "=== RATE CONTROL: ${PCT}% weekly budget used ==="
  fi
fi

# Telema2 sync
TELEMA_URL="${TELEMA_URL:-http://127.0.0.1:8000}"
if curl -sk --max-time 2 "$TELEMA_URL/health" >/dev/null 2>&1; then
  TELEMA_BIN="$(dirname "$WORKSPACE_DIR")/telema2/tl"
  if [ -x "$TELEMA_BIN" ]; then
    # Extract and push bets
    python3 "$WORKSPACE_DIR/tools/scripts/extract_bets.py" -o /tmp/_plotbot_bets.json 2>/dev/null
    if [ -f /tmp/_plotbot_bets.json ]; then
      [ -f "$WORKSPACE_DIR/knowledge/domains.yaml" ] && \
        "$TELEMA_BIN" sync seed-domains "$WORKSPACE_DIR/knowledge/domains.yaml" >/dev/null 2>&1
      "$TELEMA_BIN" sync push-bets /tmp/_plotbot_bets.json >/dev/null 2>&1
      "$TELEMA_BIN" sync pull -o "$WORKSPACE_DIR/context/telema_cache.json" >/dev/null 2>&1
      CACHE_STATS=$(python3 -c "
import json
d=json.load(open('$WORKSPACE_DIR/context/telema_cache.json'))
print(f\"{len(d['domains'])}d {len(d['goals'])}g {len(d['tasks'])}t {len(d['metrics'])}m\")
" 2>/dev/null)
      [ -n "$CACHE_STATS" ] && echo "" && echo "=== TELEMA SYNC: $CACHE_STATS ==="
      rm -f /tmp/_plotbot_bets.json
    fi

    # Show pending tasks
    echo ""
    echo "=== PENDING TASKS (top 5) ==="
    "$TELEMA_BIN" tasks list --json 2>/dev/null | python3 -c "
import sys, json
tasks = json.load(sys.stdin)
# Filter pending, sort by priority score desc
pending = [t for t in tasks if t.get('status') == 'pending']
pending.sort(key=lambda t: t.get('priority_score', 0), reverse=True)
for t in pending[:5]:
    screen = t.get('screen_name', t.get('screen_id','')[:8])
    print(f\"  [{t.get('urgency','normal'):8s}] {t['title'][:70]}  ({screen})\")
print(f\"  ... {len(pending)} total pending\")
" 2>/dev/null
  fi
fi

echo "=== END HIVE BOOTSTRAP ==="
