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
