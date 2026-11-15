# Kotlin

## Parser

- Wired: `internal/language/kotlin/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/fwcd/tree-sitter-kotlin/bindings/go` (commit `d6af96d2`). Tree:
  `go run ./cmd/ast-dump --lang kotlin`.
- Extensions: `.kt`, `.kts`.
- Dispatch: `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles the declaration node types and recurses into any other type, except
  `primary_constructor` (handled by its class) and `anonymous_initializer` (an
  `init` block is skipped). The grammar has no separate node for interfaces and
  enums: a `class_declaration` with the `interface` or `enum` keyword is passed
  to the interface or enum handler.
- The file and every class, object and interface body go through
  `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.processStatement`,
  which places comments and drops the doc comments of a statement that yields
  no declaration (`internal/language/commentutil/commentutil.go#AppendStatement`).
  An enum body goes through
  `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.processEnumBody`
  instead: entries, functions, properties, a companion object and nested
  classes, objects, interfaces and enums; its comments are not read.
- A comment inside a class, object or interface header (between the name and
  the constructor, the supertypes or the body) is a child of the declaration
  node; it becomes the declaration's first child, before its members.

## Visibility

- No modifier means `public` (the Kotlin default), for types and members.
- `private`, `protected` and `internal` map to themselves.
- A companion object keeps an empty visibility unless a modifier is written.

## Methods and types

- Members are nested in their `DistilledClass`; interfaces are
  `DistilledInterface`, enums `DistilledEnum` whose entries are
  `DistilledField`s with the modifiers `static` and `final` and their arguments
  as `DefaultValue` (`(1)`). An enum's primary constructor becomes a `doc`
  comment `Constructor: (...)` inside the enum.
- Class modifiers such as `data`, `sealed` and `abstract` become modifiers.
- Primary constructor: a function named `constructor` with all its
  parameters, plus a field for each `val`/`var` parameter (`val` adds `final`)
  (`internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.processPrimaryConstructor`).
  A secondary constructor is another function named `constructor`.
- An `object` declaration is a `DistilledClass` with the modifier `static`. A
  `companion object` is a nested `DistilledClass` named `Companion` (or its
  declared name) with the modifier `static`.
- An extension function `fun R.name()` is a function named `R.name` with the
  decorator `@Extension`. It stays where it is declared: at file level, or in
  the class that declares it. The fwcd grammar wraps the receiver in a
  `receiver_type` node, which
  `internal/language/kotlin/tree_sitter_processor.go#TreeSitterProcessor.processFunctionDeclaration`
  reads.
- `typealias` is a `DistilledTypeAlias`.

## IR specifics

- The `package` header is a `DistilledPackage` without children; the
  declarations of the file are its siblings.
- `Implementation` is the function body with its braces (`{ return 1 }`) or,
  for an expression body, the `= expr` text (`= 2`).
- Comments go through `internal/language/commentutil/commentutil.go#StripJSStyle`:
  only `/** */` (KDoc) is `doc`; `//` and `///` are `line`, `/* */` `block`.
- `Extensions` is not used.

## Unused files

None.

## Known gaps

See keys starting with `kotlin/` in `internal/conformance/known_gaps_test.go`.
