package keys

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"zrb/internal/config"
	"zrb/internal/crypto"

	"filippo.io/age"
)

func Generate(_ context.Context) error {
	fmt.Println("Generating age public and private key pair...")

	identity, err := age.GenerateX25519Identity()
	if err != nil {
		return fmt.Errorf("failed to generate key pair: %w", err)
	}

	publicKey := identity.Recipient().String()
	privateKey := identity.String()

	fmt.Println("\n=== Age Key Pair Generated ===")
	fmt.Printf("Public key:  %s\n", publicKey)
	fmt.Printf("Private key: %s\n", privateKey)
	fmt.Println("\n!! Keep your private key secure !!")

	return nil
}

func Test(_ context.Context, configPath, privateKeyPath string) error {
	fmt.Println("Testing age key pair compatibility...")

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	recipient, err := age.ParseX25519Recipient(cfg.AgePublicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key from config: %w", err)
	}

	fmt.Printf("Public key from config: %s\n", cfg.AgePublicKey)

	privateKeyData, err := os.ReadFile(privateKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read private key: %w", err)
	}

	identity, err := age.ParseX25519Identity(strings.TrimSpace(string(privateKeyData)))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	fmt.Printf("Private key loaded from: %s\n", privateKeyPath)

	tempDir, err := os.MkdirTemp("", "zrb_key_test_*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	testContent := "ZFS Remote Backup - Key Pair Test - " + time.Now().Format(time.RFC3339)
	testFile := filepath.Join(tempDir, "test.txt")

	if err := os.WriteFile(testFile, []byte(testContent), 0o644); err != nil {
		return fmt.Errorf("failed to create test file: %w", err)
	}

	encryptedFile := filepath.Join(tempDir, "test.txt.age")

	fmt.Println("\nEncrypting test data with public key...")

	if err := crypto.Encrypt(testFile, encryptedFile, recipient); err != nil {
		return fmt.Errorf("encryption failed: %w", err)
	}

	fmt.Println("Encryption successful")

	decryptedFile := filepath.Join(tempDir, "test_decrypted.txt")

	fmt.Println("Decrypting test data with private key...")

	if err := crypto.Decrypt(encryptedFile, decryptedFile, identity); err != nil {
		return fmt.Errorf("decryption failed: %w\nThis means the private key does not match the public key in config", err)
	}

	fmt.Println("Decryption successful")

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
