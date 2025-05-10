from datetime import datetime

from app.backup_manager import BackupTaskManager
from app.compress_handler import MockCompressionHandler
from app.encrypt_handler import MockEncryptor
from app.file_handler import MockFileSystem
from app.remote_handler import MockRemoteStorageHandler
from app.snapshot_handler import MockSnapshotHandler
from app.status import BackupStatusRaw, CurrentTask, MockBackupStatusIo, Stage, Task


class TestIntegration:
    def test_full_normal(self):
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
        snapshots = [
            "snapshot_0",
            "snapshot_1",
            "snapshot_2",
        ]
        split_count = 5

        file_system = MockFileSystem()
        status_io = MockBackupStatusIo(file_system, status_filename, status)
        snapshot_handler = MockSnapshotHandler(
            file_system, False, snapshots, split_count
        )
        remote = MockRemoteStorageHandler("test_bucket", file_system, False)
        compressor = MockCompressionHandler(file_system, False)
        encryptor = MockEncryptor(file_system, False)
        backup_mgr = BackupTaskManager(
            status_io,
            snapshot_handler,
            remote,
            compressor,
            encryptor,
            file_system,
        )

        # should contain status file
        assert file_system.check(status_filename) is True

        # export snapshot
        backup_mgr.run(False)
        exported_files = [
            f for f in file_system.file_system if snapshot_handler.filename in f
        ]
        assert len(exported_files) == split_count

        # compress
        for _ in range(split_count):
            backup_mgr.run(False)
        compressed_files = [
            f
            for f in file_system.file_system
            if (compressor.extension + encryptor.extension) in f
        ]
        assert len(compressed_files) == split_count

        # upload
        for _ in range(split_count):
            backup_mgr.run(False)
            # every uploaded file should be removed
            # assert len(file_system.file_system) == (1 + split_count - (i + 1))

        # check latest snapshot
        backup_mgr.run(False)
        assert snapshot_handler.get_latest(task0.pool, task0.type) == snapshots[0]

        # check remote files
        assert hasattr(remote, "objects") is True
        assert len(remote.objects) == split_count

        # check no task
        assert len(status_io.load().queue) == 0
