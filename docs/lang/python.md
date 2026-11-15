# Python Language Support

AI Distiller supports Python 3.x code through the official [tree-sitter-python](https://github.com/tree-sitter/tree-sitter-python) grammar (v0.23.6), compiled into the `aid` binary.

## Overview

Python support in AI Distiller is designed to extract the essential structure of Python code while preserving type information and API contracts. The distilled output maintains Python's semantic meaning while dramatically reducing token count for AI consumption.

## Supported Python Constructs

### Core Language Features

| Construct | Support Level | Notes |
|-----------|--------------|-------|
| **Classes** | ✅ Full | Base classes and class decorators are kept; a subscripted base (`Generic[T]`) and `metaclass=` are dropped |
| **Functions** | ⚠️ Partial | Regular functions, generators and decorators; the `async` keyword is dropped |
| **Methods** | ✅ Full | Instance, class, static methods and properties |
| **Type Hints** | ⚠️ Partial | Parameter, return and variable annotations are kept; PEP 695 type parameters (`def f[T]`, `class Box[T]`) and `*args: *Ts` are dropped |
| **Decorators** | ✅ Full | Function and class decorators (`--annotations=0` removes them) |
| **Imports** | ✅ Full | Regular, from, aliases; in text output, imports the distilled code does not use are dropped |
| **Docstrings** | ✅ Full | Printed by default under the class or function they document; `--docstrings=0` removes them |
| **Async/Await** | ⚠️ Partial | `async def` prints without `async` |
| **Dataclasses** | ✅ Full | Annotated class attributes (`name: str = "x"`) are kept |
| **Enums** | ✅ Full | Treated as classes |
| **Metaclasses** | ⚠️ Partial | `metaclass=Meta` is dropped from the class header |
| **Nested Classes** | ✅ Full | Printed inside their class |

### Visibility Rules

Python visibility in AI Distiller follows these conventions:
- **Public**: no underscore prefix, or a dunder name (`__init__`, `__str__`, etc.)
- **Private**: any other name that starts with an underscore, `_method` as well as `__method`

Python has no protected or internal level in AI Distiller: `--private=1` shows the underscore members, while `--protected=1` and `--internal=1` change nothing.

Attributes assigned in a method (`self.name = name`) are listed as fields of the class, before the method that assigns them.

## Key Features

### 1. **Type Information Extraction**

AI Distiller keeps the type annotations exactly as written:

```python
def process_data(items: List[Dict[str, Any]], 
                 callback: Callable[[str], None] = None) -> Optional[DataFrame]:
    """Process items and optionally notify via callback."""
    pass
```

```
<file path="process.py">
process_data(items: List[Dict[str, Any]], callback: Callable[[str], None] = None) -> Optional[DataFrame]
    """Process items and optionally notify via callback."""
</file>
```

### 2. **Docstring Handling**

Docstrings are documentation, not implementation: they are printed by default, also with `--implementation=0`, and `--docstrings=0` removes them. A function's docstring is printed under its signature, indented one level deeper, where Python has it:

```python
def calculate(x: float) -> float:
    """Calculate the square root of x."""
    return math.sqrt(x)
```

```
<file path="calculate.py">
calculate(x: float) -> float
    """Calculate the square root of x."""
</file>
```

### 3. **Imports**

Imports are printed as written. In text output, an import that the distilled code no longer uses (for example one used only inside a function body that is not printed) is dropped; `--filter-imports=false` keeps every import and `--imports=0` removes them all.

```python
from typing import List, Optional
from pandas import DataFrame
import numpy as np
from .utils import helper
```

## Output Formats

### Text Format (Recommended for AI)

In the text format:
- a public function or method prints as its signature without `def` and without the colon (`get_display_name(self) -> str`); with its body (`--implementation=1`) it prints as `def name(...):` followed by the body
- a private member (name starting with `_`) is prefixed with `-` (`-def _generate_id(self) -> str:`)
- classes print as `class Name(Base):` with their members indented

<details open>
<summary>Example: Basic Class</summary>
<blockquote>

<details>
<summary>Input: `user.py`</summary>
<blockquote>

```python
class User:
    """Represents a user in the system."""
    
    def __init__(self, name: str, email: str):
        self.name = name
        self.email = email
        self._id = self._generate_id()
    
    def get_display_name(self) -> str:
        """Returns the user's display name."""
        return self.name.title()
    
    def _generate_id(self) -> str:
        """Internal method to generate user ID."""
        return f"usr_{hash(self.email)}"
```

</blockquote>
</details>

<details open>
<summary>Default output (`aid user.py`: public only, no implementation)</summary>
<blockquote>

```
<file path="user.py">

class User:
    """Represents a user in the system."""
    name = name
    email = email
    __init__(self, name: str, email: str)
    get_display_name(self) -> str
        """Returns the user's display name."""
</file>
```

</blockquote>
</details>

<details>
<summary>Full output (`aid user.py --private=1 --protected=1 --internal=1 --implementation=1`)</summary>
<blockquote>

```
<file path="user.py">

class User:
    """Represents a user in the system."""
    name = name
    email = email
    -_id = self._generate_id()
    def __init__(self, name: str, email: str):
        self.name = name
        self.email = email
        self._id = self._generate_id()
    def get_display_name(self) -> str:
        """Returns the user's display name."""
        return self.name.title()
    -def _generate_id(self) -> str:
        """Internal method to generate user ID."""
        return f"usr_{hash(self.email)}"
</file>
```

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Complex Async Service</summary>
<blockquote>

<details>
<summary>Input: `service.py`</summary>
<blockquote>

```python
from typing import AsyncIterator, Optional
from abc import ABC, abstractmethod
import asyncio

class AsyncService(ABC):
    """Abstract base class for async services."""
    
    def __init__(self, name: str, timeout: float = 30.0):
        self.name = name
        self.timeout = timeout
        self._running = False
    
    @abstractmethod
    async def process(self, data: bytes) -> bytes:
        """Process data asynchronously."""
        pass
    
    async def start(self) -> None:
        """Start the service."""
        self._running = True
        await self._initialize()
    
    async def stop(self) -> None:
        """Stop the service."""
        self._running = False
        await self._cleanup()
    
    async def stream_data(self) -> AsyncIterator[bytes]:
        """Stream processed data."""
        while self._running:
            chunk = await self._get_next_chunk()
            if chunk:
                yield await self.process(chunk)
    
    async def _initialize(self) -> None:
        """Initialize service resources."""
        await asyncio.sleep(0.1)
    
    async def _cleanup(self) -> None:
        """Clean up service resources."""
        await asyncio.sleep(0.1)
    
    async def _get_next_chunk(self) -> Optional[bytes]:
        """Get next data chunk."""
        return b"data"
```

</blockquote>
</details>

<details open>
<summary>Default output (`aid service.py`: public only, no implementation)</summary>
<blockquote>

```
<file path="service.py">
from typing import AsyncIterator, Optional
from abc import ABC, abstractmethod

class AsyncService(ABC):
    """Abstract base class for async services."""
    name = name
    timeout = timeout
    __init__(self, name: str, timeout: float = 30.0)
    @abstractmethod
    process(self, data: bytes) -> bytes
        """Process data asynchronously."""
    start(self) -> None
        """Start the service."""
    stop(self) -> None
        """Stop the service."""
    stream_data(self) -> AsyncIterator[bytes]
        """Stream processed data."""
</file>
```

The `async` keyword is missing (see Known Issues), and `import asyncio` is dropped because only the bodies, which are not printed, use it.

</blockquote>
</details>

</blockquote>
</details>

<details>
<summary>Example: Advanced Type Annotations</summary>
<blockquote>

<details>
<summary>Input: `advanced_types.py`</summary>
<blockquote>

```python
from typing import TypeVar, Generic, Protocol, Union, Literal, TypedDict
from typing import overload

T = TypeVar('T')
K = TypeVar('K')
V = TypeVar('V')

class Comparable(Protocol):
    """Protocol for comparable objects."""
    def __lt__(self, other: 'Comparable') -> bool: ...
    def __le__(self, other: 'Comparable') -> bool: ...

class ConfigDict(TypedDict):
    """Configuration dictionary structure."""
    host: str
    port: int
    debug: bool
    features: list[str]

class Cache(Generic[K, V]):
    """Generic cache implementation."""
    
    def __init__(self, max_size: int = 100):
        self._cache: dict[K, V] = {}
        self.max_size = max_size
    
    @overload
    def get(self, key: K, default: None = None) -> Union[V, None]: ...
    
    @overload
    def get(self, key: K, default: V) -> V: ...
    
    def get(self, key: K, default: Union[V, None] = None) -> Union[V, None]:
        """Get value from cache."""
        return self._cache.get(key, default)
    
    def put(self, key: K, value: V) -> None:
        """Put value in cache."""
        if len(self._cache) >= self.max_size:
            self._evict_oldest()
        self._cache[key] = value
    
    def _evict_oldest(self) -> None:
        """Evict oldest entry from cache."""
        if self._cache:
            oldest = next(iter(self._cache))
            del self._cache[oldest]

def process_config(config: ConfigDict, 
                  mode: Literal['dev', 'prod'] = 'prod') -> bool:
    """Process configuration based on mode."""
    return config['debug'] if mode == 'dev' else False
```

</blockquote>
</details>

<details open>
<summary>Default output (`aid advanced_types.py`: public only, no implementation)</summary>
<blockquote>

```
<file path="advanced_types.py">
from typing import TypeVar, Generic, Protocol, Union, Literal, TypedDict
from typing import overload
T = TypeVar('T')
K = TypeVar('K')
V = TypeVar('V')

class Comparable(Protocol):
    """Protocol for comparable objects."""
    __lt__(self, other: 'Comparable') -> bool
    __le__(self, other: 'Comparable') -> bool

class ConfigDict(TypedDict):
    """Configuration dictionary structure."""
    host: str
    port: int
    debug: bool
    features: list[str]

class Cache:
    """Generic cache implementation."""
    max_size = max_size
    __init__(self, max_size: int = 100)
    @overload
    get(self, key: K, default: None = None) -> Union[V, None]
    @overload
    get(self, key: K, default: V) -> V
    get(self, key: K, default: Union[V, None] = None) -> Union[V, None]
        """Get value from cache."""
    put(self, key: K, value: V) -> None
        """Put value in cache."""
process_config(config: ConfigDict, mode: Literal['dev', 'prod'] = 'prod') -> bool
    """Process configuration based on mode."""
</file>
```

`class Cache(Generic[K, V])` prints as `class Cache:` (see Known Issues).

</blockquote>
</details>

</blockquote>
</details>

## Known Issues

1. **Missing `async` keyword**
   - **Issue**: `async def` functions and methods print without `async`
   - **Impact**: the output does not show which functions are coroutines

2. **Missing metaclass parameters**
   - **Issue**: `class MyClass(Base, metaclass=Meta)` prints as `class MyClass(Base):`
   - **Workaround**: Document metaclass usage in the docstring

3. **Subscripted base classes are dropped**
   - **Issue**: `class Cache(Generic[K, V])` prints as `class Cache:`

4. **PEP 695 type parameters and `*args: *Ts` are dropped**
   - **Issue**: `def first[T](xs: list[T]) -> T` prints as `first(xs: list[T]) -> T`, `class Box[T]` as `class Box:`, and `def star(*a: *Ts)` as `star()`

5. **Type comment support**
   - **Issue**: PEP 484 type comments are not extracted
   - **Example**: `x = []  # type: List[int]` prints as `x = []`
   - **Workaround**: Use PEP 526 annotations (`x: List[int] = []`)

## Best Practices

### 1. **Write Type-Annotated Code**

AI Distiller works best with fully typed Python:

```python
# Good - Full type information preserved
def process(data: List[Dict[str, Any]], 
           options: Optional[ProcessOptions] = None) -> Result:
    pass

# Less optimal - Types lost
def process(data, options=None):
    pass
```

### 2. **Use Explicit Visibility**

Make your intentions clear. AI Distiller treats every name that starts with an underscore (except dunders) as private, so both helpers below are left out of the default output:

```python
class Service:
    def public_method(self):  # Public
        pass
    
    def _internal_method(self):  # Private in aid output
        pass
    
    def __private_method(self):  # Private in aid output
        pass
```

### 3. **Structure for AI Consumption**

- Keep modules focused and under 1000 lines
- Use descriptive names (they're preserved!)
- Add type hints everywhere possible
- Write clear, concise docstrings

### 4. **Pre-process Your Code**

Before distilling:

```bash
# Format code consistently
black .

# Add type stubs
mypy --install-types

# Remove secrets
detect-secrets scan --baseline .secrets.baseline
```

## Integration Examples

### Direct CLI Usage

```bash
# Generate context for AI assistants (the defaults already keep only the public API,
# without implementation and comments)
aid ./myproject -o context.txt

# Include in your prompt
cat <<EOF > prompt.txt
Here's my codebase structure:

$(cat context.txt)

Please help me implement a new feature that...
EOF
```

### MCP Integration

The MCP server `@janreges/ai-distiller-mcp` gives Claude and other MCP clients tools such as `distill_file`, `distill_directory` and `list_files`. Setup and the full tool list are in [`mcp-npm/README.md`](../../mcp-npm/README.md):

```bash
claude mcp add aid -- npx -y @janreges/ai-distiller-mcp
```

**Example Conversation:**

> **You**: I need to add a caching layer to our user authentication. First, show me the current structure of our auth module.

> **Claude**: I'll analyze your authentication module structure using AI Distiller.

*Claude calls:*
```
list_files({ path: "auth/", pattern: "*.py" })
distill_file({ file_path: "auth/authenticator.py" })
```

*Claude receives the distilled structure (public signatures, types and docstrings) and plans the change from the real API of the module.*

**Feature implementation across a directory:**

> **You**: We need to implement API rate limiting. Show me all middleware classes and where request handling happens.

*Claude calls:*
```
distill_directory({ directory_path: "middleware/" })
distill_file({ file_path: "app.py" })
```

`distill_directory` returns every class and method signature of the directory in one call, which is cheaper than calling `distill_file` on each file.

### With Documentation Tools

```bash
# List the public classes as JSON
aid ./src --format json-structured --stdout | \
    jq -r '.files[].structure.classes[]?.name'
```

### CI/CD Integration

```yaml
# .github/workflows/api-check.yml
- name: Check API surface
  run: |
    aid . --stdout > api-new.txt
    diff api-baseline.txt api-new.txt || exit 1
```

## Language-Specific Tips

1. **Handle Dynamic Imports**: Static analysis can't detect dynamic imports. Document them:
   ```python
   # AI-DISTILLER: Dynamic imports from plugins/* 
   ```

2. **Type Aliases**: Define at module level for better extraction:
   ```python
   UserID = NewType('UserID', int)
   SessionData = Dict[str, Any]
   ```

3. **Protocol vs ABC**: Prefer `Protocol` for better type information:
   ```python
   # Better for AI Distiller
   class Drawable(Protocol):
       def draw(self) -> None: ...
   ```

4. **Async Context Managers**: Currently shown as regular methods:
   ```python
   async def __aenter__(self): ...  # Shows as regular method
   ```

## Comparison with Other Tools

| Tool | Purpose | Python Support | AI-Optimized |
|------|---------|---------------|--------------|
| **AI Distiller** | Code structure extraction | Full | ✅ Yes |
| `ast.parse()` | AST analysis | Python only | ❌ No |
| `inspect` | Runtime introspection | Python only | ❌ No |
| `cloc` | Line counting | Basic | ❌ No |
| `ctags` | Symbol indexing | Limited | ❌ No |

## Troubleshooting

### Missing or incomplete output for a file

AI Distiller does not stop on a syntax error, but a file that does not parse can come out incomplete. Check it with:
```bash
python -m py_compile yourfile.py
```

### "Missing async keyword in output"

Known issue (see Known Issues): `async def` prints as a plain function.

## Future Enhancements

- [ ] Full `async`/`await` support
- [ ] Metaclass parameter preservation  
- [ ] Type comment (PEP 484) support
- [ ] PEP 695 type parameters
- [ ] Protocol implementation detection
- [ ] Import cycle detection

## Contributing

Help improve Python support! See [CONTRIBUTING.md](../../CONTRIBUTING.md) for guidelines.

Key areas needing help:
- Complex type annotation examples
- Real-world test cases
- Performance optimization
