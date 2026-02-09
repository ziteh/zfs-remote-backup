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

func runS3RestoreTests(t *testing.T, v *vm) {
	configPath := "/tmp/zrb_s3_config.yaml"
	baseDir := "/home/ubuntu/zrb_s3_base"
	taskName := "s3_test_backup"
	restoreTarget := "testpool/restored_from_s3"

	// Check MinIO availability
	_, err := v.exec("curl -s http://127.0.0.1:9000/minio/health/live")
	if err != nil {
		t.Skip("MinIO not available, skipping S3 restore tests")
	}

	// Ensure bucket exists
	v.exec("mc alias set myminio http://127.0.0.1:9000 admin password123 >/dev/null 2>&1 || true")
	v.exec("mc mb myminio/zrb-test >/dev/null 2>&1 || true")

	// Write S3 config
	require.NoError(t, v.writeFile(configPath, s3Config(baseDir, taskName)))

	// Cleanup on finish
	t.Cleanup(func() {
		v.exec(fmt.Sprintf("sudo zfs destroy -r %s 2>/dev/null || true", restoreTarget))
		v.exec("sudo rm -rf " + baseDir)
	})

	// Ensure baseline data exists
	require.True(t, v.fileExists("/tmp/baseline_sha256.txt"),
		"baseline checksums must exist (run Backup tests first)")

	var restoreOutput string

	t.Run("BackupToS3", func(t *testing.T) {
		ts := time.Now().Unix()
		v.mustExec(t, fmt.Sprintf("sudo zfs snapshot testpool/backup_data@zrb_s3_level0_%d", ts))

		out, err := v.zrbWithS3(fmt.Sprintf("backup --config %s --task %s --level 0", configPath, taskName))
		require.NoError(t, err, "S3 backup failed: %s", out)
		assert.Contains(t, out, "Backup completed")
	})

	t.Run("VerifyManifests", func(t *testing.T) {
		out, err := v.exec("mc ls myminio/zrb-test/test-backups/manifests/ --recursive 2>&1 || true")
		require.NoError(t, err)
		assert.Contains(t, out, "task_manifest.yaml", "manifests should be uploaded to S3")
	})

	t.Run("DryRunRestore", func(t *testing.T) {
		out, err := v.zrbWithS3(fmt.Sprintf("restore --config %s --task %s --level 0 --target %s --private-key %s --source s3 --dry-run",
			configPath, taskName, restoreTarget, privateKeyPath))
		require.NoError(t, err, "dry-run failed: %s", out)
		assert.Contains(t, out, "DRY RUN MODE")
		assert.Contains(t, out, "No changes made")
		assert.True(t, strings.Contains(out, "s3") || strings.Contains(out, "S3"),
			"should show S3 as source: %s", out)
		assert.Contains(t, out, taskName)
	})

	t.Run("ActualRestore", func(t *testing.T) {
		// Clean target
		v.exec(fmt.Sprintf("sudo zfs destroy -r %s 2>/dev/null || true", restoreTarget))

		out, err := v.zrbWithS3(fmt.Sprintf("restore --config %s --task %s --level 0 --target %s --private-key %s --source s3",
			configPath, taskName, restoreTarget, privateKeyPath))
		require.NoError(t, err, "S3 restore failed: %s", out)
		assert.Contains(t, out, "Restore completed successfully")
		restoreOutput = out

		// Verify dataset exists
		_, err = v.exec(fmt.Sprintf("sudo zfs list %s", restoreTarget))
		assert.NoError(t, err, "restored dataset should exist")
	})

	t.Run("DataIntegrity", func(t *testing.T) {
		v.mustExec(t, fmt.Sprintf(`cd /%s && \
sudo find . -type f -print0 | sudo xargs -0 sha256sum | sort > /tmp/s3_restore_sha256.txt && \
sudo chown ubuntu:ubuntu /tmp/s3_restore_sha256.txt`, restoreTarget))

		diff, _ := v.exec("diff -u /tmp/baseline_sha256.txt /tmp/s3_restore_sha256.txt")
		assert.Empty(t, diff, "S3 restored data should match baseline")
	})

	t.Run("S3DownloadLogs", func(t *testing.T) {
		assert.Contains(t, restoreOutput, "Downloading",
			"restore output should show S3 download operations")
	})
}
