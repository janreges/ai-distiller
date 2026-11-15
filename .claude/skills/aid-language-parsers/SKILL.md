---
name: aid-language-parsers
description: >-
  How AI Distiller's 12 language processors in internal/language/ turn source
  code into IR: which parser each language really uses (tree-sitter via cgo,
  go/parser for Go, regex line parsers for Rust and Swift), visibility rules,
  how methods attach to types, and where each language's known gaps are
  recorded. Use whenever output for a specific language is missing or wrong (a
  construct disappears, wrong visibility, missing body, wrong signature), when
  adding syntax support, touching a tree-sitter grammar or the stdin language
  detector, or adding a new language. Also for "parser nevidí", "chybí ve
  výstupu", "přidej podporu pro", "nový jazyk".
---

# Language parsers

Run every command from the repository root with `CGO_ENABLED=1 CGO_CFLAGS=-w`
exported: the tree-sitter grammars are C code, and aid does not compile without
cgo (`references/tree-sitter-cgo.md`).

References, loaded when needed:

- `references/<lang>.md`, one per language (`python`, `typescript`, `javascript`,
  `go`, `rust`, `swift`, `ruby`, `java`, `csharp`, `kotlin`, `cpp`, `php`): the
  parser, visibility rule, how methods attach, IR specifics, unused files and
  where the known gaps are listed.
- `references/ir-conventions.md`: IR node types and fields, comment formats, and
  the concepts each language encodes differently.
- `references/add-language.md`: every place a new language or file extension
  has to be added.
- `references/tree-sitter-cgo.md`: where each grammar comes from, the
  TypeScript submodule, what fails without cgo.
- `references/detector.md`: how stdin input gets its language.

## 1. The contract

Every language implements `internal/processor/interface.go#LanguageProcessor`.
The method that runs is `ProcessWithOptions`, for files and for stdin alike:

