import subprocess

SPLIT_SIZE = "4G"


def zfs_export(
    zfs_pool: str,
    base_snapshot: str,
    incr_snapshot: str | None,
    output_filename: str,
):
    base_snapshot_str = f"{zfs_pool}@{base_snapshot}"
    incr_snapshot_str = f"{zfs_pool}@{incr_snapshot}" if incr_snapshot else None

    zfs_cmd = ["zfs", "send", base_snapshot_str]
    if incr_snapshot_str:
        zfs_cmd.extend(["-i", incr_snapshot_str])

    split_cmd = ["split", "-b", SPLIT_SIZE, "-", output_filename]

    zfs_proc = subprocess.Popen(zfs_cmd, stdout=subprocess.PIPE)
    split_proc = subprocess.Popen(split_cmd, stdin=zfs_proc.stdout)

    split_proc.communicate()
