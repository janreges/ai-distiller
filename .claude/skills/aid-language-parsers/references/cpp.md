# C++

## Parser

- Wired: `internal/language/cpp/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/cpp/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-cpp/bindings/go`, wrapped with
  `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang cpp`.
- Extensions: `.cpp`, `.cc`, `.cxx`, `.c++`, `.h`, `.hpp`, `.hh`, `.hxx`,
  `.h++`. A `.h` file is parsed as C++. A `.c` file has no processor (`no
  processor found for file`), and the detector has no C language.
- The processor registers as `cpp`.
- Dispatch: `internal/language/cpp/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles the declaration node types and recurses into any other type. The
  file, `#if`/`#ifdef` branches (include guards), `extern "C"` blocks and
  namespace bodies are walked as statements of the enclosing scope. A
  `friend_declaration` is skipped, and so are `requires` clauses.
- Two passes: after the walk,
  `internal/language/cpp/tree_sitter_processor.go#TreeSitterProcessor.indexMembers`
  records the visibility of every member declared in a class body, and
  `internal/language/cpp/tree_sitter_processor.go#inheritOutOfClassVisibility`
  gives each out-of-class definition (below) that visibility.

## Visibility

- In a `class`, members are `private` until an access specifier; in a `struct`
  or `union` they are `public` until one. `public:`, `protected:` and
  `private:` switch the section.
- File-level and namespace-level declarations are `public`; so are enum
  entries.
- An out-of-class definition `void W::open() {}` takes the visibility of the
  in-class declaration of `W::open` (the most open one when overloads give
  several, and never more open than an enclosing nested class allows).

## Methods and types

- `class` is a `DistilledClass`; `struct` and `union` are `DistilledStruct`;
  `enum` and `enum class` are `DistilledEnum` with `DistilledField` entries.
- In-class declarations and inline definitions are children of their type. An
  out-of-class definition is a separate function named with its qualifier
  (`W::open`), placed where it is defined (file or namespace level), not moved
  into the class.
- All base classes go to `Extends`; their access specifiers are not kept.
- `virtual`, `static` and a method's `const` become modifiers; a pure virtual
  method (`= 0`) also gets `abstract`.
- A template declaration gives its function or class `TypeParams`. A C++20
  `concept` becomes a `doc` comment `C++20 Concept: ...`.

## IR specifics

- A `namespace` is a `DistilledPackage` whose `Children` are its
  declarations; an unnamed namespace is named `anonymous`.
- `#include` is a `DistilledImport` with `ImportType` `include` and the header
  name as `Module`; `using std::x;` one with `ImportType` `using`.
- `Implementation` is the function body with its braces (`{ return; }`);
  `= default` and `= delete` are kept as the implementation text.
- Comments go through `internal/language/commentutil/commentutil.go#StripDoxygen`:
  `/** */`, `/*! */`, `///` and `//!` are `doc` (a run of `///` or `//!` lines
  is one doc), other comments `line` or `block`. A trailing member doc
  (`int x; ///< the x`) is moved in front of the member it documents. A
  file-level doc with `@file` or `\file` is `Inner`.
- `Extensions` is not used.

## Unused files

None.

Also off the live path, inside `tree_sitter_processor.go`:
`TreeSitterProcessor.processStructDeclaration` and
`TreeSitterProcessor.processStructMethodDeclaration`.

## Known gaps

See keys starting with `cpp/` in `internal/conformance/known_gaps_test.go`.
