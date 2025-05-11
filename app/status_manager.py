from abc import ABCMeta, abstractmethod
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
from typing import Any, Literal

from app.define import BackupType
from app.file_handler import FileHandler
from app.snapshot_handler import SnapshotHandler

type BackupTaskStage = Literal[
    "snapshot_export",
    "snapshot_test",
    "snapshot_hash",
    "split",
    "compress",
    "compress_test",
    "compress_hash",
    "encrypt",
    "encrypt_test",
    "encrypt_hash",
    "upload",
    "clear",
    "done",
    # "error",
]


@dataclass(slots=True)
class Stage:
    snapshot_exported: str

    snapshot_tested: bool

    snapshot_hash: bytes

    spited: list[bytes]  #  hash

    compressed: int

    compressed_test: int

    compressed_hash: bytes

    encrypted: int

    encrypted_test: int

    encrypted_hash: bytes

    uploaded: int

    cleared: int


@dataclass(slots=True)
class CurrentTask:
    base: str
    """Base snapshot name"""

    ref: str
    """Reference snapshot name, empty if no reference snapshot"""

    split_quantity: int
    """Number of split files"""

    stage: Stage
    """Current stage of the backup task"""

    stream_hash: bytes


@dataclass(slots=True)
class LatestSnapshot:
    update_time: datetime
    name: str


@dataclass(slots=True)
class DatasetLatestSnapshot:
    latest: dict[str, dict[BackupType, LatestSnapshot]]  # dataset -> type -> snapshot


@dataclass(slots=True)
class BackupTarget:
    date: datetime
    """Task target date"""

    type: BackupType
    """Task backup type"""

    dataset: str
    """ZFS dataset name"""


@dataclass(slots=True)
class TaskQueue:
    tasks: list[BackupTarget]


class StatusManager(metaclass=ABCMeta):
    _current_task: CurrentTask
    _task_queue: TaskQueue
    _latest_snapshot: DatasetLatestSnapshot

    _current_task_path: Path
    _task_queue_path: Path
    _latest_snapshot_path: Path

    def __init__(
        self,
        status_dir: Path,
        current_task_filename: str,
        task_queue_filename: str,
        latest_snapshot_filename: str,
    ) -> None:
        self._current_task_path = status_dir / current_task_filename
        self._task_queue_path = status_dir / task_queue_filename
        self._latest_snapshot_path = status_dir / latest_snapshot_filename

    @property
    @abstractmethod
    def current_task(self) -> CurrentTask:
        raise NotImplementedError()

    @property
    @abstractmethod
    def task_queue(self) -> TaskQueue:
        raise NotImplementedError()

    @abstractmethod
    def latest_snapshot(self, dataset: str, type: BackupType) -> LatestSnapshot | None:
        raise NotImplementedError()

    @abstractmethod
    def restore_status(self) -> tuple[BackupTaskStage, int, int]:
        raise NotImplementedError()

    @abstractmethod
    def update_stage(self, stage: BackupTaskStage, value: Any) -> None:
        raise NotImplementedError()

    @abstractmethod
    def update_split_quantity(self, quantity: int) -> None:
        raise NotImplementedError()

    @abstractmethod
    def update_latest_snapshot(
        self,
        dataset: str,
        type: BackupType,
        name: str,
    ) -> None:
        raise NotImplementedError()

    @abstractmethod
    def enqueue_task(
        self,
        task: BackupTarget,
    ) -> None:
        raise NotImplementedError()

    @abstractmethod
    def dequeue_task(self) -> int:
        raise NotImplementedError()


