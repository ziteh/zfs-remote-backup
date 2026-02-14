package backup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"zrb/internal/config"
	"zrb/internal/crypto"
	"zrb/internal/lock"
	"zrb/internal/manifest"
	"zrb/internal/remote"
	"zrb/internal/util"
	"zrb/internal/zfs"

	"filippo.io/age"
)

func Run(ctx context.Context, configPath string, backupLevel int16, taskName string) error {
	if backupLevel < 0 {
		return fmt.Errorf("backup level must be non-negative")
	}
	if taskName == "" {
		return fmt.Errorf("task name must be specified")
	}
	if ctx.Err() != nil {
		return fmt.Errorf("backup cancelled before start: %w", ctx.Err())
	}

	// Load configuration
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find the backup task
	task, err := cfg.FindTask(taskName)
	if err != nil {
		return err
	}
	if !task.Enabled {
		return fmt.Errorf("backup task is disabled: %s", taskName)
	}

	// Ensure base directory
	if err := os.MkdirAll(cfg.BaseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	// Setup logging
	logPath := filepath.Join(util.LogDir(cfg.BaseDir, task.Pool, task.Dataset), fmt.Sprintf("%s.log", time.Now().Format("2006-01-02")))
	logger, logFile, err := util.SetupLogging(logPath)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}
	defer logFile.Close()
	slog.SetDefault(logger)
	slog.Info("Backup started", "level", backupLevel, "pool", task.Pool, "dataset", task.Dataset)

	// Ensure run directory
	runDir := util.RunDir(cfg.BaseDir, task.Pool, task.Dataset)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	// Backup state management
	statePath := filepath.Join(runDir, "backup_state.yaml")
	state, err := loadOrCreateState(statePath, taskName, backupLevel)
	if err != nil {
		return fmt.Errorf("failed to load backup state: %w", err)
	}

	// Acquire lock for the dataset
	lockPath := filepath.Join(runDir, "zrb.lock")
	releaseLock, err := lock.Acquire(lockPath, task.Pool, task.Dataset)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}
	defer func() {
		if err := releaseLock(); err != nil {
			slog.Warn("Failed to release lock", "error", err)
		}
	}()

	// List snapshots and determine target snapshot for backup
	snapshots, err := zfs.ListSnapshots(task.Pool, task.Dataset, "zrb_level"+fmt.Sprint(backupLevel))
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}
	if len(snapshots) == 0 {
		return fmt.Errorf("no snapshots found for pool=%s dataset=%s", task.Pool, task.Dataset)
	}
	targetSnapshot := snapshots[0]
	if state.TargetSnapshot != "" {
		targetSnapshot = state.TargetSnapshot
	}
	slog.Info("Target snapshot determined", "targetSnapshot", targetSnapshot, "count", len(snapshots))

	// Determine task directory name
	taskDirName := util.TaskDirName(backupLevel, time.Now())
	if state.OutputDir != "" {
		outputDirParent := filepath.Dir(state.OutputDir)
		levelDir := filepath.Base(outputDirParent)
		dateDir := filepath.Base(state.OutputDir)
		taskDirName = filepath.Join(levelDir, dateDir)
	}

	// Ensure output directory
	outputDir := filepath.Join(cfg.BaseDir, "task", task.Pool, task.Dataset, taskDirName)
	if state.OutputDir == "" {
		if _, err := os.Stat(outputDir); err == nil {
			slog.Info("Cleaning up existing output directory", "path", outputDir)

			if err := os.RemoveAll(outputDir); err != nil {
				return fmt.Errorf("failed to remove existing output directory: %w", err)
			}
		}
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Determine parent snapshot
	lastPath := filepath.Join(cfg.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
	var parentSnapshot string
	var last *manifest.Last
	if backupLevel > 0 {
		// For level >= 1, we need to find the parent snapshot from the last backup manifest
		last, err = manifest.ReadLast(lastPath)
		if err != nil || last == nil {
			return fmt.Errorf("failed to determine base for backup: %w", err)
		}

		if last.BackupLevels != nil && int16(len(last.BackupLevels)) >= backupLevel {
			// We have a previous backup at the required level
			parentSnapshot = last.BackupLevels[backupLevel-1].Snapshot
			slog.Info("Found parent snapshot from last backup manifest", "parentSnapshot", parentSnapshot)
		} else {
			return fmt.Errorf("failed to determine base for backup, no previous backups found")
		}
	}
	// Resume from state if parent snapshot was already determined in a previous run
	if state.ParentSnapshot != "" {
		parentSnapshot = state.ParentSnapshot
	}

	if ctx.Err() != nil {
		return fmt.Errorf("backup cancelled before ZFS send: %w", ctx.Err())
	}

	// Check zfs send and split already done
	var blake3Hash string
	if state.Blake3Hash == "" {
		// Need to run zfs send and split
		slog.Info("Running zfs send and split", "targetSnapshot", targetSnapshot, "parentSnapshot", parentSnapshot)
		blake3Hash, err = zfs.SendAndSplit(ctx, targetSnapshot, parentSnapshot, outputDir)
		if err != nil {
			return fmt.Errorf("failed to run zfs send and split: %w", err)
		}
		slog.Info("Snapshot BLAKE3", "hash", blake3Hash)
	} else {
		// Skip zfs send and split, resume from existing state
		blake3Hash = state.Blake3Hash
		slog.Info("Using stored BLAKE3 hash", "hash", blake3Hash)
	}

	// Find snapshot part files (both raw and encrypted) and build unique index list
	allParts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
		return fmt.Errorf("failed to find snapshot parts: %w", err)
	}
	partIndexSet := make(map[string]bool)
	for _, part := range allParts {
		baseName := filepath.Base(part)
		baseName = strings.TrimSuffix(baseName, ".age")
		index := strings.TrimPrefix(baseName, "snapshot.part-")
		partIndexSet[index] = true
	}
	var partIndices []string
	for idx := range partIndexSet {
		partIndices = append(partIndices, idx)
	}
	sort.Strings(partIndices)
	if len(partIndices) == 0 {
		return fmt.Errorf("no snapshot parts found in %s", outputDir)
	}

	// Load encryption public key
	recipient, err := age.ParseX25519Recipient(cfg.AgePublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse age public key: %w", err)
	}

	// Update state
	if state.TaskName == "" {
		state.TaskName = taskName
		state.BackupLevel = backupLevel
		state.TargetSnapshot = targetSnapshot
		state.ParentSnapshot = parentSnapshot
		state.OutputDir = outputDir
		state.Blake3Hash = blake3Hash
		state.PartsCompleted = make(map[string]string)
		state.LastUpdated = time.Now().Unix()

		// Persist initial state to allow resuming if backup is interrupted during part processing
		if err := manifest.WriteState(statePath, state); err != nil {
			return fmt.Errorf("failed to persist initial backup state: %w", err)
		}
	}

	// Initialize remote backend
	var backend remote.Backend
	var manifestBackend remote.Backend
	if cfg.S3.Enabled {
		maxRetryAttempts := cfg.S3RetryAttempts()
		if int(backupLevel) >= len(cfg.S3.StorageClass.BackupData) {
			return fmt.Errorf("backup level %d exceeds configured storage classes (only %d defined)", backupLevel, len(cfg.S3.StorageClass.BackupData))
		}
		storageClass := cfg.S3.StorageClass.BackupData[backupLevel]
		s3Backend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region, cfg.S3.Prefix, cfg.S3.Endpoint, storageClass, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}

		backend = s3Backend
		slog.Info("S3 backend initialized", "bucket", cfg.S3.Bucket, "region", cfg.S3.Region, "prefix", cfg.S3.Prefix)
		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("AWS credentials verification failed: %w", err)
		}

		mBackend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region, cfg.S3.Prefix, cfg.S3.Endpoint, cfg.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend for manifests: %w", err)
		}

		manifestBackend = mBackend
		slog.Info("S3 backend for manifests initialized")
	}

	// Process parts
	partInfos, err := processPartsWithWorkerPool(ctx, partIndices, outputDir, state, statePath, recipient, backend, task, taskDirName, backupLevel)
	if err != nil {
		return err
	}

	// Sort part infos by index to ensure correct order in manifest
	sort.Slice(partInfos, func(i, j int) bool {
		return partInfos[i].Index < partInfos[j].Index
	})
	slog.Info("All part files processed", "count", len(partInfos))

	// Manifest management
	var manifestPath string
	if state.ManifestCreated {
		manifestPath = filepath.Join(outputDir, "task_manifest.yaml")
	} else {
		// Create and write manifest
		systemInfo, err := manifest.GetSystemInfo()
		if err != nil {
			slog.Warn("Failed to get system info", "error", err)

			systemInfo = manifest.SystemInfo{OS: "unknown", ZFSVersion: struct {
				Userland string `yaml:"userland"`
				Kernel   string `yaml:"kernel"`
			}{Userland: "unknown", Kernel: "unknown"}}
		}

		m := manifest.Backup{
			Datetime:       time.Now().Unix(),
			System:         systemInfo,
			Pool:           task.Pool,
			Dataset:        task.Dataset,
			BackupLevel:    backupLevel,
			TargetSnapshot: targetSnapshot,
			ParentSnapshot: parentSnapshot,
			AgePublicKey:   cfg.AgePublicKey,
			Blake3Hash:     blake3Hash,
			Parts:          partInfos,
			TargetS3Path:   filepath.Join(task.Pool, task.Dataset, taskDirName),
			ParentS3Path:   "",
		}
		if backupLevel > 0 {
			m.ParentS3Path = last.BackupLevels[backupLevel-1].S3Path
		}

		manifestPath = filepath.Join(outputDir, "task_manifest.yaml")
		if err := manifest.Write(manifestPath, &m); err != nil {
			return fmt.Errorf("failed to write manifest: %w", err)
		}
		slog.Info("Manifest written", "path", manifestPath)

		state.ManifestCreated = true
		state.LastUpdated = time.Now().Unix()

		if err := manifest.WriteState(statePath, state); err != nil {
			slog.Warn("Failed to save backup state", "error", err)
		}
	}

	// Upload manifest
	if manifestBackend != nil && !state.ManifestUploaded {
		manifestBlake3, err := crypto.BLAKE3File(manifestPath)
		if err != nil {
			return fmt.Errorf("failed to calculate manifest BLAKE3: %w", err)
		}

		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, taskDirName, "task_manifest.yaml")
		if err := manifestBackend.Upload(ctx, manifestPath, remotePath, manifestBlake3, -1); err != nil {
			return fmt.Errorf("failed to upload manifest: %w", err)
		}
		slog.Info("Manifest upload completed")

		state.ManifestUploaded = true
		state.LastUpdated = time.Now().Unix()

		if err := manifest.WriteState(statePath, state); err != nil {
			slog.Warn("Failed to save backup state", "error", err)
		}
	}

	// Update last successful backup manifest
	var currentLast manifest.Last
	if existing, err := manifest.ReadLast(lastPath); err == nil && existing != nil {
		currentLast = *existing
	}
	currentLast.Pool = task.Pool
	currentLast.Dataset = task.Dataset
	ref := &manifest.Ref{
		Datetime:   time.Now().Unix(),
		Snapshot:   targetSnapshot,
		Manifest:   manifestPath,
		Blake3Hash: blake3Hash,
		S3Path:     filepath.Join(task.Pool, task.Dataset, taskDirName),
	}
	if currentLast.BackupLevels == nil {
		currentLast.BackupLevels = make([]*manifest.Ref, int(backupLevel)+1)
	} else if len(currentLast.BackupLevels) <= int(backupLevel) {
		needed := int(backupLevel) + 1 - len(currentLast.BackupLevels)
		for range needed {
			currentLast.BackupLevels = append(currentLast.BackupLevels, nil)
		}
	}
	currentLast.BackupLevels[backupLevel] = ref
	if err := manifest.WriteLast(lastPath, &currentLast); err != nil {
		return fmt.Errorf("failed to write last backup manifest: %w", err)
	}
	slog.Info("Last backup manifest written", "path", lastPath)

	// Upload last backup manifest
	if manifestBackend != nil {
		lastBlake3, err := crypto.BLAKE3File(lastPath)
		if err != nil {
			return fmt.Errorf("failed to calculate BLAKE3 for last backup manifest: %w", err)
		}

		remoteLastPath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		if err := manifestBackend.Upload(ctx, lastPath, remoteLastPath, lastBlake3, -1); err != nil {
			return fmt.Errorf("failed to upload last backup manifest: %w", err)
		}
		slog.Info("Uploaded last backup manifest to remote", "remote", remoteLastPath)
	}

	if backend != nil {
		slog.Info("Cleaning up local backup files", "path", outputDir)

		if err := os.RemoveAll(outputDir); err != nil {
			slog.Warn("Failed to clean up local files", "error", err)
		}
	}

	// Cleanup state file
	if err := os.Remove(statePath); err != nil {
		slog.Warn("Failed to remove backup state file", "error", err)
	}

	slog.Info("Backup completed successfully!")
	return nil
}