- a file: `internal/processor/processor.go#Processor.ProcessFile` picks the
  processor by the file's last extension, lower-cased
  (`internal/processor/registry.go#registry.GetByFilename`, filled from each
  processor's `SupportedExtensions`), and calls its `ProcessWithOptions`;
- stdin: `internal/cli/root.go#processStdinWithContext` takes `--lang` (through
  `internal/language/names.go#Canonical`) or asks the detector, then calls the
  same method with the file name `stdin` (`stdin.tsx` for TSX).

Every `ProcessWithOptions` does the same three things:

1. parses the source into an `*ir.DistilledFile` with nothing removed;
2. calls `opts.ToStripperOptions()`
   (`internal/processor/interface.go#ProcessOptions.ToStripperOptions`);
3. when `internal/stripper/stripper.go#Options.HasAnyOption` is true, runs
   `stripper.New(...)` over the file with `file.Accept(s)` and returns the result.

The stripper runs inside each processor, not as a central stage after parsing.
So visibility, `--implementation`, `--comments` and the other content flags
reach the output only because each processor calls it; a new processor that
skips step 3 prints everything. Formatting happens later, outside the processor.

One processor instance per language is registered at start-up
(`internal/language/registry_cgo.go#RegisterAll`) and shared by all workers, so
files of the same language are parsed in parallel. Each call therefore builds
its own parser object (`NewTreeSitterProcessor()`, `NewASTParser()`,
`NewLineParser(...)`); keep per-file state in that object, never in `Processor`.

Errors: a tree-sitter parse fails only when its context is cancelled; a syntax
error becomes `ERROR` nodes in the tree and the file is processed anyway. Go's
`go/parser` returns an error for a syntax error: a single file then fails, and
in a directory run aid prints `Warning: failed to process ...` and goes on.

## 2. Parser per language

Three kinds: tree-sitter through cgo (nine languages), the standard library's
`go/parser` (Go), and regex line parsers (Rust, Swift). The entry point is the
function the package's `ProcessWithOptions` calls.

| Language | Parser actually wired | Grammar import | Reference |
|---|---|---|---|
| python | `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-python/bindings/go` | `references/python.md` |
| typescript | `internal/language/typescript/ast_parser.go#ASTParser.ProcessSource` | `tree-sitter-typescript` (replaced by `internal/parser/grammars/tree-sitter-typescript`) | `references/typescript.md` |
| javascript | `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-javascript/bindings/go` | `references/javascript.md` |
| go | `internal/language/golang/ast_parser.go#ASTParser.ProcessSource` | `go/parser`, `go/ast` (standard library) | `references/go.md` |
| rust | `internal/language/rust/line_parser.go#LineParser.Parse` (regex) | none | `references/rust.md` |
| swift | `internal/language/swift/line_parser.go#LineParser.Parse` (regex) | none | `references/swift.md` |
| ruby | `internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-ruby/bindings/go` | `references/ruby.md` |
| java | `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-java/bindings/go` | `references/java.md` |
| csharp | `internal/language/csharp/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-c-sharp/bindings/go` | `references/csharp.md` |
| kotlin | `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/fwcd/tree-sitter-kotlin/bindings/go` | `references/kotlin.md` |
| cpp | `internal/language/cpp/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-cpp/bindings/go` | `references/cpp.md` |
| php | `internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource` | `github.com/tree-sitter/tree-sitter-php/bindings/go` | `references/php.md` |

All tree-sitter languages run on the binding `github.com/smacker/go-tree-sitter`;
the grammars from `github.com/tree-sitter/...` and `github.com/fwcd/...` are wrapped with
`sitter.NewLanguage(...)`. The Go package is `internal/language/golang/`, and its
processor registers as `go`.

Rust and Swift hold a second parser that is never called (a tree-sitter parser,
kept for the tree-sitter migration). A failed tree-sitter parse returns an
error; no language falls back to another parser. Each reference lists the
unused files under `## Unused files`; fixing a bug there changes nothing.

## 3. Which layer is wrong

Before touching a parser, make sure the parser is the layer that loses or
changes the construct. The method is in the `aid-testing-debugging` skill,
section 5 "Finding the layer of a bug" (`.claude/skills/aid-testing-debugging/SKILL.md`),
with its script `.claude/skills/aid-testing-debugging/scripts/triage.sh`:

```bash
printf 'enum class Color { RED, GREEN }\n' | sh .claude/skills/aid-testing-debugging/scripts/triage.sh kotlin -
```

A construct missing from its section 3 (the IR as jsonl, nothing stripped) is
lost in the parser, which is this skill's subject. A node that is there with the
wrong `visibility` is the parser's visibility rule (the language's reference,
`## Visibility`).

## 4. Reading the tree

For the nine tree-sitter languages, `cmd/ast-dump` prints the tree that the
processor walks, built with the same grammar:

```bash
printf 'class A:\n    def f(self):\n        return 1\n' | go run ./cmd/ast-dump --lang python
go run ./cmd/ast-dump --lang python --all testdata/python/01_basic/source.py | head -40
```

`--lang` takes `python`, `typescript`, `tsx`, `javascript`, `ruby`, `java`,
`csharp`, `kotlin`, `cpp` or `php`. It refuses `go`, `rust` and `swift` (exit
status 2, which `go run` reports as `exit status 2`), because aid does not parse
them with tree-sitter; read their parser instead. A file argument or stdin (no
argument, or `-`) is the input.

How to read a line, here from the first command:

```text
module [0:0-3:0]
  class_definition [0:0-2:16]
    name: identifier [0:6-0:7] "A"
    body: block [1:4-2:16]
```

- Indentation is the depth. `type [row:col-row:col]` is the node type and its
  span; rows and columns count from 0, so row 0 is line 1.
- `name:` and `body:` are field names. A processor reaches such a child with
  `node.ChildByFieldName("name")`; a child shown without a field is reached
  only by position or by its type.
