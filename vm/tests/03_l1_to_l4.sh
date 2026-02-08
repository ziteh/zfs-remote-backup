#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_config.yaml"

  echo "Creating config directly on VM for incremental backups"
  multipass exec "$VM" -- bash -lc "cat > $CONFIG_REMOTE <<'YAML'
base_dir: /home/ubuntu/zrb_base
age_public_key: \"age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav\"
s3:
  enabled: false
  bucket: \"\"
  region: \"\"
  prefix: \"\"
  endpoint: \"\"
  storage_class:
    backup_data: []
    manifest: \"\"
tasks:
  - name: test_backup
    enabled: true
    pool: testpool
    dataset: backup_data
YAML" || true

for level in 1 2 3 4; do
  echo "-- Modify data for level $level --"
  multipass exec "$VM" -- sudo bash -lc "echo modified-$level >> /testpool/backup_data/test-file-1.txt"
  echo "Running backup level $level"
  # create a snapshot for this level to be used by the backup
  multipass exec "$VM" -- sudo bash -lc "zfs snapshot testpool/backup_data@zrb_level${level}_$(date +%s) || true"
  multipass exec "$VM" -- sudo /tmp/zrb backup --config "$CONFIG_REMOTE" --task test_backup --level $level
done

echo "L1-L4 backups completed"
