package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/zeebo/blake3"
)

// runZfsSendAndSplit executes zfs send and splits the output into parts while computing BLAKE3 hash
func runZfsSendAndSplit(snapshotPath, baseSnapshot, exportDir string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// TODO: save as .tmp files and rename on success?
	outputPattern := filepath.Join(exportDir, "snapshot.part-")

	// Cleanup function in case of failure
	success := false
	defer func() {
		if !success {
			matches, _ := filepath.Glob(outputPattern + "*")
			for _, f := range matches {
				if err := os.Remove(f); err != nil {
					log.Printf("Warning: failed to clean up %s: %v", f, err)
				}
			}
		}
	}()

	// Prepare zfs send command
	args := []string{"send", "-L", "-c"}
	if baseSnapshot != "" {
		args = append(args, "-i", baseSnapshot)
		log.Printf("Running incremental send: %s %s", baseSnapshot, snapshotPath)
	} else {
		log.Printf("Running full send: %s", snapshotPath)
	}
	args = append(args, snapshotPath)
	zfsCmd := exec.CommandContext(ctx, "zfs", args...)
	zfsCmd.Stderr = os.Stderr

	// Prepare split command
	splitCmd := exec.CommandContext(ctx, "split", "-b", "3G", "-a", "6", "-", outputPattern)
	splitCmd.Stderr = os.Stderr

	// Hold zfs snapshot to prevent deletion during send
	holdTag := fmt.Sprintf("zrb:%d", time.Now().Unix())
	holdCtx, cancelHold := context.WithTimeout(ctx, 10*time.Second)
	if err := exec.CommandContext(holdCtx, "zfs", "hold", holdTag, snapshotPath).Run(); err != nil {
		cancelHold()
		return "", fmt.Errorf("failed to hold snapshot: %w", err)
	}
	cancelHold()
	defer func() {
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelRelease()
		if err := exec.CommandContext(releaseCtx, "zfs", "release", holdTag, snapshotPath).Run(); err != nil {
			log.Printf("Warning: failed to release snapshot hold %s: %v", holdTag, err)
		}
	}()

	// Pipeline: ZFS stdout -> TeeReader(hasher) -> Split stdin
	zfsStdout, err := zfsCmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to get zfs stdout: %w", err)
	}

	hasher := blake3.New()
	splitCmd.Stdin = io.TeeReader(zfsStdout, hasher)

	// Start the commands
	if err := splitCmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start split: %w", err)
	}

	if err := zfsCmd.Start(); err != nil {
		_ = splitCmd.Process.Kill()
		_ = splitCmd.Wait() // Clean up zombie process
		return "", fmt.Errorf("failed to start zfs: %w", err)
	}

	// Wait for both processes to complete
	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := zfsCmd.Wait(); err != nil {
			// Check if the error is due to context cancellation
			if ctx.Err() == nil {
				errChan <- fmt.Errorf("zfs send failed: %w", err)
			}
			cancel() // Ensure the other process also terminates
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := splitCmd.Wait(); err != nil {
			// Check if the error is due to context cancellation
			if ctx.Err() == nil {
				errChan <- fmt.Errorf("split failed: %w", err)
			}
			cancel()
		}
	}()

	wg.Wait()
	close(errChan)

	// Collect errors
	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		return "", fmt.Errorf("pipeline failed: %v", errs)
	}

	// All operations successful
	success = true
	blake3Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	log.Printf("ZFS send and split completed successfully")
	log.Printf("Output: %s*", outputPattern)
	log.Printf("BLAKE3: %s", blake3Hash)

	return blake3Hash, nil
}

func listSnapshots(pool, dataset, prefix string) ([]string, error) {
	// Snapshot name format: dataset@prefix_YYYY-MM-DD_HH-MM
	cmd := exec.Command(
		"zfs",
		"list",
		"-H", // scripting mode
		"-o",
		"name", // GUID?
		"-t",
		"snapshot",
		fmt.Sprintf("%s/%s", pool, dataset),
	)
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var snapshots []string
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "@", 2)
		if len(parts) != 2 {
			continue
		}

		snapName := parts[1]
		if prefix != "" && !strings.HasPrefix(snapName, prefix) {
			continue
		}

		snapshots = append(snapshots, line)
	}

	// Lexicographical order == chronological order
	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i] > snapshots[j]
	})

	return snapshots, nil
}

func createSnapshot(pool, dataset, prefix string) error {
	// Snapshot name format: dataset@prefix_YYYY-MM-DD_HH-MM
	date := time.Now().Format("2006-01-02_15-04")
	fullSnapshotName := fmt.Sprintf("%s/%s@%s_%s", pool, dataset, prefix, date)

	cmd := exec.Command("zfs", "snapshot", fullSnapshotName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
