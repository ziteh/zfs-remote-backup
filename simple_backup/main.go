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
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "zrb_simple",
		Usage:   "ZFS Remote Backup",
		Version: "0.1.0-alpha.1",
		Commands: []*cli.Command{
			{
				Name:  "genkey",
				Usage: "Generate public and private key pair",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return generateKey(ctx)
				},
			},
			{
				Name:  "backup",
				Usage: "Run backup task",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "path to configuration yaml file",
						Value: "zrb_simple_config.yaml",
					},
					&cli.StringFlag{
						Name:     "task",
						Usage:    "Name of the backup task to run.",
						Required: true,
					},
					&cli.Int16Flag{
						Name:     "level",
						Usage:    "Backup level to perform.",
						Required: true,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					configPath := cmd.String("config")
					taskName := cmd.String("task")
					backupLevel := cmd.Int16("level")
					return runBackup(ctx, configPath, backupLevel, taskName)
				},
			},
			{
				Name:  "snapshot",
				Usage: "Create a ZFS snapshot for the specified pool and dataset",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "pool",
						Usage:    "ZFS pool name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "dataset",
						Usage:    "ZFS dataset name",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "prefix",
						Usage: "Snapshot name prefix",
						Value: "zrb_level0",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					pool := cmd.String("pool")
					dataset := cmd.String("dataset")
					prefix := cmd.String("prefix")
					return createSnapshot(pool, dataset, prefix)
				},
			},
		},
	}

	if err := cmd.Run(context.Background(), os.Args); err != nil {
		slog.Error("CLI error", "error", err)
		os.Exit(1)
	}
}

func generateKey(_ context.Context) error {
	fmt.Println("Generating age public and private key pair...")

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	publicKey := identity.Recipient().String()
	privateKey := identity.String()

	// TODO: Securely store this key
	fmt.Println("\n=== Age Key Pair Generated ===")
	fmt.Printf("Public key:  %s\n", publicKey)
	fmt.Printf("Private key: %s\n", privateKey)
	fmt.Println("\n!! Keep your private key secure !!")

	return nil
}

