from abc import ABCMeta, abstractmethod
from pathlib import Path

from app.file_handler import FileHandler


class Hasher(metaclass=ABCMeta):
    @abstractmethod
    def update(self, data: bytes) -> None:
        raise NotImplementedError()

    @abstractmethod
    def cal_file(self, filepath: Path) -> None:
        raise NotImplementedError()

    @abstractmethod
    def reset(self) -> None:
        raise NotImplementedError()

    @property
    @abstractmethod
    def digest(self) -> bytes:
        raise NotImplementedError()

    @property
    @abstractmethod
    def hexdigest(self) -> str:
        raise NotImplementedError()


class SumHasher(Hasher):
    _sum: int
    _file_system: FileHandler

    def __init__(self, file_system: FileHandler) -> None:
        self._file_system = file_system
        self.reset()

    def update(self, data: bytes) -> None:
        self._sum = (self._sum + sum(data)) % 2**32

    def cal_file(self, filepath: Path) -> None:
        if not self._file_system.check_file(filepath):
            raise FileNotFoundError(f"File '{filepath}' not found.")

        data = self._file_system.read(filepath)
        self.update(data)

    def reset(self) -> None:
        self._sum = 0

    @property
    def digest(self) -> bytes:
        return self._sum.to_bytes(4, "big", signed=False)

    @property
    def hexdigest(self) -> str:
        return self.digest.hex()
