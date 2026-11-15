# Snapshot test suite

`TestIntegration` runs the real `aid` binary on every fixture under `testdata/`
and compares its stdout byte for byte with a stored snapshot. The suite builds
its own binary (`testrunner.BuildBinary`), so nothing has to be built first.

## Layout

```
testdata/<lang>/<NN_name>/source.<ext>
testdata/<lang>/<NN_name>/expected/<flags>.txt
```

Discovery is strict: a scenario directory without a source file, without an
`expected/` directory, or with no `expected/*.txt` makes the suite fail.

## File name to flags

The file name (without `.txt`) is a comma-separated list of `key=value` pairs.
Each pair becomes `--key=value`. `default.txt` means no flags. There are no
other aliases. A value that contains a comma cannot be encoded.

Examples: `implementation=1.txt` runs `--implementation=1`;
`private=1,protected=1,internal=1,implementation=0.txt` runs those four flags.

## The five standard variants

Every scenario carries these snapshots:

- `default.txt`
- `implementation=1.txt`
- `private=1,protected=1,internal=1,implementation=0.txt`
- `comments=1.txt`
- `imports=0.txt`

(`private=0`, `protected=0` and `private=0,protected=0,internal=0` are not
kept: they are identical to the default.)

## Running

Run commands from the repository root with cgo enabled:

```bash
export CGO_ENABLED=1 CGO_CFLAGS=-w
go test -count=1 ./internal/testrunner                                   # everything
go test -count=1 -run 'TestIntegration/python/' ./internal/testrunner    # one language
go test -count=1 -run 'TestIntegration/python/01_basic/' ./internal/testrunner  # one scenario
go test -count=1 -run 'TestIntegration/python/01_basic/default.txt' ./internal/testrunner  # one file
```

A failure prints the snapshot path, the command to reproduce it and a diff.

## Updating snapshots

```bash
UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/python/' ./internal/testrunner
```

Regenerate one language at a time, then read `git diff testdata/<lang>` and
confirm every changed line is intended.

## Adding a scenario

1. Create `testdata/<lang>/<NN_name>/source.<ext>`.
2. Create empty files in `expected/` for the five standard variants.
3. Run the update command for that scenario, e.g.
   `UPDATE_EXPECTED=true go test -count=1 -run 'TestIntegration/<lang>/<NN_name>/' ./internal/testrunner`.
4. Review the generated snapshots by hand before committing.
