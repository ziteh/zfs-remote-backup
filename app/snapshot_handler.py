import subprocess
from abc import ABCMeta, abstractmethod
from pathlib import Path

from app.define import SPLIT_SIZE
from app.file_handler import MockFileSystem


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


class MockSnapshotHandler(SnapshotHandler):
    def __init__(
        self,
        file_system: MockFileSystem,
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
