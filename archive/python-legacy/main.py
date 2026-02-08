import os
from pathlib import Path

from loguru import logger

from app.backup_manager import BackupTaskManager
from app.compress_handler import ZstdCompressor
from app.define import STATUS_FILENAME, WORK_DIR
from app.encrypt_handler import AgeEncryptor
from app.file_handler import OsFileHandler
from app.remote_handler import AwsS3Oss
from app.snapshot_handler import ZfsSnapshotHandler
from app.status import MsgpackBackupStatusIo


def main():
    public_key = os.getenv("AGE_PUBLIC_KEY")
    if public_key is None:
        raise ValueError("AGE_PUBLIC_KEY environment variable is not set")

    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    status_io = MsgpackBackupStatusIo(status_file_path)
    snapshot_mgr = ZfsSnapshotHandler()
    remote_mgr = AwsS3Oss()
    compress_mgr = ZstdCompressor()
    encrypt_mgr = AgeEncryptor(public_key)
    file_mgr = OsFileHandler()

    backup_task_manager = BackupTaskManager(
        status_io,
        snapshot_mgr,
        remote_mgr,
        compress_mgr,
        encrypt_mgr,
        file_mgr,
    )

    backup_task_manager.run()
    logger.info("Backup task completed successfully.")


if __name__ == "__main__":
    main()
