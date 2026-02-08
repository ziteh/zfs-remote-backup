package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

func restoreBackup(ctx context.Context, configPath, taskName string, level int16, target, privateKeyPath, source string, dryRun bool) error {
	slog.Info("Restore started", "task", taskName, "level", level, "target", target, "source", source, "dryRun", dryRun)

	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	task, err := findTask(config, taskName)
	if err != nil {
		return err
	}

	targetParts := strings.Split(target, "/")
	if len(targetParts) < 2 {
		return fmt.Errorf("target must be in format pool/dataset, got: %s", target)
	}

	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(privateKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	slog.Info("Private key loaded successfully")

	var manifest *BackupManifest

	var manifestPath string

	if source == "s3" {
		if !config.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		var storageClass string
		if level >= 0 && int(level) < len(config.S3.StorageClass.BackupData) {
			storageClass = string(config.S3.StorageClass.BackupData[level])
		} else {
			return fmt.Errorf("invalid backup level %d for configured storage classes", level)
		}

		if err := validateStorageClassAccessible(storageClass); err != nil {
			return fmt.Errorf("cannot restore from S3: backup data storage class is %s (not immediately accessible)\n"+
				"You need to:\n"+
				"1. Initiate a restore request in AWS S3 console or via AWS CLI\n"+
				"2. Wait for the restore to complete (12-48 hours for DEEP_ARCHIVE)\n"+
				"3. Then retry this restore command", storageClass)
		}

		manifestStorageClass := string(config.S3.StorageClass.Manifest)
		if err := validateStorageClassAccessible(manifestStorageClass); err != nil {
			return fmt.Errorf("cannot restore from S3: manifest %w", err)
		}

		maxRetryAttempts := getS3RetryConfig(config)

		backend, err := NewS3Backend(ctx, config.S3.Bucket, config.S3.Region,
			config.S3.Prefix, config.S3.Endpoint,
			config.S3.StorageClass.Manifest, maxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to initialize S3 backend: %w", err)
		}

		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("AWS credentials verification failed: %w", err)
		}

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

		manifestPath = filepath.Join(os.TempDir(), fmt.Sprintf("restore_manifest_%s_level%d.yaml", taskName, level))
		defer os.Remove(manifestPath)

		remoteManifestPath := filepath.Join("manifests", s3Path, "task_manifest.yaml")
		slog.Info("Downloading task manifest from S3", "remote", remoteManifestPath)

		if err := backend.Download(ctx, remoteManifestPath, manifestPath); err != nil {
			return fmt.Errorf("failed to download task manifest: %w", err)
		}
	} else {
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

	tempDir := filepath.Join(os.TempDir(), fmt.Sprintf("zrb_restore_%s_%d_%d", taskName, level, time.Now().Unix()))
	if err := os.MkdirAll(tempDir, 0o755); err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}

	defer func() {
		slog.Info("Cleaning up temp directory", "path", tempDir)

		if err := os.RemoveAll(tempDir); err != nil {
			slog.Warn("Failed to remove temp directory", "error", err)
		}
	}()

	slog.Info("Created temp directory", "path", tempDir)

	slog.Info("Processing parts", "count", len(manifest.Parts))
	decryptedParts := make([]string, len(manifest.Parts))

	for i, partInfo := range manifest.Parts {
		encryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))
		decryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s", partInfo.Index))

		if source == "s3" {
			maxRetryAttempts := getS3RetryConfig(config)
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
			localEncrypted := filepath.Join(config.BaseDir, "task", manifest.Pool, manifest.Dataset,
				fmt.Sprintf("level%d", manifest.BackupLevel), time.Unix(manifest.Datetime, 0).Format("20060102"),
				fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))

			slog.Info("Copying part from local", "part", partInfo.Index, "path", localEncrypted)

			if err := copyFile(localEncrypted, encryptedFile); err != nil {
				return fmt.Errorf("failed to copy part %s: %w", partInfo.Index, err)
			}
		}

		slog.Info("Decrypting and verifying part", "part", partInfo.Index)

		if err := decryptPartAndVerify(encryptedFile, decryptedFile, partInfo.Blake3Hash, identity); err != nil {
			return fmt.Errorf("failed to decrypt/verify part %s: %w", partInfo.Index, err)
		}

		decryptedParts[i] = decryptedFile
	}

	mergedFile := filepath.Join(tempDir, "snapshot.merged")
	slog.Info("Merging parts", "output", mergedFile)

	if err := mergeParts(decryptedParts, mergedFile); err != nil {
		return fmt.Errorf("failed to merge parts: %w", err)
	}

	slog.Info("Verifying BLAKE3 hash")

	actualBlake3, err := calculateBLAKE3(mergedFile)
	if err != nil {
		return fmt.Errorf("failed to calculate BLAKE3: %w", err)
	}

	if actualBlake3 != manifest.Blake3Hash {
		return fmt.Errorf("BLAKE3 mismatch: expected %s, got %s", manifest.Blake3Hash, actualBlake3)
	}

	slog.Info("BLAKE3 verified", "hash", actualBlake3)

	slog.Info("Executing ZFS receive", "target", target)

	if err := executeZfsReceive(mergedFile, target); err != nil {
		return fmt.Errorf("ZFS receive failed: %w", err)
	}

	slog.Info("Restore completed successfully!")

	return nil
}

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
