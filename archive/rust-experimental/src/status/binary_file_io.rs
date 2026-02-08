use anyhow::{Context, Error, Result};
use std::fs::{create_dir_all, File};
use std::io::{BufReader, BufWriter};
use std::path::{Path, PathBuf};

use super::manager::FileIo;
use crate::status::model::*;

pub struct BinaryFileIo {
    storage_dir: PathBuf,
}

impl BinaryFileIo {
    pub fn new<P: AsRef<Path>>(storage_dir: P) -> Result<Self, Error> {
        let storage_dir = storage_dir.as_ref().to_path_buf();

        if !storage_dir.exists() {
            create_dir_all(&storage_dir).with_context(|| {
                format!(
                    "Failed to create storage directory: {}",
                    storage_dir.display()
                )
            })?;
        }

        Ok(BinaryFileIo { storage_dir })
    }

    fn target_queue_path(&self) -> PathBuf {
        self.storage_dir.join("target_queue.bin")
    }

    fn active_tasks_path(&self) -> PathBuf {
        self.storage_dir.join("active_tasks.bin")
    }

    fn latest_snapshot_map_path(&self) -> PathBuf {
        self.storage_dir.join("latest_snapshot_map.bin")
    }

    fn load_from_file<T>(&self, file_path: &Path) -> Result<T, Error>
    where
        T: serde::de::DeserializeOwned + Default,
    {
        if !file_path.exists() {
            return Ok(T::default());
        }

        let file = File::open(file_path)
            .with_context(|| format!("Failed to open file: {}", file_path.display()))?;
        let reader = BufReader::new(file);

        bincode::deserialize_from(reader)
            .with_context(|| format!("Failed to deserialize data from file: {}", file_path.display()))
    }

    fn save_to_file<T>(&self, file_path: &Path, data: &T) -> Result<(), Error>
    where
        T: serde::Serialize,
    {
        let file = File::create(file_path)
            .with_context(|| format!("Failed to create file: {}", file_path.display()))?;
        let writer = BufWriter::new(file);

        bincode::serialize_into(writer, data)
            .with_context(|| format!("Failed to serialize data to file: {}", file_path.display()))
    }
}

impl FileIo for BinaryFileIo {
    fn load_target_queue(&self) -> Result<BackupTargetQueue, Error> {
        let path = self.target_queue_path();
        self.load_from_file(&path)
    }

    fn load_active_tasks(&self) -> Result<ActiveBackupTask, Error> {
        let path = self.active_tasks_path();
        self.load_from_file(&path)
    }

    fn load_latest_snapshot_map(&self) -> Result<LatestSnapshotMap, Error> {
        let path = self.latest_snapshot_map_path();
        self.load_from_file(&path)
    }

    fn save_target_queue(&self, queue: &BackupTargetQueue) -> Result<(), Error> {
        let path = self.target_queue_path();
        self.save_to_file(&path, queue)
    }

    fn save_active_tasks(&self, task: &ActiveBackupTask) -> Result<(), Error> {
        let path = self.active_tasks_path();
        self.save_to_file(&path, task)
    }

    fn save_latest_snapshot_map(&self, map: &LatestSnapshotMap) -> Result<(), Error> {
        let path = self.latest_snapshot_map_path();
        self.save_to_file(&path, map)
    }
}
