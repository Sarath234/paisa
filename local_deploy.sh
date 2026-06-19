#!/usr/bin/env bash
# Build and restart the local paisa stack.
# Run from the repo root (launches ./paisa and ./paisa-agent).

set -ue

LOG_DIR="$HOME/Documents/paisa"
mkdir -p "$LOG_DIR"

echo "building frontend..."
npm run build 2>&1 | tail -3

echo "building paisa..."
go build -o paisa .

echo "building paisa-agent..."
go build -o paisa-agent ./cmd/paisa-agent/

pkill -x paisa && echo "stopped paisa" || echo "paisa was not running"
pkill -x paisa-agent && echo "stopped paisa-agent" || echo "paisa-agent was not running"

sleep 1

nohup ./paisa serve >> "$LOG_DIR/log.txt" 2>&1 &
echo "paisa launched"

nohup ./paisa-agent --config "$LOG_DIR/paisa-agent.yaml" >> "$LOG_DIR/log-agent.txt" 2>&1 &
echo "paisa-agent launched"

sleep 2
echo "paisa PID: $(pgrep -x paisa || echo MISSING)"
echo "paisa-agent PID: $(pgrep -x paisa-agent || echo MISSING)"
