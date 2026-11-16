package importfilter

import (
	"strings"
	"testing"
)

func TestPythonFilter(t *testing.T) {
	filter := NewPythonFilter()

	tests := []struct {
		name     string
		code     string
		expected []string // Expected removed imports
	}{
		{
			name: "simple imports with usage",
			code: `<file path="test.py">
import os
import sys
import json

class DataProcessor:
    def process(self, data):
        return json.dumps(data)
</file>`,
			expected: []string{"import os", "import sys"},
		},
		{
			name: "from imports with aliases",
			code: `<file path="test.py">
from typing import List, Dict, Optional
from collections import defaultdict
import numpy as np

def analyze(items: List[str]) -> Dict[str, int]:
    result = defaultdict(int)
    for item in items:
        result[item] += 1
    return dict(result)
</file>`,
			expected: []string{"import numpy as np"},
		},
		{
			name: "wildcard imports",
			code: `<file path="test.py">
from os import *
from sys import path

print(path)
</file>`,
			expected: []string{}, // Wildcard imports are kept
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, removed, err := filter.FilterUnusedImports(tt.code, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that expected imports were removed
			for _, exp := range tt.expected {
				found := false
				for _, rem := range removed {
					if strings.TrimSpace(rem) == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected import %q to be removed, but it wasn't", exp)
				}
			}

			// Check that removed imports are not in the output
			for _, rem := range removed {
				if strings.Contains(filtered, rem) {
					t.Errorf("removed import %q still appears in output", rem)
				}
			}
		})
	}
}

func TestJavaScriptFilter(t *testing.T) {
	filter := NewJavaScriptFilter()

	tests := []struct {
		name     string
		code     string
		expected []string // Expected removed imports
	}{
		{
			name: "ES6 imports",
			code: `<file path="app.js">
import React from 'react'
import { useState, useEffect } from 'react'
import axios from 'axios'

function App() {
    const [data, setData] = useState([])
    
    useEffect(() => {
        console.log('mounted')
    }, [])
    
    return <div>{data.length}</div>
}
</file>`,
			expected: []string{"import React from 'react'", "import axios from 'axios'"},
		},
		{
			name: "namespace imports",
			code: `<file path="utils.js">
import * as fs from 'fs'
import * as path from 'path'
import { debounce } from 'lodash'

export const readFile = (file) => {
    return fs.readFileSync(path.join(__dirname, file))
}
</file>`,
			expected: []string{"import { debounce } from 'lodash'"},
		},
		{
			name: "side-effect imports",
			code: `<file path="index.js">
import './polyfills'
import 'reflect-metadata'
import express from 'express'

const app = express()
</file>`,
			expected: []string{}, // Side-effect imports are kept
		},
	}
	// The forms the JavaScript and TypeScript formatters print.
	forms := []struct {
		name string
		line string
		uses string // a printed line that uses the import; "" for none
	}{
		{"default", "import D from './d'", "function f(d: D)"},
		{"named", "import { A } from './a'", "function f(a: A)"},
		{"alias", "import { B as C } from './b'", "function f(c: C)"},
		{"alias, original name only", "import { B as C } from './b'", ""},
		{"default and named, default used", "import React, { useState } from 'react'", "class K extends React.Component"},
		{"default and named, named used", "import React, { useState } from 'react'", "const s = useState"},
		{"default and namespace", "import H, * as I from './h'", "function f(i: I.T)"},
		{"namespace", "import * as z from 'zod'", "function f(x: z.infer)"},
		{"type import", "import type { Foo } from './foo'", "function f(x: Foo)"},
		{"type default import", "import type Def from './def'", "function f(x: Def)"},
		{"inline type", "import { type T, V } from './x'", "function f(t: T)"},
		{"default as", "import { A, default as B } from './m'", "function f(b: B)"},
		{"import equals require", "import F = require('f')", "function f(x: F)"},
		{"import alias", "import G = ns.Inner.H", "function f(x: G)"},
		{"import alias, unused", "import G = ns.Inner.H", ""},
	}
	for _, form := range forms {
		// B, the original name of the alias C, occurs in every file.
		code := "<file path=\"x.ts\">\n" + form.line + "\n" + form.uses + "\nfunction B()\n</file>"
		_, removed, err := filter.FilterUnusedImports(code, 0)
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", form.name, err)
		}
		wantRemoved := form.uses == ""
		if gotRemoved := len(removed) == 1; gotRemoved != wantRemoved {
			t.Errorf("%s: %q removed=%v, want %v", form.name, form.line, gotRemoved, wantRemoved)
		}
	}
	// A re-export is never an import to remove, even when nothing uses its names.
	reexports := "<file path=\"x.ts\">\nimport { used } from './u'\nexport * from './a'\nexport { x, y as z } from './c'\nexport type { T } from './t'\nexport default used\nexport import E = ns.E\nexport as namespace Lib\n</file>"
	if filtered, removed, _ := filter.FilterUnusedImports(reexports, 0); len(removed) != 0 || filtered != reexports {
		t.Errorf("re-exports changed: removed %v", removed)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, removed, err := filter.FilterUnusedImports(tt.code, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that expected imports were removed
			for _, exp := range tt.expected {
				found := false
				for _, rem := range removed {
					if strings.TrimSpace(rem) == exp {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected import %q to be removed, but it wasn't", exp)
				}
			}

			// Check that removed imports are not in the output
			for _, rem := range removed {
				if strings.Contains(filtered, rem) {
					t.Errorf("removed import %q still appears in output", rem)
				}
			}
		})
	}
}

