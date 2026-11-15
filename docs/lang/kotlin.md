# Kotlin Language Support

AI Distiller supports Kotlin codebases through the [tree-sitter-kotlin](https://github.com/fwcd/tree-sitter-kotlin) grammar (fwcd, commit `d6af96d2`), compiled into the `aid` binary.

## Overview

Kotlin support in AI Distiller is designed to extract the structure of Kotlin code while preserving null safety and functional programming constructs. The distilled output keeps Kotlin's syntax for declarations while optimizing for AI consumption.

## Recent Improvements

- **Function bodies**: `--implementation=1` prints block bodies in braces and expression bodies on one line (`fun area(): Double = r * r`)
- **Comments**: KDoc (`/** */`) is printed by default; `//` and `/* */` comments with `--comments=1`
- **Delegation**: a class that delegates an interface (`class D(s: Shape) : Shape by s`) keeps its members
- **Annotated functions**: the KDoc above an annotated function is printed with it
- **Locals**: variables declared inside a function body no longer appear as top-level properties
- **Functional interfaces**: `fun interface Op` prints as `interface Op`, also when it is nested
- **Nested types**: interfaces and enums nested in a class, object, interface or enum are printed; an interface prints its properties and nested types, an enum its companion object
- **Object Declarations**: `object` keyword instead of `class`
- **Enum Values**: all enum entries are printed
- **Const Properties**: `const val` declarations

## Recent Fixes (2025-06-15)

1. **Annotation classes** (✅ Fixed)
   - **Issue**: Annotation classes were shown as regular classes
   - **Fix**: Added ModifierAnnotation and updated formatter to show `annotation class`
   - **Impact**: Proper Kotlin annotation syntax in output

2. **Interface keyword** (✅ Fixed)
   - **Issue**: Interfaces were displayed as `class` instead of `interface`
   - **Fix**: Updated formatter to check for ModifierInterface
   - **Impact**: Correct idiomatic Kotlin syntax

## Supported Kotlin Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ⚠️ Partial | Including nested classes, objects, interfaces and enums, and inner classes; type parameters, supertypes and the `open` and `inner` modifiers are not printed |
| **Data Classes** | ✅ Full | Primary constructor properties preserved |
| **Sealed Classes** | ✅ Full | Including all variants |
| **Objects** | ✅ Full | Singleton objects and companion objects (printed as `object Name`) |
| **Interfaces** | ⚠️ Partial | Functions, properties and nested types; default methods print as signatures; type parameters, supertypes and the `fun` and `sealed` modifiers are not printed |
| **Enums** | ⚠️ Partial | Entries with their arguments, methods, a companion object and nested types; the enum's constructor parameters and the comments inside the enum body are not printed, and a property declared in the enum body prints as a bare name like an entry |
| **Functions** | ✅ Full | Including suspend, inline, extension |
| **Properties** | ⚠️ Partial | Types kept; a `val` declared outside a primary constructor prints as `var` |
| **Generic Types** | ⚠️ Partial | Function type parameters are kept (`reified` is dropped); class and interface type parameters are not printed |
| **Nullable Types** | ✅ Full | `?` suffix preserved |
| **Extension Functions** | ✅ Full | Receiver type shown in name |
| **Extension Properties** | ⚠️ Partial | Missing receiver type and generics |
| **Type Aliases** | ❌ Not printed | Parsed, but the text output does not print them |
| **Annotations** | ❌ Not printed | `@Serializable`, `@Deprecated` and other annotations are dropped |
| **Lambda Types** | ✅ Full | Function types are kept as written (`f: (T) -> R`) |
| **Coroutines** | ✅ Full | `suspend` modifier preserved |
| **Inline Functions** | ✅ Full | `inline` modifier preserved |

### Visibility Modifiers

Kotlin visibility in AI Distiller:
- **public**: Default visibility (not shown explicitly)
- **private**: Member-only access, shown with `--private=1`
- **protected**: Class and subclass access, shown with `--protected=1`
- **internal**: Module-wide access, shown with `--internal=1`

## Known Issues

### Supertypes and Type Parameters
A class header prints without its supertypes and type parameters: `class A<T> : B(), C` prints as `class A`, `interface Repository<T>` as `interface Repository`.

### Extension Properties
Extension properties are not fully supported. They appear as regular properties without the receiver type:
```kotlin
// Source
val <T> List<T>.secondOrNull: T?
    get() = if (size >= 2) this[1] else null

// Current output (incorrect)
var secondOrNull: T?

// Expected output
val <T> List<T>.secondOrNull: T?
```

### Val Properties
A `val` declared outside a primary constructor prints as `var`: at the top level and in a class, object or interface body, with or without an initializer (`val size: Int = 1` prints as `var size: Int`, an abstract `val label: String` in an interface as `var label: String`). `const val` and primary constructor properties keep `val`.

### Default Parameter Values
Default parameter values in functions are not extracted.

### Annotations and Type Aliases
Annotations and `typealias` declarations are not printed.

## Key Features

### 1. **Constructor Properties**

Primary constructor properties keep `val` and `var`:

```kotlin
// Input
data class User(
    val id: Long,
    val name: String,
    var lastLoginTime: LocalDateTime?
)

// Output
data class User(val id: Long, val name: String, var lastLoginTime: LocalDateTime?) {
}
```

### 2. **Null Safety**

Nullable types are preserved with the `?` suffix:

```kotlin
// Input
fun findUser(id: Long): User? {
    return users.find { it.id == id }
}

// Output
fun findUser(id: Long): User?
```

### 3. **Sealed Class Hierarchies**

Sealed classes and all their variants are extracted (without type parameters and supertypes):

```kotlin
// Input
sealed class Result<out T> {
    data class Success<T>(val data: T) : Result<T>()
    data class Error(val message: String) : Result<Nothing>()
    object Loading : Result<Nothing>()
}

// Output
sealed class Result {
    data class Success(val data: T) {
    }
    data class Error(val message: String) {
    }
    object Loading {
    }
}
```

### 4. **Object Declarations**

Kotlin objects (singletons) are correctly identified:

```kotlin
// Input
object AppConfig {
    const val API_KEY = "xyz"
    val baseUrl = "https://api.example.com"
    
    fun initialize() { }
}

// Output
object AppConfig {
    const val API_KEY
    var baseUrl
    fun initialize()
}
```

## Output Formats

### Text Format (Recommended for AI)

The text format keeps Kotlin syntax for declarations:

```kotlin
// Input file
package com.example

import kotlinx.coroutines.*

interface Repository<T> {
    suspend fun get(id: Long): T?
    suspend fun save(item: T)
}

class UserRepository : Repository<User> {
    private val cache = mutableMapOf<Long, User>()
    
    override suspend fun get(id: Long): User? {
        return cache[id] ?: fetchFromDatabase(id)
    }
    
    override suspend fun save(item: User) {
        cache[item.id] = item
        saveToDatabase(item)
    }
    
    private suspend fun fetchFromDatabase(id: Long): User? = 
        withContext(Dispatchers.IO) {
            // Database logic
        }
}

// Output (default - public only, no implementation)
<file path="UserRepository.kt">
import kotlinx.coroutines
interface Repository {
    suspend fun get(id: Long): T?
    suspend fun save(item: T)
}
class UserRepository {
    override suspend fun get(id: Long): User?
    override suspend fun save(item: User)
}
</file>
```

## Best Practices

### 1. **Use Explicit Type Annotations**

While Kotlin has excellent type inference, explicit types help AI understand the code better:

```kotlin
// Good - explicit types
val users: List<User> = repository.getAllUsers()
val count: Int = users.size

// Less optimal - relies on inference
val users = repository.getAllUsers()
val count = users.size
```

### 2. **Leverage Kotlin Idioms**

Use Kotlin's expressive features that are well-preserved:

```kotlin
// Data classes for DTOs
data class UserDto(val id: Long, val name: String)

// Sealed classes for state
sealed class ViewState {
    object Loading : ViewState()
    data class Success(val data: List<Item>) : ViewState()
    data class Error(val message: String) : ViewState()
}

// Extension functions for utilities
fun String.isValidEmail(): Boolean = 
    contains("@") && contains(".")
```

### 3. **Structure for Clarity**

- Use meaningful names for type parameters
- Group related functionality in companion objects
- Use explicit visibility modifiers when needed
- Prefer immutability (`val` over `var`)

## Integration Examples

### Direct CLI Usage

```bash
# Generate context for AI assistants (public API, no implementation: the defaults)
aid ./src -o context.txt

# Include internal APIs
aid ./src --internal=1 -o full-api.txt
```

### CI/CD Integration

```yaml
# .github/workflows/api-check.yml
- name: Extract Public API
  run: |
    aid ./src --stdout > api.txt
    
    # Check for breaking changes
    if ! diff api-baseline.txt api.txt; then
      echo "::error::Public API has changed"
      exit 1
    fi
```

## Comparison with Other Tools

| Tool | Purpose | Kotlin Support | AI-Optimized |
|------|---------|---------------|--------------|
| **AI Distiller** | Code structure extraction | Modern Kotlin | ✅ Yes |
| Dokka | Documentation generation | Full | ❌ No |
| detekt | Static analysis | Full | ❌ No |
| ktlint | Code formatting | Full | ❌ No |

## Future Enhancements

- [ ] Supertypes and class type parameters in class headers
- [ ] Full extension property support with receiver types
- [ ] Default parameter value extraction
- [ ] Annotations and type aliases in the text output
- [ ] Inline class support
- [ ] Contracts and experimental features
- [ ] Multiplatform annotations

## Contributing

Help improve Kotlin support! Key areas:
- Extension property parsing
- Complex generic scenarios
- Coroutine flow types
- Inline/value classes
