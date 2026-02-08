# from datetime import datetime
# from dataclasses import dataclass
from datetime import datetime
from typing import Literal

type BackupType = Literal["full", "diff", "incr"]

ENCODING = "utf-8"
NEWLINE = "\n"

LOG_DIR = "/app/log/"

WORK_DIR = "/app/work/"
STATUS_FILENAME = "status.msgpack"

RECORD_DIR = "/app/record/"
LATEST_SNAPSHOT_FILENAME = "latest.txt"

SPLIT_SIZE = "4G"
# ‘b’  =>            512 ("blocks")
# ‘KB’ =>           1000 (KiloBytes)
# ‘K’  =>           1024 (KibiBytes)
# ‘MB’ =>      1000*1000 (MegaBytes)
# ‘M’  =>      1024*1024 (MebiBytes)
# ‘GB’ => 1000*1000*1000 (GigaBytes)
# ‘G’  => 1024*1024*1024 (GibiBytes)
# https://www.gnu.org/software/coreutils/manual/html_node/split-invocation.html

TEMP_DIR = "/backup_temp/"


def get_export_filename(pool: str, type: BackupType, date: datetime) -> str:
    return f"{pool}_{type}_{date.isoformat()}_p"


EXPORT_FILENAME = get_export_filename


class SystemShutdownError(Exception):
    """Exception raised when the system is shutting down."""

    pass
