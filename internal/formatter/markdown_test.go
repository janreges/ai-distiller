package formatter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarkdownFormatter_Format(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		file        *ir.DistilledFile
		contains    []string
		notContains []string
	}{
		{
			name: "basic file with class and function",
			options: Options{
				IncludeLocation: false,
				IncludeMetadata: false,
			},
			file: &ir.DistilledFile{
				BaseNode: ir.BaseNode{
					Location: ir.Location{StartLine: 1, EndLine: 100},
				},
				Path:     "example.py",
				Language: "python",
				Version:  "3.9",
				Children: []ir.DistilledNode{
					&ir.DistilledClass{
						BaseNode: ir.BaseNode{
							Location: ir.Location{StartLine: 10, EndLine: 50},
						},
						Name:       "TestClass",
						Visibility: ir.VisibilityPublic,
						Children: []ir.DistilledNode{
							&ir.DistilledFunction{
								BaseNode: ir.BaseNode{
									Location: ir.Location{StartLine: 15, EndLine: 20},
								},
								Name:       "__init__",
								Visibility: ir.VisibilityPublic,
								Parameters: []ir.Parameter{
									{Name: "self"},
									{Name: "value", Type: ir.TypeRef{Name: "int"}},
								},
							},
						},
					},
				},
			},
			contains: []string{
				"### example.py",
				"```python",
				"class TestClass:",
				"__init__(self, value: int)",
			},
			notContains: []string{
				"<sub>L",       // No location info
				"Language:",    // No metadata
				"🏛️",           // No emojis
				"🔧",            // No emojis
				"## Structure", // No Structure heading
			},
		},
		{
			name: "file with location info",
			options: Options{
				IncludeLocation: true,
				IncludeMetadata: false,
			},
			file: &ir.DistilledFile{
				BaseNode: ir.BaseNode{
					Location: ir.Location{StartLine: 1, EndLine: 100},
				},
				Path:     "test.go",
				Language: "go",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						BaseNode: ir.BaseNode{
							Location: ir.Location{StartLine: 5, EndLine: 10},
						},
						Name:       "main",
						Visibility: ir.VisibilityPublic,
					},
				},
			},
			contains: []string{
				"### test.go",
				"```go",
				"func main()",
			},
		},
		{
			name:    "file with errors",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "error.py",
				Language: "python",
				Errors: []ir.DistilledError{
					{
						BaseNode: ir.BaseNode{
							Location: ir.Location{StartLine: 15},
						},
						Severity: "error",
						Message:  "Syntax error",
						Code:     "E001",
					},
				},
			},
			contains: []string{
				"### error.py",
				"```python",
			},
			notContains: []string{
				"⚠️",           // No emojis
				"❌",            // No emojis
				"## ⚠️ Errors", // No error section
			},
		},
		{
			name:    "imports formatting",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "imports.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledImport{
						ImportType: "from",
						Module:     "os.path",
						Symbols: []ir.ImportedSymbol{
							{Name: "join"},
							{Name: "dirname", Alias: "dir"},
						},
					},
					&ir.DistilledImport{
						ImportType: "import",
						Module:     "sys",
					},
				},
			},
			contains: []string{
				"### imports.py",
				"```python",
				"from os.path import join, dirname as dir",
				"import sys",
			},
			notContains: []string{
				"📥", // No emojis
			},
		},
		{
			name:    "modifiers and visibility",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "modifiers.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						Name:       "process",
						Visibility: ir.VisibilityPrivate,
						Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierAsync},
					},
				},
			},
			contains: []string{
				"### modifiers.py",
				"```python",
				"-async process()", // Python formatter outputs visibility prefix + async + name
			},
			notContains: []string{
				"🔧",         // No emojis
				"_private_", // No visibility labels
				"_static_",  // No modifier labels
				"_async_",   // No modifier labels
			},
		},
		{
			name:    "class inheritance",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "inheritance.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledClass{
						Name:       "Child",
						Visibility: ir.VisibilityPublic,
						Extends:    []ir.TypeRef{{Name: "Parent"}},
						Implements: []ir.TypeRef{{Name: "Interface1"}, {Name: "Interface2"}},
					},
				},
			},
			contains: []string{
				"### inheritance.py",
				"```python",
				"class Child(Parent):", // Python formatter handles inheritance
			},
			notContains: []string{
				"🏛️",         // No emojis
				"implements", // Python doesn't have explicit implements
			},
		},
		{
			name: "function with implementation",
			options: Options{
				Compact: false,
			},
			file: &ir.DistilledFile{
				Path:     "impl.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						Name:           "calculate",
						Visibility:     ir.VisibilityPublic,
						Implementation: "def calculate():\n    return 42",
					},
				},
			},
			contains: []string{
				"### impl.py",
				"```python",
				"def calculate():",
				"    return 42",
				"```",
			},
		},
		{
			name: "compact mode hides implementation",
			options: Options{
				Compact: true,
			},
			file: &ir.DistilledFile{
				Path:     "impl.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						Name:       "calculate",
						Visibility: ir.VisibilityPublic,
						// In compact mode, implementation should already be stripped by processor
						Implementation: "", // No implementation in compact mode
					},
				},
			},
			contains: []string{
				"### impl.py",
				"```python",
				"calculate()", // Without implementation, no def keyword
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewMarkdownFormatter(tt.options)
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.file)
			require.NoError(t, err)

			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected, "Output should not contain: %s", notExpected)
			}
		})
	}
}

