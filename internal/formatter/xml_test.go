package formatter

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXMLFormatter_Format(t *testing.T) {
	tests := []struct {
		name        string
		options     Options
		file        *ir.DistilledFile
		contains    []string
		notContains []string
		checkXML    func(t *testing.T, content string)
	}{
		{
			name: "basic file with class and function",
			options: Options{
				IncludeLocation: false,
			},
			file: &ir.DistilledFile{
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
				`<?xml version="1.0" encoding="UTF-8"?>`,
				`<distilled>`,
				`</distilled>`,
				`<file path="example.py" language="python" version="3.9">`,
				`<class name="TestClass" visibility="public">`,
				`<function name="__init__" visibility="public">`,
				`<parameter name="self"/>`,
				`<parameter name="value" type="int"/>`,
			},
			notContains: []string{
				`line=`, // No location attributes
			},
		},
		{
			name: "with location info",
			options: Options{
				IncludeLocation: true,
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
							Location: ir.Location{
								StartLine:   5,
								EndLine:     10,
								StartColumn: 1,
								EndColumn:   20,
							},
						},
						Name: "main",
					},
				},
			},
			contains: []string{
				`<file path="test.go" language="go" line="1" endLine="100">`,
				`<function name="main" line="5" endLine="10" column="1" endColumn="20"/>`,
			},
		},
		{
			name:    "with errors",
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
				`<errors>`,
				`<error severity="error" code="E001">Syntax error</error>`,
				`</errors>`,
			},
		},
		{
			name: "with metadata",
			options: Options{
				IncludeMetadata: true,
			},
			file: &ir.DistilledFile{
				Path:     "meta.py",
				Language: "python",
				Metadata: &ir.FileMetadata{
					Size:         1234,
					Hash:         "abc123",
					Encoding:     "utf-8",
					LastModified: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
				},
			},
			contains: []string{
				`<metadata size="1234" hash="abc123" encoding="utf-8" modified="2024-01-15T10:30:00Z"/>`,
			},
		},
		{
			name:    "imports with symbols",
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
				`<import type="from" module="os.path">`,
				`<symbol name="join"/>`,
				`<symbol name="dirname" alias="dir"/>`,
				`</import>`,
				`<import type="import" module="sys"/>`,
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
						Implementation: "def calculate():\n    return 42",
						Returns:        &ir.TypeRef{Name: "int"},
					},
				},
			},
			contains: []string{
				`<function name="calculate" returns="int">`,
				`<implementation><![CDATA[`,
				`def calculate():`,
				`    return 42`,
				`]]></implementation>`,
			},
		},
		{
			name:    "modifiers and visibility",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "modifiers.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledClass{
						Name:       "AbstractBase",
						Visibility: ir.VisibilityProtected,
						Modifiers:  []ir.Modifier{ir.ModifierAbstract, ir.ModifierFinal},
					},
					&ir.DistilledField{
						Name:       "count",
						Visibility: ir.VisibilityPrivate,
						Modifiers:  []ir.Modifier{ir.ModifierStatic},
						Type:       &ir.TypeRef{Name: "int"},
					},
				},
			},
			contains: []string{
				`<class name="AbstractBase" visibility="protected" modifiers="abstract final"/>`,
				`<field name="count" visibility="private" modifiers="static" type="int"/>`,
			},
		},
		{
			name:    "escaped XML content",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "escape.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledComment{
						Format: "line",
						Text:   "This has <xml> & \"quotes\"",
					},
					&ir.DistilledField{
						Name:         "html",
						DefaultValue: "<div>Test</div>",
					},
				},
			},
			checkXML: func(t *testing.T, content string) {
				// Check CDATA section for comment
				assert.Contains(t, content, `<comment format="line"><![CDATA[This has <xml> & "quotes"]]></comment>`)
				// Check escaped attribute
				assert.Contains(t, content, `default="&lt;div&gt;Test&lt;/div&gt;"`)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewXMLFormatter(tt.options)
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.file)
			require.NoError(t, err)

			output := buf.String()

			// Check it's valid XML
			var doc struct {
				XMLName xml.Name `xml:"distilled"`
			}
			err = xml.Unmarshal([]byte(output), &doc)
			assert.NoError(t, err, "Output should be valid XML")

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected, "Output should contain: %s", expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected, "Output should not contain: %s", notExpected)
			}

			if tt.checkXML != nil {
				tt.checkXML(t, output)
			}
		})
	}
}

func TestXMLFormatter_FormatMultiple(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

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

	// Check structure
	assert.Contains(t, output, `<?xml version="1.0" encoding="UTF-8"?>`)
	assert.Contains(t, output, `<distilled>`)
	assert.Contains(t, output, `</distilled>`)

	// Check both files are present
	assert.Contains(t, output, `<file path="file1.py"`)
	assert.Contains(t, output, `<file path="file2.py"`)
	assert.Contains(t, output, `<function name="func1"`)
	assert.Contains(t, output, `<function name="func2"`)

	// Count file elements
	fileCount := strings.Count(output, "<file ")
	assert.Equal(t, 2, fileCount)
}

