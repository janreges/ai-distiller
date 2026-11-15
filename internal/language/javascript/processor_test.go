package javascript

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
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
				{module: "react", symbols: []string{"React"}},
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
				{module: "react", symbols: []string{"React"}},
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
