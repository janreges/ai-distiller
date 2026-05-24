# AI Distiller - Claude Development Instructions

## Project Overview

AI Distiller is a high-performance CLI tool that extracts essential code structure from large codebases, making them digestible for LLMs by removing unnecessary details while preserving semantic information. Think of it as "code compression for AI context windows."

## Key Project Goals

1. **Single native binary** - No runtime dependencies, works everywhere
2. **Blazingly fast** - Process 10MB codebase in <2 seconds
3. **Multi-language support** - 15+ languages via tree-sitter WASM
4. **Flexible output** - Text, Markdown, JSON, JSONL, XML formats
5. **Granular control** - Strip exactly what you don't need

## CLI Interface Specification

The main CLI is `aid` (AI Distiller):

```bash
aid [path] [flags]

# Examples:
aid                                    # Process current directory
aid src/                              # Process src directory
aid main.py                           # Process single file
aid --comments=0,implementation   # Remove comments and implementations
aid --format json-structured --output api.json   # JSON output to file
aid --private=0 --protected=0 --internal=0 --stdout       # Print only public members to stdout

# Special git mode (activated when path is .git):
aid .git                              # Show full git history
aid .git --git-limit=50              # Show last 50 commits
aid .git --with-analysis-prompt      # Include AI analysis prompt for insights

# AI-powered analysis modes:
aid --ai-action=flow-for-deep-file-to-file-analysis     # Generate comprehensive task list
aid --ai-action=flow-for-multi-file-docs                # Generate documentation workflow
aid --ai-action=prompt-for-refactoring-suggestion       # Generate refactoring analysis prompt
aid --ai-action=prompt-for-complex-codebase-analysis    # Full codebase analysis with diagrams
aid --ai-action=prompt-for-security-analysis            # Security-focused analysis prompt
aid --ai-action=prompt-for-performance-analysis         # Performance optimization analysis
aid --ai-action=prompt-for-best-practices-analysis      # Best practices and code quality analysis
aid --ai-action=prompt-for-bug-hunting                  # Systematic bug hunting analysis
aid --ai-action=prompt-for-single-file-docs             # Single file documentation analysis
aid --ai-action=prompt-for-diagrams                     # Generate 10 beneficial Mermaid diagrams

# Legacy mode (deprecated):
aid --ai-analysis-task-list                    # Use --ai-action=flow-for-deep-file-to-file-analysis instead
```

### Important Flags

**NEW: Individual Filtering Flags (Recommended)**

The new flag system provides precise control over what to include:

**Visibility Flags** (control which members to show based on access level):
- `--public=0/1` (default: 1) - Include public members
- `--protected=0/1` (default: 0) - Include protected members
- `--internal=0/1` (default: 0) - Include internal/package-private members
- `--private=0/1` (default: 0) - Include private members

**Content Flags** (control what content to include):
- `--comments=0/1` (default: 0) - Include comments
- `--docstrings=0/1` (default: 1) - Include documentation comments
- `--implementation=0/1` (default: 0) - Include function/method bodies
- `--expand "<glob>"` (default: none) - Keep full implementation ONLY for symbols whose name matches (comma-separated / repeatable). Matches a function/method name, or a class/struct name to expand all of its methods. Signatures everywhere else.
- `--imports=0/1` (default: 1) - Include import statements
- `--annotations=0/1` (default: 1) - Include decorators/annotations
- `--fields=0/1` (default: 1) - Include class fields and properties
- `--methods=0/1` (default: 1) - Include methods and functions

**Group Filtering** (alternative syntax):
- `--include-only=public,protected,imports` - Include only these categories
- `--exclude-items=private,comments` - Exclude these categories

**File Pattern Filtering** (NEW - supports multiple syntaxes):
- `--include "*.go,*.py,*.ts"` - Comma-separated patterns
- `--include "*.go" --include "*.py"` - Multiple flags  
- `--exclude "*test*,*spec*,*.json"` - Exclude patterns
- `--exclude "*test*" --exclude "*.json"` - Multiple exclusions

