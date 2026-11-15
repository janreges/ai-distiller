package python

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
)

func TestNewProcessor(t *testing.T) {
	proc := NewProcessor()

	assert.NotNil(t, proc)
	assert.Equal(t, "python", proc.Language())
	assert.Equal(t, "1.0.0", proc.Version())
	assert.Equal(t, []string{".py", ".pyw", ".pyi"}, proc.SupportedExtensions())
}

func TestProcessorCanProcess(t *testing.T) {
	proc := NewProcessor()

	tests := []struct {
		filename string
		expected bool
	}{
		{"main.py", true},
		{"script.pyw", true},
		{"types.pyi", true},
		{"test.PY", true}, // Case insensitive
		{"main.go", false},
		{"script.js", false},
		{"README.md", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			result := proc.CanProcess(tt.filename)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessMock(t *testing.T) {
	proc := NewProcessor()

	ctx := context.Background()
	source := `# Example Python file
from typing import List, Dict, Optional

class ExampleClass:
    """A simple example class."""
    
    def example_method(self, arg1: str, arg2: int) -> str:
        """Process the arguments."""
        return f"{arg1}: {arg2}"

def process_data(data: List[Dict]) -> Dict:
    """Process a list of dictionaries."""
    result = {}
    for item in data:
        result.update(item)
    return result

def _private_function():
    """This is a private function."""
    pass
`

	reader := strings.NewReader(source)
	file, _ := proc.Process(ctx, reader, "example.py")

	assert.NotNil(t, file)
	assert.Equal(t, "example.py", file.Path)
	assert.Equal(t, "python", file.Language)
	assert.NotEmpty(t, file.Children)
}

func TestProcessWithOptions(t *testing.T) {
	proc := NewProcessor()

	ctx := context.Background()
	source := `from typing import List

class MyClass:
    def public_method(self):
        pass
    
    def _private_method(self):
        pass

def public_function():
    # Implementation
    pass
`

	tests := []struct {
		name      string
		opts      processor.ProcessOptions
		checkFunc func(t *testing.T, file interface{})
	}{
		{
			name: "ExcludeComments",
			opts: processor.ProcessOptions{
				IncludeComments: false,
				IncludeImports:  true,
				IncludePrivate:  true,
			},
			checkFunc: func(t *testing.T, file interface{}) {
				// Should not have comments
				// Note: In mock implementation, we'd verify this
			},
		},
		{
			name: "ExcludeImports",
			opts: processor.ProcessOptions{
				IncludeComments: true,
				IncludeImports:  false,
				IncludePrivate:  true,
			},
			checkFunc: func(t *testing.T, file interface{}) {
				// Should not have imports
			},
		},
		{
			name: "ExcludePrivate",
			opts: processor.ProcessOptions{
				IncludeComments: true,
				IncludeImports:  true,
				IncludePrivate:  false,
			},
			checkFunc: func(t *testing.T, file interface{}) {
				// Should not have private methods/functions
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(source)
			file, _ := proc.ProcessWithOptions(ctx, reader, "test.py", tt.opts)

			assert.NotNil(t, file)

			if tt.checkFunc != nil {
				tt.checkFunc(t, file)
			}
		})
	}
}

func TestCountLines(t *testing.T) {
	tests := []struct {
		source string
		lines  int
	}{
		{"", 0},
		{"single line", 1},
		{"line1\nline2", 2},
		{"line1\nline2\n", 3},
		{"line1\nline2\nline3", 3},
	}

	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			result := countLines([]byte(tt.source))
			assert.Equal(t, tt.lines, result)
		})
	}
}

func TestIsPrivate(t *testing.T) {
	tests := []struct {
		name     string
		expected bool
	}{
		{"public_function", false},
		{"_private_function", true},
		{"__dunder_method__", false},
		{"", false},
		{"normal_name", false},
		{"_", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isPrivate(tt.name)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestInitializeWASM(t *testing.T) {
	proc := NewProcessor()

	ctx := context.Background()

	// Test with empty WASM bytes (will fail but tests the flow)
	err := proc.InitializeWASM(ctx, []byte{})
	assert.Error(t, err) // Empty WASM should fail

	// Test that runtime was created
	assert.NotNil(t, proc.wasmRuntime)
}

func TestProcessorImplementsInterface(t *testing.T) {
	proc := NewProcessor()

	// Verify it implements LanguageProcessor interface
	var _ processor.LanguageProcessor = proc
}

func TestReadError(t *testing.T) {
	proc := NewProcessor()

	ctx := context.Background()

	// Create a reader that always fails
	reader := &failingReader{}

	_, err := proc.Process(ctx, reader, "test.py")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read source")
}

// failingReader is a reader that always returns an error
type failingReader struct{}

func (r *failingReader) Read(p []byte) (n int, err error) {
	return 0, bytes.ErrTooLarge
}

func TestPython_AnnotatedClassAttributes(t *testing.T) {
	src := "from dataclasses import dataclass\n\n@dataclass\nclass User:\n    id: int\n    name: str = \"x\"\n    plain = 1\n    tags: list[str] = field(default_factory=list)\n"
	tsp, err := NewNativeTreeSitterProcessor()
	if err != nil {
		t.Fatal(err)
	}
	defer tsp.parser.Close()
	file, err := tsp.ProcessSource(context.Background(), []byte(src), "user.py") // raw IR, no stripper
	if err != nil {
		t.Fatal(err)
	}
	var cls *ir.DistilledClass
	for _, c := range file.Children {
		if k, ok := c.(*ir.DistilledClass); ok && k.Name == "User" {
			cls = k
		}
	}
	if cls == nil {
		t.Fatal("class User not found")
	}
	got := map[string][2]string{}
	for _, c := range cls.Children {
		if f, ok := c.(*ir.DistilledField); ok {
			typ := ""
			if f.Type != nil {
				typ = f.Type.Name
			}
			got[f.Name] = [2]string{typ, f.DefaultValue}
		}
	}
	assert.Equal(t, map[string][2]string{
		"id":    {"int", ""},
		"name":  {"str", `"x"`},
		"plain": {"", "1"},
		"tags":  {"list[str]", "field(default_factory=list)"},
	}, got)
}
