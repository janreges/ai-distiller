use crate::store::Store;

/// How much a report says.
pub enum Level {
    Info,
    Debug,
}

/// Summarizes a store.
pub fn summary(store: &Store) -> String {
    format!("store at {}", store.path)
}
