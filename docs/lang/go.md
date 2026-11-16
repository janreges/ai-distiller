# Go Language Support

## Overview

AI Distiller provides support for Go (Golang) source code analysis and distillation. The Go processor uses the standard library parser (`go/parser`, `go/ast`) to build an Abstract Syntax Tree, which is then transformed into a simplified, AI-friendly format.

## Main Constructs

Go is a statically typed, compiled language with the following primary constructs:

- **Packages**: Organizational units (`package main`)
- **Imports**: Dependency management with aliasing support
- **Types**: Structs, interfaces, defined types (`type Named int`) and type aliases (`type Alias = int`, `type Pair = struct{...}`); the structured formats (jsonl, json-structured, xml) mark an alias with the modifier `type_alias`
- **Functions**: Standalone functions and methods with receivers
- **Variables/Constants**: Package and function-scoped declarations
- **Interfaces**: Behavioral contracts with implicit satisfaction
- **Generics**: Type parameters and constraints (Go 1.18+)

## Key Features

The Go distiller focuses on extracting the essential structure that helps AI understand:

1. **Type System**: All types are preserved including custom types, built-in types, and generic type parameters
2. **Method Receivers**: Distinction between value and pointer receivers
3. **Visibility**: Uppercase = exported (public), lowercase = unexported (internal: shown with `--internal=1`)
4. **Import Aliases**: Support for renamed imports (`import m "math"`)
5. **Function Bodies**: With `--implementation=1`, bodies are printed verbatim from the source, comments inside them included
6. **Tab Indentation**: The output indents with tabs, as gofmt does: bodies keep their source tabs, and struct fields, interface methods and imports are indented one tab per level

## Text Format Output

The text format keeps Go syntax: types and functions print as in the source, methods stay at file level after their type with their receiver, and bodies (with `--implementation=1`) keep their braces. For this input:

```go
package main

import (
	"fmt"
	m "math"
)

type Writer interface {
	Write(p []byte) (int, error)
}

type Logger struct {
	prefix string
	writer Writer
}

func NewLogger(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

func (l *Logger) Log(message string) {
	fmt.Printf("[%s] %s %f\n", l.prefix, message, m.Pi)
}
```

`aid example.go --internal=1 --implementation=1` prints:

```
<file path="example.go">
package main

import (
	"fmt"
	m "math"
)

type Writer interface {
	Write(p []byte) (int, error)
}

type Logger struct {
	prefix string
	writer Writer
}

func NewLogger(prefix string) *Logger {
	return &Logger{prefix: prefix}
}

func (l *Logger) Log(message string) {
	fmt.Printf("[%s] %s %f\n", l.prefix, message, m.Pi)
}
</file>
```

## Known Issues

### 1. Parameter and Field Grouping
**Status**: Expanded  
**Impact**: Low - Cosmetic difference

Grouped parameters and fields are expanded (the parameters inside a function type keep their grouping: `f func(x, y int)`):
```go
// Original
func Move(x, y int)
type Point struct { X, Y int }

// Distilled
func Move(x int, y int)

type Point struct {
	X int
	Y int
}
```

### 2. Single Import Formatting
**Status**: Always uses parentheses  
**Impact**: Low - Cosmetic issue

Single imports are formatted with parentheses like multi-imports:
```go
// Original
import "fmt"

// Distilled
import (
	"fmt"
)
```

### 3. Generic Type Parameters
**Status**: Basic support  
**Impact**: Medium - Complex constraints may not be fully preserved

Type parameters and constraints are kept; grouped type parameters are expanded (`[T, V any]` prints as `[T any, V any]`).

### 4. CGO Support
**Status**: Minimal  
**Impact**: Low - C code blocks not properly preserved

CGO import "C" and associated C code blocks are not handled correctly.

## Examples

<details open><summary>Basic Go Constructs</summary><blockquote>
  <details><summary>construct_1_basic.go - source</summary><blockquote>

```go
// Package basic provides fundamental Go constructs.
// It serves as the baseline for parser testing.
package basic

import (
	"fmt" // Standard library import
	m "math" // Aliased import
)

// Global constant Pi, a fundamental value.
const Pi = 3.14159

// var block for multiple declarations.
var (
	// IsEnabled controls a feature. Doc comments for vars are important.
	IsEnabled = true
	// UserCount is a package-level counter.
	UserCount int64 = 100
)

// Add sums two integers. A trivial function.
// It tests basic function declaration and parameter parsing.
func Add(x int, y int) int { // Line comment on the function signature
	// A comment inside the function body.
	z := x + y // Short variable declaration
	var result = z // Standard variable declaration
	_ = m.Abs(-1) // Using an aliased import
	fmt.Println(result)
	return result
}
```

  </blockquote></details>
  <details open><summary>Default compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>

