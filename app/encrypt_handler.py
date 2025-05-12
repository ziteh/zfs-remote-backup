from pathlib import Path
import subprocess
from abc import ABCMeta, abstractmethod

from app.file_handler import FileHandler
from app.hash_handler import Hasher


class EncryptionHandler(metaclass=ABCMeta):
    @property
    @abstractmethod
    def extension(self) -> str:
        raise NotImplementedError()

    @abstractmethod
    def encrypt(self, filepath: Path) -> Path:
        """Encrypts the given filename using the specified encryption method.

        Args:
            filename: The name of the file to encrypt.

        Returns:
            The name of the encrypted file.
        """
        raise NotImplementedError()

    def decrypt(self, filepath: Path) -> Path:
        raise NotImplementedError()


class AgeEncryptor(EncryptionHandler):
    """Age encryption, https://github.com/FiloSottile/age"""

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


class MockEncryptor(EncryptionHandler):
    def __init__(
        self,
        file_system: FileHandler,
    ) -> None:
        self._file_system = file_system

    @property
    def extension(self) -> str:
        return ".mock_cry"

    def encrypt(self, filepath: Path) -> Path:
        if not self._file_system.check_file(filepath):
            raise FileNotFoundError(f"File '{filepath}' not found.")

        ori_data = self._file_system.read(filepath)
        out_filepath = filepath.with_suffix(filepath.suffix + self.extension)
        self._file_system.save(out_filepath, ori_data)

        return out_filepath

    def decrypt(self, filepath: Path) -> Path:
        if filepath.suffix != self.extension:
            raise ValueError(
                f"File '{filepath}' is not encrypted with the expected extension '{self.extension}'."
            )

        ori_data = self._file_system.read(filepath)
        out_filepath = filepath.with_suffix("")
        self._file_system.save(out_filepath, ori_data)

        return out_filepath
