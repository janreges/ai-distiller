# IR conventions

The IR is defined in `internal/ir/types.go` (file, visibility, modifiers,
extensions), `internal/ir/nodes.go` (the node types) and `internal/ir/comment.go`
(comment formats). Every processor builds the same node types; what differs per
language is listed in the last section.

## Nodes

Every node embeds `internal/ir/types.go#BaseNode`: `Location` (`StartLine` and
`EndLine` counted from 1, plus columns), `SymbolID` and `Extensions`. The
stripper uses the lines to decide which comments sit directly above a
declaration, so a parser has to fill them.

| Node | Holds children | Main fields |
|---|---|---|
| `DistilledFile` | yes (`Children`, JSON `nodes`) | `Path`, `Language`, `Version`, `Errors` (`DistilledError`: `Message`, `Severity`) |
| `DistilledPackage` | yes | `Name` |
| `DistilledImport` | no | `ImportType` (`import`, `from`, `require`, `use`, `include`, `namespace`, ...), `Module`, `Symbols` (`Name`, `Alias`), `IsType` |
| `DistilledClass` | yes | `Name`, `Visibility`, `Modifiers`, `Decorators`, `TypeParams`, `Extends`, `Implements`, `Mixins`, `Deprecated`, `Description`, `APIDocblock` |
| `DistilledInterface` | yes | `Name`, `Visibility`, `Modifiers`, `TypeParams`, `Extends`, `Permits` |
| `DistilledStruct` | yes | `Name`, `Visibility`, `TypeParams` |
| `DistilledEnum` | yes | `Name`, `Visibility`, `Type` |
| `DistilledTypeAlias` | no | `Name`, `Visibility`, `Modifiers`, `TypeParams`, `Type` |
| `DistilledFunction` | no | `Name`, `Visibility`, `Modifiers`, `Decorators`, `TypeParams`, `Parameters`, `Returns`, `Throws`, `Implementation`, `Deprecated`, `Description` |
| `DistilledField` | no | `Name`, `Visibility`, `Modifiers`, `Type`, `DefaultValue`, `Decorators`, `IsProperty`, `HasGetter`, `HasSetter`, `GetterVisibility`, `SetterVisibility` |
| `DistilledComment` | no | `Text` (without delimiters), `Format`, `Inner` |
| `DistilledRawContent` | no | `Content`, printed as is |

A `Parameter` has `Name`, `Type`, `DefaultValue`, `IsVariadic`, `IsOptional`
and `Decorators`; a `TypeRef` has `Name`, `Package`, `TypeArgs` and nullability
and array flags. Nothing else in the IR has children: a method is a child of
its type, never a child of another function.

Not every language uses every node. Struct, enum and type alias in particular
are encoded differently (Rust and Swift use `DistilledClass` with a modifier,
Go uses `DistilledClass` for a struct); the language's reference says which.

## Visibility

`internal/ir/types.go#Visibility` is set by the parser on every declaration.
The stripper maps the values to the flags
(`internal/stripper/stripper.go#Stripper.shouldRemoveByVisibility`):

