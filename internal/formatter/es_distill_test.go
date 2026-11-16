package formatter

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/javascript"
	"github.com/janreges/ai-distiller/internal/language/typescript"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// distillES runs the real TypeScript or JavaScript processor (by the
// extension of name), with the stripper for opts, and the text formatter
// with the import filter on, as `aid --format text` does, and returns the
// lines between the <file> tags.
func distillES(t *testing.T, name, src string, opts processor.ProcessOptions) []string {
	t.Helper()
	var file *ir.DistilledFile
	var err error
	if ext := filepath.Ext(name); ext == ".js" || ext == ".jsx" {
		file, err = javascript.NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), name, opts)
	} else {
		file, err = typescript.NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), name, opts)
	}
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, NewLanguageAwareTextFormatter(Options{ProcessingOptions: ProcessingInfo{FilterImports: true}}).Format(&b, file))
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	return lines[1 : len(lines)-1]
}

// esDefault are the default flags: public declarations with their docs, no
// bodies, no other comments.
var esDefault = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
}

// esAll adds the private, protected and internal declarations.
var esAll = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
	IncludePrivate: true, IncludeProtected: true,
}

// esImpl adds the bodies.
var esImpl = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
	IncludeImplementation: true,
}

// The import filter keeps an import whose name the printed text uses: a name
// with `$` in it, a use on any line outside the imports (also below an
// import-looking line in a namespace, a `declare module` or a body), and a
// use in the entity of an import alias.
func TestESImportFilterKeepsUsedImports(t *testing.T) {
	cases := []struct {
		name, file, src string
		opts            processor.ProcessOptions
		kept, removed   []string
	}{
		{
			name: "names with $",
			file: "dollar.ts",
			src: "import type { $ZodType } from './schemas';\nimport { obs$ } from './rx';\nimport { obs } from './plain';\n" +
				"export function f(x: $ZodType): typeof obs$ { return obs$; }\n",
			opts:    esDefault,
			kept:    []string{"import type { $ZodType } from './schemas'", "import { obs$ } from './rx'"},
			removed: []string{"import { obs } from './plain'"},
		},
		{
			name:    "a lone $ in JavaScript",
			file:    "dollar.js",
			src:     "import $ from 'jquery';\nexport const el = $('#x');\n",
			opts:    esDefault,
			kept:    []string{"import $ from 'jquery'"},
			removed: nil,
		},
		{
			name: "an import alias in a namespace below the use",
			file: "f1.ts",
			src: "import { Alpha } from './alpha';\nimport { Beta } from './beta';\nimport { Gone } from './gone';\n\n" +
				"export function useAlpha(a: Alpha): Beta {\n  return null as any;\n}\n\n" +
				"export namespace Shapes {\n  import Polygon = Geometry.Polygon;\n  export const area = (p: Polygon): number => 0;\n}\n",
			opts:    esDefault,
			kept:    []string{"import { Alpha } from './alpha'", "import { Beta } from './beta'", "    import Polygon = Geometry.Polygon"},
			removed: []string{"import { Gone } from './gone'"},
		},
		{
			name: "an import in a declare module below the use",
			file: "f2.ts",
			src: "import { Alpha } from './alpha';\nimport Beta from './beta';\n\n" +
				"export function useAlpha(a: Alpha): Beta {\n  return null as any;\n}\n\n" +
				"declare module 'ext' {\n  import { Gamma } from './gamma';\n  export function g(x: Gamma): void;\n}\n",
			opts: esDefault,
			kept: []string{"import { Alpha } from './alpha'", "import Beta from './beta'", "    import { Gamma } from './gamma'"},
		},
		{
			name:    "a use between two imports",
			file:    "between.ts",
			src:     "import { execa } from 'execa';\nconst $ = execa({});\nimport * as gen from './gen';\n\nexport function run(): void {}\n",
			opts:    esAll,
			kept:    []string{"import { execa } from 'execa'"},
			removed: []string{"import * as gen from './gen'"},
		},
		{
			name: "a require in a printed body below the use",
			file: "f3.ts",
			src: "import { Alpha } from './alpha';\n\nexport function useAlpha(a: Alpha): void {}\n\n" +
				"export class Loader {\n  load(): void {\n    const fs = require('fs');\n    fs.readFileSync('x');\n  }\n}\n",
			opts: esImpl,
			kept: []string{"import { Alpha } from './alpha'", "        const fs = require('fs');"},
		},
		{
			name: "a require in a printed JavaScript body below the use",
			file: "f6.js",
			src: "import { Alpha } from './alpha';\n\n/** @param {Alpha} a */\nexport function useAlpha(a) {}\n\n" +
				"export class Loader {\n  load() {\n    const fs = require('fs');\n    fs.readFileSync('x');\n  }\n}\n",
			opts: esImpl,
			kept: []string{"import { Alpha } from './alpha'"},
		},
		{
			name: "the namespace an import alias names",
			file: "f4.ts",
			src: "import * as Geometry from './geometry';\nimport * as Unused from './unused';\nimport Polygon = Geometry.Polygon;\n\n" +
				"export function area(p: Polygon): number {\n  return 0;\n}\n",
			opts:    esDefault,
			kept:    []string{"import * as Geometry from './geometry'", "import Polygon = Geometry.Polygon"},
			removed: []string{"import * as Unused from './unused'"},
		},
		{
			// The alias goes, so nothing uses its namespace either.
			name:    "the namespace of an unused alias",
			file:    "f4b.ts",
			src:     "import * as Geometry from './geometry';\nimport Polygon = Geometry.Polygon;\n\nexport function area(): number {\n  return 0;\n}\n",
			opts:    esDefault,
			removed: []string{"import * as Geometry from './geometry'", "import Polygon = Geometry.Polygon"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := distillES(t, tc.file, tc.src, tc.opts)
			for _, want := range tc.kept {
				assert.Contains(t, lines, want)
			}
			for _, gone := range tc.removed {
				assert.NotContains(t, lines, gone)
			}
		})
	}
}

