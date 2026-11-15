# C# Language Support

AI Distiller supports C# codebases through the [tree-sitter-c-sharp](https://github.com/tree-sitter/tree-sitter-c-sharp) grammar (v0.23.5), compiled into the `aid` binary. It extracts namespaces, types, methods, properties and fields; the gaps are listed under Known Issues.

## Overview

C# support in AI Distiller extracts the structure of .NET code with method signatures, generic constraints and access modifiers. The distilled output keeps C# declaration syntax while optimizing for AI consumption.

## Recent Improvements

1. **Combined access modifiers** (✅ Fixed)
   - `protected internal` and `private protected` are recognised and printed. A combined modifier shows when the flag of either of its words is on: `protected internal` with `--protected=1` or `--internal=1`, `private protected` with `--private=1` or `--protected=1`
   - `--exclude-items=internal` removes `internal` members (it used to remove private ones)

2. **Comments** (✅ Fixed)
   - XML documentation comments (`///`) print by default and are removed with `--docstrings=0`; `//` and `/* */` comments print with `--comments=1`

3. **Enum members** (✅ Fixed)
   - Enum members print with their values

4. **Operators and method parameters**
   - User-defined operators such as `operator +` are extracted (conversion operators are not)
   - Method parameters print type first (`T instance`) and method constraints keep their type arguments (`INumber<T>`)

## Supported C# Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ⚠️ Partial | Type parameters and constraints are kept; generic arguments in the base list are dropped (`: IRepository<User>` prints as `: IRepository`) |
| **Interfaces** | ⚠️ Partial | Methods are printed; properties and events count as private (shown only with `--private=1`, an event as `private dynamic Changed;`) and variance (`in`, `out`) is dropped |
| **Structs** | ⚠️ Partial | Printed without `public` and `readonly` (`public readonly struct R` prints as `struct R`) |
| **Records** | ⚠️ Partial | Primary constructor parameters are mangled in some cases and `record struct` prints as `record` (see Known Issues) |
| **Properties** | ⚠️ Partial | The accessor list is kept; accessor visibility, `init` (printed as `set`), `required` and initializers are dropped |
| **Methods** | ✅ Full | Including async and extension methods; default parameter values are dropped |
| **Generic Constraints** | ✅ Full | On types and methods, with type arguments (`where T : INumber<T>`) |
| **Nullable Reference Types** | ✅ Full | `#nullable` directives and `T?` types |
| **Attributes** | ⚠️ Partial | Attributes on types and record parameters are printed (removed with `--annotations=0`); attributes on methods, properties and parameters are dropped |
| **Pattern Matching** | ➖ In bodies | Printed as part of method bodies with `--implementation=1` |
| **Global Usings** | ⚠️ Partial | Printed as a plain `using` |
| **File-scoped Namespaces** | ✅ Full | `namespace Foo;` syntax |
| **Enums** | ⚠️ Partial | Members with their values; the underlying type (`: byte`) and `[Flags]` are dropped |
| **Delegates** | ❌ Not printed | Delegate declarations are dropped |
| **Events** | ⚠️ Partial | A class event prints as a `dynamic` field (`public dynamic Updated;`); an interface event counts as private and prints only with `--private=1` (`private dynamic Changed;`) |
| **Operators** | ⚠️ Partial | User-defined operators (`operator +`); conversion operators (`implicit operator int`) are dropped |
| **Indexers** | ❌ Not printed | `this[...]` indexers are dropped |
| **Tuple Types** | ✅ Full | Named and unnamed tuples |
| **Local Functions** | ➖ In bodies | Printed as part of method bodies with `--implementation=1` |
| **Init-only Properties** | ⚠️ Partial | `{ get; init; }` prints as `{ get; set; }` |

### Visibility Rules

C# visibility in AI Distiller uses the full keyword representation:
- **public**: Accessible from any code (shown by default)
- **private**: Accessible only within the containing type (shown with `--private=1`)
- **protected**: Accessible within the type and derived types (shown with `--protected=1`)
- **internal**: Accessible within the same assembly (shown with `--internal=1`)
- **protected internal**: Accessible within assembly or derived types (shown with `--protected=1` or `--internal=1`)
- **private protected**: Accessible within the containing class or derived types in the same assembly (shown with `--private=1` or `--protected=1`)

