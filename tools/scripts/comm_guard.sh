#!/bin/bash
# Communication Guard — PreToolUse hook for Bash tool.
# Blocks outbound messaging to anyone except allowed endpoints.
#
# Exit codes:
#   0 = allow
#   2 = block (stderr shown to model as reason)
#
# Receives tool input as JSON on stdin:
#   {"tool_name": "Bash", "tool_input": {"command": "..."}}

set -euo pipefail

# Read tool input from stdin
INPUT=$(cat)
COMMAND=$(echo "$INPUT" | python3 -c "
import json, sys
d = json.load(sys.stdin)
print(d.get('tool_input', {}).get('command', ''))
" 2>/dev/null || echo "")

# No command = allow (non-Bash tool or parse error)
[ -z "$COMMAND" ] && exit 0

# --- WHITELIST: allowed external endpoints ---
# Data sources (ArcGIS, NAPR, Place.ge, SS.ge, Copernicus)
# Hive operations (spora.live — bootstrap, escalation, logging)
# GitHub (git push/pull)

# --- BLOCKLIST: outbound messaging patterns ---
# Each pattern = potential communication with humans

BLOCKED_PATTERNS=(
    # Telegram Bot API
    'api.telegram.org'
    'sendMessage'
    'sendDocument'
    'sendPhoto'

    # Slack
    'hooks.slack.com'
    'api.slack.com'
    'slack.com/api'

    # Email
    'sendmail'
    'mail\b'
    '/usr/sbin/sendmail'
    'smtp://'
    'smtps://'

    # Discord
    'discord.com/api'
    'discordapp.com'

    # WhatsApp
    'graph.facebook.com.*whatsapp'
    'wa.me'

    # Generic messaging patterns
    'send.*message'
    'post.*comment'
    'reply.*to'

    # Hive messaging (ONLY hive-send.sh is blocked; escalation allowed)
    'hive-send.sh'
    'hive.*reply'
    '/api/v1/messages'
    'action.*send_message'

    # SSH to external hosts (allow only known Mac Mini)
    # Note: bot shouldn't SSH anywhere
    'ssh\b'

    # Generic HTTP POST to unknown messaging endpoints
    'twilio.com'
    'mailgun'
    'sendgrid'
    'postmark'
    'amazonaws.com/ses'
)

# Check command against each blocked pattern
CMD_LOWER=$(echo "$COMMAND" | tr '[:upper:]' '[:lower:]')

for pattern in "${BLOCKED_PATTERNS[@]}"; do
    if echo "$CMD_LOWER" | grep -qiE "$pattern"; then
        # Exception: spora.live/api/v1/escalate is ALLOWED (escalation to Vadim)
        if echo "$CMD_LOWER" | grep -qE 'spora\.live/api/v1/escalate'; then
            exit 0
        fi
        # Exception: spora.live/api/v1/logs is ALLOWED (loop logging)
        if echo "$CMD_LOWER" | grep -qE 'spora\.live/api/v1/logs'; then
            exit 0
        fi
        # Exception: git push/pull are ALLOWED
        if echo "$CMD_LOWER" | grep -qE '^git (push|pull|fetch|clone)'; then
            exit 0
        fi

        # BLOCK
        echo "🚫 COMMUNICATION GUARD: BLOCKED" >&2
        echo "" >&2
        echo "Command matched blocked pattern: $pattern" >&2
        echo "Command: $(echo "$COMMAND" | head -c 200)" >&2
        echo "" >&2
        echo "ABSOLUTE RULE: You NEVER communicate with anyone except Vadim." >&2
        echo "No messages, no emails, no API calls to messaging services." >&2
        echo "If you need external communication → escalate to Vadim:" >&2
        echo '  curl -s -X POST "https://spora.live/api/v1/escalate" ...' >&2
        echo "" >&2
        echo "This action has been BLOCKED and logged." >&2
        exit 2
    fi
done

# No blocked pattern matched = allow
exit 0
