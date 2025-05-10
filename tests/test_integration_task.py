from datetime import datetime
from typing import Any

import pytest

from app.backup_manager import BackupTaskManager
from app.compress_handler import CompressionHandler, MockCompressionHandler
from app.define import BackupType
from app.encrypt_handler import EncryptionHandler, MockEncryptor
from app.file_handler import MockFileSystem
from app.remote_handler import MockRemoteStorageHandler, RemoteStorageHandler
from app.snapshot_handler import MockSnapshotHandler, SnapshotHandler
from app.status import BackupStatusRaw, CurrentTask, MockBackupStatusIo, Stage, Task


class TestIntegration:
    @pytest.fixture
    def split_count(self) -> int:
        return 5

    @pytest.fixture
    def snapshots(self) -> list[str]:
        number = 6
        return [f"snapshot_{i}" for i in range(number, 0, -1)]

    @pytest.fixture
    def status_filename(self) -> str:
        return "status.txt"

    @pytest.fixture
    def base_setup(
        self,
        status_filename: str,
        snapshots: list[str],
        split_count: int,
    ) -> dict[str, Any]:
        file_system = MockFileSystem()
        compressor = MockCompressionHandler(file_system, False)
        encryptor = MockEncryptor(file_system, False)
        remote = MockRemoteStorageHandler("test_bucket", file_system, False)
        snapshot_handler = MockSnapshotHandler(
            file_system, False, snapshots, split_count
        )

        return {
            "file_system": file_system,
            "compressor": compressor,
            "encryptor": encryptor,
            "remote": remote,
            "snapshot_handler": snapshot_handler,
            "status_filename": status_filename,
            "snapshots": snapshots,
            "split_count": split_count,
        }

    def create_status(
        self, task_type: BackupType, pool: str
    ) -> tuple[BackupStatusRaw, Task]:
        task = Task(datetime.now(), task_type, pool)
        status = BackupStatusRaw(
            last_record=datetime.now(),
            queue=[task],
            current=CurrentTask(
                base="",
                ref="",
                split_quantity=0,
                stage=Stage(
                    exported=False,
                    compressed=0,
                    encrypted=0,
                    uploaded=0,
                ),
            ),
        )
        return (status, task)

    def setup_backup_manager(
        self,
        base_setup: dict[str, Any],
        task_type: BackupType = "full",
        pool: str = "pool1",
        reference_type: BackupType | None = None,
        reference_index: int | None = None,
    ):
        status, task = self.create_status(task_type, pool)

        status_io = MockBackupStatusIo(
            base_setup["file_system"], base_setup["status_filename"], status
        )

        snapshot_handler = base_setup["snapshot_handler"]

        if reference_type and reference_index is not None:
            snapshot_handler.set_latest(
                task.pool, reference_type, base_setup["snapshots"][reference_index]
            )

        backup_mgr = BackupTaskManager(
            status_io,
            snapshot_handler,
            base_setup["remote"],
            base_setup["compressor"],
            base_setup["encryptor"],
            base_setup["file_system"],
        )

        return (backup_mgr, status_io, task, snapshot_handler)

    def run_backup_workflow(
        self,
        backup_mgr: BackupTaskManager,
        file_system: MockFileSystem,
        snapshot_handler: SnapshotHandler,
        compressor: CompressionHandler,
        encryptor: EncryptionHandler,
        remote: RemoteStorageHandler,
        split_count: int,
    ):
        # check status file
        assert file_system.check("status.txt") is True

        # export
        backup_mgr.run(False)
        exported_files = [
            f for f in file_system.file_system if snapshot_handler.filename in f
        ]
        assert len(exported_files) == split_count

        # compress
        for _ in range(split_count):
            backup_mgr.run(False)
        compressed_files = [
            f for f in file_system.file_system if compressor.extension in f
        ]
        assert len(compressed_files) == split_count

        # encrypt
        for _ in range(split_count):
            backup_mgr.run(False)
        encrypted_files = [
            f
            for f in file_system.file_system
            if (compressor.extension + encryptor.extension) in f
        ]
        assert len(encrypted_files) == split_count

        # upload
        for _ in range(split_count):
            backup_mgr.run(False)

        backup_mgr.run(False)

        # check remote files
        assert hasattr(remote, "objects") is True
        assert len(remote.objects) == split_count  # type: ignore

    def test_full_backup(self, base_setup: dict[str, Any]):
        backup_mgr, status_io, task, snapshot_handler = self.setup_backup_manager(
            base_setup, task_type="full", pool="pool1"
        )

        self.run_backup_workflow(
            backup_mgr,
            base_setup["file_system"],
            snapshot_handler,
            base_setup["compressor"],
            base_setup["encryptor"],
            base_setup["remote"],
            base_setup["split_count"],
        )

        # check latest snapshot
        assert (
            snapshot_handler.get_latest(task.pool, task.type)
            == base_setup["snapshots"][0]
        )
        assert len(status_io.load().queue) == 0

    def test_diff_backup(self, base_setup: dict[str, Any]):
        full_snapshot_index = 2

        backup_mgr, status_io, task, snapshot_handler = self.setup_backup_manager(
            base_setup,
            task_type="diff",
            pool="pool1",
            reference_type="full",
            reference_index=full_snapshot_index,
        )

        self.run_backup_workflow(
            backup_mgr,
            base_setup["file_system"],
            snapshot_handler,
            base_setup["compressor"],
            base_setup["encryptor"],
            base_setup["remote"],
            base_setup["split_count"],
        )

        # check latest snapshot
        assert (
            snapshot_handler.get_latest(task.pool, task.type)
            == base_setup["snapshots"][0]
        )
        assert (
            snapshot_handler.get_latest(task.pool, "full")
            == base_setup["snapshots"][full_snapshot_index]
        )
        assert len(status_io.load().queue) == 0

    def test_incr_backup(self, base_setup: dict[str, Any]):
        diff_snapshot_index = 4

        backup_mgr, status_io, task, snapshot_handler = self.setup_backup_manager(
            base_setup,
            task_type="incr",
            pool="pool1",
            reference_type="diff",
            reference_index=diff_snapshot_index,
        )

        self.run_backup_workflow(
            backup_mgr,
            base_setup["file_system"],
            snapshot_handler,
            base_setup["compressor"],
            base_setup["encryptor"],
            base_setup["remote"],
            base_setup["split_count"],
        )

        # check latest snapshot
        assert (
            snapshot_handler.get_latest(task.pool, task.type)
            == base_setup["snapshots"][0]
        )
        assert (
            snapshot_handler.get_latest(task.pool, "diff")
            == base_setup["snapshots"][diff_snapshot_index]
        )
        assert len(status_io.load().queue) == 0
