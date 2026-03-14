#!/bin/bash
# Plot Bot — Log cycle result to Hive (POST /api/v1/logs)
#
# Usage: hive_log.sh <summary> [tokens] [duration_s] [outcome] [extra_json]
# Example: hive_log.sh "cycle 2: data collection" 14200 1800 success '{"rate_pct":0.7}'

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/../.." && pwd)"

# Load credentials
[ -f "$REPO_DIR/.env" ] && source "$REPO_DIR/.env"
HIVE_URL="${HIVE_API_URL:-https://spora.live}"
API_TOKEN=$(cat "$REPO_DIR/.claude/api_token" 2>/dev/null | tr -d '[:space:]')
API_TOKEN="${API_TOKEN:-$HIVE_API_TOKEN}"
AGENT_ID=$(cat "$REPO_DIR/.claude/agent_id" 2>/dev/null | tr -d '[:space:]')

SUMMARY="${1:?Usage: hive_log.sh <summary> [tokens] [duration_s] [outcome] [extra_json]}"
TOKENS="${2:-0}"
DURATION="${3:-0}"
OUTCOME="${4:-success}"
EXTRA="${5:-}"

if [ -z "$API_TOKEN" ] || [ -z "$AGENT_ID" ]; then
  echo "ERROR: Missing api_token or agent_id in .claude/"
  exit 1
fi

# Get rate control data
RATE_JSON=""
if command -v node >/dev/null 2>&1 && [ -f "$HOME/.claude/rate-control/claude_limits.mjs" ]; then
  RATE_JSON=$(node "$HOME/.claude/rate-control/claude_limits.mjs" --json 2>/dev/null || echo "")
fi

# Build JSON payload with full rate data
PAYLOAD=$(python3 -c "
import json, sys, os
d = {
    'summary': sys.argv[1],
    'tokens_spent': int(sys.argv[2]),
    'duration_seconds': int(sys.argv[3]),
    'outcome': sys.argv[4],
}
# Merge extra fields
extra = sys.argv[5] if len(sys.argv) > 5 and sys.argv[5] else '{}'
d.update(json.loads(extra))
# Add rate control data
rate_raw = sys.argv[6] if len(sys.argv) > 6 and sys.argv[6] else ''
if rate_raw:
    try:
        r = json.loads(rate_raw)
        w = r.get('windows', {}).get('7d', {})
        s = r.get('windows', {}).get('session', {})
        d['weekly_pct'] = w.get('pct', 0)
        d['weekly_cost_usd'] = w.get('spent', 0)
        d['weekly_budget_usd'] = w.get('limit', 0)
        d['days_left'] = w.get('days_left', 0)
        d['session_cost_usd'] = s.get('spent', 0)
        d['session_budget_usd'] = s.get('limit', 0)
        d['session_pct'] = s.get('pct', 0)
    except: pass
# Estimate cost from tokens (Opus: $15/M input, $75/M output; rough ~$45/M avg)
tokens = int(sys.argv[2])
d['cost_usd'] = round(tokens * 45 / 1_000_000, 2) if tokens > 0 else 0
# Add mode
d['mode'] = d.get('mode', os.environ.get('RATE_MODE', 'FULL'))
print(json.dumps(d))
" "$SUMMARY" "$TOKENS" "$DURATION" "$OUTCOME" "$EXTRA" "$RATE_JSON")

curl -s -X POST "$HIVE_URL/api/v1/logs" \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "X-Agent-Id: $AGENT_ID" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD"
