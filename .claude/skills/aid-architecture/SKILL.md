---
name: aid-architecture
description: >-
  Map of AI Distiller's language-agnostic core: the CLI and its modes (stdin,
  git, raw, AI actions, dependency-aware), how flags become ProcessOptions and
  stripper options, file discovery (.aidignore, include/exclude), the IR node
  types and the stripper, output file naming, and which packages are live. Use
  when you need to find where something is implemented, when adding or
  changing a CLI flag or a mode, changing filtering semantics or file
  selection, touching internal/ir or internal/stripper, or when aid behaves
  unexpectedly and the cause is not language-specific. Also for "kde se
  řeší", "jak funguje", "přidej přepínač".
---

# AI Distiller architecture

The language-agnostic core: what happens between the command line and the
printed output. How one language is parsed is in the `aid-language-parsers`
skill (`.claude/skills/aid-language-parsers/SKILL.md`); running and writing
tests is in `.claude/skills/aid-testing-debugging/SKILL.md`. Run commands from
the repository root with `CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"`.

References, loaded when needed:

- `references/cli-flags.md`: all 42 flags with their real default and the
  option each sets; the four ways flags become options; output file names;
  the checklist for adding a flag.
- `references/modes.md`: stdin, git mode, raw mode, AI actions (and how to add
  one), dependency-aware.
- `references/file-discovery.md`: which files a directory run reads, with
  examples: `.aidignore`, default-ignored directories, `--include`/`--exclude`.
- `references/ir-and-stripper.md`: what the stripper removes per option,
  `--expand`, the comment rules.
- `references/code-map.md`: the packages `cmd/aid` is built from and what
  else is in `internal/` and `cmd/`.
- `.claude/skills/aid-architecture/scripts/live-map.sh`: the API of one
  directory, printed by aid itself (section 7).

## 1. Data flow

A path run in the normal mode, step by step:

- `cmd/aid/main.go#main` calls `internal/cli/root.go#Execute`. Cobra parses
  the flags and runs the PreRun (set in `internal/cli/root.go#initFlags`,
  wrapped by `internal/cli/help.go#initializeHelpSystem`), which handles
  `--version`, `--help-extended` and `--cheat` and reads the 0/1 flags.
- `internal/cli/root.go#runDistiller` picks the mode (section 3). In the
  normal mode:
  1. `internal/cli/root.go#createProcessOptionsFromFlags` turns the flags
     into `internal/processor/interface.go#ProcessOptions` (section 4).
  2. `internal/processor/processor.go#Processor.ProcessPath`: a directory
     goes to `internal/processor/concurrent.go#Processor.processDirectoryConcurrent`,
     which walks it (section 5) and runs a worker pool; a file goes straight
     to the next step. With `--dependency-aware`,
     `internal/processor/dependency.go#Processor.ProcessWithDependencies`
     runs instead: the same steps for the input, then the same for every
     project file it imports (`references/modes.md`).
  3. `internal/processor/processor.go#Processor.ProcessFile` picks the
     language processor by extension
     (`internal/processor/registry.go#GetByFilename`) and calls its
     `ProcessWithOptions`. The processor parses into an `ir.DistilledFile`
     and runs `internal/stripper/stripper.go#Stripper` with
     `internal/processor/interface.go#ProcessOptions.ToStripperOptions`
     (section 6).
  4. `internal/formatter/registry.go#Get` returns the formatter for
     `--format`; it renders one file (`Format`) or a directory
     (`FormatMultiple`). The text formatter passes each file's text through
     `internal/importfilter/filter.go#FilterImports` unless
     `--filter-imports=false`.
  5. For `text`, `runDistiller` deletes `<file>` blocks that hold only
     whitespace, then writes the output file, prints, or both (section 3).
  6. `internal/summary/summary.go#Print` writes the summary to stderr unless
     `--summary-type=off`.

## 2. Where do I change X

