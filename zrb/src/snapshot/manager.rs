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

#[cfg(test)]
mod tests {
    use super::*;
    use mockall::predicate::*;

    #[test]
    fn test_snapshot_manager_mock() {
        let filename = "snapshot.zfs";

        let mut mock = MockSnapshotManager::new();
        mock.expect_get_filename()
            .returning(|| filename.to_string());

        assert_eq!(mock.get_filename(), filename);
    }
}
