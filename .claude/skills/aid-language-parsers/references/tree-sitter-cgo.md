# Tree-sitter and cgo

## Why cgo

Tree-sitter grammars are generated C parsers. Go reaches them through the
binding `github.com/smacker/go-tree-sitter`, which is itself cgo code: without
cgo its package has no `Node` type, and every grammar package is excluded by
its build constraints. Nine processors use it (all but Go, Rust and Swift), and
so does `internal/semantic` (Python grammar), which
`internal/processor/dependency.go` imports. So nothing in aid compiles without
cgo, not even `internal/language` on its own.

## Grammar sources

| Language | Go import | Where the C code comes from |
|---|---|---|
| python | `github.com/smacker/go-tree-sitter/python` | bundled in the binding module |
| kotlin | `github.com/smacker/go-tree-sitter/kotlin` | bundled in the binding module |
| javascript, java, csharp, cpp, ruby, php | `github.com/tree-sitter/tree-sitter-<name>/bindings/go` | the grammar's own Go module (version in `go.mod`); the processor wraps its pointer with `sitter.NewLanguage(...)`, PHP with `LanguagePHP()` |
| typescript, tsx | `tree-sitter-typescript` | a local wrapper module, see below |
| swift | `tree-sitter-swift` | vendored in `internal/parser/grammars/tree-sitter-swift`; only the unused tree-sitter processor imports it (`references/swift.md`) |
| go | none | standard library `go/parser` |
| rust | none | line parser; `go.mod` has only a commented-out replace line for a Rust grammar |

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
github.com/smacker/go-tree-sitter/python: build constraints exclude all Go files in ...
../go/pkg/mod/github.com/smacker/go-tree-sitter@.../iter.go:17:18: undefined: Node
```

There is no CGO=0 binary that starts and then reports missing parsers: the
`!cgo` files (`internal/language/registry_nocgo.go`,
`internal/language/nocgo.go#RegisterTreeSitterProcessors`, the `_nocgo.go`
grammar wrappers) are never part of a successful build.
`scripts/check-cgo.sh` fails when the `Makefile`, the `Dockerfile`, the scripts
or the workflows set `CGO_ENABLED=0`.

Use `CGO_ENABLED=1 CGO_CFLAGS=-w` for every build and test; `CGO_CFLAGS=-w`
only silences the C compiler warnings of the generated grammars. Cross builds
need a C cross-compiler for the target (the release workflow uses
`aarch64-linux-gnu-gcc` and `x86_64-w64-mingw32-gcc`).
