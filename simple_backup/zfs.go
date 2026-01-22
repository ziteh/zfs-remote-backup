package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/zeebo/blake3"
)

// runZfsSendAndSplit executes zfs send and splits the output into parts while computing BLAKE3 hash
func runZfsSendAndSplit(snapshotPath, exportDir string) (string, error) {
	log.Printf("Running: zfs send %s | tee >(blake3) | split -b 3G -a 4 - snapshot.part-", snapshotPath)

	zfsCmd := exec.Command("zfs", "send", snapshotPath)

	outputPattern := filepath.Join(exportDir, "snapshot.part-")
	splitCmd := exec.Command("split", "-b", "3G", "-a", "4", "-", outputPattern)

	zfsPipe, err := zfsCmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	hasher := blake3.New()
	teeReader := io.TeeReader(zfsPipe, hasher)
	splitCmd.Stdin = teeReader

	zfsCmd.Stderr = os.Stderr
	splitCmd.Stderr = os.Stderr

	if err := splitCmd.Start(); err != nil {
		return "", err
	}
	if err := zfsCmd.Start(); err != nil {
		return "", err
	}

	if err := zfsCmd.Wait(); err != nil {
		return "", err
	}
	if err := splitCmd.Wait(); err != nil {
		return "", err
	}

	blake3Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	log.Println("ZFS send and split completed")
	return blake3Hash, nil
}

// calculateBLAKE3 computes the BLAKE3 hash of a file
func calculateBLAKE3(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
