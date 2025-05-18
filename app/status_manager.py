from abc import ABCMeta, abstractmethod
from dataclasses import dataclass
from datetime import datetime
from typing import Any, Literal

from app.define import BackupType
from app.file_handler import FileHandler
from app.snapshot_handler import SnapshotHandler

type BackupTaskStage = Literal[
    "snapshot_export",
    "snapshot_test",
    "split",
    "compress",
    "encrypt",
    "upload",
    "verify",
    "clear",
    "done",
    # "error",
]


@dataclass(slots=True)
class Stage:
    snapshot_exported: str
    """Exported snapshot name, empty if not exported"""

    snapshot_tested: bool
    """Snapshot test status"""

    spit: list[bytes]  #  hash
    """List of split hashes, empty if not split"""

    compressed: int
    """Compressed file count"""

    encrypted: int
    """Encrypted file count"""

    uploaded: int
    """Uploaded file count"""

    cleared: int
    """Cleared file count"""

    verify: bool
    """Verification status"""


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


class StatusFilesIo(metaclass=ABCMeta):
    @abstractmethod
    def load_task_queue(self) -> TaskQueue:
        raise NotImplementedError()

    @abstractmethod
    def load_current_task(self) -> CurrentTask:
        raise NotImplementedError()

    @abstractmethod
    def load_latest_snapshot(self) -> DatasetLatestSnapshot:
        raise NotImplementedError()

    @abstractmethod
    def save_task_queue(self, task_queue: TaskQueue) -> None:
        raise NotImplementedError()

    @abstractmethod
    def save_current_task(self, current_task: CurrentTask) -> None:
        raise NotImplementedError()

    @abstractmethod
    def save_latest_snapshot(self, latest_snapshot: DatasetLatestSnapshot) -> None:
        raise NotImplementedError()


class MockStatusFilesIo(StatusFilesIo):
    def __init__(
        self,
        tasks_filename: str,
        current_filename: str,
        latest_filename: str,
        file_system: FileHandler,
    ) -> None:
        self._file_system = file_system

        self._task_filename = tasks_filename
        self._current_filename = current_filename
        self._latest_filename = latest_filename

    def load_task_queue(self) -> TaskQueue:
        if not self._file_system.check_file(self._task_filename):
            raise FileNotFoundError(f"File '{self._task_filename}' not found.")

        return self._file_system.read(self._task_filename)

    def load_current_task(self) -> CurrentTask:
        if not self._file_system.check_file(self._current_filename):
            raise FileNotFoundError(f"File '{self._current_filename}' not found.")

        return self._file_system.read(self._current_filename)

    def load_latest_snapshot(self) -> DatasetLatestSnapshot:
        if not self._file_system.check_file(self._latest_filename):
            raise FileNotFoundError(f"File '{self._latest_filename}' not found.")

        return self._file_system.read(self._latest_filename)

    def save_task_queue(self, task_queue: TaskQueue) -> None:
        self._file_system.save(self._task_filename, task_queue)

    def save_current_task(self, current_task: CurrentTask) -> None:
        self._file_system.save(self._current_filename, current_task)

    def save_latest_snapshot(self, latest_snapshot: DatasetLatestSnapshot) -> None:
        self._file_system.save(self._latest_filename, latest_snapshot)


