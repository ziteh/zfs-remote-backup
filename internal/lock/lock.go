package lock

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type Entry struct {
	Pid       int    `yaml:"pid"`
	StartedAt string `yaml:"started_at"`
}

func readLock(path string) (*Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entry Entry
	if err := yaml.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func writeLock(path string, entry *Entry) error {
	data, err := yaml.Marshal(entry)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true
	}
	if err == syscall.ESRCH {
		return false
	}
	return true
}

// Returns a release function which should be called (deferred) when work is done.
func Acquire(lockPath string) (func() error, error) {
	existing, err := readLock(lockPath)
	if err != nil {
		return nil, err
	}

	if existing != nil && existing.Pid > 0 && isProcessAlive(existing.Pid) {
		return nil, fmt.Errorf("already locked by pid %d (started %s)", existing.Pid, existing.StartedAt)
	}

	entry := &Entry{
		Pid:       os.Getpid(),
		StartedAt: time.Now().Format(time.RFC3339),
	}
	if err := writeLock(lockPath, entry); err != nil {
		return nil, err
	}

	release := func() error {
		if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}

	return release, nil
}
