module github.com/janreges/ai-distiller

go 1.25.0

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/dustin/go-humanize v1.1.0
	github.com/fwcd/tree-sitter-kotlin v0.0.0-20261031170920-d6af96d2e23e
	github.com/mattn/go-isatty v0.0.24
	github.com/pmezard/go-difflib v1.0.0
	github.com/spf13/cobra v1.10.2
	github.com/spf13/pflag v1.1.0
	github.com/stretchr/testify v1.12.1
	github.com/tree-sitter/go-tree-sitter v0.25.0
	github.com/tree-sitter/tree-sitter-c-sharp v0.23.1
	github.com/tree-sitter/tree-sitter-cpp v0.23.4
	github.com/tree-sitter/tree-sitter-java v0.23.5
	github.com/tree-sitter/tree-sitter-javascript v0.23.1
	github.com/tree-sitter/tree-sitter-php v0.23.12
	github.com/tree-sitter/tree-sitter-python v0.25.0
	github.com/tree-sitter/tree-sitter-ruby v0.23.1
	golang.org/x/term v0.45.0
	tree-sitter-typescript v0.0.0
)

replace tree-sitter-typescript => ./internal/parser/grammars/tree-sitter-typescript

require (
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/mattn/go-pointer v0.0.1 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/sys v0.47.0 // indirect
)
