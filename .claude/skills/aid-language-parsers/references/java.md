# Java

## Parser

- Wired: `internal/language/java/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-java/bindings/go`, wrapped with
  `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang java`.
- Extensions: `.java`.
- Dispatch: `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles the declaration node types and recurses into any other type, with two
  exceptions: `static_initializer` and `block` are skipped (nothing in an
  initializer is a member), and an `ERROR` node is recorded in `file.Errors`
  without recursing into it.
- The program and every class, interface, enum and annotation body go through
  `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.processStatement`,
  which places comments where they are and drops the doc comments of a
  statement that yields no declaration
  (`internal/language/commentutil/commentutil.go#AppendStatement`).

## Visibility

- No modifier means package-private: the visibility `package`
  (`internal/ir/types.go#VisibilityPackage`). The stripper treats it like
  `internal`, so it is shown with `--internal=1`.
- `public`, `protected` and `private` map to themselves.
- Interface methods without a modifier are `public`; annotation elements and
  enum constants are `public`.
- Top-level and nested types follow the same rule (`class A {}` without a
  modifier is `package`).

## Methods and types

- Methods, constructors and fields are nested in their `DistilledClass`,
  `DistilledInterface` or `DistilledEnum`. A constructor is a
  `DistilledFunction` named after the class, without `Returns`.
- `extends` goes to `Extends`, `implements` to `Implements`. The `permits`
  clause of a sealed interface goes to `Permits`
  (`internal/ir/nodes.go#DistilledInterface`); a class has no such field and its
  `permits` clause is not recorded.
- Enum constants are `DistilledField`s with the modifiers `static` and `final`
  and the enum as their type.
- A record is a `DistilledClass` with `Extensions.Java.IsRecord` set; its
  components are `Extensions.Java.RecordParameters`, not fields
  (`internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.processRecordDeclaration`).
- An annotation type (`@interface`) is a `DistilledClass` whose `Decorators`
  are `["@interface"]`; its elements are functions with
  `Extensions.Java.IsAnnotationElement` and the `default` value in
  `Extensions.Java.DefaultValue`.
- Annotations on a declaration go to `Decorators`; `throws` goes to `Throws`.

## IR specifics

- The `package` declaration is a `DistilledPackage` without children; the
  declarations of the file are its siblings.
- Imports: `ImportType` `import`, or `static import` for `import static`.
- `Implementation` is the method body without its outer braces, trimmed
  (`return;`).
- Comments go through `internal/language/commentutil/commentutil.go#StripCStyle`:
  `/** */` is a `doc` comment, cleaned of its `*` gutter by
  `internal/language/java/tree_sitter_processor.go#TreeSitterProcessor.parseJavaDoc`;
  consecutive `///` lines are one `doc` comment; `//` is `line`, `/* */`
  `block`.
- `Extensions.Java` is used (records and annotation elements, above).

## Unused files

None.

## Known gaps

See keys starting with `java/` in `internal/conformance/known_gaps_test.go`.