class StatusManager:
    def __init__(
        self,
        status_io: StatusFilesIo,
        snapshot_manager: SnapshotHandler,
    ) -> None:
        self._io = status_io
        self._snapshot_manager = snapshot_manager

        self._task_queue: TaskQueue = self._io.load_task_queue()
        self._latest_snapshot: DatasetLatestSnapshot = self._io.load_latest_snapshot()
        self._current_task: CurrentTask = self._io.load_current_task()

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
        self._io.save_task_queue(self._task_queue)

    def dequeue_task(self) -> int:
        if len(self._task_queue.tasks) == 0:
            return -1
        _task = self._task_queue.tasks.pop(0)
        self._io.save_task_queue(self._task_queue)
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
                spit=[],
                compressed=-1,
                encrypted=-1,
                uploaded=-1,
                verify=False,
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
                self._current_task.ref = latest_full.name if latest_full else "ERROR_NONE"

            case "incr":
                latest_diff = self.latest_snapshot(task.dataset, "diff")
                self._current_task.ref = latest_diff.name if latest_diff else "ERROR_NONE"

        self._io.save_current_task(self._current_task)

    def update_latest_snapshot(
        self,
        dataset: str,
        type: BackupType,
        name: str,
    ) -> None:
        if dataset not in self._latest_snapshot.latest:
            self._latest_snapshot.latest[dataset] = {}

        self._latest_snapshot.latest[dataset][type] = LatestSnapshot(datetime.now(), name)
        self._io.save_latest_snapshot(self._latest_snapshot)

    def update_split_quantity(self, quantity: int) -> None:
        if quantity <= 0:
            raise ValueError("Split quantity must be greater than 0")

        self._current_task.split_quantity = quantity
        self._io.save_current_task(self._current_task)

    def update_stage(self, stage: BackupTaskStage, value: Any) -> None:
        match stage:
            case "snapshot_export":
                self._current_task.stage.snapshot_exported = value
            case "snapshot_test":
                self._current_task.stage.snapshot_tested = value
            case "split":
                self._current_task.stage.spit.append(value)
            case "compress":
                self._current_task.stage.compressed = value
            case "encrypt":
                self._current_task.stage.encrypted = value
            case "upload":
                self._current_task.stage.uploaded = value
            case "verify":
                self._current_task.stage.verify = value
            case "clear":
                self._current_task.stage.cleared = value
            case _:
                raise ValueError(f"Unknown stage: {stage}")

        self._io.save_current_task(self._current_task)

    def restore_status(self) -> tuple[BackupTaskStage, int, int]:
        self._task_queue = self._io.load_task_queue()
        if len(self._task_queue.tasks) == 0:
            return ("done", 0, 0)

        self._current_task = self._io.load_current_task()
        stage = self._current_task.stage

        if not stage.snapshot_exported:
            return ("snapshot_export", 0, 0)

        if not stage.snapshot_tested:
            return ("snapshot_test", 0, 0)

        split_count = len(stage.spit)
        total_split_qty = self._current_task.split_quantity

        if split_count > total_split_qty:
            return ("split", -total_split_qty, -split_count)  # error

        if split_count == 0:
            return ("split", total_split_qty, 0)

        # Check if all stages are completed
        if (
            split_count == self._current_task.split_quantity
            and stage.compressed == split_count
            and stage.encrypted == split_count
            and stage.uploaded == split_count
            and stage.cleared == split_count
        ):
            if stage.verify:
                return ("done", 0, 0)
            else:
                return ("verify", split_count, 0)

        # Check if any stage has a negative value
        if stage.compressed < 0:
            return ("compress", -split_count, stage.compressed)  # error

        if stage.encrypted < 0:
            return ("encrypt", -split_count, stage.encrypted)  # error

        if stage.uploaded < 0:
            return ("upload", -split_count, stage.uploaded)  # error

        if stage.cleared < 0:
            return ("clear", -split_count, stage.cleared)  # error

        # Check if any stage is not completed
        if stage.compressed < split_count:
            return ("compress", split_count, stage.compressed)
        elif stage.compressed > split_count:
            return ("compress", -split_count, -stage.compressed)  # error

        if stage.encrypted < split_count:
            return ("encrypt", split_count, stage.encrypted)
        elif stage.encrypted > split_count:
            return ("encrypt", -split_count, -stage.encrypted)  # error

        if stage.uploaded < split_count:
            return ("upload", split_count, stage.uploaded)
        elif stage.uploaded > split_count:
            return ("upload", -split_count, -stage.uploaded)  # error

        if stage.cleared < split_count:
            return ("clear", split_count, stage.cleared)
        elif stage.cleared > split_count:
            return ("clear", -split_count, -stage.cleared)  # error

        return ("split", total_split_qty, split_count)

    def latest_snapshot(self, dataset: str, type: BackupType) -> LatestSnapshot | None:
        return self._latest_snapshot.latest[dataset][type]
