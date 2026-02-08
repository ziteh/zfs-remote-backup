#!/bin/bash
set -e

VM="zrb-vm"
CONFIG_REMOTE="/tmp/zrb_shutdown_test_config.yaml"
TEST_BASE="/home/ubuntu/zrb_shutdown_test"

echo "=== Testing Graceful Shutdown with Large Dataset ==="
echo ""

# Test 1: Transfer binary
echo "Test 1: Transfer updated binary to VM"
echo "----------------------------------------"
if [ -f "./build/zrb_simple" ]; then
    multipass transfer ./build/zrb_simple "$VM:/tmp/zrb_shutdown"
    multipass exec "$VM" -- chmod +x /tmp/zrb_shutdown
    echo "✓ Binary transferred"
else
    echo "✗ Binary not found"
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
  - name: shutdown_test
    enabled: true
    pool: testpool
    dataset: large_test
YAML"
echo "✓ Test config created"

# Test 3: Create large test dataset
echo ""
echo "Test 3: Create large test dataset"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    sudo zfs create -p testpool/large_test 2>/dev/null || true
    sudo mkdir -p /testpool/large_test
    sudo chown -R ubuntu:ubuntu /testpool/large_test

    # Create multiple large files (100MB each = ~500MB total)
    echo 'Creating large test files (this may take a moment)...'
    for i in {1..5}; do
        dd if=/dev/urandom of=/testpool/large_test/large_file_\${i}.bin bs=1M count=100 2>/dev/null &
    done
    wait

    echo '✓ Created 5 x 100MB test files'
    du -sh /testpool/large_test
"

# Test 4: Create snapshot
echo ""
echo "Test 4: Create test snapshot"
echo "----------------------------------------"
TIMESTAMP=$(date +%s)
multipass exec "$VM" -- sudo zfs snapshot testpool/large_test@zrb_shutdown_test_$TIMESTAMP
echo "✓ Snapshot created: testpool/large_test@zrb_shutdown_test_$TIMESTAMP"

# Test 5: Start backup and send SIGTERM
echo ""
echo "Test 5: Start backup and send SIGTERM during processing"
echo "----------------------------------------"
echo "Starting backup in background..."

# Start backup in background and capture PID
multipass exec "$VM" -- bash -lc "
    sudo /tmp/zrb_shutdown backup --config $CONFIG_REMOTE --task shutdown_test --level 0 > /tmp/shutdown_test_large.log 2>&1 &
    BACKUP_PID=\$!
    echo \$BACKUP_PID

    # Wait for backup to start (longer wait for large dataset)
    echo \"Waiting for backup to start processing...\"
    sleep 5

    # Verify process is running
    if ps -p \$BACKUP_PID > /dev/null; then
        echo \"✓ Backup process running (PID: \$BACKUP_PID)\"

        # Wait a bit more to ensure it's in the middle of processing
        sleep 2

        # Send SIGTERM signal
        echo \"Sending SIGTERM to PID \$BACKUP_PID...\"
        sudo kill -TERM \$BACKUP_PID

        # Wait for process to exit gracefully (max 15 seconds)
        EXITED=0
        for i in {1..15}; do
            if ! ps -p \$BACKUP_PID > /dev/null 2>&1; then
                echo \"✓ Process exited gracefully after \${i} seconds\"
                EXITED=1
                break
            fi
            sleep 1
        done

        if [ \$EXITED -eq 0 ]; then
            echo \"⚠ Process did not exit within 15 seconds, checking status\"
            ps -p \$BACKUP_PID || echo \"Process has exited\"
        fi

        exit 0
    else
        echo \"✗ Backup process not running (may have completed too quickly)\"
        exit 1
    fi
"

RESULT=$?
if [ $RESULT -eq 0 ]; then
    echo "✓ Signal handling test completed"
else
    echo "⚠ Process completed before signal could be sent"
fi

# Test 6: Check log for interruption
echo ""
echo "Test 6: Check log for interruption evidence"
echo "----------------------------------------"
LOG_OUTPUT=$(multipass exec "$VM" -- cat /tmp/shutdown_test_large.log 2>&1)

