# Text format conventions per language

These are the conventions the twelve text formatters follow today, recorded
from aid's own output. They are not a specification. The languages disagree
(a visibility keyword, a prefix or nothing; braces or indentation;
`class X:` in JavaScript), and whether to unify them is an open design
question. Changing a convention changes every snapshot of the language that
shows it, so it is a decision, not a cleanup.

## How the rows were produced

Each row comes from the language's conformance fixture in
`internal/conformance/testdata/<lang>/`, run three times: with default flags,
with everything included, and without the import filter:

```bash
export CGO_ENABLED=1 CGO_CFLAGS="-O2 -w"
tmp=$(mktemp -d) && go build -o "$tmp/aid" ./cmd/aid
for f in internal/conformance/testdata/*/*; do
  echo "== $f"
  "$tmp/aid" "$f" --stdout --summary-type=off </dev/null
  "$tmp/aid" "$f" --stdout --summary-type=off --private=1 --protected=1 --internal=1 --comments=1 --implementation=1 </dev/null
  "$tmp/aid" "$f" --stdout --summary-type=off --filter-imports=false </dev/null
done
```

The examples in the tables are lines of that output. Where a fixture does not
show a case, the cell names the formatter function it was read from. Members
are indented four spaces per level in every language but Go, which indents
with one tab per level as gofmt does (`internal/formatter/go_formatter.go#goIndent`)
and prints bodies with their source tabs; a printed body keeps its own
relative indentation under the signature.

## Declarations

| Language | Visibility | Function, no body (default) | Body (`--implementation=1`) |
|---|---|---|---|
| cpp | nothing: the private `confHiddenOut()` prints like a public member | `int confInnerOpen()` in a class; `ConfWidget::confOut();` out of class | braces |
| csharp | keyword: `public`, `private`, `protected internal string ConfProtInt()` | `public string ConfPublic();` | braces |
| go | nothing; the name's case is the visibility | `func (w *ConfWidget) ConfPublic() string` | braces |
| java | keyword: `public`, `private`; package-private prints nothing (`internal/formatter/java_formatter.go#JavaFormatter.getJavaVisibilityKeyword`) | `public String confPublic();` | braces |
| javascript | nothing; `#confPrivate()` keeps the `#` of its name; top-level declarations print `export` or `export default` as written (`export class ConfWidget:`, `export async function* confGen(n)`) | `confPublic()` | braces |
| kotlin | keyword, `public` left out: `private fun confPrivate()`, `fun confPublic()` | `fun confPublic(): String` | braces |
| php | keyword on members: `public int $confField;`, `private confPrivate(): string`; none on top-level functions (`internal/formatter/php_formatter.go#PHPFormatter.formatFunction`) | `public confPublic(): string` (no `function`) | braces |
| python | prefix `-` on private functions and fields: `-def _conf_private(self):` | `conf_public(self)`, no `def` and no colon, its docstring indented below | `def conf_public(self):`, then the indented body, no delimiter |
| ruby | a `# private` line above the method and above its doc (`internal/formatter/ruby_formatter.go#RubyFormatter.getRubyVisibilityComment`) | `def conf_public` | the indented body, no `end` |
| rust | `pub`; private prints nothing: `fn conf_private`; `pub(crate)` and `pub(super)` for internal and protected (`internal/formatter/rust_formatter.go#RustFormatter.getRustVisibilityKeyword`) | `pub fn conf_public(&self) -> &'static str`; in a trait `fn conf_area(&self) -> i32;` | braces |
| swift | keyword: `public func confPublic()`, `private func confPrivate()` | `public func confPublic() -> String` | braces |
| typescript | keyword on class members, `public` included: `public confPublic()`, `private #confSecret`; top-level declarations print `export` instead | `public confPublic(): string` | braces |

"Braces" means `{` on the signature line, the body one level deeper and `}` at
the signature's indentation (section 3 of `SKILL.md`).

## Comments, types, imports