## Known Issues

### 🔴 Critical Limitations

**Record Primary Constructors**
- A parameter whose type is not a built-in type prints name first: `record P(Guid Id)` prints as `P(Id Guid)`
- A nullable parameter type is dropped: `string? Name` prints as `Name`
- `record struct` prints as `record`
- An expression-bodied property declared in a record with a body (`public bool IsValid => ...;`) is added to the parameter list

**Generic Arguments in Base Lists**
- Generic arguments in base class and interface lists are dropped (e.g., `: IRepository` instead of `: IRepository<User>`)

**Interface Properties and Events**
- Interface properties and events are treated as private, so the default output omits them (`--private=1` shows them, with a `private` keyword: `private int Count { get; }`, `private dynamic Changed;`)

**Events, Delegates, Indexers and Conversion Operators**
- A class event is shown as a `dynamic` field; delegates, indexers and conversion operators are not printed

### 🟡 Major Limitations

**Properties**
- Different visibility on getters/setters is not preserved (e.g., `{ get; protected set; }` prints as `{ get; set; }`)
- `init` prints as `set`; `required` and property initializers (`= 30`) are dropped
- An expression-bodied property prints garbled: `public string Name => "n";` prints as `public string Name = => "n" { get; }`

**Struct Modifiers**
- `public` and `readonly` are not printed on structs

**Enums**
- The underlying type (`: byte`) and `[Flags]` are dropped

**Optional Parameters**
- Default parameter values are not shown in method signatures

**Attributes**
- Attributes on methods, properties and parameters are dropped

**Generic Variance**
- Variance modifiers (`in`, `out`) on generic parameters are not preserved

**Usings**
- The text output drops a `using` whose namespace it does not find in the distilled text. The check is a heuristic: `using System;` and `using Microsoft.Extensions.Logging;` can disappear although `ILogger` is used. `--filter-imports=false` keeps every `using`

## Key Features

### 1. **Generic Type Information**

Type parameters and constraints are kept; the generic argument in the base list is dropped:

```csharp
// Input
public interface IRepository<T> where T : class, IEntity, new()
{
    Task<T?> GetByIdAsync(int id);
    IAsyncEnumerable<T> GetAllAsync();
}

public class UserRepository : IRepository<User>
{
    public async Task<User?> GetByIdAsync(int id) { ... }
    public async IAsyncEnumerable<User> GetAllAsync() { ... }
}
```

```
<file path="repo.cs">
public interface IRepository<T> where T : class, IEntity, new() {
    public Task<T?> GetByIdAsync(int id);
    public IAsyncEnumerable<T> GetAllAsync();
}
public class UserRepository : IRepository {
    public async Task<User?> GetByIdAsync(int id);
    public async IAsyncEnumerable<User> GetAllAsync();
}
</file>
```

### 2. **Records and Nullable Context**

Records with attributed primary constructor parameters and `#nullable` directives are printed; the `init` accessor prints as `set` and the initializer is dropped:

```csharp
// Input
#nullable enable

public record UserDto(
    [property: Required] string Username,
    [property: EmailAddress] string Email)
{
    public DateTime CreatedAt { get; init; } = DateTime.UtcNow;
}
```

```
<file path="dto.cs">
#nullable enable


public record UserDto(
    [property: Required] string Username,
    [property: EmailAddress] string Email) {
    public DateTime CreatedAt { get; set; }
}
</file>
```

### 3. **Property Syntax**

Properties are displayed with their accessor list; the initializer, `required`, `init` and the accessor's `private` are lost:

```csharp
// Input
public class Configuration
{
    public string ConnectionString { get; set; }
    public int Timeout { get; } = 30;
    public bool IsEnabled { get; private set; }
    public required string ApiKey { get; init; }
}
```

```
<file path="Configuration.cs">
public class Configuration {
    public string ConnectionString { get; set; }
    public int Timeout { get; }
    public bool IsEnabled { get; set; }
    public string ApiKey { get; set; }
}
</file>
```

