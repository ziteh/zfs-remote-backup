package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTaskDirName(t *testing.T) {
	tests := []struct {
		name      string
		level     int16
		timestamp time.Time
		want      string
	}{
		{
			name:      "level 0 backup",
			level:     0,
			timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
			want:      "level0/20240115",
		},
		{
			name:      "level 1 backup",
			level:     1,
			timestamp: time.Date(2024, 2, 28, 14, 45, 0, 0, time.UTC),
			want:      "level1/20240228",
		},
		{
			name:      "level 4 backup",
			level:     4,
			timestamp: time.Date(2024, 12, 31, 23, 59, 59, 0, time.UTC),
			want:      "level4/20241231",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TaskDirName(tt.level, tt.timestamp)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRunDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		pool    string
		dataset string
		want    string
	}{
		{
			name:    "standard path",
			baseDir: "/home/user/zrb_base",
			pool:    "testpool",
			dataset: "backup_data",
			want:    "/home/user/zrb_base/run/testpool/backup_data",
		},
		{
			name:    "relative path",
			baseDir: "./data",
			pool:    "mypool",
			dataset: "mydataset",
			want:    "data/run/mypool/mydataset",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := RunDir(tt.baseDir, tt.pool, tt.dataset)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestLogDir(t *testing.T) {
	tests := []struct {
		name    string
		baseDir string
		pool    string
		dataset string
		want    string
	}{
		{
			name:    "standard path",
			baseDir: "/var/log/zrb",
			pool:    "tank",
			dataset: "data",
			want:    "/var/log/zrb/logs/tank/data",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LogDir(tt.baseDir, tt.pool, tt.dataset)
			assert.Equal(t, tt.want, got)
		})
	}
}
