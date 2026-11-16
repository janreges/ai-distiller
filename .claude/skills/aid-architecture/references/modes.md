# Modes

`internal/cli/root.go#runDistiller` picks one mode per run; the order is in
`SKILL.md` section 3. This file says what each mode does and where it lives.

## Stdin

When it is taken: `CLAUDE.md` section 4 (a path argument wins; `-` or no
argument with a non-terminal stdin reads stdin).

`internal/cli/root.go#processStdinWithContext`:

1. reads all of stdin; with `--lang auto` (the default) empty input fails with
   `no input on stdin`, and the detector (`internal/language/detector.go`;
   rules in `.claude/skills/aid-language-parsers/references/detector.md`)
   picks the language or the run fails with a list of the languages;
2. maps aliases through `internal/language/names.go#Canonical` (`py`, `ts`,
   `c++`, ...); an unknown name fails with the list of names and aliases;
3. parses under the name `stdin.tsx` when the language is TypeScript and
   either `--lang tsx` was given or `--lang auto` found JSX
   (`internal/language/detector.go#HasJSX`), else under `stdin`;
4. calls the language processor's `ProcessWithOptions` directly with the
   options from `createProcessOptionsFromFlags`, then formats.

What it skips: the walk and every file-selection rule, `--dependency-aware`
(that runs in `Processor.ProcessFile`), the check of `--format` against the
five names (so the registry's aliases `markdown`, `json` and `json-lines` work
here), the removal of empty `<file>` blocks, `--show-ai-agent-instructions`
and the summary. The output is always printed, labelled `stdin`; `-o FILE`
also writes it to FILE.

## Git mode

Taken when the base name of the path is `.git`, after the check that the path
exists and before AI actions. `internal/cli/root.go#handleGitMode` runs
`git -C <parent of .git> log` with a custom format, adding `-n<N>` when
`--git-limit` is above 0 (default 200), and prints one block per commit:

```text
[0894db6] 2026-11-15 14:16:24 | t                    | second
        body line
```

The author is the name without the e-mail, padded to 20 characters and cut to
17 plus `...` when longer; the subject is cut to 77 plus `...` beyond 80
characters; body lines are trimmed, empty ones dropped, and indented by 8
spaces; a blank line separates commits.

`--with-analysis-prompt` puts the `git-analysis` template in front
(`internal/aiactions/templates/git-analysis.md`, loaded by
`internal/cli/root.go#getGitAnalysisPrompt`, with a short built-in prompt if
loading fails) and wraps the log in `=== BEGIN GIT LOG ===` and
`=== END GIT LOG ===`.

Output: stdout by default. `-o FILE` writes FILE and prints only with
`--stdout`; without it a `Git log written to: <path>` line goes to stderr.
No `.aid/` file, no summary; `--format` and every filter flag are ignored. No
test runs git mode.

## Raw mode (`--raw`)

Not a branch of `runDistiller`: the normal mode with `RawMode` set. The walk
drops its "has a processor" test, and `Processor.processFile` hands every
file, source code included, to `internal/processor/raw_processor.go#RawProcessor`,
which returns the whole text as one `DistilledRawContent` node; nothing is
stripped. A file whose extension is on the raw processor's binary list
(images, archives, `.pyc`, `.class`, ...) is skipped with `Warning: skipping
binary file`; a binary file with another extension is read as text.
`.aidignore`, the default-ignored directories, `.aid.` names and
`--include`/`--exclude` still apply.

Without `--raw`, a file with no processor that `.aidignore` re-includes with a
`!` pattern also goes to the raw processor (`file-discovery.md`).

## AI actions (`--ai-action`)

Taken when `--ai-action` is set; the deprecated `--ai-analysis-task-list`
becomes `--ai-action=flow-for-deep-file-to-file-analysis`. With `-o` the run
fails with a message pointing to `--ai-output`.

The parts:

- `internal/ai/action.go#AIAction`: `Name`, `Description`, `Type`,
  `DefaultOutput`, `Validate`. A prompt action is an
  `internal/ai/action.go#ContentAction` (`GenerateContent` returns the text to
  put before and after the distilled code); a flow action is an
  `internal/ai/action.go#FlowAction` (`ExecuteFlow` returns files and
  messages).
