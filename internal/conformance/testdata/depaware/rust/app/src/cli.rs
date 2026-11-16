/// Command-line arguments.
pub struct Args {
    pub path: String,
}

/// Reads the arguments.
pub fn parse() -> Args {
    Args { path: std::env::args().nth(1).unwrap_or_default() }
}
