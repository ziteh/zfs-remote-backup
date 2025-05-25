use anyhow::{Error, Ok, anyhow};

use mockall::{automock, predicate::*};

use crate::*;

#[automock]
trait FileIo {
    fn load_target_queue(&self) -> Result<BackupTargetQueue, Error>;
    fn load_active_tasks(&self) -> Result<ActiveBackupTask, Error>;
    fn load_latest_snapshot_map(&self) -> Result<LatestSnapshotMap, Error>;

    fn save_target_queue(&self, queue: &BackupTargetQueue) -> Result<(), Error>;
    fn save_active_tasks(&self, task: &ActiveBackupTask) -> Result<(), Error>;
    fn save_latest_snapshot_map(&self, map: &LatestSnapshotMap) -> Result<(), Error>;
}

struct StatusManager {
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
}

#[cfg(test)]
mod tests {
    use std::collections::VecDeque;

    use super::*;
    use chrono::Utc;

    fn sample_target(dataset: &str) -> BackupTarget {
        BackupTarget {
            date: Utc::now(),
            backup_type: BackupType::Full,
            dataset: dataset.to_string(),
        }
    }

    fn setup_restore_status_test(
        total_splits: u64,
        has_queue_items: bool,
        snap_exported: bool,
        snapshot_tested: bool,
        split_done: u64,
        compressed: u64,
        encrypted: u64,
        uploaded: u64,
        cleanup: u64,
        verified: bool,
    ) -> MockFileIo {
        let mut mock_io = MockFileIo::new();

        let mut queue = VecDeque::new();
        if has_queue_items {
            queue.push_back(sample_target("dataset1"));
        }

        let snapshot_exported_name = if snap_exported {
            "snapshot1".to_string()
        } else {
            String::new()
        };

        let split_hashes = (0..split_done)
            .map(|i| vec![i as u8, (i + 1) as u8])
            .collect();

        let progress = BackupStageStatus {
            snapshot_exported_name,
            snapshot_tested,
            split_hashes,
            compressed,
            encrypted,
            uploaded,
            cleanup,
            verified,
        };

        let active = ActiveBackupTask {
            progress,
            split_qty: total_splits,
            base_snapshot: "base".to_string(),
            ref_snapshot: "ref".to_string(),
            full_hash: vec![1, 2, 3],
        };

        mock_io
            .expect_load_target_queue()
            .returning(move || Ok(queue.clone()));

        mock_io
            .expect_load_active_tasks()
            .returning(move || Ok(active.clone()));

        mock_io
            .expect_load_latest_snapshot_map()
            .returning(move || Ok(LatestSnapshotMap::default()));

        mock_io
    }

    #[test]
    fn test_enqueue_and_dequeue() {
        let mut mock_io = MockFileIo::new();

        mock_io
            .expect_load_target_queue()
            .returning(|| Ok(VecDeque::new()));

        mock_io
            .expect_load_active_tasks()
            .returning(|| Ok(ActiveBackupTask::default()));

        mock_io
            .expect_load_latest_snapshot_map()
            .returning(|| Ok(LatestSnapshotMap::default()));

        mock_io
            .expect_save_target_queue()
            .times(4)
            .returning(|_| Ok(()));

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();

        let target1 = sample_target("dataset1");
        manager.enqueue_target(target1.clone()).unwrap();

        let target2 = sample_target("dataset2");
        manager.enqueue_target(target2.clone()).unwrap();

        let out1 = manager.dequeue_target().unwrap();
        assert_eq!(out1.dataset, target1.dataset);

        let out2 = manager.dequeue_target().unwrap();
        assert_eq!(out2.dataset, target2.dataset);

        assert!(manager.dequeue_target().is_err());
    }

    #[test]
    fn test_enqueue_target() {
        let mut mock_io = MockFileIo::new();

        mock_io
            .expect_load_target_queue()
            .returning(|| Ok(VecDeque::new()));
        mock_io
            .expect_load_active_tasks()
            .returning(|| Ok(ActiveBackupTask::default()));
        mock_io
            .expect_load_latest_snapshot_map()
            .returning(|| Ok(LatestSnapshotMap::default()));

        mock_io
            .expect_save_target_queue()
            .withf(|queue| queue.len() == 1 && queue[0].dataset == "dataset1")
            .times(1)
            .returning(|_| Ok(()));

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let target = sample_target("dataset1");

        manager.enqueue_target(target).unwrap();
    }

    #[test]
    fn test_dequeue_target() {
        let target = sample_target("dataset2");

        let mut queue = VecDeque::new();
        queue.push_back(target.clone());

        let mut mock_io = MockFileIo::new();

        mock_io
            .expect_load_target_queue()
            .returning(move || Ok(queue.clone()));
        mock_io
            .expect_load_active_tasks()
            .returning(|| Ok(ActiveBackupTask::default()));
        mock_io
            .expect_load_latest_snapshot_map()
            .returning(|| Ok(LatestSnapshotMap::default()));

        mock_io
            .expect_save_target_queue()
            .withf(|queue| queue.is_empty())
            .times(1)
            .returning(|_| Ok(()));

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();

        let result = manager.dequeue_target().unwrap();
        assert_eq!(result.dataset, target.dataset);
    }

