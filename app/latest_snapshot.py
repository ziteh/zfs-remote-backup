from pathlib import Path

from app.define import ENCODING, LATEST_SNAPSHOT_FILENAME, NEWLINE, RECORD_DIR, BackupType


def read_latest(pool: str, type: BackupType) -> str:
    full_path = __get_path(pool, type)

    with open(full_path, "r", encoding=ENCODING, newline=NEWLINE) as f:
        latest_snapshot = f.read()

    return latest_snapshot


def update_latest(pool: str, type: BackupType, new_snapshot: str):
    full_path = __get_path(pool, type)

    with open(full_path, "w", encoding=ENCODING, newline=NEWLINE) as f:
        f.write(new_snapshot)


def __get_path(pool: str, type: BackupType) -> Path:
    """Get the full path for the latest backup file based on the pool and backup type.

    Args:
        pool: The name of the ZFS pool for which the backup is being accessed.
        type: The type of backup (full, diff, incr).

    Returns:
        The full path to the latest backup file.
    """

    filename = f"{type}_{LATEST_SNAPSHOT_FILENAME}"
    full_path = Path(RECORD_DIR) / pool / filename
    return full_path
