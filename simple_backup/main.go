package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"filippo.io/age"
	"github.com/zeebo/blake3"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Pool             string `yaml:"pool"`
	Dataset          string `yaml:"dataset"`
	BaseSnapshotName string `yaml:"base_snapshot_name"`
	AgePublicKey     string `yaml:"age_public_key"`
	ExportDir        string `yaml:"export_dir"`
}

type PartInfo struct {
	Index      string `yaml:"index"`
	SHA256Hash string `yaml:"sha256_hash"`
}

type SystemInfo struct {
	OS         string `yaml:"os"`
	ZFSVersion struct {
		Userland string `yaml:"userland"`
		Kernel   string `yaml:"kernel"`
	} `yaml:"zfs_version"`
}

type BackupManifest struct {
	Datetime         time.Time  `yaml:"datetime"`
	System           SystemInfo `yaml:"system"`
	Pool             string     `yaml:"pool"`
	Dataset          string     `yaml:"dataset"`
	BaseSnapshotName string     `yaml:"base_snapshot_name"`
	AgePublicKey     string     `yaml:"age_public_key"`
	Blake3Hash       string     `yaml:"blake3_hash"`
	Parts            []PartInfo `yaml:"parts"`
}

func main() {
	config, err := loadConfig("zrb_simple_config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	outputDir := filepath.Join(config.ExportDir, config.Pool, config.Dataset, config.BaseSnapshotName)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create export directory: %v", err)
	}

	snapshotPath := fmt.Sprintf("%s/%s@%s", config.Pool, config.Dataset, config.BaseSnapshotName)
	blake3Hash, err := runZfsSendAndSplit(snapshotPath, outputDir)
	if err != nil {
		log.Fatalf("Failed to run zfs send and split: %v", err)
	}
	log.Printf("Snapshot BLAKE3: %s", blake3Hash)

	parts, err := filepath.Glob(filepath.Join(outputDir, "snapshot.part-*"))
	if err != nil {
		log.Fatalf("Failed to find snapshot parts: %v", err)
	}

	var rawParts []string
	for _, part := range parts {
		if !strings.HasSuffix(part, ".age") {
			rawParts = append(rawParts, part)
		}
	}
	sort.Strings(rawParts)

	recipient, err := age.ParseX25519Recipient(config.AgePublicKey)
	if err != nil {
		log.Fatalf("Failed to parse age public key: %v", err)
	}

	var partInfos []PartInfo
	for _, partFile := range rawParts {
		sha256Hash, err := processPartFile(partFile, recipient)
		if err != nil {
			log.Fatalf("Failed to process %s: %v", partFile, err)
		}

		baseName := filepath.Base(partFile)
		index := strings.TrimPrefix(baseName, "snapshot.part-")
		partInfos = append(partInfos, PartInfo{
			Index:      index,
			SHA256Hash: sha256Hash,
		})
	}

	systemInfo, err := getSystemInfo()
	if err != nil {
		log.Printf("Warning: Failed to get system info: %v", err)
		systemInfo = SystemInfo{}
		systemInfo.OS = "unknown"
		systemInfo.ZFSVersion.Userland = "unknown"
		systemInfo.ZFSVersion.Kernel = "unknown"
	}

	manifest := BackupManifest{
		Datetime:         time.Now(),
		System:           systemInfo,
		Pool:             config.Pool,
		Dataset:          config.Dataset,
		BaseSnapshotName: config.BaseSnapshotName,
		AgePublicKey:     config.AgePublicKey,
		Blake3Hash:       blake3Hash,
		Parts:            partInfos,
	}

	manifestPath := filepath.Join(outputDir, "backup_manifest.yaml")
	if err := writeManifest(manifestPath, &manifest); err != nil {
		log.Fatalf("Failed to write manifest: %v", err)
	}
	log.Printf("Manifest written to: %s", manifestPath)

	log.Println("All parts processed successfully")
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

func runZfsSendAndSplit(snapshotPath, exportDir string) (string, error) {
	log.Printf("Running: zfs send %s | tee >(blake3) | split -b 3G -a 4 - snapshot.part-", snapshotPath)

	zfsCmd := exec.Command("zfs", "send", snapshotPath)

	outputPattern := filepath.Join(exportDir, "snapshot.part-")
	splitCmd := exec.Command("split", "-b", "3G", "-a", "4", "-", outputPattern)

	zfsPipe, err := zfsCmd.StdoutPipe()
	if err != nil {
		return "", err
	}

	hasher := blake3.New()
	teeReader := io.TeeReader(zfsPipe, hasher)
	splitCmd.Stdin = teeReader

	zfsCmd.Stderr = os.Stderr
	splitCmd.Stderr = os.Stderr

	if err := splitCmd.Start(); err != nil {
		return "", err
	}
	if err := zfsCmd.Start(); err != nil {
		return "", err
	}

	if err := zfsCmd.Wait(); err != nil {
		return "", err
	}
	if err := splitCmd.Wait(); err != nil {
		return "", err
	}

	blake3Hash := fmt.Sprintf("%x", hasher.Sum(nil))
	log.Println("ZFS send and split completed")
	return blake3Hash, nil
}

func processPartFile(partFile string, recipient age.Recipient) (string, error) {
	log.Printf("Processing %s...", partFile)

	// Age encryption
	encryptedFile := partFile + ".age"
	if err := encryptWithAge(partFile, encryptedFile, recipient); err != nil {
		return "", fmt.Errorf("age encryption failed: %w", err)
	}
	log.Printf("  Encrypted to: %s", encryptedFile)

	// SHA-256 hash
	sha256Hash, err := calculateSHA256(encryptedFile)
	if err != nil {
		return "", fmt.Errorf("SHA-256 hash failed: %w", err)
	}
	log.Printf("  SHA-256: %s", sha256Hash)

	// Delete original file
	if err := os.Remove(partFile); err != nil {
		return "", fmt.Errorf("failed to remove original file: %w", err)
	}
	log.Printf("  Removed original file: %s", partFile)

	return sha256Hash, nil
}

func calculateBLAKE3(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func encryptWithAge(inputFile, outputFile string, recipient age.Recipient) error {
	in, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	w, err := age.Encrypt(out, recipient)
	if err != nil {
		return err
	}

	if _, err := io.Copy(w, in); err != nil {
		return err
	}

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}

func calculateSHA256(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func getSystemInfo() (SystemInfo, error) {
	osVersion := "unknown"
	if data, err := os.ReadFile("/etc/version"); err == nil {
		osVersion = strings.TrimSpace(string(data))
	}

	zfsVersionCmd := exec.Command("zfs", "version", "-j")
	zfsVersionOutput, err := zfsVersionCmd.Output()
	if err != nil {
		return SystemInfo{}, err
	}

	var result struct {
		ZFSVersion struct {
			Userland string `json:"userland"`
			Kernel   string `json:"kernel"`
		} `json:"zfs_version"`
	}

	if err := json.Unmarshal(zfsVersionOutput, &result); err != nil {
		return SystemInfo{}, err
	}

	var systemInfo SystemInfo
	systemInfo.OS = osVersion
	systemInfo.ZFSVersion.Userland = result.ZFSVersion.Userland
	systemInfo.ZFSVersion.Kernel = result.ZFSVersion.Kernel

	return systemInfo, nil
}

func writeManifest(filename string, manifest *BackupManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}
