package check

import (
	"context"
	"fmt"
	"zrb/internal/config"
	"zrb/internal/remote"
	"zrb/internal/zfs"
)

func Run(ctx context.Context, configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	fmt.Println("config: OK")

	for _, task := range cfg.Tasks {
		if !task.Enabled {
			fmt.Printf("task %s: skipped (disabled)\n", task.Name)
			continue
		}
		if err := zfs.CheckDatasetExists(task.Pool, task.Dataset); err != nil {
			return fmt.Errorf("task %s: %w", task.Name, err)
		}
		fmt.Printf("task %s dataset %s/%s: OK\n", task.Name, task.Pool, task.Dataset)
	}

	if cfg.S3.Enabled {
		backend, err := remote.NewS3(ctx, cfg.S3.Bucket, cfg.S3.Region,
			cfg.S3.Prefix, cfg.S3.Endpoint,
			cfg.S3.StorageClass.Manifest, cfg.S3RetryAttempts())
		if err != nil {
			return fmt.Errorf("S3 init: %w", err)
		}
		if err := backend.VerifyCredentials(ctx); err != nil {
			return fmt.Errorf("S3 credentials: %w", err)
		}
		fmt.Printf("S3 bucket %s: OK\n", cfg.S3.Bucket)
	}

	fmt.Println("all checks passed")
	return nil
}
