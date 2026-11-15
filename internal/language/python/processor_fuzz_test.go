//go:build go1.18
// +build go1.18

package python

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
)

// FuzzProcess tests the parser with random inputs to ensure it never panics
func FuzzProcess(f *testing.F) {
	// Add seed corpus from our test files
	testFiles, err := filepath.Glob("../../../testdata/python/*/source.py")
	if err != nil {
		f.Fatal(err)
	}

	// Add contents of test files as corpus
	seeds := 0
	for _, file := range testFiles {
		content, err := os.ReadFile(file)
		if err != nil {
			f.Fatalf("reading seed %s: %v", file, err)
		}
		f.Add(content)
		seeds++
	}
	if seeds < 5 {
		f.Fatalf("expected at least 5 python seed files, found %d", seeds)
	}

	// Add some specific edge cases as corpus
	f.Add([]byte("class A:\n\tpass"))
	f.Add([]byte("def f():\n\tpass"))
	f.Add([]byte("import os\nfrom sys import *"))
	f.Add([]byte("class 中文类名:\n\tdef 方法(self):\n\t\tpass"))
	f.Add([]byte("def f(\n\tx: int,\n\ty: str\n) -> bool:\n\tpass"))
	f.Add([]byte("from . import (\n\ta,\n\tb,\n\tc\n)"))

	// Add malformed inputs
	f.Add([]byte("class"))
	f.Add([]byte("def"))
	f.Add([]byte("import"))
	f.Add([]byte("class A(\n"))
	f.Add([]byte("def f(x:"))
	f.Add([]byte("\x00\x01\x02"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, input []byte) {
		// Create processor
		p := NewProcessor()
		ctx := context.Background()
		reader := bytes.NewReader(input)

		// Property 1: Parser should never panic
		file, err := p.Process(ctx, reader, "fuzz_test.py")

		// Property 2: If no error, result should be valid
		if err == nil && file != nil {
			// Validate the IR structure
			assertValidIR(t, file)
		}

		// Property 3: Parser should be deterministic
		// Run again with same input
		reader2 := bytes.NewReader(input)
		file2, err2 := p.Process(ctx, reader2, "fuzz_test.py")

		// Results should be identical
		if (err == nil) != (err2 == nil) {
			t.Error("Parser not deterministic: different error states")
		}

		if err == nil && err2 == nil {
			// Both succeeded, structures should be equal
			// (simplified check - in reality would need deep comparison)
			if len(file.Children) != len(file2.Children) {
				t.Error("Parser not deterministic: different child counts")
			}
		}
	})
}

// assertValidIR validates that the IR structure is internally consistent
func assertValidIR(t *testing.T, file *ir.DistilledFile) {
	// Check file properties
	if file.Path != "fuzz_test.py" {
		t.Error("Invalid file path")
	}
	if file.Language != "python" {
		t.Error("Invalid language")
	}

	// Walk the tree and validate each node
	var validateNode func(node ir.DistilledNode, parent ir.DistilledNode, depth int)
	validateNode = func(node ir.DistilledNode, parent ir.DistilledNode, depth int) {
		if node == nil {
			t.Error("Nil node in tree")
			return
		}

		// Check location
		loc := node.GetLocation()
		if loc.StartLine < 0 || loc.EndLine < loc.StartLine {
			t.Errorf("Invalid location: start=%d, end=%d", loc.StartLine, loc.EndLine)
		}

		// Type-specific validations
		switch n := node.(type) {
		case *ir.DistilledClass:
			if n.Name == "" {
				t.Error("Class with empty name")
			}
			// Classes should have valid visibility
			if n.Visibility != "" && !isValidVisibility(n.Visibility) {
				t.Errorf("Invalid visibility: %s", n.Visibility)
			}

		case *ir.DistilledFunction:
			if n.Name == "" {
				t.Error("Function with empty name")
			}
			// Check parameters
			for _, param := range n.Parameters {
				if param.Name == "" {
					t.Error("Parameter with empty name")
				}
			}

		case *ir.DistilledImport:
			if n.Module == "" && len(n.Symbols) == 0 {
				t.Error("Import with no module or symbols")
			}
			// Check import type
			if n.ImportType != "import" && n.ImportType != "from" {
				t.Errorf("Invalid import type: %s", n.ImportType)
			}
		}

		// Recursively validate children
		children := node.GetChildren()
		for _, child := range children {
			validateNode(child, node, depth+1)
		}
	}

	// Start validation from root
	validateNode(file, nil, 0)
}

func isValidVisibility(v ir.Visibility) bool {
	validVisibilities := []ir.Visibility{
		ir.VisibilityPublic,
		ir.VisibilityPrivate,
		ir.VisibilityProtected,
		ir.VisibilityInternal,
	}
	for _, valid := range validVisibilities {
		if v == valid {
			return true
		}
	}
	return false
}