class MockStatusManager(StatusManager):
    _file_system: FileHandler

    def __init__(
        self,
        status_dir: Path,
        file_system: FileHandler,
        snapshot_manager: SnapshotHandler,
    ) -> None:
        super().__init__(status_dir, "current.dict", "queue.dict", "latest.dict")
        self._file_system = file_system
        self._snapshot_manager = snapshot_manager

        self._load_current_task()
        self._load_task_queue()
        self._load_latest_snapshot()

    @property
    def current_task(self) -> CurrentTask:
        return self._current_task

    @property
    def task_queue(self) -> TaskQueue:
        return self._task_queue

    def enqueue_task(
        self,
        task: BackupTarget,
    ) -> None:
        self._task_queue.tasks.append(task)
        self.save_task_queue(self._task_queue)

    def dequeue_task(self) -> int:
        if len(self._task_queue.tasks) == 0:
            return -1
        _task = self._task_queue.tasks.pop(0)
        self.save_task_queue(self._task_queue)
        self._load_task()
        return len(self._task_queue.tasks)

    def _load_task(self) -> None:
        if len(self._task_queue.tasks) == 0:
            return

        self._current_task = CurrentTask(
            base="",
            ref="",
            split_quantity=-1,
            stream_hash=b"",
            stage=Stage(
                snapshot_exported="",
                snapshot_tested=False,
                snapshot_hash=b"",
                spited=[],
                compressed=-1,
                compressed_test=-1,
                compressed_hash=b"",
                encrypted=-1,
                encrypted_test=-1,
                encrypted_hash=b"",
                uploaded=-1,
                cleared=-1,
            ),
        )

        task = self._task_queue.tasks[0]
        snapshots = self._snapshot_manager.list(task.dataset)
        self._current_task.base = snapshots[0]

        match task.type:
            case "full":
                self._current_task.ref = ""

            case "diff":
                latest_full = self.latest_snapshot(task.dataset, "full")
                self._current_task.ref = (
                    latest_full.name if latest_full else "ERROR_NONE"
                )

            case "incr":
                latest_diff = self.latest_snapshot(task.dataset, "diff")
                self._current_task.ref = (
                    latest_diff.name if latest_diff else "ERROR_NONE"
                )

        self.save_current_task(self._current_task)

    def update_latest_snapshot(
        self,
        dataset: str,
        type: BackupType,
        name: str,
    ) -> None:
        if dataset not in self._latest_snapshot.latest:
            self._latest_snapshot.latest[dataset] = {}

        self._latest_snapshot.latest[dataset][type] = LatestSnapshot(
            datetime.now(), name
        )

    def update_split_quantity(self, quantity: int) -> None:
        if quantity <= 0:
            raise ValueError("Split quantity must be greater than 0")

        self._current_task.split_quantity = quantity
        self.save_current_task(self._current_task)

    def update_stage(self, stage: BackupTaskStage, value: Any) -> None:
        match stage:
            case "snapshot_export":
                self._current_task.stage.snapshot_exported = value
            case "snapshot_test":
                self._current_task.stage.snapshot_tested = value
            case "snapshot_hash":
                self._current_task.stage.snapshot_hash = value
            case "split":
                self._current_task.stage.spited.append(value)
            case "compress":
                self._current_task.stage.compressed = value
            case "compress_test":
                self._current_task.stage.compressed_test = value
            case "compress_hash":
                self._current_task.stage.compressed_hash = value
            case "encrypt":
                self._current_task.stage.encrypted = value
            case "encrypt_test":
                self._current_task.stage.encrypted_test = value
            case "encrypt_hash":
                self._current_task.stage.encrypted_hash = value
            case "upload":
                self._current_task.stage.uploaded = value
            case "clear":
                self._current_task.stage.cleared = value
            case _:
                raise ValueError(f"Unknown stage: {stage}")

        self.save_current_task(self._current_task)

    def restore_status(self) -> tuple[BackupTaskStage, int, int]:
        self._load_task_queue()
        if len(self._task_queue.tasks) == 0:
            return ("done", 0, 0)

        self._load_current_task()
        stage = self._current_task.stage

        if not stage.snapshot_exported:
            return ("snapshot_export", 0, 0)

        if not stage.snapshot_tested:
            return ("snapshot_test", 0, 0)

        if not stage.snapshot_hash:
            return ("snapshot_hash", 0, 0)

        split_count = len(stage.spited)
        if split_count == 0:
            return ("split", 0, 0)
        elif split_count == self._current_task.split_quantity:
            return ("done", 0, split_count)

        if stage.compressed < split_count:
            return ("compress", split_count, stage.compressed)
        elif stage.compressed > split_count:
            return ("compress", -split_count, -stage.compressed)  # error

        if stage.compressed_test < split_count:
            return ("compress_test", split_count, stage.compressed_test)
        elif stage.compressed_test > split_count:
            return ("compress_test", -split_count, -stage.compressed_test)  # error

        if stage.compressed_hash == b"":
            return ("compress_hash", split_count, split_count)

        if stage.encrypted < split_count:
            return ("encrypt", split_count, stage.encrypted)
        elif stage.encrypted > split_count:
            return ("encrypt", -split_count, -stage.encrypted)  # error

        if stage.encrypted_test < split_count:
            return ("encrypt_test", split_count, stage.encrypted_test)
        elif stage.encrypted_test > split_count:
            return ("encrypt_test", -split_count, -stage.encrypted_test)  # error

        if stage.encrypted_hash == b"":
            return ("encrypt_hash", split_count, split_count)

        if stage.uploaded < split_count:
            return ("upload", split_count, stage.uploaded)
        elif stage.uploaded > split_count:
            return ("upload", -split_count, -stage.uploaded)  # error

        if stage.cleared < split_count:
            return ("clear", split_count, stage.cleared)
        elif stage.cleared > split_count:
            return ("clear", -split_count, -stage.cleared)  # error

        return ("split", split_count, split_count)

    def latest_snapshot(self, dataset: str, type: BackupType) -> LatestSnapshot | None:
        return self._latest_snapshot.latest[dataset][type]

    def _load_current_task(self):
        if not self._file_system.check_file(self._current_task_path):
            raise FileNotFoundError(f"File '{self._current_task_path}' not found.")

        data: CurrentTask = self._file_system.read(self._current_task_path)
        self._current_task = data

    def save_current_task(self, current_task: CurrentTask):
        self._file_system.save(self._current_task_path, self._current_task)
        self._current_task = current_task

    def _load_task_queue(self):
        if not self._file_system.check_file(self._task_queue_path):
            raise FileNotFoundError(f"File '{self._task_queue_path}' not found.")

        data: TaskQueue = self._file_system.read(self._task_queue_path)
        self._task_queue = data

    def save_task_queue(self, task_queue: TaskQueue):
        self._file_system.save(self._task_queue_path, self._task_queue)
        self._task_queue = task_queue

    def _load_latest_snapshot(self):
        if not self._file_system.check_file(self._latest_snapshot_path):
            raise FileNotFoundError(f"File '{self._latest_snapshot_path}' not found.")

        data: DatasetLatestSnapshot = self._file_system.read(self._latest_snapshot_path)
        self._latest_snapshot = data

    def save_latest_snapshot(self, latest_snapshot: DatasetLatestSnapshot):
        self._file_system.save(self._latest_snapshot_path, self._latest_snapshot)
        self._latest_snapshot = latest_snapshot
