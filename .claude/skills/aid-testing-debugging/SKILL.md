---
name: aid-testing-debugging
description: >-
  How to test, debug and verify changes in the AI Distiller repo: the snapshot
  suite in testdata/, the expected-file naming convention, updating snapshots,
  the conformance suite and its known-gaps list, the -v/-vv/-vvv debug levels
  and the bug-to-fix workflow. Use whenever a change touches a parser, the
  stripper, a formatter or CLI flags, whenever tests fail or pass suspiciously,
  when adding a test scenario, updating snapshots, reproducing a GitHub issue,
  or before claiming that something works. Also for "přegeneruj snapshoty",
  "proč neprochází testy", "přidej testovací scénář", "ověř, že to funguje".
---

# Testing and debugging AI Distiller

Run every command from the repository root. Every Go build and test needs
`CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"`: the tree-sitter parsers are C code linked through
cgo, and with `CGO_ENABLED=0` the project does not compile (`go build ./cmd/aid`
stops with "build constraints exclude all Go files" in the tree-sitter grammar
packages). `-w` only silences C compiler warnings.

References, loaded when needed:

- `references/commands.md`: every test command as a copy-paste block.
- `references/add-scenario.md`: adding a snapshot scenario step by step.
- `references/test-inventory.md`: each test file group and what it asserts.
- `references/ci.md`: what the CI and release workflows run.
- `.claude/skills/aid-testing-debugging/scripts/triage.sh`: three views of one input
  (section 5).

## 1. Three test layers and what each proves

| Layer | Where | What a pass proves |
|---|---|---|
| Unit tests | `*_test.go` beside the code | one function or package does what its test states, usually on inline source or a hand-built IR |
| Snapshot suite | `internal/testrunner`, fixtures in `testdata/` | the text output of every scenario is byte for byte what it was when its snapshot was written |
| Conformance suite | `internal/conformance` | a named capability (private members hidden, bodies controlled by `--implementation`, `--expand`, stdin detection, ...) works in a language; also the CLI contract tests |

A green snapshot suite does not mean a feature works. Snapshots record whatever
aid printed when they were generated, wrong output included, so they only prove
that nothing changed since. To show that something works, write a conformance
check or a unit test that states the expected behaviour.

The snapshot suite (`internal/testrunner/integration_test.go#TestIntegration`) and
the conformance suite build their own `aid` into a temp directory
(`internal/testrunner/build.go#BuildBinary`), so nothing has to be built first.
If `AID_TEST_BINARY` names an existing file, they run that binary instead. CI
sets it; locally it means a source edit is not tested until that binary is rebuilt.

## 2. Commands

```bash
export CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"

# full suite, the command CI runs
go test -count=1 $(go list -e ./... | grep -vE '/tools/|/test_project/|/cmd/aid-validate')

# snapshot suite: one language, one scenario, one expected file
go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner
go test -count=1 -run 'TestIntegration/^python$/^01_basic$/' ./internal/testrunner
go test -count=1 -run 'TestIntegration/^python$/^01_basic$/^default\.txt$' ./internal/testrunner

# conformance checks of one language
go test -count=1 -run 'TestConformance/^python$/' ./internal/conformance
```

- `-count=1` turns off Go's test cache. The cache replays a pass while the test
  package and the files it reads are unchanged, and the two suites run a binary
  they build themselves, so the cache cannot see edits to the code inside that
  binary: after an edit to `internal/formatter/python_formatter.go` a repeated
  snapshot run still printed `ok ... (cached)`.
- The `^...$` anchors are needed because each `/`-separated part of `-run` is an
  unanchored regular expression: `TestIntegration/java/` also runs `javascript`.
- The `grep -vE` drops directories that exist only in local checkouts and are
  ignored by git (an aid-validate command under cmd/ does not compile).

More forms (one check, update mode, audit, MCP tests) are in `references/commands.md`.

## 3. Snapshots

Layout: `testdata/<lang>/<scenario>/source.<ext>` and
`testdata/<lang>/<scenario>/expected/<flags>.txt`. The file name is the flag list:
`default.txt` means no flags; any other name is comma-separated `key=value` pairs,
each run as `--key=value` (`internal/testrunner/runner.go#parseParametersFromFilename`).
Examples: `implementation=1.txt`, `private=1,protected=1,internal=1,implementation=0.txt`,
`expand=get_status.txt`. Any other name fails discovery
(`internal/testrunner/runner.go#validateExpectedName`) instead of silently running
with fewer flags.

The runner runs `aid <lang>/<scenario>/source.<ext> <flags> --format text --stdout
--summary-type=off` inside `testdata/` (`internal/testrunner/runner.go#Runner.RunTest`).
So a snapshot cannot encode:

- another output format: the `--format text` after the encoded flags wins;
- a value containing a comma (the separator) or a `/` (not valid in a file name);
- any input other than the scenario's one source file: no directory, no stdin.

Use a conformance check or a unit test for those.

### Updating

