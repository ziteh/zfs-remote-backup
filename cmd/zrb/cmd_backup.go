package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"filippo.io/age"
)

func runBackup(ctx context.Context, configPath string, backupLevel int16, taskName string) error {
	if backupLevel < 0 {
		return fmt.Errorf("backup level must be non-negative")
	}

	if taskName == "" {
		return fmt.Errorf("task name must be specified")
	}

	if ctx.Err() != nil {
		return fmt.Errorf("backup cancelled before start: %w", ctx.Err())
	}

	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	task, err := findTask(config, taskName)
	if err != nil {
		return err
	}

	if !task.Enabled {
		return fmt.Errorf("backup task is disabled: %s", taskName)
	}

	if err := os.MkdirAll(config.BaseDir, 0o755); err != nil {
		return fmt.Errorf("failed to create base directory: %w", err)
	}

	logPath := filepath.Join(buildLogDir(config.BaseDir, task.Pool, task.Dataset), fmt.Sprintf("%s.log", time.Now().Format("2006-01-02")))

	logger, logFile, err := setupLogging(logPath)
	if err != nil {
		return fmt.Errorf("failed to setup logging: %w", err)
	}

	defer logFile.Close()
	slog.SetDefault(logger)
	slog.Info("Backup started", "level", backupLevel, "pool", task.Pool, "dataset", task.Dataset)

	runDir := buildRunDir(config.BaseDir, task.Pool, task.Dataset)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	statePath := filepath.Join(runDir, "backup_state.yaml")

	state, err := loadOrCreateBackupState(statePath, taskName, backupLevel)
	if err != nil {
		return fmt.Errorf("failed to load backup state: %w", err)
	}

	lockPath := filepath.Join(runDir, "zrb.lock")

	releaseLock, err := AcquireLock(lockPath, task.Pool, task.Dataset)
	if err != nil {
		return fmt.Errorf("failed to acquire lock: %w", err)
	}

	defer func() {
		if err := releaseLock(); err != nil {
			slog.Warn("Failed to release lock", "error", err)
		}
	}()

	snapshots, err := listSnapshots(task.Pool, task.Dataset, "zrb_level"+fmt.Sprint(backupLevel))
	if err != nil {
		return fmt.Errorf("failed to list snapshots: %w", err)
	}

	if len(snapshots) == 0 {
		return fmt.Errorf("no snapshots found for pool=%s dataset=%s", task.Pool, task.Dataset)
	}

	latestSnapshot := snapshots[0]

	targetSnapshot := latestSnapshot
	if state.TargetSnapshot != "" {
		targetSnapshot = state.TargetSnapshot
	}

	slog.Info("Latest snapshot found", "latestSnapshot", latestSnapshot, "count", len(snapshots))

	taskDirName := buildTaskDirName(backupLevel, time.Now())

	if state.OutputDir != "" {
		outputDirParent := filepath.Dir(state.OutputDir)
		levelDir := filepath.Base(outputDirParent)
		dateDir := filepath.Base(state.OutputDir)
		taskDirName = filepath.Join(levelDir, dateDir)
	}

	outputDir := filepath.Join(config.BaseDir, "task", task.Pool, task.Dataset, taskDirName)

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

	lastPath := filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")

	var parentSnapshot string

	var last *LastBackup
	if backupLevel > 0 {
		last, err = readLastBackupManifest(lastPath)
		if err != nil || last == nil {
			return fmt.Errorf("failed to determine base for backup: %w", err)
		}

		if last.BackupLevels != nil && int16(len(last.BackupLevels)) >= backupLevel {
			parentSnapshot = last.BackupLevels[backupLevel-1].Snapshot
			slog.Info("Found parent snapshot from last backup manifest", "parentSnapshot", parentSnapshot)
		} else {
			return fmt.Errorf("failed to determine base for backup, no previous backups found")
		}
	}

	if state.ParentSnapshot != "" {
		parentSnapshot = state.ParentSnapshot
	}

	if ctx.Err() != nil {
		return fmt.Errorf("backup cancelled before ZFS send: %w", ctx.Err())
	}

	var blake3Hash string

	if state.Blake3Hash == "" {
		slog.Info("Running zfs send and split", "targetSnapshot", targetSnapshot, "parentSnapshot", parentSnapshot)

		blake3Hash, err = runZfsSendAndSplit(targetSnapshot, parentSnapshot, outputDir)
		if err != nil {
			return fmt.Errorf("failed to run zfs send and split: %w", err)
		}

		slog.Info("Snapshot BLAKE3", "hash", blake3Hash)
	} else {
		blake3Hash = state.Blake3Hash
		slog.Info("Using stored BLAKE3 hash", "hash", blake3Hash)
	}

	parts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
		return fmt.Errorf("failed to find snapshot parts: %w", err)
	}

	var rawParts []string

	for _, part := range parts {
		if !strings.HasSuffix(part, ".age") {
			rawParts = append(rawParts, part)
		}
	}

	sort.Strings(rawParts)

	recipient, err := age.ParseX25519Recipient(config.AgePublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse age public key: %w", err)
	}

	if state.TaskName == "" {
		state.TaskName = taskName
		state.BackupLevel = backupLevel
		state.TargetSnapshot = targetSnapshot
		state.ParentSnapshot = parentSnapshot
		state.OutputDir = outputDir
		state.Blake3Hash = blake3Hash
		state.PartsProcessed = make(map[string]bool)
		state.PartsUploaded = make(map[string]bool)
		state.LastUpdated = time.Now().Unix()
	}

	var backend RemoteBackend

	var manifestBackend RemoteBackend

	if config.S3.Enabled {
		maxRetryAttempts := getS3RetryConfig(config)
		storageClass := config.S3.StorageClass.BackupData[backupLevel]

		s3Backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, storageClass, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}

		backend = s3Backend

		slog.Info("S3 backend initialized", "bucket", config.S3.Bucket, "region", config.S3.Region, "prefix", config.S3.Prefix)

		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("AWS credentials verification failed: %w", err)
		}

		manifestBackend, err = NewS3Backend(ctx, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, config.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend for manifests: %w", err)
		}

		slog.Info("S3 backend for manifests initialized")
	}

	partInfos, err := processPartsWithWorkerPool(ctx, rawParts, state, statePath, recipient, backend, task, taskDirName, backupLevel)
	if err != nil {
		return err
	}

	slog.Info("All part files processed", "count", len(partInfos))

	var manifestPath string

	if !state.ManifestCreated {
		systemInfo, err := getSystemInfo()
		if err != nil {
			slog.Warn("Failed to get system info", "error", err)

			systemInfo = SystemInfo{OS: "unknown", ZFSVersion: struct {
				Userland string `yaml:"userland"`
				Kernel   string `yaml:"kernel"`
			}{Userland: "unknown", Kernel: "unknown"}}
		}

		manifest := BackupManifest{
			Datetime:       time.Now().Unix(),
			System:         systemInfo,
			Pool:           task.Pool,
			Dataset:        task.Dataset,
			BackupLevel:    backupLevel,
			TargetSnapshot: targetSnapshot,
			ParentSnapshot: parentSnapshot,
			AgePublicKey:   config.AgePublicKey,
			Blake3Hash:     blake3Hash,
			Parts:          partInfos,
			TargetS3Path:   filepath.Join(task.Pool, task.Dataset, taskDirName),
			ParentS3Path:   "",
		}
		if backupLevel > 0 {
			manifest.ParentS3Path = last.BackupLevels[backupLevel-1].S3Path
		}

		manifestPath = filepath.Join(outputDir, "task_manifest.yaml")
		if err := writeManifest(manifestPath, &manifest); err != nil {
			return fmt.Errorf("failed to write manifest: %w", err)
		}

		slog.Info("Manifest written", "path", manifestPath)

		state.ManifestCreated = true
		state.LastUpdated = time.Now().Unix()

		if err := writeBackupState(statePath, state); err != nil {
			slog.Warn("Failed to save backup state", "error", err)
		}
	} else {
		manifestPath = filepath.Join(outputDir, "task_manifest.yaml")
	}

	if manifestBackend != nil && !state.ManifestUploaded {
		manifestBlake3, err := calculateBLAKE3(manifestPath)
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

		if err := writeBackupState(statePath, state); err != nil {
			slog.Warn("Failed to save backup state", "error", err)
		}
	}

	var currentLast LastBackup
	if existing, err := readLastBackupManifest(lastPath); err == nil && existing != nil {
		currentLast = *existing
	}

	currentLast.Pool = task.Pool
	currentLast.Dataset = task.Dataset
	ref := &BackupRef{
		Datetime:   time.Now().Unix(),
		Snapshot:   latestSnapshot,
		Manifest:   manifestPath,
		Blake3Hash: blake3Hash,
		S3Path:     filepath.Join(task.Pool, task.Dataset, taskDirName),
	}

	if currentLast.BackupLevels == nil {
		currentLast.BackupLevels = make([]*BackupRef, int(backupLevel)+1)
	} else if len(currentLast.BackupLevels) <= int(backupLevel) {
		needed := int(backupLevel) + 1 - len(currentLast.BackupLevels)
		for range needed {
			currentLast.BackupLevels = append(currentLast.BackupLevels, nil)
		}
	}

	currentLast.BackupLevels[backupLevel] = ref
	if err := writeLastBackupManifest(lastPath, &currentLast); err != nil {
		slog.Warn("Failed to write last backup manifest", "error", err)
	} else {
		slog.Info("Last backup manifest written", "path", lastPath)
	}

	if manifestBackend != nil {
		if lastBlake3, err := calculateBLAKE3(lastPath); err == nil {
			remoteLastPath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
			if err := manifestBackend.Upload(ctx, lastPath, remoteLastPath, lastBlake3, -1); err != nil {
				slog.Warn("Failed to upload last backup manifest", "error", err)
			} else {
				slog.Info("Uploaded last backup manifest to remote", "remote", remoteLastPath)
			}
		} else {
			slog.Warn("Failed to calculate BLAKE3 for last backup manifest", "error", err)
		}
	}

	if backend != nil {
		slog.Info("Cleaning up local backup files", "path", outputDir)

		if err := os.RemoveAll(outputDir); err != nil {
			slog.Warn("Failed to clean up local files", "error", err)
		}
	}

	if err := os.Remove(statePath); err != nil {
		slog.Warn("Failed to remove backup state file", "error", err)
	}

	slog.Info("Backup completed successfully!")

	return nil
}

