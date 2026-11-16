# The import filter

`internal/importfilter` removes import lines that look unused. It works on the
formatted text of one file, with regular expressions: it does not parse the
code and never sees the IR. So whether an import survives depends on what the
text formatter printed, not on what the source uses. The one exception is
`internal/importfilter/filter.go#Source`: the file's path and whether its
source has JSX, which the formatter passes along.

## When it runs

- Only for `--format text`. `internal/cli/root.go#runDistiller` and
  `internal/cli/root.go#processStdinWithContext` set `FilterImports` to
  `--filter-imports && --format == text`. `md` wraps the text formatter but
  gets the same options, so it is never filtered; `jsonl`, `json-structured`
  and `xml` print the IR and keep every import the stripper kept.
- The prompt AI actions distil as `text` with the filter always on
  (`internal/cli/root.go#distillForAction`;
  `.claude/skills/aid-architecture/references/modes.md`).
- `internal/formatter/language_aware_text_formatter.go#LanguageAwareTextFormatter.formatWithFiltering`
  renders one file into a buffer and calls
  `internal/importfilter/filter.go#FilterImportsOf` with the file's language
  and its `Source` (the path, and `HasJSX` from the file's
  `Extensions.JavaScript` or `Extensions.TypeScript`).
  In a directory run each `<file>` block is filtered on its own. A language
  without a registered filter gets its text back unchanged; a filter error
  writes the unfiltered text.
- `--imports=0` removes imports in the stripper, before formatting; the
  filter then finds none.
- The snapshot suite runs `--format text` and never passes
  `--filter-imports=false`, so the expected files show filtered imports.

Turn it off to see what the formatter printed:

```bash
export CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"
go run ./cmd/aid internal/conformance/testdata/python/sample.py --stdout --summary-type=off </dev/null | head -3
go run ./cmd/aid internal/conformance/testdata/python/sample.py --stdout --summary-type=off --filter-imports=false </dev/null | head -3
go run ./cmd/aid internal/conformance/testdata/python/sample.py --stdout --summary-type=off --private=1 --implementation=1 </dev/null | head -3
```

The first prints `from dataclasses import dataclass` alone; the second and the
third also print `import os`. `os` is used only in the body of the private
`_conf_private`, so the import survives only when that body is printed.

The filter logs to stderr with the prefix `[<lang> import filter]`. Which
progress lines appear at `-v` differs per language; from `-vv` every filter
names each import it removes (Python: `Removing unused import: import os`),
and at `-vvv` the text formatter adds `[import filter] Removed unused import: ...`
lines.

## How it decides

Each filter finds the import lines of the file with its own regular
expressions, derives a name per import, and keeps the import when
`internal/importfilter/base_filter.go#BaseFilter.SearchForUsage` finds that
name in the text:

- only the lines after the last import line are searched;
- the name must match as a whole word (`\bname\b`);
- lines whose trimmed text starts with `#` or `//` are skipped, so a name in
  such a comment keeps nothing, while a name in a printed docstring or `/** */`
  doc does keep the import.

PHP searches on its own
(`internal/importfilter/php_filter.go#PHPFilter.FilterUnusedImports`): every
line that is not a use statement, so a file with several namespaces finds a
usage between two import blocks; it skips `//` and `#` comment lines but not
`#[...]` attributes, and also the `<file>` tags and `namespace` lines.

The removed lines are cut out of the text; nothing else changes, except in
Go: a removed import takes the `//` comment lines directly above it (the
comment at the end of its line goes with the line), and a block none of
whose imports is kept goes whole, `import (`, `)`, the comments in it, the
comment lines directly above it and the blank line above those
(`internal/importfilter/go_filter.go#GoFilter.removeEmptiedBlocks`).

## Languages

Filters register themselves with `internal/importfilter/filter.go#Register`
under the language name the processors use.

| Language | File | Name searched in the text | Always kept |
|---|---|---|---|
| python | `internal/importfilter/python_filter.go` | each imported name or its alias; for `import m` also `m` | `from m import *` |
| javascript, typescript | `internal/importfilter/javascript_filter.go` (one filter for both) | every name the import binds (`internal/importfilter/javascript_filter.go#jsLocalNames`): the default import's name, the namespace alias, each named import's alias or else its name; also for `import type`, `import x = require('m')` and the import alias `import G = ns.H`; an import that binds `React` counts as used in a file with JSX and in a `.jsx` or `.tsx` file, since the classic JSX transform calls `React.createElement` (`internal/importfilter/javascript_filter.go#JavaScriptFilter.filterSource`) | side-effect imports (`import 'x'`); `export` lines are never read as imports |
| go | `internal/importfilter/go_filter.go` | the alias, else the last path element (the one before a `/vN` suffix; a few standard paths are mapped) | blank imports and `"C"` |
| java | `internal/importfilter/java_filter.go` | the class name (last element); for `import static`, the member | wildcard imports |
| csharp (`c#`) | `internal/importfilter/csharp_filter.go` | the alias, else the last namespace element; for some `System.*` namespaces also typical type names (`internal/importfilter/csharp_filter.go#CSharpFilter.checkSystemNamespaceUsage`) | nothing |
| php | `internal/importfilter/php_filter.go` | the alias, else the last name element, of each name; a group is kept when one member is used. Only `use` lines at column 0 are imports (indented ones are trait uses in a class). Every other line is searched, `#[...]` attributes included; class and function names match case-insensitively | nothing |
| ruby | `internal/importfilter/ruby_filter.go` | the required file's base name in CamelCase (`json` gives `Json`), then the same in lower case | requires whose path contains `/` or `-` |
| cpp (`c++`) | `internal/importfilter/cpp_filter.go` | for `<header>`: a marker for five headers (`std::string` for `<string>`, ...), any `std::` for the others | every `"header"` include |
| kotlin, rust, swift | none | | everything |

## Why printing changes which imports survive

The filter sees only the formatter's text, so a formatter change can add or
remove imports without touching the filter:

- **Bodies and members.** A name used only in a body or in a hidden member
  keeps its import only when that body or member is printed (`import os`
  above). Every flag that changes what is printed changes the imports.
- **How a name is printed.** If a formatter stops printing a type (a return
  type, a parameter type, an annotation) or prints it under another name,
  the import that supplied it can disappear; the Ruby filter's `Json` does not
  match `JSON.generate`, so in the Ruby fixture `require 'json'` goes even with
  bodies printed.
- **How the import line is printed.** The filter classifies the printed line.
  C++'s `#include <string>` prints as `#include "string"`, which the C++
  filter takes for a user header and always keeps
  (`text-conventions.md`, column "Imports").

So after a formatter change that alters names or import lines, compare the
default output with `--filter-imports=false`, and read every changed import
line in the snapshot diff.
