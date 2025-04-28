import subprocess
from abc import ABCMeta, abstractmethod


class CompressionManager(metaclass=ABCMeta):
    @property
    @abstractmethod
    def extension(self) -> str:
        raise NotImplementedError()

    @abstractmethod
    def compress(self, filename: str) -> str:
        """Compresses the given filename using the specified compression method.

        Args:
            filename: The name of the file to compress.

        Returns:
            The name of the compressed file.
        """
        raise NotImplementedError()

    @abstractmethod
    def verify(self, filename: str) -> bool:
        """Tests the integrity of the compressed file.

        Args:
            filename: The name of the compressed file to test.

        Returns:
            `True` if the test is successful, `False` otherwise.
        """
        raise NotImplementedError()


class ZstdCompression(CompressionManager):
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


class EncryptionManager(metaclass=ABCMeta):
    @property
    @abstractmethod
    def extension(self) -> str:
        raise NotImplementedError()

    @abstractmethod
    def encrypt(self, filename: str) -> str:
        """Encrypts the given filename using the specified encryption method.

        Args:
            filename: The name of the file to encrypt.

        Returns:
            The name of the encrypted file.
        """
        raise NotImplementedError()


class AgeEncryption(EncryptionManager):
    _public_key: str

    def __init__(self, public_key: str):
        self._public_key = public_key

    @property
    def extension(self) -> str:
        return ".age"

    def encrypt(self, filename: str) -> str:
        encrypted_filename = f"{filename}{self.extension}"
        subprocess.run(
            [
                "age",
                "-r",
                self._public_key,
                "-o",
                encrypted_filename,
                filename,
            ],
            check=True,
        )
        return encrypted_filename
