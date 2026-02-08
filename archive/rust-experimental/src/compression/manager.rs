use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

#[automock]
pub trait Compressor {
    fn get_extension(&self) -> String;

    fn compress(&self, filepath: &Path) -> Result<PathBuf, Error>;

    fn verify(&self, filepath: &Path) -> Result<(), Error>;
}