## Output Formats

### Text Format (Recommended for AI)

The text format preserves idiomatic C# syntax with full visibility keywords. In the first example the import filter drops `using System;` and `using Microsoft.Extensions.Logging;` (see Known Issues):

<details open>
<summary>Example: Repository Pattern with Dependency Injection</summary>
<blockquote>

<details>
<summary>Input: `UserService.cs`</summary>
<blockquote>

```csharp
using System;
using System.Threading.Tasks;
using Microsoft.Extensions.Logging;

namespace MyApp.Services;

public interface IUserService
{
    Task<User?> GetUserAsync(int id);
    Task<bool> UpdateUserAsync(User user);
}

public sealed class UserService : IUserService
{
    private readonly IUserRepository _repository;
    private readonly ILogger<UserService> _logger;
    private readonly ICacheService _cache;

    public UserService(
        IUserRepository repository,
        ILogger<UserService> logger,
        ICacheService cache)
    {
        _repository = repository ?? throw new ArgumentNullException(nameof(repository));
        _logger = logger ?? throw new ArgumentNullException(nameof(logger));
        _cache = cache ?? throw new ArgumentNullException(nameof(cache));
    }

    public async Task<User?> GetUserAsync(int id)
    {
        var cacheKey = $"user:{id}";
        
        if (_cache.TryGet<User>(cacheKey, out var cached))
        {
            _logger.LogDebug("User {UserId} found in cache", id);
            return cached;
        }

        var user = await _repository.GetByIdAsync(id);
        if (user != null)
        {
            await _cache.SetAsync(cacheKey, user, TimeSpan.FromMinutes(5));
        }

        return user;
    }

    public async Task<bool> UpdateUserAsync(User user)
    {
        ArgumentNullException.ThrowIfNull(user);
        
        try
        {
            await _repository.UpdateAsync(user);
            await _cache.RemoveAsync($"user:{user.Id}");
            _logger.LogInformation("User {UserId} updated successfully", user.Id);
            return true;
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Failed to update user {UserId}", user.Id);
            return false;
        }
    }

    private async Task<bool> ValidateUserAsync(User user)
    {
        // Validation logic
        return await Task.FromResult(true);
    }
}
```

</blockquote>
</details>

<details open>
<summary>Default output (public only, no implementation)</summary>
<blockquote>

```
<file path="UserService.cs">
using System.Threading.Tasks;
namespace MyApp.Services;
public interface IUserService {
    public Task<User?> GetUserAsync(int id);
    public Task<bool> UpdateUserAsync(User user);
}
public sealed class UserService : IUserService {
    public UserService(IUserRepository repository, ILogger<UserService> logger, ICacheService cache);
    public async Task<User?> GetUserAsync(int id);
    public async Task<bool> UpdateUserAsync(User user);
}
</file>
```

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Generic Constraints</summary>
<blockquote>

<details>
<summary>Input: `MathProcessor.cs`</summary>
<blockquote>

```csharp
#nullable enable
using System;
using System.Numerics;

namespace AdvancedMath;

public interface IVectorOperations<T> where T : INumber<T>
{
    T DotProduct<TVector>(TVector a, TVector b) 
        where TVector : IVector<T>;
    
    TVector Add<TVector>(TVector a, TVector b) 
        where TVector : IVector<T>, new();
}

public class VectorProcessor<T> : IVectorOperations<T> 
    where T : INumber<T>, IMinMaxValue<T>
{
    private readonly ILogger? _logger;

    public VectorProcessor(ILogger? logger = null)
    {
        _logger = logger;
    }

    public T DotProduct<TVector>(TVector a, TVector b) 
        where TVector : IVector<T>
    {
        if (a.Dimension != b.Dimension)
            throw new ArgumentException("Vectors must have same dimension");

        var result = T.Zero;
        for (int i = 0; i < a.Dimension; i++)
        {
            result += a[i] * b[i];
        }
        
        _logger?.Log($"Dot product calculated: {result}");
        return result;
    }

    public TVector Add<TVector>(TVector a, TVector b) 
        where TVector : IVector<T>, new()
    {
        var result = new TVector { Dimension = a.Dimension };
        for (int i = 0; i < a.Dimension; i++)
        {
            result[i] = a[i] + b[i];
        }
        return result;
    }

    protected virtual T ClampValue(T value)
    {
        return value switch
        {
            _ when value > T.MaxValue => T.MaxValue,
            _ when value < T.MinValue => T.MinValue,
            _ => value
        };
    }

    private static bool IsValidDimension(int dimension) => dimension > 0;
}
```

