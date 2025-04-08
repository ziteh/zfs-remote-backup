from datetime import datetime
from typing import Literal
from dataclasses import dataclass

type BackupType = Literal["full", "diff", "incr"]

ENCODING = "utf-8"
NEWLINE = "\n"

LOG_DIR = "/app/log/"

WORK_DIR = "/app/work/"
STATUS_FILENAME = "status.toml"

type BackupTaskStage = Literal[
    "export",
    "compress",
    "compress_test",
    "encrypt",
    "upload",
    "remove",
    "done",
    # "error",
]


@dataclass(slots=True)
class Task:
    type: BackupType
    target: datetime
    last_record: datetime


@dataclass(slots=True)
class Snapshot:
    pool: str
    base: str
    incr: str
    split_quantity: int


@dataclass(slots=True)
class Stage:
    exported: bool
    compressed: int
    compress_tested: int
    encrypted: int
    uploaded: int
    removed: int


@dataclass(slots=True)
class BackupStatus:
    task: Task
    snapshot: Snapshot
    stage: Stage
