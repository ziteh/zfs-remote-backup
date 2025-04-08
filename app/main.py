from pathlib import Path
from define import (
    WORK_DIR,
    STATUS_FILENAME,
    BackupStatus,
    Task,
    Snapshot,
    Stage,
    BackupTaskStage,
)
import tomllib


def main():
    status_file_path = Path(WORK_DIR) / STATUS_FILENAME
    status = load_status(status_file_path)
    (stage, current, total) = check_stage(status)

    if total <= 0 or current <= 0:
        print(stage, "error")
    elif stage == "done":
        return


def load_status(path: str | Path) -> BackupStatus:
    with open(path, "rb") as f:
        toml_data = tomllib.load(f)
        print(toml_data)

    task_data = toml_data["task"]
    snapshot_data = toml_data["snapshot"]
    stage_data = toml_data["stage"]

    return BackupStatus(
        task=Task(**task_data),
        snapshot=Snapshot(**snapshot_data),
        stage=Stage(**stage_data),
    )


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


if __name__ == "__main__":
    main()
