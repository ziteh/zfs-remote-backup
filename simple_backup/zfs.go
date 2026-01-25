package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"

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
func runZfsSendAndSplit(targetSnapshot, parentSnapshot, exportDir string) (string, error) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	outputPattern := filepath.Join(exportDir, "snapshot.part-")
	outputPatternTmp := filepath.Join(exportDir, "snapshot.part-.tmp")

	// Cleanup function in case of failure
	success := false
	defer func() {
		if !success {
			matches, _ := filepath.Glob(outputPatternTmp + "*")
			for _, f := range matches {
				if err := os.Remove(f); err != nil {
					slog.Warn("Failed to clean up", "file", f, "error", err)
				}
			}
		}
	}()

	// Prepare zfs send command
	args := []string{"send", "-L"} // TODO: -c for compression
	if parentSnapshot != "" {
		args = append(args, "-i", parentSnapshot)
		slog.Info("Running incremental send", "parentSnapshot", parentSnapshot, "snapshot", targetSnapshot)
	} else {
		slog.Info("Running full send", "snapshot", targetSnapshot)
	}
	args = append(args, targetSnapshot)
	zfsCmd := exec.CommandContext(ctx, "zfs", args...)
	zfsCmd.Stderr = os.Stderr

	// Prepare split command
	splitCmd := exec.CommandContext(ctx, "split", "-b", "3G", "-a", "6", "-", outputPatternTmp)
	splitCmd.Stderr = os.Stderr

	// Hold zfs snapshot to prevent deletion during send
	holdTag := fmt.Sprintf("zrb:%d", time.Now().Unix())
	holdCtx, cancelHold := context.WithTimeout(ctx, 10*time.Second)
	if err := exec.CommandContext(holdCtx, "zfs", "hold", holdTag, targetSnapshot).Run(); err != nil {
		cancelHold()
		slog.Error("Failed to hold snapshot", "snapshot", targetSnapshot, "error", err)
		return "", fmt.Errorf("failed to hold snapshot: %w", err)
	}
	cancelHold()
	defer func() {
		releaseCtx, cancelRelease := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancelRelease()
		if err := exec.CommandContext(releaseCtx, "zfs", "release", holdTag, targetSnapshot).Run(); err != nil {
			slog.Warn("Failed to release snapshot hold", "holdTag", holdTag, "error", err)

		}
	}()

	// Pipeline: ZFS stdout -> TeeReader(hasher) -> Split stdin
	zfsStdout, err := zfsCmd.StdoutPipe()
	if err != nil {
		slog.Error("Failed to get zfs stdout", "error", err)
		return "", fmt.Errorf("failed to get zfs stdout: %w", err)
	}

	hasher := blake3.New()
	splitCmd.Stdin = io.TeeReader(zfsStdout, hasher)

	// Start the commands
	if err := splitCmd.Start(); err != nil {
		slog.Error("Failed to start split command", "error", err)
		return "", fmt.Errorf("failed to start split: %w", err)
	}

	if err := zfsCmd.Start(); err != nil {
		_ = splitCmd.Process.Kill()
		_ = splitCmd.Wait() // Clean up zombie process
		slog.Error("Failed to start zfs command", "error", err)
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
				slog.Error("ZFS send failed", "error", err)
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
				slog.Error("Split failed", "error", err)
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
		slog.Error("Pipeline failed", "errors", errs)
		return "", fmt.Errorf("pipeline failed: %v", errs)
	}

	// Rename .tmp files to final names
	matches, err := filepath.Glob(outputPatternTmp + "*")
	if err != nil {
		slog.Error("Failed to glob tmp files", "error", err)
		return "", fmt.Errorf("failed to glob tmp files: %w", err)
	}
	for _, tmpFile := range matches {
		finalFile := strings.TrimSuffix(tmpFile, ".tmp")
		if err := os.Rename(tmpFile, finalFile); err != nil {
			slog.Error("Failed to rename tmp file", "tmpFile", tmpFile, "finalFile", finalFile, "error", err)
			return "", fmt.Errorf("failed to rename tmp file: %w", err)
		}
		slog.Debug("Renamed tmp file", "tmpFile", tmpFile, "finalFile", finalFile)
	}

	// All operations successful
	success = true
	blake3Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	slog.Info("ZFS send and split completed successfully", "outputPattern", outputPattern, "blake3", blake3Hash)

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
		// Newest first
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
