# JavaScript

## Parser

- Wired: `internal/language/javascript/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-javascript/bindings/go`, wrapped
  with `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang javascript`.
- Extensions: `.js`, `.mjs`, `.cjs`, `.jsx`.
- Before the walk, `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.collectJSDocComments`
  reads every `/** */` block and keys its tags by the line of the next code
  line; members look their JSDoc up by their own start line.
- Dispatch: `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  recurses into node types it does not list.
  `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.processClassBody`
  and `internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.processExport`
  have no default: unlisted child types yield nothing.

## Visibility

- Everything is `public` unless one of these applies; not being exported does
  not make a declaration less visible.
- A `#name` field or method is `private` and keeps the `#` in its name.
- A JSDoc `@private` tag makes the member `private`, `@protected` makes it
  `protected` (`internal/language/javascript/tree_sitter_processor.go#TreeSitterProcessor.parseJSDoc`).
- A name starting with `_` is `private` by convention: class methods and fields
  (unless their JSDoc says `@public`,
  `internal/language/javascript/tree_sitter_processor.go#isUnderscorePrivate`),
  and top-level functions and variables.

## Methods and types

- Methods and fields are nested in their `DistilledClass`. Getters and setters
  are named `get x` and `set x`, generator methods `*name`; `static` and `async`
  become modifiers.
- A variable whose value is an arrow function, a `function` expression or a
  generator function becomes a `DistilledFunction`; other variables become
  `DistilledField`s. `const` adds the modifier `final`.
- `export class/function/const ...` is parsed as the declaration itself; no
  modifier records the export.

## IR specifics

- No namespace node.
- `Implementation` is the body's `statement_block` text with its braces; an
  arrow function with an expression body keeps the expression.
- Exports that are not declarations (`export * from "m"`, `export { a, b as
  c } from "m"`, `export { a }`, `export default <expression>`) are
  `DistilledExport` nodes, read by
  `internal/language/esutil/esutil.go#Export`, the same function TypeScript
  uses. `module.exports = X` is a comment with format `export`
  (`internal/ir/comment.go#CommentExport`); it is not `doc`, so it follows
  `--comments` and does not appear by default.
- Comments go through `internal/language/commentutil/commentutil.go#StripJSStyle`:
  `/** */` is `doc`, `/* */` `block`, `//` and `///` `line`; a file-level doc with
  a file tag is `Inner`. JSDoc `@param {T} name` and `@returns {T}` also fill
  parameter and return types.
- `Extensions` is not used.

## Unused files

None. When the tree-sitter parse fails, the processor returns the error; there
is no fallback that returns an empty file.

## Known gaps

See keys starting with `javascript/` in `internal/conformance/known_gaps_test.go`.