| Symptom or intent | Where | Read |
|---|---|---|
| A construct is missing or wrong in one language's IR; wrong visibility | `internal/language/<lang>/` | `aid-language-parsers` |
| The IR is right but the printed text is wrong | `internal/formatter/<lang>_formatter.go` | `aid-output-formatting` |
| An import vanished from the text output | `internal/importfilter` (text only) | `aid-output-formatting` `references/import-filter.md` |
| Add or change a flag | `internal/cli/root.go` | `references/cli-flags.md` |
| A flag combination selects the wrong members | `internal/cli/root.go#createProcessOptionsFromFlags` | section 4, `references/cli-flags.md` |
| What a flag removes (bodies, comments, docs, members) | `internal/stripper/stripper.go` | `references/ir-and-stripper.md` |
| A file or directory is read or skipped unexpectedly | `internal/processor/concurrent.go`, `internal/ignore` | `references/file-discovery.md` |
| Skip another directory by default | `internal/processor/processor.go#isDefaultIgnoredDir` | `references/file-discovery.md` |
| `--include`/`--exclude` semantics | `internal/processor/processor.go#MatchesPatterns` | `references/file-discovery.md` |
| A new IR field or node type | `internal/ir`, then stripper and formatters | `aid-language-parsers` `references/ir-conventions.md`, `references/ir-and-stripper.md` |
| Output file name or location | `internal/cli/root.go#generateOutputFilename`, `internal/project` | `CLAUDE.md` section 4, `references/cli-flags.md` |
| Stdin, its language or TSX | `internal/cli/root.go#processStdinWithContext`, `internal/language/detector.go` | `references/modes.md` |
| Git log output | `internal/cli/root.go#handleGitMode` | `references/modes.md` |
| An AI action or its prompt text | `internal/aiactions`, `internal/aiactions/templates` | `references/modes.md` |
| `--dependency-aware` | `internal/processor/dependency.go` | `references/modes.md` |
| The summary line | `internal/summary` | `aid-output-formatting` `references/summary.md` |
| A test fails, `-v` output, snapshots | | `aid-testing-debugging` |

## 3. Modes

Before `runDistiller`: cobra prints the help for `--help`; the PreRun prints
and exits 0 for `--version`, `--help-extended` and `--cheat`, and exits 1 for
a bad value of one of the nine 0/1 flags it parses (`references/cli-flags.md`)
or for `--include-only` together with `--exclude-items`. `aid help [actions|filtering|git]` is a cobra
subcommand.

`runDistiller` then decides in this order; the first match wins:

1. **Stdin**: the argument is `-`, or there is no argument and stdin is not a
   terminal (the rule: `CLAUDE.md` section 4).
   `internal/cli/root.go#processStdinWithContext` runs the language processor
   on the input. It skips the walk, `--dependency-aware`, the summary and
   the default output file.
2. **Help**: no argument (stdin is a terminal): cobra's help.
3. The path must exist, else `path does not exist`.
4. **Git**: the path's base name is `.git`:
   `internal/cli/root.go#handleGitMode` prints `git log`. It skips every
   filter flag, `--format` and the summary.
5. **AI action**: `--ai-action` is set (or the deprecated
   `--ai-analysis-task-list`); `-o` is an error here.
   `internal/cli/root.go#handleAIAction` runs a prompt action (distils as
   `text`, puts a template in front) or a flow action (lists the files, writes
   task-list files). It skips `--format`, the default output file and the summary.
6. **Normal**: section 1. `--raw` is not a mode of its own: it is the normal
   mode with every file read as text.

Details of each: `references/modes.md`. Where the output goes:

| Run | Prints the result | Writes |
|---|---|---|
| `aid PATH` | no (the summary on stderr names the file) | `.aid/aid.<name>[.<options>].<ext>` in the project root (`references/cli-flags.md`, "Output file name") |
| `aid PATH --stdout` | yes | nothing |
| `aid PATH -o FILE` | no | FILE |
| `aid PATH -o FILE --stdout` | yes | FILE |
| stdin | yes | FILE with `-o` |
| `aid .git` | yes, except with `-o` and no `--stdout` | FILE with `-o` |
| prompt AI action | the prompt with `--stdout`, else a status block | `--ai-output`, default in the project root's `.aid/` |
| flow AI action | status lines | files under `--ai-output`, default `.aid/` of the working directory |

So `aid src/` printing nothing is the normal mode working as designed: the
result is in `.aid/aid.src.txt` under the project root, and the summary on
stderr names it. Add `--stdout` to print it. If that file is empty as well,
file discovery skipped every file (section 5).

## 4. Flags to options

