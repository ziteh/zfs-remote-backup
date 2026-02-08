package main

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

type LockEntry struct {
	Pid       int    `yaml:"pid"`
	Pool      string `yaml:"pool"`
	Dataset   string `yaml:"dataset"`
	StartedAt string `yaml:"started_at"`
}

func readLocks(path string) ([]LockEntry, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var locks []LockEntry
	if err := yaml.Unmarshal(data, &locks); err != nil {
		return nil, err
	}
	return locks, nil
}

func writeLocks(path string, locks []LockEntry) error {
	data, err := yaml.Marshal(locks)
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
	// for EPERM and other errors assume process exists
	return true
}

// AcquireLock tries to register a lock for pool+dataset in the YAML lock file.
// Returns a release function which should be called (deferred) when work is done.
func AcquireLock(lockPath, pool, dataset string) (func() error, error) {
	pid := os.Getpid()

	locks, err := readLocks(lockPath)
	if err != nil {
		return nil, err
	}

	var kept []LockEntry
	for _, l := range locks {
		if l.Pool == pool && l.Dataset == dataset {
			if isProcessAlive(l.Pid) {
				return nil, fmt.Errorf("dataset %s/%s is already locked by pid %d (started %s)", pool, dataset, l.Pid, l.StartedAt)
			}
			// stale entry: skip it
			continue
		}
		kept = append(kept, l)
	}

	// append our entry
	kept = append(kept, LockEntry{
		Pid:       pid,
		Pool:      pool,
		Dataset:   dataset,
		StartedAt: time.Now().Format(time.RFC3339),
	})

	if err := writeLocks(lockPath, kept); err != nil {
		return nil, err
	}

	release := func() error {
		locks, err := readLocks(lockPath)
		if err != nil {
			return err
		}
		var rem []LockEntry
		for _, l := range locks {
			if l.Pid == pid && l.Pool == pool && l.Dataset == dataset {
				continue
			}
			rem = append(rem, l)
		}
		if len(rem) == 0 {
			// remove file if empty
			if err := os.Remove(lockPath); err != nil && !os.IsNotExist(err) {
				return err
			}
			return nil
		}
		return writeLocks(lockPath, rem)
	}

	return release, nil
}
