from abc import ABCMeta, abstractmethod
from pathlib import Path
from typing import Any

import boto3
from minio import Minio

from app.file_handler import FileHandler
from app.sha256 import bytes_to_base64, cal_sha256


class RemoteStorageHandler(metaclass=ABCMeta):
    @abstractmethod
    def upload(
        self,
        src_filepath: Path,
        dest_filepath: Path,
        tags: dict[str, str] | None,
        metadata: dict[str, str] | None,
    ) -> None:
        """Uploads a file to the remote storage.

        Args:
            src_filepath: The path to the source file.
            dest_filepath: The path to the destination file.
            tags: Optional tags.
            metadata: Optional metadata.
        """
        raise NotImplementedError()


class MockRemoteStorageHandler(RemoteStorageHandler):
    def __init__(
        self,
        bucket: str,
        file_system: FileHandler,
        shutdown: bool = False,
    ):
        self.bucket = bucket
        self.shutdown = shutdown
        self.objects: dict[str, Any] = {}
        self._file_system = file_system

    def upload(
        self,
        src_filepath: Path,
        dest_filepath: Path,
        tags: dict[str, str] | None,
        metadata: dict[str, str] | None,
    ) -> None:
        if self.shutdown:
            raise RuntimeError("System is shutting down.")

        if not self._file_system.check_file(src_filepath):
            raise FileNotFoundError(f"File '{src_filepath}' not found.")

        target = f"{self.bucket}:{dest_filepath}"
        self.objects[target] = {
            "content": self._file_system.read(src_filepath),
            "tags": tags,
            "metadata": metadata,
        }


class AwsS3Oss(RemoteStorageHandler):
    """AWS S3 Object Storage Service"""

    def __init__(self, bucket: str) -> None:
        self.bucket = bucket

    def upload(
        self,
        filename: str,
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
        s3.upload_file(filename, self.bucket, key, extra_args)

        print(
            f"File '{filename}' uploaded to S3://{self.bucket}/{key} with SHA256(Base64) '{sha256_base64}'."
        )


class MinioOss(RemoteStorageHandler):
    """MinIO Object Storage Service"""

    def __init__(
        self,
        endpoint: str,
        access_key: str,
        secret_key: str,
        bucket: str,
        secure: bool = False,
    ) -> None:
        self.client = Minio(endpoint, access_key, secret_key, secure=secure)
        self.bucket = bucket

    def upload(
        self,
        filename: str,
        key: str,
        tags: dict[str, str] | None = None,
        metadata: dict[str, str] | None = None,
    ) -> None:
        sha256_base64 = bytes_to_base64(cal_sha256(filename))

        extra_headers = {"x-amz-meta-sha256": sha256_base64}

        if metadata:
            extra_headers.update({f"x-amz-meta-{k}": v for k, v in metadata.items()})

        if not self.client.bucket_exists(self.bucket):
            self.client.make_bucket(self.bucket)

        self.client.fput_object(
            bucket_name=self.bucket,
            object_name=key,
            file_path=filename,
            metadata=extra_headers,  # type: ignore
        )

        print(
            f"File '{filename}' uploaded to MinIO bucket '{self.bucket}' with key '{key}' and SHA256(Base64) '{sha256_base64}'."
        )