`createProcessOptionsFromFlags` has four return paths: `--strip`
(deprecated), `--include-only`, `--exclude-items`, and the individual flags.
The group flags ignore the individual flags; a field a path does not set
keeps its zero value (`references/cli-flags.md`).

With the individual flags, visibility takes one of two branches:

- **Public only** (`--public=1`, the other three `0`; the default):
  `IncludePrivate` is false, which becomes the stripper's `RemovePrivate`. It
  removes every non-public visibility at once, plus declarations without
  visibility whose name starts with `_`.
- **Any other combination**: `IncludePrivate` follows `--private`, and
  `RemovePrivateOnly`, `RemoveProtectedOnly`, `RemoveInternalOnly` are set
  for each level that is `0`. The stripper then decides per level and ignores
  `RemovePrivate`. With all four at `1` nothing is removed by visibility.

In both branches `RemovePublic` is `!--public`. It removes public functions,
fields and type aliases, except inside a non-public type, and then public
types and namespaces left without members (`--public=0 --private=1` prints a
public class holding only its private members).

`ProcessOptions.ToStripperOptions` negates `IncludePrivate`,
`IncludeImplementation`, `IncludeComments`, `IncludeImports`,
`IncludeDocstrings`, `IncludeAnnotations`, `IncludeFields` and
`IncludeMethods` into the matching `Remove*`, and copies `RemovePublic`, the
three `Remove*Only` and `ExpandSymbols`. `IncludePublic`, `IncludeProtected`
and `IncludeInternal` reach only the formatter options. Which visibility value counts as which level:
`aid-language-parsers` `references/ir-conventions.md`, section "Visibility".

## 5. File discovery

Only a directory run selects files; a file path is processed whatever the
rules say. The walk tests each entry in this order (details and examples:
`references/file-discovery.md`):

1. `.aidignore` from the walk root (and from subdirectories, for the entries
   directly in them); `!` re-includes.
2. Directories named `.aid` are skipped; files with `.aid.` in the name too.
3. Default-ignored directory names
   (`internal/processor/processor.go#isDefaultIgnoredDir`: `node_modules`,
   `vendor`, `build`, `dist`, `bin`, `.git`, ...), the walk root included,
   unless `.aidignore` re-includes them.
4. `--recursive=0` skips subdirectories.
5. `--include`/`--exclude` (`internal/processor/processor.go#MatchesPatterns`):
   a slash-less exclude matches any path segment below the walk root, a
   slash-less include only the base name; a pattern with `/` matches a
   trailing part of the absolute path; a matching directory covers everything
   below it; exclude wins.
6. Files without a processor for their extension are skipped (not with
   `--raw`).

The output keeps the walk's lexical order for any worker count. With
`AID_CONFINE_ROOT` set (the MCP server sets it), a file whose real path lies
outside that directory is skipped as well, a path argument outside it is an
error, and the project root for `.aid/` stays inside it
(`references/file-discovery.md`, "AID_CONFINE_ROOT").

## 6. Stripper

`internal/stripper/stripper.go#Stripper` copies each node it keeps and changes
only what an option names; it returns `nil` to remove a node. Per option
(details: `references/ir-and-stripper.md`):

- visibility as in section 4;
- `--implementation=0` clears function bodies, except for functions whose
  name matches `--expand` or that lie in a class or struct whose name
  matches;
- `--comments=0` removes non-doc comments, `--docstrings=0` doc comments,
  `--imports=0` imports, `--annotations=0` decorators, `--fields=0` every
  field (enum constants too), `--methods=0` every function (top-level ones
  too).

The doc-comment rule: comments directly above a removed declaration are
removed with it, and a run of doc comments is kept only when the declaration
after it survives. Without that, the doc of a hidden private method would
appear above the next public one.

## 7. Live map

```bash
sh .claude/skills/aid-architecture/scripts/live-map.sh internal/stripper
```

Builds aid from the working tree into a temp directory and runs
`aid <dir> --stdout --summary-type=off --exclude '*_test.go' --internal=1 --private=1`:
every type, function and method in the directory and its subdirectories,
with signatures and doc comments, unexported Go names included, without test
files and without bodies. Use it before reading a package's sources; the map is produced by the
code, so it is never out of date, unlike the lists in this skill.
