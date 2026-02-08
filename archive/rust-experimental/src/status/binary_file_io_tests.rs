#[cfg(test)]
mod tests {
    use crate::status::manager::FileIo;
    use crate::status::binary_file_io::BinaryFileIo;
    use crate::status::model::*;
    use chrono::Utc;
    use std::collections::HashMap;
    use tempfile::TempDir;

    #[test]
    fn test_binary_file_io_target_queue() {
        // Create temporary directory
        let temp_dir = TempDir::new().unwrap();
        let io = BinaryFileIo::new(temp_dir.path()).unwrap();

        // Create test data
        let mut queue = BackupTargetQueue::new();
        let target = BackupTarget {
            date: Utc::now(),
            backup_type: BackupType::Full,
            dataset: "test_dataset".to_string(),
        };
        queue.push_back(target.clone());

        // Test save and load
        io.save_target_queue(&queue).unwrap();
        let loaded_queue = io.load_target_queue().unwrap();

        assert_eq!(queue.len(), loaded_queue.len());
        assert_eq!(queue.front().unwrap().dataset, loaded_queue.front().unwrap().dataset);
        assert_eq!(queue.front().unwrap().backup_type, loaded_queue.front().unwrap().backup_type);
    }

    #[test]
    fn test_binary_file_io_active_tasks() {
        // Create temporary directory
        let temp_dir = TempDir::new().unwrap();
        let io = BinaryFileIo::new(temp_dir.path()).unwrap();

        // Create test data
        let task = ActiveBackupTask {
            base_snapshot: "base_snap".to_string(),
            ref_snapshot: "ref_snap".to_string(),
            split_qty: 10,
            progress: BackupStageStatus {
                snapshot_exported_name: "exported.bin".to_string(),
                snapshot_tested: true,
                split_hashes: vec![vec![1, 2, 3], vec![4, 5, 6]],
                compressed: 5,
                encrypted: 3,
                uploaded: 2,
                cleanup: 1,
                verified: false,
            },
            full_hash: vec![7, 8, 9],
        };

        // Test save and load
        io.save_active_tasks(&task).unwrap();
        let loaded_task = io.load_active_tasks().unwrap();

        assert_eq!(task.base_snapshot, loaded_task.base_snapshot);
        assert_eq!(task.ref_snapshot, loaded_task.ref_snapshot);
        assert_eq!(task.split_qty, loaded_task.split_qty);
        assert_eq!(task.progress.snapshot_exported_name, loaded_task.progress.snapshot_exported_name);
        assert_eq!(task.progress.snapshot_tested, loaded_task.progress.snapshot_tested);
        assert_eq!(task.progress.split_hashes, loaded_task.progress.split_hashes);
        assert_eq!(task.progress.compressed, loaded_task.progress.compressed);
        assert_eq!(task.progress.encrypted, loaded_task.progress.encrypted);
        assert_eq!(task.progress.uploaded, loaded_task.progress.uploaded);
        assert_eq!(task.progress.cleanup, loaded_task.progress.cleanup);
        assert_eq!(task.progress.verified, loaded_task.progress.verified);
        assert_eq!(task.full_hash, loaded_task.full_hash);
    }

    #[test]
    fn test_binary_file_io_latest_snapshot_map() {
        // Create temporary directory
        let temp_dir = TempDir::new().unwrap();
        let io = BinaryFileIo::new(temp_dir.path()).unwrap();

        // Create test data
        let mut map = LatestSnapshotMap::new();
        let mut dataset_map = HashMap::new();

        let snapshot_info = LatestSnapshotInfo {
            update: Utc::now(),
            snapshot: "test_snapshot".to_string(),
        };

        dataset_map.insert(BackupType::Full, snapshot_info.clone());
        dataset_map.insert(BackupType::Diff, snapshot_info.clone());
        map.insert("test_dataset".to_string(), dataset_map);

        // Test save and load
        io.save_latest_snapshot_map(&map).unwrap();
        let loaded_map = io.load_latest_snapshot_map().unwrap();

        assert_eq!(map.len(), loaded_map.len());
        assert!(loaded_map.contains_key("test_dataset"));

        let loaded_dataset_map = loaded_map.get("test_dataset").unwrap();
        assert!(loaded_dataset_map.contains_key(&BackupType::Full));
        assert!(loaded_dataset_map.contains_key(&BackupType::Diff));

        let loaded_full_info = loaded_dataset_map.get(&BackupType::Full).unwrap();
        assert_eq!(snapshot_info.snapshot, loaded_full_info.snapshot);
    }

    #[test]
    fn test_binary_file_io_empty_data() {
        // Create temporary directory
        let temp_dir = TempDir::new().unwrap();
        let io = BinaryFileIo::new(temp_dir.path()).unwrap();

        // Test loading from non-existent files should return default values
        let queue = io.load_target_queue().unwrap();
        assert!(queue.is_empty());

        let task = io.load_active_tasks().unwrap();
        assert_eq!(task.base_snapshot, "");
        assert_eq!(task.ref_snapshot, "");
        assert_eq!(task.split_qty, 0);

        let map = io.load_latest_snapshot_map().unwrap();
        assert!(map.is_empty());
    }

    #[test]
    fn test_binary_file_io_overwrite() {
        // Create temporary directory
        let temp_dir = TempDir::new().unwrap();
        let io = BinaryFileIo::new(temp_dir.path()).unwrap();

        // Create initial data
        let mut queue1 = BackupTargetQueue::new();
        let target1 = BackupTarget {
            date: Utc::now(),
            backup_type: BackupType::Full,
            dataset: "dataset1".to_string(),
        };
        queue1.push_back(target1);

        // Save initial data
        io.save_target_queue(&queue1).unwrap();

        // Create new data
        let mut queue2 = BackupTargetQueue::new();
        let target2 = BackupTarget {
            date: Utc::now(),
            backup_type: BackupType::Diff,
            dataset: "dataset2".to_string(),
        };
        queue2.push_back(target2);

        // Save new data (should overwrite)
        io.save_target_queue(&queue2).unwrap();

        // Load and verify it's the new data
        let loaded_queue = io.load_target_queue().unwrap();
        assert_eq!(loaded_queue.len(), 1);
        assert_eq!(loaded_queue.front().unwrap().dataset, "dataset2");
        assert_eq!(loaded_queue.front().unwrap().backup_type, BackupType::Diff);
    }
}
