# Rust Language Support

AI Distiller parses Rust with tree-sitter (the `tree-sitter-rust` grammar v0.24.2, compiled into the binary). It covers the item declarations: structs (also tuple structs), unions, enums, traits, impl blocks, functions, constants, type aliases, modules, `extern` blocks, `macro_rules!` definitions, `use` and `extern crate`.

## Overview

Rust support in AI Distiller captures the essential structure of Rust code including ownership annotations, lifetime parameters, trait bounds, and async functions. A declaration written across several lines (a parameter list, a `where` clause, a `use` group) is printed on one line. Items inside function bodies are not distilled; the items inside an item-level macro call with braces (`cfg_if! { ... }`, `lazy_static! { ... }`) are. The limitations are listed below.

Doc comments print in their own style (`///` and `/** */` as `///`, `//!` and `/*! */` as `//!`, with the ` * ` gutters of a block removed), also inside struct, enum and union bodies above their fields and variants; other comments print as `//` with `--comments=1`. The outer attributes of an item, a named field and an enum variant (`#[derive(Debug)]`, `#[repr(C)]`, `#[serde(rename = "x")]`, `#[default]`) print one per line above it, after its docs, unless `--annotations=0` is given. The docs, the attributes and the item follow each other without a blank line; the blank line that separates an item from the one before it goes above its docs.

## Supported Rust Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Structs** | ✅ Full | Named-field structs with generics and a `where` clause, hidden private fields as `/* private fields */` like rustdoc (`pub struct Config { /* private fields */ }`, or a line after the public fields left); tuple structs with their fields, a hidden field as `_` (`pub struct Pair(pub i32, _);`) |
| **Unions** | ✅ Full | Printed like structs: `pub union FFIData {` with the visible fields |
| **Enums** | ✅ Full | Variants with tuple or struct payloads and discriminants (`Ok = 0`), also on one line |
| **Traits** | ✅ Full | Supertraits (`pub trait Animal: Display + Clone {`), the `where` clause, methods, including `async fn` and `unsafe fn` members (default methods with their bodies under `--implementation=1`; a required method ends with `;`), associated types with their generics, `const` items (`const LEGS: u32;`) and the comments of the trait body |
| **Implementations** | ✅ Full | Inherent and trait impls with their generics and `where` clause (`impl<T> Copy for Own<T> where T: ?Sized {`), their methods, and the associated types and consts of a trait impl (`type Item = u32;`), without a visibility keyword like its methods. As in rustdoc, an impl block is hidden with its type or trait when the block's path names one declared in this file (in the block's own module, or in the inline module its path names) and the visibility flags hide it without a `pub use` that re-exports it (`impl Read for Reader<'_>` of a private `struct Reader` prints only with `--private=1`; a `pub struct Engine` of a private `mod imp` with `pub use imp::Engine;` keeps its impls). A type that comes in through `use` or from another file leaves the block shown, where rustdoc may still hide it. An inherent impl none of whose members is shown is not printed; a trait impl without members prints (`impl Copy for X {`) |
| **Functions** | ⚠️ Partial | Free functions and methods, also with a parameter list or `where` clause over several lines, tuple-pattern parameters, and the qualifiers in Rust's order (`pub const async unsafe extern "C" fn f()`; a bare `extern fn` prints bare). A `where` clause prints after the return type without its trailing comma. Partial: `default fn` (specialization) and `safe fn` (in an `unsafe extern` block) print without that keyword, a typed `self` (`self: Box<Self>`) prints as `self`, and an associated type with a default in a trait (`type Out = u8;`) is not printed |
| **Type Aliases** | ✅ Full | Printed with their generics and value (`pub type Map<K> = HashMap<K, u32>;`) |
| **Constants** | ✅ Full | `const` and `static`, names as written; a value over several lines prints as `...` |
| **Modules** | ✅ Full | An inline `mod name { ... }` prints with every item it holds (`use`, nested structs, impls, modules, functions, constants); `mod name;` when it holds none or is declared without a body |
| **Use statements** | ✅ Full | Paths, nested and multi-line groups, `self`, globs and `as` renames, as written; re-exports keep their visibility (`pub use`, `pub(crate) use`); `extern crate alloc;` and `extern crate foo as bar;`. Imports follow `--imports`, not the visibility flags |
| **Extern blocks** | ✅ Full | `extern "C" {` with its foreign functions (`fn abs(x: i32) -> i32;`) and statics, each by its own visibility; a block none of whose items is shown is not printed. An extern `type` written with a visibility (`pub type Opaque;`) is not parsed by tree-sitter-rust v0.24.2 |
| **Macros** | ⚠️ Partial | A `macro_rules!` definition prints as `macro_rules! name {` with one `(matcher) => ...;` line per rule, or its whole body with `--implementation=1`; it is public with `#[macro_export]` and internal (`--internal=1`) without it. Macros are not expanded; the items written inside an item-level macro call with braces (`s! { ... }`, `cfg_if! { if #[cfg(unix)] { ... } else { ... } }`) print where the call stands, also when one item uses syntax of the macro's own before its keyword or after its name (`pub {const} fn`, `pub struct ExprArray #full { ... }`). An item whose name the parser reads across such syntax is skipped with its docs (bitflags' `pub struct Mode: libc::mode_t { ... }`, paste's `pub fn [<get_ $name>]()`, and `pub struct X #full { ... }`, where the parser takes `full` for the name), and so is an item the parser cannot read at all (`fn f() -> T { ... } ?= default`) |
| **Async/Await** | ✅ Full | `async fn` keeps `async` |
| **Unsafe** | ✅ Full | `unsafe fn`, `unsafe impl Send for X {`, `pub unsafe trait Z {`, `unsafe extern "C" {` and `static mut C: u64 = 0;` print as written |

