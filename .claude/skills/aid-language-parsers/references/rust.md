# Rust

## Parser

- Wired: `internal/language/rust/processor.go#Processor.ProcessWithOptions`
  creates `NewLineParser(source, filename)` per call and runs
  `internal/language/rust/line_parser.go#LineParser.Parse`.
- Parser: line-based, with regular expressions, no tree-sitter. There is no Rust
  grammar in the repository (`go.mod` holds only a commented-out replace line
  for one), and `ast-dump` does not cover Rust. To see what the parser made of a
  file, use the IR: `--format jsonl`, or `-vvv` (`aid-testing-debugging`,
  section 7).
- Each construct is matched by a regular expression against one line
  (`useRe`, `modRe`, `structRe`, `enumRe`, `traitRe`, `implRe`, `fnRe`,
  `constRe`, `staticRe`, `typeRe` in `line_parser.go`); a line that matches none
  is skipped. Blocks are followed by counting the braces of each line, ignoring
  those inside string and character literals and after `//`
  (`internal/language/rust/line_parser.go#codeBraces`).
- The parser never returns an error: input it does not understand yields fewer
  nodes.
- Extensions: `.rs`.

## Visibility

Decided by `internal/language/rust/line_parser.go#LineParser.parseVisibility`:

- no modifier: `private`;
- `pub(crate)`: `internal`;
- `pub(super)` and `pub(in path)`: `protected`;
- `pub`: `public`.

Methods inside `impl Trait for Type` have no modifier in the source but are
`public`. Methods declared in a trait body are `public`. Enum variants are
`public`.

## Methods and types

- An `impl` block is a `DistilledClass` named `impl Type` or
  `impl Trait for Type`, always `public`, whose children are the methods of the
  block (`internal/language/rust/line_parser.go#LineParser.parseImpl`). Methods
  are not moved into the struct; the struct and each of its `impl` blocks are
  separate nodes.
- `struct` is a `DistilledClass` with the modifier `struct` and its fields as
  children; `enum` a `DistilledClass` with the modifier `enum` and its variants
  as `DistilledField`s; `trait` a `DistilledClass` with the modifier `abstract`,
  whose methods also carry `abstract`.
- `mod name { ... }` is a `DistilledClass` without modifiers whose children are
  the items of the module.
- `const` is a `DistilledField` with the modifier `final`, `static` one with
  `static`, `type X = ...` one with `type_alias`; the value is `DefaultValue`.
- `async` becomes the modifier `async`; a `const fn` gets `final`. A parameter
  list holding `self` keeps it as a parameter (`&self`).

## IR specifics

- `use` is a `DistilledImport` with `ImportType` `use`.
- No `DistilledPackage`: a module is a `DistilledClass` (see above).
- `Implementation` runs from the opening `{` of the body to the matching `}`,
  braces included; a body on one line is kept as written (`{ self.x }`).
- Comments: `///` and `/** */` are `doc`; `//!` and `/*! */` are `doc` with
  `Inner` set (they document the enclosing item); other `//` comments are
  `line`, other `/* */` comments `block`.
- `Extensions` is not used.

## Unused files

- `ast_parser.go`: a tree-sitter parser without a grammar. `NewProcessor`
  stores a `NewASTParser()` whose `parser` field is nil, and nothing calls its
  `ProcessSource`. The `deadcode` tool does not flag it, because its methods are
  exported.

Also off the live path, inside `line_parser.go`: `LineParser.getIndent`.

## Known gaps

See keys starting with `rust/` in `internal/conformance/known_gaps_test.go`.
