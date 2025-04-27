from datetime import datetime

import pytest

from app.status import BackupStatusRaw, CurrentTask, Stage, check_stage, is_error_stage


class TestStage:
    @pytest.fixture
    def default_status(self) -> BackupStatusRaw:
        return BackupStatusRaw(
            last_record=datetime.now(),
            queue=[],
            current=CurrentTask(
                base="base",
                ref="incr",
                split_quantity=3,
                stage=Stage(
                    exported=False,
                    compressed=0,
                    compress_tested=0,
                    encrypted=0,
                    uploaded=0,
                    removed=0,
                ),
            ),
        )

    def test_check_stage_export(self, default_status: BackupStatusRaw):
        """export stage"""
        default_status.current.stage = Stage(
            exported=False,
            compressed=0,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "export"
        assert current == 0
        assert total == 0

    def test_check_stage_compress(self, default_status: BackupStatusRaw):
        """compress stage"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=0,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "compress"
        assert current == default_status.current.stage.compressed
        assert total == default_status.current.split_quantity

    def test_check_stage_compress_error(self, default_status: BackupStatusRaw):
        """compress stage error"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity + 1,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "compress"
        assert is_error_stage(current, total) is True

    def test_check_stage_compress_test(self, default_status: BackupStatusRaw):
        """compress test stage"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=0,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "compress_test"
        assert current == default_status.current.stage.compress_tested
        assert total == default_status.current.split_quantity

    def test_check_stage_compress_test_error(self, default_status: BackupStatusRaw):
        """compress test stage error"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity + 1,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "compress_test"
        assert is_error_stage(current, total) is True

    def test_check_stage_encrypt(self, default_status: BackupStatusRaw):
        """encrypt stage"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=0,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "encrypt"
        assert current == default_status.current.stage.encrypted
        assert total == default_status.current.split_quantity

    def test_check_stage_encrypt_error(self, default_status: BackupStatusRaw):
        """encrypt stage error"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity + 1,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "encrypt"
        assert is_error_stage(current, total) is True

    def test_check_stage_upload(self, default_status: BackupStatusRaw):
        """upload stage"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity,
            uploaded=0,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "upload"
        assert current == default_status.current.stage.uploaded
        assert total == default_status.current.split_quantity

    def test_check_stage_upload_error(self, default_status: BackupStatusRaw):
        """upload stage error"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity,
            uploaded=default_status.current.split_quantity + 1,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "upload"
        assert is_error_stage(current, total) is True

    def test_check_stage_remove(self, default_status: BackupStatusRaw):
        """remove stage"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity,
            uploaded=default_status.current.split_quantity,
            removed=0,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "remove"
        assert current == default_status.current.stage.removed
        assert total == default_status.current.split_quantity

    def test_check_stage_remove_error(self, default_status: BackupStatusRaw):
        """remove stage error"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity,
            uploaded=default_status.current.split_quantity,
            removed=default_status.current.split_quantity + 1,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "remove"
        assert is_error_stage(current, total) is True

    def test_check_stage_done(self, default_status: BackupStatusRaw):
        """all done"""
        default_status.current.stage = Stage(
            exported=True,
            compressed=default_status.current.split_quantity,
            compress_tested=default_status.current.split_quantity,
            encrypted=default_status.current.split_quantity,
            uploaded=default_status.current.split_quantity,
            removed=default_status.current.split_quantity,
        )

        (stage, current, total) = check_stage(default_status)

        assert stage == "done"
        assert current == 0
        assert total == default_status.current.split_quantity
