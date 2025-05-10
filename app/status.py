import os
from abc import ABCMeta, abstractmethod
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Literal

import msgpack

from app.define import BackupType
from app.file_handler import FileHandler
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
    _file_system: FileHandler
    _filename: str
    status: BackupStatusRaw
    saved: list[BackupStatusRaw]

    def __init__(
        self,
        file_system: FileHandler,
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
