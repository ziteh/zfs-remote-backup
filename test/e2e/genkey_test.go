package e2e

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenkeyCommand tests the genkey command generates valid age key pairs
func TestGenkeyCommand(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "../../build/zrb_test", "../../cmd/zrb")
	err := buildCmd.Run()
	require.NoError(t, err, "Failed to build zrb binary for testing")

	// Run genkey command
	cmd := exec.Command("../../build/zrb_test", "genkey")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "genkey command should execute successfully")

	outputStr := string(output)

	// Verify output contains expected elements
	t.Run("output contains public key", func(t *testing.T) {
		assert.Contains(t, outputStr, "Public key:", "output should contain public key label")
		assert.Contains(t, outputStr, "age1", "public key should start with 'age1'")
	})

	t.Run("output contains private key", func(t *testing.T) {
		assert.Contains(t, outputStr, "Private key:", "output should contain private key label")
		assert.Contains(t, outputStr, "AGE-SECRET-KEY-", "private key should start with 'AGE-SECRET-KEY-'")
	})

	t.Run("output contains warning", func(t *testing.T) {
		assert.Contains(t, outputStr, "Keep your private key secure", "output should contain security warning")
	})

	t.Run("keys are valid format", func(t *testing.T) {
		lines := strings.Split(outputStr, "\n")
		var publicKey, privateKey string

		for _, line := range lines {
			if strings.Contains(line, "Public key:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					publicKey = strings.TrimSpace(parts[1])
				}
			}
			if strings.Contains(line, "Private key:") {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					privateKey = strings.TrimSpace(parts[1])
				}
			}
		}

		assert.NotEmpty(t, publicKey, "public key should be extracted")
		assert.NotEmpty(t, privateKey, "private key should be extracted")
		assert.True(t, strings.HasPrefix(publicKey, "age1"), "public key should have correct prefix")
		assert.True(t, strings.HasPrefix(privateKey, "AGE-SECRET-KEY-"), "private key should have correct prefix")

		// Age public keys are 62 characters (age1 + 58 chars)
		assert.Len(t, publicKey, 62, "public key should be 62 characters")
		// Age private keys are 74 characters (AGE-SECRET-KEY-1 + 58 chars)
		assert.Len(t, privateKey, 74, "private key should be 74 characters")
	})
}

// TestGenkeyMultipleRuns tests that genkey produces different keys on each run
func TestGenkeyMultipleRuns(t *testing.T) {
	// Build the binary first
	buildCmd := exec.Command("go", "build", "-o", "../../build/zrb_test", "../../cmd/zrb")
	err := buildCmd.Run()
	require.NoError(t, err, "Failed to build zrb binary for testing")

	// Run genkey twice
	cmd1 := exec.Command("../../build/zrb_test", "genkey")
	output1, err := cmd1.CombinedOutput()
	require.NoError(t, err, "first genkey run should succeed")

	cmd2 := exec.Command("../../build/zrb_test", "genkey")
	output2, err := cmd2.CombinedOutput()
	require.NoError(t, err, "second genkey run should succeed")

	// Extract keys from both outputs
	extractKey := func(output, prefix string) string {
		lines := strings.Split(output, "\n")
		for _, line := range lines {
			if strings.Contains(line, prefix) {
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					return strings.TrimSpace(parts[1])
				}
			}
		}
		return ""
	}

	publicKey1 := extractKey(string(output1), "Public key:")
	publicKey2 := extractKey(string(output2), "Public key:")
	privateKey1 := extractKey(string(output1), "Private key:")
	privateKey2 := extractKey(string(output2), "Private key:")

	t.Run("different public keys", func(t *testing.T) {
		assert.NotEqual(t, publicKey1, publicKey2, "each run should generate different public key")
	})

	t.Run("different private keys", func(t *testing.T) {
		assert.NotEqual(t, privateKey1, privateKey2, "each run should generate different private key")
	})
}
