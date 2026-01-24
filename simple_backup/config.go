package main

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Pool         string   `yaml:"pool"`
	Dataset      string   `yaml:"dataset"`
	AgePublicKey string   `yaml:"age_public_key"`
	ExportDir    string   `yaml:"export_dir"`
	S3           S3Config `yaml:"s3"`
}

type S3Config struct {
	Enabled      bool   `yaml:"enabled"`
	Bucket       string `yaml:"bucket"`
	Region       string `yaml:"region"`
	Prefix       string `yaml:"prefix"`
	Endpoint     string `yaml:"endpoint"`      // For S3 compatible services
	StorageClass string `yaml:"storage_class"` // S3 storage class (STANDARD, GLACIER, DEEP_ARCHIVE, etc.)
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
