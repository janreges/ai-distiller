package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseJS parses src with the real JavaScript processor and returns the raw IR.
func parseJS(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	f, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "t.js")
	if err != nil {
		t.Fatalf("process: %v", err)
	}
	return f
}

// findJSClass returns the top-level class called name; it fails the test when absent.
func findJSClass(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledClass {
	t.Helper()
	for _, c := range file.Children {
		if cls, ok := c.(*ir.DistilledClass); ok && cls.Name == name {
			return cls
		}
	}
	t.Fatalf("class %q not found", name)
	return nil
}

func jsMemberVisibility(cls *ir.DistilledClass) map[string]ir.Visibility {
	vis := map[string]ir.Visibility{}
	for _, c := range cls.Children {
		switch n := c.(type) {
		case *ir.DistilledFunction:
			vis[n.Name] = n.Visibility
		case *ir.DistilledField:
			vis[n.Name] = n.Visibility
		}
	}
	return vis
}

func TestJS_UnderscoreMembersArePrivate(t *testing.T) {
	cls := findJSClass(t, parseJS(t, "class K {\n  _helper() { return 1; }\n  open() { return 2; }\n  static _cache = 1;\n  /** @public */\n  _exposed() { return 3; }\n}\n"), "K")
	vis := jsMemberVisibility(cls)
	assert.Equal(t, ir.VisibilityPrivate, vis["_helper"])
	assert.Equal(t, ir.VisibilityPublic, vis["open"])
	assert.Equal(t, ir.VisibilityPrivate, vis["_cache"])
	assert.Equal(t, ir.VisibilityPublic, vis["_exposed"], "JSDoc @public overrides the underscore rule")
}

func TestJS_HashMembersAreNamedAndPrivate(t *testing.T) {
	cls := findJSClass(t, parseJS(t, "class K {\n  #p() { return 1; }\n  get #g() { return 1; }\n  static #s = 1;\n}\n"), "K")
	vis := jsMemberVisibility(cls)
	assert.Equal(t, ir.VisibilityPrivate, vis["#p"])
	assert.Equal(t, ir.VisibilityPrivate, vis["get #g"])
	assert.Equal(t, ir.VisibilityPrivate, vis["#s"])
}

func TestProcessorGeneratorFunctions(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string // Expected function names
	}{
		{
			name: "generator_function",
			input: `
function* fibonacci() {
    yield 1;
}`,
			expected: []string{"*fibonacci"},
		},
		{
			name: "async_generator_function",
			input: `
async function* asyncGen() {
    yield await fetch('/data');
}`,
			expected: []string{"*asyncGen"},
		},
		{
			name: "generator_expression",
			input: `
const gen = function* () {
    yield 42;
};`,
			expected: []string{"*gen"},
		},
		{
			name: "mixed_functions",
			input: `
function regular() { return 1; }
function* generator() { yield 2; }
async function asyncFunc() { return 3; }`,
			expected: []string{"regular", "*generator", "asyncFunc"},
		},
	}

	processor := NewProcessor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Process(context.Background(), strings.NewReader(tt.input), "test.js")
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}

			functions := extractFunctionNames(result)

			if len(functions) != len(tt.expected) {
				t.Errorf("Expected %d functions, got %d: %v", len(tt.expected), len(functions), functions)
			}

			for i, expected := range tt.expected {
				if i < len(functions) && functions[i] != expected {
					t.Errorf("Function %d: expected %q, got %q", i, expected, functions[i])
				}
			}
		})
	}
}

