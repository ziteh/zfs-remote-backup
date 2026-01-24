package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/urfave/cli/v3"
)

func main() {
	cmd := &cli.Command{
		Name:  "zrb_simple",
		Usage: "ZFS Remote Backup",
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
						Name:  "type",
						Usage: "Type of backup to perform (full, diff, incr).",
						Value: "full",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					configPath := cmd.String("config")
					backupType := cmd.String("type")
					return runBackup(ctx, configPath, backupType)
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
						Value: "zrb",
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
		log.Fatal(err)
	}
}

func generateKey(ctx context.Context) error {
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

func runBackup(ctx context.Context, configPath string, backupType string) error {
	if backupType != "full" && backupType != "diff" && backupType != "incr" {
		return fmt.Errorf("invalid backup type: %s", backupType)
	}

	fmt.Println("Running backup task...")

	config, err := loadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := os.MkdirAll(config.ExportDir, 0755); err != nil {
		log.Fatalf("Failed to create export directory: %v", err)
	}

	// TODO: change path to /var/log/zrb/XXX_YYYY-MM-DD.log ? and ENV override?
	logPath := filepath.Join(config.ExportDir, config.Pool, config.Dataset, fmt.Sprintf("zrb_backup_%s.log", time.Now().Format("2006-01-02")))
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatalf("Failed to open log file: %v", err)
	}
	defer file.Close()
	logger := slog.New(slog.NewJSONHandler(file, nil))
	slog.SetDefault(logger)
	logger.Info("Backup started", "type", backupType, "pool", config.Pool, "dataset", config.Dataset)

	lockPath := filepath.Join(config.ExportDir, "locks.yaml")
	releaseLock, err := AcquireLock(lockPath, config.Pool, config.Dataset)
	if err != nil {
		logger.Error("Failed to acquire lock", "error", err)
		log.Fatalf("Failed to acquire lock: %v", err)
	}
	defer func() {
		if err := releaseLock(); err != nil {
			logger.Warn("Failed to release lock", "error", err)
			log.Printf("Warning: Failed to release lock: %v", err)
		}
	}()

	snapshots, err := listSnapshots(config.Pool, config.Dataset, "zrb_"+backupType)
	if err != nil {
		logger.Error("Failed to list snapshots", "error", err)
		log.Fatalf("Failed to list snapshots: %v", err)
	}
	if len(snapshots) == 0 {
		logger.Error("No snapshots found", "pool", config.Pool, "dataset", config.Dataset)
		log.Fatalf("No snapshots found for %s/%s", config.Pool, config.Dataset)
	}

	latestSnapshot := snapshots[len(snapshots)-1]
	logger.Info("Latest snapshot found", "snapshot", latestSnapshot)

	outputDir := filepath.Join(config.ExportDir, config.Pool, config.Dataset, backupType)
	// Clean up output directory if it exists
	if _, err := os.Stat(outputDir); err == nil {
		logger.Info("Cleaning up existing output directory", "path", outputDir)
		if err := os.RemoveAll(outputDir); err != nil {
			logger.Error("Failed to remove existing output directory", "error", err)
			log.Fatalf("Failed to remove existing output directory: %v", err)
		}
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		logger.Error("Failed to create export directory", "error", err)
		log.Fatalf("Failed to create export directory: %v", err)
	}

	snapshotPath := latestSnapshot // TODO: refactor

	// determine base snapshot for incremental sends when needed
	lastPath := filepath.Join(config.ExportDir, config.Pool, config.Dataset, "last_backup.yaml")
	var baseSnapshot string
	if backupType == "diff" {
		last, err := readLastBackupManifest(lastPath)
		if err != nil || last == nil || last.Full == nil {
			logger.Error("Failed to determine base for diff", "error", err)
			log.Fatalf("Failed to determine base for diff: no previous full backup recorded")
		}
		baseSnapshot = last.Full.Snapshot
	} else if backupType == "incr" {
		last, err := readLastBackupManifest(lastPath)
		if err != nil || last == nil {
			logger.Error("Failed to determine base for incr", "error", err)
			log.Fatalf("Failed to determine base for incr: no last backup record")
		}
		// prefer most recent: incr > diff > full
		if last.Incr != nil {
			baseSnapshot = last.Incr.Snapshot
		} else if last.Diff != nil {
			baseSnapshot = last.Diff.Snapshot
		} else if last.Full != nil {
			baseSnapshot = last.Full.Snapshot
		} else {
			logger.Error("Failed to determine base for incr", "error", err)
			log.Fatalf("Failed to determine base for incr: no prior backups recorded")
		}
	}

	blake3Hash, err := runZfsSendAndSplit(snapshotPath, baseSnapshot, outputDir)
	if err != nil {
		logger.Error("Failed to run zfs send and split", "error", err)
		log.Fatalf("Failed to run zfs send and split: %v", err)
	}
	logger.Info("Snapshot BLAKE3", "hash", blake3Hash)

	parts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
		logger.Error("Failed to find snapshot parts", "error", err)
		log.Fatalf("Failed to find snapshot parts: %v", err)
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
		logger.Error("Failed to parse age public key", "error", err)
		log.Fatalf("Failed to parse age public key: %v", err)
	}

	// Initialize S3 backend if enabled
	var backend RemoteBackend = nil
	ctxBg := context.Background()
	if config.S3.Enabled {
		s3Backend, err := NewS3Backend(ctxBg, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, config.S3.StorageClass)
		if err != nil {
			logger.Error("Failed to initialize S3 backend", "error", err)
			log.Fatalf("Failed to initialize S3 backend: %v", err)
		}
		backend = s3Backend
		logger.Info("S3 backend initialized", "bucket", config.S3.Bucket, "region", config.S3.Region, "prefix", config.S3.Prefix)
	}

	var partInfos []PartInfo
	for _, partFile := range rawParts {
		sha256Hash, encryptedFile, err := processPartFile(partFile, recipient)
		if err != nil {
			logger.Error("Failed to process part file", "partFile", partFile, "error", err)
			log.Fatalf("Failed to process %s: %v", partFile, err)
		}

		baseName := filepath.Base(partFile)
		index := strings.TrimPrefix(baseName, "snapshot.part-")
		partInfos = append(partInfos, PartInfo{
			Index:      index,
			SHA256Hash: sha256Hash,
		})

		// Upload to remote backend if configured
		if backend != nil {
			remotePath := filepath.Join(config.Pool, config.Dataset, latestSnapshot, filepath.Base(encryptedFile))
			if err := backend.Upload(ctxBg, encryptedFile, remotePath, sha256Hash); err != nil {
				logger.Error("Failed to upload part file", "encryptedFile", encryptedFile, "error", err)
				log.Fatalf("Failed to upload %s: %v", encryptedFile, err)
			}
		}
	}

	systemInfo, err := getSystemInfo()
	if err != nil {
		logger.Warn("Failed to get system info", "error", err)
		log.Printf("Warning: Failed to get system info: %v", err)
		systemInfo = SystemInfo{}
		systemInfo.OS = "unknown"
		systemInfo.ZFSVersion.Userland = "unknown"
		systemInfo.ZFSVersion.Kernel = "unknown"
	}

	manifest := BackupManifest{
		Datetime:         time.Now().Unix(),
		System:           systemInfo,
		Pool:             config.Pool,
		Dataset:          config.Dataset,
		BaseSnapshotName: latestSnapshot,
		AgePublicKey:     config.AgePublicKey,
		Blake3Hash:       blake3Hash,
		Parts:            partInfos,
	}

	manifestPath := filepath.Join(outputDir, "backup_manifest.yaml")
	if err := writeManifest(manifestPath, &manifest); err != nil {
		logger.Error("Failed to write manifest", "error", err)
		log.Fatalf("Failed to write manifest: %v", err)
	}
	logger.Info("Manifest written", "path", manifestPath)

	// Upload manifest to S3
	if backend != nil {
		manifestSHA256, err := calculateSHA256(manifestPath)
		if err != nil {
			logger.Error("Failed to calculate manifest SHA256", "error", err)
			log.Fatalf("Failed to calculate manifest SHA256: %v", err)
		}
		remotePath := filepath.Join(config.Pool, config.Dataset, latestSnapshot, "backup_manifest.yaml")
		if err := backend.Upload(ctxBg, manifestPath, remotePath, manifestSHA256); err != nil {
			logger.Error("Failed to upload manifest", "error", err)
			log.Fatalf("Failed to upload manifest: %v", err)
		}
	}

	// update last_backup.yaml preserving other fields if present
	lastPath = filepath.Join(config.ExportDir, config.Pool, config.Dataset, "last_backup.yaml")
	var last LastBackup
	if existing, err := readLastBackupManifest(lastPath); err == nil && existing != nil {
		last = *existing
	}
	last.Pool = config.Pool
	last.Dataset = config.Dataset
	ref := &BackupRef{
		Datetime:   time.Now().Unix(),
		Snapshot:   latestSnapshot,
		Manifest:   manifestPath,
		Blake3Hash: blake3Hash,
	}
	switch backupType {
	case "full":
		last.Full = ref
	case "diff":
		last.Diff = ref
	case "incr":
		last.Incr = ref
	}

	if err := writeLastBackupManifest(lastPath, &last); err != nil {
		logger.Warn("Failed to write last backup manifest", "error", err)
	} else {
		logger.Info("Last backup manifest written", "path", lastPath)
	}

	// Clean up local files if S3 is used
	if backend != nil {
		logger.Info("Cleaning up local backup files", "path", outputDir)
		if err := os.RemoveAll(outputDir); err != nil {
			logger.Warn("Failed to clean up local files", "error", err)
			log.Printf("Warning: Failed to clean up local files: %v", err)
		}
	}

	logger.Info("All parts processed successfully")
	logger.Info("Backup completed successfully!")
	return nil
}
