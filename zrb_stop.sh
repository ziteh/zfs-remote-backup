#!/bin/bash

# ZFS Remote Backup - Stop Script
# Usage: ./zrb_stop.sh <POOL> <DATASET>

if [ $# -lt 2 ]; then
  echo "Usage: $0 <POOL> <DATASET>"
  exit 1
fi

POOL="$1"
DATASET="$2"

# Try to get base_dir from config or use script directory
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [ -f "$SCRIPT_DIR/zrb_config.yaml" ]; then
  # Extract base_dir from config
  BASE_DIR=$(grep "^base_dir:" "$SCRIPT_DIR/zrb_config.yaml" | awk '{print $2}' | tr -d '"')
else
  BASE_DIR="$SCRIPT_DIR"
fi

LOCK_FILE="$BASE_DIR/run/$POOL/$DATASET/zrb.lock"

if [ ! -f "$LOCK_FILE" ]; then
  echo "Error: Lock file not found: $LOCK_FILE"
  echo "No backup process running for $POOL/$DATASET"
  exit 1
fi

# Extract PID from lock file
PID=$(grep "^pid:" "$LOCK_FILE" | awk '{print $2}')

if [ -z "$PID" ]; then
  echo "Error: Could not extract PID from lock file"
  cat "$LOCK_FILE"
  exit 1
fi

echo "Found backup process: PID=$PID"
echo "Lock file: $LOCK_FILE"

# Check if process exists
if ! ps -p "$PID" > /dev/null 2>&1; then
  echo "Warning: Process $PID does not exist"
  echo "Removing stale lock file..."
  rm -f "$LOCK_FILE"
  exit 0
fi

# Confirm kill
read -p "Kill backup process (PID=$PID)? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
  echo "Killing process $PID..."
  kill -15 "$PID"

  # Wait a bit for graceful shutdown
  sleep 2

  # Force kill if still running
  if ps -p "$PID" > /dev/null 2>&1; then
    echo "Process still running, force killing..."
    kill -9 "$PID"
    sleep 1
  fi

  # Remove lock file
  if [ -f "$LOCK_FILE" ]; then
    echo "Removing lock file..."
    rm -f "$LOCK_FILE"
  fi

  echo "Done."
else
  echo "Cancelled."
  exit 1
fi
