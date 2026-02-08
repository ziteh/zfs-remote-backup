package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestS3RetryAttempts(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
		want   int
	}{
		{
			name: "custom retry attempts",
			config: &Config{
				S3: S3Config{
					Retry: struct {
						MaxAttempts int `yaml:"max_attempts"`
					}{
						MaxAttempts: 5,
					},
				},
			},
			want: 5,
		},
		{
			name: "default retry attempts",
			config: &Config{
				S3: S3Config{
					Retry: struct {
						MaxAttempts int `yaml:"max_attempts"`
					}{
						MaxAttempts: 0,
					},
				},
			},
			want: 3,
		},
		{
			name: "zero retry config",
			config: &Config{
				S3: S3Config{},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.S3RetryAttempts()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFindTask(t *testing.T) {
	cfg := &Config{
		Tasks: []Task{
			{Name: "task1", Pool: "pool1", Dataset: "dataset1", Enabled: true},
			{Name: "task2", Pool: "pool2", Dataset: "dataset2", Enabled: false},
			{Name: "task3", Pool: "pool3", Dataset: "dataset3", Enabled: true},
		},
	}

	tests := []struct {
		name     string
		taskName string
		wantTask *Task
		wantErr  bool
	}{
		{
			name:     "find existing task",
			taskName: "task1",
			wantTask: &Task{Name: "task1", Pool: "pool1", Dataset: "dataset1", Enabled: true},
			wantErr:  false,
		},
		{
			name:     "find disabled task",
			taskName: "task2",
			wantTask: &Task{Name: "task2", Pool: "pool2", Dataset: "dataset2", Enabled: false},
			wantErr:  false,
		},
		{
			name:     "task not found",
			taskName: "nonexistent",
			wantTask: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task, err := cfg.FindTask(tt.taskName)

			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, task)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, task)
				assert.Equal(t, tt.wantTask.Name, task.Name)
				assert.Equal(t, tt.wantTask.Pool, task.Pool)
				assert.Equal(t, tt.wantTask.Dataset, task.Dataset)
				assert.Equal(t, tt.wantTask.Enabled, task.Enabled)
			}
		})
	}
}
