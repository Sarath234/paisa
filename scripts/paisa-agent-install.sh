#!/bin/bash
set -e

BINARY=$(which paisa-agent 2>/dev/null || echo "$(pwd)/paisa-agent")
CONFIG="$HOME/.config/paisa-agent/paisa-agent.yaml"
PLIST="$HOME/Library/LaunchAgents/com.paisa.agent.plist"
LOG_DIR="$HOME/.local/share/paisa-agent"

if [ ! -f "$BINARY" ]; then
  echo "paisa-agent binary not found. Run: go build ./cmd/paisa-agent/ first."
  exit 1
fi

mkdir -p "$LOG_DIR"
mkdir -p "$(dirname "$PLIST")"

TMPL="$(dirname "$0")/paisa-agent.plist.tmpl"
sed \
  -e "s|AGENT_BINARY_PATH|$BINARY|g" \
  -e "s|AGENT_CONFIG_PATH|$CONFIG|g" \
  -e "s|HOME_DIR|$HOME|g" \
  "$TMPL" > "$PLIST"

launchctl load "$PLIST"
echo "paisa-agent installed and started."
echo "Logs: $LOG_DIR/agent.log"
echo ""
echo "Next steps:"
echo "  1. Edit $CONFIG with your Paisa URL, Ollama model, Telegram token, and journal_dir"
echo "  2. Run: paisa-agent --config $CONFIG  (first run will prompt Gmail OAuth2)"
echo "  3. Add 'include auto-import.ledger' to your main Paisa journal file"
echo "  4. Set up iPhone Shortcut: Automation → 'When message from [HDFC-xxxx, ICICI-xxxx]'"
echo "     Action: URL Session POST to https://api.telegram.org/bot<TOKEN>/sendMessage"
echo "     Body: {\"chat_id\": <YOUR_CHAT_ID>, \"text\": \"Shortcut.input.Messages.last\"}"
