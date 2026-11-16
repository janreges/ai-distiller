# PHP

## Parser

- Wired: `internal/language/php/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-php/bindings/go` (v0.25.1 in
  `go.mod`, ABI 15; before v0.24 a `const` in an enum body was a syntax
  error), the `php` grammar (`LanguagePHP()`, which includes the HTML text
  around `<?php`), wrapped with `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang php`.
- Extensions: `.php`, `.phtml`, `.php3`, `.php4`, `.php5`, `.php7`, `.phps`,
  `.inc`.
- Before the walk,
  `internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.collectPHPDocComments`
  reads every `/** */` docblock and keys it by the next code line; declarations
  take `@param`, `@return` and `@var` types from it.
- Dispatch: `internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles the declaration node types and recurses into any other type. An
  `attribute_list` (`#[...]`) is held and attached to the next declaration as
  `Decorators`.

## Visibility

- Members without a modifier are `public`; `public`, `protected` and `private`
  map to themselves. Top-level functions, classes, interfaces, traits and enums
  are `public`.
- A class docblock can change the class's visibility
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.parseDocblockTag`):
  `@internal` makes it `internal`, `@api` makes it `public`.
- Constructor property promotion (`__construct(private int $age)`) adds a field
  with the parameter's visibility
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.processPromotedProperties`).

## Methods and types

- Members are nested in their `DistilledClass` or `DistilledInterface`.
  `extends` goes to `Extends`, `implements` to `Implements`, `use Trait;`
  inside a class to `Mixins`.
- A trait is a `DistilledClass` with `Extensions.PHP.IsTrait`.
- An enum is a `DistilledClass` with the modifier `final` and
  `Extensions.PHP.IsEnum` (backing type in `EnumBackingType`); its header
  fills `Implements` and `Decorators` as a class's does. Its members are its
  children
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.processEnumBody`):
  each `case` is a `static` `final` field with `Extensions.PHP.IsEnumCase` and
  the value as `DefaultValue`, constants and methods as in a class, and
  `use Trait;` goes to `Mixins`.
- Class constants are `static` `final` fields.
- The class docblock is parsed into the class
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.parseClassDocblock`):
  its text becomes `Description`; `@property`, `@property-read` and
  `@property-write` create virtual `public` fields with `Extensions.PHP`
  `Origin` `docblock`, `AccessMode` and `SourceAnnotation`; `@method` creates a
  virtual method (`static` when the tag says so); `@deprecated` fills
  `Deprecated`. When it holds such API tags, the whole docblock is also kept in
  `APIDocblock`, which the formatters print above the class. Without such
  tags it is also emitted as a `doc` comment node ahead of the class
  (`processClass`), so it prints under `--docstrings` like the docblocks of
  members. Docblocks of methods, properties and constants are comment nodes among
  the class children; `PHPFormatter.formatClass` prints them through
  `formatCommentedNodes`, so a docblock is dropped with a member it skips.
  `processPromotedProperties` puts the promoted fields ahead of the constructor's
  docblock.
  Interface methods and enum members get their docblocks the same way
  (`commentPrints` keeps a docblock only when the next declaration is of a
  kind the formatter prints in place: not a trait use, which prints above the
  members).

## IR specifics

- A namespace is not a `DistilledPackage`: `namespace X;` (or `namespace X {
  }`) is a `DistilledImport` with `ImportType` `namespace`, and the
  declarations of its body are its siblings. `--imports=0` removes it.
- A use statement is a `DistilledImport` per clause
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.processUseDeclaration`):
  `ImportType` is `use`, `use function` or `use const`; `Module` is the name
  as written and the one symbol holds that name and the alias
  (`use A\B as C;`). A group `use A\{B, C as D};` is one import whose
  `Module` is the prefix with its trailing backslash (`A\`) and whose symbols
  are the members; a member with its own kind keeps it in the symbol name
  (`function f`). The text formatter prints all of them as written.
- Types are read as written from the grammar's `type` and `return_type`
  fields, whatever their kind (`never` is a `bottom_type`, `(A&B)|null` a
  `disjunctive_normal_form_type`). A `...$x` parameter sets `IsVariadic`, a
  `&$x` (also a promoted `private &$x`) sets `IsReference`, and `function
  &f()` sets `Extensions.PHP.ReturnsReference`
  (`internal/language/php/tree_sitter_processor.go#TreeSitterProcessor.processParameter`).
- `Implementation` is the function body with its braces (`{ return; }`).
- Comments: `/** */` has the legacy format `docblock`, which counts as doc
  (`internal/ir/comment.go#DistilledComment.IsDoc`); `/* */` is `block`, `//`
  and `#` are `line`.
- `Extensions.PHP` is used (traits, enums, docblock members).

## Unused files

None. When the tree-sitter parse fails, the processor returns the error; there
is no fallback that returns an empty file.

Also off the live path: `Processor.ProcessFile` in `processor.go` and
`TreeSitterProcessor.resolveTypeName` in `tree_sitter_processor.go`.

## Known gaps

See keys starting with `php/` in `internal/conformance/known_gaps_test.go`.
