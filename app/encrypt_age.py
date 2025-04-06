from pyrage import encrypt, x25519
import argparse


def main(input_file: str, output_file: str, pub_key_str: str):
    pub_key = x25519.Recipient.from_str(pub_key_str)

    try:
        with open(input_file, "rb") as f:
            plaintext = f.read()
    except FileNotFoundError:
        raise FileNotFoundError

    encrypted = encrypt(plaintext, [pub_key])

    with open(output_file, "wb") as f:
        f.write(encrypted)


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description="Encrypt with age")
    parser.add_argument("input_file", help="Input file path")
    parser.add_argument("output_file", help="Output file path")
    parser.add_argument("public_key", help="Public key")
    args = parser.parse_args()

    input_file = args.input_file
    output_file = args.output_file
    pub_key_str = args.public_key
    main(input_file, output_file, pub_key_str)
