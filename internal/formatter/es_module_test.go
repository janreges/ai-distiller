package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestESImportLine(t *testing.T) {
	sym := func(name, alias string) ir.ImportedSymbol { return ir.ImportedSymbol{Name: name, Alias: alias} }
	for _, tc := range []struct {
		imp  ir.DistilledImport
		want string
	}{
		{ir.DistilledImport{ImportType: "import", Module: "./polyfill"}, "import './polyfill'"},
		{ir.DistilledImport{ImportType: "import", Module: "./d", Symbols: []ir.ImportedSymbol{sym("default", "D")}}, "import D from './d'"},
		{ir.DistilledImport{ImportType: "import", Module: "./x", Symbols: []ir.ImportedSymbol{sym("A", ""), sym("B", "C")}}, "import { A, B as C } from './x'"},
		{ir.DistilledImport{ImportType: "import", Module: "react", Symbols: []ir.ImportedSymbol{sym("default", "React"), sym("useState", "")}}, "import React, { useState } from 'react'"},
		{ir.DistilledImport{ImportType: "import", Module: "zod", Symbols: []ir.ImportedSymbol{sym("*", "z")}}, "import * as z from 'zod'"},
		{ir.DistilledImport{ImportType: "import", Module: "./h", Symbols: []ir.ImportedSymbol{sym("default", "H"), sym("*", "I")}}, "import H, * as I from './h'"},
		{ir.DistilledImport{ImportType: "import", Module: "./foo", IsType: true, Symbols: []ir.ImportedSymbol{sym("Foo", "")}}, "import type { Foo } from './foo'"},
		{ir.DistilledImport{ImportType: "import", Module: "./def", IsType: true, Symbols: []ir.ImportedSymbol{sym("default", "Def")}}, "import type Def from './def'"},
		{ir.DistilledImport{ImportType: "import", Module: "./x", Symbols: []ir.ImportedSymbol{{Name: "T", IsType: true}, sym("V", "")}}, "import { type T, V } from './x'"},
		// A "default" symbol after the first one was written in braces.
		{ir.DistilledImport{ImportType: "import", Module: "./m", Symbols: []ir.ImportedSymbol{sym("A", ""), sym("default", "B")}}, "import { A, default as B } from './m'"},
		{ir.DistilledImport{ImportType: "require", Module: "f", Symbols: []ir.ImportedSymbol{sym("*", "F")}}, "import F = require('f')"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			imp := tc.imp
			assert.Equal(t, tc.want, esImportLine(&imp))
		})
	}
}

// structuredOutputs formats file as jsonl, json-structured and xml.
func structuredOutputs(t *testing.T, file *ir.DistilledFile) map[string]string {
	t.Helper()
	out := map[string]string{}
	for name, f := range map[string]Formatter{
		"jsonl":           NewJSONLFormatter(Options{}),
		"json-structured": NewJSONStructuredFormatter(Options{}),
		"xml":             NewXMLFormatter(Options{}),
	} {
		var b strings.Builder
		require.NoError(t, f.Format(&b, file))
		out[name] = b.String()
	}
	return out
}

// `import type` and an inline `type` specifier reach every structured format.
func TestStructuredFormatsShowTypeImports(t *testing.T) {
	file := &ir.DistilledFile{Path: "t.ts", Language: "typescript", Children: []ir.DistilledNode{
		&ir.DistilledImport{ImportType: "import", Module: "./foo", IsType: true, Symbols: []ir.ImportedSymbol{{Name: "Foo"}}},
		&ir.DistilledImport{ImportType: "import", Module: "./x", Symbols: []ir.ImportedSymbol{{Name: "T", IsType: true}}},
	}}
	out := structuredOutputs(t, file)
	assert.Equal(t, 2, strings.Count(out["jsonl"], `"is_type":true`), out["jsonl"])
	assert.Equal(t, 2, strings.Count(out["json-structured"], `"is_type": true`), out["json-structured"])
	assert.Equal(t, 2, strings.Count(out["xml"], `is_type="true"`), out["xml"])
}

func TestESExportLine(t *testing.T) {
	sym := func(name, alias string) ir.ImportedSymbol { return ir.ImportedSymbol{Name: name, Alias: alias} }
	for _, tc := range []struct {
		exp  ir.DistilledExport
		want string
	}{
		{ir.DistilledExport{Module: "./a", Symbols: []ir.ImportedSymbol{sym("*", "")}}, "export * from './a'"},
		{ir.DistilledExport{Module: "./b", Symbols: []ir.ImportedSymbol{sym("*", "ns")}}, "export * as ns from './b'"},
		{ir.DistilledExport{Module: "./c", Symbols: []ir.ImportedSymbol{sym("x", ""), sym("y", "z")}}, "export { x, y as z } from './c'"},
		{ir.DistilledExport{Symbols: []ir.ImportedSymbol{sym("q", "")}}, "export { q }"},
		{ir.DistilledExport{Module: "./t", IsType: true, Symbols: []ir.ImportedSymbol{sym("T", "")}}, "export type { T } from './t'"},
		{ir.DistilledExport{Symbols: []ir.ImportedSymbol{{Name: "P", IsType: true}, sym("default", "")}}, "export { type P, default }"},
		{ir.DistilledExport{IsDefault: true, Value: "Inner"}, "export default Inner"},
		{ir.DistilledExport{Value: "Foo"}, "export = Foo"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			exp := tc.exp
			assert.Equal(t, tc.want, esExportLine(&exp))
			assert.Equal(t, tc.want+"\n", tsFormat(t, &exp, 0))
			assert.Equal(t, tc.want+"\n", jsFormat(t, &exp, 0))
		})
	}
}

