# TypeScript Language Support

AI Distiller provides comprehensive support for TypeScript codebases using the [tree-sitter-typescript](https://github.com/tree-sitter/tree-sitter-typescript) parser, with full support for modern TypeScript features including generics, decorators, and advanced type system constructs.

## Overview

TypeScript support in AI Distiller is designed to capture the rich type information and structural patterns that make TypeScript powerful. The distilled output preserves type safety contracts, generic constraints, and architectural relationships while minimizing token count for efficient AI processing.

## Supported TypeScript Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ✅ Full | Including abstract classes, inheritance, generics; every `extends` and `implements` base as written (`React.Component<Props>`, `ns.Base<string>`) |
| **Interfaces** | ⚠️ Partial | With extends (as written, `extends Base<string>, ns.Other`) and generics; index signatures (`[key: string]: any`) are not printed |
| **Functions** | ✅ Full | Regular, async, arrow functions, overloads (every signature is printed) |
| **Methods** | ✅ Full | Public, private, protected, abstract, static; accessors (`get size(): number`, `set size(v: number)`), generators (`async *iter()`, `function* gen()`) |
| **Type System** | ✅ Full | Union, intersection, conditional, mapped types |
| **Generics** | ✅ Full | With constraints, defaults, inference |
| **Type Aliases** | ✅ Full | Including complex utility types |
| **Imports/Exports** | ✅ Full | Imports as written (`import D, { A, B as C } from './m'`, `import type`, `import * as ns`); re-exports and export lists (`export * from './a'`, `export { x as y } from './c'`, `export { q }`, `export default Inner`); `export` and `export default` on declarations |
| **Enums** | ✅ Full | Members with their initialisers; `const enum` prints as `enum` |
| **Decorators** | ⚠️ Partial | Method decorators are printed with their arguments; class and property decorators are dropped |
| **Namespaces** | ✅ Full | `namespace X {}` and `declare module "x" {}` with their members |
| **Parameters** | ✅ Full | As written: destructuring patterns (`{ a, b = 1 }: P`), rest (`...rest: U[]`), optional (`c?: string`), defaults (`d: number = 0`), `this: K` |
| **Parameter Properties** | ✅ Full | Constructor shorthand syntax becomes fields with their visibility and `readonly`; the field does not repeat the parameter's `?` or default |

### Visibility Rules

TypeScript visibility in AI Distiller follows the language's access modifiers:
- **Public**: Default visibility or explicit `public` keyword, exported declarations
- **Private**: `private` keyword, `#` private fields and methods (the `#` stays in the name), or underscore-prefixed non-exported functions and variables
- **Protected**: `protected` keyword
- **Internal**: Non-exported top-level declarations (module-private), shown with `--internal=1`

The default output therefore shows exported declarations only. A file whose classes and functions are not exported prints nothing without `--internal=1`; the examples below that have no `export` use that flag. In an ambient declaration (`declare namespace`, `declare module "x"`) every member is public.

## Key Features

### 1. **Advanced Generic Type Preservation**

AI Distiller captures complex generic constraints and relationships:

```typescript
// Input
class EventEmitter<TEventMap extends object> {
  emit<K extends keyof TEventMap>(
    event: K, 
    payload: TEventMap[K]
  ): void { /* ... */ }
}

type Predicate<T> = T extends (...args: any[]) => infer R ? R : never;
```

```
# Output (--internal=1)
class EventEmitter<TEventMap extends object> {
    public emit<K extends keyof TEventMap>(event: K, payload: TEventMap[K]): void
}
type Predicate<T> = T extends (...args: any[]) => infer R ? R : never
```

### 2. **Interface and Type Relationships**

Inheritance and implementation contracts are preserved:

```typescript
// Input
interface INotifiable<P> {
  handleNotification(payload: P): void;
}

abstract class BaseService {
  abstract process<T extends {id: string}>(item: T): Promise<boolean>;
}

class EmailService extends BaseService implements INotifiable<EmailPayload> {
  // Implementation
}
```

```
# Output (--internal=1)
interface INotifiable<P> {
    method handleNotification(payload: P): void
}

abstract class BaseService {
    public abstract process<T extends {id: string}>(item: T): Promise<boolean>
}

class EmailService extends BaseService implements INotifiable<EmailPayload> {
}
```

### 3. **Parameter Properties**

TypeScript's constructor shorthand becomes fields with the parameter's visibility:

```typescript
// Input
class UserModel {
  constructor(
    public readonly id: number,
    private email: string,
    protected lastLogin?: Date
  ) {}
}
```

```
# Output (--internal=1 --private=1 --protected=1)
class UserModel {
    public readonly id: number
    private email: string
    protected lastLogin: Date
    public constructor(id: number, email: string, lastLogin?: Date)
}
```

## Output Formats

### Text Format (Recommended for AI)

AI Distiller uses a TypeScript-specific formatter that keeps TypeScript syntax: `export` on top-level declarations, the access keyword on class members, `property` and `method` on interface members, and `: Type` return types. With `--implementation=1`, bodies are printed in braces:

<details open>
<summary>Example: Complex TypeScript Class</summary>
<blockquote>

<details>
<summary>Input: `plugin-manager.ts`</summary>
<blockquote>

```typescript
export interface IPlugin {
  name: string;
  execute(data: any): void;
}

export class PluginManager {
  private plugins: Map<string, IPlugin> = new Map();

  constructor(private settings: PluginSettings = {}) {}

  @LogExecution
  public registerPlugin(plugin: IPlugin): void {
    if (this.plugins.has(plugin.name)) {
      throw new Error(`Plugin "${plugin.name}" already registered`);
    }
    this.plugins.set(plugin.name, plugin);
  }

  public async runAll(data: any): Promise<void> {
    for (const plugin of this.plugins.values()) {
      await plugin.execute(data);
    }
  }
}
```

</blockquote>
</details>

<details>
<summary>Output: Default (`aid plugin-manager.ts`)</summary>
<blockquote>

```
<file path="plugin-manager.ts">

export interface IPlugin {
    property name: string
    method execute(data: any): void
}

export class PluginManager {
    public constructor(settings: PluginSettings)
    @LogExecution
    public registerPlugin(plugin: IPlugin): void
    public async runAll(data: any): Promise<void>
}
</file>
```

</blockquote>
</details>

<details>
<summary>Output: `--private=1 --protected=1 --internal=1`</summary>
<blockquote>

```
<file path="plugin-manager.ts">

export interface IPlugin {
    property name: string
    method execute(data: any): void
}

export class PluginManager {
    private plugins: Map<string, IPlugin> = new Map()
    private settings: PluginSettings
    public constructor(settings: PluginSettings)
    @LogExecution
    public registerPlugin(plugin: IPlugin): void
    public async runAll(data: any): Promise<void>
}
</file>
```

</blockquote>
</details>

<details>
<summary>Output: `--implementation=1`</summary>
<blockquote>

```
<file path="plugin-manager.ts">

export interface IPlugin {
    property name: string
    method execute(data: any): void
}

export class PluginManager {
    public constructor(settings: PluginSettings) {}
    @LogExecution
    public registerPlugin(plugin: IPlugin): void {
        if (this.plugins.has(plugin.name)) {
          throw new Error(`Plugin "${plugin.name}" already registered`);
        }
        this.plugins.set(plugin.name, plugin);
    }
    public async runAll(data: any): Promise<void> {
        for (const plugin of this.plugins.values()) {
          await plugin.execute(data);
        }
    }
}
</file>
```

</blockquote>
</details>

</blockquote>
</details>

### Markdown Format

The markdown format (`--format md`) wraps the text output of each file in a fenced code block under a heading with the file path:

````markdown
### plugin-manager.ts

```typescript

export interface IPlugin {
    property name: string
    method execute(data: any): void
}

export class PluginManager {
    public constructor(settings: PluginSettings)
    @LogExecution
    public registerPlugin(plugin: IPlugin): void
    public async runAll(data: any): Promise<void>
}
```
````

## Advanced TypeScript Features

### Conditional Types and Type Inference

```typescript
type ChangeEvent<T extends string> = `${T}Changed`;
type Payload<T> = T extends (payload: infer P) => void ? P : never;

type ListenerMap<TEventMap extends object> = {
  [K in keyof TEventMap as ChangeEvent<K & string>]: (payload: TEventMap[K]) => void;
};
```

AI Distiller preserves these complex type manipulations verbatim, maintaining the full expressiveness of TypeScript's type system.

### Mapped Types and Index Signatures

```typescript
type PluginSettings = {
  [K in 'timeout' | 'retries']?: K extends 'timeout' ? number : number;
};

interface StringIndex {
  [key: string]: any;
  length: number;  // Named property
}
```

## Integration with AI Workflows

### Optimizing for Context Windows

TypeScript code often includes extensive type definitions. Use the filtering flags strategically:

```bash
# For implementation analysis - keep bodies, remove docs
aid src/ --implementation=1 --docstrings=0

# For API understanding - exported signatures only (the defaults)
aid src/

# For architecture overview - include module-internal declarations too
aid src/ --internal=1
```

### Framework-Specific Patterns

AI Distiller recognizes common TypeScript framework patterns:

- **Angular**: Components, services, method decorators (class and property decorators are dropped)
- **React**: Function components, hooks, props interfaces
- **Node.js**: Express middleware, async handlers
- **NestJS**: Controllers, providers, modules

## Known Limitations

1. **Decorators**: Class decorators (`@Component({...})`) and property decorators (`@Input()`) are dropped; method decorators are printed with their arguments
2. **Index Signatures**: `[key: string]: T` members of interfaces are not printed
3. **Triple-slash Directives**: Reference comments are ignored
4. **Generic arrow functions**: The trailing comma of `<T,>` is not kept (`export const fn = <T>(x: T): T`)
5. **`const enum`**: Printed as a plain `enum`
6. **Optional and readonly markers on members**: The `?` of optional properties and methods and the `readonly` of interface members are dropped (`email?: string` prints as `property email: string`); class members keep `readonly` (`public readonly limit: number = 3`) and parameters keep their `?`
7. **`const` type parameters**: The `const` of `<const T>` is dropped (`<T>`)

## Best Practices

### 1. Type-First Development

AI Distiller works best with well-typed TypeScript:

```typescript
// ✅ Good - Explicit types preserved
function process(data: UserData[]): ProcessedResult {
  return data.map(transform);
}

// ❌ Avoid - Type information lost
function process(data) {
  return data.map(transform);
}
```

### 2. Interface Segregation

Smaller, focused interfaces distill better:

```typescript
// ✅ Good - Clear, focused interfaces
interface Readable {
  read(): Buffer;
}

interface Writable {
  write(data: Buffer): void;
}

// ❌ Avoid - Large, monolithic interfaces
interface Stream {
  read(): Buffer;
  write(data: Buffer): void;
  // ... 20 more methods
}
```

### 3. Consistent Visibility

Use TypeScript's visibility modifiers consistently for better distillation:

```typescript
class Service {
  public api(): void {}      // Explicitly public
  protected hook(): void {}  // For extension points
  private helper(): void {}  // Internal only
}
```

## Known Issues & Recent Fixes

### Recently Fixed (2025-06-15)

1. **Export modifiers missing** (✅ Fixed)
   - **Issue**: Export keywords were not captured for classes, interfaces, and functions
   - **Fix**: Added ModifierExport throughout the IR pipeline
   - **Impact**: Export information now properly preserved in output

2. **Class syntax error** (✅ Fixed)
   - **Issue**: Classes were formatted with `:` instead of `{` in text output
   - **Fix**: Updated TypeScript formatter to use proper brace syntax
   - **Impact**: Valid TypeScript-like syntax in output

3. **Return type arrow syntax** (✅ Fixed)
   - **Issue**: Return types used `->` instead of TypeScript's `:`
   - **Fix**: Updated formatter to use TypeScript-style colon syntax
   - **Impact**: More familiar syntax for TypeScript developers

4. **Underscore-prefixed members** (✅ Fixed)
   - **Issue**: Functions like `_normalizeString` appeared in default output
   - **Fix**: Added convention-based private detection for underscore prefix
   - **Impact**: Better alignment with JavaScript/TypeScript conventions

5. **Top-level function visibility** (✅ Fixed)
   - **Issue**: Top-level functions showed "public" prefix incorrectly
   - **Fix**: Updated formatter to omit visibility for top-level declarations
   - **Impact**: Cleaner, more idiomatic output

### Recently Fixed (hardening)

1. **Enums** (✅ Fixed): enums were missing from the text output; they print with one member per line and their initialisers (`A = -1`, `B = 1 << 2`)
2. **Namespaces and ambient modules** (✅ Fixed): `namespace X {}` and `declare module "x" {}` print with their members
3. **Return and constant types** (✅ Fixed): top-level functions keep their return type, type guards (`x is number`, `asserts x is string`) are kept, constants keep their type and a short initialiser
4. **`#private` members** (✅ Fixed): keep their `#name` and are private
5. **Function bodies** (✅ Fixed): `--implementation=1` prints the real body instead of a placeholder
6. **Comments and JSDoc** (✅ Fixed): JSDoc blocks print by default (`--docstrings=0` removes them), `//` and `/* */` comments with `--comments=1`
7. **Exported type aliases** (✅ Fixed): keep their `export` keyword

### Recently Fixed (after v1.5.0)

1. **Imports** (✅ Fixed): printed as written (`import D, { A, B as C } from './m'`, `import * as ns`, `import type`) instead of a bare `import './m'`
2. **Re-exports and default exports** (✅ Fixed): `export * from`, `export { x as y } from`, `export { q }`, `export default Inner` and `export = X` print, so a barrel file is no longer empty; `export default function main()` keeps its `default`
3. **Generic and qualified bases** (✅ Fixed): `interface A extends B<string>`, `class X extends React.Component<P>` and `extends ns.Base<T>` lost their base
4. **Parameters** (✅ Fixed): destructured parameters (`{ a }: P`, `[x, y]: T`) and rest parameters were dropped, defaults and the `?` of optional parameters were not shown, and parameter properties lost `readonly`
5. **Accessors, generators, generic defaults** (✅ Fixed): `get size()` and `set size(v)` printed as two plain `size` methods, the `*` of generators was lost and `function* gen()` was missing, and a type parameter's default (`<T extends object = {}>`) was dropped
6. **Arrow-function and wrapped constants** (✅ Fixed): `export const handler = async <T>(req: Request): Response => ...` printed as `export const handler(req: Request): Response`, without `=`, `async` and the type parameters; it prints `export const handler = async <T>(req: Request): Response`. A constant initialised with a call that takes a function printed bare (`export const Button`) or with its body; it prints `export const Button = forwardRef<HTMLButtonElement, ButtonProps>(({ variant = "a", ...props }, ref))` and the whole initializer with `--implementation=1`. A typed arrow constant keeps its type: `const Inner: React.FC<P> = ({ a })`
7. **`let` and `var`** (✅ Fixed): a `let` or `var` initialised with an arrow function printed as `function f(a: number)` and a `var` as `let`; every variable prints with its keyword as written (`let f = (a: number)`, `var v = 3`)

### Current Limitations

See Known Limitations above.

## Performance Characteristics

TypeScript parsing performance with tree-sitter:

| Codebase Size | Parse Time | Memory Usage |
|--------------|------------|--------------|
| Small (< 1MB) | < 100ms | ~10MB |
| Medium (1-10MB) | < 500ms | ~50MB |
| Large (10-50MB) | < 2s | ~200MB |
| Huge (> 50MB) | < 5s | ~500MB |

## Contributing

TypeScript support is actively maintained. Key areas for contribution:

1. **Decorator Support**: Class and property decorators in `internal/language/typescript/ast_parser.go`
2. **Index Signatures**: Print them in interfaces
3. **Performance**: Optimize tree-sitter grammar loading

See [CONTRIBUTING.md](../../CONTRIBUTING.md) for development setup.