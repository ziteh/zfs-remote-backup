use anyhow::{Error, Ok, anyhow};

use mockall::{automock, predicate::*};

use crate::*;

#[automock]
pub trait FileIo {
    fn load_target_queue(&self) -> Result<BackupTargetQueue, Error>;
    fn load_active_tasks(&self) -> Result<ActiveBackupTask, Error>;
    fn load_latest_snapshot_map(&self) -> Result<LatestSnapshotMap, Error>;

    fn save_target_queue(&self, queue: &BackupTargetQueue) -> Result<(), Error>;
    fn save_active_tasks(&self, task: &ActiveBackupTask) -> Result<(), Error>;
    fn save_latest_snapshot_map(&self, map: &LatestSnapshotMap) -> Result<(), Error>;
}

pub struct StatusManager {
    io: Box<dyn FileIo>,
    target_queue: BackupTargetQueue,
    active_tasks: ActiveBackupTask,
    latest_snapshot_map: LatestSnapshotMap,
}

impl StatusManager {
    pub fn new(io: Box<dyn FileIo>) -> Result<Self, Error> {
        let target_queue = io.load_target_queue()?;
        let active_tasks = io.load_active_tasks()?;
        let latest_snapshot_map = io.load_latest_snapshot_map()?;

        Ok(StatusManager {
            io,
            target_queue,
            active_tasks,
            latest_snapshot_map,
        })
    }

    pub fn get_target_queue(&self) -> &BackupTargetQueue {
        &self.target_queue
    }

    pub fn get_active_task(&self) -> &ActiveBackupTask {
        &self.active_tasks
    }

    pub fn get_latest_snapshot_map(&self) -> &LatestSnapshotMap {
        &self.latest_snapshot_map
    }

    pub fn enqueue_target(&mut self, target: BackupTarget) -> Result<(), Error> {
        self.target_queue.push_back(target);
        self.io.save_target_queue(&self.target_queue)
    }

    pub fn dequeue_target(&mut self) -> Result<BackupTarget, Error> {
        if let Some(target) = self.target_queue.pop_front() {
            self.io.save_target_queue(&self.target_queue)?;
            Ok(target)
        } else {
            Err(anyhow!("Empty queue"))
        }
    }

    pub fn restore_status(&mut self) -> Result<(BackupTaskStage, u64, u64), Error> {
        self.target_queue = self.io.load_target_queue()?;
        if self.target_queue.is_empty() {
            return Ok((BackupTaskStage::Done, 0, 0));
        }

        self.active_tasks = self.io.load_active_tasks()?;
        let stage = &self.active_tasks.progress;

        if stage.snapshot_exported_name.is_empty() {
            return Ok((BackupTaskStage::SnapshotExport, 0, 0));
        }

        if !stage.snapshot_tested {
            return Ok((BackupTaskStage::SnapshotTest, 0, 0));
        }

        let split_count = stage.split_hashes.len() as u64;
        let total_split_qty = self.active_tasks.split_qty;

        if split_count > total_split_qty {
            return Err(anyhow!("split"));
        } else if split_count == 0 {
            return Ok((BackupTaskStage::Split, total_split_qty, 0));
        }

        // check if any stage is not completed
        let check_stage = |stage: BackupTaskStage, act: u64| {
            if act < split_count {
                let res = Ok((stage, split_count, act));
                return Some(res);
            } else if act > split_count {
                let res = Err(anyhow!("Error stage {:?}", stage));
                return Some(res);
            }
            None
        };

        if let Some(res) = check_stage(BackupTaskStage::Compress, stage.compressed) {
            return res;
        }

        if let Some(res) = check_stage(BackupTaskStage::Encrypt, stage.encrypted) {
            return res;
        }

        if let Some(res) = check_stage(BackupTaskStage::Upload, stage.uploaded) {
            return res;
        }

        if let Some(res) = check_stage(BackupTaskStage::Cleanup, stage.cleanup) {
            return res;
        }

        // check if all stages are completed
        if split_count == total_split_qty {
            if stage.verified {
                return Ok((BackupTaskStage::Done, 0, 0));
            } else {
                return Ok((BackupTaskStage::Verify, split_count, 0));
            }
        }

        Ok((BackupTaskStage::Split, total_split_qty, split_count))
    }

    pub fn update_full_hash(&mut self, hash: Hash) -> Result<(), Error> {
        self.active_tasks.full_hash = hash;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_snapshot_exported(&mut self, filename: String) -> Result<(), Error> {
        self.active_tasks.progress.snapshot_exported_name = filename;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_snapshot_tested(&mut self, status: bool) -> Result<(), Error> {
        self.active_tasks.progress.snapshot_tested = status;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_split_hashes(&mut self, hash: Hash) -> Result<(), Error> {
        self.active_tasks.progress.split_hashes.push(hash);
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_compressed(&mut self, count: u64) -> Result<(), Error> {
        self.active_tasks.progress.compressed = count;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_encrypted(&mut self, count: u64) -> Result<(), Error> {
        self.active_tasks.progress.encrypted = count;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_uploaded(&mut self, count: u64) -> Result<(), Error> {
        self.active_tasks.progress.uploaded = count;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_cleanup(&mut self, count: u64) -> Result<(), Error> {
        self.active_tasks.progress.cleanup = count;
        self.io.save_active_tasks(&self.active_tasks)
    }

    pub fn update_stage_status_verified(&mut self, status: bool) -> Result<(), Error> {
        self.active_tasks.progress.verified = status;
        self.io.save_active_tasks(&self.active_tasks)
    }
}
