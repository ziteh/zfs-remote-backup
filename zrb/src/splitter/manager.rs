use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

#[automock]
pub trait Splitter {
    fn get_extension(&self, index: u64) -> String;

    fn split(&self, filename: &Path, index: u64) -> Result<PathBuf, Error>;
}
