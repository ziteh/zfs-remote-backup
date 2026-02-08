use crate::encryption::manager::Encryptor;
use age::x25519;
use anyhow::{Context, Error, Result, anyhow};
use std::fs::File;
use std::io::{BufReader, BufWriter, Write};
use std::path::{Path, PathBuf};

pub struct AgeEncryptor {
    public_key: Vec<x25519::Recipient>,
    private_key: Option<x25519::Identity>,
}

impl AgeEncryptor {
    /// Create new encryptor with recipients (public keys)
    pub fn new(public_key: Vec<x25519::Recipient>) -> Self {
        Self {
            public_key,
            private_key: None,
        }
    }

    /// Create new encryptor with identity (private key) for decryption
    pub fn with_identity(
        public_key: Vec<x25519::Recipient>,
        private_key: x25519::Identity,
    ) -> Self {
        Self {
            public_key,
            private_key: Some(private_key),
        }
    }

    /// Generate a new identity (key pair)
    pub fn generate_identity() -> x25519::Identity {
        x25519::Identity::generate()
    }

    /// Parse recipient from public key string
    pub fn parse_recipient(public_key: &str) -> Result<x25519::Recipient, Error> {
        public_key
            .parse::<x25519::Recipient>()
            .map_err(|e| anyhow!("Failed to parse recipient: {}", e))
    }

    /// Parse identity from private key string
    pub fn parse_identity(private_key: &str) -> Result<x25519::Identity, Error> {
        private_key
            .parse::<x25519::Identity>()
            .map_err(|e| anyhow!("Failed to parse identity: {}", e))
    }
}

impl Encryptor for AgeEncryptor {
    fn get_extension(&self) -> String {
        "age".to_string()
    }

    fn encrypt(&self, filepath: &Path) -> Result<PathBuf, Error> {
        if !filepath.exists() {
            return Err(anyhow!("File does not exist: {}", filepath.display()));
        }

        if self.public_key.is_empty() {
            return Err(anyhow!("No recipients specified for encryption"));
        }

        // Create encrypted file path
        let mut encrypted_path = filepath.to_path_buf();
        encrypted_path.set_extension(format!(
            "{}.age",
            filepath
                .extension()
                .and_then(|ext| ext.to_str())
                .unwrap_or("")
        ));

        // Open source file
        let input_file = File::open(filepath)
            .with_context(|| format!("Failed to open source file: {}", filepath.display()))?;
        let mut reader = BufReader::new(input_file);

        // Create encrypted file
        let output_file = File::create(&encrypted_path).with_context(|| {
            format!(
                "Failed to create encrypted file: {}",
                encrypted_path.display()
            )
        })?;
        let writer = BufWriter::new(output_file);

        // Create encryptor with recipients
        let encryptor = age::Encryptor::with_recipients(
            self.public_key.iter().map(|r| r as &dyn age::Recipient),
        )
        .map_err(|e| anyhow!("Failed to create age encryptor: {}", e))?;

        // Perform encryption
        let mut encrypted_writer = encryptor
            .wrap_output(writer)
            .map_err(|e| anyhow!("Failed to wrap output for encryption: {}", e))?;

        std::io::copy(&mut reader, &mut encrypted_writer)
            .with_context(|| format!("Failed to encrypt file: {}", filepath.display()))?;

        encrypted_writer
            .finish()
            .map_err(|e| anyhow!("Failed to finalize encryption: {}", e))?;

        Ok(encrypted_path)
    }

    fn decrypt(&self, filepath: &Path) -> Result<PathBuf, Error> {
        if !filepath.exists() {
            return Err(anyhow!(
                "Encrypted file does not exist: {}",
                filepath.display()
            ));
        }

        let identity = self
            .private_key
            .as_ref()
            .ok_or_else(|| anyhow!("No identity (private key) available for decryption"))?;

        // Check file extension
        if !filepath
            .extension()
            .and_then(|ext| ext.to_str())
            .map(|ext| ext == "age")
            .unwrap_or(false)
        {
            return Err(anyhow!(
                "File is not an age encrypted file: {}",
                filepath.display()
            ));
        }

        // Create decrypted file path
        let mut decrypted_path = filepath.to_path_buf();
        let original_extension = filepath
            .file_stem()
            .and_then(|stem| stem.to_str())
            .and_then(|stem| {
                if let Some(dot_pos) = stem.rfind('.') {
                    Some(&stem[dot_pos + 1..])
                } else {
                    None
                }
            });

        if let Some(ext) = original_extension {
            decrypted_path.set_extension(ext);
        } else {
            decrypted_path.set_extension("");
        }

        // Open encrypted file
        let input_file = File::open(filepath)
            .with_context(|| format!("Failed to open encrypted file: {}", filepath.display()))?;
        let reader = BufReader::new(input_file);

        // Create decrypted file
        let output_file = File::create(&decrypted_path).with_context(|| {
            format!(
                "Failed to create decrypted file: {}",
                decrypted_path.display()
            )
        })?;
        let mut writer = BufWriter::new(output_file);

        // Create decryptor with identity
        let decryptor = age::Decryptor::new(reader)
            .map_err(|e| anyhow!("Failed to create age decryptor: {}", e))?;

        let mut decrypted_reader = decryptor
            .decrypt(std::iter::once(&*identity as &dyn age::Identity))
            .map_err(|e| anyhow!("Failed to decrypt file: {}", e))?;

        // Perform decryption
        std::io::copy(&mut decrypted_reader, &mut writer).with_context(|| {
            format!(
                "Failed to write decrypted data: {}",
                decrypted_path.display()
            )
        })?;

        writer.flush().with_context(|| {
            format!(
                "Failed to flush decrypted file: {}",
                decrypted_path.display()
            )
        })?;

        Ok(decrypted_path)
    }
}