### Type System Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Lifetimes** | ✅ Full | In function signatures, including `'static`, and on impl blocks (`impl<'a>`) |
| **References** | ✅ Full | `&T`, `&mut T`, `&'a T` |
| **Generic parameters** | ✅ Full | Type, lifetime and const generics on functions and structs |
| **Trait bounds** | ✅ Full | Inline bounds, supertraits, and `where` clauses on functions and on struct, enum, union, trait and impl headers, printed on one line also when they span several |
| **Associated types** | ✅ Full | In traits (`type Item: Clone;`) and in trait implementations (`type Item = u32;`) |
| **Return types** | ✅ Full | Including `impl Trait`, `-> Result<T, E>` |
| **Self types** | ✅ Full | `self`, `&self`, `&mut self`, `&'a self` |

## Recent Fixes

Earlier fixes (2025): `&'a self` and other self parameters print; complex and generic return types are kept whole; the members of a trait print without `pub`.

Later fixes: methods of `impl Trait for Type` are public (as visible as the trait) and print in the default output; one-line structs and enums get their fields and variants; a trait's members after a default method are no longer lost; printed bodies (`--implementation=1`) no longer repeat the signature; constant names print as written.

November 2026: the line parser was replaced by a tree-sitter parser. Multi-line signatures, `where` clauses and `use` groups, impl generics, trait `async fn` members, and the generics of type aliases and associated types now print; trailing comments are no longer glued into types; the items inside item-level macro calls with braces print; a comment at the end of a code line no longer prints as a comment line of its own. Then inline modules with all their items, tuple-struct fields, unions, `extern` blocks, `macro_rules!` definitions, `extern crate`, `pub use`, trait consts, supertraits, `where` clauses on item headers and the associated types of trait impls started to print, each with the docs above it. Finally doc comments print as `///` and `//!` instead of `//`, struct, enum and union bodies print the docs above their fields and variants, and outer attributes print. Then the blank line between a doc comment and its item moved above the doc, where it no longer makes the doc look like part of the item before, and a struct or union whose private fields are hidden prints `/* private fields */` instead of looking like a unit struct (`pub struct Config;`). After v1.5.0 the impl blocks of a hidden type are hidden with it, an inherent impl without a shown member is not printed, and the body of an impl block, trait, extern block or module starts on the line after its `{` instead of after a blank line.

## Key Features

### 1. **Ownership and Borrowing Representation**

```rust
// Input
impl<'a> Parser<'a> {
    pub fn new(input: &'a str) -> Self {
        Self { input, position: 0 }
    }
    
    pub fn parse(&mut self) -> Result<Ast, ParseError> {
        // implementation
    }
    
    fn consume(&mut self, expected: &str) -> Option<&'a str> {
        // implementation
    }
}
```

