#!/bin/bash
set -e

VM="zrb-vm"

echo "Find latest task output dir on VM and restore to testpool/restore_data"
LIST_CMD="ls -1t /home/ubuntu/zrb_base/task/testpool/backup_data || true"
# get full listing from VM, then pick first line locally to avoid multiline issues
LISTING=$(multipass exec "$VM" -- bash -lc "$LIST_CMD")
LATEST_DIR=$(printf "%s" "$LISTING" | head -n1)

if [ -z "$LATEST_DIR" ]; then
  echo "No backup task output found under /home/ubuntu/zrb_base/task/testpool/backup_data"
  exit 1
fi

REMOTE_TASK_DIR="/home/ubuntu/zrb_base/task/testpool/backup_data/$LATEST_DIR"
echo "Latest task dir: $REMOTE_TASK_DIR"

echo "Checking for age private key at /home/ubuntu/age_private_key.txt"
if multipass exec "$VM" -- test -f /home/ubuntu/age_private_key.txt; then
  KEY_PATH="/home/ubuntu/age_private_key.txt"
  echo "Using $KEY_PATH"
else
  echo "ERROR: no age private key found at /home/ubuntu/age_private_key.txt"
  exit 1
fi


echo "Recreating ZFS dataset testpool/restore_data (destroy if exists)"
multipass exec "$VM" -- sudo -n zfs destroy -r testpool/restore_data || true
multipass exec "$VM" -- sudo -n zfs create testpool/restore_data

echo "Applying backups in order (level0 .. levelN)"
LEVELS=$(multipass exec "$VM" -- bash -lc "ls -1 /home/ubuntu/zrb_base/task/testpool/backup_data | sort -V | grep '^level' || true")
if [ -z "$LEVELS" ]; then
  echo "No level directories found to restore"
  exit 1
fi

for d in $LEVELS; do
  echo "Applying $d"
  # Find the date subdirectory under the level directory
  DATE_DIRS=$(multipass exec "$VM" -- bash -lc "ls -1 /home/ubuntu/zrb_base/task/testpool/backup_data/$d | sort -V || true")
  if [ -z "$DATE_DIRS" ]; then
    echo "No date subdirectories found under $d"
    continue
  fi
  # Use the first (latest) date directory
  DATE_DIR=$(printf "%s" "$DATE_DIRS" | head -n1)
  REMOTE_DIR="/home/ubuntu/zrb_base/task/testpool/backup_data/$d/$DATE_DIR"
  multipass exec "$VM" -- bash -lc "cat $REMOTE_DIR/snapshot.part-* | age --decrypt -i $KEY_PATH | sudo -n zfs receive -F testpool/restore_data"
done

echo "Computing SHA256 sums of restored files"
multipass exec "$VM" -- bash -lc "cd /testpool/restore_data && sudo find . -type f -print0 | sudo xargs -0 sha256sum > /tmp/restore_sha256.txt && sudo chown ubuntu:ubuntu /tmp/restore_sha256.txt"

echo "Compare baseline and restore results (if baseline is at /tmp/baseline_sha256.txt)"
multipass exec "$VM" -- bash -lc "if [ -f /tmp/baseline_sha256.txt ]; then diff -u /tmp/baseline_sha256.txt /tmp/restore_sha256.txt || true; else echo 'No baseline found at /tmp/baseline_sha256.txt'; fi"

echo "Restore complete"