```bash
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/' ./internal/testrunner
git diff --stat testdata/python
git diff testdata/python
```

In update mode every selected snapshot is overwritten with the current output and
the test passes, so the run itself tells you nothing; the review is the diff. Read
all of it and, for each changed line, name the change of yours that caused it and
why the new line is right. A line you cannot explain is a regression about to be
committed.

Update one language, the one your change touches. Updating all languages at once
buries an unintended change in one language under many intended lines in another,
and turns the suite into a record of whatever the code prints now.

`testdata/go/01_basic` and `testdata/java/01_basic` are also read by unit tests
(`internal/language/golang/comprehensive_test.go#TestGoConstructs`,
`internal/language/java/processor_comprehensive_test.go#TestJavaConstructs`), which
format in-process and compare with three of their snapshots. After updating Go or
Java, run those packages too (see `references/commands.md`).

New scenario: `references/add-scenario.md`.

## 4. Conformance and known gaps

`internal/conformance` runs the real binary against one fixture file per language in
`internal/conformance/testdata/<lang>/`. `fixtures`
(`internal/conformance/fixtures_test.go#fixtures`) gives each language its file and
its checks. A `check` (`internal/conformance/fixtures_test.go#check`) has a
capability id, extra flags, and strings the text output must contain, must not
contain, or must contain exactly once. Most checks come from `standard`
(`internal/conformance/fixtures_test.go#standard`), filled with each fixture's
marker names; fixtures mark bodies, comments and docs with strings such as
`CONF_BODY_A`, `CONF_LINE_COMMENT` and `CONF_DOC_COMMENT`. Each check runs as the
subtest `TestConformance/<lang>/<cap>`.

`knownGaps` (`internal/conformance/known_gaps_test.go#knownGaps`) lists the checks
that fail today, keyed `<lang>/<cap>`, each with a reason. It is the live list of
what does not work; read it rather than any list in prose. For a listed key,
`internal/conformance/conformance_test.go#TestConformance` logs the failure and
passes while the check fails, and fails with
`<lang>/<cap> passes now: delete it from knownGaps in known_gaps_test.go` once the
check passes. So a fix that closes a gap fails the suite until its entry is deleted,
in the same commit. That keeps the list from going stale without anyone having to
remember it. When you see "passes now", delete the entry; changing the check so it
fails again would hide a fix. `TestKnownGapsAreLive` fails when a key matches no
check, so renaming or removing a check means renaming or removing its entry too.

Adding a check for a new capability:

1. Put the construct into the language's fixture in
   `internal/conformance/testdata/<lang>/`, with a marker string that occurs
   nowhere else in the file.
2. Add a `check` to that language's entry in `fixtures`. Capability ids are unique
   within a language. A capability every language must have belongs in
   `standard`, which then needs markers in all twelve fixtures.
3. Run `go test -count=1 -run 'TestConformance/^<lang>$/' ./internal/conformance`.
   If the check fails because the feature is missing, fix the feature or add a
   `knownGaps` entry with the reason. A check edited until it passes proves nothing.

`knownUnhandled` (`internal/conformance/unhandled_test.go#knownUnhandled`) is the
same idea for the text formatters. `TestNoUnhandledNodesInTestdata` parses every
`testdata/*/*/source.*` with nothing stripped and formats it as text while
`internal/formatter/unhandled.go#OnUnhandledNode` records each node kind a
formatter dropped without rendering it. A dropped `<lang>/<kind>` that is not
listed fails; a listed pair that is rendered now fails with
`is rendered now: delete it from knownUnhandled`.

## 5. Finding the layer of a bug

```bash
sh .claude/skills/aid-testing-debugging/scripts/triage.sh python testdata/python/01_basic/source.py
printf 'class A:\n    def _p(self):\n        return 1\n' | sh .claude/skills/aid-testing-debugging/scripts/triage.sh python -
```

`triage.sh <lang> <file|->` builds aid from the working tree into a temp directory
and prints three labelled sections:

1. default text output;
2. text output with `--private=1 --protected=1 --internal=1 --implementation=1 --comments=1`;
3. `--format jsonl` with the same flags: one JSON line per IR node, with its kind,
   name, parent path (`path`) and, for declarations, `visibility`.

With every include flag on the stripper has nothing to remove and does not run
(`internal/stripper/stripper.go#Options.HasAnyOption` is false), so section 3 is
the parser's IR and section 2 is that IR through the text formatter.

| What you see | Layer |
|---|---|
| Missing from section 3 | parser, `internal/language/<lang>/` (Go: `internal/language/golang/`) |
| In section 3, missing or wrong in section 2 | the language's text formatter, `internal/formatter/<lang>_formatter.go`; `-v` names node kinds a formatter dropped (section 7) |
| In section 2 but not section 1, or in section 1 though it should be hidden | the stripper (`internal/stripper/stripper.go`) or the visibility the parser assigned: if `visibility` in section 3 is wrong, the parser; if it is right, the stripper |

