# PHP Language Support

AI Distiller supports PHP 7.4+ codebases through the [tree-sitter-php](https://github.com/tree-sitter/tree-sitter-php) grammar, compiled into the `aid` binary. It handles PHP 8.x features including enums, attributes and union types, and reads PSR-19 PHPDoc tags; the gaps are listed under Known Issues.

## Overview

PHP support in AI Distiller is designed to extract the essential structure of PHP code while preserving type information, visibility modifiers, and API contracts. The distilled output maintains PHP's semantic meaning while dramatically reducing token count for AI consumption.

## Supported PHP Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ✅ Full | Including abstract, final, readonly (8.2+) |
| **Interfaces** | ✅ Full | Multiple inheritance supported |
| **Traits** | ⚠️ Partial | Printed with the `trait` keyword; a class's `use A, B;` prints as one `use` line per trait; an `{ ... as ... }` conflict-resolution block is not printed |
| **Enums** | ⚠️ Partial | Pure and backed enums (8.1+) with their case values; enum constants and methods print after the enum at file level |
| **Functions** | ✅ Full | Global functions with type hints |
| **Methods** | ✅ Full | Including magic methods, final, abstract |
| **Properties** | ✅ Full | Typed, readonly and promoted properties; `@property` tags stay in the class docblock |
| **Constants** | ⚠️ Partial | Class constants with visibility and value; interface constants are not printed |
| **Namespaces** | ✅ Full | `use` statements print as written: aliases (`use Foo\Bar as Baz;`), `use function`, `use const` and groups (`use Foo\{A, B as C};`); `use A, B;` prints one line per name |
| **Attributes** | ✅ Full | PHP 8.0+ attributes on classes and methods |

### Type System Features

| Feature | Support Level | Notes |
|---------|--------------|-------|
| **Type declarations** | ✅ Full | Parameters, returns, properties |
| **Union types** | ✅ Full | PHP 8.0+ `string\|int` |
| **Intersection types** | ✅ Full | PHP 8.1+ `Countable&Iterator` |
| **Nullable types** | ✅ Full | `?string` syntax |
| **Array types** | ✅ Full | Via PHPDoc: `array<K,V>`, `list<T>` (printed as `T[]`); a one-line array shape (`array{id: int}`) prints in the signature, a multi-line one stays in the docblock |
| **Advanced types** | ✅ Full | `class-string<T>`, `key-of<>`, `value-of<>` from `@param` and `@return` |
| **Default values** | ✅ Full | Parameter defaults preserved |

### PSR-19 PHPDoc Support

| Annotation | Support Level | Notes |
|------------|--------------|-------|
| **@property** | ✅ Full | Read into virtual properties; the output shows them in the class docblock, not as separate lines |
| **@property-read** | ✅ Full | As `@property` |
| **@property-write** | ✅ Full | As `@property` |
| **@method** | ✅ Full | Creates virtual methods (shown as actual methods in output) |
| **@deprecated** | ✅ Full | Makes the class docblock an API docblock (see below) |
| **@api** / **@internal** | ✅ Full | On a class: sets public or internal visibility |
| **@param** | ✅ Full | Enhanced parameter types |
| **@return** | ✅ Full | Enhanced return types |
| **@throws** | ✅ Full | Kept in the docblock |
| **@template** | ⚠️ Partial | Kept in the docblock only |
| **@psalm-type** / **@phpstan-type** | ⚠️ Partial | Kept in the docblock only |

### Visibility Rules

PHP visibility in AI Distiller follows standard PHP conventions:
- **Public**: `public` keyword, or no modifier (`function f()` and `var $x` are public); shown by default
- **Protected**: `protected` keyword (shown with `--protected=1`)
- **Private**: `private` keyword (shown with `--private=1`)
- **Internal**: no PHP keyword; a class whose docblock has `@internal` counts as internal (shown with `--internal=1`)

## Key Features

### 1. **Magic Property Support (PSR-19)**

AI Distiller reads PHPDoc `@property*` annotations as virtual properties. The class docblock that declares them is always printed, so the magic properties are visible even when `__get`/`__set` are not:

```php
<?php
/**
 * @property-read int $id Auto-generated ID
 * @property string $name User's full name
 * @property-write array<string, mixed> $metadata
 */
class User {
    public function __get(string $key): mixed { /* ... */ }
    public function __set(string $key, mixed $value): void { /* ... */ }
}
```

```
<file path="user.php">
/**
 * @property-read int $id Auto-generated ID
 * @property string $name User's full name
 * @property-write array<string, mixed> $metadata
 */
class User {
}
</file>
```

### 2. **Enum Support with Values**

PHP 8.1+ enums are properly displayed with the `enum` keyword and their case values are always shown:

```php
<?php
enum Status: string {
    case DRAFT = 'draft';
    case PUBLISHED = 'published';
    case ARCHIVED = 'archived';
}
```

```
<file path="status.php">
enum Status: string {
    case DRAFT = 'draft';
    case PUBLISHED = 'published';
    case ARCHIVED = 'archived';
}
</file>
```

### 3. **Advanced Type Preservation**

Complex PHPDoc types are fully preserved for AI understanding:

```php
<?php
class Processor {
    /**
     * @param array{
     *   id: int,
     *   name: non-empty-string,
     *   tags: non-empty-list<string>,
     *   meta?: array<string, mixed>
     * } $data
     * @return class-string<Model>
     */
    public function process(array $data): string {}
}
```

```
<file path="process.php">
class Processor {
    /**
     * @param array{
     * id: int,
     * name: non-empty-string,
     * tags: non-empty-list<string>,
     * meta?: array<string, mixed>
     * } $data
     * @return class-string<Model>
     */
    public process(array $data): class-string<Model>
}
</file>
```

### 4. **Trait Support**

PHP traits are displayed with the `trait` keyword (not as classes). The `use Timestampable, Loggable;` line inside the class is printed as one `use` per trait:

```php
<?php
trait Timestampable {
    private ?DateTime $createdAt = null;
    
    public function touch(): void {
        $this->createdAt = new DateTime();
    }
}

class User {
    use Timestampable, Loggable;
    
    public string $name;
}
```

```
<file path="trait.php">
trait Timestampable {
    public touch(): void
}
class User {
    use Timestampable;
    use Loggable;
    public string $name;
}
</file>
```

### 5. **Docblock Handling**

- Class, method, property and function docblocks print by default; `--docstrings=0` removes them
- A class docblock with an API-defining tag (`@property*`, `@method`, `@deprecated`, `@api`, `@internal`, `@throws`) is printed even with `--docstrings=0`
- A docblock whose text fits on one line prints on one line (`/** User service */`)
- Regular `//`, `#` and `/* */` comments print with `--comments=1`

## Output Formats

### Text Format (Recommended for AI)

The text format is optimized for AI comprehension with minimal syntax:
- Visibility keyword on every member (`public`, `protected`, `private`)
- Methods without the `function` keyword (`public send(...): Result|false`)
- Properties and constants in PHP syntax (`public readonly string $name;`)
- A constructor without parameters is not printed

<details open>
<summary>Example: PSR-19 Magic Properties</summary>
<blockquote>

<details>
<summary>Input: `magic_properties.php`</summary>
<blockquote>

```php
<?php

namespace App\Models;

/**
 * Active Record model with magic properties
 * 
 * @property-read int $id Primary key
 * @property-read \DateTime $createdAt Creation timestamp
 * @property string $name Full name
 * @property string $email Email address
 * @property-write string $password Hashed password (write-only)
 * @property-read array<string, mixed> $attributes All attributes
 * 
 * @method static self|null find(int $id)
 * @method static self[] findAll()
 * @method bool save()
 */
abstract class ActiveRecord {
    protected array $data = [];
    
    public function __construct(array $attributes = []) {
        $this->fill($attributes);
    }
    
    public function __get(string $name): mixed {
        return $this->data[$name] ?? null;
    }
    
    public function __set(string $name, mixed $value): void {
        $this->data[$name] = $value;
    }
    
    protected function fill(array $attributes): void {
        $this->data = $attributes;
    }
}

class User extends ActiveRecord {
    protected string $table = 'users';
    
    public function getFullName(): string {
        return $this->name;
    }
    
    public function isAdmin(): bool {
        return $this->role === 'admin';
    }
}
```

</blockquote>
</details>

<details open>
<summary>Default output (public only, no implementation)</summary>
<blockquote>

```
<file path="magic_properties.php">
namespace App\Models;
/**
 * Active Record model with magic properties
 * 
 * @property-read int $id Primary key
 * @property-read \DateTime $createdAt Creation timestamp
 * @property string $name Full name
 * @property string $email Email address
 * @property-write string $password Hashed password (write-only)
 * @property-read array<string, mixed> $attributes All attributes
 * 
 * @method static self|null find(int $id)
 * @method static self[] findAll()
 * @method bool save()
 */
abstract class ActiveRecord {
    public static find(int $id): self|null
    public static findAll(): self[]
    public save(): bool
    public __construct(array $attributes = [])
}
class User extends ActiveRecord {
    public getFullName(): string
    public isAdmin(): bool
}
</file>
```

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Modern PHP 8 Features</summary>
<blockquote>

<details>
<summary>Input: `modern_features.php`</summary>
<blockquote>

```php
<?php

declare(strict_types=1);

namespace App\Services;

use App\Contracts\{Cacheable, Loggable};
use App\Traits\{TimestampableTrait, ValidatableTrait};
use App\Enums\Permission;

/**
 * User service with modern PHP 8 features
 */
#[\Attribute(\Attribute::TARGET_CLASS)]
class Service {
    public function __construct(
        public readonly string $name,
        public readonly int $version = 1
    ) {}
}

trait TimestampableTrait {
    private ?DateTime $createdAt = null;
    private ?DateTime $updatedAt = null;
    
    public function touch(): void {
        $this->updatedAt = new DateTime();
        if ($this->createdAt === null) {
            $this->createdAt = $this->updatedAt;
        }
    }
}

#[Service(name: 'notifications', version: 2)]
class NotificationService implements Cacheable, Loggable {
    use TimestampableTrait, ValidatableTrait;
    public function __construct(
        private readonly DatabaseConnection $db,
        private readonly CacheInterface $cache,
        private LoggerInterface $logger,
        private bool $debug = false
    ) {}
    
    public function send(
        string|Email $message,
        User|string $recipient,
        ?array $options = null
    ): Result|false {
        // Implementation
        return new Result(true);
    }
    
    public function hasPermission(
        User $user,
        Permission $permission
    ): bool {
        return match($permission) {
            Permission::READ => true,
            Permission::WRITE => $user->isAdmin(),
            Permission::DELETE => $user->isSuperAdmin(),
        };
    }
}

enum Permission: string {
    case READ = 'read';
    case WRITE = 'write';
    case DELETE = 'delete';
}

interface CacheInterface {
    public function get(string $key): mixed;
    public function set(string $key, mixed $value, ?int $ttl = null): bool;
}
```

</blockquote>
</details>

<details open>
<summary>Default output (public only, no implementation)</summary>
<blockquote>

```
<file path="modern_features.php">
namespace App\Services;
use App\Contracts\{Cacheable, Loggable};
use App\Traits\{TimestampableTrait, ValidatableTrait};
use App\Enums\Permission;
/** User service with modern PHP 8 features */
#[\Attribute(\Attribute::TARGET_CLASS)]
class Service {
    public readonly string $name;
    public readonly int $version;
    public __construct(string $name, int $version = 1)
}
trait TimestampableTrait {
    public touch(): void
}
#[Service(name: 'notifications', version: 2)]
class NotificationService implements Cacheable, Loggable {
    use TimestampableTrait;
    use ValidatableTrait;
    public __construct(DatabaseConnection $db, CacheInterface $cache, LoggerInterface $logger, bool $debug = false)
    public send(string|Email $message, User|string $recipient, ?array $options = null): Result|false
    public hasPermission(User $user, Permission $permission): bool
}
enum Permission: string {
    case READ = 'read';
    case WRITE = 'write';
    case DELETE = 'delete';
}
interface CacheInterface {
    public get(string $key): mixed
    public set(string $key, mixed $value, ?int $ttl = null): bool
}
</file>
```

The import filter keeps a `use` statement while one of its names (the alias, else the last name segment) appears in the output; a group stays whole when one member is used. The private promoted properties of `NotificationService` are hidden by default.

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Complex Type Annotations</summary>
<blockquote>

<details>
<summary>Input: `complex_types.php`</summary>
<blockquote>

```php
<?php

namespace App\Types;

/**
 * Repository with complex type annotations
 */
class Repository {
    /**
     * Find entities by criteria
     * 
     * @param array{
     *   where?: array<string, mixed>,
     *   orderBy?: array<string, 'ASC'|'DESC'>,
     *   limit?: positive-int,
     *   offset?: non-negative-int
     * } $criteria
     * @return list<Entity>
     */
    public function findBy(array $criteria): array {
        // Implementation
    }
    
    /**
     * @param class-string<T> $className
     * @param array<string, mixed> $data
     * @return T
     * @template T of Entity
     */
    public function hydrate(string $className, array $data): object {
        return new $className($data);
    }
    
    /**
     * @param callable(Entity): bool $predicate
     * @return Entity|null
     */
    public function findOneBy(callable $predicate): ?Entity {
        // Implementation
    }
    
    /**
     * @param non-empty-array<int> $ids
     * @return array<int, Entity>
     */
    public function findByIds(array $ids): array {
        // Implementation
    }
    
    /**
     * @param key-of<self::ALLOWED_FIELDS> $field
     * @param value-of<self::ALLOWED_VALUES> $value
     */
    public function validateField(string $field, mixed $value): bool {
        // Implementation
    }
    
    public const ALLOWED_FIELDS = [
        'name' => true,
        'email' => true,
        'status' => true
    ];
    
    public const ALLOWED_VALUES = [
        'active',
        'inactive',
        'pending'
    ];
}
```

</blockquote>
</details>

<details open>
<summary>Default Output</summary>
<blockquote>

```
<file path="complex_types.php">
namespace App\Types;
/** Repository with complex type annotations */
class Repository {
    /**
     * Find entities by criteria
     *
     * @param array{
     * where?: array<string, mixed>,
     * orderBy?: array<string, 'ASC'|'DESC'>,
     * limit?: positive-int,
     * offset?: non-negative-int
     * } $criteria
     * @return list<Entity>
     */
    public findBy(array $criteria): Entity[]
    /**
     * @param class-string<T> $className
     * @param array<string, mixed> $data
     * @return T
     * @template T of Entity
     */
    public hydrate(class-string<T> $className, array<string, mixed> $data): T
    /**
     * @param callable(Entity): bool $predicate
     * @return Entity|null
     */
    public findOneBy(callable(Entity): bool $predicate): ?Entity
    /**
     * @param non-empty-array<int> $ids
     * @return array<int, Entity>
     */
    public findByIds(non-empty-array<int> $ids): array<int, Entity>
    /**
     * @param key-of<self::ALLOWED_FIELDS> $field
     * @param value-of<self::ALLOWED_VALUES> $value
     */
    public validateField(key-of<self::ALLOWED_FIELDS> $field, value-of<self::ALLOWED_VALUES> $value): bool
    public const ALLOWED_FIELDS = [
        'name' => true,
        'email' => true,
        'status' => true
    ];
    public const ALLOWED_VALUES = [
        'active',
        'inactive',
        'pending'
    ];
}
</file>
```

</blockquote>
</details>

</blockquote>
</details>

## Known Issues

### Critical Issues

1. **Interface and enum members** (🔴 Bug)
   - **Issue**: Interface constants are not printed; an enum's constants and methods are printed after the enum, at file level and without visibility
   - **Example**: `enum E: int { case A = 1; const D = self::A; public function label(): string {} }` prints `enum E: int { case A = 1; }` followed by `const D = self::A` and `label(): string`

2. **Empty constructors hidden** (🟢 Intentional)
   - **Behavior**: Constructors without parameters are not shown
   - **Rationale**: Reduces noise in output

### Minor Issues

3. **Template annotations** (🟢 Minor)
   - **Issue**: `@template` tags stay in the docblock; they do not change the signature
   - **Impact**: Generic type information in docblock only

4. **Multi-line array shapes** (🟡 Minor)
   - **Issue**: An array shape written over several docblock lines stays in the docblock; the signature keeps the native type (`array $data`, see the `process.php` example above)
   - **Example**: a one-line shape is used instead: `@param array{id: int, name: string} $data` prints `save(array{id: int, name: string} $data)`

5. **Docblock indentation** (🟡 Minor)
   - **Issue**: A member docblock loses the indentation inside its lines (`*   id: int` prints as `* id: int`)

## Best Practices

### 1. **Use PSR-19 Annotations for Virtual APIs**

Define public APIs through PHPDoc when using magic methods:

```php
/**
 * @property-read int $id
 * @property string $name  
 * @method static self create(array $data)
 */
class Model {
    // Magic methods implementation
}
```

### 2. **Leverage PHP 8+ Features**

Modern PHP features provide better type safety and cleaner code:

```php
// Constructor property promotion
public function __construct(
    private readonly LoggerInterface $logger,
    private CacheInterface $cache,
    private bool $debug = false
) {}

// Union types instead of mixed
public function process(string|array $data): Result|false

// Enums with backed values
enum Status: string {
    case ACTIVE = 'active';
    case INACTIVE = 'inactive';
}
```

### 3. **Document Array Types Precisely**

Use PHPDoc to specify array structures:

```php
/**
 * @param Product[] $products Simple array of products
 * @param array<string, Product> $indexed Associative array
 * @param list<string> $tags Non-empty list
 * @param array{id: int, name: string} $user Shape definition
 */
```

### 4. **Use Traits for Code Reuse**

Traits are displayed with the `trait` keyword:

```php
trait Timestampable {
    use LoggerAwareTrait; // Traits can use other traits
    
    private ?DateTime $createdAt = null;
    
    public function touch(): void {
        $this->createdAt = new DateTime();
    }
}

class User {
    use Timestampable, SoftDeletes;
}
```

### 5. **Structure for AI Consumption**

- Keep classes under 500 lines
- Use meaningful names (preserved in output!)
- Group related functionality in namespaces
- Document complex types in PHPDoc
- Implementation details are hidden by default (`--implementation=0`)

## Integration Examples

### Direct CLI Usage

```bash
# Extract public API for AI context (the defaults)
aid ./src --output api.txt

# Include all members but no implementation
aid ./src --private=1 --protected=1

# Full extraction with implementations
aid ./src --private=1 --protected=1 --implementation=1

# Extract specific directory
aid ./src/Models --stdout > models.txt
```

### MCP Integration

When using AI Distiller through MCP, Claude can directly access your PHP codebase. The MCP server `@janreges/ai-distiller-mcp` provides tools such as `distill_directory`, `distill_file` and `list_files`; setup for Claude Code, Claude Desktop and other clients, and the parameters of every tool, are in [`mcp-npm/README.md`](../../mcp-npm/README.md):

```bash
claude mcp add aid -- npx -y @janreges/ai-distiller-mcp
```

**Example Conversation:**

> **You**: Analyze our user authentication system and suggest improvements for adding OAuth support.

> **Claude**: I'll analyze your authentication system structure using AI Distiller.

*Claude calls:*
```
distill_directory({ directory_path: "src/Auth", include_private: false, include_implementation: false })
distill_file({ file_path: "src/Auth/AuthManager.php" })
```

*Claude receives the distilled structure and responds:*

> Based on the analysis, your authentication system has:
>
> **Current Structure:**
> ```
> AuthManager:
>   public authenticate(string $username, string $password): ?User
>   public createSession(User $user): Session
>   public validateToken(string $token): bool
>   
> User:
>   @property-read int $id
>   @property string $email
>   @property-write string $password
>   public verifyPassword(string $password): bool
> ```
>
> **OAuth Integration Recommendations:**
>
> 1. Create an `AuthProvider` interface:
> ```php
> interface AuthProvider {
>     public function authenticate(array $credentials): ?User;
>     public function getUserInfo(string $token): array;
> }
> ```
>
> 2. Implement OAuth providers:
> ```php
> class GoogleOAuthProvider implements AuthProvider { }
> class GitHubOAuthProvider implements AuthProvider { }
> ```
>
> 3. Extend `AuthManager` to support multiple providers...

### With Laravel/Symfony

```bash
# Extract Laravel models structure
aid app/Models --stdout > models-api.txt

# Symfony entities with private and protected members
aid src/Entity --private=1 --protected=1 --stdout > entities-full.txt

# Controllers public API only
aid src/Controller --stdout > controllers-api.txt
```

### CI/CD Integration

```yaml
# .github/workflows/api-check.yml
- name: Check API Surface
  run: |
    aid src/ --stdout > api-current.txt
    diff api-baseline.txt api-current.txt || {
      echo "API surface changed! Review the differences:"
      diff -u api-baseline.txt api-current.txt
      exit 1
    }
```

### Composer Scripts

```json
{
  "scripts": {
    "analyze": "aid src/ --stdout > structure.txt",
    "analyze:api": "aid src/ -o api.txt",
    "analyze:full": "aid src/ --private=1 --protected=1 --implementation=1 -o full.txt"
  }
}
```

## Language-Specific Tips

1. **Magic Properties Best Practices**:
   ```php
   /**
    * Always document magic properties
    * @property-read int $computed This is computed dynamically
    * @property-write array $bulk Write-only for bulk updates
    */
   ```

2. **Enum Usage**:
   ```php
   // Backed enums are fully supported
   enum Status: string {
       case ACTIVE = 'active';
   }
   
   // Use match expressions with enums
   return match($status) {
       Status::ACTIVE => 'Running',
   };
   ```

3. **Type Documentation**:
   ```php
   /**
    * Document complex types in PHPDoc
    * @param array{
    *   id: int,
    *   items: list<Product>,
    *   total: float
    * } $order
    */
   ```

## @method Tag Behavior

The `@method` PHPDoc tags are parsed and displayed as actual methods in the output. This is by design to make magic methods visible to AI systems, but it means:

1. **Duplication**: If you have both a docblock with `@method` tags and the actual method implementation, both will appear
2. **Parameter defaults**: Default values in `@method` tags (e.g., `array $options = []`) may not be parsed correctly and could show as `$[]`

Example:
```php
<?php
/**
 * @method static User|null find(int $id)
 * @method void sendNotification(string $message, array $options = [])
 */
class User { }
```

Will output:
```
<file path="user.php">
/**
 * @method static User|null find(int $id)
 * @method void sendNotification(string $message, array $options = [])
 */
class User {
    public static find(int $id): User|null
    public sendNotification(string $message, array $options = $[]): void
}
</file>
```

This is considered a feature for AI comprehension, as it makes magic methods explicit in the API surface.

## Comparison with Other Tools

| Tool | Purpose | PHP Support | AI-Optimized |
|------|---------|-------------|--------------|
| **AI Distiller** | Code structure extraction | PHP 7.4-8.3 (gaps above) | ✅ Yes |
| PHPStan | Static analysis | Full | ❌ No |
| PHP-Parser | AST generation | Full | ❌ No |
| phpDocumentor | Documentation | Partial | ❌ No |
| PHP CS Fixer | Code formatting | N/A | ❌ No |

## Troubleshooting

### "Magic properties not showing"

Ensure your class has @property annotations in its docblock:
```php
/**
 * @property-read int $id
 */
class Model { }
```

### "A use statement is missing"

The text format's import filter removes a `use` statement whose name (the alias, else the last name segment) does not appear in the rest of the output, for example a class used only in a method body that is not printed. Add `--filter-imports=false` to keep every `use`.

### "Docblocks missing"

Docblocks print by default. Check that the command does not pass `--docstrings=0`.

## Future Enhancements

- [ ] Capture multi-line `@param` array shapes in method signatures
- [ ] Anonymous class support
- [ ] Closure and arrow function type extraction
- [ ] `@template` as API-defining tag
- [ ] Trait conflict resolution (`as`, `insteadof`)
- [ ] Interface constants, and enum constants and methods inside the enum
- [ ] Property hooks (PHP 8.4+)

## Contributing

Help improve PHP support! See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

Key areas needing help:
- Complex PHPDoc patterns
- Framework-specific patterns
- Performance optimizations
- Real-world test cases
