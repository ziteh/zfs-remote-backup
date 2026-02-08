import argparse
import base64
import hashlib
from pathlib import Path


def cal_sha256(file_path: str | Path) -> bytes:
    hash = hashlib.sha256()
    try:
        with open(file_path, "rb") as f:
            for byte_block in iter(lambda: f.read(4096), b""):
                hash.update(byte_block)

        return hash.digest()
    except FileNotFoundError as e:
        raise FileNotFoundError() from e


def bytes_to_hex(data: bytes) -> str:
    return data.hex()


def bytes_to_base64(data: bytes) -> str:
    return base64.b64encode(data).decode("utf-8")


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Calculate SHA256 hash。")
    parser.add_argument("file_path", help="File path")
    args = parser.parse_args()

    file_path = args.file_path
    sha256_byte = cal_sha256(file_path)
    sha256_hex = bytes_to_hex(sha256_byte)
    sha256_base64 = bytes_to_base64(sha256_byte)

    print(f"{file_path}")
    print(f"SHA256    (Hex): {sha256_hex}")
    print(f"SHA256 (Base64): {sha256_base64}")