- The registry: `internal/ai/registry.go#ActionRegistry`, the process-wide
  instance `internal/ai/registry_global.go#GetRegistry`, filled at start-up by
  `internal/cli/root.go#registerAIActions` through
  `internal/aiactions/register.go#Register`.
- The actions, `internal/aiactions`: two flows
  (`internal/aiactions/deep_analysis_flow.go#DeepAnalysisFlowAction`,
  `internal/aiactions/multi_file_docs_flow.go#MultiFileDocsFlowAction`) and
  eight prompt actions in `internal/aiactions/template_actions.go`, each
  rendering one template. The older prompt types in `refactoring_prompt.go`,
  `security_prompt.go`, `performance_prompt.go` and
  `complex_codebase_prompt.go` are compiled but not registered.
- The templates, `internal/aiactions/templates/*.md`, embedded by
  `//go:embed templates/*.md` (`internal/aiactions/template_loader.go#embeddedTemplates`).
  `internal/aiactions/template_loader.go#LoadTemplate` reads the embedded file
  (falling back to a search of the file system) and renders it with
  `text/template`: data `{{.ProjectName}}` and `{{.AnalysisDate}}`, functions
  `VERSION` and `WEBSITE_URL`.

A prompt action (`internal/cli/root.go#handleAIAction`, then
`#distillForAction` and `#executeContentAction`) distils the path with the
same options as a normal run, but with relative paths and always as `text`
with the import filter on, regardless of `--format` and `--filter-imports`. It
writes the template followed by the distilled text to the output path and
prints a status block; with `--stdout` it prints the prompt instead (status on
stderr) and still writes the file, unless `--ai-output -` is given.

A flow action (`#executeFlowAction`) distils nothing. It lists files with
`internal/aiactions/source_files.go#collectSourceFiles`, a walk of its own:

- its own extension list;
- below the walk root it skips the directories named in its map `skipDirs`
  (`node_modules`, `__pycache__`, `target`, `build`, `dist`, `out`, `vendor`,
  `coverage`, `grammars`, `test-data`, `testdata`, `docs`, `examples`,
  `assets`, `static`, and hidden names such as `.git`), every directory whose
  name starts with `.`, and every directory whose path relative to the walk
  root contains `grammars` or `tree-sitter` (`lib/my-tree-sitter-x` too);
- it leaves out files whose name starts with `.` or contains `generated`;
- it reads no `.aidignore`; `--include`/`--exclude` go through
  `internal/processor/processor.go#MatchesPatterns`.

It writes the returned files under the output directory and prints status
lines to stdout, with or without `--stdout`.

