//go:build e2e_vm

package e2e

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testPool    = "testpool"
	testDataset = "testdata"
	testTask    = "e2e-backup"
	testBaseDir = "/tmp/zrb_e2e"
	keyPath     = "/tmp/zrb_e2e_private_key.txt"
)

func TestBackupToMinIO(t *testing.T) {
	v := newVM()

	// Verify VM is reachable
	out, err := v.exec("echo ok")
	require.NoError(t, err, "VM not reachable: %s", out)
	require.Equal(t, "ok", out)

	var agePublicKey string

	t.Run("Setup", func(t *testing.T) {
		buildAndTransfer(t, v)

		// Ensure MinIO is healthy
		out := v.mustExec(t, "curl -sf http://127.0.0.1:9000/minio/health/live && echo ok")
		require.Contains(t, out, "ok", "MinIO not healthy")

		// Create test bucket
		v.mustExec(t, "mc mb --ignore-existing myminio/"+minioBucket)
	})

	t.Run("GenerateKeys", func(t *testing.T) {
		out := v.mustExec(t, remoteBin+" genkey")

		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Public key:") {
				agePublicKey = strings.TrimSpace(strings.TrimPrefix(line, "Public key:"))
			}
			if strings.HasPrefix(line, "AGE-SECRET-KEY-") {
				v.writeFile(t, keyPath, line)
			}
		}

		require.NotEmpty(t, agePublicKey, "failed to extract public key from genkey output")
		require.True(t, strings.HasPrefix(agePublicKey, "age1"), "invalid public key format")
	})

	t.Run("PrepareData", func(t *testing.T) {
		// Create dataset
		v.mustExecSudo(t, "zfs create "+testPool+"/"+testDataset)

		// Generate test files
		mountpoint := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+testPool+"/"+testDataset)
		v.mustExecSudo(t, "dd if=/dev/urandom of="+mountpoint+"/random.bin bs=1M count=2")
		v.mustExecSudo(t, "mkdir -p "+mountpoint+"/subdir")
		v.mustExecSudo(t, "bash -c \"echo 'hello zrb e2e test' > "+mountpoint+"/subdir/hello.txt\"")
		v.mustExecSudo(t, "bash -c \"seq 1 10000 > "+mountpoint+"/numbers.txt\"")

		// Create snapshot via zrb
		v.mustExecSudo(t, remoteBin+" snapshot --pool "+testPool+" --dataset "+testDataset+" --prefix zrb_level0")

		// Verify snapshot exists
		out := v.mustExecSudo(t, "zfs list -t snapshot -H -o name "+testPool+"/"+testDataset)
		assert.Contains(t, out, "zrb_level0", "snapshot not created")
	})

	t.Run("Backup", func(t *testing.T) {
		cfg := s3Config(testBaseDir, testTask, testPool, testDataset, agePublicKey)
		v.writeFile(t, configPath, cfg)

		out := v.mustExecWithS3(t, "sudo -E "+remoteBin+" backup --config "+configPath+" --task "+testTask+" --level 0")
		assert.Contains(t, out, "Backup completed", "backup did not complete successfully\noutput: %s", out)
	})

	t.Run("VerifyS3", func(t *testing.T) {
		// Check data files exist in MinIO
		out := v.mustExec(t, "mc ls --recursive myminio/"+minioBucket+"/backups/data/"+testPool+"/"+testDataset+"/")
		assert.NotEmpty(t, out, "no data files found in MinIO")
		assert.Contains(t, out, ".age", "no encrypted files found in MinIO")

		// Check manifest exists in MinIO
		out = v.mustExec(t, "mc ls --recursive myminio/"+minioBucket+"/backups/manifests/"+testPool+"/"+testDataset+"/")
		assert.NotEmpty(t, out, "no manifest files found in MinIO")
		assert.Contains(t, out, "task_manifest.yaml", "manifest not found in MinIO")
	})

	t.Run("Cleanup", func(t *testing.T) {
		v.execSudo("zfs destroy -rf " + testPool + "/" + testDataset)
		v.execSudo("rm -rf " + testBaseDir)
		v.exec("rm -f " + configPath + " " + keyPath)
		v.exec("mc rb --force myminio/" + minioBucket)
	})
}
