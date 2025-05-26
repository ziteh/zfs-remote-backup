use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

#[automock]
pub trait Encryptor {
    fn get_extension(&self) -> String;

    fn encrypt(&self, filepath: &Path) -> Result<PathBuf, Error>;

    fn decrypt(&self, filepath: &Path) -> Result<PathBuf, Error>;
}
