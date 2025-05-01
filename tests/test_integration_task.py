from datetime import datetime

from app.backup_manager import BackupTaskManager
from app.compress_handler import MockCompressionHandler
from app.encrypt_handler import MockEncryptor
from app.file_handler import MockFileSystem
from app.remote_handler import MockRemoteStorageHandler
from app.snapshot_handler import MockSnapshotHandler
from app.status import BackupStatusRaw, CurrentTask, MockBackupStatusIo, Stage, Task


class TestIntegration:
    def test_normal(self):
        task0 = Task(datetime.now(), "full", "pool1")
        status = BackupStatusRaw(
            last_record=datetime.now(),
            queue=[task0],
            current=CurrentTask(
                base="",
                ref="",
                split_quantity=0,
                stage=Stage(
                    exported=False,
                    compressed=0,
                    compress_tested=0,
                    encrypted=0,
                    uploaded=0,
                    removed=0,
                ),
            ),
        )
        status_filename = "status.txt"

        file_system = MockFileSystem()
        status_io = MockBackupStatusIo(file_system, status_filename, status)
        snapshot_handler = MockSnapshotHandler(file_system)
        snapshot_handler.snapshots.append("snapshot_0")
        snapshot_handler.snapshots.append("snapshot_1")
        snapshot_handler.snapshots.append("snapshot_2")
        remote = MockRemoteStorageHandler(file_system)
        compressor = MockCompressionHandler(file_system)
        encryptor = MockEncryptor(file_system)
        backup_mgr = BackupTaskManager(
            "test_bucket",
            status_io,
            snapshot_handler,
            remote,
            compressor,
            encryptor,
            file_system,
        )

        assert len(file_system.file_system) == 1
        assert file_system.check(status_filename) is True
        backup_mgr.run()
        assert len(file_system.file_system) == 4
