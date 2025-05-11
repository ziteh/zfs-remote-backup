import os
import subprocess
from abc import ABCMeta, abstractmethod
from pathlib import Path

from app.define import SPLIT_SIZE, BackupType
from app.file_handler import FileHandler


class SnapshotHandler(metaclass=ABCMeta):
    @property
    @abstractmethod
    def filename(self) -> str:
        raise NotImplementedError()

    @abstractmethod
    def export(
        self,
        dataset: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: Path,
    ) -> Path:
        """Export snapshot.

        Args:
            dataset: The name of the ZFS dataset.
            base_snapshot: The base snapshot to export.
            ref_snapshot: The reference snapshot for incremental exports.
            output_dir: The directory where the output files will be saved.

        Returns:
            The exported file path.
        """
        raise NotImplementedError()

    @abstractmethod
    def test(self, dataset: str, filepath: Path) -> bool:
        raise NotImplementedError()

    @abstractmethod
    def list(self, dataset: str) -> list[str]:
        """List all snapshots in the given ZFS dataset.

        Args:
            dataset: The name of the ZFS dataset.

        Returns:
            A list of snapshot names.
        """
        raise NotImplementedError()

    @abstractmethod
    def get_latest(self, dataset: str, type: BackupType) -> str | None:
        """Get the latest snapshot in the given ZFS dataset.

        Args:
            dataset: The name of the ZFS dataset.
            type: The type of backup.

        Returns:
            The name of the latest snapshot or None if no snapshot exists.
        """
        raise NotImplementedError()

    @abstractmethod
    def set_latest(self, dataset: str, type: BackupType, snapshot: str) -> None:
        """Set the latest snapshot in the given ZFS dataset.

        Args:
            dataset: The name of the ZFS dataset.
            type: The type of backup.
            snapshot: The name of the snapshot to set as latest.
        """
        raise NotImplementedError()


class ZfsSnapshotHandler(SnapshotHandler):
    @property
    def filename(self) -> str:
        return "snapshot_"

    def export(
        self,
        dataset: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: Path,
    ) -> Path:
        base_snapshot_str = f"{dataset}@{base_snapshot}"
        ref_snapshot_str = f"{dataset}@{ref_snapshot}" if ref_snapshot else None

        zfs_cmd = ["zfs", "send", base_snapshot_str]
        if ref_snapshot_str:
            zfs_cmd.extend(["-i", ref_snapshot_str])

        output_path = Path(output_dir)
        full_path = output_path / self.filename
        split_cmd = [
            "split",
            f"--bytes={SPLIT_SIZE}",
            "--numeric-suffixes=0",
            "--suffix-length=6",
            "-",
            str(full_path),
        ]

        zfs_proc = subprocess.Popen(zfs_cmd, stdout=subprocess.PIPE)
        split_proc = subprocess.Popen(split_cmd, stdin=zfs_proc.stdout)

        split_proc.communicate()

        split_files = list(output_path.glob(f"{self.filename}*"))
        return len(split_files)

    def list(self, dataset: str) -> list[str]:
        cmd = ["zfs", "list", "-H", "-o", "name", "-t", "snapshot", dataset]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            raise RuntimeError(f"Error listing snapshots: {result.stderr}")

        snapshots = result.stdout.splitlines()
        snapshots.reverse()
        return snapshots

    def get_latest(self, dataset: str, type: BackupType) -> str:
        return ""  # TODO

    def set_latest(self, dataset: str, type: BackupType, snapshot: str) -> None:
        pass  # TODO


class MockSnapshotHandler(SnapshotHandler):
    def __init__(
        self,
        file_system: FileHandler,
        shutdown: bool = False,
        snapshots: list[str] = [],
        filename: str = "mock_snapshot.snp",
    ) -> None:
        self._filename = filename
        self._file_system = file_system
        self.shutdown = shutdown
        self.snapshots: list[str] = snapshots

    @property
    def filename(self) -> str:
        return self._filename

    def export(
        self,
        dataset: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: Path,
    ) -> Path:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        filepath = output_dir / self.filename
        # content = "\n".join([dataset, base_snapshot, ref_snapshot or "NONE"])
        content = os.urandom(1024)
        self._file_system.save(filepath, content)

        return filepath

    def test(self, dataset: str, filepath: Path) -> bool:
        return True

    def list(self, dataset: str) -> list[str]:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        return [f"{dataset}@{s}" for s in self.snapshots]

    def get_latest(self, dataset: str, type: BackupType) -> str | None:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        latest_dir_path = Path(dataset)
        full_path = latest_dir_path / self.__get_latest_filename(type)
        if not self._file_system.check(full_path):
            return None  # no latest snapshot

        latest_snapshot: str = self._file_system.read(full_path)
        return latest_snapshot if latest_snapshot else None  # empty file

    def set_latest(self, dataset: str, type: BackupType, snapshot: str) -> None:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        latest_dir_path = Path(dataset)
        full_path = latest_dir_path / self.__get_latest_filename(type)
        self._file_system.save(full_path, snapshot)

        if not self._file_system.check(full_path):
            raise RuntimeError(f"Latest snapshot file save failed: {full_path}")

    def __get_latest_filename(self, type: BackupType) -> str:
        return f"latest_{type}.txt"