// `export import fs = require('fs')` prints as written, with module fs.
func TestESExportImportRequire(t *testing.T) {
	src := "export import fs = require('fs');\nimport path = require('path');\nexport function f(p: typeof path): void {}\n"
	assert.Equal(t, []string{"export import fs = require('fs')", "import path = require('path')", "export function f(p: typeof path): void"},
		distillES(t, "req.ts", src, esDefault))
}

// A JSDoc `{` that never closes leaves its own tag untyped, not the tags
// after it.
func TestESJSDocUnclosedBrace(t *testing.T) {
	src := "/**\n * @param {Object a - broken\n * @param {string} b\n * @returns {number}\n */\nexport function f(a, b) {}\n"
	lines := distillES(t, "jsdoc_open.js", src, esDefault)
	assert.Equal(t, "export function f(a, b: string) -> number", lines[len(lines)-1])
}

// `export =` and `export default` leave out a value over the cap that holds
// no function.
func TestESExportValueCap(t *testing.T) {
	long := "{\n  alpha: 'aaaaaaaaaaaaaaaaaaaa',\n  beta: 'bbbbbbbbbbbbbbbbbbbbbbbbbb',\n" +
		"  gamma: 'cccccccccccccccccccccccccc',\n  delta: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],\n};\n"
	for _, stmt := range []string{"export = ", "export default "} {
		assert.Equal(t, []string{"", "export interface Keep {", "}"},
			distillES(t, "expeq.ts", "export interface Keep {}\n"+stmt+long, esDefault), stmt)
	}
	assert.Equal(t, []string{"", "export interface Keep {", "}", "export = { a: 1 }"},
		distillES(t, "expeq.ts", "export interface Keep {}\nexport = { a: 1 };\n", esDefault))
}

// --expand NAME keeps the body of a getter, setter or generator called NAME.
func TestESExpandMatchesAccessorsAndGenerators(t *testing.T) {
	expand := func(patterns ...string) processor.ProcessOptions {
		opts := esDefault
		opts.ExpandSymbols = patterns
		return opts
	}
	ts := "export class Box {\n  get size(): number { return 1; }\n  set size(v: number) { this.v = v; }\n" +
		"  *items(): Generator<number> { yield 1; }\n  load(): void { console.log(1); }\n}\n" +
		"export function* gen(): Generator<number> { yield 2; }\n"
	assert.Equal(t, []string{"", "export class Box {",
		"    public get size(): number { return 1; }",
		"    public set size(v: number) { this.v = v; }",
		"    public *items(): Generator<number> { yield 1; }",
		"    public load(): void",
		"}",
		"export function* gen(): Generator<number> { yield 2; }"},
		distillES(t, "acc.ts", ts, expand("size", "items", "gen")))

	js := "export class Box {\n  get size() { return 1; }\n  *items() { yield 1; }\n}\nexport function* gen() { yield 2; }\n"
	assert.Equal(t, []string{"", "export class Box:",
		"    get size() { return 1; }",
		"    *items() { yield 1; }",
		"export function* gen() { yield 2; }"},
		distillES(t, "acc.js", js, expand("size", "items", "gen")))
}
