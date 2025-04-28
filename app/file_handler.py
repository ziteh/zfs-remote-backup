import os
from abc import ABCMeta, abstractmethod
from pathlib import Path


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