**Examples:**
```bash
# Default: public APIs only
aid src/

# Include all visibility levels
aid src/ --public=1 --protected=1 --internal=1 --private=1

# Include implementation details
aid src/ --implementation=1

# Exclude comments but include everything else
aid src/ --exclude-items=comments

# Only public and protected members with imports
aid src/ --include-only=public,protected,imports

# Extract only method signatures (no fields/properties) - great for large codebases  
aid src/ --fields=0 --implementation=0

# Signatures everywhere, but full bodies only for selected symbols (agent context building)
aid src/ --expand "GetUser"          # full body of GetUser, or all methods of class/struct GetUser
aid src/ --expand "*Service*,run"    # multiple glob patterns

# Extract only data structures (no method noise)
aid models/ --methods=0

# Focus on public API methods only
aid services/ --fields=0 --private=0 --protected=0 --internal=0

# Performance control
aid large-project/ --workers=1        # Single-threaded for debugging
aid huge-codebase/ --workers=8        # Use 8 parallel workers
aid . --recursive=0                   # Only current directory, no subdirs
```

**Legacy Flag (Deprecated):**
- `--strip <items>` - Still works but deprecated
  - Values: `comments`, `imports`, `implementation`, `non-public`, `private`, `protected`
  - Comma-separated: `--comments=0,implementation`
  
**Output Format:**
- `--format <fmt>` - Output format
  - `text` (default) - Ultra-compact plaintext (best for AI)
  - `md` - Clean, structured Markdown
  - `jsonl` - One JSON object per file
  - `json-structured` - Rich semantic data
  - `xml` - Structured XML

**Output File:**
- `-o, --output <file>` - Output file (default: auto-generated)
  - Default pattern: `.aid.<dirname>.[options].txt`
  - Example: `.aid.MyProject.prot.int.impl.txt` (includes protected, internal, implementation)

**Performance & Processing:**
- `-w, --workers <num>` - Parallel workers (default: 0)
  - `0` = auto (80% CPU cores)
  - `1` = serial processing
  - `2+` = specific number of parallel workers
- `-r, --recursive <0/1>` - Process directories recursively (default: 1)
  - `1` = process subdirectories (default)
  - `0` = only process files in specified directory

**AI Actions:**
- `--ai-action <action>` - AI-powered analysis action
  - `flow-for-deep-file-to-file-analysis` - Generate task list for systematic analysis
  - `prompt-for-refactoring-suggestion` - Refactoring analysis prompt
  - `prompt-for-complex-codebase-analysis` - Comprehensive analysis with architecture diagrams
  - `prompt-for-security-analysis` - Security audit prompt with vulnerability detection
- `--ai-output <path>` - Custom output path for AI action (default: action-specific)
  - Each action has sensible defaults like `./.aid/ACTION-NAME.%YYYY-MM-DD.HH-MM-SS%.%folder-basename%.md`

**Git Mode:**
- `--git-limit <n>` - Number of commits to show (default: 200, 0=all)
- `--with-analysis-prompt` - Prepend AI analysis prompt for comprehensive insights
  - Activated automatically when path is `.git`
  - Shows commit history in clean format: `[hash] date time | author | subject`
  - Multi-line commit messages are properly indented
  - With analysis prompt: Guides AI to generate statistics, patterns, and insights

Example output:
```
[1b4aa1b] 2025-06-17 00:56:32 | jan.reges            | fix(formatters): major C++ and Ruby formatter improvements
        - Ruby: Add 'def' keyword to methods, remove Python-style colons from class/module declarations
        - C++: Fix critical bug where implementation was always shown in text format
        - C++: Improve return type parsing - collect types before function declarator
        - C++: Fix parameter type extraction
        This resolves major issues in both languages' output formatting.

[816a7d3] 2025-06-17 00:39:48 | jan.reges            | fix(swift): improve Swift line parser formatting
        - Add func keyword to function declarations
        - Fix protocol/class/struct inheritance regex to exclude opening braces
        - Fix protocol property regex to be optional
```

With `--with-analysis-prompt`, the output includes a comprehensive AI prompt that guides analysis of:
- Contributor statistics and expertise areas
- Timeline analysis and development patterns
- Functional categorization of commits
- Codebase evolution insights
- Interesting discoveries and recommendations

## Quick Testing with stdin

