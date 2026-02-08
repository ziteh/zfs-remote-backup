package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"filippo.io/age"
	"github.com/urfave/cli/v3"
	"gopkg.in/yaml.v3"
)

func main() {
	cmd := &cli.Command{
		Name:    "zrb",
		Usage:   "ZFS Remote Backup",
		Version: "0.1.0",
		Commands: []*cli.Command{
			{
				Name:  "genkey",
				Usage: "Generate public and private key pair",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return generateKey(ctx)
				},
			},
			{
				Name:  "test-keys",
				Usage: "Test if public and private key pair match",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "path to configuration yaml file",
						Value: "zrb_config.yaml",
					},
					&cli.StringFlag{
						Name:     "private-key",
						Usage:    "Path to age private key file",
						Required: true,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return testKeys(ctx, cmd.String("config"), cmd.String("private-key"))
				},
			},
			{
				Name:  "backup",
				Usage: "Run backup task",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "path to configuration yaml file",
						Value: "zrb_config.yaml",
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
			{
				Name:  "list",
				Usage: "List available backups",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "path to configuration yaml file",
						Value: "zrb_config.yaml",
					},
					&cli.StringFlag{
						Name:     "task",
						Usage:    "Name of the backup task",
						Required: true,
					},
					&cli.Int16Flag{
						Name:  "level",
						Usage: "Filter by backup level (-1 for all levels)",
						Value: -1,
					},
					&cli.StringFlag{
						Name:  "source",
						Usage: "Data source: local or s3",
						Value: "local",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					configPath := cmd.String("config")
					taskName := cmd.String("task")
					filterLevel := cmd.Int16("level")
					source := cmd.String("source")
					return listBackups(ctx, configPath, taskName, filterLevel, source)
				},
			},
			{
				Name:  "restore",
				Usage: "Restore backup from S3 or local",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "config",
						Usage: "path to configuration yaml file",
						Value: "zrb_config.yaml",
					},
					&cli.StringFlag{
						Name:     "task",
						Usage:    "Name of the backup task",
						Required: true,
					},
					&cli.Int16Flag{
						Name:     "level",
						Usage:    "Backup level to restore",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "target",
						Usage:    "Target pool/dataset (e.g., newpool/restored_data)",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "private-key",
						Usage:    "Path to age private key file",
						Required: true,
					},
					&cli.StringFlag{
						Name:  "source",
						Usage: "Data source: local or s3",
						Value: "s3",
					},
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "Show what would be restored without actually restoring",
						Value: false,
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return restoreBackup(ctx, cmd.String("config"), cmd.String("task"),
						cmd.Int16("level"), cmd.String("target"), cmd.String("private-key"),
						cmd.String("source"), cmd.Bool("dry-run"))
				},
			},
		},
	}

	// Set up signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Run the CLI with cancellable context
	if err := cmd.Run(ctx, os.Args); err != nil {
		// Check if error is due to context cancellation (user interrupt)
		if ctx.Err() == context.Canceled {
			fmt.Fprintln(os.Stderr, "\n⚠ Backup interrupted by user")
			os.Exit(130) // Standard exit code for SIGINT
		}
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

func testKeys(_ context.Context, configPath, privateKeyPath string) error {
	fmt.Println("Testing age key pair compatibility...")

	// Load config to get public key
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Parse public key from config
	recipient, err := age.ParseX25519Recipient(config.AgePublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key from config: %w", err)
	}
	fmt.Printf("Public key from config: %s\n", config.AgePublicKey)

	// Load private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(privateKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	fmt.Printf("Private key loaded from: %s\n", privateKeyPath)

	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "zrb_key_test_*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file with known content
	testContent := "ZFS Remote Backup - Key Pair Test - " + time.Now().Format(time.RFC3339)
	testFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(testFile, []byte(testContent), 0644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}

	// Encrypt with public key
	encryptedFile := filepath.Join(tempDir, "test.txt.age")
	fmt.Println("\nEncrypting test data with public key...")
	if err := encryptWithAge(testFile, encryptedFile, recipient); err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}
	fmt.Println("Encryption successful")

	// Decrypt with private key
	decryptedFile := filepath.Join(tempDir, "test_decrypted.txt")
	fmt.Println("Decrypting test data with private key...")
	if err := decryptWithAge(encryptedFile, decryptedFile, identity); err != nil {
		return fmt.Errorf("decryption failed: %w\nThis means the private key does not match the public key in config", err)
	}
	fmt.Println("Decryption successful")

	// Verify content matches
	decryptedContent, err := os.ReadFile(decryptedFile)
	if err != nil {
		return fmt.Errorf("failed to read decrypted file: %w", err)
	}

	if string(decryptedContent) != testContent {
		return fmt.Errorf("content mismatch: decrypted content does not match original")
	}

	fmt.Println("Content verification successful")
	return nil
}