Two traps. An import present in section 3 but missing from the text may have been
dropped by the unused-import filter of the text format; run aid with
`--filter-imports=false` to tell. And jsonl has every node but not every field
(no decorators, no type parameters, no visibility for structs and enums): for a
missing detail read the full IR with `-vvv` (section 7) before blaming the parser.

## 6. Bug-to-fix workflow

1. Reproduce with the smallest input, through stdin, so the repro is one line you
   can paste into a test or an issue. For a GitHub issue, take its snippet and flags:
   ```bash
   printf 'class A:\n    def _p(self):\n        return 1\n' | go run ./cmd/aid --lang python --private=1
   ```
   aid reads stdin when the argument is `-`, or when there is no argument and stdin
   is not a terminal. Stdin output is labelled `stdin`, and an empty `<file>` block
   is kept, which a file run drops; if a bug shows only on a file, save the snippet
   with the right extension and use the file.
2. Find the layer (section 5).
3. Write the failing test first, at the level that states the behaviour: a
   conformance check for a capability, or a unit test in the layer's package. Run
   it and watch it fail for the reason you expect. A snapshot cannot be that test:
   it would record the bug as expected output.
4. Fix it in that layer, run the focused test until it passes, and delete the
   `knownGaps` or `knownUnhandled` entry if there was one.
5. Update the snapshots of the one language the fix touches and justify every
   changed line (section 3).
6. Run the full suite (section 2).
7. Before committing, read `git status --short`, stage the files you changed by
   name, and read `git diff --cached`. Staging by name keeps out untracked files
   that are not part of the change. If the change renamed or moved code a skill
   names, fix the skill in the same commit and run the SKILL CHECK from
   `.claude/skills/README.md`.

## 7. Debug output

`-v`, `-vv` and `-vvv` write to stderr; stdout stays the distilled output. Lines
look like `[HH:MM:SS.mmm] [subsystem] LEVEL: message`, except the import filter's,
which start with `[<lang> import filter]`. What each level printed for
`testdata/python/01_basic/source.py`:

- `-v`: INFO lines (version, input, output, format, worker count), the import
  filter's progress lines, and `formatter: no text rendering for <kind> in <lang>`
  once per node kind and language a text formatter dropped
  (`internal/cli/root.go#runDistiller` sets the hook).
- `-vv`: adds DEBUG lines: the effective visibility and content flags, which
  processor handled the file, timings for parsing and formatting, the imports the
  filter removed, and the output size.
- `-vvv`: adds DUMP blocks, go-spew dumps of the IR between `DUMP: === <label> ===`
  and `=== End <label> ===`. For a single file every language dumps
  `IR for <file>` (after stripping) and `IR being formatted`. Python and Go also
  dump the parser's result before stripping: Python `Raw tree-sitter AST` and
  `Final Python IR structure`; Go `Raw Go AST`, `Final Go IR structure`,
  `Raw Go IR before stripping` and `Stripped Go IR`. For the other languages, see
  the unstripped IR by adding every include flag, as triage section 3 does.

The IR dumps need a file: stdin input dumps only the Python- and Go-specific blocks,
and a directory dumps `IR for <file>` per file but no `IR being formatted`. A dump
runs to thousands of lines for a small file, so list the blocks first:

```bash
go run ./cmd/aid testdata/python/01_basic/source.py --stdout --summary-type=off -vvv 2>&1 >/dev/null | grep 'DUMP'
```

## 8. What the tests do not cover

Read from `references/test-inventory.md`; when a change lands here, add a test.

- Output formats other than text. The snapshot and conformance suites run
  `--format text` only; `md`, `jsonl`, `json-structured` and `xml` have formatter
  unit tests on hand-built IR and one CLI test that they leak no metadata.
- Directory runs. Snapshots run one file each; walking, `.aidignore`,
  `--include`/`--exclude` and output order have unit tests in `internal/processor`
  and `internal/ignore` only.
- Swift below the CLI is thin. Unit tests cover the line parser's body
  extraction, property defaults, the `struct` modifier, comments, code after a
  block comment and the extension header
  (`internal/language/swift/line_parser_test.go`), and the formatter's `struct`
  and `extension` headers (`internal/formatter/swift_formatter_test.go`);
  everything else is covered only by the snapshots and the conformance checks.
- Git mode (`aid .git`) and the text of AI action prompts. Tests cover the AI
  actions' source collection, `-o` being rejected with `--ai-action`, and that the
  docs list every action; nothing runs git mode.
- `--dependency-aware`: only that it is off by default. `--raw`: only the raw
  processor's text/binary decision.
- Debug output: only that `-v` and `-vvv` set the verbosity count, not what is printed.
- The MCP server end to end. `mcp-npm/test` exercises the runner, the tool texts,
  the file listing and the postinstall helpers with `node` as the child process,
  never the real aid binary.
- Platforms. CI runs the tests on Linux; macOS only builds and runs a smoke test;
  Windows binaries are built for releases but never run (`references/ci.md`).
