package main

import (
	"os"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"gopkg.in/yaml.v3"
)

type BackupTask struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description,omitempty"`
	Pool        string `yaml:"pool"`
	Dataset     string `yaml:"dataset"`
	Enabled     bool   `yaml:"enabled"`
}

type Config struct {
	BaseDir      string       `yaml:"base_dir"`
	AgePublicKey string       `yaml:"age_public_key"`
	S3           S3Config     `yaml:"s3"`
	Tasks        []BackupTask `yaml:"tasks"`
}

type S3Config struct {
	Enabled      bool   `yaml:"enabled"`
	Bucket       string `yaml:"bucket"`
	Prefix       string `yaml:"prefix"`
	Region       string `yaml:"region"`
	Endpoint     string `yaml:"endpoint"` // For S3 compatible services
	StorageClass struct {
		BackupData []types.StorageClass `yaml:"backup_data"` // By level
		Manifest   types.StorageClass   `yaml:"manifest"`
	} `yaml:"storage_class"`
	Retry struct {
		MaxAttempts int `yaml:"max_attempts"` // Maximum number of attempts (including initial request)
	} `yaml:"retry,omitempty"`
}

type PartInfo struct {
	Index      string `yaml:"index"`
	SHA256Hash string `yaml:"sha256_hash"`
}

type SystemInfo struct {
	Hostname   string `yaml:"hostname"`
	OS         string `yaml:"os"`
	ZFSVersion struct {
		Userland string `yaml:"userland"`
		Kernel   string `yaml:"kernel"`
	} `yaml:"zfs_version"`
}

type BackupManifest struct {
	Datetime       int64      `yaml:"datetime"`
	System         SystemInfo `yaml:"system"`
	Pool           string     `yaml:"pool"`
	Dataset        string     `yaml:"dataset"`
	BackupLevel    int16      `yaml:"backup_level"`
	TargetSnapshot string     `yaml:"target_snapshot"`
	ParentSnapshot string     `yaml:"parent_snapshot"`
	AgePublicKey   string     `yaml:"age_public_key"`
	Blake3Hash     string     `yaml:"blake3_hash"`
	Parts          []PartInfo `yaml:"parts"`
	TargetS3Path   string     `yaml:"target_s3_path"`
	ParentS3Path   string     `yaml:"parent_s3_path"`
}

type BackupRef struct {
	Datetime   int64  `yaml:"datetime"`
	Snapshot   string `yaml:"snapshot"`
	Manifest   string `yaml:"manifest"`
	Blake3Hash string `yaml:"blake3_hash"`
	S3Path     string `yaml:"s3_path"`
}

type LastBackup struct {
	Pool         string       `yaml:"pool"`
	Dataset      string       `yaml:"dataset"`
	BackupLevels []*BackupRef `yaml:"backup_levels"`
}

type BackupState struct {
	TaskName         string          `yaml:"task_name"`
	BackupLevel      int16           `yaml:"backup_level"`
	TargetSnapshot   string          `yaml:"target_snapshot"`
	ParentSnapshot   string          `yaml:"parent_snapshot"`
	OutputDir        string          `yaml:"output_dir"`
	Blake3Hash       string          `yaml:"blake3_hash"`
	PartsProcessed   map[string]bool `yaml:"parts_processed"` // part index -> processed
	PartsUploaded    map[string]bool `yaml:"parts_uploaded"`  // part index -> uploaded
	ManifestCreated  bool            `yaml:"manifest_created"`
	ManifestUploaded bool            `yaml:"manifest_uploaded"`
	LastUpdated      int64           `yaml:"last_updated"`
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}
