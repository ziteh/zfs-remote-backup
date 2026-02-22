package zfs

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

// SendAndSplit executes zfs send and splits the output into parts while computing BLAKE3 hash
func SendAndSplit(ctx context.Context, targetSnapshot, parentSnapshot, exportDir string) (string, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	outputPattern := filepath.Join(exportDir, "snapshot.part-")
	outputPatternTmp := filepath.Join(exportDir, "snapshot.part-")

	success := false
	defer func() {
		if !success {
			matches, _ := filepath.Glob(outputPatternTmp + "*.tmp")
			for _, f := range matches {
				if err := os.Remove(f); err != nil {
					slog.Warn("Failed to clean up", "file", f, "error", err)
				}
			}
		}
	}()

	args := []string{"send", "-L"}
	if parentSnapshot != "" {
		args = append(args, "-i", parentSnapshot)
		slog.Info("Running incremental send", "parentSnapshot", parentSnapshot, "snapshot", targetSnapshot)
	} else {
		slog.Info("Running full send", "snapshot", targetSnapshot)
	}
	args = append(args, targetSnapshot)
	zfsCmd := exec.CommandContext(ctx, "zfs", args...)
	zfsCmd.Stderr = os.Stderr

	splitCmd := exec.CommandContext(ctx, "split", "-b", "3G", "-a", "6", "--additional-suffix=.tmp", "-", outputPatternTmp)
	splitCmd.Stderr = os.Stderr

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

	pr, pw, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("failed to create pipe: %w", err)
	}
	zfsCmd.Stdout = pw

	hasher := blake3.New()
	splitCmd.Stdin = io.TeeReader(pr, hasher)

	if err := splitCmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		slog.Error("Failed to start split command", "error", err)
		return "", fmt.Errorf("failed to start split: %w", err)
	}

	if err := zfsCmd.Start(); err != nil {
		pw.Close()
		pr.Close()
		_ = splitCmd.Process.Kill()
		_ = splitCmd.Wait()
		slog.Error("Failed to start zfs command", "error", err)
		return "", fmt.Errorf("failed to start zfs: %w", err)
	}

	// Close our copy of the write end so split gets EOF when zfs exits.
	pw.Close()

	var wg sync.WaitGroup
	errChan := make(chan error, 2)

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := zfsCmd.Wait(); err != nil {
			if ctx.Err() == nil {
				slog.Error("ZFS send failed", "error", err)
				errChan <- fmt.Errorf("zfs send failed: %w", err)
			}
			cancel()
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := splitCmd.Wait(); err != nil {
			if ctx.Err() == nil {
				slog.Error("Split failed", "error", err)
				errChan <- fmt.Errorf("split failed: %w", err)
			}
			cancel()
		}
	}()

	wg.Wait()
	pr.Close()
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		slog.Error("Pipeline failed", "errors", errs)
		return "", fmt.Errorf("pipeline failed: %v", errs)
	}

	matches, err := filepath.Glob(outputPatternTmp + "*.tmp")
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

	success = true
	blake3Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	slog.Info("ZFS send and split completed successfully", "outputPattern", outputPattern, "blake3", blake3Hash)

	return blake3Hash, nil
}

func ListSnapshots(pool, dataset, prefix string) ([]string, error) {
	cmd := exec.Command(
		"zfs",
		"list",
		"-H",
		"-o",
		"name",
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

	sort.SliceStable(snapshots, func(i, j int) bool {
		return snapshots[i] > snapshots[j]
	})

	return snapshots, nil
}

func CheckDatasetExists(pool, dataset string) error {
	cmd := exec.Command("zfs", "list", "-H", "-o", "name", fmt.Sprintf("%s/%s", pool, dataset))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ZFS dataset %s/%s not found or not accessible", pool, dataset)
	}
	return nil
}

func CheckPoolExists(pool string) error {
	cmd := exec.Command("zfs", "list", "-H", "-o", "name", pool)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ZFS pool %s not found or not accessible", pool)
	}
	return nil
}

func CreateSnapshot(pool, dataset, prefix string) error {
	date := time.Now().Format("2006-01-02_15-04")
	fullSnapshotName := fmt.Sprintf("%s/%s@%s_%s", pool, dataset, prefix, date)

	cmd := exec.Command("zfs", "snapshot", fullSnapshotName)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