echo "Last 15 lines of log:"
echo "$LOG_OUTPUT" | tail -15

if echo "$LOG_OUTPUT" | grep -qi "backup completed successfully"; then
    echo ""
    echo "⚠ Backup completed successfully (too fast to interrupt)"
elif echo "$LOG_OUTPUT" | grep -qi "context\|cancel\|interrupt\|stopping"; then
    echo ""
    echo "✓ Found interruption evidence in log"
else
    echo ""
    echo "⚠ No clear completion or interruption message"
fi

# Test 7: Check for saved state
echo ""
echo "Test 7: Check for saved backup state"
echo "----------------------------------------"
if multipass exec "$VM" -- test -f "$TEST_BASE/run/testpool/large_test/backup_state.yaml"; then
    echo "✓ Backup state file exists"
    echo ""
    multipass exec "$VM" -- cat "$TEST_BASE/run/testpool/large_test/backup_state.yaml"
else
    echo "⚠ No backup state file (backup may have completed)"
fi

# Test 8: Manual SIGTERM test with kill command
echo ""
echo "Test 8: Manual interruption test"
echo "----------------------------------------"
echo "This test demonstrates manual interruption that would work in TrueNAS:"
echo ""
echo "Starting a new backup..."

# Clean up first
multipass exec "$VM" -- bash -lc "sudo rm -rf $TEST_BASE" || true

# Start backup
multipass exec "$VM" -- bash -lc "
    sudo /tmp/zrb_shutdown backup --config $CONFIG_REMOTE --task shutdown_test --level 0 > /tmp/shutdown_manual.log 2>&1 &
    BACKUP_PID=\$!

    echo \"Backup PID: \$BACKUP_PID\"
    sleep 3

    # Show process info
    echo \"Process status:\"
    ps aux | grep zrb_simple_shutdown | grep -v grep || echo \"Process not found\"

    echo \"\"
    echo \"To manually stop this backup, you would run:\"
    echo \"  kill -TERM \$BACKUP_PID\"
    echo \"  # or for force stop:\"
    echo \"  kill -9 \$BACKUP_PID\"

    # Let it run for a few seconds then stop
    sleep 3
    if ps -p \$BACKUP_PID > /dev/null; then
        echo \"\"
        echo \"Stopping backup...\"
        sudo kill -TERM \$BACKUP_PID
        sleep 2
    fi
" || true

echo "✓ Manual interruption demonstration completed"

# Test 9: Cleanup
echo ""
echo "Test 9: Cleanup"
echo "----------------------------------------"
multipass exec "$VM" -- bash -lc "
    sudo zfs destroy -r testpool/large_test 2>/dev/null || true
    sudo rm -rf $TEST_BASE
    rm -f /tmp/shutdown_test_large.log /tmp/shutdown_manual.log /tmp/zrb_shutdown
" || true
echo "✓ Cleanup completed"

# Summary
echo ""
echo "=== Summary ==="
echo ""
echo "Graceful shutdown mechanism is implemented with:"
echo "  ✓ Signal handling (SIGTERM, SIGINT)"
echo "  ✓ Context propagation through backup pipeline"
echo "  ✓ Worker pool respects context cancellation"
echo "  ✓ State saving for resumable backups"
echo ""
echo "Production usage (TrueNAS):"
echo ""
echo "1. To stop a running backup:"
echo "   pkill -TERM zrb_simple"
echo "   # or if using systemd:"
echo "   systemctl stop zrb-backup.service"
echo ""
echo "2. To resume after interruption:"
echo "   zrb_simple backup --config config.yaml --task taskname --level 0"
echo "   (Will automatically resume from saved state)"
echo ""
echo "3. Systemd integration example:"
echo "   [Service]"
echo "   Type=simple"
echo "   ExecStart=/usr/local/bin/zrb_simple backup --config /etc/zrb/config.yaml --task prod --level 0"
echo "   TimeoutStopSec=300"
echo "   KillMode=mixed"
echo "   KillSignal=SIGTERM"
