# C#

## Parser

- Wired: `internal/language/csharp/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/csharp/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-c-sharp/bindings/go`, wrapped
  with `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang csharp`.
- Extensions: `.cs`. The processor registers as `csharp`.
- Dispatch: `internal/language/csharp/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles the declaration node types and the preprocessor directives, and
  recurses into any other type. The compilation unit and every namespace, class,
  interface and struct body go through
  `internal/language/csharp/tree_sitter_processor.go#TreeSitterProcessor.processStatements`,
  which places comments and drops the doc comments of a statement that yields
  no declaration (`internal/language/commentutil/commentutil.go#AppendStatement`).

## Visibility

Derived from all modifier tokens of a declaration
(`internal/language/csharp/tree_sitter_processor.go#visibilityFromModifiers`):

- `protected internal` is the visibility `protected internal`, `private
  protected` the visibility `private protected`; both are distinct values
  (`internal/ir/types.go#VisibilityProtectedInternal`,
  `internal/ir/types.go#VisibilityPrivateProtected`);
- otherwise `public`, `private`, `protected` and `internal` map to themselves.

Without a modifier:

- a type (class, interface, struct, enum, record, delegate) is `internal`,
  whether it is top-level or nested;
- a member (method, constructor, field, property, event) is `private`;
- an interface method is `public`;
- enum members, operators and the positional parameters of a record are
  `public`.

## Methods and types

- Members are nested in their type. A class is a `DistilledClass`, an interface
  a `DistilledInterface`, a struct a `DistilledStruct`, an enum a
  `DistilledEnum` whose members are `DistilledField`s with the modifiers
  `static` and `const`.
- The first entry of a class's base list goes to `Extends`, the rest to
  `Implements` (the parser cannot tell a base class from an interface).
- A record (`record`, `record struct`) is a `DistilledClass` with the modifier
  `data`; its positional parameters are `public` fields with the modifier
  `readonly`.
- A delegate is a `DistilledTypeAlias`.
- A property is a `DistilledField` with `IsProperty`, `HasGetter` and
  `HasSetter` set (`init` counts as a setter); an expression-bodied
  property (`int P => 1;`) is a `readonly` field whose `DefaultValue` is the
  arrow clause. An event is a `DistilledField`.
- Attributes go to `Decorators`.

## IR specifics

- A block namespace (`namespace N { ... }`) is a `DistilledPackage` whose
  `Children` are its declarations. A file-scoped namespace (`namespace N;`) is a
  `DistilledPackage` without children: the grammar puts the declarations after
  it as its siblings, so they stay at file level.
- `using` directives are `DistilledImport`s with `ImportType` `using`.
- Preprocessor directives (`#region`, `#if`, `#nullable`, ...) are
  `DistilledRawContent` holding the directive line plus `\n`, placed in the body
  where they occur.
- `Implementation` is the method body with its braces (`{ return; }`), or the
  arrow clause of an expression-bodied method (`=> 1`).
- Comments go through `internal/language/commentutil/commentutil.go#StripCStyle`:
  consecutive `///` lines are one `doc` comment whose XML tags stay in the text;
  `/** */` is `doc`, `//` `line`, `/* */` `block`.
- `Extensions` is not used.

## Unused files

None.

## Known gaps

See keys starting with `csharp/` in `internal/conformance/known_gaps_test.go`.
