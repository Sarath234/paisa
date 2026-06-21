#!/usr/bin/env bash
# Usage: ./scripts/deploy-mac.sh [path/to/paisa-cli-macos-amd64]
# If no path given, picks the most recent paisa-cli-macos-* from ~/Downloads.

set -euo pipefail

LOG="$HOME/Documents/paisa/log.txt"
LOG_AGENT="$HOME/Documents/paisa/log-agent.txt"
INSTALL_PATH="/usr/local/bin/paisa"
INSTALL_PATH_AGENT="/usr/local/bin/paisa-agent"
CONFIG_AGENT="$HOME/Documents/paisa/paisa-agent.yaml"

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

# --- Locate binary ---
if [ $# -ge 1 ]; then
  BINARY_AGENT="$1"
else
  BINARY_AGENT=$(ls -t "$HOME/Downloads"/paisa-agent-macos-* 2>/dev/null | head -1)
  if [ -z "$BINARY_AGENT" ]; then
    echo "Error: no paisa-agent-macos-* file found in ~/Downloads"
    echo "Usage: $0 [path/to/binary]"
    exit 1
  fi
fi

echo "Using binary: $BINARY_AGENT"

# --- Stop running paisa ---
if pgrep -x paisa > /dev/null 2>&1; then
  echo "Stopping paisa (PID $(pgrep -x paisa))..."
  pkill -x paisa
  sleep 1
else
  echo "No running paisa process found."
fi

if pgrep -x paisa-agent > /dev/null 2>&1; then
  echo "Stopping paisa-agent (PID $(pgrep -x paisa-agent))..."
  pkill -x paisa-agent
  sleep 1
else
  echo "No running paisa-agent process found."
fi

# --- Install ---
chmod u+x "$BINARY"
echo "Installing to $INSTALL_PATH..."
sudo mv "$BINARY" "$INSTALL_PATH"
sudo xattr -rd com.apple.quarantine $INSTALL_PATH

chmod u+x "$BINARY_AGENT"
echo "Installing to $INSTALL_PATH_AGENT..."
sudo mv "$BINARY_AGENT" "$INSTALL_PATH_AGENT"
sudo xattr -rd com.apple.quarantine $INSTALL_PATH_AGENT

# --- Start ---
mkdir -p "$(dirname "$LOG")"
echo "Starting paisa serve → $LOG"
nohup paisa serve >> "$LOG" 2>&1 &
echo "Done. paisa started (PID $!)"

mkdir -p "$(dirname "$LOG_AGENT")"
echo "Starting paisa serve → $LOG_AGENT"
nohup paisa-agent --config $CONFIG_AGENT>> "$LOG_AGENT" 2>&1 &
echo "Done. paisa-agent started (PID $!)"