func loadOrCreateBackupState(statePath, taskName string, backupLevel int16) (*BackupState, error) {
	if existingState, err := readBackupState(statePath); err == nil && existingState != nil {
		if existingState.TaskName == taskName && existingState.BackupLevel == backupLevel {
			slog.Info("Found existing backup state, resuming", "state", existingState)

			return existingState, nil
		}

		slog.Info("Existing backup state is for different task/level, starting fresh")
	}

	return &BackupState{}, nil
}

func processPartsWithWorkerPool(ctx context.Context, rawParts []string, state *BackupState, statePath string, recipient age.Recipient, backend RemoteBackend, task *BackupTask, taskDirName string, backupLevel int16) ([]PartInfo, error) {
	var partInfos []PartInfo

	numWorkers := 4

	var wg sync.WaitGroup

	partInfoChan := make(chan PartInfo, len(rawParts))
	errChan := make(chan error, len(rawParts))
	taskChan := make(chan string, len(rawParts))

	for range numWorkers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for partFile := range taskChan {
				if ctx.Err() != nil {
					slog.Warn("Worker stopping due to context cancellation")
					errChan <- ctx.Err()

					return
				}

				baseName := filepath.Base(partFile)
				index := strings.TrimPrefix(baseName, "snapshot.part-")

				if state.PartsProcessed[index] {
					slog.Info("Skipping already processed part", "part", partFile)

					continue
				}

				slog.Info("Encryption and upload started for part file", "partFile", partFile)

				blake3Hash, encryptedFile, err := processPartFile(partFile, recipient)
				if err != nil {
					slog.Error("Failed to process part file", "partFile", partFile, "error", err)
					errChan <- err

					continue
				}

				slog.Debug("Part file encrypted", "partFile", partFile, "encryptedFile", encryptedFile, "blake3", blake3Hash)

				partInfo := PartInfo{
					Index:      index,
					Blake3Hash: blake3Hash,
				}
				partInfoChan <- partInfo

				state.PartsProcessed[index] = true
				state.LastUpdated = time.Now().Unix()

				if err := writeBackupState(statePath, state); err != nil {
					slog.Warn("Failed to save backup state", "error", err)
				}

				if backend != nil {
					if ctx.Err() != nil {
						slog.Warn("Worker stopping before upload due to context cancellation")
						errChan <- ctx.Err()

						return
					}

					if !state.PartsUploaded[index] {
						slog.Info("Uploading part file to remote backend", "encryptedFile", encryptedFile)

						remotePath := filepath.Join("data", task.Pool, task.Dataset, taskDirName, filepath.Base(encryptedFile))
						if err := backend.Upload(ctx, encryptedFile, remotePath, blake3Hash, backupLevel); err != nil {
							slog.Error("Failed to upload part file", "encryptedFile", encryptedFile, "error", err)
							errChan <- err

							continue
						}

						state.PartsUploaded[index] = true
						state.LastUpdated = time.Now().Unix()

						if err := writeBackupState(statePath, state); err != nil {
							slog.Warn("Failed to save backup state", "error", err)
						}
					} else {
						slog.Info("Skipping already uploaded part", "encryptedFile", encryptedFile)
					}
				}
			}
		}()
	}

	for _, partFile := range rawParts {
		taskChan <- partFile
	}

	close(taskChan)

	wg.Wait()
	close(partInfoChan)
	close(errChan)

	if len(errChan) > 0 {
		err := <-errChan

		return nil, fmt.Errorf("failed to process part files: %w", err)
	}

	for pi := range partInfoChan {
		partInfos = append(partInfos, pi)
	}

	return partInfos, nil
}
