# C++ Language Support

AI Distiller supports C++ codebases through the [tree-sitter-cpp](https://github.com/tree-sitter/tree-sitter-cpp) grammar, compiled into the `aid` binary. It extracts classes, structs, functions, enums and their members; many modern C++ details are not printed yet (see Known Limitations).

## Overview

C++ support in AI Distiller captures the essential structure of C++ code: classes with their methods and fields, inheritance lists, free functions, out-of-class member definitions, enums and Doxygen documentation. The distilled output keeps C++ declaration syntax while optimizing for AI consumption.

## Supported C++ Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ✅ Full | Including inheritance lists and nested classes |
| **Structs** | ✅ Full | Treated similar to classes |
| **Templates** | ⚠️ Partial | `template<typename K, typename V>` is kept on classes and functions; a template parameter with a default, a non-type parameter (`int N`) and member templates are not |
| **Functions** | ✅ Full | Free functions and methods; out-of-class definitions (`int Foo::bar() {...}`) print with their qualified name |
| **Namespaces** | ⚠️ Partial | Members are printed; the `namespace` line itself is not |
| **Enums** | ⚠️ Partial | Enumerators are listed; their values (`= 2`) are dropped and a plain `enum` prints as `enum class` |
| **Typedefs/Using** | ❌ Not printed | `typedef` and `using` aliases are dropped |
| **Unions** | ⚠️ Partial | Printed as `struct` |
| **Operators** | ⚠️ Partial | Overloaded operators such as `operator=` and `operator==` are kept; `operator[]` is dropped and a conversion operator (`operator bool`) prints without a name |
| **Friend declarations** | ❌ Not printed | Friend declarations are skipped |

### Advanced Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Virtual functions** | ✅ Full | Including pure virtual (`= 0`); `override` is dropped |
| **Constructors/Destructors** | ⚠️ Partial | Printed; `= default`, `= delete`, `explicit` and `noexcept` are dropped |
| **Return types** | ⚠️ Partial | Unqualified return types (`int`, `T&`, `iterator`) are kept; a namespace-qualified one (`std::string`, `std::pair<iterator, bool>`) and a trailing return type (`auto f() -> T`) are dropped |
| **Parameter types** | ⚠️ Partial | Unqualified types are kept; a namespace-qualified type is lost (`const std::string& s` prints as `const & s`, `std::size_t n` as `auto n`), default values are dropped, an unnamed reference parameter loses its `&`, and a pack (`Args&&... args`) is dropped |
| **Const correctness** | ✅ Full | const methods and parameters |
| **Reference types** | ⚠️ Partial | `&` is kept; `&&` (rvalue reference) prints as `&` |
| **C++20 Concepts** | ❌ Not printed | `concept` definitions and `requires` clauses are dropped |
| **Attributes** | ❌ Not printed | `[[nodiscard]]`, `[[deprecated]]` are dropped |
| **Doxygen comments** | ✅ Full | `/** */`, `///`, `//!` and trailing `///<` docs print by default as `/** */` above their declaration |

## Recent Fixes

1. **Function bodies** (✅ Fixed)
   - **Issue**: `--implementation=1` and `--expand` printed no bodies
   - **Fix**: The body is printed in braces after the signature
   - **Impact**: Implementation details are available on request

2. **Out-of-class member definitions** (✅ Fixed)
   - **Issue**: `int Foo::bar() {...}` printed as a nameless `()`
   - **Fix**: The definition is named `Foo::bar` and takes the visibility of the member's in-class declaration
   - **Impact**: Definitions of private members no longer show in public-only output; `--expand=bar` expands `Foo::bar`

3. **Comments** (✅ Fixed)
   - **Issue**: `--comments` and `--docstrings` did nothing for C++
   - **Fix**: Doxygen docs print by default, `//` and `/* */` comments with `--comments=1`

## Key Features

### 1. **Template Support**

```cpp
// Input
template<typename T, typename Allocator = std::allocator<T>>
class Vector {
public:
    using value_type = T;
    using size_type = std::size_t;
    
    Vector() noexcept(noexcept(Allocator())) = default;
    explicit Vector(size_type count, const T& value = T());
    
    template<typename InputIt>
    Vector(InputIt first, InputIt last);
    
    void push_back(const T& value);
    void push_back(T&& value);
    
    template<typename... Args>
    void emplace_back(Args&&... args);
    
private:
    T* data_;
    size_type size_;
    size_type capacity_;
};
```

```
<file path="vector.hpp">
template<typename T>
class Vector {
    Vector()
    Vector(size_type count, const T& value)
    Vector(InputIt first, InputIt last)
    void push_back(const T& value)
    void push_back(T& value)
    void emplace_back()
};
</file>
```

The defaulted template parameter, the `using` aliases, `noexcept`/`= default`/`explicit`, the member templates, the default argument, the rvalue reference and the parameter pack are lost (see Known Limitations).

### 2. **Special Member Functions**

```cpp
// Input
class Widget {
public:
    Widget() = default;
    Widget(const Widget&) = delete;
    Widget(Widget&&) noexcept = default;
    Widget& operator=(const Widget&) = delete;
    Widget& operator=(Widget&&) noexcept = default;
    ~Widget() = default;
    
    [[nodiscard]] bool process() const;
    void update() && = delete;  // Only lvalues
    
    explicit operator bool() const { return valid_; }
    
private:
    bool valid_ = false;
};
```

```
<file path="widget.hpp">
class Widget {
    Widget()
    Widget(const Widget )
    Widget(Widget )
    Widget& operator=(const Widget )
    Widget& operator=(Widget )
    ~Widget()
    bool process() const
    void update()
    ()
};
</file>
```

### 3. **C++20 Concepts**

Concepts are not printed; a constrained function template keeps its `template<...>` line but not its `requires` clause:

```cpp
// Input
template<typename T>
concept Arithmetic = std::is_arithmetic_v<T>;

template<typename T>
    requires Arithmetic<T>
T add(T a, T b) {
    return a + b;
}
```

```
<file path="concepts.cpp">
template<typename T>
T add(T a, T b);
</file>
```

## Visibility Rules

C++ visibility in AI Distiller:
- **public**: Accessible from anywhere (shown by default)
- **protected**: Accessible in class and derived classes (shown with `--protected=1`)
- **private**: Accessible only within the class (shown with `--private=1`)
- **(default)**: private in class, public in struct

The text output does not print access specifiers: a private member shown with `--private=1` looks like a public one. An out-of-class definition takes the visibility of the member's declaration in its class.

## Known Limitations

1. **Trailing return types**: `auto func() -> T` prints as `func()`
2. **C++20 Concepts**: Concepts and `requires` clauses are dropped
3. **Templates**: a template parameter with a default, non-type parameters and member templates are dropped; template specializations may not be fully captured
4. **Preprocessor directives**: Macros and conditional compilation are not printed (`#include` is)
5. **Type aliases**: `typedef` and `using` are not printed
6. **Special members**: `= default`, `= delete`, `explicit`, `noexcept`, `override` and attributes are dropped
7. **Parameters**: default values are dropped, `&&` prints as `&`, an unnamed `const T&` prints as `const T `, and parameter packs are dropped
8. **Enums**: enumerator values are dropped and a plain `enum` prints as `enum class`
9. **Inheritance access**: a base class listed as `private B` prints as `public B`
10. **Access specifiers**: not printed (see Visibility Rules)
11. **Namespace-qualified types**: a type written with `::` is lost in parameters and return types: `std::string name(const std::string& s, std::size_t n);` prints as `name(const & s, auto n);`
12. **Subscript operator**: `operator[]` is not printed

## Best Practices

### 1. **Clear Template Constraints**

Use readable template constraints:

```cpp
// Good - Clear intent
template<typename T>
    requires std::is_integral_v<T>
class Counter {
    T count = 0;
};

// Also good - Traditional style
template<typename T, 
         typename = std::enable_if_t<std::is_integral_v<T>>>
class Counter {
    T count = 0;
};
```

### 2. **Explicit Access Specifiers**

Group members by access level:

```cpp
class Service {
public:
    // Public interface
    void start();
    void stop();
    
protected:
    // Extension points
    virtual void onStart();
    virtual void onStop();
    
private:
    // Implementation details
    void cleanup();
    bool running_ = false;
};
```

### 3. **RAII and Smart Pointers**

Use modern memory management:

```cpp
class ResourceManager {
public:
    using ResourcePtr = std::unique_ptr<Resource>;
    
    ResourcePtr acquire(const std::string& name);
    void release(ResourcePtr resource);
    
private:
    std::unordered_map<std::string, ResourcePtr> resources_;
};
```

## Output Examples

<details>
<summary>Complex Template Class</summary>

Input:
```cpp
template<typename Key, typename Value, 
         typename Hash = std::hash<Key>,
         typename KeyEqual = std::equal_to<Key>,
         typename Allocator = std::allocator<std::pair<const Key, Value>>>
class HashMap {
public:
    using key_type = Key;
    using mapped_type = Value;
    using value_type = std::pair<const Key, Value>;
    
    HashMap() = default;
    explicit HashMap(std::size_t bucket_count);
    
    template<typename InputIt>
    HashMap(InputIt first, InputIt last);
    
    iterator find(const Key& key);
    const_iterator find(const Key& key) const;
    
    template<typename K>
    iterator find(const K& key);
    
    std::pair<iterator, bool> insert(const value_type& value);
    
    template<typename... Args>
    std::pair<iterator, bool> emplace(Args&&... args);
    
    Value& operator[](const Key& key);
    Value& operator[](Key&& key);
    
private:
    struct Node {
        value_type data;
        Node* next;
    };
    
    std::vector<Node*> buckets_;
    std::size_t size_ = 0;
    Hash hasher_;
    KeyEqual key_equal_;
};
```

Output (default):
```
<file path="hashmap.hpp">
template<typename Key, typename Value>
class HashMap {
    HashMap()
    HashMap(auto bucket_count)
    HashMap(InputIt first, InputIt last)
    iterator find(const Key& key)
    const_iterator find(const Key& key) const
    iterator find(const K& key)
    insert(const value_type& value)
    emplace()
};
</file>
```

The private `Node` struct and fields are hidden by default. The `std::` types, the return types of `insert` and `emplace`, both `operator[]` overloads and the defaulted template parameters are lost (see Known Limitations).

</details>

## Integration Tips

### For API Documentation
```bash
# Extract public API only (the defaults)
aid include/

# Include protected members for library users
aid include/ --protected=1
```

### For Code Review
```bash
# All members without implementations
aid src/ --private=1 --protected=1 --internal=1
```

### For Architecture Analysis
```bash
# High-level overview, Doxygen docs removed
aid . --docstrings=0
```

## Future Improvements

- Full C++20 concepts support
- Trailing return types, default arguments and rvalue references
- `using`/`typedef` aliases and namespace lines
- Template specialization tracking
- Constexpr/consteval support
- Module support (C++20)
- Coroutine support

## Contributing

C++ support is actively maintained. Key areas for contribution:
- C++20/23 features
- Template metaprogramming patterns
- Complex inheritance scenarios
- Preprocessor handling

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup.
