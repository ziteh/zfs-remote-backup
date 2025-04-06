from pathlib import Path
from define import BackupType, ENCODING as ENC, NEWLINE as NL


RECORD_DIR = r"/app/record/"

LATEST_FULL_FILENAME = "latest_full.txt"
LATEST_DIFF_FILENAME = "latest_diff.txt"
LATEST_INCR_FILENAME = "latest_incr.txt"

FILENAME_MAP: dict[BackupType, str] = {
    "full": LATEST_FULL_FILENAME,
    "diff": LATEST_DIFF_FILENAME,
    "incr": LATEST_INCR_FILENAME,
}


def read_latest(type: BackupType) -> str:
    full_path = __get_path(type)

    with open(full_path, "r", encoding=ENC, newline=NL) as f:
        latest_snapshot = f.read()

    return latest_snapshot


def update_latest(type: BackupType, new_snapshot: str) -> str:
    full_path = __get_path(type)

    with open(full_path, "w", encoding=ENC, newline=NL) as f:
        f.write(new_snapshot)

    return new_snapshot


def __get_path(type: BackupType) -> Path:
    filename = FILENAME_MAP.get(type)
    if filename is None:
        raise ValueError(f"Invalid backup type: {type}")

    record_path = Path(RECORD_DIR)
    full_path = record_path / filename

    return full_path
