import os
from abc import ABCMeta, abstractmethod
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Literal

import msgpack

from app.define import BackupType
from app.file_handler import MockFileSystem
from app.latest_snapshot import read_latest
from app.snapshot_handler import SnapshotHandler

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


class BackupStatusIo(metaclass=ABCMeta):
    @abstractmethod
    def load(self) -> BackupStatusRaw:
        raise NotImplementedError()

    @abstractmethod
    def save(self, status: BackupStatusRaw) -> None:
        raise NotImplementedError()


class MsgpackBackupStatusIo(BackupStatusIo):
    __file_path: Path

    def __init__(self, file_path: Path) -> None:
        self.__file_path = file_path

    def load(self) -> BackupStatusRaw:
        if not self.__file_path.exists():
            # File not exists, create a new status
            status = BackupStatusRaw(
                datetime.now(), [], CurrentTask("", "", 0, Stage(False, 0, 0, 0, 0, 0))
            )
            self.save(status)
            return status

        with open(self.__file_path, "rb") as f:
            data: dict[str, Any] = msgpack.load(f, object_hook=self.__decode)
            status = BackupStatusRaw(**data)
            return status

    def save(self, status: BackupStatusRaw) -> None:
        status.last_record = datetime.now()

        with open(self.__file_path, "wb") as f:
            msgpack.dump(asdict(status), f, default=self.__encode)  # type: ignore

    def __decode(self, obj: dict[str, Any]):
        if "__datetime__" in obj:
            return datetime.fromisoformat(obj["value"])  # type: ignore

        return obj

    def __encode(self, obj: Any) -> dict[str, Any]:
        if isinstance(obj, datetime):
            return {"__datetime__": True, "value": obj.isoformat()}

        raise TypeError(f"Type {type(obj)} not serializable")


class MockBackupStatusIo(BackupStatusIo):
    _file_system: MockFileSystem
    _filename: str
    status: BackupStatusRaw
    saved: list[BackupStatusRaw]

    def __init__(
        self,
        file_system: MockFileSystem,
        filename: str,
        init_status: BackupStatusRaw | None = None,
    ) -> None:
        if init_status is None:
            init_status = BackupStatusRaw(
                last_record=datetime.now(),
                queue=[],
                current=CurrentTask(
                    base="__base",
                    ref="__ref",
                    split_quantity=0,
                    stage=Stage(
                        exported=False,
                        compressed=0,
                        compress_tested=0,
                        encrypted=0,
                        uploaded=0,
                        removed=0,
                    ),
                ),
            )

        self._filename = filename
        self._file_system = file_system
        self.save(init_status)

    def load(self) -> BackupStatusRaw:
        return self._file_system.read(self._filename)

    def save(self, status: BackupStatusRaw) -> None:
        self._file_system.save(self._filename, status)


class BackupStatusManager:
    __status: BackupStatusRaw
    __status_manager: BackupStatusIo

    def __init__(self, manager: BackupStatusIo, snapshot_handler: SnapshotHandler) -> None:
        self._snapshot_handler = snapshot_handler
        self.__status_manager = manager
        self.__status = manager.load()
        self.load_backup_task()

    @property
    def is_no_task(self) -> bool:
        return len(self.__status.queue) == 0

    @property
    def pool(self) -> str:
        return self.__status.queue[0].pool

    @property
    def target_date(self) -> datetime:
        return self.__status.queue[0].date

    @property
    def backup_type(self) -> BackupType:
        return self.__status.queue[0].type

    @property
    def base_snapshot(self) -> str:
        return self.__status.current.base

    @property
    def ref_snapshot(self) -> str:
        return self.__status.current.ref

    @property
    def split(self) -> int:
        return self.__status.current.split_quantity

    @staticmethod
    def is_error_stage(current: int, total: int) -> bool:
        return total < 0 or current < 0

    def __save(self) -> None:
        self.__status_manager.save(self.__status)

    def set_stage_export(self, parts: int | None):
        """Sets the export stage of the backup status.

        Args:
            parts: Number of parts export and split. `None` if not exported.
        """

        if parts is None:
            self.__status.current.stage.exported = False
            self.__status.current.split_quantity = 0
        else:
            self.__status.current.stage.exported = True
            self.__status.current.split_quantity = parts

        self.__save()

    def set_stage_compress(self, done: int):
        self.__status.current.stage.compressed = done
        self.__save()

    def set_stage_compress_test(self, done: int):
        self.__status.current.stage.compress_tested = done
        self.__save()

    def set_stage_encrypt(self, done: int):
        self.__status.current.stage.encrypted = done
        self.__save()

    def set_stage_upload(self, done: int):
        self.__status.current.stage.uploaded = done
        self.__save()

    def set_stage_remove(self, done: int):
        self.__status.current.stage.removed = done
        self.__save()

    def enqueue_backup_task(self, task: Task) -> int:
        self.__status.queue.append(task)
        self.__save()
        return len(self.__status.queue)

    def dequeue_backup_task(self) -> int:
        if len(self.__status.queue) == 0:
            return 0

        _task = self.__status.queue.pop(0)
        self.load_backup_task()
        self.__save()
        return len(self.__status.queue)

    def load_backup_task(self) -> Task | None:
        if len(self.__status.queue) == 0:
            return None

        task = self.__status.queue[0]

        # Clear the task status
        self.__status.current.split_quantity = 0
        self.__status.current.stage.exported = False
        self.__status.current.stage.compressed = 0
        self.__status.current.stage.compress_tested = 0
        self.__status.current.stage.encrypted = 0
        self.__status.current.stage.uploaded = 0
        self.__status.current.stage.removed = 0

        # Set the snapshot
        snapshots = self._snapshot_handler.list(task.pool)
        self.__status.current.base = snapshots[0]

        match task.type:
            case "full":
                # Full backup, no reference snapshot
                self.__status.current.ref = ""

            case "diff":
                # Differential backup, use the latest full snapshot as reference
                self.__status.current.ref = read_latest(task.pool, "full")

            case "incr":
                # Incremental backup, use the latest differential snapshot as reference
                self.__status.current.ref = read_latest(task.pool, "diff")

            case _:
                msg = f"Unknown backup type: {task.type}"
                raise ValueError(msg)

        return task

    def get_stage(self) -> tuple[BackupTaskStage, int, int]:
        stage = self.__status.current.stage
        if not stage.exported:
            return ("export", 0, 0)

        split_qty = self.__status.current.split_quantity
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