// Every structured format shows an export with all its parts.
func TestStructuredFormatsShowExports(t *testing.T) {
	file := &ir.DistilledFile{Path: "t.ts", Language: "typescript", Children: []ir.DistilledNode{
		&ir.DistilledExport{Module: "./c", IsType: true, Symbols: []ir.ImportedSymbol{{Name: "y", Alias: "z"}}},
		&ir.DistilledExport{IsDefault: true, Value: "Inner"},
	}}
	out := structuredOutputs(t, file)
	assert.Contains(t, out["jsonl"], `{"file":"t.ts","is_type":true,"module":"./c","name":"./c","path":[],"symbols":[{"name":"y","alias":"z"}],"type":"export"}`)
	assert.Contains(t, out["jsonl"], `{"file":"t.ts","is_default":true,"name":"default","path":[],"type":"export","value":"Inner"}`)
	assert.Contains(t, out["json-structured"], `"exports": [`)
	assert.Contains(t, out["json-structured"], `"value": "Inner"`)
	assert.Contains(t, out["xml"], `<export module="./c" is_type="true">`)
	assert.Contains(t, out["xml"], `<symbol name="y" alias="z"/>`)
	assert.Contains(t, out["xml"], `<export value="Inner" is_default="true"/>`)
}

// An exported declaration prints `export` or `export default`.
func TestTypeScriptFormatter_ExportDefault(t *testing.T) {
	exportDefault := []ir.Modifier{ir.ModifierExport, ir.ModifierDefault}
	assert.Equal(t, "export default function main(): void\n",
		tsFormat(t, &ir.DistilledFunction{Name: "main", Modifiers: exportDefault, Returns: &ir.TypeRef{Name: "void"}}, 0))
	assert.Equal(t, "\nexport default class {\n}\n", tsFormat(t, &ir.DistilledClass{Modifiers: exportDefault}, 0))
	assert.Equal(t, "\nexport default interface I {\n}\n", tsFormat(t, &ir.DistilledInterface{Name: "I", Modifiers: exportDefault}, 0))
}

// The TypeScript and JavaScript formatters print an import the same way.
func TestESImportFormatters(t *testing.T) {
	imp := &ir.DistilledImport{ImportType: "import", Module: "react", Symbols: []ir.ImportedSymbol{{Name: "default", Alias: "React"}, {Name: "memo"}}}
	assert.Equal(t, "import React, { memo } from 'react'\n", tsFormat(t, imp, 0))
	assert.Equal(t, "import React, { memo } from 'react'\n", jsFormat(t, imp, 0))
}

// A top-level variable prints its signature value by default and its whole
// initializer when it keeps its Implementation; an async arrow const keeps
// its async.
func TestESVariableInitializers(t *testing.T) {
	mods := []ir.Modifier{ir.ModifierFinal, ir.ModifierExport}
	field := &ir.DistilledField{Name: "B", Modifiers: mods, DefaultValue: "forwardRef((p, r))"}
	assert.Equal(t, "export const B = forwardRef((p, r))\n", tsFormat(t, field, 0))
	assert.Equal(t, "export const B = forwardRef((p, r))\n", jsFormat(t, field, 0))
	field.Implementation = "forwardRef((p, r) => {\n  return p;\n})"
	assert.Equal(t, "export const B = forwardRef((p, r) => {\n    return p;\n})\n", tsFormat(t, field, 0))
	assert.Equal(t, "export const B = forwardRef((p, r) => {\n    return p;\n})\n", jsFormat(t, field, 0))
	fn := &ir.DistilledFunction{Name: "f", Modifiers: []ir.Modifier{ir.ModifierAsync, ir.ModifierFinal, ir.ModifierExport},
		TypeParams: []ir.TypeParam{{Name: "T"}}, Parameters: []ir.Parameter{{Name: "x", Type: ir.TypeRef{Name: "T"}}}}
	assert.Equal(t, "export const f = async <T>(x: T)\n", tsFormat(t, fn, 0))
	fn.TypeParams = nil
	fn.Parameters = []ir.Parameter{{Name: "x"}}
	assert.Equal(t, "export const f = async (x)\n", jsFormat(t, fn, 0))
}

// Every structured format shows a variable's implementation.
func TestStructuredFormatsShowFieldImplementation(t *testing.T) {
	file := &ir.DistilledFile{Path: "t.ts", Language: "typescript", Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "B", Visibility: ir.VisibilityPublic, DefaultValue: "memo(function L())", Implementation: "memo(function L() { return 1; })"},
	}}
	out := structuredOutputs(t, file)
	for _, format := range []string{"jsonl", "json-structured", "xml"} {
		assert.Contains(t, out[format], "memo(function L() { return 1; })", format)
	}
}
