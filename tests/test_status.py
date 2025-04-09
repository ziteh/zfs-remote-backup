from datetime import datetime
from app.define import BackupStatus, Stage, Snapshot, Task
from app.status import check_stage


def test_check_stage_exported_false():
    status = BackupStatus(
        stage=Stage(
            exported=False,
            compressed=0,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "export"
    assert current == 0
    assert total == 0


def test_check_stage_compress():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=5,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "compress"
    assert current == 5
    assert total == 20


def test_check_stage_compress_test():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=20,
            compress_tested=5,
            encrypted=0,
            uploaded=0,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "compress_test"
    assert current == 5
    assert total == 20


def test_check_stage_encrypt():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=20,
            compress_tested=20,
            encrypted=5,
            uploaded=0,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "encrypt"
    assert current == 5
    assert total == 20


def test_check_stage_upload():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=20,
            compress_tested=20,
            encrypted=20,
            uploaded=5,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "upload"
    assert current == 5
    assert total == 20


def test_check_stage_done():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=20,
            compress_tested=20,
            encrypted=20,
            uploaded=20,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "remove"
    assert current == 0
    assert total == 20


def test_check_stage_error_uploaded():
    status = BackupStatus(
        stage=Stage(
            exported=True,
            compressed=20,
            compress_tested=20,
            encrypted=20,
            uploaded=21,
            removed=0,
        ),
        task=Task("full", datetime.now(), datetime.now()),
        snapshot=Snapshot(pool="gen_pool_1", base="snap1", incr="", split_quantity=20),
    )
    stage, current, total = check_stage(status)
    assert stage == "upload"
    assert current == -21
    assert total == -20
