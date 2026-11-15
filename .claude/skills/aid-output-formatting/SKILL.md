---
name: aid-output-formatting
description: >-
  How AI Distiller renders IR into output: the per-language text formatters,
  the shared body and comment renderers, the md/jsonl/json-structured/xml
  formatters, the regex import filter and the summary line. Use whenever the
  printed output looks wrong although the IR is right, when changing how
  something is printed, adding an IR field or node type that must appear in
  output, adding an output format, or estimating which snapshots a formatter
  change will touch. Also for "výstup vypadá špatně", "změň formát", "přidej
  do výstupu", "proč zmizel import".
---

# Output formatting

What happens after the stripper: how an `ir.DistilledFile` becomes the bytes aid
prints. How the IR is built is in `.claude/skills/aid-language-parsers/SKILL.md`
(node fields: its `references/ir-conventions.md`); flags, modes and the stripper
are in `.claude/skills/aid-architecture/SKILL.md`; running and updating tests is in
`.claude/skills/aid-testing-debugging/SKILL.md`. Run commands from the repository
root with `CGO_ENABLED=1 CGO_CFLAGS=-w` exported.

References, loaded when needed:

- `references/text-conventions.md`: per language, how visibility, bodies,
  comments, types and imports print today, recorded from aid's output.
- `references/structured-formats.md`: the shape of `jsonl`, `json-structured`
  and `xml`, a real example of each, and what each leaves out.
- `references/import-filter.md`: the unused-import filter of the `text` format.
- `references/summary.md`: the summary on stderr, `--summary-type`, sizes and
  the token estimate.
- `.claude/skills/aid-output-formatting/scripts/format-matrix.sh`: one file in
  all five formats (section 2).

## 1. From IR to bytes

The `text` format, which is the default, the format of the snapshots and the
one the prompt AI actions distil to:

1. `internal/cli/root.go#runDistiller` builds `internal/formatter/formatter.go#Options`.
   Its `ProcessingInfo.FilterImports` is true only when `--filter-imports` is on
   (the default) and `--format` is `text`. `internal/formatter/registry.go#Get`
   returns the formatter registered under the `--format` name.
2. For `text` that is `internal/formatter/language_aware_text_formatter.go#LanguageAwareTextFormatter`.
   `NewLanguageAwareTextFormatter` registers one
   `internal/formatter/language_formatter.go#LanguageFormatter` per language
   name (`c#` and `c++` as aliases); the names match the `Language` each
   processor puts on the file.