func TestGoFilterRemovesEmptiedImportBlock(t *testing.T) {
	filter := NewGoFilter()
	run := func(code string) string {
		t.Helper()
		out, _, err := filter.FilterUnusedImports(code, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}

	// No import is left: the block goes whole, with its comment and the
	// blank line above it.
	code := "<file path=\"a.go\">\npackage user\n\nimport (\n\t\"fmt\"\n// for errors\n\tm \"math\"\n)\n\n" +
		"// User doc\n\ntype User struct {\n\tID string\n}\n</file>"
	want := "<file path=\"a.go\">\npackage user\n\n// User doc\n\ntype User struct {\n\tID string\n}\n</file>"
	if got := run(code); got != want {
		t.Errorf("emptied block:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// One import is still used: the block stays with it.
	code = "<file path=\"a.go\">\npackage user\n\nimport (\n\t\"fmt\"\n\t\"io\"\n)\n\ntype R struct {\n\tr io.Reader\n}\n</file>"
	want = "<file path=\"a.go\">\npackage user\n\nimport (\n\t\"io\"\n)\n\ntype R struct {\n\tr io.Reader\n}\n</file>"
	if got := run(code); got != want {
		t.Errorf("block with a used import:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// A blank import is always kept, and so is its block.
	code = "<file path=\"a.go\">\npackage user\n\nimport (\n\t\"fmt\"\n\t_ \"embed\"\n)\n\ntype T struct{}\n</file>"
	want = "<file path=\"a.go\">\npackage user\n\nimport (\n\t_ \"embed\"\n)\n\ntype T struct{}\n</file>"
	if got := run(code); got != want {
		t.Errorf("block with a blank import:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// A ")" in a comment inside the block does not end it.
	code = "<file path=\"a.go\">\npackage user\n\nimport (\n\t\"fmt\"\n\t// os (files)\n\t\"os\"\n)\n\nfunc F(f *os.File)\n</file>"
	want = "<file path=\"a.go\">\npackage user\n\nimport (\n\t// os (files)\n\t\"os\"\n)\n\nfunc F(f *os.File)\n</file>"
	if got := run(code); got != want {
		t.Errorf("comment with a parenthesis:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestGoFilterRemovesTheCommentsOfARemovedImport(t *testing.T) {
	filter := NewGoFilter()
	run := func(code string) string {
		t.Helper()
		out, _, err := filter.FilterUnusedImports(code, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return out
	}

	// The comment at the end of a removed import's line and the comment
	// lines directly above it go with it; those of a kept import stay.
	code := "<file path=\"a.go\">\npackage p\n\nimport (\n\t// Standard library\n\t// (printing)\n\t\"fmt\" // fmt\n" +
		"\t// files\n\t\"os\" // os\n)\n\nfunc F(f *os.File)\n</file>"
	want := "<file path=\"a.go\">\npackage p\n\nimport (\n\t// files\n\t\"os\" // os\n)\n\nfunc F(f *os.File)\n</file>"
	if got := run(code); got != want {
		t.Errorf("removed import:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// A block that goes whole takes the comment lines directly above it.
	code = "<file path=\"a.go\">\npackage p\n\n// Imports.\nimport (\n\t\"fmt\" // fmt\n)\n\nfunc F()\n</file>"
	want = "<file path=\"a.go\">\npackage p\n\nfunc F()\n</file>"
	if got := run(code); got != want {
		t.Errorf("removed block:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestGoFilter(t *testing.T) {
	filter := NewGoFilter()

	tests := []struct {
		name     string
		code     string
		expected []string // Expected removed imports
	}{
		{
			name: "standard imports",
			code: `<file path="main.go">
package main

import (
    "fmt"
    "os"
    "strings"
)

func main() {
    fmt.Println(strings.ToUpper("hello"))
}
</file>`,
			expected: []string{`"os"`},
		},
		{
			name: "aliased imports",
			code: `<file path="server.go">
package main

import (
    "net/http"
    mux "github.com/gorilla/mux"
    _ "github.com/lib/pq"
)

func main() {
    r := mux.NewRouter()
    http.ListenAndServe(":8080", r)
}
</file>`,
			expected: []string{}, // Blank import is kept
		},
		{
			name: "unused aliased import",
			code: `<file path="utils.go">
package utils

import (
    j "encoding/json"
    "fmt"
)

func Debug(v interface{}) {
    fmt.Printf("%+v\n", v)
}
</file>`,
			expected: []string{`j "encoding/json"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered, removed, err := filter.FilterUnusedImports(tt.code, 0)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Check that expected imports were removed
			for _, exp := range tt.expected {
				found := false
				for _, rem := range removed {
					if strings.Contains(strings.TrimSpace(rem), exp) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected import %q to be removed, but it wasn't", exp)
				}
			}

			// Check that removed imports are not in the output
			for _, rem := range removed {
				if strings.Contains(filtered, rem) {
					t.Errorf("removed import %q still appears in output", rem)
				}
			}
		})
	}
}

func TestSearchForUsage(t *testing.T) {
	base := NewBaseFilter("test")

	tests := []struct {
		name          string
		code          string
		searchName    string
		importEndLine int
		expectedFound bool
	}{
		{
			name:          "simple usage",
			code:          "import foo\n\nbar = foo.method()",
			searchName:    "foo",
			importEndLine: 1,
			expectedFound: true,
		},
		{
			name:          "no usage",
			code:          "import foo\n\nbar = baz.method()",
			searchName:    "foo",
			importEndLine: 1,
			expectedFound: false,
		},
		{
			name:          "usage before import line",
			code:          "x = foo\nimport foo\n",
			searchName:    "foo",
			importEndLine: 2,
			expectedFound: false, // Should not find usage before imports
		},
		{
			name:          "word boundary test",
			code:          "import foo\n\nfoobar = 1",
			searchName:    "foo",
			importEndLine: 1,
			expectedFound: false, // Should not match partial words
		},
		{
			name:          "usage in string",
			code:          "import foo\n\nprint('using foo here')",
			searchName:    "foo",
			importEndLine: 1,
			expectedFound: true, // We do match in strings
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := base.SearchForUsage(tt.code, tt.searchName, tt.importEndLine)
			if found != tt.expectedFound {
				t.Errorf("SearchForUsage(%q) = %v, want %v", tt.searchName, found, tt.expectedFound)
			}
		})
	}
}
