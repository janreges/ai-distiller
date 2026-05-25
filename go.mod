module github.com/janreges/ai-distiller

go 1.25.0

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/dustin/go-humanize v1.0.1
	github.com/mattn/go-isatty v0.0.22
	github.com/smacker/go-tree-sitter v0.0.0-20240827094217-dd81d9e9be82
	github.com/spf13/cobra v1.10.2
	github.com/stretchr/testify v1.11.1
	github.com/tetratelabs/wazero v1.11.0
	github.com/tree-sitter/tree-sitter-c-sharp v0.23.1
	github.com/tree-sitter/tree-sitter-cpp v0.23.4
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.23.1
	github.com/tree-sitter/tree-sitter-php v0.23.12
	github.com/tree-sitter/tree-sitter-ruby v0.23.1
	golang.org/x/term v0.43.0
	tree-sitter-swift v0.0.0
	tree-sitter-typescript v0.0.0
)

replace tree-sitter-swift => ./internal/parser/grammars/tree-sitter-swift

replace tree-sitter-typescript => ./internal/parser/grammars/tree-sitter-typescript

// replace tree-sitter-rust => ./internal/parser/grammars/tree-sitter-rust

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/spf13/pflag v1.0.9 // indirect
	golang.org/x/sys v0.44.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
