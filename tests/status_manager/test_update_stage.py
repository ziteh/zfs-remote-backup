from unittest.mock import Mock

from app.status_manager import StatusManager


class TestStatusManagerUpdateStage:
    """Test StatusManager.update_stage() method."""

    def test_update_snapshot_export(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test updating snapshot_export stage"""
        snapshot_name = "test_snapshot"
        status_manager.update_stage("snapshot_export", snapshot_name)

        # assert snapshot_exported update
        assert status_manager.current_task.stage.snapshot_exported == snapshot_name

        # assert save_current_task called
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_snapshot_test(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test updating snapshot_test stage"""
        status_manager.update_stage("snapshot_test", True)

        assert status_manager.current_task.stage.snapshot_tested is True
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_split(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating split stage"""
        test_hash = b"test_hash_value"
        status_manager.update_stage("split", test_hash)

        assert status_manager.current_task.stage.spit == [test_hash]
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

        # Test adding multiple split hashes
        second_hash = b"second_hash"
        mock_status_io.save_current_task.reset_mock()  # Reset mock
        status_manager.update_stage("split", second_hash)

        assert status_manager.current_task.stage.spit == [test_hash, second_hash]
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_compress(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating compress stage"""
        status_manager.update_stage("compress", 3)

        assert status_manager.current_task.stage.compressed == 3
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_encrypt(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating encrypt stage"""
        status_manager.update_stage("encrypt", 4)

        assert status_manager.current_task.stage.encrypted == 4
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_upload(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating upload stage"""
        status_manager.update_stage("upload", 5)

        assert status_manager.current_task.stage.uploaded == 5
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_verify(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating verify stage"""
        status_manager.update_stage("verify", True)

        assert status_manager.current_task.stage.verify is True
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_clear(self, status_manager: StatusManager, mock_status_io: Mock) -> None:
        """Test updating clear stage"""
        status_manager.update_stage("clear", 5)

        assert status_manager.current_task.stage.cleared == 5
        mock_status_io.save_current_task.assert_called_once_with(status_manager.current_task)

    def test_update_all_stages_sequence(
        self, status_manager: StatusManager, mock_status_io: Mock
    ) -> None:
        """Test sequentially updating all stages"""
        mock_status_io.save_current_task.reset_mock()

        # Update all stages in sequence
        status_manager.update_stage("snapshot_export", "snapshot1")
        status_manager.update_stage("snapshot_test", True)
        status_manager.update_stage("split", b"hash1")
        status_manager.update_stage("split", b"hash2")
        status_manager.update_stage("compress", 2)
        status_manager.update_stage("encrypt", 2)
        status_manager.update_stage("upload", 2)
        status_manager.update_stage("verify", True)
        status_manager.update_stage("clear", 2)

        # Verify each stage is correctly updated
        assert status_manager.current_task.stage.snapshot_exported == "snapshot1"
        assert status_manager.current_task.stage.snapshot_tested is True
        assert status_manager.current_task.stage.spit == [b"hash1", b"hash2"]
        assert status_manager.current_task.stage.compressed == 2
        assert status_manager.current_task.stage.encrypted == 2
        assert status_manager.current_task.stage.uploaded == 2
        assert status_manager.current_task.stage.verify is True
        assert status_manager.current_task.stage.cleared == 2

        # Verify save method was called correct number of times
        assert mock_status_io.save_current_task.call_count == 9
