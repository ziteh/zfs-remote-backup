package config

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidate(t *testing.T) {
	validConfig := func() *Config {
		return &Config{
			BaseDir:      "/tmp/zrb",
			AgePublicKey: "age1ql3z7hjy54pw3hyww5ayyfg7zqgvc7w3j2elw8zmrj2kg5sfn9aqmcac8p",
			Tasks: []Task{
				{Name: "t1", Pool: "p1", Dataset: "d1", Enabled: true},
			},
		}
	}

	t.Run("valid config", func(t *testing.T) {
		require.NoError(t, validConfig().Validate())
	})

	t.Run("empty base_dir", func(t *testing.T) {
		cfg := validConfig()
		cfg.BaseDir = ""
		assert.ErrorContains(t, cfg.Validate(), "base_dir is required")
	})

	t.Run("empty age_public_key", func(t *testing.T) {
		cfg := validConfig()
		cfg.AgePublicKey = ""
		assert.ErrorContains(t, cfg.Validate(), "age_public_key is required")
	})

	t.Run("invalid age_public_key prefix", func(t *testing.T) {
		cfg := validConfig()
		cfg.AgePublicKey = "invalid-key"
		assert.ErrorContains(t, cfg.Validate(), "age_public_key must start with")
	})

	t.Run("no tasks", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tasks = nil
		assert.ErrorContains(t, cfg.Validate(), "at least one task")
	})

	t.Run("task missing name", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tasks = []Task{{Pool: "p", Dataset: "d"}}
		assert.ErrorContains(t, cfg.Validate(), "tasks[0].name is required")
	})

	t.Run("task missing pool", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tasks = []Task{{Name: "t", Dataset: "d"}}
		assert.ErrorContains(t, cfg.Validate(), "tasks[0].pool is required")
	})

	t.Run("task missing dataset", func(t *testing.T) {
		cfg := validConfig()
		cfg.Tasks = []Task{{Name: "t", Pool: "p"}}
		assert.ErrorContains(t, cfg.Validate(), "tasks[0].dataset is required")
	})

	t.Run("s3 enabled without bucket", func(t *testing.T) {
		cfg := validConfig()
		cfg.S3.Enabled = true
		cfg.S3.Region = "us-east-1"
		cfg.S3.StorageClass.BackupData = []types.StorageClass{"STANDARD"}
		assert.ErrorContains(t, cfg.Validate(), "s3.bucket is required")
	})

	t.Run("s3 enabled without region", func(t *testing.T) {
		cfg := validConfig()
		cfg.S3.Enabled = true
		cfg.S3.Bucket = "my-bucket"
		cfg.S3.StorageClass.BackupData = []types.StorageClass{"STANDARD"}
		assert.ErrorContains(t, cfg.Validate(), "s3.region is required")
	})

	t.Run("s3 enabled without storage classes", func(t *testing.T) {
		cfg := validConfig()
		cfg.S3.Enabled = true
		cfg.S3.Bucket = "my-bucket"
		cfg.S3.Region = "us-east-1"
		assert.ErrorContains(t, cfg.Validate(), "s3.storage_class.backup_data")
	})

	t.Run("valid s3 config", func(t *testing.T) {
		cfg := validConfig()
		cfg.S3.Enabled = true
		cfg.S3.Bucket = "my-bucket"
		cfg.S3.Region = "us-east-1"
		cfg.S3.StorageClass.BackupData = []types.StorageClass{"STANDARD"}
		require.NoError(t, cfg.Validate())
	})
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