</blockquote>
</details>

<details open>
<summary>Default Output</summary>
<blockquote>

```
<file path="MathProcessor.cs">
#nullable enable

namespace AdvancedMath;
public interface IVectorOperations<T> where T : INumber<T> {
    public T DotProduct<TVector>(TVector a, TVector b) where TVector : IVector<T>;
    public TVector Add<TVector>(TVector a, TVector b) where TVector : IVector<T>, new();
}
public class VectorProcessor<T> : IVectorOperations where T : INumber<T>, IMinMaxValue<T> {
    public VectorProcessor(ILogger? logger);
    public T DotProduct<TVector>(TVector a, TVector b) where TVector : IVector<T>;
    public TVector Add<TVector>(TVector a, TVector b) where TVector : IVector<T>, new();
}
</file>
```

The constraints keep their type arguments; the default value `= null` and the generic argument of `IVectorOperations<T>` in the base list are dropped.

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Records (shows the record parameter bugs)</summary>
<blockquote>

<details>
<summary>Input: `Commands.cs`</summary>
<blockquote>

```csharp
#nullable enable
using System;
using System.ComponentModel.DataAnnotations;

namespace MyApp.Commands;

public abstract record Command
{
    public Guid Id { get; init; } = Guid.NewGuid();
    public DateTime Timestamp { get; init; } = DateTime.UtcNow;
}

public sealed record CreateUserCommand(
    [property: Required] string Username,
    [property: EmailAddress] string Email,
    [property: StringLength(100)] string? FullName = null) : Command
{
    public bool IsValid => !string.IsNullOrWhiteSpace(Username) && 
                          !string.IsNullOrWhiteSpace(Email);
}

public sealed record UpdateUserCommand(
    [property: Required] Guid UserId,
    [property: StringLength(100)] string? FullName,
    [property: Phone] string? PhoneNumber) : Command;

public sealed record DeleteUserCommand(
    [property: Required] Guid UserId,
    [property: Required] string Reason) : Command;

public static class CommandHandler
{
    public static async Task<CommandResult> HandleAsync(Command command) => command switch
    {
        CreateUserCommand { IsValid: true } cmd => await CreateUserAsync(cmd),
        CreateUserCommand { IsValid: false } => CommandResult.ValidationFailed("Invalid user data"),
        UpdateUserCommand cmd => await UpdateUserAsync(cmd),
        DeleteUserCommand { Reason.Length: > 10 } cmd => await DeleteUserAsync(cmd),
        DeleteUserCommand => CommandResult.ValidationFailed("Deletion reason too short"),
        null => throw new ArgumentNullException(nameof(command)),
        _ => CommandResult.NotSupported($"Command type {command.GetType().Name} not supported")
    };

    private static async Task<CommandResult> CreateUserAsync(CreateUserCommand cmd)
    {
        // Implementation
        return await Task.FromResult(CommandResult.Success(cmd.Id));
    }

    private static async Task<CommandResult> UpdateUserAsync(UpdateUserCommand cmd)
    {
        // Implementation
        return await Task.FromResult(CommandResult.Success(cmd.Id));
    }

    private static async Task<CommandResult> DeleteUserAsync(DeleteUserCommand cmd)
    {
        // Implementation  
        return await Task.FromResult(CommandResult.Success(cmd.Id));
    }
}

public record CommandResult(bool IsSuccess, Guid? CommandId, string? Error)
{
    public static CommandResult Success(Guid commandId) => new(true, commandId, null);
    public static CommandResult ValidationFailed(string error) => new(false, null, error);
    public static CommandResult NotSupported(string error) => new(false, null, error);
}
```

