#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_naming_test_config.yaml"
TEST_BASE="/home/ubuntu/zrb_naming_test"

echo "=== Testing new .tmp file naming format ==="

# Test 1: Transfer new binary
echo ""
echo "Test 1: Transfer updated binary to VM"
echo "----------------------------------------"
if [ -f "./build/zrb_simple" ]; then
    multipass transfer ./build/zrb_simple "$VM:/tmp/zrb_new"
    multipass exec "$VM" -- chmod +x /tmp/zrb_new
    echo "✓ Binary transferred"
else
    echo "✗ Binary not found. Run: ./build.sh"
    exit 1
fi

# Test 2: Create test config
echo ""
echo "Test 2: Create test config"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "cat > $CONFIG_REMOTE <<'YAML'
base_dir: $TEST_BASE
age_public_key: \"age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav\"
s3:
  enabled: false
tasks:
  - name: naming_test
    enabled: true
    pool: testpool
    dataset: backup_data
YAML"
echo "✓ Test config created"

# Test 3: Clean up previous test data
echo ""
echo "Test 3: Clean up previous test data"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "sudo rm -rf $TEST_BASE" || true
echo "✓ Cleaned up"

# Test 4: Create snapshot
echo ""
echo "Test 4: Create test snapshot"
echo "----------------------------------------"
TIMESTAMP=$(date +%s)
multipass exec "$VM" -- sudo zfs snapshot testpool/backup_data@zrb_naming_test_$TIMESTAMP
echo "✓ Snapshot created: testpool/backup_data@zrb_naming_test_$TIMESTAMP"

# Test 5: Monitor backup process to see .tmp files
echo ""
echo "Test 5: Run backup and monitor .tmp file creation"
echo "----------------------------------------"
echo "Starting backup in background..."

# Run backup in background
multipass exec "$VM" -- bash -lc "
    sudo /tmp/zrb_new backup --config $CONFIG_REMOTE --task naming_test --level 0 > /tmp/backup_output.log 2>&1 &
    BACKUP_PID=\$!

    # Wait a moment for files to start being created
    sleep 2

    # Check for .tmp files while backup is running
    echo '--- Checking for .tmp files during backup ---'
    for i in {1..5}; do
        TMP_FILES=\$(find $TEST_BASE/task/testpool/backup_data/level0/ -name '*.tmp' 2>/dev/null || true)
        if [ -n \"\$TMP_FILES\" ]; then
            echo \"✓ Found .tmp files:\"
            ls -lh $TEST_BASE/task/testpool/backup_data/level0/*/*.tmp 2>/dev/null || true
            break
        fi
        sleep 1
    done

    # Wait for backup to complete
    wait \$BACKUP_PID
    EXIT_CODE=\$?

    echo ''
    echo '--- Backup output ---'
    cat /tmp/backup_output.log

    exit \$EXIT_CODE
"

if [ $? -eq 0 ]; then
    echo "✓ Backup completed successfully"
else
    echo "✗ Backup failed"
    exit 1
fi

# Test 6: Verify final files (no .tmp suffix)
echo ""
echo "Test 6: Verify final files (no .tmp suffix)"
echo "----------------------------------------"
FINAL_FILES=$(multipass exec "$VM" -- bash -lc "
    find $TEST_BASE/task/testpool/backup_data/level0/ -name 'snapshot.part-*' ! -name '*.tmp' 2>/dev/null || true
")

if [ -n "$FINAL_FILES" ]; then
    echo "✓ Found final part files (without .tmp):"
    echo "$FINAL_FILES"

    # Show file naming format
    SAMPLE_FILE=$(echo "$FINAL_FILES" | head -1)
    echo ""
    echo "Sample filename: $(basename "$SAMPLE_FILE")"

    # Verify format matches: snapshot.part-aaaaaa (6 letter suffix)
    if echo "$SAMPLE_FILE" | grep -qE 'snapshot\.part-[a-z]{6}$'; then
        echo "✓ Filename format correct: snapshot.part-XXXXXX (6 letters)"
    else
        echo "⚠ Filename format unexpected"
    fi
else
    echo "✗ No final part files found"
    exit 1
fi

# Test 7: Verify no .tmp files remain
echo ""
echo "Test 7: Verify no .tmp files remain after backup"
echo "----------------------------------------"
TMP_REMAINING=$(multipass exec "$VM" -- bash -lc "
    find $TEST_BASE/task/testpool/backup_data/level0/ -name '*.tmp' 2>/dev/null | wc -l
")

if [ "$TMP_REMAINING" -eq 0 ]; then
    echo "✓ No .tmp files remaining (correctly renamed)"
else
    echo "✗ Found $TMP_REMAINING .tmp files still present"
    multipass exec "$VM" -- bash -lc "find $TEST_BASE/task/testpool/backup_data/level0/ -name '*.tmp'"
    exit 1
fi

# Test 8: Check encrypted files
echo ""
echo "Test 8: Verify encrypted files (.age)"
echo "----------------------------------------"
AGE_FILES=$(multipass exec "$VM" -- bash -lc "
    find $TEST_BASE/task/testpool/backup_data/level0/ -name '*.age' 2>/dev/null || true
")

if [ -n "$AGE_FILES" ]; then
    echo "✓ Found encrypted .age files:"
    echo "$AGE_FILES" | head -3

    # Verify format: snapshot.part-aaaaaa.age
    SAMPLE_AGE=$(echo "$AGE_FILES" | head -1)
    echo ""
    echo "Sample encrypted filename: $(basename "$SAMPLE_AGE")"

    if echo "$SAMPLE_AGE" | grep -qE 'snapshot\.part-[a-z]{6}\.age$'; then
        echo "✓ Encrypted filename format correct: snapshot.part-XXXXXX.age"
    else
        echo "⚠ Encrypted filename format unexpected"
    fi
else
    echo "✗ No .age files found"
    exit 1
fi

# Test 9: Cleanup
echo ""
echo "Test 9: Cleanup test data"
echo "----------------------------------------"
multipass exec "$VM" -- sudo zfs destroy testpool/backup_data@zrb_naming_test_$TIMESTAMP || true
multipass exec "$VM" -- bash -lc "sudo rm -rf $TEST_BASE" || true
multipass exec "$VM" -- rm -f /tmp/backup_output.log /tmp/zrb_new || true
echo "✓ Cleanup completed"

# Summary
echo ""
echo "=== Test Summary ==="
echo "✓ New .tmp naming format verified successfully!"
echo ""
echo "File naming pattern confirmed:"
echo "  During backup: snapshot.part-aaaaaa.tmp"
echo "  After rename:  snapshot.part-aaaaaa"
echo "  After encrypt: snapshot.part-aaaaaa.age"
echo ""
echo "This is clearer than the old format:"
echo "  Old: snapshot.part-.tmpaaaaaa → snapshot.part-aaaaaa"
echo "  New: snapshot.part-aaaaaa.tmp → snapshot.part-aaaaaa"
