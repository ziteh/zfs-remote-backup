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
}

type BackupRef struct {
	Datetime   int64  `yaml:"datetime"`
	Snapshot   string `yaml:"snapshot"`
	Manifest   string `yaml:"manifest"`
	Blake3Hash string `yaml:"blake3_hash"`
}

type LastBackup struct {
	Pool         string       `yaml:"pool"`
	Dataset      string       `yaml:"dataset"`
	BackupLevels []*BackupRef `yaml:"backup_levels"`
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
