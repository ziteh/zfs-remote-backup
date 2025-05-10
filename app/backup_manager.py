from datetime import datetime
from pathlib import Path

from loguru import logger

from app.compress_handler import CompressionHandler
from app.define import TEMP_DIR, BackupType, SystemShutdownError
from app.encrypt_handler import EncryptionHandler
from app.file_handler import FileHandler
from app.remote_handler import RemoteStorageHandler
from app.snapshot_handler import SnapshotHandler
from app.status import (
    BackupStatusIo,
    BackupStatusRaw,
    BackupTaskStage,
    Task,
)


class BackupTaskManager:
    __snapshot_mgr: SnapshotHandler
    __remote_mgr: RemoteStorageHandler
    __compress_mgr: CompressionHandler
    __encrypt_mgr: EncryptionHandler
    __file_mgr: FileHandler

    __status: BackupStatusRaw
    __status_manager: BackupStatusIo

    def __init__(
        self,
        status_io: BackupStatusIo,
        snapshot_handler: SnapshotHandler,
        remote_handler: RemoteStorageHandler,
        compression_handler: CompressionHandler,
        encryption_handler: EncryptionHandler,
        file_handler: FileHandler,
    ):
        self.__snapshot_mgr = snapshot_handler
        self.__remote_mgr = remote_handler
        self.__compress_mgr = compression_handler
        self.__encrypt_mgr = encryption_handler
        self.__file_mgr = file_handler

        self.__status_manager = status_io
        self.__status = status_io.load()
        self.__load_backup_task()

    @property
    def pool(self) -> str:
        """ZFS pool"""
        # self.__status = self.__status_manager.load()
        return self.__status.queue[0].pool

    @property
    def type(self) -> BackupType:
        """Backup type"""
        # self.__status = self.__status_manager.load()
        return self.__status.queue[0].type

    @property
    def date(self) -> datetime:
        """Target date"""
        # self.__status = self.__status_manager.load()
        return self.__status.queue[0].date

    @property
    def base(self) -> str:
        """Base snapshot"""
        # self.__status = self.__status_manager.load()
        return self.__status.current.base

    @property
    def ref(self) -> str:
        """Ref snapshot"""
        # self.__status = self.__status_manager.load()
        return self.__status.current.ref

    @property
    def is_no_task(self) -> bool:
        # self.__status = self.__status_manager.load()
        return len(self.__status.queue) == 0

    @property
    def temp_path(self) -> Path:
        """Get the temporary path for the backup task."""
        return (
            Path(TEMP_DIR) / self.pool / f"{self.type}_{self.date.strftime('%Y-%m-%d')}"
        )

    def run(self, auto: bool = False) -> None:
        if self.is_no_task:
            return

        (stage, current, total) = self.__get_stage()

        if self.__is_error_stage(current, total):
            logger.error(f"Error in stage: {stage}, current: {current}, total: {total}")
            return

        try:
            match stage:
                case "export":
                    self.__handle_export_stage()

                case "compress":
                    self.__handle_compress_stage(current)

                case "encrypt":
                    self.__handle_encrypt_stage(current)

                case "upload":
                    self.__handle_update_stage(current)

                case "done":
                    self.__handle_done_stage()
                    return  # all tasks are done

                case _:
                    msg = f"Unknown stage {stage}"
                    logger.critical(msg)
                    raise ValueError(msg)

        except SystemShutdownError as e:
            logger.error(f"System is shutting down. {e}")
            return

        if auto:
            self.run(True)

    def __handle_export_stage(self):
        num_parts = self.__snapshot_mgr.export(
            self.pool, self.base, self.ref, str(self.temp_path)
        )
        self.__set_stage_export(num_parts)
        pass

    def __handle_compress_stage(self, index: int):
        filename = self.temp_path / f"{self.__snapshot_mgr.filename}{index:06}"
        compressed_filename = self.__compress_mgr.compress(str(filename))
        self.__set_stage_compress(index + 1)

        if not self.__compress_mgr.verify(compressed_filename):
            raise RuntimeError(f"Compression failed for {compressed_filename}")

        self.__file_mgr.delete(str(filename))  # delete the original file
        self.__set_stage_compress(index + 1)

    def __handle_encrypt_stage(self, index: int):
        filename = self.temp_path / f"{self.__snapshot_mgr.filename}{index:06}"
        compressed_filename = str(filename) + self.__compress_mgr.extension

        _encrypted_filename = self.__encrypt_mgr.encrypt(compressed_filename)
        self.__file_mgr.delete(str(compressed_filename))  # delete the compressed file
        self.__set_stage_encrypt(index + 1)

    def __handle_update_stage(self, index: int):
        filename = self.temp_path / (
            f"{self.__snapshot_mgr.filename}{index:06}"
            + self.__compress_mgr.extension
            + self.__encrypt_mgr.extension
        )

        tags = {"backup-type": self.type}
        metadata = {
            "pool": self.pool,
            "base-snapshot": self.base,
            "ref-snapshot": self.ref,
        }

        self.__remote_mgr.upload(str(filename), str(filename), tags, metadata)
        self.__set_stage_upload(index + 1)
        self.__file_mgr.delete(str(filename))  # delete the uploaded file
        self.__set_stage_remove(index + 1)

    def __handle_done_stage(self):
        snapshot_name = self.base.split("@")[-1]
        self.__snapshot_mgr.set_latest(self.pool, self.type, snapshot_name)
        self.__dequeue_backup_task()

    def __is_error_stage(self, current: int, total: int) -> bool:
        return total < 0 or current < 0

    def __get_stage(self) -> tuple[BackupTaskStage, int, int]:
        stage = self.__status.current.stage
        if not stage.exported:
            return ("export", 0, 0)

        split_qty = self.__status.current.split_quantity
        if split_qty <= 0:
            return ("export", -1, split_qty)  # error

        if stage.compressed > split_qty:
            return ("compress", -stage.compressed, -split_qty)  # error
        elif stage.compressed < split_qty:
            return ("compress", stage.compressed, split_qty)

        if stage.encrypted > split_qty:
            return ("encrypt", -stage.encrypted, -split_qty)  # error
        elif stage.encrypted < split_qty:
            return ("encrypt", stage.encrypted, split_qty)

        if stage.uploaded > split_qty:
            return ("upload", -stage.uploaded, -split_qty)  # error
        elif stage.uploaded < split_qty:
            return ("upload", stage.uploaded, split_qty)

        return ("done", 0, split_qty)

    def __save_status(self) -> None:
        self.__status_manager.save(self.__status)

    def __load_backup_task(self) -> Task | None:
        if len(self.__status.queue) == 0:
            return None

        task = self.__status.queue[0]

        # Clear the task status
        self.__status.current.split_quantity = 0
        self.__status.current.stage.exported = False
        self.__status.current.stage.compressed = 0
        self.__status.current.stage.encrypted = 0
        self.__status.current.stage.uploaded = 0
        self.__status.current.stage.removed = 0

        # Set the snapshot
        snapshots = self.__snapshot_mgr.list(task.pool)
        self.__status.current.base = snapshots[0]

        match task.type:
            case "full":
                # Full backup, no reference snapshot
                self.__status.current.ref = ""

            case "diff":
                # Differential backup, use the latest full snapshot as reference
                self.__status.current.ref = (
                    self.__snapshot_mgr.get_latest(task.pool, "full") or "ERROR_NONE"
                )

            case "incr":
                # Incremental backup, use the latest differential snapshot as reference
                self.__status.current.ref = (
                    self.__snapshot_mgr.get_latest(task.pool, "diff") or "ERROR_NONE"
                )

            case _:
                msg = f"Unknown backup type: {task.type}"
                raise ValueError(msg)

        return task

    def enqueue_backup_task(self, task: Task) -> int:
        self.__status.queue.append(task)
        self.__save_status()
        return len(self.__status.queue)

    def __dequeue_backup_task(self) -> int:
        if len(self.__status.queue) == 0:
            return 0

        _task = self.__status.queue.pop(0)
        self.__load_backup_task()
        self.__save_status()
        return len(self.__status.queue)

    def __set_stage_export(self, parts: int | None):
        """Sets the export stage of the backup status.

        Args:
            parts: Number of parts export and split. `None` if not exported.
        """

        if parts is None:
            self.__status.current.stage.exported = False
            self.__status.current.split_quantity = 0
        else:
            self.__status.current.stage.exported = True
            self.__status.current.split_quantity = parts

        self.__save_status()

    def __set_stage_compress(self, done: int):
        self.__status.current.stage.compressed = done
        self.__save_status()

    def __set_stage_encrypt(self, done: int):
        self.__status.current.stage.encrypted = done
        self.__save_status()

    def __set_stage_upload(self, done: int):
        self.__status.current.stage.uploaded = done
        self.__save_status()

    def __set_stage_remove(self, done: int):
        self.__status.current.stage.removed = done
        self.__save_status()
