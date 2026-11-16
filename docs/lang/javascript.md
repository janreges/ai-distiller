# JavaScript Language Support

AI Distiller provides comprehensive support for JavaScript (ES2015+) and JSX, extracting the essential structure of your code while maintaining type information from JSDoc comments.

## Main Constructs

JavaScript is a dynamically-typed language with several key constructs:

- **Functions** - Regular functions, arrow functions, async functions, generators
- **Classes** - ES6 classes with constructors, methods, and static members
- **Objects** - Object literals with properties and methods
- **Modules** - ES6 imports/exports and CommonJS module.exports
- **Variables** - const, let, var declarations

## Primary Goal of Text Format

The text format aims to provide AI systems with a complete understanding of available functions, classes, and their signatures. With the default flags (public members only, no comments, no implementation) the output focuses on:

- Public API surface (exported functions, classes, methods)
- Function and method signatures with parameter names
- Object structure showing available properties and methods
- Module imports and exports

This gives AI systems enough context to understand what functionality is available and how to use it, without overwhelming them with implementation details.

## Important Features

### Type Information from JSDoc

While JavaScript is dynamically typed, AI Distiller extracts type information from JSDoc comments:

```javascript
/**
 * @param {string} name - User name
 * @param {number} age - User age
 * @returns {User} The created user
 */
function createUser(name, age) { ... }

/**
 * @param {Object} options - Configuration options
 * @param {boolean} [options.cache=true] - Enable caching
 * @returns {Promise<Data>} The fetched data
 */
async function fetchData(options) { ... }
```

This appears in the distilled output with type annotations (the JSDoc block itself is printed above the function, `--docstrings=0` removes it):
- `function createUser(name: string, age: number) -> User`
- `async function fetchData(options: Object) -> Promise<Data>`

A type is read whole, nested braces and spaces included, also when it goes on over lines: `@type {WeakMap<object, Set<Function>>}` gives `const listeners: WeakMap<object, Set<Function>>`, and `@param {number} [timeout=5000]` types the optional parameter `timeout`. A `{` that never closes leaves only its own tag untyped: the next line that starts with `@` is the next tag. A `@typedef` declares a type name and does not type the declaration after it.

### Function Parameter Extraction

The parser extracts the parameters of functions, arrow functions and class methods, including:
- Parameter names and types from JSDoc
- Default parameter values (`a = 1`)
- Rest parameters (`...args`)
- Destructured parameters (shown as patterns); one with a default value is dropped (`function d({p} = {}, z = 1)` prints as `function d(z = 1)`)

Object-literal methods print their parameter names only: defaults and rest parameters are dropped (`call(x = 4)` prints as `call(x)`, `m(...xs)` as `m()`).

### Modern JavaScript Support

The parser supports modern JavaScript features including:
- ES6+ syntax (arrow functions, destructuring, template literals)
- Async/await and Promises (async methods of object literals get a `-> Promise` return type)
- Generators and iterators (marked with `*` prefix)
- ES6 modules and dynamic imports
- JSX syntax for React components
- Getters and setters in objects and classes
- Static class members and private fields (`#private`)
- Spread syntax in object literals (`...object`)

### Object Literal Analysis

Object literals are parsed to show their structure with full method signatures including parameters, async/generator modifiers, and spread syntax:
```javascript
const api = {
  name: 'MyAPI',
  getData(id) { ... },
  async processData(input, options = {}) { ... },
  *generateItems(count) { ... },
  get status() { ... },
  set status(value) { ... }
}

const extended = {
  ...api,
  cache: new Map(),
  async getCached(key) { ... }
}
```

Distills to:
- `const api = { name, getData(id), async processData(input, options) -> Promise, *generateItems(count), get status(), set status(value) }`
- `const extended = { ...api, cache, async getCached(key) -> Promise }`

## What Works Well

### Fully Supported Features
- ✅ **Function signatures** with all parameters and default values (object-literal methods: names only, see above)
- ✅ **Return types** from JSDoc `@returns` annotations
- ✅ **Async/generator detection** with proper prefixes for functions, class methods and object-literal methods
- ✅ **Object literal analysis** including methods and spread syntax
- ✅ **ES6 classes** with constructors, methods, getters/setters
- ✅ **Exported declarations** are printed with their `export` or `export default` keyword, as in TypeScript (`export const createClient = (apiKey)`, `export default class:` for an anonymous class); `module.exports = ...` is shown only with `--comments=1`
- ✅ **React import kept for JSX** - the unused-import filter keeps the import that binds `React` (`import React from 'react'`, `import * as React from 'react'`) in a file with JSX and in any `.jsx` file: the classic JSX transform compiles `<div />` to `React.createElement('div')`, although the distilled text never names `React`
- ✅ **Unused-import filter** keeps an import whose name the printed text uses anywhere in the file, also below a later `require(...)` in a printed body; names with `$` (`import $ from 'jquery'`, `obs$`) match as whole names
- ✅ **Re-exports and export lists** print as written: `export * from './a'`, `export { x, y as z } from './c'`, `export { q }`, `export default Inner`
- ✅ **Generator functions** print `function* name()` (`async function* name()`), generator methods `*name()` (`async *name()`, `static *name()`)
- ✅ **JSDoc type extraction** for parameters, return values and variables (`@type`)
- ✅ **Private members** detection: `#private` names, and class members whose name starts with `_` unless their JSDoc says `@public`
- ✅ **JSDoc blocks** printed by default above the declaration they document; `//` and `/* */` comments with `--comments=1`