3. `LanguageAwareTextFormatter.Format`, per file: writes `<file path="...">`,
   picks the language formatter by `file.Language`, calls its `Reset` (per-file
   state), then
   - if it implements `internal/formatter/language_formatter.go#SiblingFormatter`
     (C++, C#, Kotlin, Python, Ruby), calls `FormatNodes` on the whole child
     list, because where those formatters print a node depends on the node
     after it (a doc comment waits for the declaration it documents);
   - otherwise calls `FormatNode` for each child;
   - if no formatter is registered for the language, uses the generic fallback
     `internal/formatter/language_aware_text_formatter.go#LanguageAwareTextFormatter.formatNodeGeneric`.
     At this commit only `--raw` files reach it (their language is `text`).

   Then it writes `</file>`.
4. With `FilterImports` on and a non-empty `file.Language`,
   `LanguageAwareTextFormatter.formatWithFiltering` renders the file into a
   buffer first and passes that text to
   `internal/importfilter/filter.go#FilterImports`, one file at a time; on a
   filter error the unfiltered text is written. Details: `references/import-filter.md`.
5. A directory goes through `LanguageAwareTextFormatter.FormatMultiple`:
   `Format` per file, a blank line between files.
6. Back in `runDistiller`, for `text` only: `--show-ai-agent-instructions`
   puts its paragraph in front (when at least one file was processed), and a
   regular expression deletes every `<file path="...">` block that holds only
   whitespace (the path is escaped, so it never holds a raw `"`). Then the
   output is written or printed and the summary goes to stderr (`references/summary.md`).

Stdin (`internal/cli/root.go#processStdinWithContext`) formats the same way,
import filter included, but keeps empty blocks and prints no summary.

The helper `internal/formatter/visibility.go#getVisibilityPrefix` is live: the
Python formatter prints its `-` prefix with it.

## 2. Formats

| `--format` | Formatter | What it prints |
|---|---|---|
| `text` (default) | `LanguageAwareTextFormatter` | one `<file path="...">` block per file in that language's syntax; the only format with the import filter and the removal of empty blocks |
| `md` | `internal/formatter/markdown.go#MarkdownFormatter` | the text formatter's output, each file as `### <path>` and a code fence |
| `jsonl` | `internal/formatter/jsonl.go#JSONLFormatter` | one JSON object per file and one per IR node |
| `json-structured` | `internal/formatter/json_structured.go#JSONStructuredFormatter` | one indented JSON document; nodes grouped by kind (`classes`, `functions`, ...) |
| `xml` | `internal/formatter/xml.go#XMLFormatter` | a `<distilled>` root, a `<file>` element per file, one element per IR node |

The registry also knows `markdown`, `json-lines` and `json`. A path run accepts
only the five names above (`invalid output format` otherwise); stdin passes the
name straight to `Get`, so the aliases work there.

`md` is the text output wrapped in a code fence. `MarkdownFormatter` holds its
own `LanguageAwareTextFormatter`, cuts the content out of each `<file>` block
and writes it under `### <path>` in a fence tagged with the language (from
`file.Language` for one file, from the extension for a directory:
`MarkdownFormatter.getLanguageFromPath`). Because the CLI turns the import
filter on only for `text`, `md` shows every import the stripper kept, and it
keeps an empty file as an empty fence. That is why imports differ between the
two formats.

The structured formats print the IR, not the text: no import filter, and only
the fields each formatter knows (`references/structured-formats.md`).

```bash
sh .claude/skills/aid-output-formatting/scripts/format-matrix.sh internal/conformance/testdata/java/ConfWidget.java
sh .claude/skills/aid-output-formatting/scripts/format-matrix.sh internal/conformance/testdata/java/ConfWidget.java --private=1 --implementation=1
```

`format-matrix.sh <file> [aid flags...]` builds aid from the working tree into
a temp directory and prints the file in `text`, `md`, `jsonl`,
`json-structured` and `xml`, each under `=== <format> ===`; extra flags go to
every run. In the first run `text` has no `import java.io.File` and `md` has it.

## 3. Shared renderers

Bodies and comments are printed through a few helpers in `internal/formatter`.
New body or comment printing goes through them: a fix in them (tab expansion,
blank-line trimming, `*/` escaping) then reaches every language that calls
them, while a private copy in one formatter would drift away.

- `internal/formatter/body.go#renderBody` prints an `Implementation` that
  carries its own braces: the first line goes on the signature line after a
  space, a last line starting with `}` goes back to the signature's
  indentation, and the lines between are re-indented one level (four spaces)
  deeper. Used by C++, Go, JavaScript, Kotlin, Rust, Swift and TypeScript. Go
  first expands body tabs (`internal/formatter/body.go#expandLeadingTabs`).
- `internal/formatter/body.go#dedentBody` returns the body's lines re-indented
  under a prefix and leaves the delimiters to the caller: Java and PHP wrap
  them in braces, Python and Ruby print them as they are. Both helpers go
  through `internal/formatter/body.go#dedentLines`: the indentation the lines
  share is replaced, tabs are expanded to 4-column stops when the lines mix
  tabs and spaces so that no indentation is a prefix of all the others, and a
  line at column 0 inside an indented body (a multi-line
  string or heredoc) stays verbatim. C# re-indents its bodies in its own
  `internal/formatter/csharp_formatter.go#CSharpFormatter.formatFunction`.
- `internal/formatter/comment.go#writeCStyleComment` writes a comment
  node: `//` before each line of a line comment; `/* x */` for a one-line
  block and `/** x */` for a one-line doc; with several lines, `/*` or `/**`,
  a `*` line per text line and `*/`. A `*/` inside the text becomes `*\/`.
  Used by C++, JavaScript, Kotlin and TypeScript for every comment, and by Java
  and C# for non-doc comments (Java writes the Javadoc layout itself, C# writes
  docs as `///` in `internal/formatter/csharp_formatter.go#writeCSharpComment`).
