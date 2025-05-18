from datetime import datetime
from unittest.mock import Mock

import pytest

from app.status_manager import (
    BackupTarget,
    CurrentTask,
    DatasetLatestSnapshot,
    Stage,
    StatusFilesIo,
    StatusManager,
    TaskQueue,
)


@pytest.fixture
def mock_status_io() -> Mock:
    mock_io = Mock(spec=StatusFilesIo)

    # Set up return values for load methods
    mock_io.load_task_queue.return_value = TaskQueue(tasks=[])
    mock_io.load_latest_snapshot.return_value = DatasetLatestSnapshot(latest={})
    mock_io.load_current_task.return_value = CurrentTask(
        base="base_snapshot",
        ref="ref_snapshot",
        split_quantity=5,
        stream_hash=b"hash",
        stage=Stage(
            snapshot_exported="",
            snapshot_tested=False,
            spit=[],
            compressed=0,
            encrypted=0,
            uploaded=0,
            verify=False,
            cleared=0,
        ),
    )

    return mock_io


@pytest.fixture
def mock_snapshot_handler():
    mock_handler = Mock()
    mock_handler.list.return_value = ["snapshot1", "snapshot2"]
    return mock_handler


@pytest.fixture
def status_manager(mock_status_io: Mock, mock_snapshot_handler: Mock) -> StatusManager:
    manager = StatusManager(mock_status_io, mock_snapshot_handler)

    mock_status_io.load_task_queue.reset_mock()
    mock_status_io.load_latest_snapshot.reset_mock()
    mock_status_io.load_current_task.reset_mock()

    return manager