</blockquote>
</details>

<details open>
<summary>Default Output</summary>
<blockquote>

```
<file path="Commands.cs">
#nullable enable

namespace MyApp.Commands;
public abstract record Command {
    public Guid Id { get; set; }
    public DateTime Timestamp { get; set; }
}
public sealed record CreateUserCommand(
    [property: Required] string Username,
    [property: EmailAddress] string Email,
    [property: StringLength(100)] FullName,
    bool IsValid) : Command;
public sealed record UpdateUserCommand(
    [property: Required] UserId Guid,
    [property: StringLength(100)] FullName,
    [property: Phone] PhoneNumber) : Command;
public sealed record DeleteUserCommand(
    [property: Required] UserId Guid,
    [property: Required] string Reason) : Command;
public static class CommandHandler {
    public static async Task<CommandResult> HandleAsync(Command command);
}
public record CommandResult(
    bool IsSuccess,
    CommandId,
    Error) {
    public static CommandResult Success(Guid commandId);
    public static CommandResult ValidationFailed(string error);
    public static CommandResult NotSupported(string error);
}
</file>
```

The initializers and `init` accessors of `Command` are lost; `Guid UserId` prints as `UserId Guid`, the nullable types (`string?`, `Guid?`) and the default `= null` are dropped, and the expression-bodied `IsValid` property of `CreateUserCommand` appears as a parameter (see Known Issues).

</blockquote>
</details>

</blockquote>
</details>

## Best Practices

### 1. **Use Full Type Annotations**

AI Distiller keeps the declared types of methods and parameters (default values are dropped):

```csharp
// Good - Complete type information
public async Task<Result<User>> GetUserAsync(
    int id, 
    CancellationToken cancellationToken = default)
{
    // ...
}

// Less optimal - Generic type info may be incomplete  
public async Task GetUserAsync(int id)
{
    // ...
}
```

### 2. **Leverage Modern C# Features**

Modern syntax parses; method bodies (pattern matching included) print with `--implementation=1`:

```csharp
// File-scoped namespaces (C# 10+)
namespace MyApp.Services;

// Global usings (C# 10+), printed as a plain using
global using System.Text.Json;

// Records with primary constructors (C# 9+)
public record UserDto(string Name, string Email);

// Pattern matching (C# 8+)
var message = user switch
{
    { IsActive: true } => "Active user",
    { IsActive: false } => "Inactive user",
    null => "User not found"
};
```

### 3. **Structure for AI Understanding**

- Use clear, descriptive names
- Add XML documentation comments
- Group related functionality
- Use interfaces for contracts
- Apply consistent visibility modifiers

### 4. **Optimize for Distillation**

```csharp
// Good - Clear API surface
public interface IUserService
{
    Task<User?> GetUserAsync(int id);
    Task<IEnumerable<User>> GetActiveUsersAsync();
    Task<bool> DeactivateUserAsync(int id);
}

// Implementation details are stripped by default
public class UserService : IUserService
{
    private readonly IDbContext _context;
    private readonly ICache _cache;
    
    // Public methods define the API
    public async Task<User?> GetUserAsync(int id) { }
    
    // Private methods are stripped by default
    private async Task<bool> ValidateUserAsync(User user) { }
}
```

## Integration Examples

### Direct CLI Usage

```bash
# Generate context for AI assistants (public API, no implementation: the defaults)
aid ./src --output context.txt

# Include in your prompt
cat <<EOF > prompt.txt
Here's my C# codebase structure:

$(cat context.txt)

Please help me implement a new feature that...
EOF
```

### MCP Integration

When using AI Distiller through MCP, Claude can directly access your C# codebase. The MCP server `@janreges/ai-distiller-mcp` provides tools such as `distill_directory`, `distill_file` and `list_files`; setup for Claude Code, Claude Desktop and other clients, and the parameters of every tool, are in [`mcp-npm/README.md`](../../mcp-npm/README.md):

```bash
claude mcp add aid -- npx -y @janreges/ai-distiller-mcp
```

