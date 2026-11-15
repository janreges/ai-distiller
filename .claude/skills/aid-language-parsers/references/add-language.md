# Adding a language

A checklist of every place that names the languages. The core (parser,
registration, formatter, tests) makes the language work; the rest keeps help,
docs and the MCP package truthful. `<lang>` is the processor name, the name the
processor registers under and the IR's `Language`.

## Core

1. Processor package `internal/language/<lang>/`:
   - `processor.go` with a `Processor` embedding `processor.BaseProcessor`,
     built by `NewProcessor()` with the name, a version and the extensions
     (`processor.NewBaseProcessor`), and a `ProcessWithOptions` that follows the
     contract in `SKILL.md` section 1 (new parser object per call, parse,
     `ToStripperOptions`, stripper).
   - The parser. For tree-sitter, copy the shape of an existing processor, for
     example `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`,
     and keep the conventions in `references/ir-conventions.md`.
   - Unit tests beside it.
2. Grammar: add the grammar module to `go.mod` and wrap it with
   `sitter.NewLanguage(...)`; the processor creates its parser with
   `internal/language/tsparse/tsparse.go#NewParser`, parses with
   `internal/language/tsparse/tsparse.go#Parse` and closes both parser and
   tree (`references/tree-sitter-cgo.md`, "The runtime"), and add it to
   `cmd/ast-dump/main.go#language` and the usage string in the same file, so
   the tree can be dumped.
3. Registration: `internal/language/registry_cgo.go#RegisterAll`.
   (There is no `!cgo` registry: the module does not compile with
   `CGO_ENABLED=0`; see `references/tree-sitter-cgo.md`.)
4. Aliases for `--lang`, if any: `internal/language/names.go#aliases`.
   `internal/language/names_test.go#TestCanonical_EveryAliasHasAProcessor`
   fails for an alias without a processor.
5. Stdin detection: a `has<Lang>Patterns` function and a score in
   `internal/language/detector.go#LanguageDetector.detectLanguage`, and the
   fixture in `internal/language/detector_test.go#TestDetector_ConformanceFixtures`
   (`references/detector.md`).
6. Text formatter: `internal/formatter/<lang>_formatter.go`, registered in
   `internal/formatter/language_aware_text_formatter.go#NewLanguageAwareTextFormatter`.
   Without one, files of the language go through the generic fallback.
   The language formatter prints the visibility keywords itself, for example
   through `internal/formatter/csharp_formatter.go#CSharpFormatter.getVisibilityKeyword`
   (Python uses `internal/formatter/visibility.go#getVisibilityPrefix`).
   Nothing calls `BaseLanguageFormatter.GetVisibilityKeyword`, so changing it
   changes no output.
7. Markdown code fences: `internal/formatter/markdown.go#MarkdownFormatter.getLanguageFromPath`
   (by extension) and `internal/formatter/markdown.go#MarkdownFormatter.getLanguageIdentifier`
   (by language name).
8. Import filter (optional; not every language has one):
   `internal/importfilter/<lang>_filter.go`, registered from its `init()` with
   `internal/importfilter/filter.go#Register`.
9. Dependency analysis (optional, for the AI actions that follow calls): the
   language switches in `internal/processor/dependency.go`
   (`DependencyAnalyzer.extractIncludeStatements`,
   `DependencyAnalyzer.buildSymbolTableForFile`,
   `DependencyAnalyzer.detectLanguage`, `DependencyAnalyzer.extractCallsFromFile`).

## Tests

10. Conformance: a fixture `internal/conformance/testdata/<lang>/` and an entry
    in `internal/conformance/fixtures_test.go#fixtures` with the standard
    checks, `stdin-detect` included. Run
    `go test -count=1 -run 'TestConformance/^<lang>$/' ./internal/conformance`.
    A check that cannot pass yet goes into `knownGaps` in
    `internal/conformance/known_gaps_test.go` with its reason.
11. Formatter coverage: `internal/conformance/unhandled_test.go#TestNoUnhandledNodesInTestdata`
    fails when the text formatter drops a node kind of the new language; fix
    the formatter or list the pair in
    `internal/conformance/unhandled_test.go#knownUnhandled`.
12. Snapshots: a directory per construct under `testdata/<lang>/` with a
    `source.<ext>` and expected outputs, and the extension in
    `internal/testrunner/runner.go#findSourceFile`, which maps a language to
    its source extensions (`aid-testing-debugging`, section 3).

## Help, docs and MCP

13. `--lang` help: the flag description in `internal/cli/root.go#initFlags`,
    the `--lang` line in `internal/cli/help.go#getHelpTemplate`, and in
    `internal/cli/help.go#getExtendedHelpContent` both the `--lang` option and
    the supported-languages list. The error text for an unknown or undetected
    language is built from the registry and needs no change.
14. Docs: `docs/lang/<lang>.md`, and in `README.md` the language list with its
    docs links, the support table, the `--lang` row, the auto-detection
    sentence and the list of extensions.
15. MCP package: `mcp-npm/list-files.ts#LANGUAGE_BY_EXTENSION` (extension to
    language), the `supported_languages` array in
    `mcp-npm/mcp-server-sdk.ts#supported_languages`, and the "Supported
    Languages" section of `mcp-npm/README.md`.

## A new extension for an existing language

When an existing processor takes one more file extension, only these apply:

- the extension list in the package's `NewProcessor()` (for example
  `internal/language/cpp/processor.go#NewProcessor`); the registry maps
  extensions to processors from it;
- `internal/formatter/markdown.go#MarkdownFormatter.getLanguageFromPath`;
- `internal/testrunner/runner.go#findSourceFile`, if snapshots use the
  extension;
- `mcp-npm/list-files.ts#LANGUAGE_BY_EXTENSION`;
- the extension list in `README.md` and the language's `docs/lang/<lang>.md`.

The processor is chosen by the last extension only, lower-cased, so a double
extension such as `.d.ts` arrives as `.ts`.
