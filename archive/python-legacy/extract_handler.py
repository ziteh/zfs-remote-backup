from pathlib import Path
from abc import ABCMeta, abstractmethod

from app.file_handler import FileHandler
from app.hash_handler import Hasher


class Splitter(metaclass=ABCMeta):
    def __init__(self, chunk_size: int, hasher: Hasher) -> None:
        if chunk_size <= 0:
            raise ValueError("Chunk size must be greater than 0")

        self._chunk_size = chunk_size
        self._hasher = hasher

    @property
    def chunk_size(self) -> int:
        return self._chunk_size

    @abstractmethod
    def extension(self, index: int) -> str:
        raise NotImplementedError()

    @abstractmethod
    def split(
        self,
        filepath: Path,
        index: int,
        in_hash: bytes,
    ) -> bytes:
        raise NotImplementedError()


class MockExtractor(Splitter):
    def __init__(
        self, chunk_size: int, hash_handler: Hasher, file_system: FileHandler
    ) -> None:
        super().__init__(chunk_size, hash_handler)
        self._file_system = file_system

    def extension(self, index: int) -> str:
        return f".p{index:06d}"

    def split(
        self,
        filepath: Path,
        index: int,
        in_hash: bytes,
    ) -> bytes:
        if not self._file_system.check_file(filepath):
            raise FileNotFoundError()

        offset = index * self._chunk_size
        context: bytes = self._file_system.read(filepath)
        data = context[offset : offset + self._chunk_size]

        self._hasher.reset()
        self._hasher.update(in_hash)
        self._hasher.update(data)

        out_filepath = filepath.with_suffix(filepath.suffix + self.extension(index))
        self._file_system.save(out_filepath, data)
        return self._hasher.digest