- A node without named children shows its text in quotes, cut after 60 bytes
  (`...`) with line breaks as `\n`.
- Without `--all` only named nodes are printed. `--all` adds the anonymous
  nodes: keywords and punctuation such as `class`, `:`, `async`, `static`,
  `get` or `*`. Processors test those by type to find modifiers (for example
  `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.processMethod`
  checks `get`, `set`, `static`, `async` and `*`), so a missing modifier is
  read with `--all`.
- `ERROR` marks text the grammar could not place, and a zero-width token such as
  `) [0:6-0:6] ""` (seen with `--all`) is one tree-sitter inserted to recover.
  Either means the input or the grammar is at fault, not the processor. A
  `.ts` file with JSX is one case: the TypeScript grammar reads `<div>` as a
  type assertion, the TSX grammar (`--lang tsx`) as `jsx_element`.

Processors dispatch with `switch node.Type()`. A switch whose `default:` recurses
into the children (Java's
`internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.processNode`)
searches through node types it does not list. A switch without such a default
drops every node type it does not list, silently: no error, no warning, no
`-v` line. `internal/language/typescript/ast_parser.go#ASTParser.parseStatement`
and `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.processClassBody`
are of this kind. So when a construct vanishes: dump it, note the type of its
node and of its parent, find the function that switches over the parent's
children, and check whether that type is one of its cases.

## 5. IR conventions

The short version; details and the per-language table are in
`references/ir-conventions.md`.

- A file is an `ir.DistilledFile`; its `Children` are imports, comments and
  declarations. Classes, interfaces, structs, enums and packages hold their
  members in `Children`; functions, fields, imports, comments, type aliases and
  raw content are leaves. Types are in `internal/ir/nodes.go`.
- The parser decides visibility and stores it on every declaration
  (`internal/ir/types.go#Visibility`); the stripper only reads it
  (`internal/stripper/stripper.go#Stripper.shouldRemoveByVisibility`).
- A comment's `Format` (`internal/ir/comment.go`) decides which flag governs it:
  a `doc` comment follows `--docstrings`, every other format `--comments`.
- The stripper copies each kept node whole and then clears only what the
  options require (the body, decorators, removed children). A field you add to
  a node therefore survives stripping without a stripper change; the text
  formatter still has to print it.
- Namespaces, methods outside their type, and the text in `Implementation` are
  encoded differently per language; look the language up in the table in
  `references/ir-conventions.md` before assuming a shape.

## 6. Changing a parser

1. Reproduce with the smallest stdin snippet and confirm the parser layer
   (section 3). Dump the tree (section 4).
2. Write the check that fails first, at the level that states the behaviour:
   a conformance check for a capability (a construct in
   `internal/conformance/testdata/<lang>/` plus a `check` in
   `internal/conformance/fixtures_test.go#fixtures`), or a unit test beside the
   parser in `internal/language/<pkg>/`. Run it and watch it fail for the reason
   you expect:

   ```bash
   go test -count=1 -run 'TestConformance/^kotlin$/' ./internal/conformance
   go test -count=1 ./internal/language/kotlin/
   ```

   The package directory is the language name except Go (`golang`). A snapshot
   cannot be that check: it would record the bug as the expected output.
3. Fix the parser. When you add a case to a closed switch, look for the same
   node type in the other switches of that processor (top level, class body,
   namespace body): each place needs it.
4. Update the snapshots of this one language only and justify every changed
   line (`aid-testing-debugging`, section 3 "Snapshots"). Updating every
   language at once hides an unintended change in another parser.
5. If a `knownGaps` entry of the language now passes, delete it; then run the
   full suite (`aid-testing-debugging`, section 2).

## 7. Adding a language

Follow the checklist in `references/add-language.md`. It also lists the subset
that applies when an existing processor takes a new file extension. The stdin
detector part is in `references/detector.md`.
