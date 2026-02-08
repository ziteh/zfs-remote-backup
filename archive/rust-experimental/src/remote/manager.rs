use anyhow::{Error, Result, anyhow};
use mockall::automock;
use std::{
    collections::HashMap,
    path::{Path, PathBuf},
};

pub type Tags = HashMap<String, String>;
pub type Metadata = HashMap<String, String>;

#[automock]
pub trait RemoteManager {
    fn upload(
        &self,
        src_filepath: &Path,
        dst_filepath: &Path,
        tags: Option<Tags>,
        metadata: Option<Metadata>,
    ) -> Result<(), Error>;
}
