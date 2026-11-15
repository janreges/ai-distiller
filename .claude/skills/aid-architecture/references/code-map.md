# Code map

Which packages the `aid` binary is built from, and what else lives under
`internal/` and `cmd/`. Generated at this commit with:

```bash
export CGO_ENABLED=1 CGO_CFLAGS=-w
go list -deps ./cmd/aid | grep ai-distiller        # packages aid is built from
go list -e ./... | grep -E '/(internal|cmd)/'      # every package in the checkout
```

A package that `cmd/aid` reaches is compiled into the binary; whether its code
runs is a second question, answered per package below. Plan-level clean-ups
may delete packages; rerun the commands when this list matters.

## Reached by `cmd/aid`

Entry and CLI:

- `cmd/aid`: `main` sets the version and calls `internal/cli/root.go#Execute`.
- `internal/cli`: the flags, the modes and the output (`internal/cli/root.go`),
  the help texts (`internal/cli/help.go`).

Pipeline:

- `internal/processor`: `ProcessOptions` and the `LanguageProcessor`
  interface (`internal/processor/interface.go`), the processor registry, the
  directory walk and worker pool (`internal/processor/concurrent.go`), the
  pattern matcher, the raw processor and the dependency-aware analysis
  (`internal/processor/dependency.go`).
- `internal/ignore`: the `.aidignore` matcher.
- `internal/language`: registers the 12 language processors
  (`internal/language/registry_cgo.go#RegisterAll`), the stdin detector and
  the `--lang` aliases.
- `internal/language/python`, `typescript`, `javascript`, `golang`, `rust`,
  `swift`, `ruby`, `java`, `csharp`, `kotlin`, `cpp`, `php`: one processor
  each; what runs inside them is in the `aid-language-parsers` skill.
- `internal/language/commentutil`: comment-stripping helpers shared by the
  processors.
- `internal/ir`: the IR node types and the visitor.
- `internal/stripper`: removes what the flags exclude
  (`ir-and-stripper.md`).
- `internal/formatter`: the `text`, `md`, `jsonl`, `json-structured` and `xml`
  formatters and the per-language text formatters.
- `internal/importfilter`: the per-language filter of unused imports, run on
  the formatted text of the `text` format.
- `internal/summary`: the summary on stderr after a path run, with the token
  estimate (bytes divided by 4).

AI actions and support:

- `internal/ai`: the action interfaces, the registry and the output-path
  expansion.
- `internal/aiactions`: the ten registered actions, their embedded templates
  (`internal/aiactions/templates`) and the flows' file listing.
- `internal/project`: project root detection and the `.aid/` directory, used
  for the default output file and the AI action paths.
- `internal/debug`: the `-v`, `-vv`, `-vvv` debugger carried in the context.
- `internal/version`: version, commit and date, set by `-ldflags` at build
  time (`dev` otherwise).

Reached, but running only in a corner:

- `internal/semantic`: tree-sitter (Python) analysis and the symbol-table
  types; imported only by `internal/processor/dependency.go`, so it matters
  only with `--dependency-aware`. Its queries are embedded from
  `internal/semantic/queries`.
- `internal/parser/grammars`: not a package of this module but separate Go
  modules: `tree-sitter-typescript` (live) and `tree-sitter-swift` (imported
  only by Swift's unused tree-sitter processor), which `go.mod` replaces with
  these paths (`aid-language-parsers`, `references/tree-sitter-cgo.md`).

## Not reached by `cmd/aid`

| Package | What it is |
|---|---|
| `cmd/ast-dump` | prints the tree-sitter tree a processor walks (`aid-language-parsers`, section 4) |
| `cmd/test-audit` | audits the snapshot fixtures in `testdata/` through `internal/testrunner` (`aid-testing-debugging`) |
| `internal/conformance` | test-only package: the conformance suite and the CLI contract tests (`internal/conformance/doc.go`) |
| `internal/testrunner` | the snapshot suite, `BuildBinary` for tests, and the audit used by `cmd/test-audit` |

Every package in the table compiles with `go build`; only `cmd/aid` is built
by the release workflow.

## Directories without a package

- `internal/aiactions/templates`, `internal/semantic/queries`: files embedded
  with `//go:embed`.
- `internal/parser/grammars`: separate Go modules (above).
- `internal/conformance/testdata`: the conformance fixtures.
- `test-data/` at the top level (outside `internal/`): import-filter samples
  and Go samples; the snapshot fixtures are in `testdata/`.

A local checkout can hold more than the repository: untracked directories
that `go list` shows only there (an aid-validate command that imports a
missing internal/validation package and does not compile) and empty
directories git does not track. They are not part of the code; the full test
command excludes the first (`aid-testing-debugging`, section 2).