func TestProcessorImports(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []importInfo
	}{
		{
			name:  "default_import",
			input: `import React from 'react';`,
			expected: []importInfo{
				{module: "react", symbols: []string{"default"}, aliases: []string{"React"}},
			},
		},
		{
			name:  "default_and_named_imports",
			input: `import React, { useState } from 'react';`,
			expected: []importInfo{
				{module: "react", symbols: []string{"default", "useState"}, aliases: []string{"React"}},
			},
		},
		{
			name:  "named_imports",
			input: `import { Component, useState } from 'react';`,
			expected: []importInfo{
				{module: "react", symbols: []string{"Component", "useState"}},
			},
		},
		{
			name:  "aliased_imports",
			input: `import { Component as Comp, useState as useLocalState } from 'react';`,
			expected: []importInfo{
				{module: "react", symbols: []string{"Component", "useState"}, aliases: []string{"Comp", "useLocalState"}},
			},
		},
		{
			name:  "namespace_import",
			input: `import * as utils from './utils';`,
			expected: []importInfo{
				{module: "./utils", symbols: []string{"*"}, aliases: []string{"utils"}},
			},
		},
		{
			name: "mixed_imports",
			input: `
import React from 'react';
import { render } from 'react-dom';
import * as lodash from 'lodash';`,
			expected: []importInfo{
				{module: "react", symbols: []string{"default"}, aliases: []string{"React"}},
				{module: "react-dom", symbols: []string{"render"}},
				{module: "lodash", symbols: []string{"*"}, aliases: []string{"lodash"}},
			},
		},
	}

	processor := NewProcessor()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Process(context.Background(), strings.NewReader(tt.input), "test.js")
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}

			imports := extractImports(result)

			if len(imports) != len(tt.expected) {
				t.Errorf("Expected %d imports, got %d", len(tt.expected), len(imports))
			}

			for i, expected := range tt.expected {
				if i >= len(imports) {
					break
				}

				actual := imports[i]
				if actual.module != expected.module {
					t.Errorf("Import %d module: expected %q, got %q", i, expected.module, actual.module)
				}

				if !slicesEqual(actual.symbols, expected.symbols) {
					t.Errorf("Import %d symbols: expected %v, got %v", i, expected.symbols, actual.symbols)
				}

				if len(expected.aliases) > 0 && !slicesEqual(actual.aliases, expected.aliases) {
					t.Errorf("Import %d aliases: expected %v, got %v", i, expected.aliases, actual.aliases)
				}
			}
		})
	}
}

// Helper types and functions

type importInfo struct {
	module  string
	symbols []string
	aliases []string
}

func extractFunctionNames(file *ir.DistilledFile) []string {
	var names []string
	for _, child := range file.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok {
			names = append(names, fn.Name)
		}
	}
	return names
}

