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
