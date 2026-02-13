package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v3"
)

type Task struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Pool        string `yaml:"pool"`
	Dataset     string `yaml:"dataset"`
	Enabled     bool   `yaml:"enabled"`
}

type Config struct {
	BaseDir      string   `yaml:"base_dir"`
	AgePublicKey string   `yaml:"age_public_key"`
	S3           S3Config `yaml:"s3"`
	Tasks        []Task   `yaml:"tasks"`
}

type S3Config struct {
	Enabled      bool   `yaml:"enabled"`
	Bucket       string `yaml:"bucket"`
	Prefix       string `yaml:"prefix"`
	Region       string `yaml:"region"`
	Endpoint     string `yaml:"endpoint"`
	StorageClass struct {
		BackupData []types.StorageClass `yaml:"backup_data"`
		Manifest   types.StorageClass   `yaml:"manifest"`
	} `yaml:"storage_class"`
	Retry struct {
		MaxAttempts int `yaml:"max_attempts"`
	} `yaml:"retry,omitempty"`
}

func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &cfg, nil
}

func (c *Config) Validate() error {
	if c.BaseDir == "" {
		return fmt.Errorf("base_dir is required")
	}
	if c.AgePublicKey == "" {
		return fmt.Errorf("age_public_key is required")
	}
	if !strings.HasPrefix(c.AgePublicKey, "age1") {
		return fmt.Errorf("age_public_key must start with 'age1'")
	}
	if len(c.Tasks) == 0 {
		return fmt.Errorf("at least one task is required")
	}
	for i, t := range c.Tasks {
		if t.Name == "" {
			return fmt.Errorf("tasks[%d].name is required", i)
		}
		if t.Pool == "" {
			return fmt.Errorf("tasks[%d].pool is required", i)
		}
		if t.Dataset == "" {
			return fmt.Errorf("tasks[%d].dataset is required", i)
		}
	}
	if c.S3.Enabled {
		if c.S3.Bucket == "" {
			return fmt.Errorf("s3.bucket is required when s3 is enabled")
		}
		if c.S3.Region == "" {
			return fmt.Errorf("s3.region is required when s3 is enabled")
		}
		if len(c.S3.StorageClass.BackupData) == 0 {
			return fmt.Errorf("s3.storage_class.backup_data must have at least one entry")
		}
	}
	return nil
}

func (c *Config) FindTask(name string) (*Task, error) {
	for _, t := range c.Tasks {
		if t.Name == name {
			return &t, nil
		}
	}
	return nil, fmt.Errorf("task not found: %s", name)
}

func (c *Config) S3RetryAttempts() int {
	if c.S3.Retry.MaxAttempts > 0 {
		return c.S3.Retry.MaxAttempts
	}
	return 3
}
