#!/usr/bin/env bash
# Usage: ./scripts/deploy-mac.sh [path/to/paisa-cli-macos-amd64]
# If no path given, picks the most recent paisa-cli-macos-* from ~/Downloads.

set -euo pipefail

LOG="$HOME/Documents/paisa/log.txt"
INSTALL_PATH="/usr/local/bin/paisa"

# Prompt for sudo password upfront
sudo -v

# --- Locate binary ---
if [ $# -ge 1 ]; then
  BINARY="$1"
else
  BINARY=$(ls -t "$HOME/Downloads"/paisa-cli-macos-* 2>/dev/null | head -1)
  if [ -z "$BINARY" ]; then
    echo "Error: no paisa-cli-macos-* file found in ~/Downloads"
    echo "Usage: $0 [path/to/binary]"
    exit 1
  fi
fi

echo "Using binary: $BINARY"

# --- Stop running paisa ---
if pgrep -x paisa > /dev/null 2>&1; then
  echo "Stopping paisa (PID $(pgrep -x paisa))..."
  pkill -x paisa
  sleep 1
else
  echo "No running paisa process found."
fi

# --- Install ---
chmod u+x "$BINARY"
# Remove macOS Gatekeeper quarantine flag set on downloaded files
xattr -d com.apple.quarantine "$BINARY" 2>/dev/null || true
echo "Installing to $INSTALL_PATH..."
sudo mv "$BINARY" "$INSTALL_PATH"

# --- Start ---
mkdir -p "$(dirname "$LOG")"
echo "Starting paisa serve → $LOG"
nohup paisa serve >> "$LOG" 2>&1 &
echo "Done. paisa started (PID $!)"
