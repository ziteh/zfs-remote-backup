import os
from datetime import datetime
from pathlib import Path

from loguru import logger

from app.compress_handler import CompressionHandler
from app.define import TEMP_DIR, BackupType
from app.encrypt_handler import EncryptionHandler
from app.file_handler import FileHandler
from app.remote_handler import RemoteStorageHandler
from app.snapshot_handler import SnapshotHandler
from app.status import BackupStatusIo, BackupStatusManager


class BackupTaskManager:
    __status_mgr: BackupStatusManager
    __snapshot_mgr: SnapshotHandler
    __remote_mgr: RemoteStorageHandler
    __compress_mgr: CompressionHandler
    __encrypt_mgr: EncryptionHandler
    __file_mgr: FileHandler

    bucket: str

    def __init__(
        self,
        status_io: BackupStatusIo,
        snapshot_handler: SnapshotHandler,
        remote_handler: RemoteStorageHandler,
        compression_handler: CompressionHandler,
        encryption_handler: EncryptionHandler,
        file_handler: FileHandler,
    ):
        self.__status_mgr = BackupStatusManager(status_io)
        self.__snapshot_mgr = snapshot_handler
        self.__remote_mgr = remote_handler
        self.__compress_mgr = compression_handler
        self.__encrypt_mgr = encryption_handler
        self.__file_mgr = file_handler

        bucket = os.getenv("S3_BUCKET")
        if bucket is None:
            raise ValueError("S3_BUCKET environment variable is not set")
        self.bucket = bucket

    @property
    def pool(self) -> str:
        return self.__status_mgr.pool

    @property
    def type(self) -> BackupType:
        return self.__status_mgr.backup_type

    @property
    def date(self) -> datetime:
        return self.__status_mgr.target_date

    @property
    def base(self) -> str:
        return self.__status_mgr.base_snapshot

    @property
    def ref(self) -> str:
        return self.__status_mgr.ref_snapshot

    @property
    def temp_path(self) -> Path:
        """Get the temporary path for the backup task."""
        return Path(TEMP_DIR) / self.pool / f"{self.type}_{self.date.strftime('%Y-%m-%d')}"

    def run(self):
        if self.__status_mgr.is_no_task:
            return

        (stage, current, total) = self.__status_mgr.get_stage()

        if BackupStatusManager.is_error_stage(current, total):
            logger.error(f"Error in stage: {stage}, current: {current}, total: {total}")
            return

        match stage:
            case "export":
                self.__handle_export()

            case "compress":
                self.__handle_compress(current - 1)

            case "compress_test":
                pass

            case "encrypt":
                pass

            case "upload":
                self.__handle_update(current - 1)

            case "remove":
                pass

            case "done":
                self.__status_mgr.dequeue_backup_task()
                return

            case _:
                msg = f"Unknown stage {stage}"
                logger.critical(msg)
                raise ValueError(msg)

    def __handle_export(self):
        num_parts = self.__snapshot_mgr.export(self.pool, self.base, self.ref, str(self.temp_path))
        self.__status_mgr.set_stage_export(num_parts)
        pass

    def __handle_compress(self, index: int):
        filename = self.temp_path / f"{self.__snapshot_mgr.filename}{index:06}"
        compressed_filename = self.__compress_mgr.compress(str(filename))
        self.__status_mgr.set_stage_compress(index + 1)

        if not self.__compress_mgr.verify(compressed_filename):
            raise RuntimeError(f"Compression failed for {compressed_filename}")
        self.__status_mgr.set_stage_compress_test(index + 1)

        _encrypted_filename = self.__encrypt_mgr.encrypt(compressed_filename)
        self.__file_mgr.delete(str(compressed_filename))  # delete the original file
        self.__status_mgr.set_stage_encrypt(index + 1)

    def __handle_update(self, index: int):
        filename = (
            self.temp_path
            / f"{self.__snapshot_mgr.filename}{index:06}"
            / self.__compress_mgr.extension
            / self.__encrypt_mgr.extension
        )

        tags = {"backup-type": self.type}
        metadata = {
            "pool": self.pool,
            "base-snapshot": self.base,
            "ref-snapshot": self.ref,
        }

        self.__remote_mgr.upload(str(filename), self.bucket, str(filename), tags, metadata)
        self.__status_mgr.set_stage_upload(index + 1)
        self.__file_mgr.delete(str(filename))  # delete the uploaded file
        self.__status_mgr.set_stage_remove(index + 1)
