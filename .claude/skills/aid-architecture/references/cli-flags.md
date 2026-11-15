# CLI flags

All 42 flags of `aid`, generated from the flag set itself (`rootCmd.Flags().VisitAll`
in a throwaway test, cross-checked with the `grep` below). They are defined in
`internal/cli/root.go#initFlags`, except `--help-extended` and `--cheat`
(`internal/cli/help.go#initializeHelpSystem`). The default is the value in the
code, not the help text. "Sets" names the `internal/processor/interface.go#ProcessOptions`
field, or what else reads the flag.

```bash
grep -nE 'Flags\(\)\.(Bool|String|Int|Count)' internal/cli/root.go internal/cli/help.go   # 42 lines
```

## The flags

| Flag | Type | Default | Sets |
|---|---|---|---|
| `-o`, `--output` | string | empty | the output file; an error together with `--ai-action` |
| `--stdout` | bool | `false` | print the output; no `.aid/` file unless `-o` is given |
| `--format` | string | `text` | the name passed to `internal/formatter/registry.go#Get`; a path run accepts only `md`, `text`, `jsonl`, `json-structured`, `xml` |
| `--strip` | string list | empty | deprecated and hidden; the `--strip` branch below |
| `--include` | string list | empty | `IncludePatterns` |
| `--exclude` | string list | empty | `ExcludePatterns` |
| `--expand` | string list | empty | `ExpandSymbols` |
| `-r`, `--recursive` | string | `1` | `Recursive`, false only for the exact value `0` (`--recursive=false` still recurses) |
| `--file-path-type` | string | `relative` | `FilePathType`; set to `absolute` when the path argument is absolute and the flag is not given; any value other than `relative` prints absolute paths |
| `--relative-path-prefix` | string | empty | `RelativePathPrefix`, put before relative paths |
| `-v`, `--verbose` | count | `0` | the debug level of `internal/debug` |
| `--version` | bool | `false` | prints the version and exits 0 (PreRun) |
| `--help` | bool | `false` | cobra's help |
| `--help-extended` | bool | `false` | prints the extended help and exits 0 (PreRun) |
| `--cheat` | bool | `false` | prints the quick reference and exits 0 (PreRun) |
| `--lang` | string | `auto` | the stdin language; ignored for a path |
| `--public` | string | `1` | `IncludePublic`, `RemovePublic` |
| `--protected` | string | `0` | `IncludeProtected`, `RemoveProtectedOnly` |
| `--internal` | string | `0` | `IncludeInternal`, `RemoveInternalOnly` |
| `--private` | string | `0` | `IncludePrivate`, `RemovePrivateOnly` |
| `--comments` | string | `0` | `IncludeComments` |
| `--docstrings` | string | `1` | `IncludeDocstrings` |
| `--implementation` | string | `0` | `IncludeImplementation` |
| `--imports` | string | `1` | `IncludeImports` |
| `--annotations` | string | `1` | `IncludeAnnotations` |
| `--fields` | string | `1` | `IncludeFields` |
| `--methods` | string | `1` | `IncludeMethods` |
| `--include-only` | string | empty | the `--include-only` branch below |
| `--exclude-items` | string | empty | the `--exclude-items` branch below |
| `-w`, `--workers` | int | `0` | `Workers`: 0 is 80% of the CPU cores (at least 1), a negative count runs one worker |
| `--raw` | bool | `false` | `RawMode` (`modes.md`) |
| `--git-limit` | int | `200` | git mode: `git log -n<N>`; 0 or less shows every commit |
| `--with-analysis-prompt` | bool | `false` | git mode: prepends the analysis prompt |
| `--ai-analysis-task-list` | bool | `false` | deprecated and hidden; becomes `--ai-action=flow-for-deep-file-to-file-analysis` |
| `--ai-action` | string | empty | the AI action mode (`modes.md`) |
| `--ai-output` | string | empty | the AI action's output path (`internal/ai/action.go#ActionConfig`) |
| `--summary-type` | string | `visual-progress-bar` | the summary format (`internal/summary/summary.go#GetFormatter`); `off` prints none |
| `--no-emoji` | bool | `false` | the summary without emoji |
| `--show-ai-agent-instructions` | bool | `false` | the output of a path run in `text` format that processed at least one file starts with a paragraph of instructions for an AI agent |
| `--filter-imports` | bool | `true` | `FilterImports` of the formatter, effective for `--format text` only |
| `--dependency-aware` | bool | `false` | `SymbolResolution` (`modes.md`) |
| `--max-depth` | int | `2` | `MaxDepth` for `--dependency-aware` |

