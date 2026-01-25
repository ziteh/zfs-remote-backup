#!/bin/bash
set -e

VM="zrb-vm"

echo "Creating test files in /testpool/backup_data on VM"
multipass exec "$VM" -- sudo bash -lc "mkdir -p /testpool/backup_data && chown -R ubuntu:ubuntu /testpool/backup_data"

# create sample files (idempotent)
multipass exec "$VM" -- bash -lc "
cd /testpool/backup_data
sudo bash -c 'dd if=/dev/urandom of=test-binary.bin bs=1M count=10 >/dev/null 2>&1 || true'
sudo bash -c 'mkdir -p test-dir && echo hello > test-dir/hello.txt'
echo 'file1' > test-file-1.txt
echo 'file2' > test-file-2.txt
"

echo "Compute baseline SHA256 sums -> /tmp/baseline_sha256.txt"
multipass exec "$VM" -- bash -lc "cd /testpool/backup_data && sudo find . -type f -print0 | sudo xargs -0 sha256sum > /tmp/baseline_sha256.txt && sudo chown ubuntu:ubuntu /tmp/baseline_sha256.txt"

echo "Create MinIO bucket 'zrb-test' via mc (if mc available)"
multipass exec "$VM" -- bash -lc "if command -v mc >/dev/null 2>&1; then mc alias set myminio http://127.0.0.1:9000 admin password123 >/dev/null 2>&1 || true; mc mb myminio/zrb-test >/dev/null 2>&1 || true; fi"

echo "Environment prepared."
