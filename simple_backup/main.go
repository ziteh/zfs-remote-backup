package main

import (
	"context"
	"fmt"
	"log"
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
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runBackup(ctx)
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


	fmt.Println("Public key: age1xxxxxxxxxxxxx...")
	fmt.Println("Private key: AGE-SECRET-KEY-1xxxxxxxxxxxxx...")
	return nil
}

func runBackup(ctx context.Context) error {
	fmt.Println("Running backup task...")

	// configPath := flag.String("config", "zrb_simple_config.yaml", "path to config file")
	// flag.Parse()

	config, err := loadConfig("zrb_simple_config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	outputDir := filepath.Join(config.ExportDir, config.Pool, config.Dataset, config.BaseSnapshotName)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create export directory: %v", err)
	}

	snapshotPath := fmt.Sprintf("%s/%s@%s", config.Pool, config.Dataset, config.BaseSnapshotName)
	blake3Hash, err := runZfsSendAndSplit(snapshotPath, outputDir)
	if err != nil {
		log.Fatalf("Failed to run zfs send and split: %v", err)
	}
	log.Printf("Snapshot BLAKE3: %s", blake3Hash)

	parts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
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
		log.Fatalf("Failed to parse age public key: %v", err)
	}

	// Initialize S3 backend if enabled
	var backend RemoteBackend
	ctxBg := context.Background()
	if config.S3.Enabled {
		s3Backend, err := NewS3Backend(ctxBg, config.S3.Bucket, config.S3.Region, config.S3.Prefix, config.S3.Endpoint, config.S3.StorageClass)
		if err != nil {
			log.Fatalf("Failed to initialize S3 backend: %v", err)
		}
		backend = s3Backend
		log.Printf("S3 backend initialized: bucket=%s, region=%s, prefix=%s",
			config.S3.Bucket, config.S3.Region, config.S3.Prefix)
	}

	var partInfos []PartInfo
	for _, partFile := range rawParts {
		sha256Hash, encryptedFile, err := processPartFile(partFile, recipient)
		if err != nil {
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
			remotePath := filepath.Join(config.Pool, config.Dataset, config.BaseSnapshotName, filepath.Base(encryptedFile))
			if err := backend.Upload(ctxBg, encryptedFile, remotePath, sha256Hash); err != nil {
				log.Fatalf("Failed to upload %s: %v", encryptedFile, err)
			}
		}
	}

	systemInfo, err := getSystemInfo()
	if err != nil {
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
		BaseSnapshotName: config.BaseSnapshotName,
		AgePublicKey:     config.AgePublicKey,
		Blake3Hash:       blake3Hash,
		Parts:            partInfos,
	}

	manifestPath := filepath.Join(outputDir, "backup_manifest.yaml")
	if err := writeManifest(manifestPath, &manifest); err != nil {
		log.Fatalf("Failed to write manifest: %v", err)
	}
	log.Printf("Manifest written to: %s", manifestPath)

	// Upload manifest to S3
	if backend != nil {
		manifestSHA256, err := calculateSHA256(manifestPath)
		if err != nil {
			log.Fatalf("Failed to calculate manifest SHA256: %v", err)
		}
		remotePath := filepath.Join(config.Pool, config.Dataset, config.BaseSnapshotName, "backup_manifest.yaml")
		if err := backend.Upload(ctxBg, manifestPath, remotePath, manifestSHA256); err != nil {
			log.Fatalf("Failed to upload manifest: %v", err)
		}
	}

	log.Println("All parts processed successfully")

	fmt.Println("Backup completed successfully!")
	return nil
}
