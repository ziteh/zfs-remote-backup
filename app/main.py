import os
from pathlib import Path

from loguru import logger

from app.backup_manager import BackupTaskManager
from app.compress_encrypt import AgeEncryption, ZstdCompression
from app.define import STATUS_FILENAME, WORK_DIR
from app.file_manager import OsFileManager
from app.status import (
    MsgpackBackupStatusIo,
)
from app.upload import AwsS3
from app.zfs import ZfsSnapshotManager


def main():
    public_key = os.getenv("AGE_PUBLIC_KEY")
    if public_key is None:
        raise ValueError("AGE_PUBLIC_KEY environment variable is not set")

    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    status_io = MsgpackBackupStatusIo(status_file_path)
    snapshot_mgr = ZfsSnapshotManager()
    remote_mgr = AwsS3()
    compress_mgr = ZstdCompression()
    encrypt_mgr = AgeEncryption(public_key)
    file_mgr = OsFileManager()

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
