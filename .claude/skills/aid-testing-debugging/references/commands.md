# Test commands

Run from the repository root after `export CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"`. Why every
command has `-count=1` and why the `-run` patterns are anchored with `^...$`:
SKILL.md section 2.

## Full suite

```bash
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')
```

## Snapshot suite (`internal/testrunner`)

```bash
# all languages
go test -count=1 ./internal/testrunner
# one language
go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner
# one scenario
go test -count=1 -run 'TestIntegration/^python$/^01_basic$/' ./internal/testrunner
# one expected file: escape the dot; '=' and ',' need no escaping
go test -count=1 -run 'TestIntegration/^python$/^01_basic$/^default\.txt$' ./internal/testrunner
go test -count=1 -run 'TestIntegration/^python$/^01_basic$/^private=1,protected=1,internal=1,implementation=0\.txt$' ./internal/testrunner
```

The package also holds unit tests of the runner itself (discovery, file-name
parsing, `BuildBinary`); a `-run 'TestIntegration/...'` pattern skips them.

A failing snapshot prints the expected file, the command that reproduces the run
(relative to `testdata/`, with `aid` standing for a binary you build) and a unified
diff of expected against actual (`internal/testrunner/runner.go#formatMismatch`).

## Updating snapshots

Update mode is on only when `UPDATE_EXPECTED` is exactly `true`
(`internal/testrunner/runner.go#New`). It overwrites every selected snapshot and
passes, so the review is the diff.

```bash
# one language, the normal case
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner
# one scenario
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/^01_basic$/' ./internal/testrunner
# one expected file
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/^01_basic$/^default\.txt$' ./internal/testrunner

# review: which files changed, then every changed line
git diff --stat testdata/python
git diff testdata/python
```

After updating Go or Java snapshots, run the unit tests that read
`testdata/go/01_basic` and `testdata/java/01_basic`:

```bash
go test -count=1 ./internal/language/golang ./internal/language/java
```

## Audit of the testdata layout

```bash
go run ./cmd/test-audit
```

It prints `No issues found!` and exits 0, or lists the problems and exits 1
(`internal/testrunner/audit.go#Runner.Audit`). It checks scenario directory names
(no dots, no file extension), duplicate scenario names within a language, that each
scenario has a source file and an `expected/` directory with at least one `.txt`
file, and that the expected-file names parse. It does not check that the five
standard variants exist or that a snapshot is non-empty.

## Conformance suite (`internal/conformance`)

```bash
# the whole package: capability checks, CLI contract tests, unhandled node kinds
go test -count=1 ./internal/conformance
# capability checks of one language
go test -count=1 -run 'TestConformance/^python$/' ./internal/conformance
# one check
go test -count=1 -run 'TestConformance/^python$/^body-indent$' ./internal/conformance
# show why a known gap still fails (t.Logf output needs -v)
go test -count=1 -v -run 'TestConformance/^php$/' ./internal/conformance | grep 'known gap'
# node kinds a text formatter drops (knownUnhandled)
go test -count=1 -run 'TestNoUnhandledNodesInTestdata' ./internal/conformance
```

## One unit-test package

```bash
go test -count=1 ./internal/stripper
go test -count=1 -run 'TestExpandSymbols' ./internal/stripper
```

## MCP package tests (`mcp-npm/test`)

```bash
(cd mcp-npm && npm test)
```

`npm test` runs `npm run build && node --test`: it compiles the TypeScript into
the dist directory of mcp-npm (the `outDir` of `mcp-npm/tsconfig.json`,
ignored by git) and runs `mcp-npm/test/*.test.js` with node's built-in runner.
Run `npm ci` in `mcp-npm/` first when `node_modules/` is missing.
