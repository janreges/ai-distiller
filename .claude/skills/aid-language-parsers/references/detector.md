# Stdin language detector

## When it runs

Only for stdin with `--lang auto` (the default):
`internal/cli/root.go#processStdinWithContext` passes the whole input to
`internal/language/detector.go#LanguageDetector.DetectFromReader`. Files and
directories never use it; they pick the processor by extension. An explicit
`--lang` goes through `internal/language/names.go#Canonical` instead, which maps
aliases (`py`, `ts`, `tsx`, `golang`, `c++`, `c#`, `rb`, `rs`, `kt`, ...) to
processor names.

## How it decides

`internal/language/detector.go#LanguageDetector.detectLanguage` looks at the
first 64 KB:

1. Shebang: a first line `#!...` containing `python`, `ruby`, `node`
   (JavaScript), `php` or `swift` decides at once.
2. `<?php` anywhere decides PHP at once.
3. Otherwise each language's pattern function adds a fixed score when it
   matches:

   | Language | Function | Score |
   |---|---|---|
   | rust | `hasRustPatterns` | 100 |
   | go | `hasGoPatterns` | 100 |
   | cpp | `hasCppPatterns` | 100 |
   | swift | `hasSwiftPatterns` | 100 |
   | csharp | `hasCSharpPatterns` | 100 |
   | kotlin | `hasKotlinPatterns` | 90 |
   | ruby | `hasRubyPatterns` | 90 |
   | python | `hasPythonPatterns` | 85 |
   | java | `hasJavaPatterns` | 80 |
   | typescript | `hasTypeScriptPatterns` | 75 |
   | javascript | `hasJavaScriptPatterns` | 70 |

   Each function except TypeScript's counts its regular expressions and
   matches when at least two of them hit. TypeScript matches on one
   TypeScript-only construct (`tsOnlyPatterns`: a type annotation, `interface`
   with typed members, `type X =`, `declare`, `import type`, `as const`, ...),
   on a typed class member that is not Swift's `init`, or on a keyword it
   shares with other languages (`interface`, `enum`, `type`, `implements`)
   together with JavaScript syntax (`jsContextPatterns`). A PHP-style typed
   parameter (`int $x`) rules TypeScript out.
4. The highest score wins; a tie goes to the alphabetically first name. Below
   70, detection fails with `could not detect language`, which the CLI turns
   into `could not auto-detect the language of stdin; pass --lang with one of:
   ...` followed by the registered names and aliases.

The scores order the languages: a TypeScript or JavaScript hit loses to any
other language that also matched, so TypeScript-looking syntax in Kotlin or
Swift code does not win.

TSX: when the detector returns `typescript` (or `--lang tsx` is given) and
`internal/language/detector.go#HasJSX` finds a JSX element, the CLI passes the
name `stdin.tsx` so the TypeScript processor uses its TSX grammar.

## What it returns

The detector returns processor names as registered (`python`, `csharp`, `cpp`,
...), never aliases. Two tests guard this:
`internal/language/detector_test.go#TestDetector_ConformanceFixtures` detects
each conformance fixture and checks that a processor exists for the result,
and `internal/language/names_test.go#TestCanonical_EveryAliasHasAProcessor`
checks every alias.

## Changing it

- Every conformance fixture has a `stdin-detect` check (the fixture piped in
  without `--lang`), so a detector change is tested for all languages by
  `go test -count=1 -run TestConformance ./internal/conformance`.
- Add a case to `internal/language/detector_test.go` for the input that was
  misdetected, and run `go test -count=1 ./internal/language/`.
- A new language needs its own `has<Lang>Patterns` function and a score in
  `detectLanguage`. Choose the score against the languages whose syntax it
  shares, and require at least two distinct markers, as the other functions
  do.
