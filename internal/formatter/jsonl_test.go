package formatter

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONLFormatter_Format(t *testing.T) {
	tests := []struct {
		name       string
		options    Options
		file       *ir.DistilledFile
		lineCount  int
		checkLines []func(t *testing.T, line map[string]interface{})
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
			lineCount: 3, // file, class, function
			checkLines: []func(t *testing.T, line map[string]interface{}){
				// Line 0: file
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "file", line["type"])
					assert.Equal(t, "example.py", line["path"])
					assert.Equal(t, "python", line["language"])
					assert.Equal(t, "3.9", line["version"])
					stats := line["stats"].(map[string]interface{})
					assert.Equal(t, float64(1), stats["class"])
					assert.Equal(t, float64(1), stats["function"])
				},
				// Line 1: class
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "class", line["type"])
					assert.Equal(t, "TestClass", line["name"])
					assert.Equal(t, "example.py", line["file"])
					assert.Equal(t, []interface{}{}, line["path"])
					assert.Equal(t, "public", line["visibility"])
				},
				// Line 2: function
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "function", line["type"])
					assert.Equal(t, "__init__", line["name"])
					assert.Equal(t, "example.py", line["file"])
					assert.Equal(t, []interface{}{"TestClass"}, line["path"])
					params := line["parameters"].([]interface{})
					assert.Len(t, params, 2)
					param1 := params[0].(map[string]interface{})
					assert.Equal(t, "self", param1["name"])
					param2 := params[1].(map[string]interface{})
					assert.Equal(t, "value", param2["name"])
					assert.Equal(t, "int", param2["type"])
				},
			},
		},
		{
			name: "with location info",
			options: Options{
				IncludeLocation: true,
			},
			file: &ir.DistilledFile{
				Path:     "test.go",
				Language: "go",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						BaseNode: ir.BaseNode{
							Location: ir.Location{StartLine: 5, EndLine: 10, StartColumn: 1, EndColumn: 20},
						},
						Name: "main",
					},
				},
			},
			lineCount: 2,
			checkLines: []func(t *testing.T, line map[string]interface{}){
				nil, // Skip file line
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "function", line["type"])
					loc := line["location"].(map[string]interface{})
					assert.Equal(t, float64(5), loc["start_line"])
					assert.Equal(t, float64(10), loc["end_line"])
					assert.Equal(t, float64(1), loc["start_column"])
					assert.Equal(t, float64(20), loc["end_column"])
				},
			},
		},
		{
			name:    "imports and symbols",
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
				},
			},
			lineCount: 2,
			checkLines: []func(t *testing.T, line map[string]interface{}){
				nil, // Skip file line
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "import", line["type"])
					assert.Equal(t, "from", line["import_type"])
					assert.Equal(t, "os.path", line["module"])
					symbols := line["symbols"].([]interface{})
					assert.Len(t, symbols, 2)
					sym1 := symbols[0].(map[string]interface{})
					assert.Equal(t, "join", sym1["name"])
					sym2 := symbols[1].(map[string]interface{})
					assert.Equal(t, "dirname", sym2["name"])
					assert.Equal(t, "dir", sym2["alias"])
				},
			},
		},
		{
			name: "with metadata and errors",
			options: Options{
				IncludeMetadata: true,
			},
			file: &ir.DistilledFile{
				Path:     "meta.py",
				Language: "python",
				Metadata: &ir.FileMetadata{
					Size: 1234,
					Hash: "abc123",
				},
				Errors: []ir.DistilledError{
					{
						Severity: "error",
						Message:  "Syntax error",
						Code:     "E001",
					},
				},
			},
			lineCount: 1,
			checkLines: []func(t *testing.T, line map[string]interface{}){
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "file", line["type"])

					metadata := line["metadata"].(map[string]interface{})
					assert.Equal(t, float64(1234), metadata["size_bytes"])
					assert.Equal(t, "abc123", metadata["hash"])

					errors := line["errors"].([]interface{})
					assert.Len(t, errors, 1)
					err0 := errors[0].(map[string]interface{})
					assert.Equal(t, "error", err0["severity"])
					assert.Equal(t, "Syntax error", err0["message"])
					assert.Equal(t, "E001", err0["code"])
				},
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
					},
				},
			},
			lineCount: 2,
			checkLines: []func(t *testing.T, line map[string]interface{}){
				nil, // Skip file line
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "function", line["type"])
					assert.Equal(t, "def calculate():\n    return 42", line["implementation"])
				},
			},
		},
		{
			name: "compact mode",
			options: Options{
				Compact: true,
			},
			file: &ir.DistilledFile{
				Path:     "impl.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						Name:           "calculate",
						Implementation: "def calculate():\n    return 42",
					},
				},
			},
			lineCount: 2,
			checkLines: []func(t *testing.T, line map[string]interface{}){
				nil, // Skip file line
				func(t *testing.T, line map[string]interface{}) {
					assert.Equal(t, "function", line["type"])
					assert.Equal(t, true, line["has_implementation"])
					assert.Nil(t, line["implementation"])
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewJSONLFormatter(tt.options)
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.file)
			require.NoError(t, err)

			lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
			assert.Equal(t, tt.lineCount, len(lines), "Should have correct number of lines")

			for i, checkFunc := range tt.checkLines {
				if checkFunc != nil && i < len(lines) {
					var obj map[string]interface{}
					err := json.Unmarshal([]byte(lines[i]), &obj)
					require.NoError(t, err, "Line %d should be valid JSON", i)
					checkFunc(t, obj)
				}
			}
		})
	}
}

