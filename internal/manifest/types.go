package manifest

type PartInfo struct {
	Index      string `yaml:"index"`
	Blake3Hash string `yaml:"blake3_hash"`
}

type SystemInfo struct {
	Hostname   string `yaml:"hostname"`
	OS         string `yaml:"os"`
	ZFSVersion struct {
		Userland string `yaml:"userland"`
		Kernel   string `yaml:"kernel"`
	} `yaml:"zfs_version"`
}

type Backup struct {
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

type Ref struct {
	Datetime   int64  `yaml:"datetime"`
	Snapshot   string `yaml:"snapshot"`
	Manifest   string `yaml:"manifest"`
	Blake3Hash string `yaml:"blake3_hash"`
	S3Path     string `yaml:"s3_path"`
}

type Last struct {
	Pool         string `yaml:"pool"`
	Dataset      string `yaml:"dataset"`
	BackupLevels []*Ref `yaml:"backup_levels"`
}

type State struct {
	TaskName         string          `yaml:"task_name"`
	BackupLevel      int16           `yaml:"backup_level"`
	TargetSnapshot   string          `yaml:"target_snapshot"`
	ParentSnapshot   string          `yaml:"parent_snapshot"`
	OutputDir        string          `yaml:"output_dir"`
	Blake3Hash       string          `yaml:"blake3_hash"`
	PartsProcessed   map[string]bool `yaml:"parts_processed"`
	PartsUploaded    map[string]bool `yaml:"parts_uploaded"`
	ManifestCreated  bool            `yaml:"manifest_created"`
	ManifestUploaded bool            `yaml:"manifest_uploaded"`
	LastUpdated      int64           `yaml:"last_updated"`
}
