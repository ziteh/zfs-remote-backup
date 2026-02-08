package list

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"zrb/internal/config"
	"zrb/internal/manifest"
	"zrb/internal/remote"
)

type Info struct {
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

type Output struct {
	Task    string `json:"task"`
	Pool    string `json:"pool"`
	Dataset string `json:"dataset"`
	Source  string `json:"source"`
	Backups []Info `json:"backups"`
	Summary struct {
		TotalBackups         int `json:"total_backups"`
		FullBackups          int `json:"full_backups"`
		IncrementalBackups   int `json:"incremental_backups"`
		TotalEstimatedSizeGB int `json:"total_estimated_size_gb"`
	} `json:"summary"`
}

func Run(ctx context.Context, configPath, taskName string, filterLevel int16, source string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	task, err := cfg.FindTask(taskName)
	if err != nil {
		return err
	}

	var lastBackup *manifest.Last
	var lastPath string

	if source == "s3" {
		if !cfg.S3.Enabled {
			return fmt.Errorf("S3 is not enabled in config")
		}

		manifestStorageClass := string(cfg.S3.StorageClass.Manifest)
		if err := remote.ValidateStorageClass(manifestStorageClass); err != nil {
			return fmt.Errorf("cannot list from S3: %w", err)
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

		remotePath := filepath.Join("manifests", task.Pool, task.Dataset, "last_backup_manifest.yaml")
		lastPath = filepath.Join(os.TempDir(), fmt.Sprintf("last_backup_manifest_%s.yaml", taskName))

		slog.Info("Downloading manifest from S3", "remote", remotePath, "local", lastPath)

		if err := backend.Download(ctx, remotePath, lastPath); err != nil {
			return fmt.Errorf("failed to download manifest from S3: %w", err)
		}
		defer os.Remove(lastPath)
	} else {
		lastPath = filepath.Join(cfg.BaseDir, "run", task.Pool, task.Dataset, "last_backup_manifest.yaml")
	}

	lastBackup, err = manifest.ReadLast(lastPath)
	if err != nil {
		return fmt.Errorf("failed to read backup manifest from %s: %w", lastPath, err)
	}

	output := Output{
		Task:    taskName,
		Pool:    task.Pool,
		Dataset: task.Dataset,
		Source:  source,
		Backups: []Info{},
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
			if m, err := manifest.Read(ref.Manifest); err == nil {
				estimatedSizeGB = len(m.Parts) * 3
			}
		}

		info := Info{
			Level:           int16(level),
			Type:            backupType,
			Datetime:        ref.Datetime,
			DatetimeStr:     time.Unix(ref.Datetime, 0).Format("2006-01-02 15:04:05"),
			Snapshot:        ref.Snapshot,
			Blake3Hash:      ref.Blake3Hash,
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
			if m, err := manifest.Read(ref.Manifest); err == nil {
				info.PartsCount = len(m.Parts)
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
