use crate::hash::manager::Hasher;
use anyhow::{Context, Error, Result};
use blake3::Hasher as Blake3HashInner;
use std::fs::File;
use std::io::{BufReader, Read};
use std::path::Path;

pub struct Blake3Hasher {
    hasher: Blake3HashInner,
}

impl Blake3Hasher {
    pub fn new() -> Self {
        Self {
            hasher: Blake3HashInner::new(),
        }
    }
}

impl Default for Blake3Hasher {
    fn default() -> Self {
        Self::new()
    }
}

impl Hasher for Blake3Hasher {
    fn cal_file(&self, filepath: &Path) -> Result<(), Error> {
        if !filepath.exists() {
            return Err(anyhow::anyhow!(
                "File does not exist: {}",
                filepath.display()
            ));
        }

        let file = File::open(filepath)
            .with_context(|| format!("Failed to open file for hashing: {}", filepath.display()))?;
        let mut reader = BufReader::new(file);
        let mut buffer = [0u8; 8192];
        let mut hasher = Blake3HashInner::new();

        loop {
            let bytes_read = reader
                .read(&mut buffer)
                .with_context(|| format!("Failed to read file: {}", filepath.display()))?;

            if bytes_read == 0 {
                break;
            }

            hasher.update(&buffer[..bytes_read]);
        }

        Ok(())
    }

    fn update(&mut self, data: &[u8]) -> Result<(), Error> {
        self.hasher.update(data);
        Ok(())
    }

    fn reset(&mut self) {
        self.hasher = Blake3HashInner::new();
    }

    fn get_digest(&self) -> Vec<u8> {
        self.hasher.finalize().as_bytes().to_vec()
    }

    fn get_hex_digest(&self) -> String {
        self.hasher.finalize().to_hex().to_string()
    }
}