    #[test]
    fn test_restore_status_empty_queue() {
        let mock_io = setup_restore_status_test(0, false, false, false, 0, 0, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Done, 0, 0));
    }

    #[test]
    fn test_restore_status_snapshot_export_stage() {
        let total_splits = 5;
        let mock_io =
            setup_restore_status_test(total_splits, true, false, false, 0, 0, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::SnapshotExport, 0, 0));
    }

    #[test]
    fn test_restore_status_snapshot_test_stage() {
        let total_splits = 5;
        let mock_io =
            setup_restore_status_test(total_splits, true, true, false, 0, 0, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::SnapshotTest, 0, 0));
    }

    #[test]
    fn test_restore_status_split_stage() {
        let total_splits = 5;

        let mock_io =
            setup_restore_status_test(total_splits, true, true, true, 0, 0, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Split, total_splits, 0));
    }

    #[test]
    fn test_restore_status_split_in_progress() {
        let total_splits = 5;
        let progress_qty = total_splits - 2;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            progress_qty,
            progress_qty,
            progress_qty,
            progress_qty,
            progress_qty,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Split, total_splits, progress_qty));
    }

    #[test]
    fn test_restore_status_compression_stage() {
        let total_splits = 5;
        let split_qty = total_splits - 2;
        let compressed_qty = split_qty - 1;
        let progress_qty = compressed_qty - 1;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            split_qty,
            compressed_qty,
            progress_qty,
            progress_qty,
            progress_qty,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(
            result,
            (BackupTaskStage::Compress, split_qty, compressed_qty)
        );
    }

    #[test]
    fn test_restore_status_encryption_stage() {
        let total_splits = 5;
        let split_qty = total_splits - 2;
        let encrypted_qty = split_qty - 1;
        let progress_qty = encrypted_qty - 1;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            split_qty,
            split_qty,
            encrypted_qty,
            progress_qty,
            progress_qty,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Encrypt, split_qty, encrypted_qty));
    }

    #[test]
    fn test_restore_status_upload_stage() {
        let total_splits = 5;
        let split_qty = total_splits - 2;
        let uploaded_qty = split_qty - 1;
        let progress_qty = uploaded_qty - 1;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            split_qty,
            split_qty,
            split_qty,
            uploaded_qty,
            progress_qty,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Upload, split_qty, uploaded_qty));
    }

    #[test]
    fn test_restore_status_cleanup_stage() {
        let total_splits = 5;
        let split_qty = total_splits - 2;
        let cleanup_qty = split_qty - 1;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            split_qty,
            split_qty,
            split_qty,
            split_qty,
            cleanup_qty,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Cleanup, split_qty, cleanup_qty));
    }

    #[test]
    fn test_restore_status_verify_stage() {
        let total_splits = 5;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            total_splits,
            total_splits,
            total_splits,
            total_splits,
            total_splits,
            false,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Verify, total_splits, 0));
    }

    #[test]
    fn test_restore_status_done() {
        let total_splits = 5;

        let mock_io = setup_restore_status_test(
            total_splits,
            true,
            true,
            true,
            total_splits,
            total_splits,
            total_splits,
            total_splits,
            total_splits,
            true,
        );

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status().unwrap();

        assert_eq!(result, (BackupTaskStage::Done, 0, 0));
    }

    #[test]
    fn test_restore_status_error_split_count() {
        let mock_io = setup_restore_status_test(5, true, true, true, 6, 0, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status();

        assert!(result.is_err());
        assert_eq!(result.unwrap_err().to_string(), "split");
    }

    #[test]
    fn test_restore_status_error_compression() {
        let mock_io = setup_restore_status_test(5, true, true, true, 3, 4, 0, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status();

        assert!(result.is_err());
        assert_eq!(result.unwrap_err().to_string(), "Error stage Compress");
    }

    #[test]
    fn test_restore_status_error_encryption() {
        let mock_io = setup_restore_status_test(5, true, true, true, 3, 3, 4, 0, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status();

        assert!(result.is_err());
        assert_eq!(result.unwrap_err().to_string(), "Error stage Encrypt");
    }

    #[test]
    fn test_restore_status_error_upload() {
        let mock_io = setup_restore_status_test(5, true, true, true, 3, 3, 3, 4, 0, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status();

        assert!(result.is_err());
        assert_eq!(result.unwrap_err().to_string(), "Error stage Upload");
    }

    #[test]
    fn test_restore_status_error_cleanup() {
        let mock_io = setup_restore_status_test(5, true, true, true, 3, 3, 3, 3, 4, false);

        let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
        let result = manager.restore_status();

        assert!(result.is_err());
        assert_eq!(result.unwrap_err().to_string(), "Error stage Cleanup");
    }
}
