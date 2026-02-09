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

func runBackupTests(t *testing.T, v *vm) {
	configPath := "/tmp/zrb_config.yaml"
	baseDir := "/home/ubuntu/zrb_base"
	taskName := "test_backup"

	// Write local config to VM
	require.NoError(t, v.writeFile(configPath, localConfig(baseDir, taskName)))

	// Cleanup on finish
	t.Cleanup(func() {
		v.exec("sudo rm -rf " + baseDir)
		v.exec("sudo zfs destroy -r testpool/restore_data 2>/dev/null || true")
		v.exec("sudo zfs destroy -r testpool/restored_test 2>/dev/null || true")
	})

	t.Run("PrepareEnv", func(t *testing.T) {
		testPrepareEnv(t, v)
	})

	t.Run("L0Backup", func(t *testing.T) {
		testL0Backup(t, v, configPath, taskName)
	})

	t.Run("L1ToL4Backup", func(t *testing.T) {
		testL1ToL4Backup(t, v, configPath, taskName)
	})

	t.Run("ManualRestore", func(t *testing.T) {
		testManualRestore(t, v, baseDir)
	})

	t.Run("InterruptRecover", func(t *testing.T) {
		testInterruptRecover(t, v, configPath, taskName)
	})

	t.Run("ListCommand", func(t *testing.T) {
		testListCommand(t, v, configPath, taskName)
	})

	t.Run("RestoreCommand", func(t *testing.T) {
		testRestoreCommand(t, v, configPath, taskName)
	})
}

// testPrepareEnv creates test data and baseline SHA256 checksums (01_prepare_env.sh).
func testPrepareEnv(t *testing.T, v *vm) {
	v.mustExec(t, "sudo mkdir -p /testpool/backup_data && sudo chown -R ubuntu:ubuntu /testpool/backup_data")

	v.mustExec(t, `cd /testpool/backup_data && \
sudo bash -c 'dd if=/dev/urandom of=test-binary.bin bs=1M count=10 >/dev/null 2>&1 || true' && \
sudo bash -c 'mkdir -p test-dir && echo hello > test-dir/hello.txt' && \
echo 'file1' > test-file-1.txt && \
echo 'file2' > test-file-2.txt`)

	v.mustExec(t, "cd /testpool/backup_data && sudo find . -type f -print0 | sudo xargs -0 sha256sum > /tmp/baseline_sha256.txt && sudo chown ubuntu:ubuntu /tmp/baseline_sha256.txt")

	assert.True(t, v.fileExists("/tmp/baseline_sha256.txt"), "baseline checksums should exist")
}

// testL0Backup tests level 0 full backup (02_l0_backup.sh).
func testL0Backup(t *testing.T, v *vm, configPath, taskName string) {
	ts := time.Now().Unix()
	v.mustExec(t, fmt.Sprintf("sudo zfs snapshot testpool/backup_data@zrb_level0_%d || true", ts))

	out, err := v.zrb(fmt.Sprintf("backup --config %s --task %s --level 0", configPath, taskName))
	require.NoError(t, err, "L0 backup failed: %s", out)
	assert.Contains(t, out, "Backup completed", out)
}

// testL1ToL4Backup tests incremental backups (03_l1_to_l4.sh).
func testL1ToL4Backup(t *testing.T, v *vm, configPath, taskName string) {
	for level := 1; level <= 4; level++ {
		t.Run(fmt.Sprintf("Level%d", level), func(t *testing.T) {
			// Modify data
			v.mustExec(t, fmt.Sprintf("sudo bash -c 'echo modified-%d >> /testpool/backup_data/test-file-1.txt'", level))

			// Create snapshot
			ts := time.Now().Unix()
			v.mustExec(t, fmt.Sprintf("sudo zfs snapshot testpool/backup_data@zrb_level%d_%d || true", level, ts))

			// Run backup
			out, err := v.zrb(fmt.Sprintf("backup --config %s --task %s --level %d", configPath, taskName, level))
			require.NoError(t, err, "L%d backup failed: %s", level, out)
		})
	}
}

