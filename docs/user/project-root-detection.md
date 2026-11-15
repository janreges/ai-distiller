# Project Root Detection and Output Organization

AI Distiller uses an intelligent project root detection system to centralize all outputs in a consistent location, making it easy to manage and track generated files.

## Why Project Root Detection?

When working on projects, you often run commands from various subdirectories. AI Distiller ensures that distilled files and AI prompt files go to a single `.aid/` directory at your project root, regardless of where you run the `aid` command (the two flow actions are the exception, see Output Structure). This provides:

- **Consistent output location** - Always know where to find your files
- **Better organization** - All AI Distiller outputs in one place
- **Easy cleanup** - Simply delete the `.aid/` directory
- **IDE integration** - Browse outputs alongside your code
- **Version control friendly** - Add `.aid/` to `.gitignore`

## How It Works

AI Distiller searches upward from your current directory (not from the path you pass), looking for specific markers that indicate a project root. The first directory that holds any marker is the root, so a `.git` in a nearer directory wins over an `.aidrc` further up. The search checks at most 12 directories and stops at your home directory. Detection runs only when a file is written to `.aid/`; `--stdout` and `-o FILE` do not need it.

### Detection Priority

Within one directory, the markers are checked in this order:

1. **`.aidrc` file** (highest priority)
   - Create an empty `.aidrc` file to explicitly mark your project root
   - This is the recommended approach for clarity

2. **Language-specific markers**
   - `go.mod` - Go modules
   - `package.json` - Node.js projects
   - `Cargo.toml` - Rust projects
   - `pyproject.toml` - Modern Python projects
   - `setup.py` - Legacy Python projects
   - `pom.xml` - Java Maven projects
   - `build.gradle` - Java Gradle projects

3. **Version control**
   - `.git` directory - Git repositories

When no directory holds a marker:

4. **Environment variable** (fallback)
   - `AID_PROJECT_ROOT` - Use when markers aren't suitable
   - Useful for CI/CD environments or special cases
   - Ignored, with a warning, when it is not an existing directory

5. **Current directory** (final fallback)
   - Used when no markers are found
   - Shows a warning to encourage proper configuration

## Usage Examples

### Basic Usage

```bash
# Mark your project root (recommended)
cd /my/project
touch .aidrc

# Run from anywhere - outputs always go to project root
cd src/components/ui
aid button.tsx
# Output: /my/project/.aid/aid.button.tsx.txt

cd ../../tests
aid test_utils.py
# Output: /my/project/.aid/aid.test_utils.py.txt
```

### Working with Monorepos

In monorepos, you might want different `.aid/` directories for different sub-projects:

```bash
monorepo/
├── .git                    # Repository root
├── services/
│   ├── api/
│   │   ├── .aidrc         # Mark API service root
│   │   └── src/
│   └── worker/
│       ├── .aidrc         # Mark worker service root
│       └── src/
└── packages/
    └── shared/
        ├── .aidrc         # Mark shared package root
        └── src/
```

### Environment Variable Override

The `AID_PROJECT_ROOT` environment variable is useful as a fallback when automatic detection isn't suitable:

```bash
# CI/CD pipeline example
AID_PROJECT_ROOT=/build/workspace aid src/

# Temporary override for testing (only where no marker is found above the working directory)
AID_PROJECT_ROOT=/tmp/test-output aid src/
```

A marker found by the upward search always wins over `AID_PROJECT_ROOT`.

### Security Boundaries

AI Distiller won't search above your home directory for security reasons:

```bash
# If running from ~/projects/myapp/src
# Will search up to ~/projects/myapp and ~
# But won't search / or /home
```

## Output Structure

Outputs are organized under the `.aid/` directory:

```
project-root/
├── .aidrc                  # Project root marker
├── .gitignore             # Add ".aid/" here
├── src/
│   └── ...
└── .aid/                  # AI Distiller outputs
    ├── aid.main.go.txt    # Distilled file outputs
    ├── aid.utils.py.txt
    ├── REFACTORING-ANALYSIS.2025-06-18.10-15-00.src.md
    ├── SECURITY-AUDIT.2025-06-18.14-20-00.api.md
    ├── ANALYSIS-TASK-LIST.src.2025-06-18.md   # flow-for-deep-file-to-file-analysis
    ├── ANALYSIS-SUMMARY.src.2025-06-18.md
    ├── analysis.src/2025-06-18/                # Analysis workflow outputs
    ├── DOCS-TASK-LIST.src.2025-06-18.md       # flow-for-multi-file-docs
    ├── DOCS-INDEX.src.2025-06-18.md
    └── docs.src/2025-06-18/                    # Documentation generation outputs
```

The two flow actions (`flow-for-deep-file-to-file-analysis` and `flow-for-multi-file-docs`) write to `./.aid/` in the current directory, not in the project root; the tree above shows them as they appear when `aid` runs in the project root. The MCP server keeps no cache.

## Best Practices

1. **Always use `.aidrc`** for explicit project root marking
   ```bash
   touch .aidrc
   echo ".aid/" >> .gitignore
   ```

2. **For monorepos**, place `.aidrc` in each sub-project that needs separate outputs

3. **For CI/CD**, use `AID_PROJECT_ROOT` to ensure consistent output location:
   ```yaml
   env:
     AID_PROJECT_ROOT: ${{ github.workspace }}
   ```

4. **Check detection** with verbose mode (the log goes to stderr):
   ```bash
   aid -v . 2>&1 | grep "Output:"
   ```

## Troubleshooting

### No project root found

If you see a warning about no project root found:
```
WARN: No project root found. Using current directory. For consistent behavior, create an '.aidrc' file at your project root.
```

Solution: Create an `.aidrc` file in your project root:
```bash
cd /path/to/project
touch .aidrc
```

### Outputs going to unexpected location

Check where the output is written:
```bash
# Run with verbose output
aid -v .
# Look for the "INFO: Output:" line to see where files are being saved
```

## Migration from Old Behavior

Previously, AI Distiller wrote output files named `.aid.<name>.[options].txt` into the current directory. It now writes `.aid/aid.<name>[.<options>].<ext>` in the project root. If you have old `.aid.*.txt` files scattered throughout your project:

1. Create `.aidrc` at your project root
2. Regenerate the outputs you still need (`aid <path>` writes `.aid/aid.<name>[.<options>].<ext>`) and delete the old files:
   ```bash
   find . -name ".aid.*.txt" -not -path "./.aid/*" -delete
   ```
3. Update any scripts or workflows that expect outputs in the old location or under the old names

## Summary

The project root detection system ensures that:
- Distilled files and AI prompt files are centralized in `<project-root>/.aid/`
- You can run `aid` from any subdirectory
- Outputs are organized and easy to manage
- The system respects project boundaries in monorepos
- Security boundaries prevent traversal above home directory

For most users, simply creating an `.aidrc` file at the project root provides the best experience.