# Test inventory

Every tracked test file, grouped by package, with what it asserts. Built from
`git ls-files '*_test.go'` and `mcp-npm/test/*.js` and from reading the files. What
no test covers is in SKILL.md section 8.

Unit tests below run on inline source or on a hand-built IR unless the line says
they read `testdata/`, the conformance fixtures or a built binary.

## Suites

| Package | Files | What it asserts |
|---|---|---|
| `internal/testrunner` | `integration_test.go` | `TestIntegration`: the built `aid`, run on every `testdata/<lang>/<scenario>/source.*` with the flags in each expected file name, prints exactly that file |
| | `runner_test.go` | discovery fails for a scenario without a source file or `expected/`, rejects malformed expected names; file name to flags |
| | `build_test.go` | `BuildBinary` builds outside the repo and honours `AID_TEST_BINARY`; a mismatch message carries the command and a diff |
| `internal/conformance` | `conformance_test.go`, `fixtures_test.go`, `known_gaps_test.go` | `TestConformance`: every capability check of every language on `internal/conformance/testdata/`, with `knownGaps`; `TestKnownGapsAreLive`: every gap key names a check |
| | `unhandled_test.go` | `TestNoUnhandledNodesInTestdata`: no text formatter drops a node kind of any `testdata/*/*/source.*` unless it is in `knownUnhandled`, and every listed pair is still dropped |
| | `cli_test.go` | CLI contract on the built binary: a path argument wins over piped stdin and an open pipe does not hang it; `-` and no argument read stdin; `--lang` aliases, unknown and undetected languages; TSX on stdin matches a `.tsx` file; `--stdout` and `-o` combinations and the default `.aid/` file; summary sizes and wording; structured formats leak no metadata; `-o` with `--ai-action` fails; `--exclude-items=internal` removes internal members and keeps private ones |

## Parsers (`internal/language`)

| Package | Files | What it asserts |
|---|---|---|
| `internal/language` | `detector_test.go`, `names_test.go` | stdin language detection, on every conformance fixture and on look-alike snippets (generics, enums, JavaScript vs TypeScript or Kotlin, PHP without `<?php`); every `--lang` alias resolves to a registered processor |
| `internal/language/commentutil` | `commentutil_test.go` | stripping of C-style, JS and Doxygen comment delimiters; which doc comments describe the whole file; how comments attach to statements |
| `internal/language/cpp` | `processor_test.go` | basic and complex declarations; out-of-class definitions (qualified name, visibility); comments, Doxygen and trailing member docs; `friend`; docs inside preprocessor and `extern "C"` blocks; file doc |
| `internal/language/csharp` | `processor_test.go` | basic, modern and complex features; nullable types; visibility from modifiers including `protected internal`; comments, also inside bodies |
| `internal/language/golang` | `ast_parser_test.go`, `comprehensive_test.go` | function bodies are kept verbatim; `TestGoConstructs` formats `testdata/go/01_basic` in-process and compares with three of its snapshots |
| `internal/language/java` | `processor_comprehensive_test.go` | `TestJavaConstructs` does the same for `testdata/java/01_basic`; parser features; Javadoc and comment placement, `///` docs, initializers |
| `internal/language/javascript` | `processor_test.go` | `_` and `#` members are private, generators, imports, arrow bodies, comments, JSDoc tags, file doc |
| `internal/language/kotlin` | `processor_test.go` | basic, advanced and complex features; visibility; function bodies; comments around headers, in interfaces and objects, not taken from bodies |
| `internal/language/php` | `processor_test.go`, `processor_comprehensive_test.go` | classes, PHP 8 features, namespaces and `use`, traits, interfaces; five constructs from basic to attributes |
| `internal/language/python` | `processor_test.go` | processing options, annotated class attributes, docstrings as doc comments and their placement, decorators, comments and docstrings leaving with removed members |
| | `processor_fuzz_test.go` | `FuzzProcess`, seeded from `testdata/python/*/source.py`; without `-fuzz` it runs the seeds only |
| | `processor_bench_test.go` | benchmarks; they run only with `-bench` |
| `internal/language/ruby` | `processor_test.go` | `require`, `attr_*`, `include`, keyword calls, visibility sections, singleton methods, comment and doc-comment rules, shebang |
| `internal/language/rust` | `processor_test.go` | items and visibility; body boundaries (braces on their own line, in literals, one-line bodies); trait impl methods are public; enums with discriminants and payloads; inner docs; docs of hidden items |
| `internal/language/swift` | `line_parser_test.go` | the line parser: body extraction (multi-line and one-line bodies, braces inside string literals), property defaults without the `=`, the `struct` modifier, code after a block comment on the same line; through the processor and the text formatter: ordinary comments print only with `--comments` and doc comments by default, an empty block comment prints nothing, an extension header names its conformance once |
| `internal/language/typescript` | `ast_parser_test.go` | namespaces, ambient modules, return types and type guards, const types, `#private` fields and methods, arrow bodies, enum initialisers, comment and doc placement |

