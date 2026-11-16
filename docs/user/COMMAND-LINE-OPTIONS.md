# AI Distiller - Command Line Options Reference

AI Distiller transforms source code into optimized formats for Large Language Models. It compresses codebases by 60-90% while preserving all semantic information needed for AI analysis. The tool generates either compressed code representations or complete AI analysis prompts that can be directly copied to AI tools like Gemini 2.5 Pro (1M context), ChatGPT-o3/4o, or Claude for perfect AI-powered code analysis, refactoring, security audits, and architectural reviews.

## Output

| Flag | Default | Meaning |
|------|---------|---------|
| `<path>` | none | Source file or directory to distill. With no path and nothing piped in, `aid` prints the help; with no path and piped stdin it distills stdin. `-` reads stdin explicitly (and waits for it). Pass `.git` for git history mode. A path argument takes precedence over piped stdin. |
| `-o, --output FILE` | none | Output file (default: `.aid/aid.<name>[.<options>].<ext>` in the project root); AI actions always write to `--ai-output` and reject `-o` |
| `--stdout` | `false` | Print to stdout; no file is written unless `-o` is also given; AI actions always write to `--ai-output` and reject `-o` |
| `--format FORMAT` | `text` | Output format: `text`, `md`, `jsonl`, `json-structured`, `xml` |
| `--file-path-type TYPE` | `relative` | How paths appear in output: `relative` or `absolute` |
| `--relative-path-prefix STR` | none | Custom prefix for relative paths in output (for example `src/`) |
| `--show-ai-agent-instructions` | `false` | Put a short block of instructions for AI agents at the start of the text output |
| `--filter-imports` | `true` | Drop imports the distilled output does not use (text format only); disable with `--filter-imports=false` |

### Where the output goes

| Invocation | File written | Printed to stdout |
|---|---|---|
| `aid PATH` | `<project root>/.aid/aid.<name>[.<options>].<ext>` | nothing |
| `aid PATH -o FILE` | `FILE` | nothing |
| `aid PATH --stdout` | none | output |
| `aid PATH -o FILE --stdout` | `FILE` | output |
| `aid -`, or no path and piped stdin | none | output |
| `aid - -o FILE`, or no path, piped stdin and `-o FILE` | `FILE` | output |
| `aid .git` (git mode) | none | git log |
| `aid .git -o FILE` | `FILE` | nothing (`-o FILE --stdout`: both) |
| `aid PATH --ai-action ACTION` | the `--ai-output` path (default: under `.aid/` in the project root for a prompt action, under `./.aid/` in the current directory for a flow action, see below) | status lines; with `--stdout` a prompt action prints its prompt instead |

AI actions always write to `--ai-output` and reject `-o`: `aid PATH --ai-action ACTION -o FILE` exits 1. `--stdout` does not stop an AI action from writing its file.

## Visibility

Each flag takes `0` or `1`. Setting a flag to `0` removes members of that level, including `--public=0`, which removes public members.