| Value | Shown with |
|---|---|
| `public`, `open` | `--public=1` (default) |
| `protected` | `--protected=1` |
| `internal`, `package` | `--internal=1` |
| `private`, `fileprivate` | `--private=1` |
| `protected internal`, `private protected` (C#) | by the rule in `docs/user/COMMAND-LINE-OPTIONS.md` (visibility section) |
| empty | always, unless the name starts with `_` and private members are removed |

A member of a non-public type is removed with its type. With `--public=0`, a
public type whose members are all removed goes too
(`internal/stripper/stripper.go#Stripper.publicContainerEmptied`).

## Modifiers

`internal/ir/types.go#Modifier` values are plain words (`static`, `final`,
`abstract`, `async`, `const`, `readonly`, `virtual`, `data`, `sealed`,
`embedded`, `struct`, `enum`, `type_alias`, `export`, ...). Some processors
reuse a word with their own meaning: Go marks methods with `abstract`, Swift
marks actors with `async`, Kotlin objects and Ruby singleton methods are
`static`, Rust marks `struct`, `union`, `enum`, `extern` (an extern block),
`macro` (a `macro_rules!`) and `type_alias`. Read the language's reference
before relying on a modifier.

## Comments

`Format` is one of the constants in `internal/ir/comment.go`:

- `line` (`internal/ir/comment.go#CommentLine`): `// x`, `# x`;
- `block` (`internal/ir/comment.go#CommentBlock`): `/* x */`, Ruby `=begin`;
- `doc` (`internal/ir/comment.go#CommentDoc`): documentation of the
  declaration that follows it;
- `export` (`internal/ir/comment.go#CommentExport`): JavaScript export lists;
- `docblock`: the legacy format of PHP `/** */` docblocks.
  `internal/ir/comment.go#DistilledComment.IsDoc` is true for `doc` and
  `docblock`.

A doc comment follows `--docstrings`; every other format, an empty one
included, follows `--comments`.

Placement matters as much as the format. A doc comment is a sibling placed
directly before the declaration it documents, in the same `Children` list. The
stripper keeps a run of doc comments only when the next non-comment sibling is
a declaration that survives
(`internal/stripper/stripper.go#Stripper.visitChildList`), and drops the
comments directly above a removed declaration with it. A doc that documents its
container instead (a module docstring, Rust `//!`, a file doc with `@file`)
sets `Inner` and follows only its own flag. Shared helpers for C-style comments
are in `internal/language/commentutil/commentutil.go` (`StripCStyle`,
`StripJSStyle`, `StripDoxygen`, `AppendStatement`, `JoinLineDoc`).

## What the stripper changes

`internal/stripper/stripper.go#Stripper.Visit` copies each node it keeps with
`result := *n`, every field included, and then changes only:

- `Implementation`, cleared with `--implementation=0` unless `--expand` matches
  the function or an enclosing type;
- `Decorators`, cleared with `--annotations=0`;
- `Children`, filtered by the same rules;
- whole nodes, removed by visibility, `--fields=0`, `--methods=0`,
  `--imports=0`, `--comments=0` and `--docstrings=0`.

A field added to a node therefore survives stripping without a stripper
change, but a formatter still has to print it. `DistilledRawContent` is never
removed. A childless `DistilledPackage` (a package clause) is kept. A namespace
whose members were all removed is dropped only when public members are removed
(`--public=0`); comments and imports do not count as members
(`internal/stripper/stripper.go#Stripper.visitPackage`,
`internal/stripper/stripper.go#Stripper.publicContainerEmptied`). With default
flags it stays: a C# namespace holding only an internal class prints
`namespace N;`.

`Extensions` carries language-specific data (`internal/ir/types.go#NodeExtensions`).
On the live path only Java (`Extensions.Java`), PHP (`Extensions.PHP`) and
Rust (`Extensions.Rust`: a tuple struct's arity and a header's `where` clause)
set it.

## Encoded differently per language

| Language | Namespace or module | Methods outside their type | `Implementation` text |
|---|---|---|---|
| python | none: the file is the module | none: methods are children of the class | body after the docstring, without the `def` line |
| typescript | `DistilledPackage` with children (`namespace`, `module`, `declare module`) | none | body with braces |
| javascript | none | none | body with braces; an arrow function's expression body as is |
| go | `DistilledPackage` without children (package clause) | every method: file level, receiver as first parameter, modifier `abstract` | body with braces, verbatim |
| rust | `mod` is a `DistilledClass` with children | in `impl T` / `impl Trait for T` classes, separate from the struct | from `{` to `}`, braces included |
| swift | none | in `extension X` classes, separate from `X` | body with braces |
| ruby | a module is a `DistilledInterface` | a reopened class is a second class node | body without `def` and `end` |
| java | `DistilledPackage` without children | none | body without its outer braces, trimmed |
| csharp | block namespace: `DistilledPackage` with children; file-scoped: without children | none | body with braces, or `=> expr` |
| kotlin | `DistilledPackage` without children | extension functions `R.name`, where declared | body with braces, or `= expr` |
| cpp | `DistilledPackage` with children (`anonymous` when unnamed) | out-of-class definitions `C::name`, at file or namespace level | body with braces; `= default`, `= delete` |
| php | `DistilledImport` with `ImportType` `namespace`; declarations are siblings | none | body with braces |

A formatter or a feature that walks the IR (for example `--expand`, which
matches a function by its own name and a type by its name) has to handle each
row, not only the nested shape.
