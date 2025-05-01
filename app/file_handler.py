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


class OsFileHandler(FileHandler):
    """OS File System"""

    def delete(self, filename: str | Path) -> None:
        os.remove(filename)


class MockFileSystem(FileHandler):
    file_system: dict[str, Any] = {}

    def save(self, filename: str | Path, content: Any) -> None:
        file = str(filename)
        self.file_system[file] = content

    def read(self, filename: str | Path) -> Any:
        file = str(filename)
        if file in self.file_system:
            return self.file_system[file]
        else:
            raise FileNotFoundError(f"File '{filename}' not found.")

    def delete(self, filename: str | Path) -> None:
        file = str(filename)
        if file in self.file_system:
            del self.file_system[file]
        else:
            raise FileNotFoundError(f"File '{filename}' not found.")

    def check(self, filename: str | Path) -> bool:
        file = str(filename)
        return file in self.file_system

    def clear(self) -> None:
        self.file_system.clear()
