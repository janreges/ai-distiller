use super::cli;

#[test]
fn parses_without_arguments() {
    assert!(cli::parse().path.is_empty());
}