### Partially Supported Features
- ⚠️ **Nested objects** - Only first level is analyzed
- ⚠️ **Complex destructuring** - Shown as patterns, not fully expanded
- ✅ **Arrow functions assigned to a variable** - printed with their keyword and parameters (`const createClient = (apiKey)`, `const quickGet = async (endpoint)`, `let f = (a)`); `=> body` only with `--implementation=1`
- ✅ **Variables** print with the keyword they were declared with: `const`, `let` or `var`
- ✅ **Function expressions assigned to a variable** - printed as function expressions (`const fe = function (a)`, `const ge = function* ()`, `const af = async function (x)`); the function's own name (`function named(x)`) is not printed
- ✅ **Wrapped components and callbacks** - a constant initialised with a call that takes a function prints the call with each function's parameters and no body: `export const Button = forwardRef(({ variant = "a", ...props }, ref))`, `export const List = memo(function List({ items }))`; an object literal argument longer than 80 characters prints as `{...}` (`core.$constructor("ZodType", (inst, def), {...})`) and a template literal that spans lines as `` `...` ``; the whole initializer with `--implementation=1`
- ⚠️ **Top-level visibility** - JavaScript has no visibility keyword for top-level declarations, so non-exported functions and constants are printed too

### Known Limitations

#### Limited Recursive Parsing
The parser uses a shallow parsing approach for performance:
- Nested objects show only the first level of properties
- IIFE (Immediately Invoked Function Expressions) internals are not analyzed
- Function expressions assigned to properties show signatures but not deep analysis

#### Dynamic Patterns
JavaScript's dynamic nature means some patterns cannot be statically analyzed:
- Dynamic property access: `obj[variable]`
- Runtime module loading: `require(moduleName)`
- eval() and Function constructor usage
- Computed property names (except in method definitions)

## Real Examples

<details open><summary>Basic Function and Class</summary><blockquote>
  <details><summary>basic.js - source code</summary><blockquote>
    
```javascript
/**
 * @param {string} message - The greeting message
 * @returns {string} Formatted greeting
 */
function greet(message) {
    return `Hello, ${message}!`;
}

class User {
    constructor(name, email) {
        this.name = name;
        this.email = email;
        this._id = Math.random();
    }
    
    getName() {
        return this.name;
    }
    
    async sendEmail(subject, body) {
        console.log(`Sending email to ${this.email}`);
        // Implementation here
    }
}

module.exports = { greet, User };
```
    
  </blockquote></details>
  <details open><summary>Default compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    
```
<file path="basic.js">
/**
 * @param {string} message - The greeting message
 * @returns {string} Formatted greeting
 */
function greet(message: string) -> string

class User:
    constructor(name, email)
    getName()
    async sendEmail(subject, body)
</file>
```
    
  </blockquote></details>
  <details><summary>Full version (`--public=1 --protected=1 --internal=1 --private=1 --implementation=1`)</summary><blockquote>
    
```
<file path="basic.js">
/**
 * @param {string} message - The greeting message
 * @returns {string} Formatted greeting
 */
function greet(message: string) -> string {
    return `Hello, ${message}!`;
}

class User:
    constructor(name, email) {
        this.name = name;
        this.email = email;
        this._id = Math.random();
    }
    getName() {
        return this.name;
    }
    async sendEmail(subject, body) {
        console.log(`Sending email to ${this.email}`);
        // Implementation here
    }
</file>
```
    
  </blockquote></details>
</blockquote></details>

<details><summary>React Component with Hooks</summary><blockquote>
  <details><summary>UserList.jsx - source code</summary><blockquote>
    
```javascript
import React, { useState, useEffect } from 'react';
import PropTypes from 'prop-types';

/**
 * Custom hook for fetching users
 * @returns {Array} List of users
 */
const useUsers = () => {
    const [users, setUsers] = useState([]);
    
    useEffect(() => {
        fetch('/api/users')
            .then(res => res.json())
            .then(data => setUsers(data));
    }, []);
    
    return users;
};

/**
 * User list component
 * @param {Object} props - Component props
 * @param {string} props.title - List title
 * @param {Function} props.onUserClick - Click handler
 */
function UserList({ title, onUserClick }) {
    const users = useUsers();
    
    return (
        <div className="user-list">
            <h2>{title}</h2>
            <ul>
                {users.map(user => (
                    <li key={user.id} onClick={() => onUserClick(user)}>
                        {user.name}
                    </li>
                ))}
            </ul>
        </div>
    );
}

UserList.propTypes = {
    title: PropTypes.string.isRequired,
    onUserClick: PropTypes.func
};

export default UserList;
```
    
  </blockquote></details>
  <details open><summary>Default compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    
