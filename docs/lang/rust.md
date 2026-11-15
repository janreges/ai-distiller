# Rust Language Support

AI Distiller parses Rust with tree-sitter (the `tree-sitter-rust` grammar v0.24.2, compiled into the binary). It covers the common item declarations: structs, enums, traits, impl blocks, functions, constants, type aliases and use statements.

## Overview

Rust support in AI Distiller captures the essential structure of Rust code including ownership annotations, lifetime parameters, trait bounds, and async functions. A declaration written across several lines (a parameter list, a `where` clause, a `use` group) is printed on one line. Items inside function bodies are not distilled; the items inside an item-level macro call with braces (`cfg_if! { ... }`, `lazy_static! { ... }`) are. The limitations are listed below.

## Supported Rust Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Structs** | ⚠️ Partial | Named-field structs with generics; a tuple struct prints without its fields (`pub struct Meters;`) |
| **Enums** | ✅ Full | Variants with tuple or struct payloads and discriminants (`Ok = 0`), also on one line |
| **Traits** | ⚠️ Partial | Methods, including `async fn` and `unsafe fn` members (default methods with their bodies under `--implementation=1`; a required method ends with `;`), associated types with their generics, and the comments of the trait body; supertraits (`: Sized`) and trait `const` items are dropped |
| **Implementations** | ⚠️ Partial | Inherent and trait impls with their generics (`impl<T: Display> Trait for Type<T>`) and their methods; the associated types of a trait impl print only with `--private=1`, and a `where` clause on the impl header is dropped |
| **Functions** | ⚠️ Partial | Free functions and methods, also with a parameter list or `where` clause over several lines, tuple-pattern parameters and `extern "C" fn`; the `unsafe`, `const` and `extern` qualifiers are dropped |
| **Type Aliases** | ✅ Full | Printed with their generics and value (`pub type Map<K> = HashMap<K, u32>;`) |
| **Constants** | ✅ Full | `const` and `static`, names as written; a value over several lines prints as `...` |
| **Modules** | ❌ Broken | An inline `mod name { ... }` prints only its `const`, `static`, `type` and `fn` items, and only when it holds one of the first three; with functions alone it prints as an empty `enum name {}`, otherwise as `mod name;` |
| **Use statements** | ✅ Full | Paths, nested and multi-line groups, `self`, globs and `as` renames, as written; `pub use` prints as `use` |
| **Macros** | ⚠️ Partial | `macro_rules!` definitions are skipped and macros are not expanded; the items written inside an item-level macro call with braces (`s! { ... }`, `cfg_if! { if #[cfg(unix)] { ... } else { ... } }`) print where the call stands, also when one item uses syntax of the macro's own (`pub {const} fn`, `pub struct X #full { ... }`); an item the parser cannot read at all (`fn f() -> T { ... } ?= default`) is skipped |
| **Async/Await** | ✅ Full | `async fn` keeps `async` |
| **Unsafe** | ⚠️ Partial | The `unsafe` keyword is dropped |

### Type System Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Lifetimes** | ✅ Full | In function signatures, including `'static`, and on impl blocks (`impl<'a>`) |
| **References** | ✅ Full | `&T`, `&mut T`, `&'a T` |
| **Generic parameters** | ✅ Full | Type, lifetime and const generics on functions and structs |
| **Trait bounds** | ⚠️ Partial | Inline bounds, and a function's `where` clause (printed on the signature line, also when it spans several lines); a `where` clause on a struct, trait or impl header is dropped, while the item keeps its fields or methods |
| **Associated types** | ⚠️ Partial | In traits; not in implementations |
| **Return types** | ✅ Full | Including `impl Trait`, `-> Result<T, E>` |
| **Self types** | ✅ Full | `self`, `&self`, `&mut self`, `&'a self` |

## Recent Fixes (2025-06-15)

1. **&self parameters missing** (✅ Fixed)
   - **Issue**: Methods with lifetime-annotated self like `&'a self` were missing parameters
   - **Fix**: Enhanced `isSelfParameter()` to handle complex self patterns
   - **Impact**: All self parameter variations now correctly displayed

