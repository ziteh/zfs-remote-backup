import boto3
from sha256 import cal_sha256, bytes_to_base64


def upload_to_s3(
    filename: str,
    bucket: str,
    key: str,
    tags: dict[str, str] | None,
    metadata: dict[str, str] | None,
):
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
