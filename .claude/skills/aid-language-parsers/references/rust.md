# Rust

## Parser

- Wired: `internal/language/rust/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/rust/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-rust/bindings/go` v0.24.2 (ABI 15).
  Tree: `go run ./cmd/ast-dump --lang rust`.
- Extensions: `.rs`.
- Dispatch: `internal/language/rust/tree_sitter_processor.go#walker.items`
  reads the items of the source file and of every inline `mod`, `impl` and
  `trait` body, and hands each to
  `internal/language/rust/tree_sitter_processor.go#walker.item`. It never
  descends into function bodies (`block`), `macro_rules!` bodies or the values
  of `const` and `static` items: items written there are not distilled.
- Items inside an item-level macro call written with braces (`s! { ... }`,
  `cfg_if! { ... }`, `lazy_static! { ... }`) are distilled where the call
  stands, with their true lines
  (`internal/language/rust/tree_sitter_processor.go#walker.expand`). Only a
  body whose first token after its comments starts an item (an item keyword,
  `#`, the `if` of `cfg_if!`, or a name or path followed by `!`) is read
  (`internal/language/rust/tree_sitter_processor.go#walker.startsLikeItems`);
  so is a body of comments only; any other body (`select!` arms, tables, a
  language of the macro's own) yields no IR. The text between the braces is
  parsed on its own with a fresh parser, under a budget of progress checks
  (`internal/language/tsparse/tsparse.go#ParseBudget`, sized by
  `internal/language/rust/tree_sitter_processor.go#parseBudget`, at most
  1,648 checks, the parser's work on about 75 KiB of the densest code or
  300 KiB of typical code): the parser's error recovery
  takes time quadratic in the length of a run of text that is not items, and
  a body over its budget yields no IR. The items
  it recognises at its top level are walked, also when the parse has errors
  (an `ERROR` inside an item, such as Rust 2024 `safe fn` or libc's `{const}
  fn`, keeps the item). An item whose name the parser read across an error
  (an `ERROR` or missing node between its keyword and its name, as in
  bitflags' `pub struct Mode: libc::mode_t` or paste's `pub fn [<get_
  $name>]`) yields no IR, nor do the docs above it
  (`internal/language/rust/tree_sitter_processor.go#walker.misnamed`); an
  error after the name (syn's `pub struct ExprArray #full { .. }`) keeps it.
  Which side of the name the error lands on is the parser's choice: in
  `pub struct X #full { .. }` it takes `full` for the name, so that item is
  skipped. A top-level `ERROR` node, a top-level expression statement with an
  error (the `if #[cfg(..)] { .. } else { .. }` of `cfg_if!`) and a root that
  is itself an `ERROR` stand for the `{}` blocks of the call that lie entirely
  inside them, which are expanded the same way in place
  (`internal/language/rust/tree_sitter_processor.go#walker.expandBlocksIn`).
  A `{}` block outside them, and the first `{}` block after `fn`, `impl`,
  `struct`, `enum`, `union`, `trait` or `mod`, are never read: no function
  body and no member of an item the parser could not read reaches the
  enclosing list. Other `ERROR` nodes of a macro body yield no IR. Nesting
  stops at
  `internal/language/rust/tree_sitter_processor.go#maxMacroDepth` levels.
  Memory grows with nesting: each nested body is parsed while the trees of
  the bodies around it stay alive (real crates nest 1 to 3 levels, and the
  budget bounds the tree of each body).
  Calls with `()` or `[]` and macro calls inside bodies are not read.
- Statements that yield no IR: the macro call itself, `macro_rules!`, `union`,
  `extern` blocks, `extern crate`, inner attributes (`#![...]`) and the `const`
  items of a trait. The outer doc comments right before them are dropped, so
  they do not document the next item; inner docs (`//!`) and plain comments
  stay (`internal/language/rust/tree_sitter_processor.go#appendStatement`).
  Outer attributes (`#[...]`) are skipped, and the docs above them stay with
  the item after them.
- A syntax error becomes an `ERROR` node; one among items is read as items of
  the same list, so the items around a broken region still print. The parse
  fails only when its context is cancelled.

## Visibility

Decided by `internal/language/rust/tree_sitter_processor.go#walker.visibility`:

- no modifier, and the 2015 `crate`: `private`;
- `pub(crate)`: `internal`;
- `pub(super)` and `pub(in path)`: `protected` (the formatter prints both as
  `pub(super)`);
- `pub`: `public`, and so is `pub(self)`, which Rust treats as private.

Methods inside `impl Trait for Type` have no modifier in the source but are
`public`. Methods and associated types declared in a trait body are `public`.
Enum variants are `public`. Other members of a trait impl (`type`, `const`)
follow the rules above, so they are `private`.

## Methods and types

- An `impl` block is a `DistilledClass` named after its header with white space
  collapsed: `impl Type`, `impl<T: Clone> Trait for Type<T>`, `impl !Send for
  Type`. It is always `public`, and its children are the items of the block
  (`internal/language/rust/tree_sitter_processor.go#walker.implItem`). The
  header's `where` clause is not kept. Methods are not moved into the struct;
  the struct and each of its `impl` blocks are separate nodes.
- `struct` is a `DistilledClass` with the modifier `struct` and its named fields
  as children; tuple-struct fields are not distilled. `enum` is a
  `DistilledClass` with the modifier `enum` and its variants as public
  `DistilledField`s: the payload as written (`(f64)`, `{ w: f64, h: f64 }`, white
  space collapsed, comments removed) is the type, the discriminant the
  `DefaultValue`. `trait` is a `DistilledClass` with the modifier `abstract`
  whose children are its methods, associated types and comments.
- A function without a body (a required trait method) carries `abstract`; the
  formatter ends it with `;`.
- `mod name { ... }` is a `DistilledClass` without modifiers whose children are
  all the items of the module
  (`internal/language/rust/tree_sitter_processor.go#walker.modItem`); `mod
  name;` has none. The formatter prints only the fields and functions of a
  module, with the comments among them.
- `const` is a `DistilledField` with the modifier `final`, `static` one with
  `static`; the value is `DefaultValue` when it fits on one source line and
  `...` when it spans lines. A type alias (`type X<T> = ...;`) and an associated
  type (`type Item: Bound;`) are `DistilledField`s with the modifier
  `type_alias`.
- Generics stay in `Name` as written (`longest<'a, T>`); `TypeParams` is empty.
  `Returns` holds the return type followed by the `where` clause
  (`bool where T: Clone,`).
- `async` becomes the modifier `async`; a `const fn` gets `final`; `unsafe`,
  `extern "ABI"` and `default` are not represented. A self parameter keeps its
  whole text as its name (`&mut self`, `mut self`) and has no type; any other
  parameter's name is its pattern (`mut x`, `(a, b)`).

## IR specifics

- `use` is a `DistilledImport` with `ImportType` `use`
  (`internal/language/rust/tree_sitter_processor.go#walker.useDeclaration`).
  `use a::{b, c as d};` gives `Module` `a` and the symbols `b` and `c` (alias
  `d`), `self` included; `use {a::B, c::D};` gives an empty `Module` and the
  symbols; any other tree is the `Module` (`std::fmt::Display as Disp`,
  `super::*`). `pub use` is not told apart from `use`.
- No `DistilledPackage`: a module is a `DistilledClass` (see above).
- `Implementation` is the source text of the body, from its `{` to its `}`.
- Comments: one `DistilledComment` per comment node, in the file and in `mod`,
  `impl` and `trait` bodies; comments inside struct and enum bodies are not
  read (`internal/language/rust/tree_sitter_processor.go#walker.comment`). A
  comment that starts on the line where the previous item, attribute or brace
  ends (`pub const X: u8 = 1; // note`) yields none.
  `///` and `/** */` are `doc`; `//!` and `/*! */` are `doc` with `Inner` set
  (they document the enclosing item); other `//` comments (also `////`) are
  `line`, other `/* */` comments `block`. The text loses its delimiters and the
  doc marker; a block comment keeps its ` * ` gutters.
- Multi-line types, parameters, return types, `where` clauses and `use` trees
  are normalised: runs of white space become one space, there is no space
  after `(`, `[` and `{` nor before `)`, `]` and `}`, and no comma before a
  closing bracket
  (`internal/language/rust/tree_sitter_processor.go#normalize`); a `where`
  clause keeps its trailing comma.
- `Extensions` is not used.

## Unused files

None.

## Known gaps

See keys starting with `rust/` in `internal/conformance/known_gaps_test.go`.
