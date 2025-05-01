from abc import ABCMeta, abstractmethod
from typing import Any

import boto3

from app.file_handler import MockFileSystem
from app.sha256 import bytes_to_base64, cal_sha256


class RemoteStorageHandler(metaclass=ABCMeta):
    @abstractmethod
    def upload(
        self,
        filename: str,
        bucket: str,
        key: str,
        tags: dict[str, str] | None,
        metadata: dict[str, str] | None,
    ) -> None:
        """Uploads a file to the remote storage.

        Args:
            filename: The name of the file to upload.
            bucket: The name of the remote bucket.
            key: The key under which to store the file.
            tags: Optional tags.
            metadata: Optional metadata.
        """
        raise NotImplementedError()


class MockRemoteStorageHandler(RemoteStorageHandler):
    def __init__(self, file_system: MockFileSystem):
        self.objects: dict[str, Any] = {}
        self._file_system = file_system

    def upload(
        self,
        filename: str,
        bucket: str,
        key: str,
        tags: dict[str, str] | None,
        metadata: dict[str, str] | None,
    ) -> None:
        if not self._file_system.check(filename):
            raise FileNotFoundError(f"File '{filename}' not found.")

        target = f"{bucket}/{key}"
        self.objects[target] = {
            "content": self._file_system.read(filename),
            "tags": tags,
            "metadata": metadata,
        }


class AwsS3Oss(RemoteStorageHandler):
    """AWS S3 Object Storage Service"""

    def upload(
        self,
        filename: str,
        bucket: str,
        key: str,
        tags: dict[str, str] | None,
        metadata: dict[str, str] | None,
    ) -> None:
        sha256_base64 = bytes_to_base64(cal_sha256(filename))

        extra_args: dict[str, str | dict[str, str]] = {
            "ChecksumAlgorithm": "SHA256",
            "ChecksumSHA256": sha256_base64,
        }

        if tags:
            tags_arr = [f"{key}={value}" for key, value in tags.items()]
            extra_args["Tagging"] = "&".join(tags_arr)

        if metadata:
            extra_args["Metadata"] = metadata

        s3 = boto3.client("s3")
        s3.upload_file(filename, bucket, key, extra_args)

        print(
            f"File '{filename}' uploaded to S3://{bucket}/{key} with SHA256(Base64) '{sha256_base64}'."
        )
