# TypeScript

## Parser

- Wired: `internal/language/typescript/processor.go#Processor.ProcessWithOptions`
  calls `Processor.Process`, which creates `NewASTParser()` per call and runs
  `internal/language/typescript/ast_parser.go#ASTParser.ProcessSource`.
- Grammar: Go module `tree-sitter-typescript`, replaced in `go.mod` by the
  wrapper in `internal/parser/grammars/tree-sitter-typescript`, whose C sources
  are the git submodule `internal/parser/grammars/tree-sitter-typescript/source`
  (`references/tree-sitter-cgo.md`).
- Two grammars: a file name ending in `.tsx` selects `LanguageTSX()`, any other
  `Language()` (`internal/language/typescript/processor.go#Processor.Process`).
  On stdin, `--lang tsx`, or detected TypeScript that contains JSX
  (`internal/language/detector.go#HasJSX`), passes the name `stdin.tsx`. Tree:
  `go run ./cmd/ast-dump --lang typescript` or `--lang tsx`; the TypeScript
  grammar reads JSX as a type assertion.
- Extensions: `.ts`, `.tsx` (a `.d.ts` file arrives as `.ts`).
- Dispatch: `internal/language/typescript/ast_parser.go#ASTParser.parseStatement`
  is shared by the program, `export` statements and namespace bodies and has no
  default: a statement of any unlisted type yields nothing. Class bodies:
  `internal/language/typescript/ast_parser.go#ASTParser.parseClassBody`;
  interface bodies:
  `internal/language/typescript/ast_parser.go#ASTParser.parseInterfaceBody`.

## Visibility

- Top-level declarations: exported is `public`, not exported is `internal`
  (`internal/language/typescript/ast_parser.go#ASTParser.getVisibility`). A
  non-exported function or variable whose name starts with `_` is `private`.
- Class members default to `public`; `private`, `protected` and `public`
  accessibility modifiers set it; a `#name` member is `private` and keeps the
  `#` in its name. Constructor parameter properties (`constructor(private x: T)`,
  `constructor(readonly y: T)`) become fields with the parameter's modifier and
  `readonly`
  (`internal/language/typescript/ast_parser.go#ASTParser.parseParameterProperties`).
  A parameter's `Name` is its pattern as written, so a destructuring pattern
  (`{ a, b = 1 }`) is the name; `...` sets `IsVariadic`, `?` `IsOptional`
  (`internal/language/typescript/ast_parser.go#ASTParser.parseParameters`).
- Interface members and enum members are `public`.
- Inside a namespace, members follow the same export rule (`export` is
  `public`, the rest `internal`). Inside an ambient declaration (`declare
  namespace`, `declare module "x"` and namespaces nested in them) every member
  is `public` (`internal/language/typescript/ast_parser.go#makePublic`), without
  an `export` modifier being added.

## Methods and types

- Methods are nested in their `DistilledClass`; interfaces are
  `DistilledInterface` with their members as children; enums are
  `DistilledEnum`; `type X = ...` is a `DistilledTypeAlias`.
- `parseClassBody` reorders the members: fields first, then the constructor,
  then the methods, then trailing comments.
- Accessors are named `get x` and `set x`, generators `*name` (a generator
  function declaration too), as in JavaScript
  (`internal/language/typescript/ast_parser.go#ASTParser.accessorName`); the
  formatter prints `function* name` for a generator function. A type
  parameter's default is `TypeParam.Default`.
- An exported declaration carries the modifier `export`, and `default` after
  `export default`; `export default class {}` is a `DistilledClass` without a
  name. Every other export statement (a re-export, an export list, `export
  default <expression>`, `export = x`) is a `DistilledExport`
  (`internal/language/esutil/esutil.go#Export`). A declaration that an export
  list or `export default <name>` of the same file or namespace names is
  `public`, without an `export` modifier
  (`internal/language/typescript/ast_parser.go#publishExportedNames`). A variable whose value
  is an arrow function and that has no type annotation becomes a
  `DistilledFunction` with the arrow's type parameters; any other variable is a
  `DistilledField` (`const` adds the modifier `final`). When the field's value
  is a function expression, a typed arrow function or a call that takes a
  function (`forwardRef((props, ref) => ...)`, `new Pool(() => ...)`,
  `internal/language/esutil/esutil.go#TakesFunction`), `DefaultValue` is its
  text with the function bodies left out
  (`internal/language/esutil/esutil.go#Signature`) and `Implementation` the
  whole initializer, which the stripper keeps only with `--implementation=1`
  (`internal/language/typescript/ast_parser.go#ASTParser.parseVariableDeclaration`).
- Structural interface satisfaction
  (`internal/language/typescript/ast_parser.go#ASTParser.analyzeInterfaceSatisfaction`)
  is not called; only an explicit `implements` clause fills `Implements`.

## IR specifics

- Namespaces: `namespace X {}`, `module X {}` and `declare module "x" {}` are a
  `DistilledPackage` whose `Children` are the declarations of the body
  (`internal/language/typescript/ast_parser.go#ASTParser.parseNamespace`).
- `Implementation` is the body's `statement_block` text with its braces
  (`{ return; }`).
- Comments go through `internal/language/commentutil/commentutil.go#StripJSStyle`:
  `/** */` is `doc`, `/* */` `block`, `//` and `///` `line`. A file-level doc that
  carries a file tag (`@file`, `@module`, `@packageDocumentation`, ...) is `Inner`.
  The doc comments of a statement that yields no declaration are dropped
  (`internal/language/commentutil/commentutil.go#AppendStatement`).
- `Extensions` is not used.

## Unused files

Off the live path: `ASTParser.analyzeInterfaceSatisfaction`,
`ASTParser.classImplementsInterface` and `ASTParser.typesCompatible` in
`ast_parser.go`, and `Processor.ProcessFile` in `processor.go`.

## Known gaps

See keys starting with `typescript/` in `internal/conformance/known_gaps_test.go`.