```
<file path="construct_1_basic.go">
package basic

// Global constant Pi, a fundamental value.
const Pi = 3.14159

// var block for multiple declarations.
// IsEnabled controls a feature. Doc comments for vars are important.
var IsEnabled = true

// UserCount is a package-level counter.
var UserCount int64 = 100

// Add sums two integers. A trivial function.
// It tests basic function declaration and parameter parsing.
func Add(x int, y int) int
</file>
```

  </blockquote></details>
  <details><summary>Full version (`--public=1 --protected=1 --internal=1 --private=1 --implementation=1`)</summary><blockquote>

```
<file path="construct_1_basic.go">
package basic

import (
	"fmt"
	m "math"
)

// Global constant Pi, a fundamental value.
const Pi = 3.14159

// var block for multiple declarations.
// IsEnabled controls a feature. Doc comments for vars are important.
var IsEnabled = true

// UserCount is a package-level counter.
var UserCount int64 = 100

// Add sums two integers. A trivial function.
// It tests basic function declaration and parameter parsing.
func Add(x int, y int) int { // Line comment on the function signature
	// A comment inside the function body.
	z := x + y // Short variable declaration
	var result = z // Standard variable declaration
	_ = m.Abs(-1) // Using an aliased import
	fmt.Println(result)
	return result
}
</file>
```

  </blockquote></details>
</blockquote></details>

<details><summary>Structs and Methods</summary><blockquote>
  <details><summary>construct_2_simple.go - source</summary><blockquote>

```go
package simple

import "io"

// Writer defines a simple interface for writing data.
type Writer interface {
	// Write accepts a byte slice and returns bytes written and an error.
	Write(p []byte) (n int, err error)
}

// Data represents a container for a piece of data.
// It demonstrates a basic struct with a single field.
type Data struct {
	value string // unexported field
}

// NewData is a constructor-like function, a common Go idiom.
func NewData(v string) *Data {
	return &Data{value: v}
}

// ReadValue returns the current value. This is a value receiver method.
// It can be called on both Data and *Data types.
func (d Data) ReadValue() string {
	return d.value
}

// UpdateValue modifies the value. This is a pointer receiver method.
// It can only be called on a *Data type.
func (d *Data) UpdateValue(v string) {
	d.value = v
}

// Ensure Data does not satisfy the io.Writer interface, but we use it
// to test how the distiller handles imported interfaces.
var _ io.Writer = (*customWriter)(nil) // compile-time check idiom
type customWriter struct{}
func (cw *customWriter) Write(p []byte) (n int, err error) { return 0, nil }
```

  </blockquote></details>
  <details open><summary>Default compact AI-friendly version</summary><blockquote>

```
<file path="construct_2_simple.go">
package simple

// Writer defines a simple interface for writing data.
type Writer interface {
	Write(p []byte) (n int, err error)
}

// Data represents a container for a piece of data.
// It demonstrates a basic struct with a single field.
type Data struct {}

// NewData is a constructor-like function, a common Go idiom.
func NewData(v string) *Data

// ReadValue returns the current value. This is a value receiver method.
// It can be called on both Data and *Data types.
func (d Data) ReadValue() string

// UpdateValue modifies the value. This is a pointer receiver method.
// It can only be called on a *Data type.
func (d *Data) UpdateValue(v string)
</file>
```

  </blockquote></details>
  <details><summary>Full version</summary><blockquote>

```
<file path="construct_2_simple.go">
package simple

import (
	"io"
)

// Writer defines a simple interface for writing data.
type Writer interface {
	Write(p []byte) (n int, err error)
}

// Data represents a container for a piece of data.
// It demonstrates a basic struct with a single field.
type Data struct {
	value string
}

// NewData is a constructor-like function, a common Go idiom.
func NewData(v string) *Data {
	return &Data{value: v}
}

// ReadValue returns the current value. This is a value receiver method.
// It can be called on both Data and *Data types.
func (d Data) ReadValue() string {
	return d.value
}

// UpdateValue modifies the value. This is a pointer receiver method.
// It can only be called on a *Data type.
func (d *Data) UpdateValue(v string) {
	d.value = v
}

// Ensure Data does not satisfy the io.Writer interface, but we use it
// to test how the distiller handles imported interfaces.
var _ io.Writer = (*customWriter)(nil)

type customWriter struct {}

func (cw *customWriter) Write(p []byte) (n int, err error) { return 0, nil }
</file>
```

  </blockquote></details>
</blockquote></details>

<details><summary>Generics and Type Constraints</summary><blockquote>
  <details><summary>construct_4_complex.go - source</summary><blockquote>

