# Ruby

## Parser

- Wired: `internal/language/ruby/processor.go#Processor.ProcessWithOptions`
  creates `NewTreeSitterProcessor()` per call and runs
  `internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.ProcessSource`.
- Grammar: `github.com/tree-sitter/tree-sitter-ruby/bindings/go`, wrapped with
  `sitter.NewLanguage`. Tree: `go run ./cmd/ast-dump --lang ruby`.
- Extensions: `.rb`, `.rake`, `.gemspec`.
- Dispatch: `internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processNode`
  handles `class`, `singleton_class`, `module`, `method`, `singleton_method`,
  `assignment`, `call` and `alias`, and recurses into any other node type.
  Class, module and singleton-class bodies go through
  `internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processStatements`,
  which also handles comments and visibility sections.
- A `call` counts only without a receiver (or with `self`); then
  `internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processCall`
  acts on `require`, `require_relative`, `load`, `attr_reader`, `attr_writer`,
  `attr_accessor`, `include`, `private`, `protected`, `public` and
  `alias_method`. Every other call yields nothing.

## Visibility

- Methods are `public` by default. Each class, module or `class << self` body
  starts a new `public` section (a stack, one entry per open body).
- A bare `private`, `protected` or `public` line switches the section for the
  methods that follow in the same body.
- `private :a, :b` sets the visibility of the already declared methods named
  `a` and `b` (singleton methods excepted); `private def x ... end` sets it for
  that one method only, without changing the section
  (`internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processVisibilityCall`).
- `private attr_reader :x` gives the accessors it declares that visibility;
  `private_class_method :x` makes the singleton method `x` private
  (`...#TreeSitterProcessor.processPrivateClassMethod`).
- `def self.x` is a singleton method: it gets the modifier `static` and stays
  `public` whatever the section.
- An assignment to a constant, class variable or instance variable outside a
  method body is a `public` field (a constant gets the modifier `final`, a class
  variable `static`); the `@name` field created by `attr_*` is `private`.
- Ruby produces no `internal`.

## Methods and types

- Methods are nested in their `DistilledClass`. A superclass goes to `Extends`.
- A `module` is a `DistilledInterface` with its methods as children.
- `include M` adds `M` to `Implements` of a class or to `Extends` of a module
  (`internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processModuleInclusion`).
  `extend` and `prepend` are not recorded.
- `class << self` is a nested `DistilledClass` named `<<self>>` with the
  modifier `static`.
- `attr_reader :x` creates a getter function `x` returning `Object`,
  `attr_writer :x` a setter `x=` with one `Object` parameter, `attr_accessor`
  both; each also adds a `private` field `@x`
  (`internal/language/ruby/tree_sitter_processor.go#TreeSitterProcessor.processAttrMethods`).
- `alias new old` and `alias_method :new, :old` create a `public` function
  `new` whose `Implementation` is `alias for old`.
- Reopening a class creates a second `DistilledClass` with the same name; the
  two are not merged.

## IR specifics

- `require`, `require_relative` and `load` with a plain string argument are a
  `DistilledImport` whose `ImportType` is the method name; a computed argument
  yields nothing.
- No namespace node: a module is a `DistilledInterface`.
- `Implementation` is the method's `body_statement` text, without the `def`
  line and the `end`.
- Comments: a run of own-line `#` comments that ends on the line directly above
  a `class`, `module`, `class << self` or `def` (also `private def x`) is one
  `doc` comment. Every other `#` comment is `line`;
  `=begin`/`=end` is `block`. The doc comments of a statement that yields no
  declaration are dropped
  (`internal/language/commentutil/commentutil.go#AppendStatement`).
- `Extensions` is not used.

## Unused files

None.

## Known gaps

See keys starting with `ruby/` in `internal/conformance/known_gaps_test.go`.