func TestJSONLFormatter_FormatMultiple(t *testing.T) {
	formatter := NewJSONLFormatter(Options{})

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

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Equal(t, 4, len(lines)) // 2 files + 2 functions

	// Check file lines
	var file1Line, file2Line map[string]interface{}
	for _, line := range lines {
		var obj map[string]interface{}
		err := json.Unmarshal([]byte(line), &obj)
		require.NoError(t, err)

		if obj["type"] == "file" {
			if obj["path"] == "file1.py" {
				file1Line = obj
			} else if obj["path"] == "file2.py" {
				file2Line = obj
			}
		}
	}

	assert.NotNil(t, file1Line, "Should have file1.py")
	assert.NotNil(t, file2Line, "Should have file2.py")
}

func TestJSONLFormatter_Extension(t *testing.T) {
	formatter := NewJSONLFormatter(Options{})
	assert.Equal(t, ".jsonl", formatter.Extension())
}

func TestJSONLFormatter_AllNodeTypes(t *testing.T) {
	formatter := NewJSONLFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "all_types.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledPackage{Name: "mypackage"},
			&ir.DistilledClass{
				Name:       "MyClass",
				Visibility: ir.VisibilityProtected,
				Modifiers:  []ir.Modifier{ir.ModifierAbstract},
				Extends:    []ir.TypeRef{{Name: "BaseClass"}},
				Implements: []ir.TypeRef{{Name: "Interface1"}},
			},
			&ir.DistilledInterface{
				Name:    "IService",
				Extends: []ir.TypeRef{{Name: "IBase"}},
			},
			&ir.DistilledField{
				Name:         "count",
				Type:         &ir.TypeRef{Name: "int"},
				DefaultValue: "0",
				Modifiers:    []ir.Modifier{ir.ModifierStatic},
			},
			&ir.DistilledTypeAlias{
				Name: "ID",
				Type: ir.TypeRef{Name: "string"},
			},
			&ir.DistilledComment{
				Format: "doc",
				Text:   "Documentation",
			},
			&ir.DistilledError{
				BaseNode: ir.BaseNode{
					Location: ir.Location{StartLine: 10},
				},
				Severity: "warning",
				Message:  "Unused import",
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	assert.Equal(t, 8, len(lines)) // 1 file + 7 nodes

	// Check various node types
	foundTypes := make(map[string]bool)
	for _, line := range lines[1:] { // Skip file line
		var obj map[string]interface{}
		err := json.Unmarshal([]byte(line), &obj)
		require.NoError(t, err)

		nodeType := obj["type"].(string)
		foundTypes[nodeType] = true

		switch nodeType {
		case "class":
			assert.Equal(t, "MyClass", obj["name"])
			assert.Equal(t, "protected", obj["visibility"])
			assert.Equal(t, []interface{}{"abstract"}, obj["modifiers"])
			assert.Equal(t, []interface{}{"BaseClass"}, obj["extends"])
			assert.Equal(t, []interface{}{"Interface1"}, obj["implements"])
		case "interface":
			assert.Equal(t, "IService", obj["name"])
			assert.Equal(t, []interface{}{"IBase"}, obj["extends"])
		case "field":
			assert.Equal(t, "count", obj["name"])
			assert.Equal(t, "int", obj["field_type"])
			assert.Equal(t, "0", obj["default"])
			assert.Equal(t, []interface{}{"static"}, obj["modifiers"])
		case "type_alias":
			assert.Equal(t, "ID", obj["name"])
			assert.Equal(t, "string", obj["alias_type"])
		case "comment":
			assert.Equal(t, "doc", obj["format"])
			assert.Equal(t, "Documentation", obj["text"])
		case "error":
			assert.Equal(t, "warning", obj["severity"])
			assert.Equal(t, "Unused import", obj["message"])
			assert.Equal(t, "error:10", obj["name"])
		}
	}

	assert.True(t, foundTypes["package"])
	assert.True(t, foundTypes["class"])
	assert.True(t, foundTypes["interface"])
	assert.True(t, foundTypes["field"])
	assert.True(t, foundTypes["type_alias"])
	assert.True(t, foundTypes["comment"])
	assert.True(t, foundTypes["error"])
}

func TestJSONLFormatter_NestedPaths(t *testing.T) {
	formatter := NewJSONLFormatter(Options{})

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
					&ir.DistilledFunction{
						Name: "outer_method",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")

	// Check paths
	for _, line := range lines[1:] { // Skip file line
		var obj map[string]interface{}
		err := json.Unmarshal([]byte(line), &obj)
		require.NoError(t, err)

		name := obj["name"].(string)
		path := obj["path"].([]interface{})

		switch name {
		case "OuterClass":
			assert.Equal(t, []interface{}{}, path)
		case "InnerClass":
			assert.Equal(t, []interface{}{"OuterClass"}, path)
		case "method":
			assert.Equal(t, []interface{}{"OuterClass", "InnerClass"}, path)
		case "outer_method":
			assert.Equal(t, []interface{}{"OuterClass"}, path)
		}
	}
}

func TestJSONLFormatter_Parameters(t *testing.T) {
	formatter := NewJSONLFormatter(Options{})

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
				Returns: &ir.TypeRef{Name: "dict"},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	var funcLine map[string]interface{}
	err = json.Unmarshal([]byte(lines[1]), &funcLine)
	require.NoError(t, err)

	assert.Equal(t, "function", funcLine["type"])
	assert.Equal(t, "dict", funcLine["returns"])

	params := funcLine["parameters"].([]interface{})
	assert.Len(t, params, 4)

	// Check each parameter
	p0 := params[0].(map[string]interface{})
	assert.Equal(t, "required", p0["name"])
	assert.Equal(t, "str", p0["type"])
	assert.Nil(t, p0["optional"])
	assert.Nil(t, p0["default"])

	p1 := params[1].(map[string]interface{})
	assert.Equal(t, "optional", p1["name"])
	assert.Equal(t, "int", p1["type"])
	assert.Equal(t, true, p1["optional"])

	p2 := params[2].(map[string]interface{})
	assert.Equal(t, "default", p2["name"])
	assert.Equal(t, "bool", p2["type"])
	assert.Equal(t, "True", p2["default"])

	p3 := params[3].(map[string]interface{})
	assert.Equal(t, "args", p3["name"])
	assert.Equal(t, true, p3["variadic"])
}

// goTypeDeclsFile holds Go type declarations that differ only in whether
// they are aliases (type_alias, written with "="): ID is a defined type, Name
// an alias, Any an alias of an interface literal, Pair of a struct literal.
func goTypeDeclsFile() *ir.DistilledFile {
	alias := []ir.Modifier{ir.ModifierTypeAlias}
	return &ir.DistilledFile{
		Path:     "types.go",
		Language: "go",
		Children: []ir.DistilledNode{
			&ir.DistilledTypeAlias{Name: "ID", Visibility: ir.VisibilityPublic, Type: ir.TypeRef{Name: "int"}},
			&ir.DistilledTypeAlias{Name: "Name", Visibility: ir.VisibilityPublic, Type: ir.TypeRef{Name: "string"}, Modifiers: alias},
			&ir.DistilledInterface{Name: "Any", Visibility: ir.VisibilityPublic, Modifiers: alias},
			&ir.DistilledClass{Name: "Pair", Visibility: ir.VisibilityPublic, Modifiers: alias},
		},
	}
}

func TestJSONLFormatter_TypeAliasModifier(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewJSONLFormatter(Options{}).Format(&buf, goTypeDeclsFile()))

	modifiers := map[string]interface{}{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n")[1:] {
		var obj map[string]interface{}
		require.NoError(t, json.Unmarshal([]byte(line), &obj))
		modifiers[obj["name"].(string)] = obj["modifiers"]
	}
	assert.Len(t, modifiers, 4)
	assert.Nil(t, modifiers["ID"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Name"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Any"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Pair"])
}
