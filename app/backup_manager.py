from datetime import datetime
from pathlib import Path

from loguru import logger

from app.compress_handler import CompressionHandler
from app.define import TEMP_DIR, BackupType, SystemShutdownError
from app.encrypt_handler import EncryptionHandler
from app.extract_handler import Splitter
from app.file_handler import FileHandler
from app.hash_handler import Hasher
from app.remote_handler import RemoteStorageHandler
from app.snapshot_handler import SnapshotHandler
from app.status_manager import StatusManager


class BackupTaskManager:
    def __init__(
        self,
        status_io: StatusManager,
        snapshot_handler: SnapshotHandler,
        splitter: Splitter,
        remote_handler: RemoteStorageHandler,
        compression_handler: CompressionHandler,
        encryption_handler: EncryptionHandler,
        file_handler: FileHandler,
        hasher: Hasher,
    ):
        self.__snapshot_mgr = snapshot_handler
        self.__remote_mgr = remote_handler
        self.__compress_mgr = compression_handler
        self.__encrypt_mgr = encryption_handler
        self.__file_mgr = file_handler
        self.__splitter = splitter

        self.__hasher = hasher

        self.__status_manager = status_io

    @property
    def dataset(self) -> str:
        """ZFS dataset"""
        return self.__status_manager.task_queue.tasks[0].dataset

    @property
    def type(self) -> BackupType:
        """Backup type"""
        return self.__status_manager.task_queue.tasks[0].type

    @property
    def date(self) -> datetime:
        """Target date"""
        return self.__status_manager.task_queue.tasks[0].date

    @property
    def base(self) -> str:
        """Base snapshot"""
        return self.__status_manager.current_task.base

    @property
    def ref(self) -> str:
        """Ref snapshot"""
        return self.__status_manager.current_task.ref

    @property
    def are_tasks_available(self) -> bool:
        return len(self.__status_manager.task_queue.tasks) != 0

    @property
    def temp_dir(self) -> Path:
        """Get the temporary path for the backup task."""
        return Path(TEMP_DIR) / self.dataset / f"{self.type}_{self.date.strftime('%Y-%m-%d')}"

    def run(self, auto: bool = False) -> None:
        while self.are_tasks_available:
            (stage, total, current) = self.__status_manager.restore_status()

            if self.__is_error_stage(current, total):
                logger.error(f"Error in stage: {stage}, current: {current}, total: {total}")
                return

            try:
                match stage:
                    case "snapshot_export":
                        self.__handle_export_stage()

                    case "snapshot_test":
                        self.__handle_export_test_stage()

                    case "split":
                        self.__handle_split_stage(current)

                    case "compress":
                        self.__handle_compress_stage(current)

                    case "encrypt":
                        self.__handle_encrypt_stage(current)

                    case "upload":
                        self.__handle_update_stage(current)

                    case "verify":
                        self.__handle_verify_stage()

                    case "clear":
                        self.__handle_clear_stage()

                    case "done":
                        self.__handle_done_stage()

                    case _:
                        msg = f"Unknown stage {stage}"
                        logger.critical(msg)
                        raise ValueError(msg)

            except SystemShutdownError as e:
                logger.error(f"System is shutting down. {e}")
                return

            if not auto:
                break

    def __handle_export_stage(self):
        save_filepath = self.__snapshot_mgr.export(
            self.temp_dir,
            self.dataset,
            self.base,
            self.ref,
        )

        # calculate the split quantity
        file_size = self.__file_mgr.get_file_size(save_filepath)
        split_quantity = file_size // self.__splitter.chunk_size
        self.__status_manager.update_split_quantity(split_quantity)

        self.__status_manager.update_stage("snapshot_export", save_filepath.name)

    def __handle_export_test_stage(self):
        ok = self.__snapshot_mgr.verify(self.dataset, self.temp_dir)
        self.__status_manager.update_stage("snapshot_test", ok)

    def __handle_split_stage(self, index: int):
        snapshot_filepath = self.temp_dir / self.__snapshot_mgr.filename
        in_hash = self.__status_manager.current_task.stage.spit[-1] if index > 0 else b""

        out_hash = self.__splitter.split(snapshot_filepath, index, in_hash)

        self.__status_manager.update_stage("split", out_hash)

    def __handle_compress_stage(self, index: int):
        split_filepath = self.temp_dir / (
            self.__snapshot_mgr.filename + self.__splitter.extension(index)
        )

        compressed_filepath = self.__compress_mgr.compress(split_filepath)
        ok = self.__compress_mgr.verify(compressed_filepath)
        if ok:
            self.__status_manager.update_stage("compress", index)
            self.__file_mgr.delete(split_filepath)

    def __handle_encrypt_stage(self, index: int):
        compressed_filepath = self.temp_dir / (
            self.__snapshot_mgr.filename
            + self.__splitter.extension(index)
            + self.__compress_mgr.extension
        )

        # calculate the hash of the compressed file
        self.__hasher.reset()
        self.__hasher.cal_file(compressed_filepath)
        hash = self.__hasher.digest

        encrypted_filepath = self.__encrypt_mgr.encrypt(compressed_filepath)

        # verify the hash of the encrypted file
        decrypted_filepath = self.__encrypt_mgr.decrypt(encrypted_filepath)
        self.__hasher.reset()
        self.__hasher.cal_file(decrypted_filepath)
        self.__file_mgr.delete(decrypted_filepath)

        # compare the hash of the decrypted file with the original hash
        if hash == self.__hasher.digest:
            self.__status_manager.update_stage("encrypt", index)
            self.__file_mgr.delete(compressed_filepath)

    def __handle_update_stage(self, index: int):
        encrypted_filepath = self.temp_dir / (
            self.__snapshot_mgr.filename
            + self.__splitter.extension(index)
            + self.__compress_mgr.extension
            + self.__encrypt_mgr.extension
        )

        tags = {"backup-type": self.type}
        metadata = {
            "dataset": self.dataset,
            "base-snapshot": self.base,
            "ref-snapshot": self.ref,
        }

        self.__remote_mgr.upload(encrypted_filepath, encrypted_filepath, tags, metadata)
        self.__status_manager.update_stage("upload", index)

        self.__file_mgr.delete(encrypted_filepath)

    def __handle_verify_stage(self):
        snapshot_filepath = self.temp_dir / self.__snapshot_mgr.filename

        # calculate the hash of the original snapshot file
        self.__hasher.reset()
        self.__hasher.cal_file(snapshot_filepath)
        full_file_hash = self.__hasher.digest

        # calculate the hash of the split files
        self.__hasher.reset()
        # for hash in self.__status_manager.current_task.stage.spit:
        #     self.__hasher.update(hash)
        split_file_hash = self.__status_manager.current_task.stage.spit[-1]

        same = full_file_hash == split_file_hash
        self.__status_manager.update_stage("verify", same)

    def __handle_clear_stage(self):
        snapshot_filepath = self.temp_dir / self.__snapshot_mgr.filename
        self.__file_mgr.delete(snapshot_filepath)
        self.__status_manager.update_stage("clear", True)

    def __handle_done_stage(self):
        self.__status_manager.update_latest_snapshot(self.dataset, self.type, self.base)
        self.__status_manager.dequeue_task()  # next task

    def __is_error_stage(self, current: int, total: int) -> bool:
        return total < 0 or current < 0
