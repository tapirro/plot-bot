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

# Build JSON payload
PAYLOAD=$(python3 -c "
import json, sys
d = {
    'summary': sys.argv[1],
    'tokens_spent': int(sys.argv[2]),
    'duration_seconds': int(sys.argv[3]),
    'outcome': sys.argv[4],
}
extra = sys.argv[5] if len(sys.argv) > 5 and sys.argv[5] else '{}'
d.update(json.loads(extra))
print(json.dumps(d))
" "$SUMMARY" "$TOKENS" "$DURATION" "$OUTCOME" "$EXTRA")

curl -s -X POST "$HIVE_URL/api/v1/logs" \
  -H "Authorization: Bearer $API_TOKEN" \
  -H "X-Agent-Id: $AGENT_ID" \
  -H "Content-Type: application/json" \
  -d "$PAYLOAD"
