import subprocess
from abc import ABCMeta, abstractmethod

from app.file_handler import MockFileSystem


class EncryptionHandler(metaclass=ABCMeta):
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
    def __init__(self, file_system: MockFileSystem, extension: str = ".cry") -> None:
        self._file_system = file_system
        self._extension = extension

    @property
    def extension(self) -> str:
        return self._extension

    def encrypt(self, filename: str) -> str:
        if not self._file_system.check(filename):
            raise FileNotFoundError(f"File '{filename}' not found.")

        new_filename = f"{filename}{self.extension}"
        self._file_system.save(new_filename, self._file_system.read(filename))
        return new_filename