```
<file path="UserList.jsx">
import React, { useState, useEffect } from 'react'
/**
 * Custom hook for fetching users
 * @returns {Array} List of users
 */
const useUsers = ()
/**
 * User list component
 * @param {Object} props - Component props
 * @param {string} props.title - List title
 * @param {Function} props.onUserClick - Click handler
 */
function UserList({ title, onUserClick })
export default UserList
</file>
```
    
  </blockquote></details>
</blockquote></details>

<details><summary>Complex Module with Mixed Patterns</summary><blockquote>
  <details><summary>api-client.js - source code</summary><blockquote>
    
```javascript
import axios from 'axios';

const API_BASE = 'https://api.example.com';

// Private helper
function buildUrl(endpoint) {
    return `${API_BASE}${endpoint}`;
}

/**
 * API client with authentication
 */
class ApiClient {
    constructor(apiKey) {
        this.apiKey = apiKey;
        this.axios = axios.create({
            headers: { 'X-API-Key': apiKey }
        });
    }
    
    async get(endpoint) {
        const response = await this.axios.get(buildUrl(endpoint));
        return response.data;
    }
    
    async post(endpoint, data) {
        const response = await this.axios.post(buildUrl(endpoint), data);
        return response.data;
    }
}

// Factory function
export const createClient = (apiKey) => new ApiClient(apiKey);

// Convenience methods
export const quickGet = async (endpoint) => {
    const client = new ApiClient(process.env.API_KEY);
    return client.get(endpoint);
};

export default ApiClient;
```
    
  </blockquote></details>
  <details open><summary>Default compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    
```
<file path="api-client.js">
const API_BASE = 'https://api.example.com'
function buildUrl(endpoint)
/** API client with authentication */
class ApiClient:
    constructor(apiKey)
    async get(endpoint)
    async post(endpoint, data)
export const createClient = (apiKey)
export const quickGet = async (endpoint)
export default ApiClient
</file>
```
    
  </blockquote></details>
</blockquote></details>

<details><summary>Advanced Features Example</summary><blockquote>
  <details><summary>advanced-features.js - source code</summary><blockquote>
    
```javascript
// Object with spread syntax and methods
const baseApi = {
    timeout: 5000,
    retry: 3
};

const dataService = {
    ...baseApi,
    cache: new Map(),
    
    async getData(id, options = {}) {
        // Implementation
        return { id, ...options };
    },
    
    *generateBatch(count = 10) {
        for (let i = 0; i < count; i++) {
            yield { id: i, data: `item-${i}` };
        }
    },
    
    get cacheSize() {
        return this.cache.size;
    },
    
    set maxCacheSize(value) {
        this._maxCache = value;
    }
};

// Class with private fields
class SecureStore {
    #encryptionKey;
    #data = new Map();
    
    constructor(key) {
        this.#encryptionKey = key;
    }
    
    async store(key, value) {
        const encrypted = await this.#encrypt(value);
        this.#data.set(key, encrypted);
    }
    
    async #encrypt(data) {
        // Private method
        return btoa(JSON.stringify(data));
    }
}

/**
 * @param {string[]} items - Array of items
 * @param {Function} callback - Processing callback
 * @returns {Promise<void>}
 */
async function processItems(items, callback) {
    for (const item of items) {
        await callback(item);
    }
}
```
    
  </blockquote></details>
  <details open><summary>Default compact AI-friendly version (`default output (public only, no implementation)`)</summary><blockquote>
    
```
<file path="advanced-features.js">
const baseApi = { timeout, retry }
const dataService = { ...baseApi, cache, async getData(id, options) -> Promise, *generateBatch(count), get cacheSize(), set maxCacheSize(value) }

class SecureStore:
    constructor(key)
    async store(key, value)
/**
 * @param {string[]} items - Array of items
 * @param {Function} callback - Processing callback
 * @returns {Promise<void>}
 */
async function processItems(items: string[], callback: Function) -> Promise<void>
</file>
```
    
  </blockquote></details>
</blockquote></details>

## Usage Tips

1. **Use JSDoc comments** for type information - this greatly improves the AI's understanding of your code
2. **Export your public API** explicitly - use named exports or module.exports
3. **Keep object structures simple** - deeply nested objects may not be fully analyzed
4. **Prefer ES6 modules** over CommonJS for better analysis
5. **Use meaningful names** - since implementations can be stripped, good naming is crucial

## Command Examples

```bash
# Analyze a single JavaScript file
aid app.js

# Analyze a React project, showing only public APIs (the defaults)
aid src/ --include "*.js,*.jsx"

# Generate JSON output for tooling integration
aid src/ --format json-structured --output structure.json

# Focus on TypeScript files only
aid src/ --include "*.ts" --exclude "*.test.ts"
```