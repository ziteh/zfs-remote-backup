import os
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Literal

import msgpack

from app.define import STATUS_FILENAME, WORK_DIR, BackupType
from app.latest_snapshot import read_latest
from app.zfs import Zfs

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
    """Task target date"""

    type: BackupType
    """Task backup type"""

    pool: str
    """ZFS pool name"""


@dataclass(slots=True)
class Stage:
    exported: bool
    """Snapshot exported"""

    compressed: int
    """Number of compressed files"""

    compress_tested: int
    """Number of compressed files tested"""

    encrypted: int
    """Number of encrypted files"""

    uploaded: int
    """Number of uploaded files"""

    removed: int
    """Number of cleaned files"""


@dataclass(slots=True)
class CurrentTask:
    base: str
    """Base snapshot name"""

    ref: str
    """Reference snapshot name"""

    split_quantity: int
    """Number of split files"""

    stage: Stage
    """Current stage of the backup task"""


@dataclass(slots=True)
class BackupStatusRaw:
    last_record: datetime
    """Last status update time"""

    queue: list[Task]
    """List of backup tasks"""

    current: CurrentTask
    """Current backup task"""


class BackupStatus:
    _status: BackupStatusRaw

    def __init__(self) -> None:
        self._open()

    @property
    def task(self) -> Task | None:
        if len(self._status.queue) == 0:
            return None

        return self._status.queue[0]

    @property
    def backup_type(self) -> BackupType | None:
        task = self.task
        if task is None:
            return None

        return task.type

    @property
    def pool(self) -> str | None:
        task = self.task
        if task is None:
            return None

        return task.pool

    @property
    def date(self) -> datetime | None:
        task = self.task
        if task is None:
            return None

        return task.date

    @property
    def base_snapshot(self) -> str:
        task = self.task
        if task is None:
            return ""

        return self._status.current.base

    @property
    def ref_snapshot(self) -> str:
        task = self.task
        if task is None or self.backup_type != "full":
            return ""

        return self._status.current.ref

    @staticmethod
    def is_error_stage(current: int, total: int) -> bool:
        return total <= 0 or current <= 0

    def set_stage_export(self, parts: int | None):
        """Sets the export stage of the backup status.

        Args:
            parts: Number of parts export and split. `None` if not exported.
        """

        if parts is None:
            self._status.current.stage.exported = False
            self._status.current.split_quantity = 0
        else:
            self._status.current.stage.exported = True
            self._status.current.split_quantity = parts

        self._save()

    def set_stage_compress(self, done: int):
        self._status.current.stage.compressed = done
        self._save()

    def enqueue_back_task(self, task: Task) -> int:
        self._status.queue.append(task)
        self._save()
        return len(self._status.queue)

    def dequeue_back_task(self) -> int:
        if len(self._status.queue) == 0:
            return 0

        _task = self._status.queue.pop(0)
        self.load_backup_task()
        self._save()
        return len(self._status.queue)

    def load_backup_task(self) -> Task | None:
        if len(self._status.queue) == 0:
            return None

        task = self._status.queue[0]

        # Clear the task status
        self._status.current.split_quantity = 0
        self._status.current.stage.exported = False
        self._status.current.stage.compressed = 0
        self._status.current.stage.compress_tested = 0
        self._status.current.stage.encrypted = 0
        self._status.current.stage.uploaded = 0
        self._status.current.stage.removed = 0

        # Set the snapshot
        snapshots = Zfs.list_snapshots(task.pool)
        self._status.current.base = snapshots[0]

        match task.type:
            case "full":
                # Full backup, no reference snapshot
                self._status.current.ref = ""

            case "diff":
                # Differential backup, use the latest full snapshot as reference
                self._status.current.ref = read_latest(task.pool, "full")

            case "incr":
                # Incremental backup, use the latest differential snapshot as reference
                self._status.current.ref = read_latest(task.pool, "diff")

            case _:
                msg = f"Unknown backup type: {task.type}"
                raise ValueError(msg)

        return task

    def _open(self) -> None:
        def decode(obj: dict[str, Any]):
            if "__datetime__" in obj:
                return datetime.fromisoformat(obj["value"])  # type: ignore
            return obj

        status_file_path = Path(WORK_DIR) / STATUS_FILENAME

        if not status_file_path.exists():
            # File not exists
            status = BackupStatusRaw(
                datetime.now(), [], CurrentTask("", "", 0, Stage(False, 0, 0, 0, 0, 0))
            )
            self._status = status
            self._save()

        with open(status_file_path, "rb") as f:
            data: dict[str, Any] = msgpack.load(f, object_hook=decode)
            status = BackupStatusRaw(**data)
            self._status = status

    def _save(self) -> None:
        def encode(obj: Any) -> dict[str, Any]:
            if isinstance(obj, datetime):
                return {"__datetime__": True, "value": obj.isoformat()}
            raise TypeError(f"Type {type(obj)} not serializable")

        self._status.last_record = datetime.now()

        status_file_path = Path(WORK_DIR) / STATUS_FILENAME
        with open(status_file_path, "wb") as f:
            msgpack.dump(asdict(self._status), f, default=encode)  # type: ignore

    def get_stage(self) -> tuple[BackupTaskStage, int, int]:
        stage = self._status.current.stage
        if not stage.exported:
            return ("export", 0, 0)

        split_qty = self._status.current.split_quantity
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


def get_target_pools() -> list[str]:
    pools_str = os.getenv("TARGET_POOLS")
    if pools_str is None:
        return []  # Environment variable not found

    return pools_str.strip().split(";")