// testManualRestore tests manual restore via cat/decrypt/zfs receive (04_restore.sh).
func testManualRestore(t *testing.T, v *vm, baseDir string) {
	// Recreate restore dataset
	v.exec("sudo zfs destroy -r testpool/restore_data 2>/dev/null || true")
	v.mustExec(t, "sudo zfs create testpool/restore_data")

	// Find level directories and restore in order
	levels := v.mustExec(t, fmt.Sprintf("ls -1 %s/task/testpool/backup_data | sort -V | grep '^level' || true", baseDir))
	require.NotEmpty(t, levels, "should have level directories to restore")

	for _, d := range strings.Split(levels, "\n") {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		dateDirs := v.mustExec(t, fmt.Sprintf("ls -1 %s/task/testpool/backup_data/%s | sort -V || true", baseDir, d))
		dateDir := strings.TrimSpace(strings.Split(dateDirs, "\n")[0])
		if dateDir == "" {
			continue
		}
		remoteDir := fmt.Sprintf("%s/task/testpool/backup_data/%s/%s", baseDir, d, dateDir)
		v.mustExec(t, fmt.Sprintf("cat %s/snapshot.part-* | age --decrypt -i %s | sudo zfs receive -F testpool/restore_data", remoteDir, privateKeyPath))
	}

	// Compute restored checksums
	v.mustExec(t, "cd /testpool/restore_data && sudo find . -type f -print0 | sudo xargs -0 sha256sum > /tmp/restore_sha256.txt && sudo chown ubuntu:ubuntu /tmp/restore_sha256.txt")

	// Compare
	diff, _ := v.exec("diff -u /tmp/baseline_sha256.txt /tmp/restore_sha256.txt")
	assert.Empty(t, diff, "restored data should match baseline SHA256 checksums")
}

// testInterruptRecover tests kill -9 mid-backup then resume (05_interrupt_recover.sh).
func testInterruptRecover(t *testing.T, v *vm, configPath, taskName string) {
	// Start backup in background, capture PID, then kill
	v.mustExec(t, fmt.Sprintf(`sudo %s backup --config %s --task %s --level 2 &
echo $! > /tmp/zrb_backup_pid
sleep 2`, remoteBin, configPath, taskName))

	pid := v.mustExec(t, "cat /tmp/zrb_backup_pid")
	require.NotEmpty(t, pid, "should have captured backup PID")

	time.Sleep(3 * time.Second)
	v.exec(fmt.Sprintf("sudo kill -9 %s || true", pid))
	time.Sleep(1 * time.Second)

	// Resume
	out, err := v.zrb(fmt.Sprintf("backup --config %s --task %s --level 2", configPath, taskName))
	require.NoError(t, err, "resumed backup should complete: %s", out)
}

// testListCommand tests the list command (06_test_list.sh).
func testListCommand(t *testing.T, v *vm, configPath, taskName string) {
	// Test 1: List all backups - verify JSON structure
	t.Run("ListAll", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task %s --source local", remoteBin, configPath, taskName))
		require.NoError(t, err, "list command failed: %s", out)
		assert.Contains(t, out, `"task"`)
		assert.Contains(t, out, `"backups"`)
		assert.Contains(t, out, `"summary"`)
	})

	// Test 2: Filter by level 0
	t.Run("FilterLevel0", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task %s --level 0 --source local", remoteBin, configPath, taskName))
		require.NoError(t, err, "list --level 0 failed: %s", out)
		assert.Contains(t, out, `"level": 0`)
		assert.NotContains(t, out, `"level": 1`)
	})

	// Test 3: Filter by level 1
	t.Run("FilterLevel1", func(t *testing.T) {
		out, _ := v.exec(fmt.Sprintf("%s list --config %s --task %s --level 1 --source local", remoteBin, configPath, taskName))
		if strings.Contains(out, `"level": 1`) {
			assert.NotContains(t, out, `"level": 0`)
		}
		// Level 1 may or may not exist depending on test state
	})

	// Test 4: Verify metadata fields
	t.Run("MetadataFields", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task %s --source local", remoteBin, configPath, taskName))
		require.NoError(t, err)
		assert.Contains(t, out, `"snapshot"`)
		assert.Contains(t, out, `"blake3_hash"`)
		assert.Contains(t, out, `"parts_count"`)
		assert.Contains(t, out, `"datetime_str"`)
	})

	// Test 5: Verify summary statistics
	t.Run("SummaryStats", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task %s --source local", remoteBin, configPath, taskName))
		require.NoError(t, err)
		assert.Contains(t, out, `"total_backups"`)
		assert.Contains(t, out, `"full_backups"`)
		assert.Contains(t, out, `"incremental_backups"`)
	})

	// Test 6: Invalid task name
	t.Run("InvalidTask", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task nonexistent_task --source local", remoteBin, configPath))
		assert.Error(t, err)
		assert.Contains(t, out, "not found")
	})

	// Test 7: Valid JSON output
	t.Run("ValidJSON", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s list --config %s --task %s --source local | python3 -m json.tool > /dev/null", remoteBin, configPath, taskName))
		assert.NoError(t, err, "output should be valid JSON: %s", out)
	})
}

