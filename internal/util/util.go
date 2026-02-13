package util

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"zrb/internal/config"
	"zrb/internal/logging"
	"zrb/internal/remote"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TaskDirName(level int16, timestamp time.Time) string {
	return filepath.Join(
		fmt.Sprintf("level%d", level),
		timestamp.Format("20060102"),
	)
}

func OutputDir(baseDir, pool, dataset string, level int16, timestamp time.Time) string {
	return filepath.Join(baseDir, "task", pool, dataset, TaskDirName(level, timestamp))
}

func RunDir(baseDir, pool, dataset string) string {
	return filepath.Join(baseDir, "run", pool, dataset)
}

func LogDir(baseDir, pool, dataset string) string {
	return filepath.Join(baseDir, "logs", pool, dataset)
}

func SetupDirectories(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func SetupLogging(logPath string) (*slog.Logger, *os.File, error) {
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	logger, logFile, err := logging.NewLogger(logPath)
	if err != nil {
		return nil, nil, err
	}

	return logger, logFile, nil
}

func InitS3Backend(ctx context.Context, cfg *config.Config, level int16, forManifest bool) (remote.Backend, error) {
	if !cfg.S3.Enabled {
		return nil, fmt.Errorf("S3 is not enabled in config")
	}

	maxRetryAttempts := cfg.S3RetryAttempts()

	var storageClass types.StorageClass
	if forManifest {
		storageClass = cfg.S3.StorageClass.Manifest
	} else {
		if level < 0 || int(level) >= len(cfg.S3.StorageClass.BackupData) {
			return nil, fmt.Errorf("invalid backup level %d for configured storage classes", level)
		}
		storageClass = cfg.S3.StorageClass.BackupData[level]
	}

	backend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region, cfg.S3.Prefix, cfg.S3.Endpoint, storageClass, maxRetryAttempts)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3 backend: %w", err)
	}

	if err := backend.VerifyCredentials(ctx); err != nil {
		return nil, fmt.Errorf("AWS credentials verification failed: %w", err)
	}

	return backend, nil
}
