#!/bin/bash

if [ $# -lt 2 ]; then
  echo "Usage: $0 <TASK_NAME> <BACKUP_LEVEL>"
  exit 1
fi

# AWS
export AWS_ACCESS_KEY_ID="your-access-key-id"
export AWS_SECRET_ACCESS_KEY="your-secret-access-key"
export AWS_DEFAULT_REGION="us-east-1"

# Path
ZRB_BIN="zrb_simple"
ZRB_CONFIG="zrb_simple_config.yaml"

# Task
TASK_NAME="$1"
BACKUP_LEVEL="$2"

sudo -E "$ZRB_BIN" backup \
  --config "$ZRB_CONFIG" \
  --task "$TASK_NAME" \
  --level "$BACKUP_LEVEL"
