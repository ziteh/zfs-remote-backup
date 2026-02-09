//go:build e2e_vm

package e2e

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	vmName        = "zrb-vm"
	remoteBin     = "/tmp/zrb"
	privateKeyPath = "/home/ubuntu/age_private_key.txt"
	agePublicKey  = "age1tawkwd7rjxwjmhnyv0df6s5c9pfmk5fnsyu439mr89lrn0f0594q3hjcav"
)

type vm struct {
	name string
}

func newVM() *vm {
	return &vm{name: vmName}
}

// exec runs a command inside the VM via multipass exec.
func (v *vm) exec(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "multipass", "exec", v.name, "--", "bash", "-lc", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// execLong runs a command with a longer timeout for backup/restore operations.
func (v *vm) execLong(command string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "multipass", "exec", v.name, "--", "bash", "-lc", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mustExec runs a command and fails the test on error.
func (v *vm) mustExec(t *testing.T, command string) string {
	t.Helper()
	out, err := v.exec(command)
	require.NoError(t, err, "VM exec failed: %s\nOutput: %s", command, out)
	return out
}

// execSudo runs a sudo command inside the VM.
func (v *vm) execSudo(command string) (string, error) {
	return v.exec("sudo " + command)
}

// execSudoWithS3 runs a sudo command with AWS MinIO credentials.
func (v *vm) execSudoWithS3(command string) (string, error) {
	wrapped := fmt.Sprintf(
		"export AWS_ACCESS_KEY_ID=admin AWS_SECRET_ACCESS_KEY=password123 && sudo -E %s",
		command,
	)
	return v.execLong(wrapped)
}

// transfer sends a local file to the VM.
func (v *vm) transfer(localPath, remotePath string) error {
	cmd := exec.Command("multipass", "transfer", localPath, fmt.Sprintf("%s:%s", v.name, remotePath))
	return cmd.Run()
}

// writeFile creates a file on the VM with the given content.
func (v *vm) writeFile(remotePath, content string) error {
	_, err := v.exec(fmt.Sprintf("cat > %s <<'HEREDOC_EOF'\n%s\nHEREDOC_EOF", remotePath, content))
	return err
}

// fileExists checks if a file exists on the VM.
func (v *vm) fileExists(remotePath string) bool {
	_, err := v.exec(fmt.Sprintf("test -f %s", remotePath))
	return err == nil
}

// isReachable checks if the VM is running and reachable.
func (v *vm) isReachable() bool {
	_, err := v.exec("echo ok")
	return err == nil
}

// zrb runs the zrb binary on the VM with sudo.
func (v *vm) zrb(args string) (string, error) {
	return v.execLong(fmt.Sprintf("sudo %s %s", remoteBin, args))
}

// zrbWithS3 runs the zrb binary with S3/MinIO credentials.
func (v *vm) zrbWithS3(args string) (string, error) {
	return v.execSudoWithS3(fmt.Sprintf("%s %s", remoteBin, args))
}

// buildAndTransfer cross-compiles the binary and transfers it to the VM.
func buildAndTransfer(t *testing.T, v *vm) {
	t.Helper()

	// Cross-compile for linux/arm64
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", "../../build/zrb_linux_arm64", "../../cmd/zrb")
	cmd.Env = append(cmd.Environ(), "GOOS=linux", "GOARCH=arm64")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "Failed to cross-compile: %s", string(out))

	// Transfer to VM
	err = v.transfer("../../build/zrb_linux_arm64", "/tmp/zrb_temp")
	require.NoError(t, err, "Failed to transfer binary to VM")

	// Move to final location and make executable
	v.mustExec(t, "sudo mv /tmp/zrb_temp /tmp/zrb && sudo chmod +x /tmp/zrb")
}

// localConfig generates a YAML config with S3 disabled.
func localConfig(baseDir, taskName string) string {
	return fmt.Sprintf(`base_dir: %s
age_public_key: "%s"
s3:
  enabled: false
  bucket: ""
  region: ""
  prefix: ""
  endpoint: ""
  storage_class:
    backup_data: []
    manifest: ""
tasks:
  - name: %s
    enabled: true
    pool: testpool
    dataset: backup_data`, baseDir, agePublicKey, taskName)
}

// s3Config generates a YAML config with S3/MinIO enabled.
func s3Config(baseDir, taskName string) string {
	return fmt.Sprintf(`base_dir: %s
age_public_key: "%s"
s3:
  enabled: true
  bucket: zrb-test
  region: us-east-1
  prefix: test-backups/
  endpoint: http://127.0.0.1:9000
  storage_class:
    manifest: STANDARD
    backup_data:
      - STANDARD
      - STANDARD
      - STANDARD
  retry:
    max_attempts: 3
tasks:
  - name: %s
    enabled: true
    pool: testpool
    dataset: backup_data`, baseDir, agePublicKey, taskName)
}

// shutdownConfig generates a config for shutdown tests.
func shutdownConfig(baseDir, dataset string) string {
	return fmt.Sprintf(`base_dir: %s
age_public_key: "%s"
s3:
  enabled: false
tasks:
  - name: shutdown_test
    enabled: true
    pool: testpool
    dataset: %s`, baseDir, agePublicKey, dataset)
}
