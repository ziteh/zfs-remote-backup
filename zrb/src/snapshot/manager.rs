use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::path::{Path, PathBuf};

#[automock]
pub trait SnapshotManager {
    fn get_filename(&self) -> String;

    fn export(
        &self,
        out_dir: &Path,
        dataset: &str,
        base_snapshot: &str,
        ref_snapshot: &str,
    ) -> Result<PathBuf, Error>;

    fn verify(&self, dataset: &str, filepath: &Path) -> Result<(), Error>;

    fn list(&self, dataset: &str) -> Result<Vec<String>, Error>;
}
