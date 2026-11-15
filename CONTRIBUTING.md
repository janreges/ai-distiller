# Contributing to AI Distiller

## Prerequisites

- Go 1.25 (see `go.mod`)
- A C compiler (the tree-sitter grammars are C code)
- The grammar sources: `git submodule update --init --recursive`

cgo is always on. Every build and test command needs
`CGO_ENABLED=1 CGO_CFLAGS=-w`; the project does not compile with `CGO_ENABLED=0`.

## Build and test

Run from the repository root.

```bash
export CGO_ENABLED=1 CGO_CFLAGS=-w
make build                                   # build/aid
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')   # full suite
go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner                     # one language's snapshots
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner  # update them, then review git diff testdata
sh scripts/check-skill-anchors.sh            # checks paths and symbols named in the skills and CLAUDE.md
```

## Snapshot changes

Snapshots under `testdata/` are the expected output. After updating them, read
`git diff testdata` line by line: a snapshot that is accepted without review
hides a regression. No mocks or stubs in tests: they run the real parser,
formatter and binary.

## Before you commit

1. `git status` and `git diff`: nothing unexpected, no binaries (`aid`, `build/`) and no `.aid/` output.
2. The full suite is green.
3. `sh scripts/check-skill-anchors.sh` passes.

Code, comments, commit messages and docs are in English. Use conventional
commit messages (`feat:`, `fix:`, `docs:`).

## Skills and project guide

[`CLAUDE.md`](CLAUDE.md) is the short project guide. It routes to the skills in
[`.claude/skills/`](.claude/skills/README.md): testing and debugging, language
parsers, architecture, output formatting, and release and MCP. They describe how
the code works and are checked in CI, so read the matching one before changing
that area.

## Releases

Maintainers release by pushing a `vX.Y.Z` tag, which runs
[`.github/workflows/release.yml`](.github/workflows/release.yml). The procedure
is in [`.claude/skills/aid-release-mcp/SKILL.md`](.claude/skills/aid-release-mcp/SKILL.md).
