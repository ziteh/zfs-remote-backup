use mockall::{automock, predicate::*};

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
    pub fn new(io: Box<dyn FileIo>) -> Self {
        let target_queue = io.load_target_queue().unwrap_or_default();
        let active_tasks = io.load_active_tasks().unwrap_or_default();
        let latest_snapshot_map = io.load_latest_snapshot_map().unwrap_or_default();

        StatusManager {
            io,
            target_queue,
            active_tasks,
            latest_snapshot_map,
        }
    }

    pub fn enqueue_target(&mut self, target: BackupTarget) -> Result<(), Error> {
        self.target_queue.push_back(target);
        self.io.save_target_queue(&self.target_queue)
    }

    pub fn dequeue_target(&mut self) -> Result<BackupTarget, Error> {
        if self.target_queue.is_empty() {
            return Err(Error);
        }

        if let Some(target) = self.target_queue.pop_front() {
            self.io.save_target_queue(&self.target_queue)?;
            Ok(target)
        } else {
            Err(Error)
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use chrono::Utc;

    fn sample_target(name: &str) -> BackupTarget {
        BackupTarget {
            date: Utc::now(),
            backup_type: BackupType::Full,
            dataset: name.to_string(),
        }
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
            .times(2)
            .returning(|_| Ok(()));

        let mut manager = StatusManager::new(Box::new(mock_io));

        let target1 = sample_target("pool1/data1");
        manager.enqueue_target(target1.clone()).unwrap();

        let target2 = sample_target("pool1/data2");
        manager.enqueue_target(target2.clone()).unwrap();

        let out1 = manager.dequeue_target().unwrap();
        assert_eq!(out1.dataset, target1.dataset);

        let out2 = manager.dequeue_target().unwrap();
        assert_eq!(out2.dataset, target2.dataset);

        assert!(manager.dequeue_target().is_err());
    }
}
