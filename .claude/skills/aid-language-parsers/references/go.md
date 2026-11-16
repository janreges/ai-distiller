# Go

## Parser

- Wired: `internal/language/golang/processor.go#Processor.ProcessWithOptions`
  calls `Processor.Process`, which creates `NewASTParser()` per call and runs
  `internal/language/golang/ast_parser.go#ASTParser.ProcessSource`.
- Parser: the standard library's `go/parser` (`parser.ParseFile` with
  `parser.ParseComments`) and `go/ast`. No tree-sitter and no cgo for this
  package, but the `aid` binary as a whole still needs cgo.
- The package directory is `internal/language/golang/`; the processor registers
  as `go`.
- Tree: `ast-dump` does not cover Go. The parsed AST is printed by
  `go run ./cmd/aid <file.go> --stdout -vvv` as the dump block `Raw Go AST`
  (`aid-testing-debugging`, section 7).
- A syntax error is an error: `ParseFile` fails and the file produces no IR.
- Extensions: `.go`.
- Order: the package clause, then the imports, then `file.Decls` in source order
  (functions through `internal/language/golang/ast_parser.go#ASTParser.processFunction`,
  `type`/`var`/`const` through `internal/language/golang/ast_parser.go#ASTParser.processGenDecl`),
  then the comments that are not doc comments are inserted by line
  (`internal/language/golang/ast_parser.go#ASTParser.processComments`).

## Visibility

- A name starting with an upper-case ASCII letter is `public`, any other name is
  `internal` (`internal/language/golang/ast_parser.go#ASTParser.getVisibility`).
  Go produces no `private`: `--internal=1` shows unexported names, `--private=1`
  adds nothing.
- A method is as visible as the less visible of its own name and its receiver
  type, with `*` and type arguments removed (`func (q *sq) Area()` is `internal`
  because `sq` is).
- Named struct fields follow the same name rule. An embedded field is `public`,
  named after its type (`io.Reader`) and carries the modifier `embedded`.
  Interface methods are `public`.

## Methods and types

- Methods stay at file level in source order. They are not moved into their
  type. The receiver is the first entry of `Parameters` (named as in the source,
  `receiver` when unnamed, type as written, e.g. `*sq`), and the modifier
  `abstract` marks the function as a method.
- A `struct` type is a `DistilledClass` whose children are its fields; an
  `interface` type is a `DistilledInterface` whose children are its methods.
  Each embedded element of an interface is one `Extends` entry as written: an
  embedded interface (`io.Reader`) or a whole type-set union
  (`~int | ~string`). The text prints them one per line before the methods.
- Any other type declaration is a `DistilledTypeAlias`. Only a real alias
  (`type Alias = int`) carries the modifier `type_alias` and prints with `=`;
  a defined type (`type Named int`) prints without it.
- A function whose body contains a `go` statement gets the modifier `async`.
- `--expand` matches a method by its own name, since no type contains it.

## IR specifics

- The package clause is a `DistilledPackage` without children; the stripper
  keeps a childless package node.
- `Implementation` is the function body copied verbatim from the source,
  braces included (`{ ... }`), comments inside it included.
- Comments: the doc comments of functions and types are `doc`. The package doc
  comment and the `@build_constraint(...)` comment built from `//go:build` lines
  have an empty `Format`, so they follow `--comments`, not `--docstrings`. Every
  other comment (`//` or `/* */`) is `line` and is inserted at file level by its
  line number, also when it sits inside a function body, except the comments
  of a struct or interface body: they are children of their type
  (`internal/language/golang/ast_parser.go#ASTParser.memberNodes`), one node
  per comment group, a field's doc before the field and the comment at the end
  of its line right before it too, inside its line span, so the stripper drops
  both with the field. The text prints the latter at the end of the field's line.
- `Extensions` is not used.

## Unused files

None.

## Known gaps

See keys starting with `go/` in `internal/conformance/known_gaps_test.go`.
