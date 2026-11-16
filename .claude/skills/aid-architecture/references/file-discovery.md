# File discovery

Which files a run reads. Every example below was run on a scratch project with
`aid <dir> --stdout --summary-type=off </dev/null`; the output lists the
`<file path="...">` blocks it printed.

## A file path skips all of it

`aid path/to/file` hands the file straight to
`internal/processor/processor.go#Processor.ProcessFile`. No rule on this page
applies: `aid src/a.py --exclude '*.py'` prints `a.py`, `aid build/c.py` and
`aid src/x.aid.py` print their file. A file whose extension has no processor
fails with `no processor found for file` (with `--raw` it is printed as text).

## Directory walk: order of the rules

`internal/processor/concurrent.go#Processor.processDirectoryConcurrent` walks
the directory with `filepath.Walk` (lexical order) and tests each entry in this
order; the first rule that skips it wins.

A directory:

1. `.aidignore` ignores it: skipped with everything below it.
2. Its name is `.aid`: skipped.
3. Its name is in the default list (below): skipped, unless `.aidignore`
   re-includes it: a `!` pattern matches the directory, or a `!` pattern with
   a `/` contains `<name>/` (`internal/ignore/ignore.go#IgnoreMatcher.MightContainExplicitIncludes`).
4. `--recursive=0` and it is not the walk root: skipped.

A file:

1. `.aidignore` ignores it.
2. With `AID_CONFINE_ROOT` set (below), its real path lies outside that
   directory: skipped, and `-v` logs `Skipping <path>: it does not resolve to
   a file inside AID_CONFINE_ROOT`. Unset, a symlink is read wherever it
   points, as in v1.5.0.
3. Its base name contains `.aid.` (for example `x.aid.py`).
4. It fails `--include`/`--exclude` (below).
5. No processor handles its extension (`internal/processor/registry.go#GetByFilename`),
   and `.aidignore` does not re-include it with a `!` pattern. With `--raw`
   this step is skipped and every remaining file is read as text
   (`modes.md`).

The surviving files go to a pool of `--workers` goroutines. The results are
sorted back into walk order, so the output order does not depend on the worker
count (`internal/processor/walk_test.go#TestWorkerCountDoesNotChangeFilesOrOrder`).
A file that fails prints `Warning: failed to process <path>: <error>` to
stderr and is left out; the run goes on.

The name rules (2 and 3 for directories) also apply to the walk root. `aid
build` and `cd build && aid .` print nothing, because the root's base name is
`build`. The names of the directories above the walk root never matter.

Hidden directories and files are walked like any other unless the default list
names them: `.hidden/h.py` and `src/.dot.py` are printed.

## Default-ignored directories

`internal/processor/processor.go#isDefaultIgnoredDir` compares the base name
exactly with: `node_modules`, `vendor`, `target`, `build`, `dist`, `.gradle`,
`gradle`, `__pycache__`, `.pytest_cache`, `venv`, `.venv`, `env`, `.env`,
`Pods`, `.bundle`, `bin`, `obj`, `.vs`, `.idea`, `.vscode`, `coverage`,
`.nyc_output`, `bower_components`, `.terraform`, `.git`, `.svn`, `.hg`.

To add one (say `.turbo`):

1. add it to the list in `isDefaultIgnoredDir`;
2. add a row to `internal/processor/aidignore_test.go#TestAidignoreDefaultIgnoredDirs`;
3. update the list in `README.md` (the `.aidignore` section names the defaults);
4. if the AI flow actions should skip it too, add it to `skipDirs` in
   `internal/aiactions/source_files.go#collectSourceFiles`, which has its own
   list (`modes.md`).

A project that only wants to skip it for itself writes `.turbo/` into its
`.aidignore` instead.

## `.aidignore`

`internal/ignore/ignore.go#New` loads `.aidignore` from the walk root, the
directory given on the command line. A `.aidignore` above it is not read: with
`sub/` in the project root's `.aidignore`, `aid src` still prints `sub/b.py`.

A `.aidignore` in a subdirectory is consulted for the entries directly inside
that subdirectory (`internal/ignore/ignore.go#IgnoreMatcher.ShouldIgnore`);
when one of its patterns matches, its decision replaces the root's. It does
not reach deeper: with `*.py` in `src/.aidignore`, `aid .` drops `src/a.py`
and still prints `src/sub/b.py`. A subdirectory it ignores is skipped with
everything below.

Syntax: empty lines and lines starting with `#` are skipped, `!` negates, a
trailing `/` makes a directory pattern. Patterns are matched against the path
relative to the directory of the `.aidignore`, with `filepath.Match` globs
(`*` does not cross `/`):