The output path is `--ai-output` or the action's `DefaultOutput`, expanded by
`internal/ai/template.go#ExpandTemplate` and checked by
`internal/ai/template.go#ValidateOutputPath` (it must lie inside the analysed
path or in the project root's `.aid/`):

- a leading `./.aid/` or `.aid/` becomes the project root's `.aid/`, so the
  prompt actions' defaults (`./.aid/<NAME>.%YYYY-MM-DD.HH-MM-SS%.%folder-basename%.md`)
  land there wherever aid runs;
- the flows' default is `./.aid` without a slash, which is not mapped: flows
  write into `.aid/` of the working directory (run from `src/sub`, a flow
  created `src/sub/.aid/`);
- placeholders: `%YYYY%`, `%MM%`, `%DD%`, `%HH%`, `%mm%`, `%SS%`,
  `%YYYY-MM-DD%`, `%HH-MM-SS%`, `%YYYY-MM-DD.HH-MM-SS%`, `%folder-basename%`
  and `%project%` (both the base name of the path).

### Adding a prompt action

1. Write `internal/aiactions/templates/<template>.md`; the embed glob picks it
   up.
2. Add a type to `internal/aiactions/template_actions.go` that implements
   `ai.ContentAction`: `Name` (`prompt-for-...`), `Type` returning
   `ai.ActionTypePrompt`, `DefaultOutput` starting with `./.aid/`, and
   `GenerateContent` calling `LoadTemplate("<template>", ...)`.
3. Register it in `internal/aiactions/register.go#Register`.
4. List it in `docs/user/COMMAND-LINE-OPTIONS.md` in backticks
   (`internal/cli/flags_doc_test.go#TestCommandLineOptionsDocListsEveryAIAction`
   fails until it is), and in the hand-written lists in
   `internal/cli/help.go` (`getHelpTemplate`, `showAIActionsHelp`,
   `showCheatSheet`); nothing checks those.
5. For the MCP server, add the name to the `ai_action` enum in
   `mcp-npm/mcp-server-sdk.ts#TOOL_SCHEMAS`.
6. Test it: run `aid <dir> --ai-action <name> --stdout --ai-output ./.aid/x.md`
   on a temporary project and read the prompt. No test checks prompt texts
   today (`aid-testing-debugging`, section 8).

A flow action implements `ai.FlowAction` instead and gets its file list from
`collectSourceFiles`; tests: `internal/aiactions/source_files_test.go`.

## Dependency-aware (`--dependency-aware`, experimental)

Off by default; `internal/processor/interface.go#DefaultProcessOptions` also
keeps it off (`SymbolResolution` false, `MaxDepth` 2). The flag sets
`SymbolResolution`, `--max-depth` sets `MaxDepth`.

`runDistiller` rejects a `--max-depth` below 1 when the flag is set, then
calls `internal/processor/dependency.go#Processor.ProcessWithDependencies`
instead of `ProcessPath` (`ProcessPath` itself routes there too, which is how
AI actions get it). That function:

1. distils the input without the option: a file with `ProcessFile`, a
   directory with the normal walk (`walkDirectory`, `processTasks` in
   `internal/processor/concurrent.go`);
2. hands the input's files to
   `internal/processor/dependency.go#DependencyAnalyzer.Follow`, which reads
   each file's imports (`DependencyAnalyzer.Imports`, one regex extractor per
   language behind `extractIncludeStatements`), breadth-first, one level per
   step up to `MaxDepth`, and keeps each resolved path once if it is an
   existing file with a processor, lies inside the boundary and passes the
   walk's selection rules (both in `internal/processor/dependency_scope.go`,
   see below); a skipped file is not read;
3. distils every followed file with `ProcessFile` and the same options and
   appends it to the result, a `DistilledDirectory`, with a path relative to
   the entry file's directory (or the input directory), `../` included
   (`dependencyDisplayPath`; absolute with `--file-path-type=absolute`).

Four languages resolve imports through their project files, each in its own
file: Go through `go.mod` (`internal/processor/dependency_go.go`), Rust
through `Cargo.toml` (`internal/processor/dependency_rust.go`), TypeScript and
JavaScript through `tsconfig.json`/`jsconfig.json` paths
(`internal/processor/dependency_javascript.go`) and PHP through the PSR-4
autoload of `composer.json` (`internal/processor/dependency_php.go`). The
other extractors in `dependency.go` match paths next to the importing file.
README's "What is followed, per language" table lists what each one follows
and what was measured on real repositories.

The boundary (`dependencyBoundary`) is the nearest directory at or above the
input that holds `.git`, else the project root of `internal/project` when it
holds the input and is not the working-directory fallback
(`RootInfo.IsFallback`, which the cached root keeps), else the input's
directory. With `AID_CONFINE_ROOT` set and below that directory,
`newFollowScope` takes the confine root instead (kind `AID_CONFINE_ROOT
directory`). `followScope.contains` compares
real paths (symlinks resolved) and also checks
`internal/project/confine.go#InConfineRoot`; the resolvers read no `go.mod`,
`Cargo.toml`, `tsconfig.json`, `composer.json` or package listing outside it
(`DependencyAnalyzer.findUp`), and a Go package directory outside it is
reported by `Follow` without being listed. `followScope.excludedBy` applies the walk's
rules to a followed file: every `.aidignore` from the boundary down to the
file's directory, the default-ignored directories (`isDefaultIgnoredDir`) and
`.aid`, and `--include`/`--exclude` (`matchPatterns`) on the path below the
deepest directory that holds both the input and the file. `-v` logs each
skipped import; the `Note:` line counts them (`DependencyStats`).

No symbol is resolved: a followed file is distilled whole. When nothing is
followed, `runDistiller` prints one `Note:` line on stderr
(`internal/cli/root.go#printNothingFollowedNote`) and exits 0. Stdin never
reaches it; the MCP tool `distill_with_dependencies` uses it. CLI tests:
`internal/conformance/dependency_aware_test.go`, fixtures in
`internal/conformance/testdata/depaware/`.
