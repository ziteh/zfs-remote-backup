//go:build e2e_vm

package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestVMAll(t *testing.T) {
	v := newVM()
	require.True(t, v.isReachable(), "VM %s must be running (provision with vm/setup_vm.sh)", vmName)

	buildAndTransfer(t, v)

	t.Run("Backup", func(t *testing.T) { runBackupTests(t, v) })
	t.Run("S3Restore", func(t *testing.T) { runS3RestoreTests(t, v) })
	t.Run("TmpNaming", func(t *testing.T) { runTmpNamingTests(t, v) })
	t.Run("GracefulShutdown", func(t *testing.T) { runGracefulShutdownTests(t, v) })
}