// testRestoreCommand tests the restore command (07_test_restore.sh).
func testRestoreCommand(t *testing.T, v *vm, configPath, taskName string) {
	restoreTarget := "testpool/restored_test"

	// Test 1: Dry-run restore level 0
	t.Run("DryRunL0", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf("%s restore --config %s --task %s --level 0 --target %s --private-key %s --source local --dry-run",
			remoteBin, configPath, taskName, restoreTarget, privateKeyPath))
		require.NoError(t, err, "dry-run failed: %s", out)
		assert.Contains(t, out, "DRY RUN MODE")
		assert.Contains(t, out, "No changes made")
		assert.Contains(t, out, taskName)
		assert.Contains(t, out, "testpool/backup_data")
	})

	// Test 2: Prepare target dataset
	t.Run("PrepareTarget", func(t *testing.T) {
		v.exec(fmt.Sprintf("sudo zfs destroy -r %s 2>/dev/null || true", restoreTarget))
		out, err := v.exec(fmt.Sprintf("sudo zfs list %s 2>/dev/null", restoreTarget))
		assert.Error(t, err, "target dataset should not exist: %s", out)
	})

	// Test 3: Actual restore level 0
	var restoreOutput string
	t.Run("RestoreL0", func(t *testing.T) {
		out, err := v.zrb(fmt.Sprintf("restore --config %s --task %s --level 0 --target %s --private-key %s --source local",
			configPath, taskName, restoreTarget, privateKeyPath))
		require.NoError(t, err, "restore failed: %s", out)
		assert.Contains(t, out, "Restore completed successfully")
		restoreOutput = out
	})

	// Test 4: Verify restored dataset exists
	t.Run("DatasetExists", func(t *testing.T) {
		_, err := v.exec(fmt.Sprintf("sudo zfs list %s", restoreTarget))
		assert.NoError(t, err, "restored dataset should exist")
	})

	// Test 5: Data integrity
	t.Run("DataIntegrity", func(t *testing.T) {
		v.mustExec(t, fmt.Sprintf(`cd /%s && \
sudo find . -type f -print0 | sudo xargs -0 sha256sum | sort > /tmp/restore_sha256_new.txt && \
sudo chown ubuntu:ubuntu /tmp/restore_sha256_new.txt`, restoreTarget))

		diff, _ := v.exec("diff -u /tmp/baseline_sha256.txt /tmp/restore_sha256_new.txt")
		assert.Empty(t, diff, "restored data should match baseline")
	})

	// Test 6: BLAKE3 verification
	t.Run("BLAKE3Verified", func(t *testing.T) {
		assert.Contains(t, restoreOutput, "BLAKE3 verified", "restore output should show BLAKE3 verification")
	})

	// Test 7: SHA256 verification
	t.Run("SHA256Verified", func(t *testing.T) {
		assert.Contains(t, restoreOutput, "SHA256 verified", "restore output should show SHA256 verification")
	})

	// Test 8: Error - invalid level
	t.Run("ErrorInvalidLevel", func(t *testing.T) {
		out, err := v.zrb(fmt.Sprintf("restore --config %s --task %s --level 99 --target testpool/invalid_restore --private-key %s --source local",
			configPath, taskName, privateKeyPath))
		assert.Error(t, err)
		assert.True(t, strings.Contains(out, "not found") || strings.Contains(out, "invalid"),
			"should report invalid level: %s", out)
	})

	// Test 9: Error - invalid task
	t.Run("ErrorInvalidTask", func(t *testing.T) {
		out, err := v.zrb(fmt.Sprintf("restore --config %s --task nonexistent_task --level 0 --target testpool/invalid_restore --private-key %s --source local",
			configPath, privateKeyPath))
		assert.Error(t, err)
		assert.True(t, strings.Contains(out, "not found") || strings.Contains(out, "error"),
			"should report invalid task: %s", out)
	})

	// Test 10: Error - missing private key
	t.Run("ErrorMissingKey", func(t *testing.T) {
		out, err := v.zrb(fmt.Sprintf("restore --config %s --task %s --level 0 --target testpool/invalid_restore --private-key /nonexistent/key.txt --source local",
			configPath, taskName))
		assert.Error(t, err)
		assert.Contains(t, out, "failed to read private key")
	})

	// Test 11: Cleanup
	t.Run("Cleanup", func(t *testing.T) {
		_, err := v.exec(fmt.Sprintf("sudo zfs destroy -r %s", restoreTarget))
		assert.NoError(t, err, "cleanup should succeed")
	})
}
