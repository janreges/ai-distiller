/// Turns text into a URL slug.
pub fn slug(text: &str) -> String {
    text.to_lowercase().replace(' ', "-")
}
