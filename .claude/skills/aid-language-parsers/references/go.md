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
- Order: the package clause, then `file.Decls` in source order: the imports
  with their comments first
  (`internal/language/golang/ast_parser.go#ASTParser.importNodes`), functions
  through `internal/language/golang/ast_parser.go#ASTParser.processFunction`,
  `type`/`var`/`const` through `internal/language/golang/ast_parser.go#ASTParser.processGenDecl`;
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
  (`type Alias = int`, also `type Pair = struct{...}` on the class and
  `type Any = interface{...}` on the interface) carries the modifier
  `type_alias` and prints with `=`; a defined type (`type Named int`) prints
  without it. The structured formats show the modifier on all three kinds.
- A function whose body contains a `go` statement gets the modifier `async`.
- `--expand` matches a method by its own name, since no type contains it.

## IR specifics

- The package clause is a `DistilledPackage` without children; the stripper
  keeps a childless package node.
- `Implementation` is the function body copied verbatim from the source,
  braces included (`{ ... }`), comments inside it included.
- Comments: the doc comments of functions and types are `doc`. The package doc
  comment and the `@build_constraint(...)` comment built from `//go:build` lines
  (located at the first of them) have an empty `Format`, so they follow `--comments`, not `--docstrings`. A
  package doc line loses `//` and one space, any further indentation stays
  (`internal/language/golang/ast_parser.go#packageDocText`). A `/* */`
  comment loses its markers and the indentation its lines share, a deeper
  one stays (`internal/language/golang/ast_parser.go#blockCommentText`).
  Every other comment (`//` or `/* */`) is `line`:
  - the comments of a struct or interface body are children of their type
    (`internal/language/golang/ast_parser.go#ASTParser.memberNodes`), one node
    per comment group, a field's doc before the field and the comment at the
    end of its line right before it too, inside its line span, so the stripper
    drops both with the field. The text prints the latter at the end of the
    field's line;
  - the comment at the end of a const, var or type spec's line goes the same
    way right before the spec's (last) node
    (`internal/language/golang/ast_parser.go#ASTParser.processGenDecl`); a
    const or var spans from its name to the end of its spec;
  - an import declaration's comments come with its imports
    (`internal/language/golang/ast_parser.go#ASTParser.importNodes`): its
    doc (the group ending on the line directly above `import`, marked
    `Extensions.Go.ImportDoc`, so the text prints it on the import block), the
    groups before each import, the comment at the end of an import's line
    right before it; a group after the last import of a block is dropped. A
    comment set off from `import` by a blank line (a file header) is a free
    comment;
  - any other comment inside a declaration (a function's parameters or body,
    a function literal or function type in a spec) is no node at all
    (`internal/language/golang/ast_parser.go#ASTParser.claimDeclComments`):
    the body in `Implementation` holds it;
  - the rest is inserted at file level by its line number
    (`internal/language/golang/ast_parser.go#ASTParser.insertCommentAtPosition`);
    a one-line comment at the end of a node's last line (a one-line
    function, the package clause) goes right before that node, inside its
    line span, like a spec's.
- `Extensions` is used only for `Go.ImportDoc` on an import declaration's doc.

## Unused files

None.

## Known gaps

See keys starting with `go/` in `internal/conformance/known_gaps_test.go`.
