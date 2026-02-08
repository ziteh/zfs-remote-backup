package crypto

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"filippo.io/age"
	"github.com/zeebo/blake3"
)

// ProcessPart encrypts a snapshot part, calculates BLAKE3, and removes the original
func ProcessPart(partFile string, recipient age.Recipient) (string, string, error) {
	slog.Info("Processing part file", "partFile", partFile)

	encryptedFile := partFile + ".age"
	if err := Encrypt(partFile, encryptedFile, recipient); err != nil {
		return "", "", fmt.Errorf("age encryption failed: %w", err)
	}
	slog.Info("Encrypted to", "encryptedFile", encryptedFile)

	blake3Hash, err := BLAKE3File(encryptedFile)
	if err != nil {
		return "", "", fmt.Errorf("BLAKE3 hash failed: %w", err)
	}
	slog.Info("BLAKE3", "hash", blake3Hash)

	if err := os.Remove(partFile); err != nil {
		return "", "", fmt.Errorf("failed to remove original file: %w", err)
	}
	slog.Info("Removed original file", "partFile", partFile)

	return blake3Hash, encryptedFile, nil
}

func Encrypt(inputFile, outputFile string, recipient age.Recipient) error {
	in, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	w, err := age.Encrypt(out, recipient)
	if err != nil {
		return err
	}

	if _, err := io.Copy(w, in); err != nil {
		return err
	}

	return w.Close()
}

// BLAKE3File computes the BLAKE3 hash of a file
func BLAKE3File(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := blake3.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func Decrypt(inputFile, outputFile string, identity age.Identity) error {
	in, err := os.Open(inputFile)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(outputFile)
	if err != nil {
		return err
	}
	defer out.Close()

	r, err := age.Decrypt(in, identity)
	if err != nil {
		return err
	}

	if _, err := io.Copy(out, r); err != nil {
		return err
	}

	return nil
}

// DecryptAndVerify decrypts an encrypted part file and verifies its BLAKE3 hash
func DecryptAndVerify(encryptedFile, outputFile, expectedBlake3 string, identity age.Identity) error {
	slog.Info("Decrypting part file", "encryptedFile", encryptedFile)

	actualBlake3, err := BLAKE3File(encryptedFile)
	if err != nil {
		return fmt.Errorf("failed to calculate BLAKE3: %w", err)
	}

	if actualBlake3 != expectedBlake3 {
		return fmt.Errorf("BLAKE3 mismatch: expected %s, got %s", expectedBlake3, actualBlake3)
	}
	slog.Info("BLAKE3 verified", "hash", actualBlake3)

	if err := Decrypt(encryptedFile, outputFile, identity); err != nil {
		return fmt.Errorf("decryption failed: %w", err)
	}
	slog.Info("Decrypted to", "outputFile", outputFile)

	return nil
}
