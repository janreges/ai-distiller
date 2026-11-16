/// Normalizes a path.
pub fn normalize(path: &str) -> String {
    path.trim_end_matches('/').to_string()
}
