use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

#[automock]
pub trait Hasher {
    fn cal_file(&self, filepath: &Path) -> Result<(), Error>;

    fn update(&mut self, data: &[u8]) -> Result<(), Error>;

    fn reset(&mut self);

    fn get_digest(&self) -> Vec<u8>;

    fn get_hex_digest(&self) -> String;
}
