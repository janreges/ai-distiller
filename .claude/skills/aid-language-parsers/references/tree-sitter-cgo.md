# Tree-sitter and cgo

## Why cgo

Tree-sitter grammars are generated C parsers. Go reaches them through the
official binding `github.com/tree-sitter/go-tree-sitter` (version in `go.mod`),
which is itself cgo code: without cgo its package has no `Node`, `Parser` or
`Language` type, and every grammar package is excluded by its build
constraints. Nine processors use it (all but Go, Rust and Swift), and so do
`internal/semantic` (Python and TypeScript grammars), which
`internal/processor/dependency.go` imports, and `cmd/ast-dump`. So nothing in
aid compiles without cgo, not even `internal/language` on its own.

## The runtime

The binding v0.25.0 bundles the tree-sitter C library 0.25 and loads grammars
of ABI 13 to 15. Code that uses it imports it as
`sitter "github.com/tree-sitter/go-tree-sitter"` and follows four rules:

- Close what you create. The binding has no finalizers: every `Parser`,
  `Tree`, `Query` and `QueryCursor` gets `defer x.Close()` right after it is
  created, or a `Close` method of its owner that the creator defers (the
  JavaScript and PHP `TreeSitterProcessor`, `semantic.Analyzer`,
  `semantic.TypeScriptAnalyzer`). A second `Close` of the same object frees
  it twice.
- A `*sitter.Node` does not keep its tree alive. Use a node only while its
  tree is open, and never store one in a struct field, map, channel or
  closure that outlives the function that closes the tree.
- Create and run the parser through `internal/language/tsparse`:
  `internal/language/tsparse/tsparse.go#NewParser` returns the error of an
  incompatible grammar, and `internal/language/tsparse/tsparse.go#Parse`
  honours the context. Do not call `Parser.ParseCtx`: in v0.25.0 a context
  cancelled during the parse makes it dereference a nil cancellation flag and
  crash the process.
- Every `Child`, `Parent` or sibling call returns a new `*sitter.Node`, so `==`
  between two nodes is false even for the same node; compare with
  `a.Equals(*b)`. `Kind()` is the node type, `StartPosition()` and
  `EndPosition()` the points, and counts, indexes and byte offsets are `uint`.

## Grammar sources

| Language | Go import | Where the C code comes from |
|---|---|---|
| python, javascript, java, csharp, cpp, ruby, php | `github.com/tree-sitter/tree-sitter-<name>/bindings/go` | the grammar's own Go module (version in `go.mod`); the processor wraps its pointer with `sitter.NewLanguage(...)`, PHP with `LanguagePHP()` |
| kotlin | `github.com/fwcd/tree-sitter-kotlin/bindings/go` | the grammar's own Go module (fwcd, pinned to commit `d6af96d2` in `go.mod`, ABI 14); its `parser.c` is about 35 MB; measured once when it replaced the copy bundled with the previous Go binding, a cold `go build -a` went from 27.3 s to 31.3 s and the binary grew by 2.0 MB; wrapped with `sitter.NewLanguage(...)` |
| typescript, tsx | `tree-sitter-typescript` | a local wrapper module, see below |
| swift | none | line parser (`references/swift.md`) |
| go | none | standard library `go/parser` |
| rust | none | line parser; no Rust grammar is in `go.mod` |

`cmd/ast-dump` imports the same grammars (`cmd/ast-dump/main.go#language`), so
the tree it prints is the tree the processor walks.

## The TypeScript submodule

`go.mod` replaces the module `tree-sitter-typescript` with the directory
`internal/parser/grammars/tree-sitter-typescript`. That directory holds a small
wrapper (`typescript.go` and `tsx.go`, which `#include` the generated
`parser.c` and `scanner.c`, and `typescript_nocgo.go`) and the C sources as
the git submodule `internal/parser/grammars/tree-sitter-typescript/source`
(upstream `tree-sitter/tree-sitter-typescript`, URL in `.gitmodules`; the
commit is pinned by the repository and `git submodule status` shows it).

A clone without the submodule has an empty `source/`, and every build that
reaches the TypeScript processor fails with:

```text
./tsx.go:7:11: fatal error: ./source/tsx/src/parser.c: No such file or directory
```

Fix it with:

```bash
git submodule update --init
```

CI and release workflows check out with `submodules: recursive`
(`.github/workflows/ci.yml`, `.github/workflows/release.yml`). The wrapper
compiles whatever `parser.c` and `scanner.c` the submodule checkout holds.

## What fails without cgo

```bash
CGO_ENABLED=0 go build ./cmd/aid
```

fails at compile time, with errors such as:

```text
github.com/tree-sitter/tree-sitter-python/bindings/go: build constraints exclude all Go files in ...
internal/language/tsparse/tsparse.go:24:29: undefined: sitter.Language
```

The module does not compile with `CGO_ENABLED=0`, and there is no `!cgo` stub
registry that would start and report missing parsers: `internal/language/registry_cgo.go`
is the only registry and carries `//go:build cgo`.
`scripts/check-cgo.sh` fails when the `Makefile`, the `Dockerfile`, the scripts
or the workflows set `CGO_ENABLED=0`.

Use `CGO_ENABLED=1 CGO_CFLAGS=-w` for every build and test; `CGO_CFLAGS=-w`
only silences the C compiler warnings of the generated grammars. Cross builds
need a C cross-compiler for the target (the release workflow uses
`aarch64-linux-gnu-gcc` and `x86_64-w64-mingw32-gcc`).
