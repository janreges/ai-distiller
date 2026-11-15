# Swift Language Support

AI Distiller provides support for Swift source code processing, enabling extraction of code structure for AI consumption.

## Overview

Swift is a powerful and intuitive programming language developed by Apple for building apps for iOS, Mac, Apple TV, and Apple Watch. AI Distiller can process Swift code to extract:

- Functions and methods with their signatures
- Classes, structs, enums, and protocols
- Properties and their types
- Access control modifiers
- Protocol conformances and inheritance
- Extensions
- Actors (Swift 5.5+), printed as classes

## Default Behavior

When running `aid` on Swift files with the default flags, you get:

- `public` and `open` declarations only (Swift's default access level is `internal`, so a declaration without a modifier is left out)
- Signatures without function bodies
- Documentation comments (`///` and `/** */`)
- Import statements

## Filtering Options

- `--internal=1`: Includes `internal` declarations (no access modifier)
- `--private=1`: Includes `private` and `fileprivate` declarations
- `--implementation=1`: Includes function and method bodies
- `--comments=1`: Includes regular `//` and `/* */` comments
- `--docstrings=0`: Removes documentation comments (`--comments=0`, the default, keeps them)
- `--imports=0`: Removes import statements

All flags are described in [COMMAND-LINE-OPTIONS.md](../user/COMMAND-LINE-OPTIONS.md).

## Key Features

### Visibility Handling

Swift has a sophisticated access control system:
- `open` and `public`: Accessible from any module
- `internal`: Accessible within the defining module (default)
- `fileprivate`: Accessible within the defining file
- `private`: Accessible within the enclosing declaration

AI Distiller maps `open` and `public` to public, a declaration without a modifier to internal, and `private` and `fileprivate` to private. The text output prints the access keyword in front of each declaration (`public func`, `internal func`, `private func`).

### Type Inference

Swift's strong type inference means many declarations don't have explicit types. AI Distiller keeps type information where it is explicitly stated; a stored property without a type annotation (`public var y = 0`) is not extracted.

### Protocol-Oriented Programming

Swift emphasizes protocol-oriented programming. AI Distiller recognizes:
- Protocol declarations with their method and property requirements
- Protocol conformances
- Extensions, printed as `extension Name: Protocol { ... }` with their members

## Known Issues and Limitations

### Current Implementation Status

⚠️ **Important**: Swift support uses a line-based parser (regular expressions), not tree-sitter. The following features are currently supported:

**Supported Features:**
- Enum declarations with their cases (raw values and the raw type are not printed)
- Basic enum cases with associated values
- Class, struct, and enum type detection
- Property declarations with type annotations
- Basic visibility modifiers (open, public, internal, fileprivate, private)
- Documentation comments, and regular comments with `--comments=1`
- Protocol declarations (method and property requirements)
- Extensions with their conformances
- Import statements
- Type aliases
- Actor declarations (printed as `class`)

## Recent Fixes (2025-06-15)

1. **Tree-sitter segfault** (✅ Fixed)
   - **Issue**: Tree-sitter Swift parser was causing segmentation faults
   - **Fix**: Temporarily disabled tree-sitter, using improved line-based parser
   - **Impact**: Stable parsing, though with some limitations

2. **Missing `func` keyword** (✅ Fixed)
   - **Issue**: Functions were displayed without the `func` keyword
   - **Fix**: Updated Swift formatter to include proper function syntax
   - **Impact**: More recognizable Swift syntax in output

3. **Protocol inheritance syntax** (✅ Fixed)
   - **Issue**: Extra `{` appeared in protocol inheritance
   - **Fix**: Updated regex to exclude opening braces from inheritance capture
   - **Impact**: Clean protocol declarations

**Known Issues:**
1. **Line-based parser limitations**
   - A function whose signature spans several lines is not extracted
   - A stored property without a type annotation (`var y = 0`) is not extracted
   - Associated types in protocols are not parsed
   - Members of an extension are internal unless they carry their own modifier, so `public extension X { var y: T }` shows `y` only with `--internal=1`
   - Initializers (`init(...)`) are not extracted
   - Argument labels are dropped: `f(with a: Int, _ b: Int)` prints as `f(a: Int, b: Int)`
   - Property requirements of a protocol print without `{ get set }`

2. **Not Yet Implemented**
   - Property wrappers (@State, @Published, etc.): the wrapper is dropped, the property is kept
   - Result builders (@ViewBuilder, etc.)
   - `throws`: dropped; `async` prints before `func` (`public async func f()`)
   - Where clauses in extensions and generics
   - Subscripts
   - Enum raw types and raw values (`enum E: String { case a = "A" }` prints as `enum E { case a }`)

3. **Minor Issues**
   - Some computed property syntax variations
   - Complex enum associated values

### Parser Implementation

Due to stability issues with the tree-sitter Swift parser, AI Distiller uses a line-based parser that covers the common Swift constructs. The tree-sitter grammar is still compiled in for a later migration but is not used.

## Examples

<details open><summary>Basic Function</summary><blockquote>
  <details><summary>input.swift</summary><blockquote>
    
```swift
public func calculateSum(a: Int, b: Int) -> Int {
    return a + b
}

private func helperFunction() {
    print("Helper")
}
```
    
  </blockquote></details>
  <details open><summary>Default output</summary><blockquote>
    
```
<file path="input.swift">
public func calculateSum(a: Int, b: Int) -> Int
</file>
```
    
  </blockquote></details>
  <details><summary>With `--private=1 --protected=1 --internal=1`</summary><blockquote>
    
```
<file path="input.swift">
public func calculateSum(a: Int, b: Int) -> Int
private func helperFunction()
</file>
```
    
  </blockquote></details>
  <details><summary>With `--implementation=1`</summary><blockquote>
    
```
<file path="input.swift">
public func calculateSum(a: Int, b: Int) -> Int {
    return a + b
}
</file>
```
    
  </blockquote></details>
</blockquote></details>

<details><summary>Class with Properties</summary><blockquote>
  <details><summary>User.swift</summary><blockquote>
    
```swift
public class User {
    public let id: String
    private var name: String
    internal var email: String?
    
    public init(id: String, name: String) {
        self.id = id
        self.name = name
    }
    
    public func updateName(_ newName: String) {
        self.name = newName
    }
    
    private func validate() -> Bool {
        return !name.isEmpty
    }
}
```
    
  </blockquote></details>
  <details open><summary>Default output</summary><blockquote>
    
```
<file path="User.swift">

public class User {
    public let id: String
    public func updateName(newName: String)
}
</file>
```
    
  </blockquote></details>
  <details><summary>With `--private=1 --protected=1 --internal=1 --implementation=1`</summary><blockquote>
    
```
<file path="User.swift">

public class User {
    public let id: String
    private var name: String
    internal var email: String?
    public func updateName(newName: String) {
        self.name = newName
    }
    private func validate() -> Bool {
        return !name.isEmpty
    }
}
</file>
```
    
  </blockquote></details>
</blockquote></details>

<details><summary>Protocol and Conformance</summary><blockquote>
  <details><summary>Drawable.swift</summary><blockquote>
    
```swift
public protocol Drawable {
    func draw()
    var color: String { get set }
}

public struct Circle: Drawable {
    public var radius: Double
    public var color: String
    
    public func draw() {
        print("Drawing circle with radius \(radius)")
    }
}
```
    
  </blockquote></details>
  <details open><summary>Default output</summary><blockquote>
    
```
<file path="Drawable.swift">

public protocol Drawable {
    public func draw()
    public var color: String
}

public struct Circle: Drawable {
    public var radius: Double
    public var color: String
    public func draw()
}
</file>
```
    
  </blockquote></details>
</blockquote></details>

## Best Practices

1. **Type Annotations**: While Swift has excellent type inference, explicit type annotations help AI Distiller provide more accurate output (an untyped stored property is not extracted)
2. **Access Control**: Use explicit access control modifiers to clearly indicate API boundaries
3. **Documentation**: Use `///` documentation comments for public APIs - they are printed unless `--docstrings=0` is given
4. **Organization**: Use `// MARK:` comments to organize code sections (printed with `--comments=1`)

## Integration Tips

When using AI Distiller output with LLMs:

1. **Use the default flags** for an API overview (public declarations, no bodies)
2. **Add `--internal=1`** for an app target, where most declarations have no access modifier
3. **Include full output** (`--private=1 --internal=1 --implementation=1`) when debugging implementation details
4. **Consider file size** - Swift files can be large, especially with generics

## Future Roadmap

1. **Fix Critical Issues**: Multi-line signatures, untyped properties, associated types, generic constraints
2. **Swift 6 Support**: Including new concurrency features and strict checking
3. **SwiftUI Support**: Property wrappers, result builders, and view modifiers
4. **Type Resolution**: Better handling of type inference and associated types
5. **Cross-file Analysis**: Understanding extensions and protocol conformances across files
6. **Complete Async Support**: Full async/await/throws modifier support
