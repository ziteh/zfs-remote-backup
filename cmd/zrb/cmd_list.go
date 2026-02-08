package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

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

	task, err := findTask(config, taskName)
	if err != nil {
		return err
	}

	var lastBackup *LastBackup

	var lastPath string

	if source == "s3" {
		if !config.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		manifestStorageClass := string(config.S3.StorageClass.Manifest)
		if err := validateStorageClassAccessible(manifestStorageClass); err != nil {
			return fmt.Errorf("cannot list from S3: %w", err)
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

		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		lastPath = filepath.Join(os.TempDir(), fmt.Sprintf("last_backup_manifest_%s.yaml", taskName))

		slog.Info("Downloading manifest from S3", "remote", remotePath, "local", lastPath)

		if err := backend.Download(ctx, remotePath, lastPath); err != nil {
			return fmt.Errorf("failed to download manifest from S3: %w", err)
		}
		defer os.Remove(lastPath)
	} else {
		lastPath = filepath.Join(config.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
	}

	lastBackup, err = readLastBackupManifest(lastPath)
	if err != nil {
		return fmt.Errorf("failed to read backup manifest from %s: %w", lastPath, err)
	}

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

		estimatedSizeGB := len(ref.Blake3Hash)

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

		if level > 0 && len(lastBackup.BackupLevels) > level-1 && lastBackup.BackupLevels[level-1] != nil {
			parentRef := lastBackup.BackupLevels[level-1]
			info.ParentSnapshot = parentRef.Snapshot
			info.ParentS3Path = parentRef.S3Path
		}

		if ref.Manifest != "" {
			if manifest, err := readManifest(ref.Manifest); err == nil {
				info.PartsCount = len(manifest.Parts)
			}
		}

		output.Backups = append(output.Backups, info)
	}

	output.Summary.TotalBackups = len(output.Backups)
	for _, backup := range output.Backups {
		if backup.Type == "full" {
			output.Summary.FullBackups++
		} else {
			output.Summary.IncrementalBackups++
		}

		output.Summary.TotalEstimatedSizeGB += backup.EstimatedSizeGB
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(output); err != nil {
		return fmt.Errorf("failed to encode JSON: %w", err)
	}

	return nil
}