For rapid testing and experimentation with AI Distiller, use stdin input:

```bash
# Test code snippets without creating files
echo 'class UserService:
    def get_user(self, id):
        return self.db.find(id)' | aid --format text

# Automatic language detection + automatic --stdout
cat snippet.ts | aid

# Force specific language if needed
echo 'const x = 10' | aid --lang javascript --implementation=0
```

**Benefits for AI assistants:**
- No need to create temporary files
- Instant feedback on code structure extraction
- Perfect for testing parser behavior
- Automatic `--stdout` when using stdin
- Language auto-detection from code patterns

**Supported languages for auto-detection:** python, typescript, javascript, go, ruby, swift, rust, java, c#, kotlin, c++, php

## Git Mode - Special Feature

When you pass a `.git` directory path to aid, it switches to a special git log mode:

```bash
# Show full git commit history
aid .git

# Limit to latest 50 commits
aid .git --git-limit=50

# Example output:
abc123 2025-06-15 14:30:00 +0100 john.doe
    feat: add new authentication system
    - Implement JWT token generation
    - Add user session management
    - Update API endpoints

def456 2025-06-15 11:20:00 +0100 jane.smith
    fix: resolve memory leak in parser
    Fixed issue where large files caused excessive memory usage
```

This mode is useful for:
- Quickly reviewing project history
- Generating commit summaries for AI context
- Understanding project evolution
- Creating release notes

## Project Root Detection and Output Organization

AI Distiller automatically detects your project root directory to centralize all outputs:

### Detection Strategy (in order of priority):
1. **`.aidrc` file** - Empty marker file to explicitly define project root (highest priority)
2. **Language markers** - `go.mod`, `package.json`, `pyproject.toml`, `Cargo.toml`, etc.
3. **Version control** - `.git` directory
4. **AID_PROJECT_ROOT** environment variable (fallback if no markers found)
5. **Current working directory** - Final fallback with warning