| Pattern | Matches | Example in the root `.aidignore` of `proj/` |
|---|---|---|
| `name` (no `/`) | the base name at any depth; also an entry whose relative path is `name` or starts with `name/` | `a.py` drops `src/a.py`; `src` drops all of `src/` |
| `dir/` | that directory and everything below, only at this position (a leading `/` changes nothing) | `src/sub/` drops `src/sub/b.py`; `sub/` drops nothing |
| `/path` | the relative path or one of its directories, anchored at the `.aidignore` | `/a.py` drops nothing (no `a.py` at the top) |
| `**/path` | the relative path or any trailing part of it | `**/b.py` drops `src/sub/b.py` |
| `a/b` | the relative path, any trailing part of it, or a directory of either | `sub/*.py` drops `src/sub/b.py` |

When several patterns match, the last one wins: `*.py` followed by
`!src/a.py` prints only `src/a.py`.

A `!` pattern also:

- re-includes a default-ignored directory: `!build/` prints `build/c.py`;
- makes a file without a processor readable as text: `!*.txt` prints
  `src/notes.txt` verbatim;
- names a file inside a default-ignored directory: `!node_modules/pkg/d.py`
  makes the walk enter `node_modules`, and then every supported file in it is
  printed, `node_modules/pkg/other.py` too.

## `--include` and `--exclude`

Both take comma-separated patterns and may repeat. The walk tests each file
with `internal/processor/processor.go#matchPatterns`, given `relPath` (the
path below the walk root) and `fullPath` (the absolute path). Exclude is
tested first and wins. `internal/processor/processor.go#MatchesPatterns` is
the same matcher for a caller with one path, which it passes as both; the AI
flow actions call it with the path relative to the analysed directory, so
there a slash pattern cannot name directories above it.

| Pattern | `--exclude` matches | `--include` matches |
|---|---|---|
| no `/` | any segment of `relPath`, directories included | the base name only |
| with `/`, relative | any trailing part of `fullPath` that starts at a `/`, or of one of its directories below the walk root | the same |
| starting with `/` or `**` | the whole of `fullPath` (or of such a directory) | the same |

A directory that matches covers every file below it. `**` matches across
`/`, `**/` matches zero or more directories, `*` stays within a segment;
patterns without `**` use `filepath.Match`, and `dir/*` matches the files
directly in `dir`.

Examples on `proj/` with `lib/e.py`, `src/a.py`, `src/sub/b.py`:

| Flags | Printed |
|---|---|
| `--include 'src/*'` | `src/a.py`, `src/sub/b.py` (the directory `src/sub` is directly in `src`) |
| `--include 'sub/*'` | `src/sub/b.py` |
| `--include sub` | nothing: a slash-less include matches base names |
| `--include 'b*'` | `src/sub/b.py` |
| `--exclude sub` | `lib/e.py`, `src/a.py` |
| `--exclude 's*'` | `lib/e.py`: the segment `src` matches |
| `--exclude proj` | all three: a slash-less pattern never matches the walk root's name |
| `--include '/src/*'` | nothing: an absolute pattern must match the absolute path |
| `--include 'proj/src/*'` | `src/a.py`, `src/sub/b.py`: a slash pattern may name directories above the walk root |
| `--exclude '*.py' --include a.py` | nothing: exclude wins |
| `--include '*.txt'` | nothing: `notes.txt` passes the pattern but has no processor |

Tests: `internal/processor/exclude_test.go`,
`internal/processor/walk_test.go#TestPatternsIgnoreDirectoriesAboveTheWalkRoot`,
`internal/ignore/ignore_test.go`, `internal/processor/aidignore_test.go`.

## AID_CONFINE_ROOT

The MCP server starts every `aid` run with `AID_CONFINE_ROOT` set to the real
path of its `AID_ROOT`; unset or empty, the variable changes nothing. The name
is `internal/project/root.go#EnvConfineRoot`, and
`internal/project/confine.go#ConfineRoot` is the one place that reads it.
When it is set, aid reads no file whose real path (symlinks resolved) lies
outside it (`internal/project/confine.go#InConfineRoot`):

- a value that is not an existing directory, and a path argument whose real
  path lies outside it, end the run with exit 1 before anything is read
  (`internal/cli/root.go#checkConfineRoot`);
- the walk skips such a file (file rule 2 above), `.aidignore` files outside
  it are not loaded (`internal/ignore/ignore.go#New`), and the AI flow
  actions' own walk (`internal/aiactions/source_files.go#collectSourceFiles`)
  leaves them out;
- `--dependency-aware` stops at it (`modes.md`);
- the project root, where `.aid/` goes, is searched for no higher than it,
  `AID_PROJECT_ROOT` counts only inside it, and the fallback is the confine
  root instead of the working directory
  (`internal/project/root.go#findRootUncached`).

Tests: `internal/conformance/confine_test.go`.