func TestMarkdownFormatter_FormatMultiple(t *testing.T) {
	formatter := NewMarkdownFormatter(Options{})

	files := []*ir.DistilledFile{
		{
			Path:     "file1.py",
			Language: "python",
			Children: []ir.DistilledNode{
				&ir.DistilledFunction{Name: "func1"},
			},
		},
		{
			Path:     "file2.py",
			Language: "python",
			Children: []ir.DistilledNode{
				&ir.DistilledFunction{Name: "func2"},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.FormatMultiple(&buf, files)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "### file1.py")
	assert.Contains(t, output, "### file2.py")
	assert.Contains(t, output, "func1")
	assert.Contains(t, output, "func2")
}

func TestMarkdownFormatter_Extension(t *testing.T) {
	formatter := NewMarkdownFormatter(Options{})
	assert.Equal(t, ".md", formatter.Extension())
}

func TestMarkdownFormatter_AllNodeTypes(t *testing.T) {
	formatter := NewMarkdownFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "all_types.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledPackage{Name: "mypackage"},
			&ir.DistilledInterface{Name: "IService"},
			&ir.DistilledStruct{Name: "Config"},
			&ir.DistilledEnum{Name: "Status"},
			&ir.DistilledField{
				Name:         "count",
				Type:         &ir.TypeRef{Name: "int"},
				DefaultValue: "0",
			},
			&ir.DistilledTypeAlias{
				Name: "ID",
				Type: ir.TypeRef{Name: "string"},
			},
			&ir.DistilledComment{
				Format: "doc",
				Text:   "This is a documentation comment",
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "### all_types.py")
	assert.Contains(t, output, "```python")
	// The actual content depends on how the python formatter handles these node types
	// but we shouldn't expect emojis or labels
	assert.NotContains(t, output, "📦")
	assert.NotContains(t, output, "🔌")
	assert.NotContains(t, output, "📐")
	assert.NotContains(t, output, "🎲")
	assert.NotContains(t, output, "📊")
	assert.NotContains(t, output, "🏷️")
	assert.NotContains(t, output, "💬")
}

func TestMarkdownFormatter_Metadata(t *testing.T) {
	formatter := NewMarkdownFormatter(Options{
		IncludeMetadata: true,
	})

	file := &ir.DistilledFile{
		Path:     "meta.py",
		Language: "python",
		Metadata: &ir.FileMetadata{
			Size:     1234,
			Hash:     "abc123",
			Encoding: "utf-8",
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "### meta.py")
	assert.Contains(t, output, "```python")
	// Metadata is not shown in the new format
	assert.NotContains(t, output, "**Language:**")
	assert.NotContains(t, output, "**Size:**")
}

func TestMarkdownFormatter_NestedStructures(t *testing.T) {
	formatter := NewMarkdownFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "nested.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledClass{
				Name: "OuterClass",
				Children: []ir.DistilledNode{
					&ir.DistilledClass{
						Name: "InnerClass",
						Children: []ir.DistilledNode{
							&ir.DistilledFunction{
								Name: "method",
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "### nested.py")
	assert.Contains(t, output, "```python")
	// Check that nested structures are present
	assert.Contains(t, output, "OuterClass")
	assert.Contains(t, output, "InnerClass")
	assert.Contains(t, output, "method")
}

func TestMarkdown_UnescapesFilePath(t *testing.T) {
	var b strings.Builder
	file := &ir.DistilledFile{Path: `a"b<c>.py`, Language: "python"}
	require.NoError(t, NewMarkdownFormatter(Options{}).Format(&b, file))
	assert.Contains(t, b.String(), `### a"b<c>.py`)
}
