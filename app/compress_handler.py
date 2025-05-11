from pathlib import Path
import subprocess
from abc import ABCMeta, abstractmethod

from app.file_handler import FileHandler


class CompressionHandler(metaclass=ABCMeta):
    @property
    @abstractmethod
    def extension(self) -> str:
        raise NotImplementedError()

    @abstractmethod
    def compress(self, filepath: Path) -> None:
        """Compresses the given file path using the specified compression method.

        Args:
            filepath: The file to compress.

        Returns:
            The compressed file.
        """
        raise NotImplementedError()

    @abstractmethod
    def verify(self, filepath: Path) -> bool:
        """Tests the integrity of the compressed file.

        Args:
            filepath: The compressed file to test.

        Returns:
            `True` if the test is successful, `False` otherwise.
        """
        raise NotImplementedError()


class ZstdCompressor(CompressionHandler):
    """Zstandard (Zstd) compression"""

    _comp_level: str = "-8"
    _threads: str = "-T0"

    def __init__(self, comp_level: str = "-8", threads: str = "-T0"):
        self._comp_level = comp_level
        self._threads = threads

    @property
    def extension(self) -> str:
        return ".zst"

    def compress(self, filename: str) -> str:
        compressed_filename = f"{filename}{self.extension}"
        subprocess.run(
            [
                "zstd",
                self._comp_level,
                self._threads,
                filename,
                "-o",
                compressed_filename,
            ],
            check=True,
        )
        return compressed_filename

    def verify(self, filename: str) -> bool:
        result = subprocess.run(
            ["zstd", "--test", filename],
            check=False,
            capture_output=True,
        )
        return result.returncode == 0


class MockCompressionHandler(CompressionHandler):
    def __init__(
        self,
        file_system: FileHandler,
    ):
        self._file_system = file_system

    @property
    def extension(self) -> str:
        return ".mock_cmp"

    def compress(self, filepath: Path) -> None:
        if not self._file_system.check_file(filepath):
            raise FileNotFoundError(f"File '{filepath}' not found.")

        ori_data = self._file_system.read(filepath)
        out_filepath = filepath.with_suffix(filepath.suffix + self.extension)
        self._file_system.save(out_filepath, ori_data)

    def verify(self, filepath: Path) -> bool:
        return self._file_system.check_file(filepath)  # just check file exist
