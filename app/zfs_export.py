import subprocess

SPLIT_SIZE = "4G"

# ‘b’  =>            512 ("blocks")
# ‘KB’ =>           1000 (KiloBytes)
# ‘K’  =>           1024 (KibiBytes)
# ‘MB’ =>      1000*1000 (MegaBytes)
# ‘M’  =>      1024*1024 (MebiBytes)
# ‘GB’ => 1000*1000*1000 (GigaBytes)
# ‘G’  => 1024*1024*1024 (GibiBytes)
# https://www.gnu.org/software/coreutils/manual/html_node/split-invocation.html


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

    split_cmd = [
        "split",
        f"--bytes={SPLIT_SIZE}",
        "--numeric-suffixes=0",
        "--suffix-length=5",
        "-",
        output_filename,
    ]

    zfs_proc = subprocess.Popen(zfs_cmd, stdout=subprocess.PIPE)
    split_proc = subprocess.Popen(split_cmd, stdin=zfs_proc.stdout)

    split_proc.communicate()
