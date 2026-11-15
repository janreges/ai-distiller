# Java Language Support

AI Distiller provides support for Java 8+ codebases using the [tree-sitter-java](https://github.com/tree-sitter/tree-sitter-java) parser, with basic support for modern Java features including records, sealed classes, and pattern matching.

## Overview

Java support in AI Distiller is designed to extract the essential structure of Java code while preserving type information, visibility modifiers, and relationships between classes, interfaces, and abstract classes. The distilled output maintains Java's semantic meaning while dramatically reducing token count for AI consumption.

## Core Philosophy

AI Distiller models Java code as a semantic graph, representing not just the syntax but the relationships between entities. This document explains how Java constructs are mapped to this representation, focusing on providing AI systems with a clear understanding of your codebase's structure and API surface.

## Java Version Compatibility

- **Minimum supported**: Java 8
- **Recommended**: Java 11+
- **Modern features**: Partial support for Java 14+ (records, sealed classes, pattern matching)

## Supported Java Constructs

### Foundational Elements

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Packages** | ✅ Full | Package declarations and structure |
| **Imports** | ✅ Full | Standard and static imports (in text output, imports the distilled code does not use are dropped) |
| **Classes** | ✅ Full | Including abstract, final, nested classes |
| **Interfaces** | ✅ Full | Full interface support including sealed interfaces with permits clause |
| **Enums** | ✅ Full | Enum declarations and constants |
| **Records** | ✅ Full | Record syntax, parameters, and implementations |

### Object-Oriented Constructs

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Methods** | ✅ Full | Including constructors, overloading and `throws` clauses; varargs print without `...` |
| **Fields** | ✅ Full | All modifiers (static, final, volatile, etc.) |
| **Visibility** | ✅ Full | public, private, protected, package-private |
| **Inheritance** | ⚠️ Partial | extends and implements relationships; their generic arguments are dropped |
| **Method Overriding** | ✅ Full | @Override annotations preserved |
| **Static Members** | ✅ Full | Static methods and fields |

### Type System Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Basic Types** | ✅ Full | Primitives and wrapper classes |
| **Generics** | ⚠️ Partial | Type parameters, multiple bounds and wildcards (`? extends T`, `? super T`) captured; generic arguments in `extends`/`implements` dropped |
| **Arrays** | ✅ Full | Array type declarations |
| **Annotations** | ✅ Full | Type/method/parameter annotations, @interface definitions with default values |
| **Lambda Expressions** | ➖ Bodies only | Appear only inside printed method bodies (`--implementation=1`) |
| **Method References** | ➖ Bodies only | Appear only inside printed method bodies (`--implementation=1`) |

### Visibility Rules

Java visibility in AI Distiller follows standard Java keywords in text format:
- **Public**: `public` keyword (accessible everywhere)
- **Private**: `private` keyword (class-only access), shown with `--private=1`
- **Protected**: `protected` keyword (package + subclass access), shown with `--protected=1`
- **Package-private**: no keyword (package-only access, Java default), shown with `--internal=1`

## Key Features

### 1. **Standard Java Visibility Keywords**

AI Distiller uses standard Java visibility keywords for clear representation:

```java
// Input
public class User {
    private String name;
    protected int id;
    public String getName() { return name; }
}
```

```
// Output (default version - public only)
public class User {
    public String getName();
}

// Output with --private=1 --protected=1
public class User {
    private String name;
    protected int id;
    public String getName();
}
```

### 2. **Method Signature Extraction**

Methods are cleanly extracted with parameter types and return types:

```java
// Input
public Optional<User> findUserById(String id, boolean includeDeleted) {
    // implementation
}
```

```
// Output (no implementation)
public Optional<User> findUserById(String id, boolean includeDeleted);
```

### 3. **Inheritance Relationships**

Class hierarchies and interface implementations are preserved:

```java
// Input
public class UserService extends BaseService implements Cacheable, Auditable {
    // implementation
}
```

```
// Output
public class UserService extends BaseService implements Cacheable, Auditable {
}
```

Generic arguments in these clauses are dropped: `extends BaseStore<User>` prints as `extends BaseStore`.

## Output Examples

<details open><summary>Basic Class Example</summary><blockquote>
  <details><summary>Basic.java - Source Code</summary><blockquote>
    ```java
    package com.aidi.test.basic;

    /**
     * A basic class to test fundamental parsing
     */
    public class Basic {
        private static final String GREETING_PREFIX = "Hello, ";
        
        public static void main(String[] args) {
            String world = "World";
            System.out.println(createGreeting(world));
        }
        
        private static String createGreeting(String name) {
            return GREETING_PREFIX + name;
        }
    }
    ```
  </blockquote></details>
  <details open><summary>Compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    ```
    <file path="Basic.java">
    package com.aidi.test.basic
    /**
     * A basic class to test fundamental parsing
     */

    public class Basic {
        public static void main(String[] args);
    }
    </file>
    ```
  </blockquote></details>
  <details><summary>Full version (`--public=1 --protected=1 --internal=1 --private=1 --implementation=1`)</summary><blockquote>
    ```
    <file path="Basic.java">
    package com.aidi.test.basic
    /**
     * A basic class to test fundamental parsing
     */

    public class Basic {
        private static final String GREETING_PREFIX = "Hello, ";
        public static void main(String[] args) {
            String world = "World";
            System.out.println(createGreeting(world));
        }
        private static String createGreeting(String name) {
            return GREETING_PREFIX + name;
        }
    }
    </file>
    ```
  </blockquote></details>
</blockquote></details>

<details><summary>Object-Oriented Example</summary><blockquote>
  <details><summary>SimpleOOP.java - Source Code</summary><blockquote>
    ```java
    package com.aidi.test.oop;

    import java.util.Objects;

    public class SimpleOOP {
        public final String id;
        protected String name;
        private int version;
        
        public SimpleOOP(String id, String name) {
            this.id = Objects.requireNonNull(id);
            this.name = name;
            this.version = 1;
        }
        
        public String getName() {
            return name;
        }
        
        @Override
        public String toString() {
            return "SimpleOOP{id='" + id + "', name='" + name + "'}";
        }
    }
    ```
  </blockquote></details>
  <details open><summary>Compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    ```
    <file path="SimpleOOP.java">
    package com.aidi.test.oop

    public class SimpleOOP {
        public final String id;
        public SimpleOOP(String id, String name);
        public String getName();
        @Override
        public String toString();
    }
    </file>
    ```
  </blockquote></details>
  <details><summary>Full version with all visibility (`--private=1 --protected=1 --internal=1`)</summary><blockquote>
    ```
    <file path="SimpleOOP.java">
    package com.aidi.test.oop

    public class SimpleOOP {
        public final String id;
        protected String name;
        private int version;
        public SimpleOOP(String id, String name);
        public String getName();
        @Override
        public String toString();
    }
    </file>
    ```
  </blockquote></details>
</blockquote></details>

## Recent Improvements

- **Throws clauses**: method `throws` declarations are kept (`process(@NotNull U input) throws Exception`)
- **Comments**: `//` and `/* */` comments are printed with `--comments=1`; a run of `///` lines (Java 23 Markdown docs) is one doc comment
- **Doc comments with removed members**: the Javadoc of a member that is filtered out leaves with it instead of appearing above the next member
- **Fixed Visibility Detection**: Package-private types no longer incorrectly promoted to public
- **Annotation Support**: @interface definitions with default values are displayed without redundant modifiers
- **Modern Java Features**: Records, sealed interfaces with permits clause, annotation default values
- **Parameter Annotations**: Method parameter annotations are extracted (e.g., `@NotNull U input`)
- **Generic Bounds**: All bounds in generic type constraints are shown (e.g., `T extends Number & Runnable & Serializable`)

## Known Issues

### 🔴 Critical Limitations

**Dependency Resolution**
- When public classes use package-private types (sealed interfaces, records), those dependencies are excluded from default output
- Example: `ModernJava` uses `Shape`, `Circle`, `Rectangle` but only `ModernJava` appears in public-only output
- **Solution**: Use `--internal=1` flag to include package-private types and see complete sealed interface definitions with permits clause
- Results in incomplete/non-compilable code representations in default output only

**Inheritance Resolution**  
- Inherited public methods from parent classes are not displayed in child classes
- Example: `UserStore extends BaseStore` - missing `save()` method from parent
- Public API surface appears incomplete for inherited classes

### 🟡 Major Limitations

**Generic Type Arguments in Inheritance**  
- Generic arguments in `extends` and `implements` clauses are dropped (e.g., `extends BaseStore<User>` becomes `extends BaseStore`, `implements Comparable<Point>` becomes `implements Comparable`)

**Varargs**
- A varargs parameter prints without `...` (`String... args` becomes `String args`)

**Initializer Blocks**
- Static and instance initializer blocks are not printed, also not with `--implementation=1`

### 🟢 Minor Issues

**Package Declaration Formatting**
- Package declarations are missing semicolons in output

**JavaDoc Parsing**
- JavaDoc tags like `@param` and `@return` are included in text but not structured
- HTML tags in JavaDoc comments are preserved as-is

## Best Practices

### For Optimal Results

1. **Use explicit types**: Declared types of fields, parameters and return values are what the output shows
2. **Document with JavaDoc**: `/** */` comments are printed by default (`--docstrings=0` removes them)
3. **Leverage generics**: Type parameters and bounds of methods and classes are preserved
4. **Use annotations**: Annotation arguments are extracted and preserved in output

### CLI Usage Examples

```bash
# Extract public API with JavaDoc (recommended for AI; these are the defaults)
aid src/

# Extract all members with generics and annotations, without bodies
aid MyClass.java --private=1 --protected=1 --internal=1 --stdout

# Complete code preservation including JavaDoc and comments
aid . --private=1 --protected=1 --internal=1 --implementation=1 --comments=1 -o java-full.txt
```

## Integration Examples

### With MCP Servers

The MCP server `@janreges/ai-distiller-mcp` exposes `distill_directory` and `distill_file` (setup and parameters in [`mcp-npm/README.md`](../../mcp-npm/README.md)):

```javascript
// Query Java codebase structure (public API, no implementation)
distill_directory({
  directory_path: "./src/main/java",
  include_patterns: "*.java"
})
```

### CI/CD Integration

```yaml
# GitHub Actions example
- name: Extract Java API
  run: |
    aid src/main/java -o api-surface.txt
```

## Language-Specific Tips

- **Method overloading**: All overloaded variants are captured
- **Static imports**: Preserved as regular imports in output
- **Nested classes**: Printed inside their enclosing class
- **Enum methods**: Enum constants (as `public static final` fields) and methods are both captured

## Comparison with Other Tools

| Feature | AI Distiller | JavaDoc | IDEs | AST Tools |
|---------|-------------|---------|------|-----------|
| **AI-optimized output** | ✅ | ❌ | ❌ | ❌ |
| **Visibility notation** | ✅ Java keywords | ❌ | ⚠️ Icons | ❌ |
| **Type preservation** | ✅ | ✅ | ✅ | ✅ |
| **Implementation stripping** | ✅ | ❌ | ❌ | ⚠️ |
| **Inheritance tracking** | ✅ | ⚠️ | ✅ | ✅ |
| **Compact output** | ✅ | ❌ | ❌ | ❌ |

## Troubleshooting

### Common Issues

**"JavaDoc tags not structured"**
- JavaDoc content is captured but tags like `@param` are in text form
- Solution: JavaDoc is now preserved, structured tag parsing is planned

**"Annotation arguments simplified"**
- Complex annotation expressions may lose some detail
- Solution: Most annotation arguments are now properly extracted

### Parser Errors

If parsing fails completely:
1. Check for valid Java syntax
2. Ensure file encoding is UTF-8
3. Try with simpler constructs first
4. Report complex cases as issues

## Future Enhancements

### Planned Improvements

- **Generic arguments in inheritance**: `extends BaseStore<User>`
- **Structured JavaDoc**: Parsed `@param`, `@return`, `@throws` tags
- **Varargs**: `String... args`

### Community Contributions

We welcome contributions to improve Java support:
- Generic arguments in `extends`/`implements`
- JavaDoc tag structuring
- Modern Java feature support
- Test case additions

## Contributing

To improve Java language support:

1. **Add test cases**: Add a scenario under `testdata/java/` (see [`internal/testrunner/README.md`](../../internal/testrunner/README.md))
2. **Report issues**: Document parsing failures with minimal reproductions
3. **Enhance parser**: Improve tree-sitter integration for missing features
4. **Update docs**: Keep this documentation current with capabilities

For questions or contributions, see the main project [README](../../README.md).