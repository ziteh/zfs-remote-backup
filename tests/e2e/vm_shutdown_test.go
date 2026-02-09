//go:build e2e_vm

package e2e

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runGracefulShutdownTests(t *testing.T, v *vm) {
	t.Run("Small", func(t *testing.T) {
		testGracefulShutdownSmall(t, v)
	})
	t.Run("Large", func(t *testing.T) {
		testGracefulShutdownLarge(t, v)
	})
}

// testGracefulShutdownSmall tests SIGTERM/SIGINT handling with the small backup_data dataset.
func testGracefulShutdownSmall(t *testing.T, v *vm) {
	configPath := "/tmp/zrb_shutdown_test_config.yaml"
	baseDir := "/home/ubuntu/zrb_shutdown_test"
	dataset := "backup_data"

	require.NoError(t, v.writeFile(configPath, shutdownConfig(baseDir, dataset)))
	v.exec("sudo rm -rf " + baseDir)

	ts := time.Now().Unix()
	snapName := fmt.Sprintf("testpool/%s@zrb_shutdown_test_%d", dataset, ts)
	v.mustExec(t, "sudo zfs snapshot "+snapName)

	t.Cleanup(func() {
		v.exec("sudo zfs destroy " + snapName + " 2>/dev/null || true")
		v.exec("sudo rm -rf " + baseDir)
		v.exec("rm -f /tmp/shutdown_test.log /tmp/shutdown_test2.log /tmp/backup_pid.txt")
	})

	// Test SIGTERM
	t.Run("SigtermGracefulExit", func(t *testing.T) {
		out, err := v.execLong(fmt.Sprintf(`
sudo %s backup --config %s --task shutdown_test --level 0 > /tmp/shutdown_test.log 2>&1 &
BACKUP_PID=$!
sleep 3
if ps -p $BACKUP_PID > /dev/null; then
    sudo kill -TERM $BACKUP_PID
    for i in $(seq 1 10); do
        if ! ps -p $BACKUP_PID > /dev/null 2>&1; then
            echo "exited_gracefully"
            break
        fi
        sleep 1
    done
    if ps -p $BACKUP_PID > /dev/null 2>&1; then
        echo "force_kill_needed"
        sudo kill -9 $BACKUP_PID
    fi
else
    echo "completed_before_signal"
fi`, remoteBin, configPath))
		require.NoError(t, err, "SIGTERM test failed: %s", out)
		assert.True(t,
			strings.Contains(out, "exited_gracefully") || strings.Contains(out, "completed_before_signal"),
			"process should exit gracefully or complete before signal: %s", out)
	})

	t.Run("LockReleased", func(t *testing.T) {
		assert.False(t, v.fileExists(fmt.Sprintf("%s/run/testpool/%s/zrb.lock", baseDir, dataset)),
			"lock file should be released after graceful shutdown")
	})

	t.Run("ZfsHoldsReleased", func(t *testing.T) {
		out, _ := v.exec(fmt.Sprintf("sudo zfs holds %s 2>&1", snapName))
		assert.False(t, strings.Contains(out, "zrb:"),
			"ZFS holds should be released: %s", out)
	})

	t.Run("StateSaved", func(t *testing.T) {
		// State file may or may not exist depending on whether backup completed
		// before the signal. Either outcome is acceptable.
		logOut, _ := v.exec("cat /tmp/shutdown_test.log")
		if strings.Contains(logOut, "Backup completed successfully") {
			t.Log("backup completed before signal, state file may not exist")
			return
		}
		assert.True(t, v.fileExists(fmt.Sprintf("%s/run/testpool/%s/backup_state.yaml", baseDir, dataset)),
			"backup state should be saved for resumption")
	})

	t.Run("LogShowsInterruption", func(t *testing.T) {
		logOut, _ := v.exec("cat /tmp/shutdown_test.log")
		if strings.Contains(logOut, "Backup completed successfully") {
			t.Log("backup completed before signal")
			return
		}
		assert.True(t,
			strings.Contains(strings.ToLower(logOut), "context") ||
				strings.Contains(strings.ToLower(logOut), "cancel") ||
				strings.Contains(strings.ToLower(logOut), "interrupt"),
			"log should show interruption evidence: %s", logOut)
	})

	// Test SIGINT
	t.Run("SigintGracefulExit", func(t *testing.T) {
		out, _ := v.execLong(fmt.Sprintf(`
sudo %s backup --config %s --task shutdown_test --level 0 > /tmp/shutdown_test2.log 2>&1 &
BACKUP_PID=$!
sleep 3
if ps -p $BACKUP_PID > /dev/null; then
    sudo kill -INT $BACKUP_PID
    sleep 2
    if ! ps -p $BACKUP_PID > /dev/null 2>&1; then
        echo "exited_gracefully"
    else
        echo "still_running"
        sudo kill -9 $BACKUP_PID
    fi
else
    echo "completed_before_signal"
fi`, remoteBin, configPath))
		assert.True(t,
			strings.Contains(out, "exited_gracefully") || strings.Contains(out, "completed_before_signal"),
			"process should handle SIGINT gracefully: %s", out)
	})

	t.Run("ResumeAfterInterrupt", func(t *testing.T) {
		stateFile := fmt.Sprintf("%s/run/testpool/%s/backup_state.yaml", baseDir, dataset)
		if !v.fileExists(stateFile) {
			t.Log("no state file to resume from (backup may have completed)")
			return
		}

		out, err := v.zrb(fmt.Sprintf("backup --config %s --task shutdown_test --level 0", configPath))
		require.NoError(t, err, "resumed backup should complete: %s", out)
		if strings.Contains(out, "Found existing backup state") {
			t.Log("backup successfully resumed from saved state")
		}
	})
}

