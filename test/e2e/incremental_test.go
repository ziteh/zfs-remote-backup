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
	incPool    = "testpool"
	incDataset = "incremental"
	incTask    = "e2e-incremental"
	incBaseDir = "/tmp/zrb_e2e_incremental"
	incKeyPath = "/tmp/zrb_e2e_incremental_key.txt"
)

func TestIncrementalBackup(t *testing.T) {
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

		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Public key:") {
				agePublicKey = strings.TrimSpace(strings.TrimPrefix(line, "Public key:"))
			}
		}

		require.NotEmpty(t, agePublicKey, "failed to extract public key")

		// genkey writes private key to zrb_private.key; move it to the test path
		v.mustExec(t, "mv zrb_private.key "+incKeyPath)
		v.exec("rm -f zrb_public.key")
	})

	t.Run("PrepareData", func(t *testing.T) {
		cfg := s3Config(incBaseDir, incTask, incPool, incDataset, agePublicKey)
		v.writeFile(t, configPath, cfg)

		v.mustExecSudo(t, "zfs create "+incPool+"/"+incDataset)

		mountpoint := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+incPool+"/"+incDataset)
		v.mustExecSudo(t, "dd if=/dev/urandom of="+mountpoint+"/data.bin bs=1M count=2")
		v.mustExecSudo(t, "bash -c \"echo 'initial content' > "+mountpoint+"/file.txt\"")

		v.mustExecSudo(t, remoteBin+" snapshot --pool "+incPool+" --dataset "+incDataset+" --prefix zrb_level0")

		out := v.mustExecSudo(t, "zfs list -t snapshot -H -o name "+incPool+"/"+incDataset)
		assert.Contains(t, out, "zrb_level0")
	})

	t.Run("BackupLevel0", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "backup --config "+configPath+" --task "+incTask+" --level 0")
		assert.Contains(t, out, "Backup completed")
	})

	t.Run("ModifyData", func(t *testing.T) {
		mountpoint := v.mustExecSudo(t, "zfs get -H -o value mountpoint "+incPool+"/"+incDataset)
		v.mustExecSudo(t, "bash -c \"echo 'modified content' > "+mountpoint+"/file.txt\"")
		v.mustExecSudo(t, "bash -c \"echo 'new file' > "+mountpoint+"/added.txt\"")
	})

	t.Run("SnapshotLevel1", func(t *testing.T) {
		v.mustExecSudo(t, remoteBin+" snapshot --pool "+incPool+" --dataset "+incDataset+" --prefix zrb_level1")

		out := v.mustExecSudo(t, "zfs list -t snapshot -H -o name "+incPool+"/"+incDataset)
		assert.Contains(t, out, "zrb_level1")
	})

	t.Run("BackupLevel1", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "backup --config "+configPath+" --task "+incTask+" --level 1")
		assert.Contains(t, out, "Backup completed")
	})

	t.Run("ListAllLevels", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "list --config "+configPath+" --task "+incTask+" --source s3 --level -1")

		var result listOutput
		require.NoError(t, json.Unmarshal([]byte(extractJSON(out)), &result), "failed to parse list JSON: %s", out)

		assert.Equal(t, 2, result.Summary.TotalBackups)
		assert.Equal(t, 1, result.Summary.FullBackups)
		assert.Equal(t, 1, result.Summary.IncrementalBackups)
	})

	t.Run("ListFilterLevel1", func(t *testing.T) {
		out := v.mustZrbWithS3(t, "list --config "+configPath+" --task "+incTask+" --source s3 --level 1")

		var result listOutput
		require.NoError(t, json.Unmarshal([]byte(extractJSON(out)), &result))

		require.Len(t, result.Backups, 1)
		assert.Equal(t, int16(1), result.Backups[0].Level)
		assert.Equal(t, "incremental", result.Backups[0].Type)
		assert.NotEmpty(t, result.Backups[0].ParentSnapshot)
	})

	t.Run("VerifyS3Structure", func(t *testing.T) {
		dataPath := "myminio/" + minioBucket + "/backups/data/" + incPool + "/" + incDataset + "/"
		out := v.mustExec(t, "mc ls --recursive "+dataPath)
		assert.Contains(t, out, ".age", "no encrypted files found")

		manifestPath := "myminio/" + minioBucket + "/backups/manifests/" + incPool + "/" + incDataset + "/"
		out = v.mustExec(t, "mc ls --recursive "+manifestPath)
		assert.Contains(t, out, "task_manifest.yaml")
		assert.Contains(t, out, "last_backup_manifest.yaml")
	})

	t.Run("Cleanup", func(t *testing.T) {
		v.execSudo("zfs destroy -rf " + incPool + "/" + incDataset)
		v.execSudo("rm -rf " + incBaseDir)
		v.exec("rm -f " + configPath + " " + incKeyPath)
		v.exec("mc rb --force myminio/" + minioBucket)
	})
}