Parsing (PreRun in `initFlags`): `--public`, `--protected`, `--internal`,
`--private`, `--comments`, `--docstrings`, `--implementation`, `--imports` and
`--annotations` go through `internal/cli/root.go#parseBoolFlag`, which accepts
`0`, `1` and what `strconv.ParseBool` accepts (`true`, `false`, ...) and exits 1
with `Error: --<flag> must be 0 or 1` otherwise. `--fields` and `--methods` go
through `internal/cli/root.go#parseBoolStringFlag`, which falls back to `1` for
a value it cannot read, without an error. `--include-only` together with
`--exclude-items` exits 1.

## From flags to `ProcessOptions`

`internal/cli/root.go#createProcessOptionsFromFlags` has four return paths,
tried in this order. A field a path does not set keeps Go's zero value
(`false`, `0`, empty).

| Path | Taken when | Reads |
|---|---|---|
| `--strip` | `--strip` is given | `--strip`, `--workers`, `--raw`, `--max-depth`, `--dependency-aware` |
| `--include-only` | `--include-only` is not empty | the list (`internal/cli/root.go#processIncludeList`), then `--workers`, `--raw`, `--recursive`, `--include`, `--exclude`, `--expand`, `--max-depth`, `--dependency-aware` |
| `--exclude-items` | `--exclude-items` is not empty | the list (`internal/cli/root.go#processExcludeList`), then the same flags as `--include-only` |
| individual flags | otherwise | every visibility and content flag, then the same flags as `--include-only` |

So the group flags ignore `--implementation`, `--private` and the other
individual flags, and the `--strip` path also ignores `--include`, `--exclude`,
`--expand` and `--recursive`. What a missing field means shows in the `--strip`
path: it never sets `IncludeDocstrings`, `IncludeAnnotations`, `IncludeFields`,
`IncludeMethods` or `Recursive`, so `aid src --strip comments` prints classes
without members, docstrings or decorators and reads `src` without its
subdirectories.

The group lists, one category at a time:

- `--include-only` starts with everything off. `public`, `protected`,
  `internal`, `private` choose visibility (public only: `IncludePrivate`
  false; any non-public level: `IncludePrivate` true and a `Remove*Only` for
  each level not listed; a non-public level without `public`: `RemovePublic`;
  no level at all: public only). `comments`, `docstrings`, `implementation`,
  `imports`, `annotations`, `fields`, `methods` switch their option on. A list
  without `methods` or `fields` prints types with empty bodies
  (`internal/cli/flags_doc_test.go#TestIncludeOnlyRecipesKeepMembers` keeps
  such recipes out of the docs).
- `--exclude-items` starts with everything on, implementation, comments and
  every visibility included. `public` sets `RemovePublic`, `private` sets
  `RemovePrivateOnly`, `protected` sets `RemoveProtectedOnly`, `internal` sets
  `RemoveInternalOnly` (each removes only its own level; the C# combined
  modifiers follow the rule in `docs/user/COMMAND-LINE-OPTIONS.md`),
  `comments` and `docstrings` both turn
  `IncludeComments` off (doc comments stay), and `implementation`, `imports`,
  `annotations`, `fields`, `methods` turn their option off.

The individual-flags path and the visibility branches are in `SKILL.md`
section 4; what each stripper option removes is in `ir-and-stripper.md`.

## Output file name

Without `--stdout` and `-o`, `internal/cli/root.go#generateOutputFilename`
names the file `aid.<base name of the path>[.<abbreviations>].<ext>` in the
project root's `.aid/` (where that root is: `CLAUDE.md` section 4). The
extension follows `--format`: `.md`, `.jsonl`, `.json` for `json-structured`,
`.xml`, else `.txt`. Each individual flag that differs from its default adds
an abbreviation, in this order: `npub`, `prot`, `int`, `priv`, `com`, `ndoc`,
`impl`, `nimp`, `nann`, `nfld`, `nmth`. With `--strip` the abbreviations are
`ncom`, `nimp`, `nimpl`, `npub`, `npriv`, `nprot`, one per item it recognises
(`comments`, `imports`, `implementation`, `non-public`, `private`,
`protected`), in the order given. The
group flags add nothing: `aid src --exclude-items=private` writes
`.aid/aid.src.txt`, the same name as `aid src`.

## Adding a flag

Take `--max-file-size` (skip files larger than N kB) as the example. Handle
each item or rule it out in the change description:

1. Define it in `internal/cli/root.go#initFlags`, with a package-level variable
   in the `var` block of `internal/cli/root.go`. An on/off filter is a string
   flag with `0`/`1` like the others.