func runBackup(_ context.Context, configPath string, backupLevel int16, taskName string) error {
	if backupLevel < 0 {
		return fmt.Errorf("backup level must be non-negative")
	}
	if taskName == "" {
		return fmt.Errorf("task name must be specified")
	}

	config, err := loadConfig(configPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// find the specified task
	var task *BackupTask = nil
	for _, t := range config.Tasks {
		slog.Info("Checking task", "taskName", t.Name)
		if t.Name == taskName {
			task = &t
			break
		}
	}
	if task == nil {
		slog.Error("Backup task not found", "taskName", taskName)
		os.Exit(1)
	}
	if !task.Enabled {
		slog.Error("Backup task is disabled", "taskName", taskName)
		os.Exit(1)
	}

	if err := os.MkdirAll(config.BaseDir, 0755); err != nil {
		slog.Error("Failed to create export directory", "error", err)
		os.Exit(1)
	}

	// Set up logging
	logDir := filepath.Join(config.BaseDir, "logs", task.Pool, task.Dataset)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		slog.Error("Failed to create log directory", "error", err)
		os.Exit(1)
	}
	logPath := filepath.Join(logDir, fmt.Sprintf("%s.log", time.Now().Format("2006-01-02")))

	logger, logFile := NewLogger(logPath)
	defer logFile.Close()
	slog.SetDefault(logger)
	slog.Info("Backup started", "level", backupLevel, "pool", task.Pool, "dataset", task.Dataset)

	// Prepare run directory
	runDir := filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		slog.Error("Failed to create run directory", "error", err)
		os.Exit(1)
	}

	// Check for existing backup state
	statePath := filepath.Join(runDir, "backup_state.yaml")
	var state *BackupState
	if existingState, err := readBackupState(statePath); err == nil && existingState != nil {
		if existingState.TaskName == taskName && existingState.BackupLevel == backupLevel {
			slog.Info("Found existing backup state, resuming", "state", existingState)
			state = existingState
		} else {
			slog.Info("Existing backup state is for different task/level, starting fresh")
		}
	}

	// Acquire lock for this pool/dataset
	lockPath := filepath.Join(runDir, "zrb.lock")
	releaseLock, err := AcquireLock(lockPath, task.Pool, task.Dataset)
	if err != nil {
		slog.Error("Failed to acquire lock", "error", err)
		os.Exit(1)
	}
	defer func() {
		if err := releaseLock(); err != nil {
			slog.Warn("Failed to release lock", "error", err)
		}
	}()

	// Find latest snapshot
	snapshots, err := listSnapshots(task.Pool, task.Dataset, "zrb_level"+fmt.Sprint(backupLevel))
	if err != nil {
		slog.Error("Failed to list snapshots", "error", err)
		os.Exit(1)
	}
	if len(snapshots) == 0 {
		slog.Error("No snapshots found", "pool", task.Pool, "dataset", task.Dataset)
		os.Exit(1)
	}
	latestSnapshot := snapshots[0]
	targetSnapshot := latestSnapshot // TODO: refactor
	slog.Info("Latest snapshot found", "latestSnapshot", latestSnapshot, "count", len(snapshots))

	// If resuming, use the stored snapshot
	if state != nil {
		targetSnapshot = state.TargetSnapshot
	}

	// Prepare output directory
	taskDirName := filepath.Join(fmt.Sprintf("level%d", backupLevel), time.Now().Format("20060102"))
	if state != nil {
		outputDirParent := filepath.Dir(state.OutputDir)
		levelDir := filepath.Base(outputDirParent)
		dateDir := filepath.Base(state.OutputDir)
		taskDirName = filepath.Join(levelDir, dateDir)
	}
	outputDir := filepath.Join(config.BaseDir, "task", task.Pool, task.Dataset, taskDirName)
	// Clean up output directory if it exists and not resuming
	if state == nil {
		if _, err := os.Stat(outputDir); err == nil {
			slog.Info("Cleaning up existing output directory", "path", outputDir)
			if err := os.RemoveAll(outputDir); err != nil {
				slog.Error("Failed to remove existing output directory", "error", err)
				os.Exit(1)
			}
		}
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		slog.Error("Failed to create export directory", "error", err)
		os.Exit(1)
	}

	// Determine parent snapshot for diff/incr backups
	lastPath := filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
	var parentSnapshot string = ""
	var last *LastBackup
	if backupLevel > 0 {
		// find parent snapshot from last backup manifest
		var err error
		last, err = readLastBackupManifest(lastPath)
		if err != nil || last == nil {
			slog.Error("Failed to determine base for backup", "error", err)
			os.Exit(1)
		}

		// Find upper level backup snapshot
		if last.BackupLevels != nil && int16(len(last.BackupLevels)) >= backupLevel {
			parentSnapshot = last.BackupLevels[backupLevel-1].Snapshot
			slog.Info("Found parent snapshot from last backup manifest", "parentSnapshot", parentSnapshot)
		} else {
			slog.Error("Failed to determine base for backup, no previous backups found")
			os.Exit(1)
		}
	}

	// If resuming, use stored parent
	if state != nil {
		parentSnapshot = state.ParentSnapshot
	}

	var blake3Hash string
	if state == nil || state.Blake3Hash == "" {
		// Run zfs send and split
		slog.Info("Running zfs send and split", "targetSnapshot", targetSnapshot, "parentSnapshot", parentSnapshot)
		blake3Hash, err = runZfsSendAndSplit(targetSnapshot, parentSnapshot, outputDir)
		if err != nil {
			slog.Error("Failed to run zfs send and split", "error", err)
			os.Exit(1)
		}
		slog.Info("Snapshot BLAKE3", "hash", blake3Hash)
	} else {
		blake3Hash = state.Blake3Hash
		slog.Info("Using stored BLAKE3 hash", "hash", blake3Hash)
	}

	// Process snapshot parts
	parts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
		slog.Error("Failed to find snapshot parts", "error", err)
		os.Exit(1)
	}
	var rawParts []string
	for _, part := range parts {
		if !strings.HasSuffix(part, ".age") {
			rawParts = append(rawParts, part)
		}
	}
	sort.Strings(rawParts)

	// Encryption setup
	recipient, err := age.ParseX25519Recipient(config.AgePublicKey)
	if err != nil {
		slog.Error("Failed to parse age public key", "error", err)
		os.Exit(1)
	}

	// Initialize state if not resuming
	if state == nil {
		state = &BackupState{
			TaskName:       taskName,
			BackupLevel:    backupLevel,
			TargetSnapshot: targetSnapshot,
			ParentSnapshot: parentSnapshot,
			OutputDir:      outputDir,
			Blake3Hash:     blake3Hash,
			PartsProcessed: make(map[string]bool),
			PartsUploaded:  make(map[string]bool),
			LastUpdated:    time.Now().Unix(),
		}
	}

	// Initialize S3 backend if enabled
	var backend RemoteBackend = nil
	var manifestBackend RemoteBackend = nil
	ctxBg := context.Background()
	if config.S3.Enabled {
		// Get retry configuration (default to 3 if not specified)
		maxRetryAttempts := 3
		if config.S3.Retry.MaxAttempts > 0 {
			maxRetryAttempts = config.S3.Retry.MaxAttempts
		}

		storageClass := config.S3.StorageClass.BackupData[backupLevel]
		s3Backend, err := NewS3Backend(ctxBg, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, storageClass, maxRetryAttempts)
		if err != nil {
			slog.Error("Failed to initialize S3 backend", "error", err)
			os.Exit(1)
		}
		backend = s3Backend
		slog.Info("S3 backend initialized", "bucket", config.S3.Bucket, "region", config.S3.Region, "prefix", config.S3.Prefix)

		manifestBackend, err = NewS3Backend(ctxBg, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, config.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			slog.Error("Failed to initialize S3 backend for manifests", "error", err)
			os.Exit(1)
		}
		slog.Info("S3 backend for manifests initialized", "bucket", config.S3.Bucket, "region", config.S3.Region, "prefix", config.S3.Prefix)
	}

	// Encrypt and upload parts concurrently with worker pool
	var partInfos []PartInfo
	numWorkers := 4 // TODO: make configurable
	var wg sync.WaitGroup
	partInfoChan := make(chan PartInfo, len(rawParts))
	errChan := make(chan error, len(rawParts))
	taskChan := make(chan string, len(rawParts))

	// Start workers
	for range numWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for partFile := range taskChan {
				baseName := filepath.Base(partFile)
				index := strings.TrimPrefix(baseName, "snapshot.part-")

				// Skip if already processed
				if state.PartsProcessed[index] {
					slog.Info("Skipping already processed part", "part", partFile)
					continue
				}

				slog.Info("Encryption and upload started for part file", "partFile", partFile)
				sha256Hash, encryptedFile, err := processPartFile(partFile, recipient)
				if err != nil {
					slog.Error("Failed to process part file", "partFile", partFile, "error", err)
					errChan <- err
					continue
				}
				slog.Debug("Part file encrypted", "partFile", partFile, "encryptedFile", encryptedFile, "sha256", sha256Hash)

				partInfo := PartInfo{
					Index:      index,
					SHA256Hash: sha256Hash,
				}
				partInfoChan <- partInfo

				// Mark as processed
				state.PartsProcessed[index] = true
				state.LastUpdated = time.Now().Unix()
				if err := writeBackupState(statePath, state); err != nil {
					slog.Warn("Failed to save backup state", "error", err)
				}

				// Upload to remote backend if configured
				if backend != nil {
					// Skip if already uploaded
					if !state.PartsUploaded[index] {
						slog.Info("Uploading part file to remote backend", "encryptedFile", encryptedFile)
						remotePath := filepath.Join("data", task.Pool, task.Dataset, taskDirName, filepath.Base(encryptedFile))
						if err := backend.Upload(ctxBg, encryptedFile, remotePath, sha256Hash, backupLevel); err != nil {
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

	// Send tasks to workers
	for _, partFile := range rawParts {
		taskChan <- partFile
	}
	close(taskChan)

	wg.Wait()
	close(partInfoChan)
	close(errChan)

	// Check for errors
	if len(errChan) > 0 {
		err := <-errChan
		slog.Error("Error during processing", "error", err)
		return fmt.Errorf("failed to process part files: %w", err)
	}

	// Collect partInfos
	for pi := range partInfoChan {
		partInfos = append(partInfos, pi)
	}
	slog.Info("All part files processed", "count", len(partInfos))

	// Create and write manifest
	var manifestPath string
	if !state.ManifestCreated {
		systemInfo, err := getSystemInfo()
		if err != nil {
			slog.Warn("Failed to get system info", "error", err)
			systemInfo = SystemInfo{}
			systemInfo.OS = "unknown"
			systemInfo.ZFSVersion.Userland = "unknown"
			systemInfo.ZFSVersion.Kernel = "unknown"
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
			slog.Error("Failed to write manifest", "error", err)
			os.Exit(1)
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

	// Upload manifest to S3
	if manifestBackend != nil && !state.ManifestUploaded {
		manifestSHA256, err := calculateSHA256(manifestPath)
		if err != nil {
			slog.Error("Failed to calculate manifest SHA256", "error", err)
			os.Exit(1)
		}
		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, taskDirName, "task_manifest.yaml")
		if err := manifestBackend.Upload(ctxBg, manifestPath, remotePath, manifestSHA256, -1); err != nil {
			slog.Error("Failed to upload manifest", "error", err)
			os.Exit(1)
		}
		slog.Info("Manifest upload completed")
		state.ManifestUploaded = true
		state.LastUpdated = time.Now().Unix()
		if err := writeBackupState(statePath, state); err != nil {
			slog.Warn("Failed to save backup state", "error", err)
		}
	}

	// Update last backup manifest
	lastPath = filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
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
	// Ensure BackupLevels slice is initialized and large enough
	if currentLast.BackupLevels == nil {
		currentLast.BackupLevels = make([]*BackupRef, int(backupLevel)+1)
	} else if len(currentLast.BackupLevels) <= int(backupLevel) {
		// extend slice to required length
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

	// Upload last backup manifest to S3
	if manifestBackend != nil {
		if lastSHA, err := calculateSHA256(lastPath); err == nil {
			remoteLastPath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
			if err := manifestBackend.Upload(ctxBg, lastPath, remoteLastPath, lastSHA, -1); err != nil {
				slog.Warn("Failed to upload last backup manifest", "error", err)
			} else {
				slog.Info("Uploaded last backup manifest to remote", "remote", remoteLastPath)
			}
		} else {
			slog.Warn("Failed to calculate SHA256 for last backup manifest", "error", err)
		}
	}

	// Clean up local files
	if backend != nil {
		slog.Info("Cleaning up local backup files", "path", outputDir)
		if err := os.RemoveAll(outputDir); err != nil {
			slog.Warn("Failed to clean up local files", "error", err)
		}
	}

	// Clean up state file
	if err := os.Remove(statePath); err != nil {
		slog.Warn("Failed to remove backup state file", "error", err)
	}

	slog.Info("Backup completed successfully!")
	return nil
}
