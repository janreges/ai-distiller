# Swift

## Parser

- Wired: `internal/language/swift/processor.go#Processor.ProcessWithOptions`
  creates `NewLineParser(source, filename)` per call and runs
  `internal/language/swift/line_parser.go#LineParser.Parse`.
- Parser: line-based, with regular expressions, no tree-sitter. The tree-sitter
  path is commented out in `ProcessWithOptions`, and `ast-dump` does not cover
  Swift. To see what the parser made of a file, use the IR: `--format jsonl`, or
  `-vvv` (`aid-testing-debugging`, section 7).
- Each line is tried against the regular expressions built in
  `internal/language/swift/line_parser.go#NewLineParser`, in this order:
  import, class, struct, protocol, extension, enum, actor; inside an enum only
  `case`, inside a protocol only requirement methods and properties; otherwise
  function, property, typealias. A line that matches none is skipped.
- A function body is collected by counting braces
  (`internal/language/swift/line_parser.go#LineParser.functionBody`); for every
  other construct the parser only tracks the brace level, and when it returns to
  0 the current type is closed.
- The parser never returns an error.
- Extensions: `.swift`.

## Visibility

Decided by `internal/language/swift/line_parser.go#LineParser.parseVisibility`:

- no modifier: `internal` (the Swift default);
- `public` and `open`: `public`;
- `private` and `fileprivate`: `private`.

Protocol requirements are `public`. Swift produces no `protected`.

## Methods and types

- The parser keeps a single current type, not a stack. Members are added to the
  type that was opened last
  (`internal/language/swift/line_parser.go#LineParser.addToParent`): a type
  declared inside another type becomes a top-level node, and the members that
  follow it in the outer type are attached to the inner one.
- `class` is a `DistilledClass` (`final` adds the modifier `final`); `struct` a
  `DistilledClass` with the modifier `struct` (the formatter prints `struct`;
  an extension, whose name starts with `extension `, prints with no keyword); `actor` a `DistilledClass` with the
  modifier `async`; `protocol` a `DistilledInterface` whose requirements carry
  the modifier `abstract`; `enum` a `DistilledEnum` whose cases are
  `DistilledField`s; `typealias` a `DistilledTypeAlias`.
- Everything after the `:` of a type declaration goes to `Implements`,
  superclass included
  (`internal/language/swift/line_parser.go#LineParser.parseInheritance`).
- `extension X` and `extension X: P` are separate `DistilledClass` nodes named
  `extension X` (the conformances are not in the name), with the
  conformances in `Implements` and the extension's members as children. They
  are not merged into `X`.

## IR specifics

- No namespace node.
- `Implementation` is the function body with its braces, each line's trailing
  whitespace trimmed.
- Comments: `///` lines and `/** */` blocks are `doc`; `// MARK:` lines are
  `line` with the text after `MARK:`; other `//` lines are `line` and `/* */`
  blocks (several lines make one node) are `block`. Ordinary comments are kept
  only at member level (in the file or directly in a type), not inside function
  bodies (`parserState.atMemberLevel`). The Swift formatter prints every kind as
  `//` lines.
- A property's `DefaultValue` holds the value without the `=` (`0`); the
  formatter adds ` = `.
- `Extensions` is not used.

## Unused files

- `tree_sitter_processor.go` (kept on purpose for the tree-sitter migration,
  roadmap decision D3, and marked with a comment at the top): its only caller in `processor.go` is commented
  out. It is still compiled, and with it the vendored grammar in
  `internal/parser/grammars/tree-sitter-swift`, which nothing else imports.

## Known gaps

See keys starting with `swift/` in `internal/conformance/known_gaps_test.go`.
