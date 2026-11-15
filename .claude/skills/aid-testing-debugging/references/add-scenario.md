# Adding a snapshot scenario

A scenario is one source file and its snapshots. It proves only that the output
stays the same (SKILL.md section 1); to prove a construct is handled correctly, add
a conformance check or a unit test as well.

## 1. Directory and source file

```
testdata/<lang>/<NN>_<name>/source.<ext>
testdata/<lang>/<NN>_<name>/expected/
```

- `<lang>` is one of the language directories in `testdata/` (`cpp`, `csharp`, `go`,
  `java`, `javascript`, `kotlin`, `php`, `python`, `ruby`, `rust`, `swift`,
  `typescript`); the runner accepts no other names
  (`internal/testrunner/runner.go#findSourceFile`).
- `<NN>_<name>`: the next free number in that language and a short snake_case
  name, as in `05_very_complex` or `06_edge_cases`. No dots and no file extension
  in the name: `go run ./cmd/test-audit` reports both.
- Name the file `source.<ext>`. The runner would also take another file with the
  right extension, but `TestNoUnhandledNodesInTestdata` only reads
  `testdata/*/*/source.*`, so any other name skips that check.
- Keep the file to the constructs the scenario is about, so a later diff in its
  snapshots points at one feature.

## 2. Empty expected files for the five standard variants

```bash
d=testdata/python/06_example
mkdir -p "$d/expected"
for v in default implementation=1 'private=1,protected=1,internal=1,implementation=0' comments=1 imports=0; do
  : > "$d/expected/$v.txt"
done
```

Every scenario carries these five; a further variant is any other flag list as a
file name (`expand=get_status.txt`, see SKILL.md section 3). The update run writes
only expected files that already exist: without an `expected/` directory it creates
only `default.txt`, and an `expected/` directory with no `.txt` file fails discovery.

## 3. Generate the snapshots for this scenario only

```bash
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/^python$/^06_example$/' ./internal/testrunner
```

## 4. Review by hand

```bash
wc -c testdata/python/06_example/expected/*.txt
cat testdata/python/06_example/expected/*.txt
```

Read every snapshot against the source. Each one is now the definition of correct
output for its flags, so a wrong line here becomes a test that protects the bug.
Check at least that public declarations appear in `default.txt`, private ones only
in the `private=1,...` file, bodies only in `implementation=1.txt`, and that
`imports=0.txt` has no imports. If a snapshot is wrong, do not commit it: find the
layer (SKILL.md section 5) and fix the code, or leave the construct out of the
scenario and add a conformance check with a `knownGaps` entry.

An empty snapshot is legitimate only when aid prints nothing for those flags (a
file run drops an empty `<file>` block); confirm it by running aid on the file.

## 5. Audit and test

```bash
go run ./cmd/test-audit
go test -count=1 -run 'TestIntegration/^python$/^06_example$/' ./internal/testrunner
go test -count=1 ./internal/conformance
```

The audit checks the layout only: it does not notice a missing variant or an
empty snapshot (`references/commands.md`). The conformance package matters here
because `TestNoUnhandledNodesInTestdata` now also formats the new source and fails
if a text formatter drops a node kind in it.
