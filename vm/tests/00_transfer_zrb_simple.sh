#!/bin/bash
set -e

# Transfer local zrb binary to VM and install to /usr/local/bin
VM="zrb-vm"
LOCAL_BIN="/Users/klein/ws/zfs-remote-backup/build/zrb_linux_arm64"
REMOTE_TMP="/tmp/zrb_temp"

if [ ! -f "$LOCAL_BIN" ]; then
  echo "Local binary not found: $LOCAL_BIN"
  exit 1
fi

echo "Transferring $LOCAL_BIN to $VM:/tmp/"
multipass transfer "$LOCAL_BIN" "$VM:$REMOTE_TMP"
echo "Installing on VM as /tmp/zrb"
multipass exec "$VM" -- sudo mv "$REMOTE_TMP" /tmp/zrb
multipass exec "$VM" -- sudo chmod +x /tmp/zrb

echo "Done. Run ./01_prepare_env.sh next."