func extractImports(file *ir.DistilledFile) []importInfo {
	var imports []importInfo
	for _, child := range file.Children {
		if imp, ok := child.(*ir.DistilledImport); ok {
			info := importInfo{
				module:  imp.Module,
				symbols: make([]string, 0, len(imp.Symbols)),
				aliases: make([]string, 0, len(imp.Symbols)),
			}

			for _, sym := range imp.Symbols {
				info.symbols = append(info.symbols, sym.Name)
				if sym.Alias != "" {
					info.aliases = append(info.aliases, sym.Alias)
				}
			}

			imports = append(imports, info)
		}
	}
	return imports
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestArrowFunctionBody(t *testing.T) {
	cases := map[string]struct {
		src        string
		wantImpl   string
		wantParams []string
	}{
		"expression body":             {"const f = (a, b) => a + b;", "a + b", []string{"a", "b"}},
		"bare parameter":              {"const f = x => x * 2;", "x * 2", []string{"x"}},
		"identifier body is no param": {"const f = x => y;", "y", []string{"x"}},
		"block body":                  {"const f = async (a) => { return a; };", "{ return a; }", []string{"a"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			file, err := NewProcessor().Process(context.Background(), strings.NewReader(c.src), "test.js")
			if err != nil {
				t.Fatalf("Failed to process: %v", err)
			}
			var fn *ir.DistilledFunction
			for _, child := range file.Children {
				if f, ok := child.(*ir.DistilledFunction); ok && f.Name == "f" {
					fn = f
				}
			}
			if fn == nil {
				t.Fatalf("function f not found in %#v", file.Children)
			}
			if fn.Implementation != c.wantImpl {
				t.Errorf("Implementation = %q, want %q", fn.Implementation, c.wantImpl)
			}
			var params []string
			for _, p := range fn.Parameters {
				params = append(params, p.Name)
			}
			if !slicesEqual(params, c.wantParams) {
				t.Errorf("Parameters = %v, want %v", params, c.wantParams)
			}
		})
	}
}

func TestJS_Comments(t *testing.T) {
	f := parseJS(t, "// line c\n/** Doc c */\nexport class K {\n  // inner\n  /** m doc */\n  m() {}\n}\n")
	top0 := f.Children[0].(*ir.DistilledComment)
	assert.Equal(t, "line c", top0.Text)
	assert.Equal(t, ir.CommentLine, top0.Format)
	top1 := f.Children[1].(*ir.DistilledComment)
	assert.Equal(t, "Doc c", top1.Text)
	assert.True(t, top1.IsDoc())
	cls := findJSClass(t, f, "K")
	assert.Equal(t, "inner", cls.Children[0].(*ir.DistilledComment).Text)
	doc := cls.Children[1].(*ir.DistilledComment)
	assert.Equal(t, "m doc", doc.Text)
	assert.True(t, doc.IsDoc())
}

// The JSDoc block is emitted as a doc and its tags are still read.
func TestJS_DocKeepsTagParsing(t *testing.T) {
	f := parseJS(t, "/**\n * Adds.\n * @param {number} a\n * @returns {number}\n */\nfunction add(a) { return a; }\n/* block c */\n")
	doc := f.Children[0].(*ir.DistilledComment)
	assert.Equal(t, "Adds.\n@param {number} a\n@returns {number}", doc.Text)
	assert.Equal(t, ir.CommentDoc, doc.Format)
	fn := f.Children[1].(*ir.DistilledFunction)
	assert.Equal(t, "number", fn.Returns.Name)
	assert.Equal(t, "number", fn.Parameters[0].Type.Name)
	block := f.Children[2].(*ir.DistilledComment)
	assert.Equal(t, "block c", block.Text)
	assert.Equal(t, ir.CommentBlock, block.Format)
}

func TestJS_FileDocIsInner(t *testing.T) {
	f := parseJS(t, "/**\n * @file app.js\n */\n/** K doc */\nclass K {}\n")
	assert.True(t, f.Children[0].(*ir.DistilledComment).Inner, "a @file doc documents the file")
	assert.False(t, f.Children[1].(*ir.DistilledComment).Inner)
}

// docTexts returns the texts of the doc comments among nodes.
func docTexts(nodes []ir.DistilledNode) []string {
	var texts []string
	for _, n := range nodes {
		if c, ok := n.(*ir.DistilledComment); ok && c.IsDoc() {
			texts = append(texts, c.Text)
		}
	}
	return texts
}

// A doc before a statement that yields no declaration (at most an export
// marker) documented that statement; it must not end up above the next
// declaration. Line comments keep their place.
func TestJS_DocOfStatementWithoutDeclarationIsDropped(t *testing.T) {
	for name, stmt := range map[string]string{
		"exports.x":      "exports.add = function (a) { return a; };",
		"module.exports": "module.exports = { a: 1 };",
		"call":           "main();",
	} {
		t.Run(name, func(t *testing.T) {
			f := parseJS(t, "/** @deprecated Use bar. */\n// line kept\n"+stmt+"\n/** bar doc */\nexport function bar() {}\n")
			assert.Equal(t, []string{"bar doc"}, docTexts(f.Children))
			assert.Equal(t, "line kept", f.Children[0].(*ir.DistilledComment).Text)
			last := len(f.Children) - 1
			assert.Equal(t, "bar doc", f.Children[last-1].(*ir.DistilledComment).Text)
			assert.Equal(t, "bar", f.Children[last].(*ir.DistilledFunction).Name)
		})
	}
	t.Run("export default class keeps its doc", func(t *testing.T) {
		f := parseJS(t, "/** Foo doc */\nexport default class Foo {}\n")
		assert.Equal(t, []string{"Foo doc"}, docTexts(f.Children))
	})
	// A re-export and `export default <expression>` are export nodes now:
	// the doc above them documents them.
	for name, stmt := range map[string]string{
		"re-export":      `export { foo } from "./foo";`,
		"export default": "export default { a: 1 };",
	} {
		t.Run(name+" keeps its doc", func(t *testing.T) {
			f := parseJS(t, "/** export doc */\n"+stmt+"\n")
			require.Len(t, f.Children, 2)
			assert.Equal(t, "export doc", f.Children[0].(*ir.DistilledComment).Text)
			assert.IsType(t, &ir.DistilledExport{}, f.Children[1])
		})
	}
}

// Statements that export without declaring become export nodes, in the
// form ir.DistilledExport documents.
func TestJS_ExportStatements(t *testing.T) {
	f := parseJS(t, "export * from \"./a\";\nexport * as ns from \"./b\";\nexport { x, y as z } from \"./c\";\nconst q = 1;\nexport { q };\nexport default q;\n")
	var exports []*ir.DistilledExport
	for _, c := range f.Children {
		if e, ok := c.(*ir.DistilledExport); ok {
			exports = append(exports, e)
		}
	}
	require.Len(t, exports, 5)
	assert.Equal(t, &ir.DistilledExport{BaseNode: exports[0].BaseNode, Module: "./a", Symbols: []ir.ImportedSymbol{{Name: "*"}}}, exports[0])
	assert.Equal(t, []ir.ImportedSymbol{{Name: "*", Alias: "ns"}}, exports[1].Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "x"}, {Name: "y", Alias: "z"}}, exports[2].Symbols)
	assert.Equal(t, "", exports[3].Module)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "q"}}, exports[3].Symbols)
	assert.True(t, exports[4].IsDefault)
	assert.Equal(t, "q", exports[4].Value)
}