```
<file path="parser.rs">

impl<'a> Parser<'a> {
    pub fn new(input: &'a str) -> Self

    pub fn parse(&mut self) -> Result<Ast, ParseError>
}
</file>
```

### 2. **Trait Definitions and Implementations**

```rust
// Input
pub trait Serialize: Sized {
    type Error: std::error::Error;
    
    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), Self::Error>;
    
    fn to_bytes(&self) -> Result<Vec<u8>, Self::Error> {
        let mut buf = Vec::new();
        self.serialize(&mut buf)?;
        Ok(buf)
    }
}

impl<T: Display> Serialize for T {
    type Error = std::io::Error;
    
    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), Self::Error> {
        write!(writer, "{}", self)
    }
}
```

```
<file path="serialize.rs">

pub trait Serialize: Sized {
    type Error: std::error::Error;

    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), Self::Error>;

    fn to_bytes(&self) -> Result<Vec<u8>, Self::Error>
}

impl<T: Display> Serialize for T {
    type Error = std::io::Error;

    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), Self::Error>
}
</file>
```

### 3. **Async and Error Handling**

```rust
// Input
#[derive(Debug, thiserror::Error)]
pub enum ApiError {
    #[error("Network error: {0}")]
    Network(#[from] reqwest::Error),
    #[error("Parse error: {0}")]
    Parse(#[from] serde_json::Error),
}

pub async fn fetch_user(id: u64) -> Result<User, ApiError> {
    let response = client.get(&format!("/users/{}", id)).send().await?;
    let user = response.json::<User>().await?;
    Ok(user)
}
```

```
<file path="api.rs">

#[derive(Debug, thiserror::Error)]
pub enum ApiError {
    #[error("Network error: {0}")]
    Network(#[from] reqwest::Error),
    #[error("Parse error: {0}")]
    Parse(#[from] serde_json::Error),
}

pub async fn fetch_user(id: u64) -> Result<User, ApiError>
</file>
```

## Visibility Rules

Rust visibility in AI Distiller:
- **pub**: Public, accessible from outside module (shown by default)
- **pub(crate)**: Visible within the crate (internal: shown with `--internal=1`)
- **pub(super)**: Visible in parent module (protected: shown with `--protected=1`)
- **pub(in path)**: Visible in specific path (protected: shown with `--protected=1`)
- **(default)**: Private to the module (shown with `--private=1`)

The members of `impl Trait for Type` (methods, associated types and consts) and the members declared in a trait are public and print without a keyword. An impl block has no visibility of its own: it shows when its self type and its trait show. A struct, enum, union or trait declared in the same file decides by its own visibility and that of the modules around it (a `pub(crate) struct` takes its impls along until `--internal=1`); a type declared elsewhere (`impl Display for Vec<u8>`, a generic `impl<T> Tr for T`) leaves the block shown. The type is found as Rust resolves the block's path, without generics or `&` (`impl Foo<'_>` and `impl Tr for &Foo` are impls of `Foo`): a bare name in the block's own module, `a::Foo`, `self::a::Foo` and `super::Foo` through the inline modules they name. A type of the same name in another module (a `struct Error;` in `mod tests`) is another type, a name that a `use` brings in (`pub use crate::error::Error;`, also a private `use`) and a `crate::` path (the file may not be the crate root) leave the block shown, and a name declared twice in one module hides it only when both declarations are hidden. A `pub use` that the flags show re-exports a type of a private module (`pub use imp::Engine;`, `pub use imp::{A, B as C};`, `pub use imp::*;`, also through another module's `pub use`), so its impls show while its own visibility allows (a `pub(crate) struct` of a glob stays hidden); a `pub(crate) use` re-exports from `--internal=1`. A `macro_rules!` is public with `#[macro_export]` and internal without it; the foreign items of an `extern` block have their own visibility. The text output prints `pub`, `pub(crate)` and `pub(super)` as written; `pub(in path)` prints as written (`pub(in crate::a) fn f()`); the 2015 `crate fn f()` prints as `pub(crate) fn f()`; a private item prints without a keyword. `pub(self)` is private, as in Rust: hidden by default, and printed without a keyword with `--private=1`.

## Known Limitations

