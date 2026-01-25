#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_simple_config.yaml"

echo "Creating config directly on VM"
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
YAML"

multipass exec "$VM" -- sudo zfs snapshot testpool/backup_data@zrb_level0_$(date +%s) || true

multipass exec "$VM" -- sudo /tmp/zrb_simple backup --config "$CONFIG_REMOTE" --task test_backup --level 0

echo "L0 backup completed. Check /home/ubuntu/zrb_base/task/... for output"
