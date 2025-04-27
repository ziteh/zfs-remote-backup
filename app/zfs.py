import subprocess
from pathlib import Path

from app.define import SPLIT_SIZE, TEMP_DIR


class Zfs:
    @staticmethod
    def export_snapshot(
        pool: str,
        base_snapshot: str,
        ref_snapshot: str | None,
        output_filename: str,
    ) -> int:
        base_snapshot_str = f"{pool}@{base_snapshot}"
        ref_snapshot_str = f"{pool}@{ref_snapshot}" if ref_snapshot else None

        zfs_cmd = ["zfs", "send", base_snapshot_str]
        if ref_snapshot_str:
            zfs_cmd.extend(["-i", ref_snapshot_str])

        full_path = Path(TEMP_DIR) / output_filename
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

        split_files = list(Path(TEMP_DIR).glob(f"{output_filename}*"))
        return len(split_files)

    @staticmethod
    def list_snapshots(pool: str) -> list[str]:
        """List all snapshots in the given ZFS pool."""

        cmd = ["zfs", "list", "-H", "-o", "name", "-t", "snapshot", pool]
        result = subprocess.run(cmd, capture_output=True, text=True)
        if result.returncode != 0:
            raise RuntimeError(f"Error listing snapshots: {result.stderr}")

        snapshots = result.stdout.splitlines()
        snapshots.reverse()
        return snapshots
