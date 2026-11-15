# Rust Language Support

AI Distiller supports Rust codebases with a line-based parser (regular expressions, no tree-sitter). It covers the common item declarations: structs, enums, traits, impl blocks, functions, constants and use statements.

## Overview

Rust support in AI Distiller captures the essential structure of Rust code including ownership annotations, lifetime parameters, trait bounds, and async functions. Because the parser reads one line at a time, a declaration written across several lines (a parameter list or a `where` clause) can be lost; the limitations are listed below.

## Supported Rust Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Structs** | ⚠️ Partial | Named-field structs with generics; a tuple struct prints without its fields (`pub struct Meters;`) |
| **Enums** | ✅ Full | Variants with tuple or struct payloads and discriminants (`Ok = 0`), also on one line |
| **Traits** | ⚠️ Partial | Methods (default methods with their bodies under `--implementation=1`) and associated types; supertraits (`: Sized`) are dropped |
| **Implementations** | ⚠️ Partial | Inherent and trait impls with their methods; the impl's own generics (`impl<T: Display>`) and associated types are dropped |
| **Functions** | ⚠️ Partial | Free functions and methods; a function whose parameter list spans several lines is dropped |
| **Type Aliases** | ⚠️ Partial | Printed with their value; the alias's type parameters are dropped (`type Map<K>` prints as `type Map`) |
| **Constants** | ✅ Full | `const` and `static`, names as written |
| **Modules** | ❌ Broken | An inline `mod name { ... }` prints as an empty `enum name {}` |
| **Use statements** | ⚠️ Partial | Paths, groups and globs; an `as` rename prints garbled |
| **Macros** | ❌ Not extracted | `macro_rules!` definitions are skipped |
| **Async/Await** | ✅ Full | `async fn` keeps `async` |
| **Unsafe** | ⚠️ Partial | The `unsafe` keyword is dropped |

### Type System Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Lifetimes** | ✅ Full | In function signatures, including `'static`; an impl block's own lifetime parameter (`impl<'a>`) is dropped |
| **References** | ✅ Full | `&T`, `&mut T`, `&'a T` |
| **Generic parameters** | ✅ Full | Type, lifetime and const generics on functions and structs |
| **Trait bounds** | ⚠️ Partial | Inline bounds, and a function's `where` clause (printed on the signature line); a multi-line `where` clause on a struct or impl block loses its fields or methods (see Known Limitations) |
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

impl Parser<'a> {

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

impl Serialize for T {

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

1. **Multi-line declarations**: a function whose parameter list spans several lines is dropped, and an item with a multi-line `where` clause loses its body: a struct prints without fields, an impl block without methods (see the example below)
2. **Modules**: an inline `mod name { ... }` prints as an empty `enum name {}`
3. **Macro definitions**: `macro_rules!` definitions are not extracted, and macros are not expanded
4. **Attributes**: `#[derive(...)]` and other attributes on items are dropped
5. **Tuple structs**: printed without their fields
6. **`use ... as ...` renames**: printed garbled (`use a::b::C as D;` prints as `use a::b::C::{a::b::C as D};`)
7. **Impl and trait headers**: the impl's generics, its associated types and a trait's supertraits are dropped
8. **Doc comments**: `///` docs are printed with `//`

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

Use descriptive trait bounds. A function's `where` clause is printed on its signature line; on a struct or impl block, prefer inline bounds, because a multi-line `where` clause there loses the fields or methods (Known Limitations, item 1):

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

impl Cache<K, V, S> {
}
</file>
```

The multi-line `where` clauses make the parser lose the struct's fields and the impl's methods (Known Limitations, item 1). Written with inline bounds (`impl<K: Hash + Eq, V: Clone, S: BuildHasher> Cache<K, V, S> {`), the methods are kept.

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

- Multi-line signatures and `where` clauses
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
