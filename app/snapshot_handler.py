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
        pool: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: str,
    ) -> int:
        """Export snapshot.

        Args:
            pool: The name of the ZFS pool.
            base_snapshot: The base snapshot to export.
            ref_snapshot: The reference snapshot for incremental exports.
            output_dir: The directory where the output files will be saved.

        Returns:
            The number of parts created during the export.
        """
        raise NotImplementedError()

    @abstractmethod
    def list(self, pool: str) -> list[str]:
        """List all snapshots in the given ZFS pool.

        Args:
            pool: The name of the ZFS pool.

        Returns:
            A list of snapshot names.
        """
        raise NotImplementedError()

    @abstractmethod
    def get_latest(self, pool: str, type: BackupType) -> str | None:
        """Get the latest snapshot in the given ZFS pool.

        Args:
            pool: The name of the ZFS pool.
            type: The type of backup.

        Returns:
            The name of the latest snapshot or None if no snapshot exists.
        """
        raise NotImplementedError()

    @abstractmethod
    def set_latest(self, pool: str, type: BackupType, snapshot: str) -> None:
        """Set the latest snapshot in the given ZFS pool.

        Args:
            pool: The name of the ZFS pool.
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
        pool: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: str,
    ) -> int:
        base_snapshot_str = f"{pool}@{base_snapshot}"
        ref_snapshot_str = f"{pool}@{ref_snapshot}" if ref_snapshot else None

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

    def list(self, pool: str) -> list[str]:
        cmd = ["zfs", "list", "-H", "-o", "name", "-t", "snapshot", pool]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            raise RuntimeError(f"Error listing snapshots: {result.stderr}")

        snapshots = result.stdout.splitlines()
        snapshots.reverse()
        return snapshots

    def get_latest(self, pool: str, type: BackupType) -> str:
        return ""  # TODO

    def set_latest(self, pool: str, type: BackupType, snapshot: str) -> None:
        pass  # TODO


class MockSnapshotHandler(SnapshotHandler):
    def __init__(
        self,
        file_system: FileHandler,
        shutdown: bool = False,
        snapshots: list[str] = [],
        export_return: int = 3,
        filename: str = "mock_snapshot_",
    ) -> None:
        self._filename = filename
        self._file_system = file_system
        self.shutdown = shutdown
        self.export_return = export_return
        self.snapshots: list[str] = snapshots
        self.export_calls: list[
            dict[str, str]
        ] = []  # keep track of calls for test assertions

    @property
    def filename(self) -> str:
        return self._filename

    def export(
        self,
        pool: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_dir: str,
    ) -> int:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        output_path = Path(output_dir)
        for i in range(self.export_return):
            filename = f"{self._filename}{i:06}"
            content = f"{pool}\n{base_snapshot}\n{ref_snapshot or 'NONE'}\n{i}"
            self._file_system.save(output_path / filename, content)

        return self.export_return

    def list(self, pool: str) -> list[str]:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        return [f"{pool}@{s}" for s in self.snapshots]

    def get_latest(self, pool: str, type: BackupType) -> str | None:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        latest_dir_path = Path(pool)
        full_path = latest_dir_path / self.__get_latest_filename(type)
        if not self._file_system.check(full_path):
            return None  # no latest snapshot

        latest_snapshot: str = self._file_system.read(full_path)
        return latest_snapshot if latest_snapshot else None  # empty file

    def set_latest(self, pool: str, type: BackupType, snapshot: str) -> None:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        latest_dir_path = Path(pool)
        full_path = latest_dir_path / self.__get_latest_filename(type)
        self._file_system.save(full_path, snapshot)

        if not self._file_system.check(full_path):
            raise RuntimeError(f"Latest snapshot file save failed: {full_path}")

    def __get_latest_filename(self, type: BackupType) -> str:
        return f"latest_{type}.txt"
