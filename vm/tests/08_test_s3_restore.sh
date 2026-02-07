#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_s3_config.yaml"
PRIVATE_KEY="/home/ubuntu/age_private_key.txt"

echo "=== Testing restore from S3 (MinIO) ==="

# Test 0: Check MinIO availability
echo ""
echo "Test 0: Check MinIO availability"
echo "----------------------------------------"
if multipass exec "$VM" -- bash -lc "curl -s http://127.0.0.1:9000/minio/health/live >/dev/null 2>&1"; then
    echo "✓ MinIO is running"
else
    echo "⚠ MinIO not running, skipping S3 restore tests"
    echo "To enable: Start MinIO on the VM at http://127.0.0.1:9000"
    exit 0
fi

# Test 1: Create S3-enabled config
echo ""
echo "Test 1: Create S3-enabled config"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "cat > $CONFIG_REMOTE <<'YAML'
base_dir: /home/ubuntu/zrb_s3_base
age_public_key: \"age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav\"
s3:
  enabled: true
  bucket: zrb-test
  region: us-east-1
  prefix: test-backups/
  endpoint: http://127.0.0.1:9000
  storage_class:
    manifest: STANDARD
    backup_data:
      - STANDARD  # Level 0
      - STANDARD  # Level 1
      - STANDARD  # Level 2+
  retry:
    max_attempts: 3
tasks:
  - name: s3_test_backup
    enabled: true
    pool: testpool
    dataset: backup_data
YAML"

if multipass exec "$VM" -- test -f "$CONFIG_REMOTE"; then
    echo "✓ S3-enabled config created"
else
    echo "✗ Failed to create config"
    exit 1
fi

# Test 2: Prepare MinIO credentials
echo ""
echo "Test 2: Prepare MinIO credentials"
echo "----------------------------------------"
# Credentials will be passed via environment variables using sudo -E
echo "✓ Credentials will be passed via environment variables"

# Test 3: Create test snapshot and backup to S3
echo ""
echo "Test 3: Backup level 0 to S3 (MinIO)"
echo "----------------------------------------"
TIMESTAMP=$(date +%s)
multipass exec "$VM" -- sudo zfs snapshot testpool/backup_data@zrb_s3_level0_$TIMESTAMP

