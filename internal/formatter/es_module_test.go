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

// The TypeScript and JavaScript formatters print an import the same way.
func TestESImportFormatters(t *testing.T) {
	imp := &ir.DistilledImport{ImportType: "import", Module: "react", Symbols: []ir.ImportedSymbol{{Name: "default", Alias: "React"}, {Name: "memo"}}}
	assert.Equal(t, "import React, { memo } from 'react'\n", tsFormat(t, imp, 0))
	assert.Equal(t, "import React, { memo } from 'react'\n", jsFormat(t, imp, 0))
}
