package restore

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
	"zrb/internal/config"
	"zrb/internal/crypto"
	"zrb/internal/manifest"
	"zrb/internal/remote"

	"filippo.io/age"
)

func Run(ctx context.Context, configPath, taskName string, level int16, target, privateKeyPath, source string, dryRun, force bool) error {
	slog.Info("Restore started", "task", taskName, "level", level, "target", target, "source", source, "dryRun", dryRun)

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	task, err := cfg.FindTask(taskName)
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

	var m *manifest.Backup
	var manifestPath string

	if source == "s3" {
		if !cfg.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		var storageClass string
		if level >= 0 && int(level) < len(cfg.S3.StorageClass.BackupData) {
			storageClass = string(cfg.S3.StorageClass.BackupData[level])
		} else {
			return fmt.Errorf("invalid backup level %d for configured storage classes", level)
		}

		if err := remote.ValidateStorageClass(storageClass); err != nil {
			return fmt.Errorf("cannot restore from S3: backup data storage class is %s (not immediately accessible)\n"+
				"You need to:\n"+
				"1. Initiate a restore request in AWS S3 console or via AWS CLI\n"+
				"2. Wait for the restore to complete (12-48 hours for DEEP_ARCHIVE)\n"+
				"3. Then retry this restore command", storageClass)
		}

		manifestStorageClass := string(cfg.S3.StorageClass.Manifest)
		if err := remote.ValidateStorageClass(manifestStorageClass); err != nil {
			return fmt.Errorf("cannot restore from S3: manifest %w", err)
		}

		maxRetryAttempts := cfg.S3RetryAttempts()

		backend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region,
			cfg.S3.Prefix, cfg.S3.Endpoint,
			cfg.S3.StorageClass.Manifest, maxRetryAttempts)
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

		lastBackup, err := manifest.ReadLast(lastManifestPath)
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
		lastPath := filepath.Join(cfg.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")

		lastBackup, err := manifest.ReadLast(lastPath)
		if err != nil {
			return fmt.Errorf("failed to read last backup manifest: %w", err)
		}

		if int(level) >= len(lastBackup.BackupLevels) || lastBackup.BackupLevels[level] == nil {
			return fmt.Errorf("backup level %d not found", level)
		}

		backupRef := lastBackup.BackupLevels[level]
		manifestPath = backupRef.Manifest
	}

	m, err = manifest.Read(manifestPath)
	if err != nil {
		return fmt.Errorf("failed to read manifest: %w", err)
	}

	slog.Info("Manifest loaded", "snapshot", m.TargetSnapshot, "parts", len(m.Parts), "blake3", m.Blake3Hash)

	if dryRun {
		fmt.Printf("\n=== DRY RUN MODE ===\n")
		fmt.Printf("Would restore backup:\n")
		fmt.Printf("  Task:            %s\n", taskName)
		fmt.Printf("  Pool/Dataset:    %s/%s\n", m.Pool, m.Dataset)
		fmt.Printf("  Target:          %s\n", target)
		fmt.Printf("  Backup Level:    %d\n", m.BackupLevel)
		fmt.Printf("  Snapshot:        %s\n", m.TargetSnapshot)
		if m.ParentSnapshot != "" {
			fmt.Printf("  Parent Snapshot: %s\n", m.ParentSnapshot)
		}
		fmt.Printf("  Parts:           %d\n", len(m.Parts))
		fmt.Printf("  BLAKE3 Hash:     %s\n", m.Blake3Hash)
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

	slog.Info("Processing parts", "count", len(m.Parts))
	decryptedParts := make([]string, len(m.Parts))

	for i, partInfo := range m.Parts {
		encryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))
		decryptedFile := filepath.Join(tempDir, fmt.Sprintf("snapshot.part-%s", partInfo.Index))

		if source == "s3" {
			maxRetryAttempts := cfg.S3RetryAttempts()
			storageClass := cfg.S3.StorageClass.BackupData[level]

			backend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region,
				cfg.S3.Prefix, cfg.S3.Endpoint, storageClass, maxRetryAttempts)
			if err != nil {
				return fmt.Errorf("failed to initialize S3 backend: %w", err)
			}

			remotePath := filepath.Join("data", m.TargetS3Path, fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))
			slog.Info("Downloading part from S3", "part", partInfo.Index, "remote", remotePath)

			if err := backend.Download(ctx, remotePath, encryptedFile); err != nil {
				return fmt.Errorf("failed to download part %s: %w", partInfo.Index, err)
			}
		} else {
			localEncrypted := filepath.Join(cfg.BaseDir, "task", m.Pool, m.Dataset,
				fmt.Sprintf("level%d", m.BackupLevel), time.Unix(m.Datetime, 0).Format("20060102"),
				fmt.Sprintf("snapshot.part-%s.age", partInfo.Index))

			slog.Info("Copying part from local", "part", partInfo.Index, "path", localEncrypted)

			if err := copyFile(localEncrypted, encryptedFile); err != nil {
				return fmt.Errorf("failed to copy part %s: %w", partInfo.Index, err)
			}
		}

		slog.Info("Decrypting and verifying part", "part", partInfo.Index)

		if err := crypto.DecryptAndVerify(encryptedFile, decryptedFile, partInfo.Blake3Hash, identity); err != nil {
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

	actualBlake3, err := crypto.BLAKE3File(mergedFile)
	if err != nil {
		return fmt.Errorf("failed to calculate BLAKE3: %w", err)
	}

	if actualBlake3 != m.Blake3Hash {
		return fmt.Errorf("BLAKE3 mismatch: expected %s, got %s", m.Blake3Hash, actualBlake3)
	}

	slog.Info("BLAKE3 verified", "hash", actualBlake3)

	slog.Info("Executing ZFS receive", "target", target)

	if err := executeZfsReceive(mergedFile, target, force); err != nil {
		return fmt.Errorf("ZFS receive failed: %w", err)
	}

	if err := verifyRestoredSnapshot(target, m.TargetSnapshot); err != nil {
		return fmt.Errorf("restore verification failed: %w", err)
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

func verifyRestoredSnapshot(target, originalSnapshot string) error {
	parts := strings.SplitN(originalSnapshot, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("cannot parse snapshot name from: %s", originalSnapshot)
	}
	expected := target + "@" + parts[1]
	cmd := exec.Command("zfs", "list", "-H", "-o", "name", "-t", "snapshot", expected)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("snapshot %s not found after restore: %w", expected, err)
	}
	slog.Info("Restored snapshot verified", "snapshot", expected)
	return nil
}

func executeZfsReceive(snapshotFile, target string, force bool) error {
	file, err := os.Open(snapshotFile)
	if err != nil {
		return fmt.Errorf("failed to open snapshot file: %w", err)
	}
	defer file.Close()

	args := []string{"receive"}
	if force {
		args = append(args, "-F")
	}
	args = append(args, target)

	cmd := exec.Command("zfs", args...)
	cmd.Stdin = file
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	slog.Info("Running zfs receive", "target", target, "force", force)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("zfs receive command failed: %w", err)
	}

	return nil
}
