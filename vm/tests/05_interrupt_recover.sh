#!/bin/bash
set -e

# Start a backup and kill it after a short delay to simulate interruption,
# then re-run the same backup to verify recovery using backup_state.yaml

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_config.yaml"

echo "Ensure config exists at $CONFIG_REMOTE on VM"
if ! multipass exec "$VM" -- test -f "$CONFIG_REMOTE"; then
  echo "Config not found on VM. Create or transfer it first (see 02_l0_backup.sh)."
  exit 1
fi

echo "Starting L2 backup in background on VM"
multipass exec "$VM" -- bash -lc "sudo /tmp/zrb backup --config $CONFIG_REMOTE --task test_backup --level 2 & echo \$! > /tmp/zrb_backup_pid && sleep 2"

PID=$(multipass exec "$VM" -- cat /tmp/zrb_backup_pid)
echo "Backup PID on VM: $PID"

echo "Sleeping 3s then killing PID to simulate interruption"
sleep 3
multipass exec "$VM" -- sudo kill -9 "$PID" || true

echo "Re-run L2 backup to verify it resumes/completes"
multipass exec "$VM" -- sudo /tmp/zrb backup --config "$CONFIG_REMOTE" --task test_backup --level 2

echo "Interrupted backup recovery test completed"