2. **Complex return types cut off** (✅ Fixed)
   - **Issue**: Return types like `-> Option<HashMap<String, Vec<T>>>` were truncated
   - **Fix**: Changed regex from non-greedy to greedy matching
   - **Impact**: Full return type signatures preserved

3. **Generic return types** (✅ Fixed)
   - **Issue**: Generic parameters in return types were not captured
   - **Fix**: Improved return type extraction in line parser
   - **Impact**: Complete generic type information shown

4. **Trait methods showing `pub`** (✅ Fixed)
   - **Issue**: Methods in trait definitions incorrectly showed `pub` keyword
   - **Fix**: Added context detection to suppress visibility in traits
   - **Impact**: Idiomatic Rust trait syntax

Later fixes: methods of `impl Trait for Type` are public (as visible as the trait) and print in the default output; one-line structs and enums get their fields and variants; a trait's members after a default method are no longer lost; printed bodies (`--implementation=1`) no longer repeat the signature; constant names print as written.

November 2026: the line parser was replaced by a tree-sitter parser. Multi-line signatures, `where` clauses and `use` groups, impl generics, trait `async fn` members, and the generics of type aliases and associated types now print; trailing comments are no longer glued into types; the docs of macros, `extern` blocks and unions no longer attach to the next item; the items inside item-level macro calls with braces print; a comment at the end of a code line no longer prints as a comment line of its own.

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

pub trait Serialize {
    type Error: std::error::Error;

    fn serialize<W: Write>(&self, writer: &mut W) -> Result<(), Self::Error>;

    fn to_bytes(&self) -> Result<Vec<u8>, Self::Error>
}

impl<T: Display> Serialize for T {

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

pub enum ApiError {
    Network(#[from] reqwest::Error),
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

Methods of `impl Trait for Type` and the methods declared in a trait are public. The text output prints `pub`, `pub(crate)` and `pub(super)` as written; `pub(in path)` prints as `pub(super)` (`pub(in crate::a) fn f()` prints as `pub(super) fn f()`); a private item prints without a keyword. `pub(self)`, which Rust treats as private, is classified as public and prints as `pub` (a known bug).

## Known Limitations

1. **Modules**: an inline `mod name { ... }` prints only its `const`, `static`, `type` and `fn` items, and only when it holds one of the first three; with functions alone it prints as an empty `enum name {}`, otherwise as `mod name;`
2. **Macros**: `macro_rules!` definitions are not extracted and macros are not expanded; the items inside a macro call are distilled only for an item-level call with braces, only the items the parser can read (or those in the `{}` blocks of a statement it cannot read, as in `cfg_if!`), and never inside `()` or `[]` calls or function bodies
3. **Attributes**: `#[derive(...)]` and other attributes on items are dropped
4. **Tuple structs**: printed without their fields
5. **Impl and trait headers**: the associated types of a trait impl print only with `--private=1`; a trait's supertraits and `const` items and the `where` clause of a struct, trait or impl header are dropped
6. **Doc comments**: `///` docs are printed with `//`
7. **Unions, `extern` blocks and `extern crate`**: not printed

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

Use descriptive trait bounds. A function's `where` clause is printed on its signature line; on a struct or impl block, prefer inline bounds, because a `where` clause on its header is not printed (Known Limitations, item 5):

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

Structure modules for clear API boundaries (inline modules print badly today, see Known Limitations; modules in their own files are processed as separate files):

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

pub struct Cache<K, V, S = RandomState>;

impl<K, V, S> Cache<K, V, S> {

    pub fn with_hasher(capacity: usize, hasher: S) -> Self

    pub fn get(&self, key: &K) -> Option<&V>

    pub fn insert(&mut self, key: K, value: V) -> Option<V>
}
</file>
```

The struct's fields are private, so it prints with `;`. The `where` clauses of the struct and impl headers are not printed (Known Limitations, item 5); written with inline bounds (`impl<K: Hash + Eq, V: Clone, S: BuildHasher> Cache<K, V, S> {`), the bounds print.

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

- `where` clauses on struct, trait and impl headers
- Inline modules, tuple structs and macro definitions
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
