from loguru import logger

from app.define import EXPORT_FILENAME
from app.latest_snapshot import read_latest
from app.status import (
    BackupStatus,
    BackupStatusRaw,
    Task,
)
from app.zfs import Zfs


def main():
    status = BackupStatus()

    while status.task is not None:
        run_current_task(status)

    logger.info("No backup tasks in queue")


def run_current_task(status: BackupStatus):
    (stage, current, total) = status.get_stage()

    if BackupStatus.is_error_stage(current, total):
        logger.error(f"Error in stage: {stage}, current: {current}, total: {total}")
        return

    match stage:
        case "export":
            handle_export(status)
            pass

        case "compress":
            pass

        case "compress_test":
            pass

        case "encrypt":
            pass

        case "upload":
            pass

        case "remove":
            pass

        case "done":
            # dequeue_backup_task(status)
            # save_status(status)
            return

        case _:
            msg = f"Unknown stage {stage}"
            logger.critical(msg)
            raise ValueError(msg)


def handle_export(status: BackupStatus):
    base = status.base_snapshot
    ref = status.ref_snapshot
    type = status.backup_type
    pool = status.pool
    date = status.date

    if not base:
        raise ValueError()

    if type != "full" and not ref:
        raise ValueError()

    if pool is None or type is None or date is None:
        raise ValueError()

    filename = EXPORT_FILENAME(pool, type, date)
    num_parts = Zfs.export_snapshot(pool, base, ref, filename)
    status.set_stage_export(num_parts)


def enqueue_backup_task(status: BackupStatusRaw, task: Task) -> int:
    status.queue.append(task)
    # save_status(status)
    logger.info(f"Task {task} added to queue")
    return len(status.queue)


def dequeue_backup_task(status: BackupStatusRaw) -> int:
    if len(status.queue) == 0:
        logger.warning("No tasks in queue to remove")
        return 0

    task = status.queue.pop(0)
    load_backup_task(status)
    # save_status(status)
    logger.info(f"Task {task} removed from queue")
    return len(status.queue)


def load_backup_task(status: BackupStatusRaw) -> Task | None:
    if len(status.queue) == 0:
        logger.warning("No tasks in queue to load")
        return None

    task = status.queue[0]
    logger.info(f"Task {task} loaded from queue")

    # Clear the task status
    status.current.split_quantity = 0
    status.current.stage.exported = False
    status.current.stage.compressed = 0
    status.current.stage.compress_tested = 0
    status.current.stage.encrypted = 0
    status.current.stage.uploaded = 0
    status.current.stage.removed = 0

    # Set the snapshot
    snapshots = Zfs.list_snapshots(task.pool)
    status.current.base = snapshots[0]

    match task.type:
        case "full":
            # Full backup, no reference snapshot
            status.current.ref = ""

        case "diff":
            # Differential backup, use the latest full snapshot as reference
            status.current.ref = read_latest(task.pool, "full")

        case "incr":
            # Incremental backup, use the latest differential snapshot as reference
            status.current.ref = read_latest(task.pool, "diff")

        case _:
            msg = f"Unknown backup type: {task.type}"
            logger.critical(msg)
            raise ValueError(msg)

    # save_status(status)
    return task


if __name__ == "__main__":
    main()
