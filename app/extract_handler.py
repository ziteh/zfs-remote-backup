from pathlib import Path
from abc import ABCMeta, abstractmethod

from app.file_handler import FileHandler
from app.hash_handler import Hasher


class ExtractHandler(metaclass=ABCMeta):
    _chunk_size: int
    _hasher: Hasher

    def __init__(self, chunk_size: int, hasher: Hasher) -> None:
        self._chunk_size = chunk_size
        self._hasher = hasher

    @abstractmethod
    def split(
        self,
        in_filepath: Path,
        out_dir: Path,
        index: int,
        in_hash: bytes,
    ) -> bytes:
        raise NotImplementedError()


class MockExtractor(ExtractHandler):
    def __init__(
        self, file_system: FileHandler, chunk_size: int, hash_handler: Hasher
    ) -> None:
        super().__init__(chunk_size, hash_handler)
        self._file_system = file_system

    def split(
        self,
        in_filepath: Path,
        out_dir: Path,
        index: int,
        in_hash: bytes,
    ) -> bytes:
        if not in_filepath.exists() or not in_filepath.is_fifo():
            raise FileNotFoundError()

        offset = index * self._chunk_size
        context: bytes = self._file_system.read(in_filepath)
        data = context[offset : offset + self._chunk_size]
        self._hasher.reset()
        self._hasher.update(in_hash)
        self._hasher.update(data)

        out_filepath = out_dir / f"{in_filepath}.p{index:06}"
        self._file_system.save(out_filepath, data)

        return self._hasher.digest