1. **Macros**: a `macro_rules!` prints its rules' matchers, not what they expand to, and macros are not expanded; the items inside a macro call are distilled only for an item-level call with braces whose body starts like items (an item keyword, an attribute, the `if` of `cfg_if!` or a macro call; not the arms of `select!` or a table), only when the body parses within a budget (the parser's work on about 75 KiB of the densest code or 300 KiB of typical code; a body over it prints nothing), only the items the parser can read (or those in the `{}` blocks of a statement it cannot read, as in `cfg_if!`), and never inside `()` or `[]` calls or function bodies
2. **Attributes**: inner attributes (`#![allow(dead_code)]`) are not printed, nor are the attributes of parameters, statements, expressions, `use` declarations and the fields of a tuple struct (`struct S(#[serde(skip)] u8);` prints `struct S(u8);`)
3. **Empty bodies**: a struct or union whose fields are all hidden prints as `pub struct X;`, and so does `struct X();`; a module whose items are all hidden prints as `pub mod x;`

## Best Practices

### 1. **Explicit Lifetime Annotations**

While Rust allows lifetime elision, explicit lifetimes help AI understanding:

```rust
// Good for AI comprehension
pub fn parse<'a>(input: &'a str) -> Result<Token<'a>, Error>

// Less clear (though valid Rust)
pub fn parse(input: &str) -> Result<Token, Error>
```

### 2. **Clear Trait Bounds**

Use descriptive trait bounds. A `where` clause is printed on the line of the signature or header it belongs to:

```rust
// Good
pub fn process<T: Serialize + Send + 'static>(item: T) -> Result<(), Error>

// Better with where clause for complex bounds
pub fn process<T>(item: T) -> Result<(), Error>
where
    T: Serialize + Send + 'static,
    T::Error: From<std::io::Error>,
```

### 3. **Module Organization**

Structure modules for clear API boundaries. An inline module prints with its items; modules in their own files are processed as separate files:

```rust
pub mod api {
    pub struct Client { /* ... */ }
    pub trait Handler { /* ... */ }
}

mod internal {  // Private implementation details
    pub(super) fn helper() { /* ... */ }
}
```

## Output Examples

<details>
<summary>Complex Generic System</summary>

Input:
```rust
pub struct Cache<K, V, S = RandomState> 
where 
    K: Hash + Eq,
    V: Clone,
    S: BuildHasher,
{
    map: HashMap<K, V, S>,
    capacity: usize,
}

impl<K, V, S> Cache<K, V, S>
where
    K: Hash + Eq,
    V: Clone,
    S: BuildHasher,
{
    pub fn with_hasher(capacity: usize, hasher: S) -> Self {
        Self {
            map: HashMap::with_hasher(hasher),
            capacity,
        }
    }
    
    pub fn get(&self, key: &K) -> Option<&V> {
        self.map.get(key)
    }
    
    pub fn insert(&mut self, key: K, value: V) -> Option<V> {
        if self.map.len() >= self.capacity {
            self.evict_oldest();
        }
        self.map.insert(key, value)
    }
    
    fn evict_oldest(&mut self) {
        // implementation
    }
}
```

Output (default):
```
<file path="cache.rs">

pub struct Cache<K, V, S = RandomState> where K: Hash + Eq, V: Clone, S: BuildHasher;

impl<K, V, S> Cache<K, V, S> where K: Hash + Eq, V: Clone, S: BuildHasher {
    pub fn with_hasher(capacity: usize, hasher: S) -> Self

    pub fn get(&self, key: &K) -> Option<&V>

    pub fn insert(&mut self, key: K, value: V) -> Option<V>
}
</file>
```

The struct's fields are private, so it prints with `;`. The `where` clauses of the struct and impl headers print on one line, without their trailing comma.

</details>

## Integration Tips

### For AI Code Review
```bash
# Get public API surface (the defaults)
aid src/

# Include crate-visible items and trait implementations
aid src/ --internal=1 -o rust-api.txt
```

### For Documentation Generation
```bash
# Extract all public items with doc comments and regular comments
aid src/ --comments=1
```

## Future Improvements

- Full macro expansion support
- Better const generic handling
- Improved async trait support
- Integration with cargo metadata

## Contributing

Rust support is actively maintained. Key areas for contribution:
- Macro system integration
- Complex lifetime scenarios
- Const generic expressions
- Async trait implementations

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup.
