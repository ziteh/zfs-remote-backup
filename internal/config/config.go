package config

import (
	"fmt"
	"os"

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

	return &cfg, nil
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
