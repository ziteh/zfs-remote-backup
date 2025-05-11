import os
from abc import ABCMeta, abstractmethod
from pathlib import Path
from typing import Any


class FileHandler(metaclass=ABCMeta):
    @abstractmethod
    def delete(self, filename: str | Path) -> None:
        """Delete a file.

        Args:
            filename: The name of the file to delete.
        """
        raise NotImplementedError()

    @abstractmethod
    def save(self, filename: str | Path, content: Any) -> None:
        raise NotImplementedError()

    @abstractmethod
    def read(self, filename: str | Path) -> Any:
        raise NotImplementedError()

    @abstractmethod
    def check_file(self, filename: str | Path) -> bool:
        raise NotImplementedError()

    @abstractmethod
    def get_file_size(self, filepath: Path) -> int:
        raise NotImplementedError()

    @abstractmethod
    def clear(self) -> None:
        raise NotImplementedError()


class OsFileHandler(FileHandler):
    """OS File System"""

    def delete(self, filename: str | Path) -> None:
        os.remove(filename)


class MockFileSystem(FileHandler):
    files: dict[str, Any] = {}

    def save(self, filename: str | Path, content: Any) -> None:
        file = str(filename)
        self.files[file] = content

    def read(self, filename: str | Path) -> Any:
        file = str(filename)
        if file in self.files:
            return self.files[file]
        else:
            raise FileNotFoundError(f"File '{filename}' not found.")

    def delete(self, filename: str | Path) -> None:
        file = str(filename)
        if file in self.files:
            del self.files[file]
        else:
            raise FileNotFoundError(f"File '{filename}' not found.")

    def check_file(self, filename: str | Path) -> bool:
        file = str(filename)
        return file in self.files

    def get_file_size(self, filepath: Path) -> int:
        try:
            file = self.files[str(filepath)]
            if file is None:
                return -1

            return len(file)
        except KeyError:
            return -1

    def clear(self) -> None:
        self.files.clear()

    def print(self) -> None:
        print(self.files)
