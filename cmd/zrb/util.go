package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func findTask(cfg *Config, taskName string) (*BackupTask, error) {
	for _, t := range cfg.Tasks {
		if t.Name == taskName {
			return &t, nil
		}
	}

	return nil, fmt.Errorf("task not found: %s", taskName)
}

func initializeS3Backend(ctx context.Context, cfg *Config, level int16, forManifest bool) (RemoteBackend, error) {
	if !cfg.S3.Enabled {
		return nil, fmt.Errorf("S3 is not enabled in config")
	}

	maxRetryAttempts := getS3RetryConfig(cfg)

	var storageClass types.StorageClass
	if forManifest {
		storageClass = cfg.S3.StorageClass.Manifest
	} else {
		if level < 0 || int(level) >= len(cfg.S3.StorageClass.BackupData) {
			return nil, fmt.Errorf("invalid backup level %d for configured storage classes", level)
		}

		storageClass = cfg.S3.StorageClass.BackupData[level]
	}

	backend, err := NewS3Backend(ctx, cfg.S3.Bucket, cfg.S3.Region, cfg.S3.Prefix, cfg.S3.Endpoint, storageClass, maxRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3 backend: %w", err)
	}

	if err := backend.VerifyCredentials(ctx); err != nil {
		return nil, fmt.Errorf("AWS credentials verification failed: %w", err)
	}

	return backend, nil
}

func getS3RetryConfig(cfg *Config) int {
	if cfg.S3.Retry.MaxAttempts > 0 {
		return cfg.S3.Retry.MaxAttempts
	}

	return 3
}

func buildOutputDir(baseDir, pool, dataset string, level int16, timestamp time.Time) string {
	taskDirName := buildTaskDirName(level, timestamp)

	return filepath.Join(baseDir, "task", pool, dataset, taskDirName)
}

func buildRunDir(baseDir, pool, dataset string) string {
	return filepath.Join(baseDir, "run", pool, dataset)
}

func buildLogDir(baseDir, pool, dataset string) string {
	return filepath.Join(baseDir, "logs", pool, dataset)
}

func buildTaskDirName(level int16, timestamp time.Time) string {
	return filepath.Join(
		fmt.Sprintf("level%d", level),
		timestamp.Format("20060102"),
	)
}

func validateStorageClassAccessible(storageClass string) error {
	if storageClass == "GLACIER" || storageClass == "DEEP_ARCHIVE" {
		return fmt.Errorf("storage class %s is not immediately accessible (requires restore)", storageClass)
	}

	return nil
}

func setupDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

func setupLogging(logPath string) (*slog.Logger, *os.File, error) {
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger, logFile := NewLogger(logPath)

	return logger, logFile, nil
}