// testGracefulShutdownLarge tests SIGTERM with a 500MB dataset.
func testGracefulShutdownLarge(t *testing.T, v *vm) {
	configPath := "/tmp/zrb_shutdown_large_config.yaml"
	baseDir := "/home/ubuntu/zrb_shutdown_large_test"
	dataset := "large_test"

	require.NoError(t, v.writeFile(configPath, shutdownConfig(baseDir, dataset)))

	// Create large dataset
	v.mustExec(t, "sudo zfs create -p testpool/large_test 2>/dev/null || true")
	v.mustExec(t, "sudo mkdir -p /testpool/large_test && sudo chown -R ubuntu:ubuntu /testpool/large_test")

	// Create 5x100MB files
	v.mustExec(t, `cd /testpool/large_test && \
for i in 1 2 3 4 5; do dd if=/dev/urandom of=large_file_${i}.bin bs=1M count=100 2>/dev/null & done && wait`)

	ts := time.Now().Unix()
	snapName := fmt.Sprintf("testpool/large_test@zrb_shutdown_test_%d", ts)
	v.mustExec(t, "sudo zfs snapshot "+snapName)

	t.Cleanup(func() {
		v.exec("sudo zfs destroy -r testpool/large_test 2>/dev/null || true")
		v.exec("sudo rm -rf " + baseDir)
		v.exec("rm -f /tmp/shutdown_test_large.log")
	})

	t.Run("SigtermDuringProcessing", func(t *testing.T) {
		out, _ := v.execLong(fmt.Sprintf(`
sudo %s backup --config %s --task shutdown_test --level 0 > /tmp/shutdown_test_large.log 2>&1 &
BACKUP_PID=$!
sleep 7
if ps -p $BACKUP_PID > /dev/null; then
    sudo kill -TERM $BACKUP_PID
    EXITED=0
    for i in $(seq 1 15); do
        if ! ps -p $BACKUP_PID > /dev/null 2>&1; then
            echo "exited_after_${i}s"
            EXITED=1
            break
        fi
        sleep 1
    done
    if [ $EXITED -eq 0 ]; then
        echo "did_not_exit"
        sudo kill -9 $BACKUP_PID
    fi
else
    echo "completed_before_signal"
fi`, remoteBin, configPath))
		assert.False(t, strings.Contains(out, "did_not_exit"),
			"process should exit within 15s after SIGTERM: %s", out)
	})

	t.Run("InterruptionEvidence", func(t *testing.T) {
		logOut, _ := v.exec("cat /tmp/shutdown_test_large.log")
		if strings.Contains(logOut, "Backup completed successfully") {
			t.Log("backup completed before signal (dataset may be too small)")
			return
		}
		assert.True(t,
			strings.Contains(strings.ToLower(logOut), "context") ||
				strings.Contains(strings.ToLower(logOut), "cancel") ||
				strings.Contains(strings.ToLower(logOut), "interrupt") ||
				strings.Contains(strings.ToLower(logOut), "stopping"),
			"log should show interruption: %s", logOut)
	})

	t.Run("StateSaved", func(t *testing.T) {
		logOut, _ := v.exec("cat /tmp/shutdown_test_large.log")
		if strings.Contains(logOut, "Backup completed successfully") {
			t.Log("backup completed, state file may not exist")
			return
		}
		assert.True(t, v.fileExists(fmt.Sprintf("%s/run/testpool/large_test/backup_state.yaml", baseDir)),
			"backup state should be saved")
	})
}
