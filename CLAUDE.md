# AI Distiller (`aid`)

## 1. What this is

`aid` is a Go CLI that distils source code into a compact structure (signatures,
types, imports, docs) so a codebase fits an LLM context window. It supports 12
languages: Python, TypeScript, JavaScript, Go, Rust, Swift, Ruby, Java, C#, Kotlin,
C++ and PHP. The default output format is `text`. The output is not valid source
code by design: it is optimised for density, not for compiling.

## 2. Rules that are never optional

- No mocks, stubs or simulated data in code or tests. Tests run the real parser,
  formatter and binary. If something cannot be done properly, leave a TODO.
- cgo is always on: `CGO_ENABLED=1 CGO_CFLAGS=-w` for every build and test. The
  tree-sitter grammars are C code and the project does not compile with
  `CGO_ENABLED=0` (CI runs `scripts/check-cgo.sh`, which rejects a
  tracked `CGO_ENABLED=0` in `Makefile`, `Dockerfile`, `scripts/`, `mcp-npm/scripts/`
  and `.github/workflows/`).
- Code, comments, commit messages and docs are English. Talk to the owner in Czech
  when they write Czech.
- Releases run through `.github/workflows/release.yml`, triggered by pushing a tag
  `vX.Y.Z` (or manually via `workflow_dispatch` with that tag). Releasing by hand is a
  last resort with its own procedure in the `aid-release-mcp` skill.
- Never commit binaries (`aid`, `build/`) or `.aid/` output.

## 3. Commands that work

Run from the repository root.

```bash
export CGO_ENABLED=1 CGO_CFLAGS=-w
make build                                   # build/aid
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')   # full suite
go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner                     # one language's snapshots
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner  # update them, then review git diff testdata
go test -count=1 ./internal/conformance      # conformance suite
make aid ARGS="testdata/python/01_basic/source.py --stdout"   # build and run
sh scripts/check-skill-anchors.sh            # SKILL CHECK
```

More commands and the update workflow: `.claude/skills/aid-testing-debugging/SKILL.md`.

## 4. Three things that surprise people

- Without `--stdout`, `aid PATH` writes `.aid/aid.<name>[.<options>].<ext>` under
  the project root and prints no distilled text (a progress line and the saved path
  go to stderr). The root is the nearest directory at or above the
  working directory (at most 12 levels) that holds a marker (`.aidrc`, a language
  marker such as `go.mod`, or `.git`), else `AID_PROJECT_ROOT`, else the working
  directory (`internal/project/root.go#findRootUncached`). `--stdout` prints and writes no file; `-o FILE` writes FILE.
- The snapshot and conformance suites build their own binary. The `./aid` in the
  repository root is not used by tests unless `AID_TEST_BINARY` is set
  (`internal/testrunner/build.go#BuildBinary`), so rebuilding it proves nothing.
- Stdin: a path argument wins over piped stdin. With no path and a non-TTY stdin,
  `aid` reads stdin; `-` reads stdin explicitly (`internal/cli/root.go#runDistiller`).
  Detach stdin with `</dev/null` when scripting a run on a path.

## 5. Map

- `cmd/aid`: the CLI entry point; other `cmd/*` are developer tools.
- `internal/cli`: flags, the run pipeline, stdin handling.
- `internal/language`: one processor per language plus the registry and stdin detector.
- `internal/parser/grammars`: the TypeScript and Swift grammar modules that `go.mod` replaces; there is no Go package here.
- `internal/ir`: the intermediate representation every processor produces.
- `internal/stripper`: removes what the flags exclude (visibility, bodies, comments).
- `internal/formatter`: text, markdown, json, jsonl and xml output.
- `internal/processor`: file discovery and parallel processing.
- `internal/aiactions`, `internal/ai`, `internal/summary`: `--ai-action` prompts and summaries.
- `internal/project`: project root detection and the `.aid/` output directory.
- `internal/testrunner`, `internal/conformance`: snapshot and conformance suites.
- `testdata/`, `mcp-npm/`: snapshot fixtures; the npm MCP server package.

Every package, and which of them run: `.claude/skills/aid-architecture/references/code-map.md`.

## 6. Which skill to load

| Task | Skill |
|---|---|
| Running or updating tests, a test fails, finding a bug's layer, `-v` debug output | `.claude/skills/aid-testing-debugging/SKILL.md` |
| A language's output is wrong, parser or visibility change, adding a language | `.claude/skills/aid-language-parsers/SKILL.md` |
| Where something is implemented, adding a CLI flag or mode, file selection, the stripper, git mode, AI actions | `.claude/skills/aid-architecture/SKILL.md` |
| The IR is right but the printed output is wrong, changing how something prints, a new IR field in the output, a new output format, imports missing from the text, the summary line | `.claude/skills/aid-output-formatting/SKILL.md` |
| Cutting or checking a release, versions, the release workflow, cgo cross-builds, `install.sh`/`install.ps1`, the npm package and MCP server, an install or MCP problem, exposing a flag through MCP | `.claude/skills/aid-release-mcp/SKILL.md` |

The index of skills is `.claude/skills/README.md`.

## 7. Keeping this true

A change that makes a statement here or in a skill false fixes that statement in
the same commit. SKILL CHECK (`sh scripts/check-skill-anchors.sh`) runs in CI and
verifies every path and symbol named in prose and enforces the 150-line limit of
this file.

## 8. Pre-commit checklist

1. `git status`: nothing unexpected is modified or untracked.
2. `git diff`: read your own change.
3. No temporary, debug, generated or binary files staged (`aid`, `build/`, `.aid/`, `*.log`).
4. Snapshot changes were reviewed line by line, not just accepted.
5. The full suite is green.
6. `sh scripts/check-skill-anchors.sh` passes.
