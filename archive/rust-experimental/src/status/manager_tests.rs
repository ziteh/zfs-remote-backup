use crate::status::manager::*;
use crate::status::model::*;
use chrono::Utc;
use std::collections::VecDeque;

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

mod queue_operations {
    use super::*;

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
}

mod restore_status {
    use super::*;

    mod snapshot_export {
        use super::*;

        #[test]
        fn test_snapshot_export_stage() {
            let total_splits = 5;
            let mock_io =
                setup_restore_status_test(total_splits, true, false, false, 0, 0, 0, 0, 0, false);

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status().unwrap();

            assert_eq!(result, (BackupTaskStage::SnapshotExport, 0, 0));
        }
    }

    mod snapshot_test {
        use super::*;

        #[test]
        fn test_snapshot_test_stage() {
            let total_splits = 5;
            let mock_io =
                setup_restore_status_test(total_splits, true, true, false, 0, 0, 0, 0, 0, false);

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status().unwrap();

            assert_eq!(result, (BackupTaskStage::SnapshotTest, 0, 0));
        }
    }

    mod split_stage {
        use super::*;

        #[test]
        fn test_split_stage() {
            let total_splits = 5;

            let mock_io =
                setup_restore_status_test(total_splits, true, true, true, 0, 0, 0, 0, 0, false);

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status().unwrap();

            assert_eq!(result, (BackupTaskStage::Split, total_splits, 0));
        }

        #[test]
        fn test_split_in_progress() {
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
        fn test_error_split_count() {
            let total_splits = 5;
            let split_qty = total_splits + 1;

            let mock_io = setup_restore_status_test(
                total_splits,
                true,
                true,
                true,
                split_qty,
                0,
                0,
                0,
                0,
                false,
            );

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status();

            assert!(result.is_err());
            assert_eq!(result.unwrap_err().to_string(), "split");
        }
    }

    mod compressed_stage {
        use super::*;

        #[test]
        fn test_compression_stage() {
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
        fn test_error_compression() {
            let total_splits = 5;
            let split_qty = 3;
            let compressed_qty = split_qty + 1;

            let mock_io = setup_restore_status_test(
                total_splits,
                true,
                true,
                true,
                split_qty,
                compressed_qty,
                0,
                0,
                0,
                false,
            );

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status();

            assert!(result.is_err());
            assert_eq!(result.unwrap_err().to_string(), "Error stage Compress");
        }
    }

    mod encrypted_stage {
        use super::*;

        #[test]
        fn test_encryption_stage() {
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
        fn test_error_encryption() {
            let total_splits = 5;
            let split_qty = 3;
            let encrypted_qty = split_qty + 1;

            let mock_io = setup_restore_status_test(
                total_splits,
                true,
                true,
                true,
                split_qty,
                split_qty,
                encrypted_qty,
                0,
                0,
                false,
            );

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status();

            assert!(result.is_err());
            assert_eq!(result.unwrap_err().to_string(), "Error stage Encrypt");
        }
    }

    mod uploaded_stage {
        use super::*;

        #[test]
        fn test_upload_stage() {
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
        fn test_error_upload() {
            let total_splits = 5;
            let split_qty = 3;
            let uploaded_qty = split_qty + 1;

            let mock_io = setup_restore_status_test(
                total_splits,
                true,
                true,
                true,
                split_qty,
                split_qty,
                split_qty,
                uploaded_qty,
                0,
                false,
            );

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status();

            assert!(result.is_err());
            assert_eq!(result.unwrap_err().to_string(), "Error stage Upload");
        }
    }

    mod cleanup_stage {
        use super::*;

        #[test]
        fn test_cleanup_stage() {
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
        fn test_error_cleanup() {
            let total_splits = 5;
            let split_qty = 3;
            let cleanup_qty = split_qty + 1;

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
            let result = manager.restore_status();

            assert!(result.is_err());
            assert_eq!(result.unwrap_err().to_string(), "Error stage Cleanup");
        }
    }

    mod verify_stage {
        use super::*;

        #[test]
        fn test_verify_stage() {
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
    }

    mod done_states {
        use super::*;

        #[test]
        fn test_empty_queue() {
            let mock_io = setup_restore_status_test(0, false, false, false, 0, 0, 0, 0, 0, false);

            let mut manager = StatusManager::new(Box::new(mock_io)).unwrap();
            let result = manager.restore_status().unwrap();

            assert_eq!(result, (BackupTaskStage::Done, 0, 0));
        }

        #[test]
        fn test_done() {
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
    }
}
