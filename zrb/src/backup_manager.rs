use anyhow::{Error, anyhow};
use chrono::{DateTime, Utc};
use mockall::automock;
use std::{
    os::linux::raw::stat,
    path::{Path, PathBuf},
};

use crate::{
    compression::manager::Compressor,
    encryption::manager::Encryptor,
    hash::manager::Hasher,
    remote::manager::RemoteManager,
    snapshot::manager::SnapshotManager,
    status::manager::{FileIo, StatusManager},
    status::model::*,
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

    pub fn run(&mut self, _auto: bool) -> Result<(), Error> {
        let (stage, _total, current) = self.status_mgr.restore_status()?;

        match stage {
            BackupTaskStage::SnapshotExport => {
                self.handle_snapshot_export()?;
            }
            BackupTaskStage::SnapshotTest => {
                self.handle_snapshot_test()?;
            }
            BackupTaskStage::Split => {
                self.handle_split(current)?;
            }
            BackupTaskStage::Compress => {
                self.handle_compress(current)?;
            }
            BackupTaskStage::Encrypt => {
                self.handle_encrypt(current)?;
            }
            BackupTaskStage::Upload => {
                self.handle_upload(current)?;
            }
            BackupTaskStage::Cleanup => {
                self.handle_cleanup(current)?;
            }
            BackupTaskStage::Verify => {
                self.handle_verify()?;
            }
            BackupTaskStage::Done => {
                self.handle_done()?;
            }
        }

        Ok(())
    }

    fn handle_snapshot_export(&mut self) -> Result<(), Error> {
        // let _ = self.snapshot_mgr.export(out_dir, dataset, base_snapshot, ref_snapshot);
        todo!()
    }

    fn handle_snapshot_test(&mut self) -> Result<(), Error> {
        todo!()
    }

    fn handle_split(&mut self, current: u64) -> Result<(), Error> {
        todo!()
    }

    fn handle_compress(&mut self, current: u64) -> Result<(), Error> {
        todo!()
    }

    fn handle_encrypt(&mut self, current: u64) -> Result<(), Error> {
        todo!()
    }

    fn handle_upload(&mut self, current: u64) -> Result<(), Error> {
        todo!()
    }

    fn handle_cleanup(&mut self, current: u64) -> Result<(), Error> {
        todo!()
    }

    fn handle_verify(&mut self) -> Result<(), Error> {
        todo!()
    }

    fn handle_done(&mut self) -> Result<(), Error> {
        todo!()
    }

    fn get_dataset(&self) -> Result<String, Error> {
        let target = self.status_mgr.get_target_queue().front().unwrap();
        Ok(target.dataset.clone())
    }

    fn get_backup_type(&self) -> Result<BackupType, Error> {
        let target = self.status_mgr.get_target_queue().front().unwrap();
        Ok(target.backup_type.clone())
    }

    fn get_date(&self) -> Result<DateTime<Utc>, Error> {
        let target = self.status_mgr.get_target_queue().front().unwrap();
        Ok(target.date.clone())
    }

    fn get_base_snapshot(&self) -> Result<String, Error> {
        let task = self.status_mgr.get_active_task();
        Ok(task.base_snapshot.clone())
    }

    fn get_ref_snapshot(&self) -> Result<String, Error> {
        let task = self.status_mgr.get_active_task();
        Ok(task.ref_snapshot.clone())
    }

    fn get_temp_dir(&self) -> PathBuf {
        let dataset = self.get_dataset().unwrap_or("unknown_dataset".to_string());
        let backup_type = self
            .get_backup_type()
            .unwrap_or(BackupType::Full)
            .to_string();
        let date = self
            .get_date()
            .unwrap_or_else(|_| Utc::now())
            .format("%Y-%m-%d")
            .to_string();

        let mut path = PathBuf::from("/tmp/");
        path.push(dataset);
        path.push(format!("{}_{}", backup_type, date));
        path
    }
}
