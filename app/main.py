from datetime import datetime
from app.define import (
    WORK_DIR,
    STATUS_FILENAME,
    BackupStatus,
    Task,
    Snapshot,
    Stage,
    BackupTaskStage,
)
from app.status import check_stage, is_error_stage, load_status, reset_new_status


def perform_full_backup():
    now = datetime.now()

    status = load_status()
    (stage, current, total) = check_stage(status)
    if is_error_stage(current, total):
        print(stage, "error")

    if stage == "done":
        if status.task.target.date() == now.date():
            print("Done, nothing to do")
            return
        else:
            print("Start new")

    # reset_new_status(now,"full",)
