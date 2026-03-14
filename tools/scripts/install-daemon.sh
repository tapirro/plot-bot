#!/bin/bash
# Install Plot Bot as a launchd daemon on macOS.
# Run once on the target machine (Mac Mini).
#
# Prerequisites:
#   1. claude CLI installed (npm install -g @anthropic-ai/claude-code)
#   2. claude auth login (OAuth with Max subscription)
#   3. claude setup-token (long-lived token for headless use)
#
# Usage:
#   ./tools/scripts/install-daemon.sh
#
set -euo pipefail

REPO_DIR="$(cd "$(dirname "$0")/../.." && pwd)"
PLIST_SRC="$REPO_DIR/tools/scripts/com.mantissa.plot-bot.plist"
PLIST_DST="$HOME/Library/LaunchAgents/com.mantissa.plot-bot.plist"
LOG_DIR="$REPO_DIR/logs"

echo "=== Plot Bot Daemon Installer ==="
echo "Repo: $REPO_DIR"
echo ""

# Pre-checks
if ! command -v claude &>/dev/null; then
  echo "ERROR: 'claude' CLI not found in PATH."
  echo "  Install: npm install -g @anthropic-ai/claude-code"
  exit 1
fi

# Check Claude auth
AUTH_STATUS=$(claude auth status 2>&1 || true)
if echo "$AUTH_STATUS" | grep -q '"loggedIn": true'; then
  SUB_TYPE=$(echo "$AUTH_STATUS" | python3 -c "import sys,json; print(json.load(sys.stdin).get('subscriptionType','unknown'))" 2>/dev/null || echo "unknown")
  echo "Claude auth: OK (subscription: $SUB_TYPE)"
else
  echo "ERROR: Not logged in to Claude."
  echo "  Run: claude auth login"
  echo "  Then: claude setup-token"
  exit 1
fi

# Warn if ANTHROPIC_API_KEY is set (would override subscription)
if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  echo ""
  echo "WARNING: ANTHROPIC_API_KEY is set in environment!"
  echo "  This will OVERRIDE your Max subscription and use pay-per-use billing."
  echo "  Unset it: unset ANTHROPIC_API_KEY"
  echo "  Remove from ~/.zshrc if present."
  echo ""
fi

# Create logs directory
mkdir -p "$LOG_DIR"

# Unload existing if running
if launchctl list 2>/dev/null | grep -q com.mantissa.plot-bot; then
  echo "Stopping existing daemon..."
  launchctl unload "$PLIST_DST" 2>/dev/null || true
fi

# Install plist
cp "$PLIST_SRC" "$PLIST_DST"

# Load
launchctl load "$PLIST_DST"
echo ""
echo "=== Installed ==="
echo "  Status:  launchctl list | grep plot-bot"
echo "  Logs:    tail -f $LOG_DIR/runner.log"
echo "  Stop:    launchctl unload $PLIST_DST"
echo "  Start:   launchctl load $PLIST_DST"
echo ""

# Verify
sleep 2
if launchctl list 2>/dev/null | grep -q com.mantissa.plot-bot; then
  echo "Plot Bot daemon is RUNNING."
else
  echo "WARNING: Daemon may not have started. Check logs."
fi
