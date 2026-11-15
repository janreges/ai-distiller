# Tree-sitter and cgo

## Why cgo

Tree-sitter grammars are generated C parsers. Go reaches them through the
official binding `github.com/tree-sitter/go-tree-sitter` (version in `go.mod`),
which is itself cgo code: without cgo its package has no `Node`, `Parser` or
`Language` type, and every grammar package is excluded by its build
constraints. Ten processors use it (all but Go and Swift), and so do
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
  crash the process. `internal/language/tsparse/tsparse.go#ParseBudget` also
  stops a parse after a number of progress checks (one per about 100 parser
  operations) and returns `ErrBudget`, so the same text gives the same result
  on any machine; the check rate is about the same per byte in clean code and
  in error recovery, so a budget bounds time only through its cap on the
  length parsed. Each call leaves about a hundred bytes that go-pointer never
  frees, so use it only for a parse that can run away (the Rust macro bodies).
- Every `Child`, `Parent` or sibling call returns a new `*sitter.Node`, so `==`
  between two nodes is false even for the same node; compare with
  `a.Equals(*b)`. `Kind()` is the node type, `StartPosition()` and
  `EndPosition()` the points, and counts, indexes and byte offsets are `uint`.

The binding points the C runtime's allocator at Go callbacks, which cost 13 to
17 percent of parse CPU; `internal/language/tsparse/allocator.go` switches it
back to libc in its `init`, so keep a `tsparse` import in every package that
parses. That is safe only for go-tree-sitter v0.25.0: on every runtime upgrade
re-read the binding's `allocator.go` and every place it frees memory.

## Grammar sources

| Language | Go import | Where the C code comes from |
|---|---|---|
| python, javascript, java, csharp, cpp, ruby, php | `github.com/tree-sitter/tree-sitter-<name>/bindings/go` | the grammar's own Go module (version in `go.mod`); the processor wraps its pointer with `sitter.NewLanguage(...)`, PHP with `LanguagePHP()` |
| kotlin | `github.com/fwcd/tree-sitter-kotlin/bindings/go` | the grammar's own Go module (fwcd, pinned to commit `d6af96d2` in `go.mod`, ABI 14); its `parser.c` is about 35 MB; measured once when it replaced the copy bundled with the previous Go binding, a cold `go build -a` went from 27.3 s to 31.3 s and the binary grew by 2.0 MB; wrapped with `sitter.NewLanguage(...)` |
| rust | `github.com/tree-sitter/tree-sitter-rust/bindings/go` | the grammar's own Go module (v0.24.2 in `go.mod`, ABI 15); its `parser.c` is about 6.5 MB; wrapped with `sitter.NewLanguage(...)` |
| typescript, tsx | `tree-sitter-typescript` | a local wrapper module, see below |
| swift | none | line parser (`references/swift.md`) |
| go | none | standard library `go/parser` |

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

Use `CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"` for every build and test. `-w` silences the C
compiler warnings of the generated grammars; `-O2` is needed because setting
`CGO_CFLAGS` replaces cgo's default `-g -O2`, and without it the tree-sitter C code
is built at `-O0` and parses about twice as slowly. Cross builds
need a C cross-compiler for the target (the release workflow uses
`aarch64-linux-gnu-gcc` and `x86_64-w64-mingw32-gcc`).