- `internal/formatter/comment.go#formatCStyleNodes` and
  `#formatCommentedNodes` format a sibling list in which a run of comments
  waits for the node after it; when the formatter does not print that node,
  the run's doc comments are dropped with it, so they cannot appear to document
  the next declaration. C++ and Kotlin use the first, C# the second.
- `internal/formatter/comment.go#isShebang` tells Python and Ruby that a
  comment is a `#!` first line, which they print as written.

Go, PHP, Python, Ruby, Rust and Swift write comments with their own
`formatComment`. Who calls what, at any commit:

```bash
grep -l 'renderBody(' internal/formatter/*_formatter.go
grep -l 'dedentBody(' internal/formatter/*_formatter.go
grep -l 'writeCStyleComment(\|formatCStyleNodes(\|formatCommentedNodes(' internal/formatter/*_formatter.go
```

Unit tests: `internal/formatter/body_test.go` and `internal/formatter/comment_test.go`.

## 4. Skipped nodes

Every language formatter's node dispatch (`FormatNode`, or in JavaScript,
TypeScript and Ruby the `formatNode` it calls) ends in a `default:` branch that
drops the node and calls `reportUnhandled`, which calls
`internal/formatter/unhandled.go#OnUnhandledNode` when it is set; the generic
fallback reports as `generic`. The variable is nil by default. With `-v` the
CLI sets it and logs each kind once per language:

```bash
go run ./cmd/aid internal/conformance/testdata/cpp/sample.cpp --stdout --summary-type=off -v </dev/null 2>&1 >/dev/null | grep 'no text rendering'
# [..] INFO: formatter: no text rendering for package in cpp
```

`internal/conformance/unhandled_test.go#TestNoUnhandledNodesInTestdata` formats
the unstripped IR of every `testdata/*/*/source.*` and fails for a dropped
`<lang>/<kind>` pair that is not in
`internal/conformance/unhandled_test.go#knownUnhandled`, the live list of what
the text formatters do not render. A listed pair that is rendered now fails
too, so rendering one means deleting its entry in the same commit
(`aid-testing-debugging`, section 4).

Only those `default:` branches report. A loop over a type's members that a
formatter writes itself can skip a kind without a report; for example
`internal/formatter/cpp_formatter.go#CppFormatter.formatMembers` prints
functions, fields and comments only. A missing member with no `-v` line is therefore
not proof that the parser lost it: check `--format jsonl`.

## 5. Checklists

### Change one language's text output

1. Confirm the IR is right and the field is set: section 3 of
   `.claude/skills/aid-testing-debugging/scripts/triage.sh` (jsonl), or the
   `-vvv` dump for fields jsonl does not show (`references/structured-formats.md`).
   If the parser never fills the field, that is a parser change first.
2. Edit `internal/formatter/<lang>_formatter.go`, `<lang>` being the
   `file.Language` value. Print bodies and comments through the shared
   renderers (section 3).
3. Test it in `internal/formatter/<lang>_formatter_test.go` on hand-built IR;
   `internal/formatter/cpp_formatter_test.go#formatFileAs` formats a file as a
   given language. Swift has no formatter test file.
4. If the change alters how a name or an import line prints, compare with
   `--filter-imports=false`: the filter searches the printed text
   (`references/import-filter.md`).
5. Run that language's conformance checks and `TestNoUnhandledNodesInTestdata`,
   update that language's snapshots only and review them (section 6).
   `md` follows automatically; the other formats do not change.
6. If a convention changes, update its row in `references/text-conventions.md`.

### Add an IR field or node type

A field:

1. The field in `internal/ir/nodes.go`, filled by the parsers that know it
   (`aid-language-parsers`); add it to the node table in
   `.claude/skills/aid-language-parsers/references/ir-conventions.md`.
2. The stripper needs nothing: it copies each kept node with all its fields
   (`internal/stripper/stripper.go#Stripper.Visit`), unless a flag should
   remove the field (`aid-architecture` `references/ir-and-stripper.md`).
3. Every text formatter that should print it.
4. `jsonl`: `internal/formatter/jsonl.go#JSONLFormatter.nodeToJSON`.
5. `json-structured`: the node's `...ToStructured` method, for example
   `internal/formatter/json_structured.go#JSONStructuredFormatter.functionToStructured`.