func TestXMLFormatter_Extension(t *testing.T) {
	formatter := NewXMLFormatter(Options{})
	assert.Equal(t, ".xml", formatter.Extension())
}

func TestXMLFormatter_AllNodeTypes(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "all_types.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledPackage{Name: "mypackage"},
			&ir.DistilledInterface{Name: "IService"},
			&ir.DistilledStruct{Name: "Config"},
			&ir.DistilledEnum{Name: "Status"},
			&ir.DistilledTypeAlias{
				Name:       "ID",
				Type:       ir.TypeRef{Name: "string"},
				Visibility: ir.VisibilityPublic,
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()

	// Check all node types are present
	assert.Contains(t, output, `<package name="mypackage"/>`)
	assert.Contains(t, output, `<interface name="IService"/>`)
	assert.Contains(t, output, `<struct name="Config"/>`)
	assert.Contains(t, output, `<enum name="Status"/>`)
	assert.Contains(t, output, `<type_alias name="ID" visibility="public" type="string"/>`)
}

func TestXMLFormatter_NestedStructures(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

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

	// Check nesting structure
	assert.Contains(t, output, `<class name="OuterClass">`)
	assert.Contains(t, output, `<class name="InnerClass">`)
	assert.Contains(t, output, `<function name="method"/>`)
	assert.Contains(t, output, `</class>`) // Closing tags

	// Verify proper nesting by checking order
	outerStart := strings.Index(output, `<class name="OuterClass">`)
	innerStart := strings.Index(output, `<class name="InnerClass">`)
	methodPos := strings.Index(output, `<function name="method"/>`)
	outerEnd := strings.LastIndex(output, `</class>`)

	assert.True(t, outerStart < innerStart)
	assert.True(t, innerStart < methodPos)
	assert.True(t, methodPos < outerEnd)
}

func TestXMLFormatter_Parameters(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "params.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledFunction{
				Name: "complex_func",
				Parameters: []ir.Parameter{
					{Name: "required", Type: ir.TypeRef{Name: "str"}},
					{Name: "optional", Type: ir.TypeRef{Name: "int"}, IsOptional: true},
					{Name: "default", Type: ir.TypeRef{Name: "bool"}, DefaultValue: "True"},
					{Name: "args", IsVariadic: true},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()

	// Check parameters section
	assert.Contains(t, output, `<parameters>`)
	assert.Contains(t, output, `<parameter name="required" type="str"/>`)
	assert.Contains(t, output, `<parameter name="optional" type="int" optional="true"/>`)
	assert.Contains(t, output, `<parameter name="default" type="bool" default="True"/>`)
	assert.Contains(t, output, `<parameter name="args" variadic="true"/>`)
	assert.Contains(t, output, `</parameters>`)
}

func TestXMLFormatter_EmptyElements(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "empty.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledClass{
				Name:     "EmptyClass",
				Children: []ir.DistilledNode{}, // Empty children
			},
			&ir.DistilledFunction{
				Name:       "no_params",
				Parameters: []ir.Parameter{}, // Empty parameters
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()

	// Empty class should be self-closing
	assert.Contains(t, output, `<class name="EmptyClass"/>`)
	assert.NotContains(t, output, `<class name="EmptyClass">`)

	// Function with no parameters should be self-closing
	assert.Contains(t, output, `<function name="no_params"/>`)
	assert.NotContains(t, output, `<parameters>`)
}

func TestXMLFormatter_SpecialCharacters(t *testing.T) {
	formatter := NewXMLFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "special.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledFunction{
				Name: "test<>&\"'",
			},
			&ir.DistilledField{
				Name:         "field",
				DefaultValue: `"quoted" & <tagged>`,
			},
			&ir.DistilledError{
				Severity: "error",
				Message:  "Error with <xml> & special chars",
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	output := buf.String()

	// Check escaping in attributes
	// XML escaping may use numeric entities for quotes
	assert.Contains(t, output, `name="test&lt;&gt;&amp;`)
	assert.Contains(t, output, `default="`)

	// Check escaping in text content
	assert.Contains(t, output, `>Error with &lt;xml&gt; &amp; special chars</error>`)
}

func TestXML_CDATAEndSequenceInImplementation(t *testing.T) {
	file := &ir.DistilledFile{Path: "a.js", Language: "javascript", Children: []ir.DistilledNode{
		&ir.DistilledFunction{Name: "f", Visibility: ir.VisibilityPublic, Implementation: "{ return a[b[0]]>c; }"},
		&ir.DistilledComment{Text: "x ]]> y", Format: "line"},
	}}
	var b strings.Builder
	require.NoError(t, NewXMLFormatter(Options{}).Format(&b, file))
	dec := xml.NewDecoder(strings.NewReader(b.String()))
	var text strings.Builder
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		require.NoError(t, err, "output is not well-formed XML:\n%s", b.String())
		if cd, ok := tok.(xml.CharData); ok {
			text.Write(cd)
		}
	}
	assert.Contains(t, text.String(), "a[b[0]]>c")
	assert.Contains(t, text.String(), "x ]]> y")
}