func runBackup(ctx context.Context, configPath string, backupLevel int16, taskName string) error {
	if backupLevel < 0 {
		return fmt.Errorf("backup level must be non-negative")
	}
	if taskName == "" {
		return fmt.Errorf("task name must be specified")
	}

	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return fmt.Errorf("backup cancelled before start: %w", ctx.Err())
	default:
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

	// Check context before starting ZFS send
	select {
	case <-ctx.Done():
		slog.Warn("Backup cancelled before ZFS send")
		return fmt.Errorf("backup cancelled: %w", ctx.Err())
	default:
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
	if config.S3.Enabled {
		// Get retry configuration (default to 3 if not specified)
		maxRetryAttempts := 3
		if config.S3.Retry.MaxAttempts > 0 {
			maxRetryAttempts = config.S3.Retry.MaxAttempts
		}

		storageClass := config.S3.StorageClass.BackupData[backupLevel]
		s3Backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, storageClass, maxRetryAttempts)
		if err != nil {
			slog.Error("Failed to initialize S3 backend", "error", err)
			os.Exit(1)
		}
		backend = s3Backend
		slog.Info("S3 backend initialized", "bucket", config.S3.Bucket, "region", config.S3.Region, "prefix", config.S3.Prefix)

		// Verify AWS credentials before proceeding
		if err := backend.VerifyCredentials(ctx); err != nil {
			slog.Error("AWS credentials verification failed", "error", err)
			os.Exit(1)
		}

		manifestBackend, err = NewS3Backend(ctx, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, config.S3.StorageClass.Manifest, maxRetryAttempts)
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
				// Check if context is cancelled
				select {
				case <-ctx.Done():
					slog.Warn("Worker stopping due to context cancellation")
					errChan <- ctx.Err()
					return
				default:
				}

				baseName := filepath.Base(partFile)
				index := strings.TrimPrefix(baseName, "snapshot.part-")

				// Skip if already processed
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

				// Mark as processed
				state.PartsProcessed[index] = true
				state.LastUpdated = time.Now().Unix()
				if err := writeBackupState(statePath, state); err != nil {
					slog.Warn("Failed to save backup state", "error", err)
				}

				// Upload to remote backend if configured
				if backend != nil {
					// Check context again before upload
					select {
					case <-ctx.Done():
						slog.Warn("Worker stopping before upload due to context cancellation")
						errChan <- ctx.Err()
						return
					default:
					}

					// Skip if already uploaded
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
		manifestBlake3, err := calculateBLAKE3(manifestPath)
		if err != nil {
			slog.Error("Failed to calculate manifest BLAKE3", "error", err)
			os.Exit(1)
		}
		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, taskDirName, "task_manifest.yaml")
		if err := manifestBackend.Upload(ctx, manifestPath, remotePath, manifestBlake3, -1); err != nil {
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

// BackupInfo represents backup information for JSON output
type BackupInfo struct {
	Level           int16  `json:"level"`
	Type            string `json:"type"`
	Datetime        int64  `json:"datetime"`
	DatetimeStr     string `json:"datetime_str"`
	Snapshot        string `json:"snapshot"`
	ParentSnapshot  string `json:"parent_snapshot,omitempty"`
	ParentS3Path    string `json:"parent_s3_path,omitempty"`
	Blake3Hash      string `json:"blake3_hash"`
	PartsCount      int    `json:"parts_count"`
	EstimatedSizeGB int    `json:"estimated_size_gb"`
	S3Path          string `json:"s3_path"`
	ManifestPath    string `json:"manifest_path,omitempty"`
}

// BackupListOutput represents the complete JSON output
type BackupListOutput struct {
	Task    string       `json:"task"`
	Pool    string       `json:"pool"`
	Dataset string       `json:"dataset"`
	Source  string       `json:"source"`
	Backups []BackupInfo `json:"backups"`
	Summary struct {
		TotalBackups         int `json:"total_backups"`
		FullBackups          int `json:"full_backups"`
		IncrementalBackups   int `json:"incremental_backups"`
		TotalEstimatedSizeGB int `json:"total_estimated_size_gb"`
	} `json:"summary"`
}

func listBackups(ctx context.Context, configPath, taskName string, filterLevel int16, source string) error {
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find the specified task
	var task *BackupTask
	for _, t := range config.Tasks {
		if t.Name == taskName {
			task = &t
			break
		}
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskName)
	}

	// Read last backup manifest based on source
	var lastBackup *LastBackup
	var lastPath string

	if source == "s3" {
		// Read from S3
		if !config.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		// Check if manifest storage class not immediately accessible
		manifestStorageClass := string(config.S3.StorageClass.Manifest)
		if manifestStorageClass == "GLACIER" || manifestStorageClass == "DEEP_ARCHIVE" {
			return fmt.Errorf("cannot list from S3: manifest storage class is %s (not immediately accessible)\n", manifestStorageClass)
		}

		maxRetryAttempts := 3
		if config.S3.Retry.MaxAttempts > 0 {
			maxRetryAttempts = config.S3.Retry.MaxAttempts
		}

		backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region,
			config.S3.Prefix, config.S3.Endpoint,
			config.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}

		// Verify AWS credentials before proceeding
		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("AWS credentials verification failed: %w", err)
		}

		// Download last_backup_manifest.yaml to temp file
		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		lastPath = filepath.Join(os.TempDir(), fmt.Sprintf("last_backup_manifest_%s.yaml", taskName))

		slog.Info("Downloading manifest from S3", "remote", remotePath, "local", lastPath)
		if err := backend.Download(ctx, remotePath, lastPath); err != nil {
			return fmt.Errorf("failed to download manifest from S3: %w", err)
		}
		defer os.Remove(lastPath) // Clean up temp file
	} else {
		// Read from local
		lastPath = filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
	}

	lastBackup, err = readLastBackupManifest(lastPath)
	if err != nil {
		return fmt.Errorf("failed to read backup manifest from %s: %w", lastPath, err)
	}

	// Build output
	output := BackupListOutput{
		Task:    taskName,
		Pool:    task.Pool,
		Dataset: task.Dataset,
		Source:  source,
		Backups: []BackupInfo{},
	}

	for level, ref := range lastBackup.BackupLevels {
		if ref == nil {
			continue
		}

		if filterLevel >= 0 && int16(level) != filterLevel {
			continue
		}

		backupType := "full"
		if level > 0 {
			backupType = "incremental"
		}

		estimatedSizeGB := len(ref.Blake3Hash) // This is a placeholder
		// Try to get actual part count from manifest if available
		if ref.Manifest != "" {
			if manifest, err := readManifest(ref.Manifest); err == nil {
				estimatedSizeGB = len(manifest.Parts) * 3
			}
		}

		info := BackupInfo{
			Level:           int16(level),
			Type:            backupType,
			Datetime:        ref.Datetime,
			DatetimeStr:     time.Unix(ref.Datetime, 0).Format("2006-01-02 15:04:05"),
			Snapshot:        ref.Snapshot,
			ParentSnapshot:  "",
			ParentS3Path:    "",
			Blake3Hash:      ref.Blake3Hash,
			PartsCount:      0,
			EstimatedSizeGB: estimatedSizeGB,
			S3Path:          ref.S3Path,
			ManifestPath:    ref.Manifest,
		}

		// Get parent info if applicable
		if level > 0 && len(lastBackup.BackupLevels) > level-1 && lastBackup.BackupLevels[level-1] != nil {
			parentRef := lastBackup.BackupLevels[level-1]
			info.ParentSnapshot = parentRef.Snapshot
			info.ParentS3Path = parentRef.S3Path
		}

		// Get actual part count from manifest
		if ref.Manifest != "" {
			if manifest, err := readManifest(ref.Manifest); err == nil {
				info.PartsCount = len(manifest.Parts)
			}
		}

		output.Backups = append(output.Backups, info)
	}

	// Calculate summary
	output.Summary.TotalBackups = len(output.Backups)
	for _, backup := range output.Backups {
		if backup.Type == "full" {
			output.Summary.FullBackups++
		} else {
			output.Summary.IncrementalBackups++
		}
		output.Summary.TotalEstimatedSizeGB += backup.EstimatedSizeGB
	}

	// Output as JSON
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}

