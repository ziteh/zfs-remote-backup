import subprocess

COMPRESSION_LEVEL = "-8"
THREADS = "-T0"

AGE_PUBLIC_KEY = "age..."


def compress_encrypt(input_filename: str) -> str:
    # zstd compress
    compressed_filename = f"{input_filename}.zst"
    subprocess.run(
        ["zstd", COMPRESSION_LEVEL, THREADS, input_filename, "-o", compressed_filename],
        check=True,
    )

    # test zstd compression
    subprocess.run(
        ["zstd", "--test", compressed_filename],
        check=True,
    )

    # age encrypt
    encrypted_filename = f"{compressed_filename}.age"
    subprocess.run(
        ["age", "-r", AGE_PUBLIC_KEY, "-o", encrypted_filename, compressed_filename],
        check=True,
    )

    return encrypted_filename
