package python

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parsePy returns the raw IR of src, before the stripper.
func parsePy(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	tsp, err := NewNativeTreeSitterProcessor()
	require.NoError(t, err)
	defer tsp.parser.Close()
	file, err := tsp.ProcessSource(context.Background(), []byte(src), "test.py")
	require.NoError(t, err)
	return file
}

// findPyClass returns the top-level class called name; the test fails when
// there is none.
func findPyClass(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledClass {
	t.Helper()
	for _, c := range file.Children {
		if cls, ok := c.(*ir.DistilledClass); ok && cls.Name == name {
			return cls
		}
	}
	t.Fatalf("class %s not found", name)
	return nil
}

// distillPy runs src through the processor and the text formatter with the
// CLI defaults (public members, no bodies, no comments, docstrings), changed
// by set when it is not nil.
func distillPy(t *testing.T, src string, set func(*processor.ProcessOptions)) string {
	t.Helper()
	opts := processor.DefaultProcessOptions()
	opts.IncludePrivate = false
	opts.IncludeImplementation = false
	opts.IncludeComments = false
	if set != nil {
		set(&opts)
	}
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.py", opts)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, formatter.NewLanguageAwareTextFormatter(formatter.Options{}).Format(&b, file))
	return b.String()
}

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

func TestPython_DocstringsAreDocComments(t *testing.T) {
	file := parsePy(t, "\"\"\"Module doc.\"\"\"\n\nclass A:\n    \"\"\"Class doc.\"\"\"\n\n    def m(self):\n        \"\"\"Method doc.\"\"\"\n        return 1\n")
	mod := file.Children[0].(*ir.DistilledComment)
	assert.True(t, mod.IsDoc())
	assert.Equal(t, "Module doc.", mod.Text)

	cls := findPyClass(t, file, "A")
	assert.Equal(t, "Class doc.", cls.Children[0].(*ir.DistilledComment).Text)
	assert.True(t, cls.Children[0].(*ir.DistilledComment).IsDoc())

	mdoc := cls.Children[1].(*ir.DistilledComment)
	assert.Equal(t, "Method doc.", mdoc.Text)
	assert.True(t, mdoc.IsDoc())
	m := cls.Children[2].(*ir.DistilledFunction)
	assert.Equal(t, "m", m.Name)
	assert.NotContains(t, m.Implementation, "Method doc.")
	assert.Contains(t, m.Implementation, "return 1")
}

// Module and class docstrings document their container; a function docstring
// documents the function and sits right before it.
func TestPython_DocstringPlacement(t *testing.T) {
	file := parsePy(t, "\"\"\"Module doc.\"\"\"\n\nclass A:\n    \"\"\"Class doc.\"\"\"\n\n    def m(self):\n        \"\"\"Method doc.\"\"\"\n        self.x = 1\n")
	assert.True(t, file.Children[0].(*ir.DistilledComment).Inner, "module docstring")
	cls := findPyClass(t, file, "A")
	assert.True(t, cls.Children[0].(*ir.DistilledComment).Inner, "class docstring")
	var labels []string
	for _, c := range cls.Children[1:] {
		switch n := c.(type) {
		case *ir.DistilledComment:
			labels = append(labels, "doc:"+n.Text)
			assert.False(t, n.Inner, "a function docstring is not an inner doc")
		case *ir.DistilledField:
			labels = append(labels, "field:"+n.Name)
		case *ir.DistilledFunction:
			labels = append(labels, "fn:"+n.Name)
		}
	}
	assert.Equal(t, []string{"field:x", "doc:Method doc.", "fn:m"}, labels, "the docstring comes directly before its function")
}

// Decorators reach their function although the fields set in its body and
// its docstring are added before it.
func TestPython_DecoratedMethodKeepsDecorators(t *testing.T) {
	cls := findPyClass(t, parsePy(t, "class A:\n    @property\n    def p(self):\n        \"\"\"P doc.\"\"\"\n        self._c = 1\n        return self._c\n"), "A")
	fn := cls.Children[len(cls.Children)-1].(*ir.DistilledFunction)
	assert.Equal(t, "p", fn.Name)
	assert.Equal(t, []string{"property"}, fn.Decorators)
}

// A multi-line docstring loses its quotes, the indentation its lines share
// and its blank edges; a string prefix is not part of the text.
func TestPython_DocstringText(t *testing.T) {
	file := parsePy(t, "def f():\n    r'''First line.\n\n    Args:\n        x: value\n    '''\n    return 1\n")
	d := file.Children[0].(*ir.DistilledComment)
	assert.Equal(t, "First line.\n\nArgs:\n    x: value", d.Text)
	assert.Equal(t, "return 1", file.Children[1].(*ir.DistilledFunction).Implementation)
}

// A function whose whole body is its docstring has an empty implementation.
func TestPython_DocstringOnlyBody(t *testing.T) {
	file := parsePy(t, "def f():\n    \"\"\"Only a doc.\"\"\"\n")
	require.Len(t, file.Children, 2)
	assert.Equal(t, "Only a doc.", file.Children[0].(*ir.DistilledComment).Text)
	assert.Equal(t, "", file.Children[1].(*ir.DistilledFunction).Implementation)
}

func TestPython_ClassDocstringSurvivesPrivateFirstMember(t *testing.T) {
	src := "class A:\n    \"\"\"Class doc.\"\"\"\n\n    def _x(self):\n        \"\"\"X doc.\"\"\"\n        return 1\n\n    def y(self):\n        return 2\n\n\nclass B:\n    \"\"\"B doc.\"\"\"\n    _x = 1\n"
	assert.Equal(t, "<file path=\"test.py\">\n\nclass A:\n    \"\"\"Class doc.\"\"\"\n    y(self)\n\nclass B:\n    \"\"\"B doc.\"\"\"\n</file>\n", distillPy(t, src, nil))
}

func TestPython_DocstringOnlyClassKeepsDocstring(t *testing.T) {
	assert.Equal(t, "<file path=\"test.py\">\n\nclass Empty:\n    \"\"\"Only a doc.\"\"\"\n</file>\n",
		distillPy(t, "class Empty:\n    \"\"\"Only a doc.\"\"\"\n", nil))
}

func TestPython_ModuleDocstringAfterShebangIsKept(t *testing.T) {
	src := "#!/usr/bin/env python\n# -*- coding: utf-8 -*-\n\"\"\"Module doc.\"\"\"\n\ndef _helper():\n    return 1\n\ndef api():\n    return 2\n"
	assert.Equal(t, "<file path=\"test.py\">\n\"\"\"Module doc.\"\"\"\napi()\n</file>\n", distillPy(t, src, nil))
}

func TestPython_PrivateFunctionDocstringLeavesWithIt(t *testing.T) {
	src := "def _helper():\n    \"\"\"Helper doc.\"\"\"\n    return 1\n\ndef api():\n    return 2\n\nclass C:\n    def _h(self):\n        \"\"\"H doc.\"\"\"\n\n    def run(self):\n        pass\n"
	assert.Equal(t, "<file path=\"test.py\">\napi()\n\nclass C:\n    run(self)\n</file>\n", distillPy(t, src, nil))
}

func TestPython_FunctionDocstringFollowsDocstringsFlag(t *testing.T) {
	src := "def f():\n    \"\"\"F doc.\n\n    More detail.\n    \"\"\"\n    x = 1\n    return x\n\nclass A:\n    def m(self):\n        \"\"\"M doc.\n\n        Detail.\n        \"\"\"\n        if True:\n            return 1\n        return 2\n"
	impl := func(o *processor.ProcessOptions) { o.IncludeImplementation = true }
	assert.Equal(t, `<file path="test.py">
def f():
    """F doc.

    More detail.
    """
    x = 1
    return x

class A:
    def m(self):
        """M doc.

        Detail.
        """
        if True:
            return 1
        return 2
</file>
`, distillPy(t, src, impl), "printed once, under the signature, before the body")

	assert.Equal(t, `<file path="test.py">
def f():
    x = 1
    return x

class A:
    def m(self):
        if True:
            return 1
        return 2
</file>
`, distillPy(t, src, func(o *processor.ProcessOptions) { impl(o); o.IncludeDocstrings = false }))

	assert.Equal(t, `<file path="test.py">
f()
    """F doc.

    More detail.
    """

class A:
    m(self)
        """M doc.

        Detail.
        """
</file>
`, distillPy(t, src, nil), "shown by default, under the signature")
}

// A function docstring prints under the signature, so it cannot pass for the
// docstring of the class or module the function is in.
func TestPython_FunctionDocstringIsNotTheContainerDocstring(t *testing.T) {
	src := "class P:\n    @property\n    def p(self):\n        \"\"\"P doc.\"\"\"\n        return 1\n"
	assert.Equal(t, `<file path="test.py">

class P:
    @property
    p(self)
        """P doc."""
</file>
`, distillPy(t, src, nil))
	assert.Equal(t, `<file path="test.py">

class P:
    @property
    def p(self):
        """P doc."""
        return 1
</file>
`, distillPy(t, src, func(o *processor.ProcessOptions) { o.IncludeImplementation = true }))

	src = "def run(x: int) -> int:\n    \"\"\"Run doc.\"\"\"\n    return x\n\nclass Q:\n    \"\"\"Class doc.\"\"\"\n    def run(self, x: int) -> int:\n        \"\"\"Method doc.\"\"\"\n        return x\n"
	assert.Equal(t, `<file path="test.py">
run(x: int) -> int
    """Run doc."""

class Q:
    """Class doc."""
    run(self, x: int) -> int
        """Method doc."""
</file>
`, distillPy(t, src, nil))
}

// A comment directly above a removed function goes with it, also above its
// decorators and when the function has a docstring; a comment separated by a
// blank line stays (spec H1). The parser's line numbers decide.
func TestPython_CommentDirectlyAboveRemovedFunctionLeavesWithIt(t *testing.T) {
	src := "# Helpers.\n\n# about _hidden\ndef _hidden():\n    \"\"\"Hidden doc.\"\"\"\n    pass\n# about _other\n@staticmethod\ndef _other():\n    pass\n# about api\ndef api(x):\n    pass\n"
	withComments := func(o *processor.ProcessOptions) { o.IncludeComments = true }
	assert.Equal(t, "<file path=\"test.py\">\n# Helpers.\n# about api\napi(x)\n</file>\n", distillPy(t, src, withComments))

	all := distillPy(t, src, func(o *processor.ProcessOptions) { o.IncludeComments = true; o.IncludePrivate = true })
	for _, line := range []string{"# about _hidden", "# about _other", "@staticmethod"} {
		assert.Contains(t, all, line, "with --private=1 everything stays")
	}
}
