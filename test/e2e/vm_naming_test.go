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

func runTmpNamingTests(t *testing.T, v *vm) {
	configPath := "/tmp/zrb_naming_test_config.yaml"
	baseDir := "/home/ubuntu/zrb_naming_test"
	taskName := "naming_test"

	// Write config
	cfg := fmt.Sprintf(`base_dir: %s
age_public_key: "%s"
s3:
  enabled: false
tasks:
  - name: %s
    enabled: true
    pool: testpool
    dataset: backup_data`, baseDir, agePublicKey, taskName)
	require.NoError(t, v.writeFile(configPath, cfg))

	// Clean previous
	v.exec("sudo rm -rf " + baseDir)

	// Create snapshot
	ts := time.Now().Unix()
	snapName := fmt.Sprintf("testpool/backup_data@zrb_naming_test_%d", ts)
	v.mustExec(t, "sudo zfs snapshot "+snapName)

	// Cleanup on finish
	t.Cleanup(func() {
		v.exec("sudo zfs destroy " + snapName + " 2>/dev/null || true")
		v.exec("sudo rm -rf " + baseDir)
	})

	// Run backup (captures output for inspection)
	t.Run("BackupCompletes", func(t *testing.T) {
		out, err := v.zrb(fmt.Sprintf("backup --config %s --task %s --level 0", configPath, taskName))
		require.NoError(t, err, "backup failed: %s", out)
	})

	t.Run("FinalFilesNoTmpSuffix", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf(
			"find %s/task/testpool/backup_data/level0/ -name 'snapshot.part-*' ! -name '*.tmp' ! -name '*.age' 2>/dev/null || true",
			baseDir))
		require.NoError(t, err)
		assert.NotEmpty(t, out, "should have final part files without .tmp suffix")

		// Verify 6-letter suffix format
		sampleFile := strings.Split(out, "\n")[0]
		assert.Regexp(t, `snapshot\.part-[a-z]{6}$`, sampleFile,
			"filename should match snapshot.part-XXXXXX format")
	})

	t.Run("NoTmpFilesRemain", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf(
			"find %s/task/testpool/backup_data/level0/ -name '*.tmp' 2>/dev/null | wc -l",
			baseDir))
		require.NoError(t, err)
		assert.Equal(t, "0", strings.TrimSpace(out), "no .tmp files should remain after backup")
	})

	t.Run("EncryptedFilesExist", func(t *testing.T) {
		out, err := v.exec(fmt.Sprintf(
			"find %s/task/testpool/backup_data/level0/ -name '*.age' 2>/dev/null || true",
			baseDir))
		require.NoError(t, err)
		assert.NotEmpty(t, out, "should have .age encrypted files")

		// Verify format: snapshot.part-XXXXXX.age
		sampleAge := strings.Split(out, "\n")[0]
		assert.Regexp(t, `snapshot\.part-[a-z]{6}\.age$`, sampleAge,
			"encrypted filename should match snapshot.part-XXXXXX.age format")
	})
}
