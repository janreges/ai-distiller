# Python

## Parser

- Wired: `internal/language/python/processor.go#Processor.ProcessWithOptions`
  creates a parser per call with `NewNativeTreeSitterProcessor()` and runs
  `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-python/bindings/go` (official, v0.23.6). Tree: `go run ./cmd/ast-dump --lang python`.
- Extensions: `.py`, `.pyw`, `.pyi`.
- Dispatch: `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.processNode`
  recurses into node types it does not list.
  `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.processClassBody`
  has no default: a class body child whose type is not one of its cases
  (function and decorated definitions, expression statements, assignments)
  yields nothing.
- After the walk,
  `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.analyzeProtocolSatisfaction`
  runs over the top-level classes (see `## Methods and types`).

## Visibility

Decided by `internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.isPrivateName`,
for classes, functions, methods and fields alike:

- a name starting with `_` is `private`, including `__name` (name mangling);
- a dunder name, starting and ending with `__` (`__init__`, `__repr__`), is
  `public`;
- every other name is `public`.

Python produces no `protected` or `internal`, so `--protected=1` and
`--internal=1` change nothing for Python.

## Methods and types

- Methods and nested classes are `Children` of their `DistilledClass`. Base
  classes go to `Extends`; decorators, as written but without the `@`, go to
  `Decorators` of the function or class.
- Inside a method body, an assignment to an attribute (`self.x = ...`, in fact
  any `obj.x = ...`) becomes a `DistilledField` of the enclosing class, named by
  the attribute (`x`), with the right-hand side as `DefaultValue`
  (`internal/language/python/native_tree_sitter_processor.go#NativeTreeSitterProcessor.processAssignmentWithClassContext`).
  These fields are placed before the method that assigns them. A plain local
  assignment (`x = 1`) in a body produces nothing.
- Protocol duck typing: a top-level class whose `Extends` contains `Protocol` is
  a protocol. Every other top-level class that has each of its methods as a
  public method, with the same number of parameters besides `self`/`cls` and the
  same type names where both sides are annotated, gets the `Implements` entry
  `<Protocol> (duck typing)`. The jsonl output shows it
  (`"implements":["Shape (duck typing)"]`); the text output does not print
  `Implements` for Python.

## IR specifics

- No namespace node: a module is the file.
- `Implementation` is the function body after the docstring: from the first
  statement to the end of the block, without the `def` line, every line after
  the first keeping its source indentation. A body that holds only the docstring
  has an empty `Implementation` and `DocOnlyBody` set, so it prints with the
  `def name():` notation of a function with a body.
- Comments: `#` comments are `line`. A docstring is a `doc` comment: module and
  class docstrings are `Inner` (they document their container); a function's
  docstring is placed directly before the function and is not part of its
  `Implementation`. A bare string after the first statement of a module or class
  is a `block` comment.
- `Extensions` is not used.

## Unused files

None. When the tree-sitter parse fails, `Processor.ProcessFile` and
`Processor.ProcessWithOptions` return the error; there is no line parser
fallback. `Processor.ProcessFile` itself is off the live path (the central
processor calls `ProcessWithOptions`).

## Known gaps

See keys starting with `python/` in `internal/conformance/known_gaps_test.go`.
