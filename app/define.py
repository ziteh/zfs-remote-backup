from datetime import datetime
from typing import Literal
from dataclasses import dataclass

type BackupType = Literal["full", "diff", "incr"]

ENCODING = "utf-8"
NEWLINE = "\n"

LOG_DIR = "/app/log/"

WORK_DIR = "/app/work/"
STATUS_FILENAME = "status.toml"
