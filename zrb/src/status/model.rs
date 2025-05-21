use chrono::{DateTime, Utc};
use std::collections::VecDeque;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BackupType {
    /// Full backup
    Full,

    /// Differential backup
    Diff,

    /// Incremental backup
    Incr,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum BackupTaskStage {
    SnapshotExport,
    SnapshotTest,
    Split,
    Compress,
    Encrypt,
    Upload,
    Cleanup,
    Verify,
    Done,
    // Error,
}

pub type Hash = Vec<u8>;

#[derive(Debug, Clone, Default)]
pub struct BackupStageStatus {
    /// Exporting snapshot name, empty if not exporting
    pub snapshot_exported_name: String,

    /// Snapshot test status
    pub snapshot_tested: bool,

    /// List of hashes for each split, empty if not split
    pub split_hashes: Vec<Hash>,

    /// Number of compressed files
    pub compressed: u64,

    /// Number of encrypted files
    pub encrypted: u64,

    /// Number of uploaded files
    pub uploaded: u64,

    /// Number of cleanup files
    pub cleanup: u64,

    /// Verification status
    pub verified: bool,
}

#[derive(Debug, Clone, Default)]
pub struct ActiveBackupTask {
    pub base_snapshot: String,
    pub ref_snapshot: String,
    pub split_qty: u64,
    pub progress: BackupStageStatus,
    pub full_hash: Hash,
}

#[derive(Debug, Clone, Default)]
pub struct LatestSnapshotInfo {
    /// Latest snapshot date
    pub update: DateTime<Utc>,

    // ZFS dataset snapshot name
    pub snapshot: String,
}

/// Dataset name -> BackupType -> LatestSnapshotInfo
pub type LatestSnapshotMap = HashMap<String, HashMap<BackupType, LatestSnapshotInfo>>;

#[derive(Debug, Clone, Default)]
pub struct BackupTarget {
    /// Target date
    pub date: DateTime<Utc>,

    /// Backup type
    pub backup_type: BackupType,

    /// ZFS dataset name
    pub dataset: String,
}

pub type BackupTargetQueue = VecDeque<BackupTarget>;
