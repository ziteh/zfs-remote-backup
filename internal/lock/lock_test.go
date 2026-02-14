package lock

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestAcquireAndRelease(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "zrb.lock")

	release, err := Acquire(lockPath)
	require.NoError(t, err)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	var entry Entry
	require.NoError(t, yaml.Unmarshal(data, &entry))
	assert.Equal(t, os.Getpid(), entry.Pid)
	assert.NotEmpty(t, entry.StartedAt)

	require.NoError(t, release())
	_, err = os.Stat(lockPath)
	assert.True(t, os.IsNotExist(err))
}

func TestAcquireBlockedByLivePid(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "zrb.lock")

	release, err := Acquire(lockPath)
	require.NoError(t, err)
	defer release()

	_, err = Acquire(lockPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already locked by pid")
}

func TestAcquireReclaimsStaleLock(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "zrb.lock")

	stale := &Entry{Pid: 999999999, StartedAt: "2024-01-01T00:00:00Z"}
	require.NoError(t, writeLock(lockPath, stale))

	release, err := Acquire(lockPath)
	require.NoError(t, err)

	data, err := os.ReadFile(lockPath)
	require.NoError(t, err)
	var entry Entry
	require.NoError(t, yaml.Unmarshal(data, &entry))
	assert.Equal(t, os.Getpid(), entry.Pid)

	require.NoError(t, release())
}

func TestReleaseIdempotent(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "zrb.lock")

	release, err := Acquire(lockPath)
	require.NoError(t, err)

	require.NoError(t, release())
	require.NoError(t, release())
}