func readManifest(filename string) (*BackupManifest, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var manifest BackupManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func restoreBackup(ctx context.Context, configPath, taskName string, level int16, target, privateKeyPath, source string, dryRun bool) error {
	slog.Info("Restore started", "task", taskName, "level", level, "target", target, "source", source, "dryRun", dryRun)

	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Find the specified task
	var task *BackupTask
	for _, t := range config.Tasks {
		if t.Name == taskName {
			task = &t
			break
		}
	}
	if task == nil {
		return fmt.Errorf("task not found: %s", taskName)
	}

	// Validate target format (should be pool/dataset)
	targetParts := strings.Split(target, "/")
	if len(targetParts) < 2 {
		return fmt.Errorf("target must be in format pool/dataset, got: %s", target)
	}

	// Load age private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}
	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(privateKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}
	slog.Info("Private key loaded successfully")

	// Load manifest
	var manifest *BackupManifest
	var manifestPath string

	if source == "s3" {
		if !config.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		// Check storage class accessibility
		var storageClass string
		if level >= 0 && int(level) < len(config.S3.StorageClass.BackupData) {
			storageClass = string(config.S3.StorageClass.BackupData[level])
		} else {
			return fmt.Errorf("invalid backup level %d for configured storage classes", level)
		}

		if storageClass == "GLACIER" || storageClass == "DEEP_ARCHIVE" {
			return fmt.Errorf("cannot restore from S3: backup data storage class is %s (not immediately accessible)\n"+
				"You need to:\n"+
				"1. Initiate a restore request in AWS S3 console or via AWS CLI\n"+
				"2. Wait for the restore to complete (12-48 hours for DEEP_ARCHIVE)\n"+
				"3. Then retry this restore command", storageClass)
		}

		// Get manifest storage class
		manifestStorageClass := string(config.S3.StorageClass.Manifest)
		if manifestStorageClass == "GLACIER" || manifestStorageClass == "DEEP_ARCHIVE" {
			return fmt.Errorf("cannot restore from S3: manifest storage class is %s (not immediately accessible)", manifestStorageClass)
		}

		maxRetryAttempts := 3
		if config.S3.Retry.MaxAttempts > 0 {
			maxRetryAttempts = config.S3.Retry.MaxAttempts
		}

		backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region,
			config.S3.Prefix, config.S3.Endpoint,
			config.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}

		// Verify AWS credentials before proceeding
		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("AWS credentials verification failed: %w", err)
		}

		// First, download the last_backup_manifest to find the specific backup
		lastManifestPath := filepath.Join(os.TempDir(), fmt.Sprintf("restore_last_manifest_%s.yaml", taskName))
		defer os.Remove(lastManifestPath)

		remoteLastPath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		slog.Info("Downloading last backup manifest from S3", "remote", remoteLastPath)
		if err := backend.Download(ctx, remoteLastPath, lastManifestPath); err != nil {
			return fmt.Errorf("failed to download last backup manifest: %w", err)
		}

		lastBackup, err := readLastBackupManifest(lastManifestPath)
		if err != nil {
			return fmt.Errorf("failed to read last backup manifest: %w", err)
		}

		if int(level) >= len(lastBackup.BackupLevels) || lastBackup.BackupLevels[level] == nil {
			return fmt.Errorf("backup level %d not found", level)
		}

		backupRef := lastBackup.BackupLevels[level]
		s3Path := backupRef.S3Path

		// Download the task manifest
		manifestPath = filepath.Join(os.TempDir(), fmt.Sprintf("restore_manifest_%s_level%d.yaml", taskName, level))
		defer os.Remove(manifestPath)

		remoteManifestPath := filepath.Join("manifests", s3Path, "task_manifest.yaml")
		slog.Info("Downloading task manifest from S3", "remote", remoteManifestPath)
		if err := backend.Download(ctx, remoteManifestPath, manifestPath); err != nil {
			return fmt.Errorf("failed to download task manifest: %w", err)
		}
	} else {
		// Read from local
		lastPath := filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		lastBackup, err := readLastBackupManifest(lastPath)
		if err != nil {
			return fmt.Errorf("failed to read last backup manifest: %w", err)
		}

		if int(level) >= len(lastBackup.BackupLevels) || lastBackup.BackupLevels[level] == nil {
			return fmt.Errorf("backup level %d not found", level)
		}

		backupRef := lastBackup.BackupLevels[level]
		manifestPath = backupRef.Manifest
	}

	manifest, err = readManifest(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	slog.Info("Manifest loaded", "snapshot", manifest.TargetSnapshot, "parts", len(manifest.Parts), "blake3", manifest.Blake3Hash)

	// Dry run mode
	if dryRun {
		fmt.Printf("\n=== DRY RUN MODE ===\n")
		fmt.Printf("Would restore backup:\n")
		fmt.Printf("  Task:            %s\n", taskName)
		fmt.Printf("  Pool/Dataset:    %s/%s\n", manifest.Pool, manifest.Dataset)
		fmt.Printf("  Target:          %s\n", target)
		fmt.Printf("  Backup Level:    %d\n", manifest.BackupLevel)
		fmt.Printf("  Snapshot:        %s\n", manifest.TargetSnapshot)
		if manifest.ParentSnapshot != "" {
			fmt.Printf("  Parent Snapshot: %s\n", manifest.ParentSnapshot)
		}
		fmt.Printf("  Parts:           %d\n", len(manifest.Parts))
		fmt.Printf("  BLAKE3 Hash:     %s\n", manifest.Blake3Hash)
		fmt.Printf("  Source:          %s\n", source)
		fmt.Printf("\nNo changes made.\n")
		return nil
	}

	// Create temp directory for restore operations
	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("zrb_restore_%s_%d_%d", taskName, level, time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer func() {
		slog.Info("Cleaning up temp directory", "path", tempDir)
		if err := os.RemoveAll(tempDir); err != nil {
			slog.Warn("Failed to remove temp directory", "error", err)
		}
	}()

	slog.Info("Created temp directory", "path", tempDir)

	// Download and decrypt parts
	slog.Info("Processing parts", "count", len(manifest.Parts))
	decryptedParts := make([]string, len(manifest.Parts))

	for i, partInfo := range manifest.Parts {
		encryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))
		decryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s", partInfo.Index))

		if source == "s3" {
			// Download from S3
			maxRetryAttempts := 3
			if config.S3.Retry.MaxAttempts > 0 {
				maxRetryAttempts = config.S3.Retry.MaxAttempts
			}

			storageClass := config.S3.StorageClass.BackupData[level]
			backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region,
				config.S3.Prefix, config.S3.Endpoint, storageClass, maxRetryAttempts)
			if err != nil {
				return fmt.Errorf("failed to initialize S3 backend: %w", err)
			}

			remotePath := filepath.Join("data", manifest.TargetS3Path, fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))
			slog.Info("Downloading part from S3", "part", partInfo.Index, "remote", remotePath)
			if err := backend.Download(ctx, remotePath, encryptedFile); err != nil {
				return fmt.Errorf("failed to download part %s: %w", partInfo.Index, err)
			}
		} else {
			// Copy from local
			localEncrypted := filepath.Join(config.BaseDir, "task", manifest.Pool, manifest.Dataset,
				fmt.Sprintf("level%d", manifest.BackupLevel), time.Unix(manifest.Datetime, 0).Format("20060102"),
				fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))

			slog.Info("Copying part from local", "part", partInfo.Index, "path", localEncrypted)
			if err := copyFile(localEncrypted, encryptedFile); err != nil {
				return fmt.Errorf("failed to copy part %s: %w", partInfo.Index, err)
			}
		}

		// Decrypt and verify
		slog.Info("Decrypting and verifying part", "part", partInfo.Index)
		if err := decryptPartAndVerify(encryptedFile, decryptedFile, partInfo.Blake3Hash, identity); err != nil {
			return fmt.Errorf("failed to decrypt/verify part %s: %w", partInfo.Index, err)
		}

		decryptedParts[i] = decryptedFile
	}

	// Merge parts into single file
	mergedFile := filepath.Join(tempDir, "snapshot.merged")
	slog.Info("Merging parts", "output", mergedFile)
	if err := mergeParts(decryptedParts, mergedFile); err != nil {
		return fmt.Errorf("failed to merge parts: %w", err)
	}

	// Verify BLAKE3
	slog.Info("Verifying BLAKE3 hash")
	actualBlake3, err := calculateBLAKE3(mergedFile)
	if err != nil {
		return fmt.Errorf("failed to calculate BLAKE3: %w", err)
	}
	if actualBlake3 != manifest.Blake3Hash {
		return fmt.Errorf("BLAKE3 mismatch: expected %s, got %s", manifest.Blake3Hash, actualBlake3)
	}
	slog.Info("BLAKE3 verified", "hash", actualBlake3)

	// Execute ZFS receive
	slog.Info("Executing ZFS receive", "target", target)
	if err := executeZfsReceive(mergedFile, target); err != nil {
		return fmt.Errorf("ZFS receive failed: %w", err)
	}

	slog.Info("Restore completed successfully!")
	return nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return nil
}

// mergeParts merges multiple part files into a single output file
func mergeParts(parts []string, outputFile string) error {
	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	for _, partFile := range parts {
		part, err := os.Open(partFile)
		if err != nil {
			return fmt.Errorf("failed to open part %s: %w", partFile, err)
		}

		if _, err := io.Copy(out, part); err != nil {
			part.Close()
			return fmt.Errorf("failed to copy part %s: %w", partFile, err)
		}
		part.Close()
	}

	return nil
}

// executeZfsReceive executes zfs receive command
func executeZfsReceive(snapshotFile, target string) error {
	file, err := os.Open(snapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	cmd := exec.Command("zfs", "receive", "-F", target)
	cmd.Stdin = file
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	slog.Info("Running zfs receive", "target", target)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("zfs receive command failed: %w", err)
	}

	return nil
}