### Implementation Details:
- Searches up to 12 parent directories from current location
- Stops at home directory for security (won't traverse above ~)
- All outputs go to `<project-root>/.aid/` regardless of where `aid` is run
- MCP cache goes to `<project-root>/.aid/cache/mcp/`
- Cache TTL: 5 minutes for MCP operations

### Key Files:
- `internal/project/root.go` - Project root detection logic
- `internal/project/root_test.go` - Comprehensive tests for detection
- Detection happens once per process and is cached

## Expected Output Examples

### Text Format (Ultra-Compact for AI)

This is the most compact format, optimized for maximum context efficiency. **Best choice for AI consumption** because:
- Minimal syntax overhead
- Natural code-like appearance
- Maximum information density
- Clear file boundaries with `<file path="...">` tags
- No decorative elements (emojis, tables, etc.)

```
<file path="src/user_service.py">
from typing import List, Optional
from datetime import datetime

class UserService:
    def __init__(self, db_connection):
        self.db = db_connection
        self._cache = {}
    
    def get_user(self, user_id: int) -> Optional[User]:
        # Implementation here if not stripped
        
    def _validate_email(self, email: str) -> bool:
        # Private method
</file>

<file path="src/models.py">
class User:
    def __init__(self, id: int, name: str, email: str):
        self.id = id
        self.name = name
        self.email = email
</file>
```

With `--implementation=0,non-public`:

```
<file path="src/user_service.py">
from typing import List, Optional
from datetime import datetime

class UserService:
    def __init__(self, db_connection)
    def get_user(self, user_id: int) -> Optional[User]
</file>

<file path="src/models.py">
class User:
    def __init__(self, id: int, name: str, email: str)
</file>
```

### Markdown Format (Default)

```markdown
# src/user_service.py

## Structure

📥 **Import** from `typing` import `List`, `Optional` <sub>L1</sub>
📥 **Import** from `datetime` import `datetime` <sub>L2</sub>

🏛️ **Class** `UserService` <sub>L5-45</sub>
  🔧 **Function** `__init__`(`self`, `db_connection`) <sub>L8-10</sub>
    ```python
    self.db = db_connection
    self._cache = {}
    ```
  🔧 **Function** `get_user`(`self`, `user_id`: `int`) → `Optional[User]` <sub>L12-18</sub>
  🔧 **Function** `_validate_email` _private_(`self`, `email`: `str`) → `bool` <sub>L20-25</sub>
```

### With `--implementation=0`:

```markdown
# src/user_service.py

## Structure

📥 **Import** from `typing` import `List`, `Optional` <sub>L1</sub>
📥 **Import** from `datetime` import `datetime` <sub>L2</sub>

🏛️ **Class** `UserService` <sub>L5-45</sub>
  🔧 **Function** `__init__`(`self`, `db_connection`) <sub>L8-10</sub>
  🔧 **Function** `get_user`(`self`, `user_id`: `int`) → `Optional[User]` <sub>L12-18</sub>
  🔧 **Function** `_validate_email` _private_(`self`, `email`: `str`) → `bool` <sub>L20-25</sub>
```

### With `--private=0 --protected=0 --internal=0`:

```markdown
# src/user_service.py

## Structure

📥 **Import** from `typing` import `List`, `Optional` <sub>L1</sub>
📥 **Import** from `datetime` import `datetime` <sub>L2</sub>

🏛️ **Class** `UserService` <sub>L5-45</sub>
  🔧 **Function** `__init__`(`self`, `db_connection`) <sub>L8-10</sub>
  🔧 **Function** `get_user`(`self`, `user_id`: `int`) → `Optional[User]` <sub>L12-18</sub>
```

## Architecture & Implementation Flow

### 1. Component Architecture

```
User Input → CLI Parser → File Discovery → Language Detection →
Parser (tree-sitter WASM) → IR Generation → Stripper Visitor →
Output Formatter → File/Stdout
```

### 2. Directory Structure

```
ai-distiller/
├── cmd/
│   └── aid/              # Main CLI entry point
├── internal/
│   ├── cli/              # CLI logic and flags
│   ├── ir/               # Intermediate Representation
│   ├── parser/           # WASM runtime and tree-sitter
│   ├── language/         # Language-specific processors
│   │   ├── python/       # Python processor
│   │   ├── go/           # Go processor
│   │   └── javascript/   # JavaScript processor
│   ├── stripper/         # Visitor for stripping elements
│   ├── formatter/        # Output formatters
│   └── processor/        # Core processing logic
└── test-data/            # Test files and scenarios
```

### 3. Key Implementation Details

#### Language Processor Interface

```go
type LanguageProcessor interface {
    Language() string
    Version() string
    SupportedExtensions() []string
    CanProcess(filename string) bool
    Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error)
}
```

#### Stripper Options

```go
type Options struct {
    RemovePrivate         bool  // --private=0 --protected=0 --internal=0
    RemoveImplementations bool  // --implementation=0
    RemoveComments        bool  // --comments=0
    RemoveImports         bool  // --imports=0
}
```

#### IR Node Types

- `DistilledFile` - Root node for a file
- `DistilledClass` - Class/struct definition
- `DistilledFunction` - Function/method
- `DistilledImport` - Import statement
- `DistilledComment` - Comment/docstring
- `DistilledField` - Class field/property

## Development Workflow for AI Assistants

### Quick Development Testing

For rapid development and testing, use the `make aid` command which automatically builds and runs the aid command with any arguments:

```bash
# Instead of:
make && ./aid test.php --stdout

# Use:
make aid test.php --stdout

# Works with all parameters:
make aid testdata/php/06_edge_cases/source.php -vvv
make aid --help
```

This saves time during development by combining the build and run steps.

### CRITICAL: No Mocks or Simulated Functions

**NEVER create mock implementations or simulated functions.** All code must be real, working implementations. This includes:
- No hardcoded test data pretending to be parsed results
- No mock parsers returning fixed data
- No placeholder functions that don't actually work
- Tests must test REAL functionality, not mocked behavior

If something can't be implemented properly, document it as TODO but don't create fake implementations.

### 1. When Adding New Features

1. **Check existing patterns** - Look at similar features
2. **Write REAL implementation** - No mocks or stubs
3. **Update tests first** - TDD approach with real tests
4. **Follow architecture** - Don't break the visitor pattern
5. **Test with real files** - Use test-data/ directory

### 2. When Fixing Bugs

1. **Reproduce in test** - Add failing test case
2. **Fix minimally** - Don't refactor unnecessarily
3. **Run all tests** - `make test` in project root
4. **Check performance** - Must stay fast

### 3. Common Tasks

#### Adding a new language:

1. Create `internal/language/<lang>/processor.go`
2. Implement `LanguageProcessor` interface
3. Add tree-sitter WASM grammar
4. Register in `internal/language/registry.go`
5. Add comprehensive tests

#### Adding a new output format:

1. Create formatter in `internal/formatter/`
2. Implement `Formatter` interface
3. Register in formatter registry
4. Add tests for all node types

Example for text formatter:
```go
// internal/formatter/text.go
type TextFormatter struct {
    options Options
}

func (f *TextFormatter) Format(w io.Writer, node ir.DistilledNode) error {
    if file, ok := node.(*ir.DistilledFile); ok {
        fmt.Fprintf(w, "<file path=\"%s\">\n", file.Path)
        
        // Write distilled content as plain text
        for _, child := range file.Children {
            f.formatNode(w, child, 0)
        }
        
        fmt.Fprintln(w, "</file>")
    }
    return nil
}

func (f *TextFormatter) formatNode(w io.Writer, node ir.DistilledNode, indent int) {
    switch n := node.(type) {
    case *ir.DistilledImport:
        if n.ImportType == "from" {
            fmt.Fprintf(w, "from %s import %s\n", n.Module, formatSymbols(n.Symbols))
        } else {
            fmt.Fprintf(w, "import %s\n", n.Module)
        }
    case *ir.DistilledClass:
        fmt.Fprintf(w, "\nclass %s", n.Name)
        if len(n.Extends) > 0 {
            fmt.Fprintf(w, "(%s)", formatTypeRefs(n.Extends))
        }
        fmt.Fprintln(w, ":")
        // Format children with indentation
    case *ir.DistilledFunction:
        fmt.Fprintf(w, "%s%s(%s)", 
            strings.Repeat("    ", indent),
            n.Name,
            formatParams(n.Parameters))
        if n.Returns != nil {
            fmt.Fprintf(w, " -> %s", n.Returns.Name)
        }
        if !f.options.Compact && n.Implementation != "" {
            fmt.Fprintf(w, ":\n%s", n.Implementation)
        }
        fmt.Fprintln(w)
    }
}
```

#### Modifying strip behavior:

1. Update `internal/stripper/stripper.go`
2. Modify the `Visit` method for affected nodes
3. Test with various combinations

## Testing Strategy

### Unit Tests
```bash
# Run all tests
make test

# Run specific package tests
go test ./internal/stripper/...
```

### Integration Tests
```bash
# In test-data directory
make test-all      # Run all test types
make test-unit     # Unit tests only
make test-quick    # Quick smoke tests
```

### Test Scenarios

Expected files use new naming convention based on CLI parameters:
1. **default.txt** - Default output (public only, no implementation)
2. **implementation=1.txt** - With implementation bodies
3. **private=1,protected=1,internal=1,implementation=0.txt** - All visibility levels
4. **imports=0.txt** - Without imports
5. **comments=1.txt** - With comments
6. File names reflect non-default parameters (e.g., `private=1.txt` when private is enabled)

## Performance Guidelines

1. **Concurrent but ordered** - Process files in parallel, maintain output order
2. **Stream everything** - Don't load entire codebases in memory
3. **One-pass visiting** - No multiple IR traversals
4. **Bounded channels** - Prevent memory explosions

## Common Pitfalls & Solutions

### Issue: Tests expect old flag system
**Solution**: Use new individual flags: `--private=1`, `--protected=1`, etc. Default shows only public members.

### Issue: Parser doesn't find all constructs
**Solution**: Check if line-based parser limitations; full AST via tree-sitter coming

### Issue: Text format not preserving syntax exactly
**Solution**: Text format aims for readability and compactness, not valid source code

### Issue: Output format inconsistency
**Solution**: All formatters must pass `format_validator.go` tests

### Issue: Performance degradation
**Solution**: Profile with `go test -bench`, check for unnecessary allocations

## Visibility Symbols in Text Format

The text format uses these symbols for visibility:
- **public**: no symbol (default)
- **private**: `-` (minus)
- **protected**: `*` (asterisk)
- **internal/package-private**: `~` (tilde)

Example:
```
class UserService:
    -_cache: dict        # private field
    *_logger: Logger     # protected field
    ~_config: Config     # internal/package-private field
    get_user(id: int)    # public method (no symbol)
    -_validate()         # private method
    *log_access()        # protected method
    ~process_internal()  # internal method
```

## Communication with User

- Use **Czech** for general communication if user writes in Czech
- Use **English** for all code, comments, and technical documentation
- Be concise but thorough
- Show real examples when explaining

## Communication with AI Assistants (Gemini, o3)

- Always communicate in **English** when using Zen MCP tools
- Use 'pro' model for Gemini for deep analysis
- Use 'o3' model (not 'o3-mini') for o3 conversations
- Request deep thinking modes when appropriate

## Debugging System

AI Distiller now has a comprehensive 3-level debugging system accessible via CLI flags:

### Debug Levels

- **Level 1 (-v)**: Basic info
  - File counts and processing summary
  - Phase transitions (parsing, formatting)
  - Configuration details
  - Worker configuration

- **Level 2 (-vv)**: Detailed info  
  - Individual file processing steps
  - Parser selection and language detection
  - Timing information for operations
  - Stripper configuration details
  - AST node counts

- **Level 3 (-vvv)**: Full trace with data dumps
  - Complete IR data structures (like PHP's print_r)
  - Raw and stripped IR comparison
  - Detailed parsing steps
  - Full configuration dumps

### How to Use Debugging

```bash
# Basic debugging
aid src/ -v

# Detailed debugging with timing
aid src/ -vv

# Full trace with data structures
aid src/ -vvv

# Debug specific file with full trace
aid main.py -vvv --implementation=1
```

### Implementation Details

The debugging system uses:
- **context.Context** for propagation through the pipeline
- **Debugger interface** with Logf(), Dump(), IsEnabledFor(), WithSubsystem()
- **Subsystem scoping** for different phases (parser, formatter, stripper)
- **go-spew** for human-readable data structure dumps
- **Performance optimization** with IsEnabledFor() checks to avoid expensive operations

### Adding Debug Output to Code

```go
// Get debugger from context
dbg := debug.FromContext(ctx).WithSubsystem("mymodule")

// Basic logging
dbg.Logf(debug.LevelBasic, "Processing %d files", count)

// Detailed logging with timing
defer dbg.Timing(debug.LevelDetailed, "parsing phase")()

// Trace-level data dumps
debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
    d.Dump(debug.LevelTrace, "Complete IR", irStructure)
})

// Performance-safe logging
if dbg.IsEnabledFor(debug.LevelDetailed) {
    dbg.Logf(debug.LevelDetailed, "Expensive info: %s", expensiveOperation())
}
```

### Debug Output Format

Debug output goes to stderr with format:
```
[HH:MM:SS.mmm] [subsystem] LEVEL: message
```

For data dumps at level 3:
```
[HH:MM:SS.mmm] [subsystem] DUMP: === Label ===
  (structured data using go-spew)
[HH:MM:SS.mmm] [subsystem] === End Label ===
```

## Git Commit Guidelines

### CRITICAL: Pre-commit Checklist

**Before EVERY commit, you MUST check for unwanted files:**

1. **Run `git status` to see all changes**
2. **Check for and remove:**
   - Temporary debug files (*.tmp, *.log, debug.*)
   - Built test binaries (aid, test executables)
   - Test output files (.aid.*.txt in root directory)
   - IDE/editor files (.vscode/, .idea/, *.swp)
   - Personal test files not meant for the repo
   - Any one-off debugging scripts

3. **Use `.gitignore` properly:**
   - Check if unwanted files should be in .gitignore
   - Add patterns for commonly generated files
   - Never commit files that should be ignored

4. **Clean commit checklist:**
   ```bash
   # Before committing, run:
   git status              # Check what will be committed
   git diff --cached       # Review actual changes
   ls -la                  # Look for unwanted files in root
   find . -name "*.tmp"    # Find temporary files
   find . -name "*.log"    # Find log files
   ```

5. **If you accidentally staged unwanted files:**
   ```bash
   git reset HEAD <file>   # Unstage specific file
   git clean -n            # Preview what would be cleaned
   git clean -f            # Remove untracked files (careful!)
   ```

Remember: A clean repository is a professional repository!

## Parser Development Guide

### Key Principles Learned from Go/TypeScript/Python/JavaScript

This guide documents the successful patterns discovered during systematic parser development:

#### 1. **Architecture Pattern: Tree-Sitter + Stripper Integration**

All parsers follow this proven architecture:

```go
// 1. Tree-sitter AST parsing
func (p *Processor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
    // Parse with tree-sitter
    file, err := p.treeparser.ProcessSource(ctx, source, filename)
    if err != nil {
        return nil, err
    }
    
    // 2. Apply standardized stripper
    stripperOpts := stripper.Options{
        RemovePrivate:         !opts.IncludePrivate,
        RemoveImplementations: !opts.IncludeImplementation,
        RemoveComments:        !opts.IncludeComments,
        RemoveImports:         !opts.IncludeImports,
    }
    
    if /* anything to strip */ {
        s := stripper.New(stripperOpts)
        stripped := file.Accept(s)
        return stripped.(*ir.DistilledFile), nil
    }
    
    return file, nil
}
```

#### 2. **Method Association Pattern**

Methods must be properly nested under their classes/structs:

- **Go**: Two-pass processing (collect types → associate methods)
- **TypeScript**: Single-pass with proper AST traversal  
- **Python**: Native tree-sitter handles this automatically
- **JavaScript**: Class methods parsed within class body

#### 3. **Interface/Protocol Satisfaction Detection**

Each language has its own approach:

- **Go**: Structural analysis comparing method sets
- **TypeScript**: Explicit `implements` + disabled structural (too complex)
- **Python**: Duck typing analysis with Protocol detection
- **JavaScript**: Inheritance tracking via `extends`

#### 4. **Visibility Handling**

Language-specific visibility rules:

- **Go**: Uppercase = public, lowercase = package-private
- **TypeScript**: `public`/`private`/`protected` keywords
- **Python**: `_private`, `__dunder__` = public API
- **JavaScript**: `#private`, JSDoc `@private`, `_convention`

#### 5. **Critical Implementation Details**

**Tree-sitter Node Text Safety:**
```go
func (p *Parser) nodeText(node *sitter.Node) string {
    if node == nil {
        return ""
    }
    start := node.StartByte()
    end := node.EndByte()
    sourceLen := uint32(len(p.source))
    if start > end || end > sourceLen {
        return ""
    }
    return string(p.source[start:end])
}
```

**Stripper Integration (CRITICAL):**
- NEVER use custom `applyOptions` - always use standardized `stripper.New()`
- Ensures consistent behavior across all languages

**AST Traversal Pattern:**
```go
func (p *Parser) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
    switch node.Type() {
    case "class_declaration", "abstract_class_declaration":
        p.parseClass(node, file, parent)
    case "function_declaration":
        p.parseFunction(node, file, parent)
    // ... other cases
    default:
        // Recurse into children
        for i := 0; i < int(node.ChildCount()); i++ {
            p.processNode(node.Child(i), file, parent)
        }
    }
}
```

### 6. **Testing Validation Pattern**

For each language:
1. Create complex examples with inheritance, generics, privacy
2. Test all stripping modes: full, `--implementation=0`, `--private=0 --protected=0 --internal=0`
3. Validate method association and interface detection
4. Compare outputs with AI assistant for accuracy review

### 7. **Common Pitfalls & Solutions**

❌ **Using line-based regex parsing** → ✅ Use tree-sitter AST  
❌ **Custom option filtering** → ✅ Use standardized stripper  
❌ **Missing method association** → ✅ Ensure proper parent/child IR structure  
❌ **Broken node text extraction** → ✅ Add boundary checks  
❌ **Ignoring language-specific features** → ✅ Handle generics, async, etc.

### 8. **Performance Guidelines**

- Single-pass AST traversal where possible
- Bounded context (avoid full-program analysis)
- Cache tree-sitter parsers
- Stream processing for large files

## Next Steps & TODOs

1. **More languages** - Java, C#, Rust following these patterns
2. **Semantic features** - Call graphs, dependency analysis  
3. **Performance optimization** - Sub-50ms for small files
4. **Release automation** - GitHub Actions for multi-platform builds

Remember: The goal is to make code understandable for AI, not humans. Optimize for context efficiency!

## Comprehensive Language Testing Protocol

### Original Instructions from User

The user requested systematic comprehensive testing of AI Distiller across all 12+ supported programming languages with the following specific workflow:

### Testing Workflow for Each Language

1. **Gemini Construct Design Phase**
   - Collaborate with Gemini to design 5 constructs per language (basic → very complex)
   - Each construct should represent real-world code patterns
   - Focus on language-specific features and idiomatic patterns
   - Constructs should progressively increase in complexity:
     - Construct 1: Basic (simple functions, basic types)
     - Construct 2: Simple (classes/structs, basic OOP)
     - Construct 3: Medium (interfaces, inheritance, advanced features)
     - Construct 4: Complex (advanced language features, frameworks)
     - Construct 5: Very Complex (edge cases, meta-programming, extreme patterns)

2. **Implementation Phase**
   - Create source files (.py, .ts, .go, etc.) for each construct
   - Generate expected output files with new naming convention based on CLI parameters:
     - `default.txt` - Default aid output (public only, no implementation)
     - `implementation=1.txt` - With implementation bodies
     - `private=1,protected=1,internal=1,implementation=0.txt` - All visibility, no implementation
     - `imports=0.txt` - Without import statements
     - `comments=1.txt` - With comments included
     - File names reflect the non-default CLI parameters used (without '--' prefix)

3. **Testing Phase** 
   - Implement comprehensive unit tests (5 constructs × 3 options = 15 test cases per language)
   - Tests must validate actual parsing results, not mock data
   - Include construct-specific validations for language features

4. **CRITICAL: Gemini Output Review Phase**
   - **Generate actual distillation outputs** using AI Distiller
   - **Send outputs to Gemini for accuracy verification**
   - Gemini reviews against 4 criteria:
     a. **Correctness Against Specification** - Accuracy, completeness, signature integrity
     b. **Idiomatic Representation** - Natural language feel, clean formatting
     c. **Edge Case Robustness** - Complex syntax handling, feature interpretation
     d. **Contextual Integrity** - Import handling, dependency preservation
   - Fix any issues identified by Gemini
   - Iterate until Gemini confirms outputs are correct

5. **Completion and Progression**
   - Mark language as completed only after Gemini approval
   - Commit comprehensive implementation
   - Move to next language systematically

### Language Priority Order

Based on project support and complexity:
1. ✅ **Python** - Completed (5 constructs, 15 tests, Gemini review pending)
2. ✅ **TypeScript** - Completed (6 constructs, 18 tests, Gemini review pending)
3. 🔄 **Go** - In progress (constructs designed by Gemini)
4. **Rust** - Planned
5. **Swift** - Planned
6. **Ruby** - Planned
7. **Java** - Planned
8. **C#** - Planned
9. **Kotlin** - Planned
10. **C++** - Planned
11. **JavaScript** - Planned
12. **PHP** - Planned

### Current Status & Next Actions

**IMMEDIATE PRIORITY:** Before continuing with Go, must complete Gemini review of Python and TypeScript actual outputs as specified in original instructions.

### Key Testing Principles

- **No mocks or simulated data** - All tests use real AI Distiller output
- **Gemini collaboration is mandatory** - Both for design and output verification
- **Systematic progression** - Complete each language fully before moving to next
- **Real-world relevance** - Constructs represent actual development patterns
- **Language-specific focus** - Test unique features of each language

### Communication Protocol

- **User communication:** Czech language for general discussion
- **Technical documentation:** English for all code, comments, technical specs  
- **Gemini communication:** English only, use 'pro' model for deep analysis
- **o3 communication:** English only, use 'o3' model (not 'o3-mini') when needed

This comprehensive testing ensures AI Distiller can accurately handle real-world codebases across all supported languages with verified accuracy.