```go
package complex

// Number is a constraint that permits any integer or floating-point type.
type Number interface {
	~int | ~int64 | ~float32 | ~float64
}

// Node is a generic struct representing a node in a linked list.
type Node[T any] struct {
	Value T
	Next  *Node[T]
}

// Map applies a function to each element of a slice, returning a new slice.
// This is a generic function with a closure.
func Map[T, V any](input []T, f func(T) V) []V {
	output := make([]V, len(input))
	for i, v := range input {
		output[i] = f(v)
	}
	return output
}

// ProcessNumericChan processes a channel of generic Nodes.
// It uses a type constraint.
func ProcessNumericChan[T Number](ch <-chan *Node[T]) T {
	var total T
	for node := range ch {
		total += node.Value // This operation is only valid because of the Number constraint.
	}
	// This function literal captures 'total' from its surrounding scope.
	defer func() {
		println("Final total:", total)
	}()
	return total
}
```

  </blockquote></details>
  <details open><summary>Default compact AI-friendly version</summary><blockquote>

```
<file path="construct_4_complex.go">
package complex

// Number is a constraint that permits any integer or floating-point type.
type Number interface {
	~int | ~int64 | ~float32 | ~float64
}

// Node is a generic struct representing a node in a linked list.
type Node[T any] struct {
	Value T
	Next *Node[T]
}

// Map applies a function to each element of a slice, returning a new slice.
// This is a generic function with a closure.
func Map[T any, V any](input []T, f func(T) V) []V

// ProcessNumericChan processes a channel of generic Nodes.
// It uses a type constraint.
func ProcessNumericChan[T Number](ch <-chan *Node[T]) T
</file>
```

  </blockquote></details>
</blockquote></details>

## Recent Fixes (2025-06-15)

1. **Generic type parameters** (✅ Fixed)
   - **Issue**: Generic type parameters were missing from function signatures
   - **Fix**: Added proper extraction of type parameters from function declarations in AST parser
   - **Impact**: Generics now properly displayed for functions like `Map[T any, V any]`

2. **Verbatim function bodies** (✅ Fixed)
   - **Issue**: Function bodies were rebuilt as lossy pseudo-code (an `if` lost its body and `else` branch, a `for` its header)
   - **Fix**: The body is copied verbatim from the source
   - **Impact**: `--implementation=1` and `--expand` print the real code with braces

## Usage Notes

1. **Visibility Rules**: Go's visibility is determined by the first character of identifiers:
   - Uppercase first letter = Exported (public)
   - Lowercase first letter = Unexported (internal: `--internal=1` shows it; `--private=1` adds nothing for Go)
   - A method is as visible as the less visible of its own name and its receiver type

2. **Import Organization**: Imports print as one `import ( ... )` block; in text output, an import that the printed code does not use is dropped (`--filter-imports=false` keeps it), and when none is left, so is the block. Blank imports (`_ "embed"`), dot imports (`. "math"`, whose names are used unqualified) and `"C"` are always kept.

3. **Generic Support**: Type parameters and constraints of Go 1.18+ generics are kept.

4. **Best Practices**: 
   - Use appropriate visibility flags to focus on public API
   - The text format provides the most compact representation for AI consumption
   - Methods print at file level with their receiver, after their type, not inside it

## Future Improvements

The following improvements are planned:

1. **Parameter Grouping**: Preserve original parameter grouping syntax
2. **Import Formatting**: Distinguish between single and multi-import blocks
3. **Generic Type Parameters**: Complete support for complex generic constraints
4. **CGO Support**: Handle C code blocks in CGO files

## Technical Implementation

The Go processor uses:
- **Parser**: Standard `go/parser` with `ParseComments` mode
- **AST Processing**: Native `go/ast` traversal
- **Visibility**: Detection based on naming conventions
- **Formatter**: Custom text formatter with Go-specific syntax rules

A file with a syntax error produces no output: in a directory run `aid` prints `Warning: failed to process ...` on stderr and goes on with the other files; a single Go file or Go stdin with a syntax error makes `aid` exit 1. Declarations are emitted in source order, one blank line between two of them (a method included), above the doc of the second; a doc comment stands directly on its declaration. Doc comments of functions and types are documentation (`--docstrings`), the package comment and other comments follow `--comments`. The comments of struct fields and interface methods also follow `--comments`; they print inside their type, above their field or at the end of its line, and go with the field when it is hidden. So does the comment at the end of a `const`, `var` or `type` line, of a function's last line (`func (s *S) A() int { return s.a } // returns a`) or of the package clause: it prints at the end of that line. The comments of the imports print in the import block, indented, the one at the end of an import's line on that line; when the import filter drops an import, the comments directly above it and at the end of its line go with it. The doc of an import declaration (the comment directly above `import`) prints directly above the block and goes with it when the filter drops the whole block; a comment set off from `import` by a blank line, such as a file header, is a free comment and stays. A comment inside a function (its parameters or body) or inside a `const`, `var` or `type` declaration is part of that declaration: a function body prints it with `--implementation=1`, and it never prints on its own.

For more details, see the implementation in `internal/language/golang/`.
