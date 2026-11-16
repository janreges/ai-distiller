mod cli;
#[cfg(test)]
mod tests;

use std::collections::HashMap;

use demo_app::store::Store;
use util_kit::slug;

/// Starts the app. A doc example such as `crate::ghost::Thing` names no
/// module the code uses.
fn main() {
    let args = cli::parse();
    let store = Store::open(&args.path);
    let mut seen: HashMap<String, usize> = HashMap::new();
    seen.insert(slug(&args.path), 1);
    println!("{}", demo_app::report::summary(&store));
}
