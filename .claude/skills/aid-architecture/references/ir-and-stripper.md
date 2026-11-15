# The stripper

The IR node types and their fields, the visibility values, the comment formats
and how each language encodes namespaces and methods are in
`.claude/skills/aid-language-parsers/references/ir-conventions.md`. This file
covers what `internal/stripper/stripper.go` does with each option.

## Where it runs

Each language processor runs it at the end of its `ProcessWithOptions`, with
the options from `internal/processor/interface.go#ProcessOptions.ToStripperOptions`,
and only when `internal/stripper/stripper.go#Options.HasAnyOption` is true
(`aid-language-parsers`, section 1). With default flags it is true
(`RemoveImplementations`, `RemoveComments`, `RemovePrivate`). It is false, and
the parser's IR reaches the formatter untouched, with `--private=1
--protected=1 --internal=1 --implementation=1 --comments=1` and every other
flag at its default; `--expand` does not count.

The raw processor (`--raw`) never strips. `internal/processor/processor.go#Processor.processFile`
builds its own `stripper.Options` only on a branch for processors without
`ProcessWithOptions`; `internal/processor/interface.go#LanguageProcessor`
requires that method, so the branch does not run.

## How it copies

`internal/stripper/stripper.go#Stripper.Visit` dispatches on the node type.
Each kept node is copied with `result := *n` and only the fields an option
names are changed; the `Children` of a file, package, class, interface,
struct or enum are rebuilt by
`internal/stripper/stripper.go#Stripper.visitChildList`. The copy is shallow:
slices such as `Parameters` still share their elements with the parser's
node, so the stripper replaces a slice (`Decorators = nil`) and never edits
one in place. `DistilledRawContent` and node kinds without a case are returned
as they are. A node that is removed is not visited, so a removed container
takes all its members with it.

## Option by option

| Stripper option | Set by (individual flags) | Removes |
|---|---|---|
| `RemovePrivate` | public-only visibility (`IncludePrivate` false) | used only when no `Remove*Only` option is set: every declaration that is `private`, `protected`, `internal`, `package`, `fileprivate`, `protected internal` or `private protected`, and one without visibility whose name starts with `_` |
| `RemovePrivateOnly` | `--private=0` with any other visibility combination | `private`, `fileprivate`, `private protected`; a declaration without visibility whose name starts with `_` |
| `RemoveProtectedOnly` | `--protected=0`, same branch | `protected`, `protected internal`, `private protected` |
| `RemoveInternalOnly` | `--internal=0`, same branch | `internal`, `package` |
| `RemovePublic` | `--public=0` | public and open functions, fields and type aliases, except inside a class, interface, struct or enum whose visibility is set and is neither public nor open; then every public container left without members (below) |
| `RemoveImplementations` | `--implementation=0` | a function's `Implementation`, unless `--expand` keeps it |
| `RemoveComments` | `--comments=0` | comments whose format is not doc (`internal/ir/comment.go#DistilledComment.IsDoc`) |
| `RemoveDocstrings` | `--docstrings=0` | doc comments |
| `RemoveImports` | `--imports=0` | `DistilledImport` nodes |
| `RemoveAnnotations` | `--annotations=0` | `Decorators` of functions, classes and fields; parameter decorators stay |
| `RemoveFields` | `--fields=0` | every `DistilledField`, enum constants included |
| `RemoveMethods` | `--methods=0` | every `DistilledFunction`, top-level functions included |
| `ExpandSymbols` | `--expand` | nothing; it keeps bodies (below) |

Visibility is checked by
`internal/stripper/stripper.go#Stripper.shouldRemoveByVisibility` on
functions, fields, type aliases, classes, interfaces, structs and enums.
`public` and `open` are never removed by it. The visibility each language
assigns is the parser's decision (`ir-conventions.md`, section "Visibility").

PHP API docblocks (`Extensions.PHP.IsAPIDocblock`) are kept whatever
`--comments` and `--docstrings` say.

## Containers left without members

- Without `--public=0`, a container whose members were all removed stays:
  `--include-only=public` prints `public class V {` with an empty body, and a
  C# namespace holding only an internal class prints `namespace Outer;`.
- With `--public=0`,
  `internal/stripper/stripper.go#Stripper.publicContainerEmptied` removes a
  class, interface, struct or enum whose visibility is public or open, and a
  package that had children, when no member survived; comments and imports do
  not count as members. A package clause without children (Go, Java) stays.

## `--expand`

`internal/stripper/stripper.go#Options.shouldExpand` matches each pattern with
`filepath.Match` (case-sensitive, `*` does not cross `/`) against a function's
name; a name containing `::` (a C++ out-of-class definition) is also matched by
the part after the last `::`. A function keeps its `Implementation` when its
name matches or when it lies inside a class or struct whose name matches,
nested types included; interfaces and enums do not open such a scope.

`--expand` acts only on `RemoveImplementations`: a matching function that
visibility or `--methods=0` removes stays removed (`--expand _priv` without
`--private=1` prints no `_priv`). Where a language puts methods outside their
type (Go, Rust, Swift, C++, Kotlin; table "Encoded differently per language"
in `ir-conventions.md`), match the method's name.

## Comments next to declarations

`visitChildList` treats the comments before a declaration as a run that
belongs to the next non-comment sibling:

1. If that sibling is a declaration and is removed, the comments directly
   above it go with it, whatever their format and even with `--comments=1`
   (`internal/stripper/stripper.go#commentsAbove`). "Directly above" is read
   from the IR line numbers: the last comment ends on the line before the
   declaration, each earlier one on the line before the next, without a blank
   line; doc comments inside the declaration's lines that the parser put before
   it (a Python docstring) count too. The run stops at a comment without line
   numbers, at an inner doc or PHP API docblock, and at a comment that starts
   on the line where the previous sibling ends.
2. Of the rest, doc comments are kept only when the sibling is a declaration
   that survives, so the doc of a removed member never seems to document the
   next one. A package or namespace counts as a declaration. Other comments
   follow `--comments` alone.
3. Comments after the last sibling: doc comments are dropped, others follow
   `--comments`.
4. Inner docs (`Inner`, such as Rust `//!` or a module docstring) and PHP API
   docblocks follow only their own flag.
5. At the top of a file (`internal/stripper/stripper.go#Stripper.visitFile`),
   comments followed by something that is not a declaration (an import) or by
   nothing are the file's header: doc comments stay unless `--docstrings=0`,
   others follow `--comments`. When a declaration follows, rules 1 to 3 apply.

Seen on a Java class with `--comments=1` and default visibility: a `//`
comment on the line above a private method disappears with the method; a `//`
comment separated from it by a blank line stays; the `/** ... */` above
`package p;` stays because the package clause survives.

## Adding a stripper option

1. A field in `internal/stripper/stripper.go#Options` and a term in
   `Options.HasAnyOption`.
2. Its line in `internal/processor/interface.go#ProcessOptions.ToStripperOptions`
   and a field in `ProcessOptions`.
3. The visit method that applies it. Copy the node and change the copy; return
   `nil` to remove it, and think about the comments above it (rule 1).
4. A unit test in `internal/stripper/stripper_test.go` on an IR built in the
   test, and a conformance check if every language should honour it
   (`aid-testing-debugging`, section 4).
5. The flag: `cli-flags.md`, "Adding a flag".
