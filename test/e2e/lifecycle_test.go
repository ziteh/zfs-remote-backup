//go:build e2e_vm

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	lcPool    = "testpool"
	lcDataset = "lifecycle"
	lcTask    = "e2e-lifecycle"
	lcBaseDir = "/tmp/zrb_e2e_lifecycle"
	lcKeyPath = "/tmp/zrb_e2e_lifecycle_key.txt"
)

func TestBackupRestoreLifecycle(t *testing.T) {
	v := newVM()

	out, err := v.exec("echo ok")
	require.NoError(t, err, "VM not reachable: %s", out)
	require.Equal(t, "ok", out)

	var agePublicKey string

	t.Run("Setup", func(t *testing.T) {
		buildAndTransfer(t, v)

		out := v.mustExec(t, "curl -sf http://127.0.0.1:9000/minio/health/live && echo ok")
		require.Contains(t, out, "ok", "MinIO not healthy")

		v.mustExec(t, "mc mb --ignore-existing myminio/"+minioBucket)
	})

	t.Run("GenerateKeys", func(t *testing.T) {
		out := v.mustExec(t, remoteBin+" genkey")

		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "Public key:") {
				agePublicKey = strings.TrimSpace(strings.TrimPrefix(line, "Public key:"))
			}
			if strings.HasPrefix(line, "Private key:") {
				privateKey := strings.TrimSpace(strings.TrimPrefix(line, "Private key:"))
				v.writeFile(t, lcKeyPath, privateKey)
			}
		}

		require.NotEmpty(t, agePublicKey, "failed to extract public key")
		require.True(t, strings.HasPrefix(agePublicKey, "age1"), "invalid public key format")
	})

	t.Run("TestKeys", func(t *testing.T) {
		cfg := s3Config(lcBaseDir, lcTask, lcPool, lcDataset, agePublicKey)
		v.writeFile(t, configPath, cfg)

		out := v.mustExec(t, remoteBin+" test-keys --config "+configPath+" --private-key "+lcKeyPath)
		assert.Contains(t, out, "Content verification successful")
	})

	t.Run("PrepareData", func(t *testing.T) {
		v.mustExecSudo(t, "zfs create "+lcPool+"/"+lcDataset)

		mountpoint := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+lcPool+"/"+lcDataset)
		v.mustExecSudo(t, "dd if=/dev/urandom of="+mountpoint+"/random.bin bs=1M count=2")
		v.mustExecSudo(t, "mkdir -p "+mountpoint+"/subdir")
		v.mustExecSudo(t, "bash -c \"echo 'hello lifecycle test' > "+mountpoint+"/subdir/hello.txt\"")
		v.mustExecSudo(t, "bash -c \"seq 1 10000 > "+mountpoint+"/numbers.txt\"")

		v.mustExecSudo(t, remoteBin+" snapshot --pool "+lcPool+" --dataset "+lcDataset+" --prefix zrb_level0")

		out := v.mustExecSudo(t, "zfs list -t snapshot -H -o name "+lcPool+"/"+lcDataset)
		assert.Contains(t, out, "zrb_level0")
	})

	t.Run("Backup", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "backup --config "+configPath+" --task "+lcTask+" --level 0")
		assert.Contains(t, out, "Backup completed")
	})

	t.Run("ListS3", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "list --config "+configPath+" --task "+lcTask+" --source s3")

		var result listOutput
		require.NoError(t, json.Unmarshal([]byte(extractJSON(out)), &result), "failed to parse list JSON: %s", out)

		assert.Equal(t, lcTask, result.Task)
		assert.Equal(t, lcPool, result.Pool)
		assert.Equal(t, lcDataset, result.Dataset)
		assert.Equal(t, "s3", result.Source)
		assert.Equal(t, 1, result.Summary.TotalBackups)
		assert.Equal(t, 1, result.Summary.FullBackups)
		assert.Equal(t, 0, result.Summary.IncrementalBackups)

		require.Len(t, result.Backups, 1)
		assert.Equal(t, int16(0), result.Backups[0].Level)
		assert.Equal(t, "full", result.Backups[0].Type)
		assert.NotEmpty(t, result.Backups[0].Snapshot)
		assert.NotEmpty(t, result.Backups[0].Blake3Hash)
	})

	t.Run("ListS3FilterLevel", func(t *testing.T) {
		// Filter level 0 — should return 1
		out := v.mustZrbWithS3(t, "list --config "+configPath+" --task "+lcTask+" --source s3 --level 0")
		var result listOutput
		require.NoError(t, json.Unmarshal([]byte(extractJSON(out)), &result))
		assert.Equal(t, 1, result.Summary.TotalBackups)

		// Filter level 1 — should return 0
		out = v.mustZrbWithS3(t, "list --config "+configPath+" --task "+lcTask+" --source s3 --level 1")
		require.NoError(t, json.Unmarshal([]byte(extractJSON(out)), &result))
		assert.Equal(t, 0, result.Summary.TotalBackups)
	})

	t.Run("RestoreDryRun", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "restore --config "+configPath+" --task "+lcTask+
			" --level 0 --target "+lcPool+"/restored --private-key "+lcKeyPath+" --source s3 --dry-run")
		assert.Contains(t, out, "DRY RUN MODE")
		assert.Contains(t, out, "No changes made.")

		// Verify no dataset was actually created (zfs list should fail)
		_, err := v.execSudo("zfs list -H -o name " + lcPool + "/restored")
		assert.Error(t, err, "dataset should not exist after dry-run")
	})

	t.Run("Restore", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "restore --config "+configPath+" --task "+lcTask+
			" --level 0 --target "+lcPool+"/restored --private-key "+lcKeyPath+" --source s3")
		assert.Contains(t, out, "Restore completed successfully")
	})

	t.Run("VerifyRestore", func(t *testing.T) {
		origMount := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+lcPool+"/"+lcDataset)
		restMount := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+lcPool+"/restored")

		// Compare hello.txt
		origHello := v.mustExecSudo(t, "cat "+origMount+"/subdir/hello.txt")
		restHello := v.mustExecSudo(t, "cat "+restMount+"/subdir/hello.txt")
		assert.Equal(t, origHello, restHello, "hello.txt content mismatch")

		// Compare numbers.txt
		origNums := v.mustExecSudo(t, "cat "+origMount+"/numbers.txt")
		restNums := v.mustExecSudo(t, "cat "+restMount+"/numbers.txt")
		assert.Equal(t, origNums, restNums, "numbers.txt content mismatch")

		// Compare random.bin via checksum
		origHash := v.mustExecSudo(t, "md5sum "+origMount+"/random.bin | awk '{print $1}'")
		restHash := v.mustExecSudo(t, "md5sum "+restMount+"/random.bin | awk '{print $1}'")
		assert.Equal(t, origHash, restHash, "random.bin checksum mismatch")
	})

	t.Run("Cleanup", func(t *testing.T) {
		v.execSudo("zfs destroy -rf " + lcPool + "/restored")
		v.execSudo("zfs destroy -rf " + lcPool + "/" + lcDataset)
		v.execSudo("rm -rf " + lcBaseDir)
		v.exec("rm -f " + configPath + " " + lcKeyPath)
		v.exec("mc rb --force myminio/" + minioBucket)
	})
}

// listOutput mirrors the JSON structure from the list command.
type listOutput struct {
	Task    string     `json:"task"`
	Pool    string     `json:"pool"`
	Dataset string     `json:"dataset"`
	Source  string     `json:"source"`
	Backups []listInfo `json:"backups"`
	Summary struct {
		TotalBackups       int `json:"total_backups"`
		FullBackups        int `json:"full_backups"`
		IncrementalBackups int `json:"incremental_backups"`
	} `json:"summary"`
}

type listInfo struct {
	Level          int16  `json:"level"`
	Type           string `json:"type"`
	Snapshot       string `json:"snapshot"`
	ParentSnapshot string `json:"parent_snapshot,omitempty"`
	Blake3Hash     string `json:"blake3_hash"`
	PartsCount     int    `json:"parts_count"`
	S3Path         string `json:"s3_path"`
}
