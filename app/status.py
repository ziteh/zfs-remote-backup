from datetime import datetime
from dataclasses import asdict
from dacite import from_dict
import dacite
from tomlkit import dumps, parse
from pathlib import Path
from app.define import (
    WORK_DIR,
    STATUS_FILENAME,
    ENCODING as ENC,
    NEWLINE as NL,
    BackupStatus,
    BackupType,
    Task,
    Snapshot,
    Stage,
    BackupTaskStage,
)


def load_status() -> BackupStatus:
    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    with open(status_file_path, "r", encoding=ENC, newline=NL) as f:
        toml_data = parse(f.read())
        print(toml_data)

    def parse_datetime(value: str) -> datetime:
        return datetime.fromisoformat(value)

    return from_dict(
        data_class=BackupStatus,
        data=toml_data,
        config=dacite.Config(type_hooks={datetime: parse_datetime}),
    )


def save_status(status: BackupStatus):
    status.task.last_record = datetime.now()

    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    with open(status_file_path, "w", encoding=ENC, newline=NL) as f:
        status_dict = asdict(status)
        f.write(dumps(status_dict))


def reset_new_status(
    target: datetime, type: BackupType, pool: str, base: str, incr: str
):
    task = Task(type, target, target)
    snapshot = Snapshot(pool, base, incr, -1)
    stage = Stage(False, -1, -1, -1, -1, -1)
    new_status = BackupStatus(task, snapshot, stage)
    save_status(new_status)


def check_stage(status: BackupStatus) -> tuple[BackupTaskStage, int, int]:
    stage = status.stage
    if not stage.exported:
        return ("export", 0, 0)

    split_qty = status.snapshot.split_quantity
    if split_qty <= 0:
        return ("export", -1, split_qty)  # error

    if stage.compressed > split_qty:
        return ("compress", -stage.compressed, -split_qty)  # error
    elif stage.compressed < split_qty:
        return ("compress", stage.compressed, split_qty)

    if stage.compress_tested > split_qty:
        return ("compress_test", -stage.compress_tested, -split_qty)  # error
    elif stage.compress_tested < split_qty:
        return ("compress_test", stage.compress_tested, split_qty)

    if stage.encrypted > split_qty:
        return ("encrypt", -stage.encrypted, -split_qty)  # error
    elif stage.encrypted < split_qty:
        return ("encrypt", stage.encrypted, split_qty)

    if stage.uploaded > split_qty:
        return ("upload", -stage.uploaded, -split_qty)  # error
    elif stage.uploaded < split_qty:
        return ("upload", stage.uploaded, split_qty)

    if stage.removed > split_qty:
        return ("remove", -stage.removed, -split_qty)  # error
    elif stage.removed < split_qty:
        return ("remove", stage.removed, split_qty)

    return ("done", 0, split_qty)


def is_error_stage(current: int, total: int):
    return total <= 0 or current <= 0
