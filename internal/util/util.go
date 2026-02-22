package util

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"
	"zrb/internal/logging"
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
