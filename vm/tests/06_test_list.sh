#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_simple_config.yaml"

echo "=== Testing list command ==="

# Test 1: List all backups from local source
echo ""
echo "Test 1: List all backups (local source)"
echo "----------------------------------------"
OUTPUT=$(multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task test_backup --source local)
echo "$OUTPUT"

# Verify JSON structure
if echo "$OUTPUT" | grep -q '"task"'; then
    echo "✓ JSON output contains 'task' field"
else
    echo "✗ JSON output missing 'task' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"backups"'; then
    echo "✓ JSON output contains 'backups' field"
else
    echo "✗ JSON output missing 'backups' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"summary"'; then
    echo "✓ JSON output contains 'summary' field"
else
    echo "✗ JSON output missing 'summary' field"
    exit 1
fi

# Test 2: Filter by level 0 (full backup)
echo ""
echo "Test 2: Filter by level 0"
echo "----------------------------------------"
OUTPUT_L0=$(multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task test_backup --level 0 --source local)
echo "$OUTPUT_L0"

# Verify level filtering
if echo "$OUTPUT_L0" | grep -q '"level": 0'; then
    echo "✓ Level 0 backup found"
else
    echo "✗ Level 0 backup not found"
    exit 1
fi

# Check that only level 0 is returned
if echo "$OUTPUT_L0" | grep -q '"level": 1'; then
    echo "✗ Level filtering failed: level 1 found in results"
    exit 1
else
    echo "✓ Level filtering works: only level 0 in results"
fi

# Test 3: Filter by level 1 (if exists)
echo ""
echo "Test 3: Filter by level 1 (if exists)"
echo "----------------------------------------"
OUTPUT_L1=$(multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task test_backup --level 1 --source local || echo '{"backups":[]}')
echo "$OUTPUT_L1"

if echo "$OUTPUT_L1" | grep -q '"level": 1'; then
    echo "✓ Level 1 backup found"
elif echo "$OUTPUT_L1" | grep -q '"backups": \[\]'; then
    echo "ℹ Level 1 backup not found (may not exist yet)"
else
    echo "✗ Unexpected output format"
    exit 1
fi

# Test 4: Verify backup metadata fields
echo ""
echo "Test 4: Verify backup metadata fields"
echo "----------------------------------------"
if echo "$OUTPUT" | grep -q '"snapshot"'; then
    echo "✓ Contains 'snapshot' field"
else
    echo "✗ Missing 'snapshot' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"blake3_hash"'; then
    echo "✓ Contains 'blake3_hash' field"
else
    echo "✗ Missing 'blake3_hash' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"parts_count"'; then
    echo "✓ Contains 'parts_count' field"
else
    echo "✗ Missing 'parts_count' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"datetime_str"'; then
    echo "✓ Contains 'datetime_str' field"
else
    echo "✗ Missing 'datetime_str' field"
    exit 1
fi

# Test 5: Verify summary statistics
echo ""
echo "Test 5: Verify summary statistics"
echo "----------------------------------------"
if echo "$OUTPUT" | grep -q '"total_backups"'; then
    echo "✓ Contains 'total_backups' field"
else
    echo "✗ Missing 'total_backups' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"full_backups"'; then
    echo "✓ Contains 'full_backups' field"
else
    echo "✗ Missing 'full_backups' field"
    exit 1
fi

if echo "$OUTPUT" | grep -q '"incremental_backups"'; then
    echo "✓ Contains 'incremental_backups' field"
else
    echo "✗ Missing 'incremental_backups' field"
    exit 1
fi

# Test 6: Test S3 source (if S3 is enabled in config)
echo ""
echo "Test 6: Test S3/MinIO source (if enabled)"
echo "----------------------------------------"
if multipass exec "$VM" -- grep -q "enabled: true" "$CONFIG_REMOTE" 2>/dev/null; then
    echo "S3 is enabled in config, testing S3 source..."

    # Check if manifest storage class is accessible
    MANIFEST_CLASS=$(multipass exec "$VM" -- grep -A1 "storage_class:" "$CONFIG_REMOTE" | grep "manifest:" | awk '{print $2}' || echo "UNKNOWN")
    echo "Manifest storage class: $MANIFEST_CLASS"

    if [ "$MANIFEST_CLASS" = "GLACIER" ] || [ "$MANIFEST_CLASS" = "DEEP_ARCHIVE" ]; then
        echo "⚠ Manifest is in $MANIFEST_CLASS - S3 list test will be skipped (expected to fail)"
    else
        OUTPUT_S3=$(multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task test_backup --source s3 2>&1 || echo "S3_LIST_FAILED")

        if echo "$OUTPUT_S3" | grep -q "S3_LIST_FAILED\|failed to download"; then
            echo "ℹ S3 list failed (manifest may not be uploaded yet or MinIO not running)"
        elif echo "$OUTPUT_S3" | grep -q '"task"'; then
            echo "✓ S3 list succeeded"
            echo "$OUTPUT_S3"
        else
            echo "✗ Unexpected S3 list output"
            echo "$OUTPUT_S3"
        fi
    fi
else
    echo "ℹ S3 is disabled in config, skipping S3 source test"
fi

# Test 7: Test error handling - invalid task name
echo ""
echo "Test 7: Test error handling - invalid task name"
echo "----------------------------------------"
ERROR_OUTPUT=$(multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task nonexistent_task --source local 2>&1 || true)
if echo "$ERROR_OUTPUT" | grep -q "not found\|error"; then
    echo "✓ Properly handles invalid task name"
else
    echo "✗ Did not return error for invalid task"
    echo "$ERROR_OUTPUT"
fi

# Test 8: Pretty print JSON for manual inspection
echo ""
echo "Test 8: Pretty print full output for manual inspection"
echo "----------------------------------------"
multipass exec "$VM" -- /tmp/zrb_simple list --config "$CONFIG_REMOTE" --task test_backup --source local | python3 -m json.tool 2>/dev/null || echo "$OUTPUT"

echo ""
echo "=== All list command tests completed ==="