2. Parse and validate it in the PreRun inside `initFlags`: `parseBoolFlag` for
   a 0/1 flag; a check that rejects bad values or combinations (exit 1 with a
   message, as for `--include-only` with `--exclude-items`).
3. Set its option in every return path of `createProcessOptionsFromFlags`
   (four at this commit). A path that forgets it silently runs with the zero
   value; the `--strip` path above shows the result. If it is a category the
   group flags should know, add a case to `processIncludeList` and
   `processExcludeList`.
4. Add the field to `internal/processor/interface.go#ProcessOptions`. Then
   decide where it acts:
   - it filters IR: a field in `internal/stripper/stripper.go#Options`, its
     line in `internal/processor/interface.go#ProcessOptions.ToStripperOptions`,
     a term in `internal/stripper/stripper.go#Options.HasAnyOption` (else the
     stripper does not run when this is the only option set), and the visit
     method that applies it (`ir-and-stripper.md`);
   - it selects files, like `--max-file-size`: the walk in
     `internal/processor/concurrent.go#Processor.processDirectoryConcurrent`
     (`file-discovery.md`). Decide too what a single file path does
     (`internal/processor/processor.go#Processor.ProcessPath` does not walk)
     and whether the AI flow actions honour it: they walk on their own in
     `internal/aiactions/source_files.go#collectSourceFiles` and receive only
     the include and exclude patterns through `internal/ai/action.go#ActionContext`.
   Stdin and the prompt AI actions reuse `createProcessOptionsFromFlags`, so
   they get the option; git mode reads none of these options.
5. If it changes the output, give it an abbreviation in
   `internal/cli/root.go#generateOutputFilename`, so runs with different
   values do not overwrite one `.aid/` file. Test:
   `internal/cli/root_test.go#TestGenerateOutputFilename`.
6. Help: `internal/cli/help.go#getExtendedHelpContent` must mention `--<flag>`.
   Mention it where it belongs in `getHelpTemplate`, `showFilteringHelp`,
   `showCheatSheet` and `showAIActionsHelp` in `internal/cli/help.go`.
7. `docs/user/COMMAND-LINE-OPTIONS.md` must mention `--<flag>`.
8. `README.md`, section "Complete CLI Reference": add a table row (not tested).
9. MCP: if the MCP server should offer it, add a parameter to the
   `inputSchema` of the tools `distill_file`, `distill_directory` and
   `distill_with_dependencies` in `mcp-npm/mcp-server-sdk.ts` (the schemas of
   the other tools are in `mcp-npm/mcp-server-sdk.ts#TOOL_SCHEMAS`), and turn
   it into the aid argument in each tool's handler.
10. A test that states the behaviour: a unit test of the option mapping in
    `internal/cli/root_test.go` (as `internal/cli/root_test.go#TestPublicZeroSetsRemovePublic`),
    and a run of the real binary in `internal/conformance/cli_test.go` or a
    unit test in the package that acts on it. Test methods:
    `.claude/skills/aid-testing-debugging/SKILL.md`.

The flag drift tests in `internal/cli/flags_doc_test.go` fail until items 6
and 7 are done; the last two below check the examples written in the same
texts:

```bash
CGO_ENABLED=1 CGO_CFLAGS=-w go test -count=1 -run 'TestExtendedHelp|TestHelpTexts|TestCommandLineOptionsDoc|TestIncludeOnlyRecipes|TestRedirectExamples' ./internal/cli
```

- `TestExtendedHelpCoversEveryFlag`: every flag that is neither deprecated nor
  hidden appears as `--<name>` in `getExtendedHelpContent`.
- `TestCommandLineOptionsDocMatchesFlags`: the same for
  `docs/user/COMMAND-LINE-OPTIONS.md`, and every `--word` in that file is a
  defined flag.
- `TestHelpTextsNameOnlyRealFlags`: every `--word` in the help template, the
  extended help, the filtering, git and cheat-sheet texts and `rootCmd.Long`
  is a defined flag, so renaming or removing a flag fails until the texts
  follow.
- `TestCommandLineOptionsDocListsEveryAIAction`: the doc names every
  registered AI action in backticks.
- `TestIncludeOnlyRecipesKeepMembers`: every `--include-only` recipe in the
  help texts and `docs/user/COMMAND-LINE-OPTIONS.md` lists `methods` or
  `fields`.
- `TestRedirectExamplesPrint`: in the same texts, every line starting with
  an `aid` command that redirects (`>`) or pipes (`|`) its output has
  `--stdout` or is a git-mode example (`.git`); without `--stdout` aid prints
  no distilled text, so the example would capture only status lines.

They compare names and example shapes only; a wrong default or description in
the texts passes.
