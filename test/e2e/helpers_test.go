//go:build e2e_vm

package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	vmName     = "zrb-test-vm"
	remoteBin  = "/tmp/zrb"
	configPath = "/tmp/zrb_test_config.yaml"

	minioEndpoint  = "http://127.0.0.1:9000"
	minioAccessKey = "admin"
	minioSecretKey = "password"
	minioBucket    = "zrb-test"
)

type vm struct {
	name string
}

func newVM() *vm {
	return &vm{name: vmName}
}

func (v *vm) execWithTimeout(command string, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "multipass", "exec", v.name, "--", "bash", "-lc", command)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func (v *vm) exec(command string) (string, error) {
	return v.execWithTimeout(command, 2*time.Minute)
}

func (v *vm) mustExec(t *testing.T, command string) string {
	t.Helper()
	out, err := v.exec(command)
	require.NoError(t, err, "command failed: %s\noutput: %s", command, out)
	return out
}

func (v *vm) execSudo(command string) (string, error) {
	return v.exec("sudo " + command)
}

func (v *vm) mustExecSudo(t *testing.T, command string) string {
	t.Helper()
	out, err := v.execSudo(command)
	require.NoError(t, err, "sudo command failed: %s\noutput: %s", command, out)
	return out
}

func (v *vm) zrbWithS3(args string) (string, error) {
	command := fmt.Sprintf("AWS_ACCESS_KEY_ID=%s AWS_SECRET_ACCESS_KEY=%s sudo -E %s %s",
		minioAccessKey, minioSecretKey, remoteBin, args)
	return v.execWithTimeout(command, 5*time.Minute)
}

func (v *vm) mustZrbWithS3(t *testing.T, args string) string {
	t.Helper()
	out, err := v.zrbWithS3(args)
	require.NoError(t, err, "zrb command failed: %s\noutput: %s", args, out)
	return out
}

func (v *vm) transfer(localPath, remotePath string) error {
	cmd := exec.Command("multipass", "transfer", localPath, fmt.Sprintf("%s:%s", v.name, remotePath))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("transfer failed: %w\noutput: %s", err, string(out))
	}
	return nil
}

func (v *vm) writeFile(t *testing.T, remotePath, content string) {
	t.Helper()
	tmp, err := os.CreateTemp("", "zrb-e2e-*")
	require.NoError(t, err)
	defer os.Remove(tmp.Name())

	_, err = tmp.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, tmp.Close())

	require.NoError(t, v.transfer(tmp.Name(), remotePath))
}

func buildBinary(t *testing.T) string {
	t.Helper()
	buildDir := "../../build"
	binary := buildDir + "/zrb_linux_arm64"

	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", binary, "./../../cmd/zrb")
	cmd.Env = append(os.Environ(), "GOOS=linux", "GOARCH=arm64")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))
	return binary
}

func buildAndTransfer(t *testing.T, v *vm) {
	t.Helper()
	binary := buildBinary(t)
	require.NoError(t, v.transfer(binary, remoteBin))
	v.mustExecSudo(t, "chmod +x "+remoteBin)
}

// extractJSON extracts a JSON object from mixed output (slog lines + JSON).
func extractJSON(output string) string {
	start := strings.Index(output, "{")
	end := strings.LastIndex(output, "}")
	if start >= 0 && end > start {
		return output[start : end+1]
	}
	return output
}

func s3Config(baseDir, taskName, pool, dataset, agePublicKey string) string {
	return fmt.Sprintf(`base_dir: %s
age_public_key: %s
s3:
  enabled: true
  bucket: %s
  region: us-east-1
  prefix: backups/
  endpoint: %s
  storage_class:
    manifest: STANDARD
    backup_data:
      - STANDARD
      - STANDARD
      - STANDARD
      - STANDARD
      - STANDARD
  retry:
    max_attempts: 3
tasks:
  - name: %s
    pool: %s
    dataset: %s
    enabled: true
`, baseDir, agePublicKey, minioBucket, minioEndpoint, taskName, pool, dataset)
}
