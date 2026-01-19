package main

import (
	"crypto/sha256"
	"filippo.io/age"
	"fmt"
	"io"
	"log"
	"os"
)

// processPartFile encrypts a snapshot part, calculates SHA256, and removes the original
func processPartFile(partFile string, recipient age.Recipient) (string, string, error) {
	log.Printf("Processing %s...", partFile)

	// Age encryption
	encryptedFile := partFile + ".age"
	if err := encryptWithAge(partFile, encryptedFile, recipient); err != nil {
		return "", "", fmt.Errorf("age encryption failed: %w", err)
	}
	log.Printf("  Encrypted to: %s", encryptedFile)

	// SHA-256 hash
	sha256Hash, err := calculateSHA256(encryptedFile)
	if err != nil {
		return "", "", fmt.Errorf("SHA-256 hash failed: %w", err)
	}
	log.Printf("  SHA-256: %s", sha256Hash)

	// Delete original file
	if err := os.Remove(partFile); err != nil {
		return "", "", fmt.Errorf("failed to remove original file: %w", err)
	}
	log.Printf("  Removed original file: %s", partFile)

	return sha256Hash, encryptedFile, nil
}

// encryptWithAge encrypts a file using age encryption
func encryptWithAge(inputFile, outputFile string, recipient age.Recipient) error {
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

	if err := w.Close(); err != nil {
		return err
	}

	return nil
}

// calculateSHA256 computes the SHA256 hash of a file
func calculateSHA256(filename string) (string, error) {
	f, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer f.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}
