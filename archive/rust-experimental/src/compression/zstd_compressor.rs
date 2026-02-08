use crate::compression::manager::Compressor;
use anyhow::{Context, Error, Result, anyhow};
use std::fs::File;
use std::io::{BufReader, BufWriter};
use std::path::{Path, PathBuf};

pub struct ZstdCompressor {
    compression_level: i32,
}

impl ZstdCompressor {
    pub fn new(compression_level: i32) -> Self {
        Self { compression_level }
    }
}

impl Default for ZstdCompressor {
    fn default() -> Self {
        Self::new(3) // Default compression level is 3
    }
}

impl Compressor for ZstdCompressor {
    fn get_extension(&self) -> String {
        "zst".to_string()
    }

    fn compress(&self, filepath: &Path) -> Result<PathBuf, Error> {
        if !filepath.exists() {
            return Err(anyhow!("File does not exist: {}", filepath.display()));
        }

        // Create compressed file path
        let mut compressed_path = filepath.to_path_buf();
        compressed_path.set_extension(format!(
            "{}.zst",
            filepath
                .extension()
                .and_then(|ext| ext.to_str())
                .unwrap_or("")
        ));

        // Open source file
        let input_file = File::open(filepath)
            .with_context(|| format!("Failed to open source file: {}", filepath.display()))?;
        let reader = BufReader::new(input_file);

        // Create compressed file
        let output_file = File::create(&compressed_path).with_context(|| {
            format!(
                "Failed to create compressed file: {}",
                compressed_path.display()
            )
        })?;
        let writer = BufWriter::new(output_file);

        // Perform compression
        zstd::stream::copy_encode(reader, writer, self.compression_level)
            .with_context(|| format!("Failed to compress file: {}", filepath.display()))?;

        Ok(compressed_path)
    }

    fn verify(&self, filepath: &Path) -> Result<(), Error> {
        if !filepath.exists() {
            return Err(anyhow!(
                "Compressed file does not exist: {}",
                filepath.display()
            ));
        }

        // Check file extension
        if !filepath
            .extension()
            .and_then(|ext| ext.to_str())
            .map(|ext| ext == "zst")
            .unwrap_or(false)
        {
            return Err(anyhow!(
                "File is not a zstd compressed file: {}",
                filepath.display()
            ));
        }

        // Try to read and decompress first few bytes to verify file integrity
        let file = File::open(filepath).with_context(|| {
            format!(
                "Failed to open compressed file for verification: {}",
                filepath.display()
            )
        })?;
        let reader = BufReader::new(file);

        // Try to create decoder to verify file format
        let mut decoder = zstd::Decoder::new(reader)
            .with_context(|| format!("Failed to create zstd decoder: {}", filepath.display()))?;

        // Try to read a small amount of data to verify file format
        let mut buffer = [0u8; 1024];
        match std::io::Read::read(&mut decoder, &mut buffer) {
            Ok(_) => Ok(()),
            Err(e) => Err(anyhow!(
                "Compressed file verification failed: {} - {}",
                filepath.display(),
                e
            )),
        }
    }
}
