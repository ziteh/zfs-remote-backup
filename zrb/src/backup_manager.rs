use anyhow::{Error, Ok, anyhow};
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
    splitter::manager::Splitter,
    status::manager::{FileIo, StatusManager},
    status::model::*,
};

pub struct BackupManager {
    status_mgr: StatusManager,
    snapshot_mgr: Box<dyn SnapshotManager>,
    remote_mgr: Box<dyn RemoteManager>,
    splitter: Box<dyn Splitter>,
    compressor: Box<dyn Compressor>,
    encryptor: Box<dyn Encryptor>,
    hasher: Box<dyn Hasher>,
}

impl BackupManager {
    pub fn new(
        file_io: Box<dyn FileIo>,
        snapshot_mgr: Box<dyn SnapshotManager>,
        remote_mgr: Box<dyn RemoteManager>,
        splitter: Box<dyn Splitter>,
        compressor: Box<dyn Compressor>,
        encryptor: Box<dyn Encryptor>,
        hasher: Box<dyn Hasher>,
    ) -> Result<Self, Error> {
        Ok(Self {
            status_mgr: StatusManager::new(file_io)?,
            snapshot_mgr,
            remote_mgr,
            splitter,
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
        let out_dir = self.get_temp_dir();
        let dataset = self.get_dataset()?;
        let base_snapshot = self.get_base_snapshot()?;
        let ref_snapshot = self.get_ref_snapshot()?;

        let filename =
            self.snapshot_mgr
                .export(out_dir.as_path(), &dataset, &base_snapshot, &ref_snapshot)?;

        self.status_mgr.update_stage_status_snapshot_exported(
            filename
                .to_str()
                .ok_or_else(|| anyhow!("Path contains invalid Unicode"))?
                .to_string(),
        )?;
        Ok(())
    }

    fn handle_snapshot_test(&mut self) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }

        let base_snapshot = self.get_base_snapshot()?;
        let filename = Path::new(&snapshot_filename);
        self.snapshot_mgr.verify(&base_snapshot, &filename)?;

        self.hasher.reset();
        self.hasher.cal_file(&filename)?;
        let hash = self.hasher.get_digest();
        self.status_mgr.update_full_hash(hash.clone())?;

        self.status_mgr.update_stage_status_snapshot_tested(true)?;

        Ok(())
    }

    fn handle_split(&mut self, current: u64) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }

        let filename = self
            .splitter
            .split(Path::new(&snapshot_filename), current)?;
        self.hasher.reset();
        self.hasher.cal_file(Path::new(&filename))?;
        let hash = self.hasher.get_digest();
        self.status_mgr
            .update_stage_status_split_hashes(hash.clone())?;

        Ok(())
    }

    fn handle_compress(&mut self, current: u64) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }
        let extension = self.splitter.get_extension(current);
        let filepath = Path::new(&snapshot_filename).with_extension(extension);
        let out_filepath = self.compressor.compress(&filepath)?;
        self.compressor.verify(&out_filepath)?;
        self.status_mgr.update_stage_status_compressed(current)?;
        Ok(())
    }

    fn handle_encrypt(&mut self, current: u64) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }
        let extension = self.compressor.get_extension();
        let filepath = Path::new(&snapshot_filename).with_extension(extension);

        self.hasher.reset();
        self.hasher.cal_file(&filepath)?;
        let ori_hash = self.hasher.get_digest();
        let out_filepath = self.encryptor.encrypt(&filepath)?;
        let dec_filepath = self.encryptor.decrypt(&out_filepath)?;
        self.hasher.reset();
        self.hasher.cal_file(&dec_filepath)?;
        let dec_hash = self.hasher.get_digest();

        if ori_hash != dec_hash {
            return Err(anyhow!("Decrypted file hash does not match original"));
        }
        self.status_mgr.update_stage_status_encrypted(current)?;
        Ok(())
    }

    fn handle_upload(&mut self, current: u64) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }
        let extension = self.encryptor.get_extension();
        let filepath = Path::new(&snapshot_filename).with_extension(extension);

        // TODO dst_filepath, tags, metadata
        self.remote_mgr.upload(&filepath, &filepath, None, None)?;
        self.status_mgr.update_stage_status_uploaded(current)?;

        Ok(())
    }

    fn handle_cleanup(&mut self, current: u64) -> Result<(), Error> {
        let snapshot_filename = self
            .status_mgr
            .get_active_task()
            .progress
            .snapshot_exported_name
            .clone();
        if snapshot_filename.is_empty() {
            return Err(anyhow!("Snapshot not exported yet"));
        }
        let extension = self.encryptor.get_extension();
        let filepath = Path::new(&snapshot_filename).with_extension(extension);

        todo!("Remove file: {}", filepath.display());

        self.status_mgr.update_stage_status_cleanup(current)?;
        Ok(())
    }

    fn handle_verify(&mut self) -> Result<(), Error> {
        let full_hash = self.status_mgr.get_active_task().full_hash.clone();
        let stream_hash = self
            .status_mgr
            .get_active_task()
            .progress
            .split_hashes
            .last()
            .cloned();

        if let Some(stream_hash) = stream_hash {
            if full_hash != stream_hash {
                return Err(anyhow!("Full hash does not match stream hash"));
            }
        } else {
            return Err(anyhow!("No stream hash found for verification"));
        }

        self.status_mgr.update_stage_status_verified(true)?;
        Ok(())
    }

    fn handle_done(&mut self) -> Result<(), Error> {
        Ok(())
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