class TestStatusManagerUpdateStage:
    """Test StatusManager.update_stage() method."""

    def test_update_snapshot_export(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test updating snapshot_export stage"""
        snapshot_name = "test_snapshot"
        status_manager.update_stage("snapshot_export", snapshot_name)

        # assert snapshot_exported update
        assert status_manager.current_task.stage.snapshot_exported == snapshot_name

        # assert save_current_task called
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_snapshot_test(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test updating snapshot_test stage"""
        status_manager.update_stage("snapshot_test", True)

        assert status_manager.current_task.stage.snapshot_tested is True
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_split(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating split stage"""
        test_hash = b"test_hash_value"
        status_manager.update_stage("split", test_hash)

        assert status_manager.current_task.stage.spit == [test_hash]
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

        # Test adding multiple split hashes
        second_hash = b"second_hash"
        mock_status_io.save_current_task.reset_mock()  # Reset mock
        status_manager.update_stage("split", second_hash)

        assert status_manager.current_task.stage.spit == [test_hash, second_hash]
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_compress(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating compress stage"""
        status_manager.update_stage("compress", 3)

        assert status_manager.current_task.stage.compressed == 3
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_encrypt(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating encrypt stage"""
        status_manager.update_stage("encrypt", 4)

        assert status_manager.current_task.stage.encrypted == 4
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_upload(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating upload stage"""
        status_manager.update_stage("upload", 5)

        assert status_manager.current_task.stage.uploaded == 5
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_verify(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating verify stage"""
        status_manager.update_stage("verify", True)

        assert status_manager.current_task.stage.verify is True
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_clear(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating clear stage"""
        status_manager.update_stage("clear", 5)

        assert status_manager.current_task.stage.cleared == 5
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_all_stages_sequence(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test sequentially updating all stages"""
        mock_status_io.save_current_task.reset_mock()

        # Update all stages in sequence
        status_manager.update_stage("snapshot_export", "snapshot1")
        status_manager.update_stage("snapshot_test", True)
        status_manager.update_stage("split", b"hash1")
        status_manager.update_stage("split", b"hash2")
        status_manager.update_stage("compress", 2)
        status_manager.update_stage("encrypt", 2)
        status_manager.update_stage("upload", 2)
        status_manager.update_stage("verify", True)
        status_manager.update_stage("clear", 2)

        # Verify each stage is correctly updated
        assert status_manager.current_task.stage.snapshot_exported == "snapshot1"
        assert status_manager.current_task.stage.snapshot_tested is True
        assert status_manager.current_task.stage.spit == [b"hash1", b"hash2"]
        assert status_manager.current_task.stage.compressed == 2
        assert status_manager.current_task.stage.encrypted == 2
        assert status_manager.current_task.stage.uploaded == 2
        assert status_manager.current_task.stage.verify is True
        assert status_manager.current_task.stage.cleared == 2

        # Verify save method was called correct number of times
        assert mock_status_io.save_current_task.call_count == 9


class TestStatusManagerRestoreStatus:
    """Test StatusManager.restore_status() method."""

    def test_empty_task_queue(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when task queue is empty."""
        # Configure empty task queue
        mock_status_io.load_task_queue.return_value = TaskQueue(tasks=[])

        stage, total, completed = status_manager.restore_status()

        assert stage == "done"
        assert total == 0
        assert completed == 0

        mock_status_io.load_task_queue.assert_called_once()

    def test_pending_snapshot_export(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when snapshot export is pending."""
        # Configure current task with empty snapshot_exported
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"",
            stage=Stage(
                snapshot_exported="",
                snapshot_tested=False,
                spit=[],
                compressed=0,
                encrypted=0,
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )  # Non-empty task queue

        stage, total, completed = status_manager.restore_status()

        assert stage == "snapshot_export"
        assert total == 0
        assert completed == 0
        mock_status_io.load_current_task.assert_called_once()

    def test_pending_snapshot_test(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when snapshot test is pending."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",  # Snapshot is exported
                snapshot_tested=False,  # But not tested
                spit=[],
                compressed=0,
                encrypted=0,
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "snapshot_test"
        assert total == 0
        assert completed == 0

    def test_pending_split(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when split is pending."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,  # Snapshot tested
                spit=[],  # No split hashes
                compressed=0,
                encrypted=0,
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "split"
        assert total == 0
        assert completed == 0

    def test_partial_compression(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when some files are compressed."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[
                    b"hash1",
                    b"hash2",
                    b"hash3",
                ],  # 3 split hashes
                compressed=2,  # 2 out of 3 compressed
                encrypted=0,
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "compress"
        assert total == 3
        assert completed == 2

    def test_partial_encryption(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when some files are encrypted."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3"],
                compressed=3,
                encrypted=2,  # 2 out of 5 encrypted
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )
        stage, total, completed = status_manager.restore_status()

        assert stage == "encrypt"
        assert total == 3
        assert completed == 2

    def test_partial_upload(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when some files are uploaded."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3"],
                compressed=3,
                encrypted=3,  # All encrypted
                uploaded=2,  # 2 out of 3 uploaded
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "upload"
        assert total == 3
        assert completed == 2

    def test_pending_clearing(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when clearing is pending."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3"],
                compressed=3,
                encrypted=3,
                uploaded=3,  # All uploaded
                verify=False,  # Not verified yet
                cleared=2,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "clear"
        assert total == 3
        assert completed == 2

    def test_partial_verification(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when files are partially verified."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3", b"hash4", b"hash5"],
                compressed=5,
                encrypted=5,
                uploaded=5,
                verify=False,  # Verified
                cleared=5,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "verify"
        assert total == 5
        assert completed == 0

    def test_completed_backup(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when backup is complete."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3", b"hash4", b"hash5"],
                compressed=5,
                encrypted=5,
                uploaded=5,
                verify=True,
                cleared=5,  # All cleared
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "done"
        assert total == 0
        assert completed == 5

    def test_error_in_compression(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when there is an error in compression (more compressed than split)."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=5,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[b"hash1", b"hash2", b"hash3"],  # Only 3 split hashes
                compressed=4,  # But 4 compressed - error condition
                encrypted=0,
                uploaded=0,
                verify=False,
                cleared=0,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "compress"
        assert total == -3  # Negative values indicate error
        assert completed == -4