// An exported declaration carries the modifier export, and default after
// `export default`; `export default class {}` is a class without a name.
func TestJS_ExportedDeclarations(t *testing.T) {
	f := parseJS(t, "export class K {}\nexport default class {}\nexport function* g() {}\nexport const c = 1, d = 2;\nexport var v = 3;\nfunction plain() {}\n")
	mods := map[string][]ir.Modifier{}
	for _, c := range f.Children {
		switch d := c.(type) {
		case *ir.DistilledClass:
			mods["class "+d.Name] = d.Modifiers
		case *ir.DistilledFunction:
			mods[d.Name] = d.Modifiers
		case *ir.DistilledField:
			mods[d.Name] = d.Modifiers
		}
	}
	export := []ir.Modifier{ir.ModifierExport}
	assert.Equal(t, map[string][]ir.Modifier{
		"class K": export,
		"class ":  {ir.ModifierExport, ir.ModifierDefault},
		"*g":      export,
		"c":       {ir.ModifierFinal, ir.ModifierExport},
		"d":       {ir.ModifierFinal, ir.ModifierExport},
		"v":       {ir.ModifierVar, ir.ModifierExport},
		"plain":   {},
	}, mods)
}

// A default value that is a name does not replace the parameter's name, and
// a destructuring pattern with a default keeps its pattern.
func TestJS_ParameterDefaults(t *testing.T) {
	f := parseJS(t, "function f(hook = useHook, { a } = {}, n = 1) {}\n")
	assert.Equal(t, []ir.Parameter{
		{Name: "hook", DefaultValue: "useHook"},
		{Name: "{ a }", DefaultValue: "{}"},
		{Name: "n", DefaultValue: "1"},
	}, f.Children[0].(*ir.DistilledFunction).Parameters)
}

// A const initialised with a call that takes a function keeps the call with
// the function's signature as its value and the whole initializer as its
// implementation.
func TestJS_WrapperConst(t *testing.T) {
	f := parseJS(t, "const Button = forwardRef(({ variant = 'a', ...props }, ref) => {\n  return props;\n});\nconst Plain = make(1);\n")
	b := f.Children[0].(*ir.DistilledField)
	assert.Equal(t, "forwardRef(({ variant = 'a', ...props }, ref))", b.DefaultValue)
	assert.Equal(t, "forwardRef(({ variant = 'a', ...props }, ref) => {\n  return props;\n})", b.Implementation)
	p := f.Children[1].(*ir.DistilledField)
	assert.Equal(t, "make(1)", p.DefaultValue)
	assert.Equal(t, "", p.Implementation)
}

// A variable whose value is a plain name keeps its own name.
func TestJS_VariableNamedValue(t *testing.T) {
	f := parseJS(t, "const encode = encodeURIComponent;\nconst { a, b } = obj;\nlet _hidden = 1;\n")
	enc := f.Children[0].(*ir.DistilledField)
	assert.Equal(t, "encode", enc.Name)
	assert.Equal(t, "encodeURIComponent", enc.DefaultValue)
	assert.Equal(t, "{ a, b }", f.Children[1].(*ir.DistilledField).Name)
	assert.Equal(t, ir.VisibilityPrivate, f.Children[2].(*ir.DistilledField).Visibility)
}

// A variable whose value is a function expression is a function marked as
// one, so it prints with `function`; an arrow function is not marked.
func TestJS_FunctionExpressionVariable(t *testing.T) {
	f := parseJS(t, "const fe = function (a) { return a; };\nconst ge = function* () {};\nconst ar = (a) => a;\n")
	isExpr := func(i int) bool {
		fn := f.Children[i].(*ir.DistilledFunction)
		return fn.Extensions != nil && fn.Extensions.JavaScript != nil && fn.Extensions.JavaScript.IsFunctionExpression
	}
	assert.True(t, isExpr(0))
	assert.True(t, isExpr(1))
	assert.Equal(t, "*ge", f.Children[1].(*ir.DistilledFunction).Name)
	assert.False(t, isExpr(2))
}