## Stripper, formatters, CLI

| Package | Files | What it asserts |
|---|---|---|
| `internal/stripper` | `stripper_test.go` | removal by visibility and of bodies; `--expand` on functions, classes, structs and qualified names; `--public=0`; a doc comment leaves with the declaration it documents and other comments stay; no node field is lost |
| `internal/formatter` | `<lang>_formatter_test.go` (all twelve languages) | the text rendering of bodies, comments, shebangs, enums, namespaces, visibility markers and type headers (Swift `struct` and `extension`) |
| | `body_test.go`, `comment_test.go`, `visibility_test.go` | shared body dedent and rendering, C-style comment writing, visibility prefixes |
| | `markdown_test.go`, `jsonl_test.go`, `json_structured_test.go`, `xml_test.go`, `registry_test.go` | each non-text format on hand-built IR; the format registry |
| `internal/cli` | `root_test.go` | output file names; flag parsing and its mapping to processing options (include and exclude lists, fields and methods, `--public=0`); the `-v` count |
| | `flags_doc_test.go` | every flag appears in the extended help and in `docs/user/COMMAND-LINE-OPTIONS.md`; help texts name only real flags; the doc lists every AI action; `--include-only` recipes keep members; redirect examples carry `--stdout` |

## Other Go packages

| Package | Files | What it asserts |
|---|---|---|
| `internal/processor` | `interface_test.go`, `registry_test.go`, `raw_processor_test.go`, `walk_test.go`, `exclude_test.go`, `aidignore_test.go`, `metadata_test.go` | default options; the processor registry; raw mode's text/binary decision; the worker count changes neither the files nor their order (walks `testdata/python`); include/exclude patterns below the walk root; `.aidignore`; the recorded source size |
| `internal/ignore` | `ignore_test.go`, `bin_obj_test.go` | `.aidignore` pattern matching, nested ignore files, comments and blank lines, `bin`/`obj` patterns with and without a slash |
| `internal/importfilter` | `filter_test.go`, `comprehensive_test.go` | the unused-import filter on Python, JavaScript and Go cases and on the samples in `test-data/import-filtering/`: a removed import no longer appears |
| `internal/ir` | `types_test.go`, `visitor_test.go`, `comment_test.go` | node types and their JSON, visitors and walker; parsers use the comment-format constants (a scan of `internal/language`) |
| `internal/project` | `root_test.go` | project root detection and the `.aid/` directory |
| `internal/semantic` | `typescript_analyzer_test.go` | the TypeScript analyzer on inline source |
| `internal/summary` | `*_formatter_test.go`, `printed_test.go` | the summary styles; printed output is not offered for loading |
| `internal/aiactions` | `source_files_test.go` | which source files an AI action collects |
| `cmd/ast-dump` | `main_test.go` | the tree-sitter dump of each tree-sitter language; Go, Rust and Swift are refused (build tag `cgo`) |

`docs/internal/proof-of-concepts/` holds two more test files, but each sits in its
own Go module, so `go list ./...` and the full suite do not include them.

## Outside `go test`

| Location | Run by | What it asserts |
|---|---|---|
| `mcp-npm/test/run-aid.test.js` | `npm test` | the process runner: output, exit codes, timeout, output cap, missing binary, the serial queue (children are `node -e` scripts) |
| `mcp-npm/test/tool-text.test.js` | `npm test` | texts the MCP tools return: saved paths, truncation notice, spawn failures, removed parameters |
| `mcp-npm/test/list-files.test.js` | `npm test` | the file listing tool: missing path, file instead of directory, unreadable subdirectories, the cap |
| `mcp-npm/test/postinstall.test.js` | `npm test` | postinstall helpers: platform mapping and unsupported platforms, asset names, versions, checksum lookup and verification, download errors |
| `scripts/tests/install_sh_test.sh`, `scripts/tests/check_skill_anchors_test.sh`, `scripts/tests/readme_links_test.sh` | CI job `checks` | the installer's version resolution; the skill anchor check; the relative links in `README.md` and `CONTRIBUTING.md` point at existing paths |