6. `xml`: `internal/formatter/xml.go#XMLFormatter.formatNode`.
7. Tests: the formatter unit tests of each format (`<lang>_formatter_test.go`,
   `jsonl_test.go`, `json_structured_test.go`, `xml_test.go`), a conformance
   check if it is a capability, and the snapshots of each language that prints it.

A node type, in addition:

1. A struct in `internal/ir/nodes.go` that embeds `BaseNode` and has
   `GetNodeKind`, `GetChildren` and `Accept`, and a `NodeKind` constant in
   `internal/ir/types.go`.
2. A case in `internal/stripper/stripper.go#Stripper.Visit`: a kind without a
   case is returned unchanged, so visibility, `--fields=0` and the other
   options never remove it.
3. A case in the node dispatch of every text formatter (section 4), in member
   loops such as `CppFormatter.formatMembers`, and in `formatNodeGeneric`; otherwise
   `TestNoUnhandledNodesInTestdata` fails as soon as a testdata source produces it.
4. `jsonl`: `nodeToJSON` and `internal/formatter/jsonl.go#JSONLFormatter.getNodeName`
   (without it the line carries only `type`, `file`, `path` and the kind as `name`).
5. `json-structured`: `internal/formatter/json_structured.go#JSONStructuredFormatter.structureNodes`
   and `#JSONStructuredFormatter.structureClassMembers`; without a case the
   node is left out silently, though `stats` still counts it.
6. `xml`: `formatNode`; without a case it prints `<node kind="..."/>`.

### Add a format

1. A type implementing `internal/formatter/formatter.go#Formatter` in
   `internal/formatter/<name>.go`, with a constructor taking `Options`.
2. Register it in the `init` of `internal/formatter/registry.go`.
3. Add the name to `validFormats` in `runDistiller`; path runs reject other names.
4. Its extension in `internal/cli/root.go#generateOutputFilename`, which has
   its own switch (`Formatter.Extension` is not used for the file name).
5. Decide whether the text-only steps apply: the import filter (`FilterImports`
   in `runDistiller` and `processStdinWithContext`), the removal of empty
   blocks and `--show-ai-agent-instructions` (section 1).
6. The `--format` help in `internal/cli/root.go#initFlags`,
   `internal/cli/help.go#getHelpTemplate`, `#getExtendedHelpContent`,
   `#showCheatSheet`, `docs/user/COMMAND-LINE-OPTIONS.md` and `README.md`.
7. MCP: every `output_format` enum and `supported_formats` in
   `mcp-npm/mcp-server-sdk.ts` (`grep -n 'json-structured' mcp-npm/mcp-server-sdk.ts`).
8. Tests: a unit test on hand-built IR as in `internal/formatter/jsonl_test.go`,
   the name in `internal/formatter/registry_test.go`, and in the format list of
   `internal/conformance/cli_test.go#TestStructuredFormatsDoNotLeakMetadata`.

## 6. Snapshots

The snapshot suite and the conformance checks run `--format text` with the
import filter on (one conformance check turns it off), so only the text path
reaches them:

| You changed | Snapshots it can touch |
|---|---|
| `internal/formatter/<lang>_formatter.go` | `testdata/<lang>/*/expected/*.txt`; for Go and Java also the unit tests that compare `01_basic` snapshots in-process |
| `internal/formatter/body.go`, `internal/formatter/comment.go` | every language that calls the helper (the `grep -l` lines in section 3) |
| `internal/formatter/language_aware_text_formatter.go` | every language |
| `internal/importfilter/<lang>_filter.go` | `testdata/<lang>` scenarios whose source has imports; the JavaScript filter also serves TypeScript |
| `md`, `jsonl`, `json-structured`, `xml`, `internal/summary` | none; their unit tests and `internal/conformance/cli_test.go` |

Before running anything, find the scenarios that contain the construct and
count the snapshots of the language:

```bash
grep -l 'throws' testdata/java/*/source.java
ls testdata/java/*/expected/*.txt | wc -l
```

Then update that one language and read the whole diff; the procedure, the
commands and the Go and Java unit tests that compare against snapshots are in
`.claude/skills/aid-testing-debugging/SKILL.md`, section 3. Every changed line
must follow from your change. A changed import line in a language whose
formatter you touched usually means the printed text now mentions a name
differently, and the filter decided again.
