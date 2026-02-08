pub mod manager;
pub mod model;
pub mod binary_file_io;

// Re-export commonly used types for convenience
pub use manager::{FileIo, StatusManager};
pub use binary_file_io::BinaryFileIo;
pub use model::*;

#[cfg(test)]
mod manager_tests;

#[cfg(test)]
mod binary_file_io_tests;
