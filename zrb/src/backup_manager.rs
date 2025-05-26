use anyhow::{Error, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

use crate::{
    compression::manager::Compressor,
    encryption::manager::Encryptor,
    hash::manager::Hasher,
    remote::manager::RemoteManager,
    snapshot::manager::SnapshotManager,
    status::manager::{FileIo, StatusManager},
};

pub struct BackupManager {
    status_mgr: StatusManager,
    snapshot_mgr: Box<dyn SnapshotManager>,
    remote_mgr: Box<dyn RemoteManager>,
    compressor: Box<dyn Compressor>,
    encryptor: Box<dyn Encryptor>,
    hasher: Box<dyn Hasher>,
}

impl BackupManager {
    pub fn new(
        file_io: Box<dyn FileIo>,
        snapshot_mgr: Box<dyn SnapshotManager>,
        remote_mgr: Box<dyn RemoteManager>,
        compressor: Box<dyn Compressor>,
        encryptor: Box<dyn Encryptor>,
        hasher: Box<dyn Hasher>,
    ) -> Result<Self, Error> {
        Ok(Self {
            status_mgr: StatusManager::new(file_io)?,
            snapshot_mgr,
            remote_mgr,
            compressor,
            encryptor,
            hasher,
        })
    }
}
