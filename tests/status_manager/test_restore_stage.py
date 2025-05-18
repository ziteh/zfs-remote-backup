from datetime import datetime
from unittest.mock import Mock

import pytest

from app.status_manager import (
    BackupTarget,
    CurrentTask,
    Stage,
    StatusManager,
    TaskQueue,
)


class TestTaskQueueStage:
    """Test restore_status related to task queue."""

    def test_empty_task_queue(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when task queue is empty."""
        # Configure empty task queue
        mock_status_io.load_task_queue.return_value = TaskQueue(tasks=[])

        stage, total, completed = status_manager.restore_status()

        assert stage == "done"
        assert total == 0
        assert completed == 0

        mock_status_io.load_task_queue.assert_called_once()


class TestSnapshotExportStage:
    """Test restore_status related to snapshot export stage."""

    def test_pending_snapshot_export(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when snapshot export is pending."""
        # Configure current task with empty snapshot_exported
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=0,
            stream_hash=b"",
            stage=Stage(
                snapshot_exported="",  # No snapshot exported
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


class TestSnapshotTestStage:
    """Test restore_status related to snapshot test stage."""

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


class TestSplitStage:
    """Test restore_status related to split stage."""

    def test_pending_split(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when split is pending."""
        total_split_qty = 5

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
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
        assert total == total_split_qty
        assert completed == 0

    def test_pending_split_2(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when split is partially complete."""
        total_split_qty = 5
        processed_qty = total_split_qty - 2  # 3 out of 5 spited

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(processed_qty)],
                compressed=processed_qty,
                encrypted=processed_qty,
                uploaded=processed_qty,
                verify=False,
                cleared=processed_qty,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "split"
        assert total == total_split_qty
        assert completed == processed_qty

    def test_error_pending_split(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test restore status when there is an error in split (more splits than expected)."""
        total_split_qty = 5
        processed_qty = total_split_qty + 1  # processed file more than split

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(processed_qty)],
                compressed=processed_qty,
                encrypted=processed_qty,
                uploaded=processed_qty,
                verify=False,
                cleared=processed_qty,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "split"
        assert total == -1 * total_split_qty  # Negative values indicate error
        assert completed == -1 * processed_qty  # Negative values indicate error


class TestCompressionStage:
    """Test restore_status related to compression stage."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 3),  # 3 out of 5 spited
            (5, 4),  # 4 out of 5 spited
            (100000, 99999),  # 99999 out of 100000 spited
        ],
    )
    def test_partial_compression(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when some files are compressed."""
        compressed_qty = spited_qty - 1  # last one not compressed

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=compressed_qty,
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
        assert total == spited_qty
        assert completed == compressed_qty

    def test_error_in_compression(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when there is an error in compression (more compressed than split)."""
        total_split_qty = 5
        spited_qty = total_split_qty - 2  # 3 out of 5 spited
        compressed_qty = spited_qty + 1  # compressed file more than spited

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=compressed_qty,
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
        assert total == -1 * spited_qty  # Negative values indicate error
        assert completed == -1 * compressed_qty  # Negative values indicate error


class TestEncryptionStage:
    """Test restore_status related to encryption stage."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 3),  # 3 out of 5 spited
            (5, 4),  # 4 out of 5 spited
            (100000, 99999),  # 99999 out of 100000 spited
        ],
    )
    def test_partial_encryption(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when some files are encrypted."""
        encrypted_qty = spited_qty - 1  # last one not encrypted

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=encrypted_qty,
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
        assert total == spited_qty
        assert completed == encrypted_qty

    def test_error_partial_encryption(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when there is an error in encryption (more encrypted than compressed)."""
        total_split_qty = 5
        spited_qty = total_split_qty - 2  # 3 out of 5 spited
        encrypted_qty = spited_qty + 1  # encrypted file more than spited

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=encrypted_qty,
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
        assert total == -1 * spited_qty  # Negative values indicate error
        assert completed == -1 * encrypted_qty  # Negative values indicate error


class TestUploadStage:
    """Test restore_status related to upload stage."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 3),  # 3 out of 5 spited
            (5, 4),  # 4 out of 5 spited
            (100000, 99999),  # 99999 out of 100000 spited
        ],
    )
    def test_partial_upload(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when some files are uploaded."""
        uploaded_qty = spited_qty - 1  # last one not uploaded

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=spited_qty,  # all compressed files are encrypted
                uploaded=uploaded_qty,
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
        assert total == spited_qty
        assert completed == uploaded_qty

    def test_error_partial_upload(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when there is an error in upload (more uploaded than encrypted)."""
        total_split_qty = 5
        spited_qty = total_split_qty - 2  # 3 out of 5 spited
        uploaded_qty = spited_qty + 1  # uploaded file more than spited

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=spited_qty,  # all compressed files are encrypted
                uploaded=uploaded_qty,
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
        assert total == -1 * spited_qty  # Negative values indicate error
        assert completed == -1 * uploaded_qty  # Negative values indicate error


class TestClearStage:
    """Test restore_status related to clear stage."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 3),  # 3 out of 5 spited
            (5, 4),  # 4 out of 5 spited
            (100000, 99999),  # 99999 out of 100000 spited
        ],
    )
    def test_pending_clearing(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when clearing is pending."""
        cleared_qty = spited_qty - 1  # last one not cleared

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=spited_qty,  # all compressed files are encrypted
                uploaded=spited_qty,  # all encrypted files are uploaded
                verify=False,
                cleared=cleared_qty,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "clear"
        assert total == spited_qty
        assert completed == cleared_qty

    def test_error_pending_clearing(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test restore status when there is an error in clearing (more cleared than uploaded)."""
        total_split_qty = 5
        spited_qty = total_split_qty - 2  # 3 out of 5 spited
        cleared_qty = spited_qty + 1  # cleared file more than spited

        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,  # all spited files are compressed
                encrypted=spited_qty,  # all compressed files are encrypted
                uploaded=spited_qty,  # all encrypted files are uploaded
                verify=False,
                cleared=cleared_qty,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "clear"
        assert total == -1 * spited_qty  # Negative values indicate error
        assert completed == -1 * cleared_qty  # Negative values indicate error


class TestVerifyStage:
    """Test restore_status related to verify stage."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 5),  # All files are split
            (10, 10),  # All files are split
            (100000, 100000),  # All files are split
        ],
    )
    def test_partial_verification(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when files are partially verified."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,
                encrypted=spited_qty,
                uploaded=spited_qty,
                verify=False,  # not verified yet
                cleared=spited_qty,
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "verify"
        assert total == spited_qty
        assert completed == 0


class TestCompletedBackup:
    """Test restore_status related to completed backup."""

    @pytest.mark.parametrize(
        "total_split_qty, spited_qty",
        [
            (5, 5),  # All files are split
            (10, 10),  # All files are split
            (100000, 100000),  # All files are split
        ],
    )
    def test_completed_backup(
        self,
        status_manager: StatusManager,
        mock_status_io: Mock,
        total_split_qty: int,
        spited_qty: int,
    ) -> None:
        """Test restore status when backup is complete."""
        current_task = CurrentTask(
            base="base_snapshot",
            ref="ref_snapshot",
            split_quantity=total_split_qty,
            stream_hash=b"hash",
            stage=Stage(
                snapshot_exported="snapshot1",
                snapshot_tested=True,
                spit=[f"hash{i}".encode() for i in range(spited_qty)],
                compressed=spited_qty,
                encrypted=spited_qty,
                uploaded=spited_qty,
                verify=True,  # verified
                cleared=spited_qty,  # all cleared
            ),
        )
        mock_status_io.load_current_task.return_value = current_task
        mock_status_io.load_task_queue.return_value = TaskQueue(
            tasks=[BackupTarget(date=datetime.now(), type="full", dataset="test_dataset")]
        )

        stage, total, completed = status_manager.restore_status()

        assert stage == "done"
        assert total == 0
        assert completed == 0
