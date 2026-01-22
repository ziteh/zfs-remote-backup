package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// getSystemInfo retrieves the OS and ZFS version information
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

// writeManifest writes the backup manifest to a YAML file
func writeManifest(filename string, manifest *BackupManifest) error {
	data, err := yaml.Marshal(manifest)
	if err != nil {
		return err
	}

	return os.WriteFile(filename, data, 0644)
}
