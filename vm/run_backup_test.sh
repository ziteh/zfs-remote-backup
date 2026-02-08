#!/usr/bin/env bash
set -euo pipefail

VM_NAME="zfs-minio"
BIN_LOCAL="$(cd "$(dirname "$0")/.." && pwd)/build/zrb_linux_arm64"
BIN_REMOTE="/tmp/zrb_linux_arm64"
CONFIG_LOCAL="$(cd "$(dirname "$0")" && pwd)/config.yaml"
CONFIG_REMOTE="/tmp/zrb_config.yaml"

if ! command -v multipass >/dev/null 2>&1; then
  echo "multipass not found; please install multipass"
  exit 1
fi

echo "Transferring config and binary to VM..."
multipass transfer "$CONFIG_LOCAL" "$VM_NAME:$CONFIG_REMOTE"

if [ -f "$BIN_LOCAL" ]; then
  multipass transfer "$BIN_LOCAL" "$VM_NAME:$BIN_REMOTE"
  multipass exec "$VM_NAME" -- sudo chmod +x "$BIN_REMOTE"
else
  echo "Binary not found at $BIN_LOCAL — build first with make build-linux" >&2
  exit 1
fi

# Configure mc alias non-interactively
multipass exec "$VM_NAME" -- /usr/local/bin/mc alias set myminio http://127.0.0.1:9000 admin password123 --insecure || true

echo "Create a fresh full snapshot inside VM"
multipass exec "$VM_NAME" -- sudo $BIN_REMOTE snapshot --pool testpool --dataset backup_data --prefix zrb_full 2>&1 || true

echo "Run full backup (will upload to MinIO)"
multipass exec "$VM_NAME" -- sudo bash -c 'export AWS_ACCESS_KEY_ID=admin AWS_SECRET_ACCESS_KEY=password123 && /tmp/zrb_linux_arm64 backup --config /tmp/zrb_config.yaml --type full --task daily_backup' 2>&1 || true

echo "Wait and show recent log lines"
multipass exec "$VM_NAME" -- sudo bash -c 'sleep 5; L=$(ls -1t /tmp/zrb_backups/logs/testpool/backup_data/*.log | head -1); tail -40 "$L" | tail -20' || true

echo "Validate uploads in MinIO"
multipass exec "$VM_NAME" -- /usr/local/bin/mc ls --recursive myminio/zfs-backups --insecure 2>&1 || true

echo "Show local run directory and last backup manifest"
multipass exec "$VM_NAME" -- sudo ls -lh /tmp/zrb_backups/run/testpool/backup_data/ || true
multipass exec "$VM_NAME" -- sudo cat /tmp/zrb_backups/run/testpool/backup_data/last_backup_manifest.yaml || true

echo "Make a small change for diff backup test"
multipass exec "$VM_NAME" -- sudo bash -c 'echo "New data for differential backup" > /testpool/backup_data/new-test-file.txt && date >> /testpool/backup_data/new-test-file.txt' 2>&1 || true

echo "Create diff snapshot"
multipass exec "$VM_NAME" -- sudo $BIN_REMOTE snapshot --pool testpool --dataset backup_data --prefix zrb_diff 2>&1 || true

echo "Run diff backup"
multipass exec "$VM_NAME" -- sudo bash -c 'export AWS_ACCESS_KEY_ID=admin AWS_SECRET_ACCESS_KEY=password123 && /tmp/zrb_linux_arm64 backup --config /tmp/zrb_config.yaml --type diff --task daily_backup' 2>&1 || true

echo "Wait and show recent log lines for diff"
multipass exec "$VM_NAME" -- sudo bash -c 'sleep 3; L=$(ls -1t /tmp/zrb_backups/logs/testpool/backup_data/*.log | head -1); tail -20 "$L"' || true

echo "Final list of MinIO objects:"
multipass exec "$VM_NAME" -- /usr/local/bin/mc ls --recursive myminio/zfs-backups --insecure 2>&1 || true

echo "Show final last_backup_manifest.yaml"
multipass exec "$VM_NAME" -- sudo cat /tmp/zrb_backups/run/testpool/backup_data/last_backup_manifest.yaml || true

echo "Test script completed."
exit 0
