package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/urfave/cli/v3"
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

					return runSnapshotCommand(pool, dataset, prefix)
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