func loadOrCreateState(statePath, taskName string, backupLevel int16) (*manifest.State, error) {
	if existingState, err := manifest.ReadState(statePath); err == nil && existingState != nil {
		if existingState.TaskName == taskName && existingState.BackupLevel == backupLevel {
			slog.Info("Found existing backup state, resuming", "state", existingState)

			return existingState, nil
		}

		slog.Info("Existing backup state is for different task/level, starting fresh")
	}

	return &manifest.State{}, nil
}

func processPartsWithWorkerPool(
	ctx context.Context,
	partIndices []string,
	outputDir string,
	state *manifest.State,
	statePath string,
	recipient age.Recipient,
	backend remote.Backend,
	task *config.Task,
	taskDirName string,
	backupLevel int16,
) ([]manifest.PartInfo, error) {
	numWorkers := 4 // TODO: make workers configurable
	var partInfos []manifest.PartInfo
	var wg sync.WaitGroup
	var stateMu sync.Mutex

	partInfoChan := make(chan manifest.PartInfo, len(partIndices))
	errChan := make(chan error, len(partIndices))
	taskChan := make(chan string, len(partIndices))

	for range numWorkers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for index := range taskChan {
				if ctx.Err() != nil {
					slog.Warn("Worker stopping due to context cancellation")
					errChan <- ctx.Err()

					return
				}

				stateMu.Lock()
				completedHash := state.PartsCompleted[index]
				stateMu.Unlock()

				if completedHash != "" {
					slog.Info("Skipping already completed part", "index", index)
					partInfoChan <- manifest.PartInfo{Index: index, Blake3Hash: completedHash}

					continue
				}

				rawFile := filepath.Join(outputDir, "snapshot.part-"+index)
				ageFile := rawFile + ".age"

				var blake3Hash string

				if _, err := os.Stat(ageFile); err == nil {
					slog.Info("Found existing encrypted file, skipping encryption", "ageFile", ageFile)

					var err error
					blake3Hash, err = crypto.BLAKE3File(ageFile)
					if err != nil {
						slog.Error("Failed to hash encrypted file", "ageFile", ageFile, "error", err)
						errChan <- err

						continue
					}

					os.Remove(rawFile)
				} else {
					slog.Info("Encrypting part file", "rawFile", rawFile)

					var err error
					blake3Hash, _, err = crypto.ProcessPart(rawFile, recipient)
					if err != nil {
						slog.Error("Failed to process part file", "rawFile", rawFile, "error", err)
						errChan <- err

						continue
					}
				}

				if backend != nil {
					if ctx.Err() != nil {
						slog.Warn("Worker stopping before upload due to context cancellation")
						errChan <- ctx.Err()

						return
					}

					slog.Info("Uploading part file to remote backend", "ageFile", ageFile)

					remotePath := filepath.Join("data", task.Pool, task.Dataset, taskDirName, filepath.Base(ageFile))
					if err := backend.Upload(ctx, ageFile, remotePath, blake3Hash, backupLevel); err != nil {
						slog.Error("Failed to upload part file", "ageFile", ageFile, "error", err)
						errChan <- err

						continue
					}
				}

				stateMu.Lock()
				state.PartsCompleted[index] = blake3Hash
				state.LastUpdated = time.Now().Unix()
				if err := manifest.WriteState(statePath, state); err != nil {
					slog.Warn("Failed to save backup state", "error", err)
				}
				stateMu.Unlock()

				partInfoChan <- manifest.PartInfo{Index: index, Blake3Hash: blake3Hash}
			}
		}()
	}

	for _, index := range partIndices {
		taskChan <- index
	}

	close(taskChan)

	wg.Wait()
	close(partInfoChan)
	close(errChan)

	var errs []error
	for err := range errChan {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("failed to process %d part(s): %w", len(errs), errors.Join(errs...))
	}

	for pi := range partInfoChan {
		partInfos = append(partInfos, pi)
	}

	return partInfos, nil
}