**Example Conversation:**

> **You**: Show me the structure of our payment processing system and identify potential improvements.

> **Claude**: I'll analyze your payment processing system using AI Distiller.

*Claude calls:*
```
list_files({ path: "Services/Payment/", pattern: "*.cs" })
distill_directory({ directory_path: "Services/Payment/", include_implementation: false })
```

*Claude receives the distilled structure and responds:*

> Based on the analysis, your payment processing system has:
> 
> **Core Components:**
> - `IPaymentProcessor` interface with methods: `ProcessPaymentAsync()`, `RefundAsync()`, `GetTransactionStatusAsync()`
> - `StripePaymentProcessor : IPaymentProcessor` - Stripe implementation
> - `PayPalPaymentProcessor : IPaymentProcessor` - PayPal implementation
> - `PaymentService` - Orchestrator using strategy pattern
> 
> **Identified Improvements:**
> 1. **Missing Circuit Breaker**: Add resilience for external API calls
> 2. **No Idempotency**: Payment operations should be idempotent
> 3. **Logging Enhancement**: Current logging lacks structured data
> 
> Here's how to implement these improvements...

### With jq

```bash
# List the public class names of a directory (attributes are not part of the json-structured output)
aid ./Models --format json-structured --stdout | jq -r '.files[].structure.classes[]?.name'
```

### CI/CD Integration

```yaml
# .github/workflows/api-surface-check.yml
- name: Check Public API Surface
  run: |
    aid ./src --stdout > api-current.txt
    
    # Compare with baseline
    if ! diff api-baseline.txt api-current.txt > api-changes.diff; then
      echo "::warning::Public API surface has changed"
      cat api-changes.diff
    fi
```

## Language-Specific Tips

1. **Nullable Reference Types**: `#nullable enable` directives are printed as written

2. **Generic Constraints**: Use meaningful constraints for better AI understanding:
   ```csharp
   // Good - Clear intent
   where T : class, IEntity, new()
   
   // Better - Even more specific
   where T : BaseEntity, IValidatable, new()
   ```

3. **Records vs Classes**: Use records for DTOs and immutable data:
   ```csharp
   // DTO - use record
   public record UserDto(string Name, string Email);
   
   // Service - use class
   public class UserService : IUserService { }
   ```

4. **Extension Methods**: Mark extension classes clearly:
   ```csharp
   public static class StringExtensions
   {
       public static bool IsNullOrEmpty(this string? value) => 
           string.IsNullOrEmpty(value);
   }
   ```

## Comparison with Other Tools

| Tool | Purpose | C# Support | AI-Optimized |
|------|---------|-----------|--------------|
| **AI Distiller** | Code structure extraction | Modern C# (gaps above) | ✅ Yes |
| Roslyn | Compiler/Analysis | Native | ❌ No |
| dotnet-format | Code formatting | Full | ❌ No |
| StyleCop | Style analysis | Full | ❌ No |
| ILSpy | Decompilation | Full | ❌ No |

## Troubleshooting

### Output is empty or incomplete

The parser is error-tolerant, but a file with syntax errors can lose declarations. Ensure your C# code compiles:
```bash
dotnet build
```

### "Generic constraints not showing"

Check that constraints are on the same line (for single constraints) or properly formatted:
```csharp
// Good
public class Service<T> where T : class, new()

// Also good  
public class ComplexService<T, U> 
    where T : class, IEntity
    where U : struct, IComparable<U>
```

### "A using statement is missing"

The text output's import filter drops a `using` it considers unused. Add `--filter-imports=false` to keep all of them.

## Future Enhancements

- [ ] Source generator support
- [ ] Partial class merging
- [ ] Delegates, indexers, conversion operators, and interface properties and events as public members
- [ ] Correct record primary constructor parameters
- [ ] Analyzer/diagnostic integration
- [ ] .NET Aspire integration
- [ ] Primary constructor body support

## Contributing

Help improve C# support! See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

Key areas needing help:
- Complex generic scenarios
- Source generator examples
- Blazor component patterns
- MAUI/WPF specific constructs
