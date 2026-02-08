from unittest.mock import Mock

import pytest

from app.status_manager import (
    CurrentTask,
    DatasetLatestSnapshot,
    Stage,
    StatusFilesIo,
    StatusManager,
    TaskQueue,
)


@pytest.fixture
def mock_status_io() -> Mock:
    mock_io = Mock(spec=StatusFilesIo)

    # Set up return values for load methods
    mock_io.load_task_queue.return_value = TaskQueue(tasks=[])
    mock_io.load_latest_snapshot.return_value = DatasetLatestSnapshot(latest={})
    mock_io.load_current_task.return_value = CurrentTask(
        base="base_snapshot",
        ref="ref_snapshot",
        split_quantity=5,
        stream_hash=b"hash",
        stage=Stage(
            snapshot_exported="",
            snapshot_tested=False,
            spit=[],
            compressed=0,
            encrypted=0,
            uploaded=0,
            verify=False,
            cleared=0,
        ),
    )

    return mock_io


@pytest.fixture
def mock_snapshot_handler():
    mock_handler = Mock()
    mock_handler.list.return_value = ["snapshot1", "snapshot2"]
    return mock_handler


@pytest.fixture
def status_manager(mock_status_io: Mock, mock_snapshot_handler: Mock) -> StatusManager:
    manager = StatusManager(mock_status_io, mock_snapshot_handler)

    mock_status_io.load_task_queue.reset_mock()
    mock_status_io.load_latest_snapshot.reset_mock()
    mock_status_io.load_current_task.reset_mock()

    return manager
