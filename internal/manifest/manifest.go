package manifest

import (
	"encoding/json"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

func GetSystemInfo() (SystemInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

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

	var info SystemInfo
	info.Hostname = hostname
	info.OS = osVersion
	info.ZFSVersion.Userland = result.ZFSVersion.Userland
	info.ZFSVersion.Kernel = result.ZFSVersion.Kernel

	return info, nil
}

func Write(filename string, m *Backup) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

func Read(filename string) (*Backup, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var m Backup
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func WriteLast(filename string, last *Last) error {
	data, err := yaml.Marshal(last)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

func ReadLast(filename string) (*Last, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var last Last
	if err := yaml.Unmarshal(data, &last); err != nil {
		return nil, err
	}
	return &last, nil
}

func WriteState(filename string, state *State) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}

func ReadState(filename string) (*State, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	var state State
	if err := yaml.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}
