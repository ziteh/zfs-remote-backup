import os
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Literal

import msgpack

from app.define import STATUS_FILENAME, WORK_DIR, BackupType

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
    date: datetime
    type: BackupType
    pool: str


@dataclass(slots=True)
class Stage:
    exported: bool
    compressed: int
    compress_tested: int
    encrypted: int
    uploaded: int
    removed: int


@dataclass(slots=True)
class CurrentTask:
    base: str
    incr: str
    split_quantity: int
    stage: Stage


@dataclass(slots=True)
class BackupStatus:
    last_record: datetime
    queue: list[Task]
    current: CurrentTask


def load_status() -> BackupStatus:
    def decode(obj: dict[str, Any]):
        if "__datetime__" in obj:
            return datetime.fromisoformat(obj["value"])  # type: ignore
        return obj

    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    with open(status_file_path, "rb") as f:
        data: dict[str, Any] = msgpack.load(f, object_hook=decode)
        return BackupStatus(**data)


def save_status(status: BackupStatus):
    def encode(obj: Any) -> dict[str, Any]:
        if isinstance(obj, datetime):
            return {"__datetime__": True, "value": obj.isoformat()}
        raise TypeError(f"Type {type(obj)} not serializable")

    status.last_record = datetime.now()

    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    with open(status_file_path, "wb") as f:
        msgpack.dump(asdict(status), f, default=encode)  # type: ignore


def check_stage(status: BackupStatus) -> tuple[BackupTaskStage, int, int]:
    stage = status.current.stage
    if not stage.exported:
        return ("export", 0, 0)

    split_qty = status.current.split_quantity
    if split_qty <= 0:
        return ("export", -1, split_qty)  # error

    if stage.compressed > split_qty:
        return ("compress", -stage.compressed, -split_qty)  # error
    elif stage.compressed < split_qty:
        return ("compress", stage.compressed, split_qty)

    if stage.compress_tested > split_qty:
        return ("compress_test", -stage.compress_tested, -split_qty)  # error
    elif stage.compress_tested < split_qty:
        return ("compress_test", stage.compress_tested, split_qty)

    if stage.encrypted > split_qty:
        return ("encrypt", -stage.encrypted, -split_qty)  # error
    elif stage.encrypted < split_qty:
        return ("encrypt", stage.encrypted, split_qty)

    if stage.uploaded > split_qty:
        return ("upload", -stage.uploaded, -split_qty)  # error
    elif stage.uploaded < split_qty:
        return ("upload", stage.uploaded, split_qty)

    if stage.removed > split_qty:
        return ("remove", -stage.removed, -split_qty)  # error
    elif stage.removed < split_qty:
        return ("remove", stage.removed, split_qty)

    return ("done", 0, split_qty)


def is_error_stage(current: int, total: int) -> bool:
    return total <= 0 or current <= 0


def is_nothing_to_do(status: BackupStatus) -> bool:
    return len(status.queue) == 0


def get_target_pools() -> list[str]:
    pools_str = os.getenv("TARGET_POOLS")
    if pools_str is None:
        return []  # Environment variable not found

    return pools_str.split(";")