| Language | Line comment (`--comments=1`) | Doc comment | Types | Imports (`--filter-imports=false`; source then printed) |
|---|---|---|---|---|
| cpp | `// CONF_LINE_COMMENT` | `/** CONF_DOC_COMMENT */` | `class ConfWidget {` ... `};`; out-of-class definitions follow at file level; a namespace's members print without the namespace line | `#include <string>` prints `#include "string"` |
| csharp | `// CONF_LINE_COMMENT` | `/// <summary>CONF_DOC_COMMENT</summary>` | `public class ConfWidget {` ... `}`; `namespace Conf;` | `using System.IO;` unchanged |
| go | `// CONF_LINE_COMMENT`; a struct field's or interface method's comment prints inside its type, above the field or at the end of its line (`ConfNote string // CONF_FIELD_TRAILING`); the comment at the end of a const, var or type line prints there too (`const ConfLimit = 10 // CONF_CONST_TRAILING`, `internal/formatter/go_formatter.go#GoFormatter.FormatNodes`); any other comment is set off by a blank line | `// CONF_DOC_COMMENT` directly on its declaration; one blank line between top-level declarations, methods included, above their docs (`internal/formatter/go_formatter.go#sitsOn`) | `type ConfWidget struct {` ... `}`; methods at file level with their receiver; an interface prints its embedded types one per line (`io.Reader`, a type set `~int \| ~int64` as one line), then its methods | `import "strings"` prints an `import (` block, one quoted path per line, then `)`; the comments between imports and directly above one print in the block, indented, the one at the end of an import's line on that line (`internal/formatter/go_formatter.go#GoFormatter.inImportBlock`); the declaration's doc prints directly above `import (` (`internal/formatter/go_formatter.go#importDoc`), any other comment before the block, such as a file header, is set off by a blank line; no block when the filter drops every import |
| java | `// CONF_LINE_COMMENT` | `/**`, ` * CONF_DOC_COMMENT`, ` */` on three lines | `public class ConfWidget {` ... `}`; `package com.example.conf`; a nested `private enum ConfColor {` with constants as `public static final ConfColor CONF_RED;` | `import java.io.File;` prints `import java.io.File` |
| javascript | `// CONF_LINE_COMMENT` | `/** CONF_DOC_COMMENT */` | `export class ConfWidget:`, then the members indented, no closing line | as written, the module in single quotes: `import ConfReact, { confHook as confUseHook } from "conf-react";` prints `import ConfReact, { confHook as confUseHook } from 'conf-react'` (`internal/formatter/es_module.go#esImportLine`); an export that declares nothing prints the same way: `export * from './conf-all'`, `export { confArrow }`, `export default ConfWidget` (`internal/formatter/es_module.go#esExportLine`) |
| kotlin | `// CONF_LINE_COMMENT` | `/** CONF_DOC_COMMENT */` | `class ConfWidget {` ... `}`; no `package` line | `import java.io.File` unchanged |
| php | `// CONF_LINE_COMMENT` | `/** x */` where a docblock prints (`internal/formatter/php_formatter.go#PHPFormatter.formatComment`) | `class ConfWidget {` ... `}`, a trait use as the member line `use ConfTrait;`; `namespace Conf;` | `use DateTimeImmutable;`, `use Conf\Base\ConfBase as ConfAliasBase;`, `use Conf\Contracts\{ConfShown as ConfAliasShown, ConfUnusedMember};`, `use function Conf\Helpers\conf_helper;` unchanged |
| python | `# CONF_LINE_COMMENT` | `"""CONF_DOC_COMMENT"""`, indented under the `class` or function line | `class ConfWidget:`, then the members indented; decorators above (`@dataclass`) | `import os`, `from dataclasses import dataclass` unchanged |
| ruby | `# CONF_LINE_COMMENT` | `# CONF_DOC_COMMENT` | `class ConfWidget`, then the members indented, no `end`; `include Comparable` as a member line | `require "json"` prints `require 'json'` |
| rust | `// CONF_LINE_COMMENT` | `/// CONF_DOC_COMMENT` (`//!` for an inner doc; outer attributes such as `#[derive(Debug)]` print above their item, field and variant; the blank line before an item goes above its comments, `internal/formatter/rust_formatter.go#docWriter`) | `pub struct ConfWidget {` fields `}`, hidden private fields as `/* private fields */` (on the header line when none is left); methods in `impl ConfWidget {` and `impl ConfShape for ConfWidget {` blocks, whose first member follows the `{` line without a blank line (`internal/formatter/rust_formatter.go#openBody`, also for traits, modules and extern blocks), an inherent impl without members not printed (`internal/formatter/rust_formatter.go#printsNothing`); `pub trait ConfShape {`; `pub enum ConfColor {`, one variant per line; a tuple struct on one line, a removed field as `_` (`pub struct Pair(pub i32, _);`); `pub mod inner {` its items `}`; `extern "C" {` its functions `}`; `macro_rules! m {`, one `($x:expr) => ...;` per rule, `}` | `use std::path::MAIN_SEPARATOR;` unchanged |
| swift | `//` for every comment node (`internal/formatter/swift_formatter.go#SwiftFormatter.formatComment`) | `// CONF_DOC_COMMENT` | `public class ConfWidget {` ... `}` | `import Foundation` unchanged |
| typescript | `// CONF_LINE_COMMENT` | `/** CONF_DOC_COMMENT */` | `export class ConfWidget {` ... `}`; `namespace ConfSpace {`, `declare module "conf-ambient" {`; `export enum ConfColor {`, one member per line without commas | as in JavaScript: `import { sep } from "path";` prints `import { sep } from 'path'`, `import type { ConfOnlyType } from "./conf-types";` keeps its `type`; exports as in JavaScript, `export type { ConfReType } from './conf-retype'` |

Where a fixture drops a declaration line (the C++ namespace line, the Kotlin
package line, a C# `using` alias), the reason is listed in
`internal/conformance/known_gaps_test.go` or in
`internal/conformance/unhandled_test.go#knownUnhandled`; read those lists
rather than this table for what is missing.

How an import line prints also decides how the import filter reads it: the
C++ quotes above make the filter keep those lines whatever the code uses
(`import-filter.md`).
