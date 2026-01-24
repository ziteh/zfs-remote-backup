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

type StorageClassSet struct {
	FullBackup types.StorageClass `yaml:"full_backup"`
	DiffBackup types.StorageClass `yaml:"diff_backup"`
	IncrBackup types.StorageClass `yaml:"incr_backup"`
	Manifest   types.StorageClass `yaml:"manifest"`
}

type S3Config struct {
	Enabled      bool            `yaml:"enabled"`
	Bucket       string          `yaml:"bucket"`
	Prefix       string          `yaml:"prefix"`
	Region       string          `yaml:"region"`
	Endpoint     string          `yaml:"endpoint"` // For S3 compatible services
	StorageClass StorageClassSet `yaml:"storage_class"`
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
	BackupType     string     `yaml:"backup_type"`
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
	Pool    string     `yaml:"pool"`
	Dataset string     `yaml:"dataset"`
	Full    *BackupRef `yaml:"full,omitempty"`
	Diff    *BackupRef `yaml:"diff,omitempty"`
	Incr    *BackupRef `yaml:"incr,omitempty"`
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
