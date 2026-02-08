package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"filippo.io/age"
)

func generateKey(_ context.Context) error {
	fmt.Println("Generating age public and private key pair...")

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	publicKey := identity.Recipient().String()
	privateKey := identity.String()

	// TODO: Securely store this key
	fmt.Println("\n=== Age Key Pair Generated ===")
	fmt.Printf("Public key:  %s\n", publicKey)
	fmt.Printf("Private key: %s\n", privateKey)
	fmt.Println("\n!! Keep your private key secure !!")

	return nil
}

func testKeys(_ context.Context, configPath, privateKeyPath string) error {
	fmt.Println("Testing age key pair compatibility...")

	// Load config to get public key
	config, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Parse public key from config
	recipient, err := age.ParseX25519Recipient(config.AgePublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key from config: %w", err)
	}

	fmt.Printf("Public key from config: %s\n", config.AgePublicKey)

	// Load private key
	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(privateKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	fmt.Printf("Private key loaded from: %s\n", privateKeyPath)

	// Create temp directory for test
	tempDir, err := os.MkdirTemp("", "zrb_key_test_*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Create test file with known content
	testContent := "ZFS Remote Backup - Key Pair Test - " + time.Now().Format(time.RFC3339)
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}

	// Encrypt with public key
	encryptedFile := filepath.Join(tempDir, "test.txt.age")

	fmt.Println("\nEncrypting test data with public key...")

	if err := encryptWithAge(testFile, encryptedFile, recipient); err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	fmt.Println("Encryption successful")

	// Decrypt with private key
	decryptedFile := filepath.Join(tempDir, "test_decrypted.txt")

	fmt.Println("Decrypting test data with private key...")

	if err := decryptWithAge(encryptedFile, decryptedFile, identity); err != nil {
		return fmt.Errorf("decryption failed: %w\nThis means the private key does not match the public key in config", err)
	}

	fmt.Println("Decryption successful")

	// Verify content matches
	decryptedContent, err := os.ReadFile(decryptedFile)
	if err != nil {
		return fmt.Errorf("failed to read decrypted file: %w", err)
	}

	if string(decryptedContent) != testContent {
		return fmt.Errorf("content mismatch: decrypted content does not match original")
	}

	fmt.Println("Content verification successful")

	return nil
}
