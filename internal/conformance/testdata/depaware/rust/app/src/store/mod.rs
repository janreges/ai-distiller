mod disk;

use super::report::Level;

/// A store of records on disk.
pub struct Store {
    pub path: String,
    pub level: Level,
}

impl Store {
    /// Opens the store at path.
    pub fn open(path: &str) -> Store {
        Store { path: disk::normalize(path), level: Level::Info }
    }
}