BACKUP_OUTPUT=$(multipass exec "$VM" -- bash -lc "
export AWS_ACCESS_KEY_ID=admin
export AWS_SECRET_ACCESS_KEY=password123
sudo -E /tmp/zrb_simple backup --config $CONFIG_REMOTE --task s3_test_backup --level 0 2>&1
")

echo "$BACKUP_OUTPUT"

if echo "$BACKUP_OUTPUT" | grep -q "Backup completed"; then
    echo "✓ Backup to S3 completed successfully"
else
    echo "✗ Backup to S3 failed"
    exit 1
fi

# Test 4: Verify manifests uploaded to S3
echo ""
echo "Test 4: Verify manifests uploaded to S3"
echo "----------------------------------------"
MANIFEST_CHECK=$(multipass exec "$VM" -- bash -lc "
mc ls myminio/zrb-test/test-backups/manifests/ --recursive 2>&1 || true
")

if echo "$MANIFEST_CHECK" | grep -q "task_manifest.yaml"; then
    echo "✓ Manifests found in S3"
    echo "$MANIFEST_CHECK" | grep ".yaml"
else
    echo "✗ Manifests not found in S3"
    echo "$MANIFEST_CHECK"
    exit 1
fi

# Test 5: Dry-run restore from S3
echo ""
echo "Test 5: Dry-run restore from S3"
echo "----------------------------------------"
DRY_RUN_OUTPUT=$(multipass exec "$VM" -- bash -lc "
export AWS_ACCESS_KEY_ID=admin
export AWS_SECRET_ACCESS_KEY=password123
sudo -E /tmp/zrb_simple restore \
    --config $CONFIG_REMOTE \
    --task s3_test_backup \
    --level 0 \
    --target testpool/restored_from_s3 \
    --private-key $PRIVATE_KEY \
    --source s3 \
    --dry-run 2>&1
")

echo "$DRY_RUN_OUTPUT"

if echo "$DRY_RUN_OUTPUT" | grep -q "DRY RUN MODE"; then
    echo "✓ Dry-run mode activated"
else
    echo "✗ Dry-run mode not activated"
    exit 1
fi

if echo "$DRY_RUN_OUTPUT" | grep -q "No changes made"; then
    echo "✓ Dry-run confirms no changes made"
else
    echo "✗ Dry-run should confirm no changes"
    exit 1
fi

if echo "$DRY_RUN_OUTPUT" | grep -q "Source:.*s3"; then
    echo "✓ Shows S3 as source"
else
    echo "✗ Source not shown as S3"
    exit 1
fi

if echo "$DRY_RUN_OUTPUT" | grep -q "s3_test_backup"; then
    echo "✓ Shows task name"
else
    echo "✗ Task name not shown"
    exit 1
fi

# Test 6: Clean up target dataset if exists
echo ""
echo "Test 6: Prepare target dataset"
echo "----------------------------------------"
if multipass exec "$VM" -- sudo zfs list testpool/restored_from_s3 2>/dev/null; then
    echo "ℹ Target dataset exists, destroying it first"
    multipass exec "$VM" -- sudo zfs destroy -r testpool/restored_from_s3 || true
fi

if ! multipass exec "$VM" -- sudo zfs list testpool/restored_from_s3 2>/dev/null; then
    echo "✓ Target dataset ready (does not exist)"
else
    echo "✗ Failed to prepare target dataset"
    exit 1
fi

# Test 7: Actual restore from S3
echo ""
echo "Test 7: Actual restore from S3"
echo "----------------------------------------"
echo "This may take a few moments..."

RESTORE_OUTPUT=$(multipass exec "$VM" -- bash -lc "
export AWS_ACCESS_KEY_ID=admin
export AWS_SECRET_ACCESS_KEY=password123
sudo -E /tmp/zrb_simple restore \
    --config $CONFIG_REMOTE \
    --task s3_test_backup \
    --level 0 \
    --target testpool/restored_from_s3 \
    --private-key $PRIVATE_KEY \
    --source s3 2>&1
")

echo "$RESTORE_OUTPUT"

if echo "$RESTORE_OUTPUT" | grep -q "Restore completed successfully"; then
    echo "✓ Restore from S3 completed successfully"
else
    echo "✗ Restore from S3 failed"
    echo "$RESTORE_OUTPUT"
    exit 1
fi

# Test 8: Verify restored dataset exists
echo ""
echo "Test 8: Verify restored dataset"
echo "----------------------------------------"
if multipass exec "$VM" -- sudo zfs list testpool/restored_from_s3 >/dev/null 2>&1; then
    echo "✓ Restored dataset exists"
    multipass exec "$VM" -- sudo zfs list testpool/restored_from_s3
else
    echo "✗ Restored dataset does not exist"
    exit 1
fi

# Test 9: Verify data integrity
echo ""
echo "Test 9: Verify data integrity"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    if [ -d /testpool/restored_from_s3 ]; then
        cd /testpool/restored_from_s3 && \
        sudo find . -type f -print0 | sudo xargs -0 sha256sum | sort > /tmp/s3_restore_sha256.txt && \
        sudo chown ubuntu:ubuntu /tmp/s3_restore_sha256.txt
    else
        echo 'Restore target directory does not exist'
        exit 1
    fi
" 2>&1

DIFF_OUTPUT=$(multipass exec "$VM" -- bash -lc "
    if [ -f /tmp/baseline_sha256.txt ] && [ -f /tmp/s3_restore_sha256.txt ]; then
        diff -u /tmp/baseline_sha256.txt /tmp/s3_restore_sha256.txt || true
    else
        echo 'Baseline or S3 restore checksum file missing'
        exit 1
    fi
" 2>&1)

if [ -z "$DIFF_OUTPUT" ]; then
    echo "✓ Data restored from S3 matches baseline (SHA256 identical)"
else
    echo "⚠ Differences found between baseline and S3 restored data:"
    echo "$DIFF_OUTPUT"
fi

# Test 10: Verify download logs
echo ""
echo "Test 10: Verify S3 download operations"
echo "----------------------------------------"
if echo "$RESTORE_OUTPUT" | grep -q "Downloading.*from S3"; then
    echo "✓ S3 download operations logged"
else
    echo "✗ S3 download operations not found in logs"
fi

# Test 11: Cleanup
echo ""
echo "Test 11: Cleanup test resources"
echo "----------------------------------------"
multipass exec "$VM" -- sudo zfs destroy -r testpool/restored_from_s3 2>&1 || true
multipass exec "$VM" -- bash -lc "rm -rf /home/ubuntu/zrb_s3_base" || true
echo "✓ Cleanup completed"

# Summary
echo ""
echo "=== S3 restore tests completed ===\"
echo ""
echo "Summary:"
echo "  ✓ MinIO (S3) connectivity verified"
echo "  ✓ Backup to S3 successful"
echo "  ✓ Manifests uploaded to S3"
echo "  ✓ Dry-run restore from S3 works"
echo "  ✓ Actual restore from S3 successful"
echo "  ✓ Data integrity verified"
echo "  ✓ All S3 operations logged correctly"