| Flag | Default | Meaning |
|------|---------|---------|
| `--public 0\|1` | `1` | Include public members (exported functions, public classes) |
| `--protected 0\|1` | `0` | Include protected members, as the language defines them (the `protected` keyword in Java, C#, C++, Kotlin, PHP, TypeScript and similar) |
| `--internal 0\|1` | `0` | Include internal/package-private members (unexported lowercase Go symbols, package-private Java) |
| `--private 0\|1` | `0` | Include private members (`private` keyword, Python `_name` and `__name` members, `#private` JS) |
| `--include-only CATEGORIES` | none | Include ONLY these categories (comma-separated) |
| `--exclude-items CATEGORIES` | none | Exclude these categories (comma-separated), for example `--exclude-items=public` |

C# combined access modifiers show when the flag of either of their words is on: `protected internal` with `--protected=1` or `--internal=1`, `private protected` with `--private=1` or `--protected=1`.

Valid categories for `--include-only` and `--exclude-items`: `public`, `protected`, `internal`, `private`, `comments`, `docstrings`, `implementation`, `imports`, `annotations`, `fields`, `methods`.

`--include-only` starts from nothing: list `methods` and/or `fields`, or every class and interface is printed with an empty body. `--exclude-items` starts from everything (all visibility levels, comments, docstrings, bodies, imports) and removes the listed categories, so `--exclude-items=internal` prints private and protected members and bodies too. In `--exclude-items`, `docstrings` is exactly `--docstrings=0`: it removes documentation comments (Python docstrings, Javadoc, JSDoc, XML docs and so on) and leaves regular comments to the `comments` category.

## Content

| Flag | Default | Meaning |
|------|---------|---------|
| `--comments 0\|1` | `0` | Include regular comments (`//`, `#`, `/* */`) |
| `--docstrings 0\|1` | `1` | Include documentation comments (JSDoc, Python docstrings, Go doc comments) |
| `--implementation 0\|1` | `0` | Include function/method bodies |
| `--imports 0\|1` | `1` | Include import/require/using statements |
| `--annotations 0\|1` | `1` | Include decorators/annotations (`@property`, `@Override`, `[Serializable]`) |
| `--fields 0\|1` | `1` | Include class fields and properties |
| `--methods 0\|1` | `1` | Include methods and functions |
| `--expand PATTERNS` | none | Keep the full body only for symbols whose name matches these globs (comma-separated or repeated, for example `GetUser,*Service*`); every other symbol stays a signature. Matching a type name expands the methods nested inside it. |

## File selection

| Flag | Default | Meaning |
|------|---------|---------|
| `--include PATTERNS` | none | Only process files matching these patterns (comma-separated or repeated, for example `"*.go,*.py"`) |
| `--exclude PATTERNS` | none | Skip files matching these patterns (for example `"*test*,*.json"`) |
| `-r, --recursive 0\|1` | `1` | Process subdirectories |

### Pattern rules

`--include` and `--exclude` take glob patterns. They apply to the files found under a directory path; a single file named on the command line is always processed. Paths are matched with `/` separators.

- `*` matches within one path segment, `?` one character, `[abc]` one of the listed characters. `**` matches any number of directories, and `**/` also none. In a pattern that contains `**`, `[` and `]` are plain characters.
- A pattern without `/`:
  - `--exclude` matches it against every directory and file name below the path you pass: `--exclude "*test*"` drops `tests/` and `foo_test.go`, `--exclude migrations` drops a `migrations/` directory.
  - `--include` matches it against the file name only: `--include "*.go"` works, `--include src` selects nothing (write `src/`).
  - The name of the path you pass and of the directories above it never match, so `aid . --exclude "*test*"` run in `.../my-tests/app` keeps every file.
- A pattern with `/` is matched against the end of the file's full path, starting at a directory boundary: `src/a.py`, `pkg/**/*.go` and `src/` (everything under `src/`) work from any directory. `**/name` matches `name` at any depth. A pattern that starts with `/` must match the whole absolute path. Because the full path is used, such a pattern can also name the directory you pass or one above it: `aid app --exclude "app/*"` drops every file.
- A pattern with `/` that matches a directory below the path you pass covers every file in it, for `--include` and `--exclude` alike: `--exclude "**/migrations"`, `--exclude "src/**/gen"` and `--exclude "third_party/*"` drop the whole directory.
- A pattern that starts with `./` matches nothing (`./src/**`); write `src/**`.
- Before any pattern, the walk skips default-ignored directories (`node_modules`, `vendor`, `build`, `dist`, `target`, virtual environments, IDE folders and similar) and what `.aidignore` lists; `--include` cannot bring them back, a `!pattern` line in `.aidignore` can.
- The AI flow actions (`flow-for-deep-file-to-file-analysis`, `flow-for-multi-file-docs`) use the same matcher on paths relative to the path you pass, so there a pattern with `/` cannot name that directory or one above it. They also always skip `docs/`, `examples/`, `testdata/`, `test-data/`, `assets/`, `static/`, `node_modules/`, `vendor/`, build output, hidden directories and files, and files whose name contains `generated`.

## Modes

| Flag | Default | Meaning |
|------|---------|---------|
| `--lang LANGUAGE` | `auto` | Language of code read from stdin, which has no filename to detect it from. Ignored for files and directories, where the language comes from the file extension. |
| `--raw` | `false` | Process all text files without parsing; overrides the content filters and keeps full file content |
| `-w, --workers NUM` | `0` | Parallel workers (`0` = auto, 80% of CPU cores; `1` = serial) |
| `--git-limit NUM` | `200` | Git mode (path `.git`): number of commits to show (`0` = all) |
| `--with-analysis-prompt` | `false` | Git mode: prepend a comprehensive AI analysis prompt to the git output |

Supported languages for `--lang`: `auto`, `python`, `typescript`, `javascript`, `go`, `ruby`, `swift`, `rust`, `java`, `csharp`, `kotlin`, `cpp`, `php`. These aliases are accepted too: `py` (python), `js` and `jsx` (javascript), `ts` and `tsx` (typescript), `golang` (go), `c++`, `cxx` and `cc` (cpp), `c#` and `cs` (csharp), `rb` (ruby), `rs` (rust), `kt` (kotlin).

`--lang` is read only for stdin. It does not force a language on a file or directory: a file with an unknown extension still fails with "no processor found".

Without `--lang`, stdin needs more than one marker of a language to be detected; a lone typed function such as `def add(a: int) -> int:` is not enough. When detection fails, `aid` exits 1 and lists the names and aliases `--lang` accepts; empty stdin fails with "no input on stdin". TypeScript on stdin is parsed as TSX (JSX allowed) with `--lang tsx`, or when the detected TypeScript contains JSX elements.

Git mode is activated automatically when `<path>` is `.git`. Output format:

```
[commit_hash] YYYY-MM-DD HH:MM:SS | author_name | commit_subject
    commit_body_line_1
    commit_body_line_2
    (properly indented)
```

## AI actions

| Flag | Default | Meaning |
|------|---------|---------|
| `--ai-action ACTION` | none | Use a predefined AI action (see below) |
| `--ai-output FILE` | action-specific | Custom output path for the AI action; supports the template variables listed at the end of this document. It must stay inside the path you pass or the project's `.aid/` directory, otherwise `aid` exits 1 with "output path must be within project directory". An absolute path is checked as given; a relative path is checked as if it started in the path you pass, but it is written relative to the current directory. |

### Available AI actions

| Action | What it produces |
|--------|------------------|
| `prompt-for-refactoring-suggestion` | Refactoring analysis prompt: context awareness, effort sizing, validation steps |
| `prompt-for-complex-codebase-analysis` | Enterprise-grade codebase analysis prompt with architecture diagrams and evidence-based findings |
| `prompt-for-security-analysis` | Security audit prompt with OWASP Top 10 focus and evidence checklists |
| `prompt-for-performance-analysis` | Performance optimization analysis prompt with profiling guidance |
| `prompt-for-best-practices-analysis` | Best practices and code quality analysis prompt |
| `prompt-for-bug-hunting` | Systematic bug hunting prompt |
| `prompt-for-single-file-docs` | Single-file documentation prompt |
| `prompt-for-diagrams` | Prompt that asks for 10 beneficial Mermaid diagrams |
| `flow-for-deep-file-to-file-analysis` | Structured task list for file-by-file analysis; creates the `.aid/` analysis directory and templates |
| `flow-for-multi-file-docs` | Structured documentation workflow for multiple files |

### Using --raw with AI actions

Adding `--raw` flag to AI actions includes full source code bodies in the generated prompts for comprehensive analysis.

**Context Size Considerations:**
- **Large codebases**: Analyze specific parts/folders that fit in AI context, or use default filtering (public APIs only, no implementation/comments) which may be insufficient for some analysis types but fits in smaller contexts
- **Small codebases**: Use `--raw` for full source code analysis  
- **Recommended**: Gemini 2.5 Pro with 1M context window for largest codebase capacity

**Examples:**
```bash
# Full source analysis for small modules
aid ./small-module --ai-action prompt-for-security-analysis --raw

# Comprehensive refactoring analysis with full code
aid ./src --ai-action prompt-for-refactoring-suggestion --raw
```

## Summary

| Flag | Default | Meaning |
|------|---------|---------|
| `--summary-type TYPE` | `visual-progress-bar` | Summary printed after processing: `visual-progress-bar`, `stock-ticker`, `speedometer-dashboard`, `minimalist-sparkline`, `ci-friendly`, `json`, `off` |
| `--no-emoji` | `false` | Disable emojis in the summary |

## Diagnostics and help

| Flag | Default | Meaning |
|------|---------|---------|
| `-v, --verbose` | `false` | Verbose output; repeat for more detail (`-vv`, `-vvv`) |
| `--version` | `false` | Show version information and exit |
| `--help` | `false` | Show the short help |
| `--help-extended` | `false` | Show the complete reference (man page style) |
| `--cheat` | `false` | Show the quick reference card |

Verbosity levels: `-v` shows basic info, file counts and phase transitions; `-vv` adds per-file processing and timing; `-vvv` adds full trace with data dumps and IR structures.

Topical help commands: `aid help actions` (AI actions with examples), `aid help filtering` (filtering reference), `aid help git` (git mode).

## Experimental

These flags may change without notice.

| Flag | Default | Meaning |
|------|---------|---------|
| `--dependency-aware` | `false` | EXPERIMENTAL: after the input, distil the project files it imports, each whole and with the same flags (regex-based). Paths are relative to the input. A directory adds the files outside it that its files import. Never follows a file outside the repository (the nearest directory holding `.git`, symlinks resolved; else the project root). When nothing can be followed, a one-line note goes to stderr |
| `--max-depth NUM` | `2` | EXPERIMENTAL: levels of imports `--dependency-aware` follows (1: what the input imports directly); must be 1 or more |

## Output File Naming & Management

AI Distiller uses intelligent file naming strategies to avoid conflicts and enable easy project management.

### Why Automatic File Naming?

**Problem:** Large codebases can generate megabytes of text output that exceed AI tool context limits.  
**Solution:** Automatic file generation with recognizable patterns instead of requiring manual file specification.

**Benefits:**
- `.aid` prefix ensures easy recognition in `git status`
- Can be easily added to `.gitignore` if desired
- Avoids requiring users to specify output files for every operation
- Prevents accidental context overflow in AI tools

### Default Output File Patterns

#### Regular Distillation Files
Pattern: `.aid/aid.<name>[.<options>].<ext>` in the project root

**Examples:**
- `.aid/aid.myproject.txt` (default options)
- `.aid/aid.a.py.impl.txt` (`aid a.py --implementation=1`)
- `.aid/aid.myproject.md` (`--format md`)

#### AI Action Output Files
Pattern: `.aid/<ACTION-NAME>.YYYY-MM-DD.HH-MM-SS.<basename>.md` in the project root

| Action | `<ACTION-NAME>` |
|--------|-----------------|
| `prompt-for-refactoring-suggestion` | `REFACTORING-ANALYSIS` |
| `prompt-for-complex-codebase-analysis` | `COMPLEX-CODEBASE-ANALYSIS` |
| `prompt-for-security-analysis` | `SECURITY-AUDIT` |
| `prompt-for-performance-analysis` | `PERFORMANCE-ANALYSIS` |
| `prompt-for-best-practices-analysis` | `BEST-PRACTICES-ANALYSIS` |
| `prompt-for-bug-hunting` | `BUG-HUNTING-ANALYSIS` |
| `prompt-for-single-file-docs` | `SINGLE-FILE-DOCS` |
| `prompt-for-diagrams` | `DIAGRAMS-ANALYSIS` |

**Examples:**
- `.aid/REFACTORING-ANALYSIS.2025-06-17.14-30-00.myproject.md`
- `.aid/SECURITY-AUDIT.2025-06-17.14-30-00.myproject.md`

#### Flow Action Output (Multiple Files)
Pattern: files and subdirectories under `./.aid/` in the current directory (not the project root), named after the basename of the path and the date

**Example:** `aid src --ai-action flow-for-deep-file-to-file-analysis` creates:
- `.aid/ANALYSIS-TASK-LIST.src.2025-06-17.md` - Sequential task list
- `.aid/ANALYSIS-SUMMARY.src.2025-06-17.md` - Analysis template
- `.aid/analysis.src/2025-06-17/` - Directory for individual file analysis reports

`flow-for-multi-file-docs` creates `.aid/DOCS-TASK-LIST.src.<date>.md`, `.aid/DOCS-INDEX.src.<date>.md` and `.aid/docs.src/<date>/` with `API-REFERENCE.md`, `README.md` and one template per source file under `files/`.

### Using --stdout

`--stdout` prints the output and writes no file unless `-o FILE` is also given (see the table under Output); AI actions always write to `--ai-output` and reject `-o`. Use it carefully:

**Safe for small projects:**
```bash
aid ./small-module --stdout | pbcopy  # Copy to clipboard
echo "code" | aid --lang python --stdout  # Stdin processing
```

**Risky for large codebases:**
```bash
aid ./entire-project --stdout  # May generate MBs of text exceeding AI context
```

**Flow actions behavior:**
- Print status lines to stdout, with or without `--stdout`
- Full directory structure still created in `./.aid/`

### Git Integration

Add AI Distiller outputs to your `.gitignore` if desired:

```gitignore
# AI Distiller outputs
.aid/
```

**Or commit them** for team collaboration and CI/CD integration:
- Share AI analysis results with team members
- Version control refactoring recommendations
- Track security analysis over time

### Template Variables (for --ai-output)
- `%YYYY-MM-DD%` - Current date
- `%HH-MM-SS%` - Current time
- `%folder-basename%` - Directory name

## Common Usage Patterns

### Basic Distillation
```bash
aid ./src                           # Public APIs only (no implementation/comments)
aid ./src --implementation=1        # Include function bodies
aid ./ --private=1 --protected=1    # Include all visibility levels
```

### AI-Powered Analysis
```bash
aid ./src --ai-action prompt-for-refactoring-suggestion
aid ./ --ai-action prompt-for-security-analysis --private=1
aid ./core --ai-action prompt-for-performance-analysis
```

### Output Control
```bash
aid ./src --format=md -o analysis.md
aid ./src --stdout | pbcopy          # Copy to clipboard (macOS)
aid ./src --format=jsonl --stdout > data.jsonl
```

### Filtering Examples
```bash
aid ./ --include "*.go,*.py"          # Only Go and Python files
aid ./ --exclude "*test*,*spec*"      # Exclude test files
aid ./ --include-only public,methods,fields,imports  # Public APIs with members, and imports
aid ./ --exclude-items comments,implementation
```

### Git Analysis
```bash
aid .git --git-limit=100             # Last 100 commits
aid .git --with-analysis-prompt      # With AI analysis guidance
```

### Advanced Usage
```bash
cat snippet.py | aid --lang python   # Language for stdin input
aid ./docs --raw                     # Process as plain text (full content, no filters)
aid ./large-project -w 1            # Single-threaded processing
```

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success. Source that does not parse does not fail the run; its output may be empty. The exception is Go: see `1`. |
| 1 | Any error: nonexistent path, a single Go file or Go stdin that does not parse (in a directory run such a file is skipped with a `Warning: failed to process` line on stderr and the run goes on), a file no language processor handles, unknown flag or more than one path, `--format` not in the list, a visibility or content flag (`--public`, `--protected`, `--internal`, `--private`, `--comments`, `--docstrings`, `--implementation`, `--imports`, `--annotations`) that is not `0`/`1` (or `true`/`false`), non-numeric `-w`, `--include-only` together with `--exclude-items`, unknown `--ai-action`, `--ai-output` outside the project, `-o` with `--ai-action`, stdin whose language is not detected, an unknown `--lang` on stdin, empty stdin without `--lang` |

These invalid values do not fail; `aid` exits 0 with a fallback:

| Flag value | What `aid` does |
|---|---|
| `--fields`, `--methods` other than `0`/`1` (or `true`/`false`) | keeps the default (`1`) |
| `--recursive` other than `0` | recursive |
| `--summary-type` not in the list | `ci-friendly` when stderr is not a terminal or `CI` is set, otherwise `visual-progress-bar` |
| `--file-path-type` other than `relative` | absolute paths |
| unknown categories in `--include-only` / `--exclude-items` | ignored |
| `-w` below `0` | one worker |
| `--git-limit` `0` or below | all commits |
| `--lang` with a file or directory path | ignored (stdin only) |

## Legacy Options (Deprecated)

| Option | Type | Status | Replacement |
|--------|------|--------|-------------|
| `--strip <items>` | string | Deprecated | Use individual flags: `--private=0`, `--implementation=0`, etc. |

**Legacy `--strip` values:** `comments`, `imports`, `implementation`, `non-public`, `private`, `protected`

---

*The flag tables above are checked against the real flags by `TestCommandLineOptionsDocMatchesFlags` in `internal/cli`.*