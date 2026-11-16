package formatter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONStructuredFormatter_Format(t *testing.T) {
	tests := []struct {
		name    string
		options Options
		file    *ir.DistilledFile
		check   func(t *testing.T, data map[string]interface{})
	}{
		{
			name: "basic file with structured output",
			options: Options{
				IncludeLocation: false,
			},
			file: &ir.DistilledFile{
				Path:     "example.py",
				Language: "python",
				Version:  "3.9",
				Children: []ir.DistilledNode{
					&ir.DistilledPackage{Name: "mypackage"},
					&ir.DistilledImport{
						ImportType: "from",
						Module:     "os",
						Symbols:    []ir.ImportedSymbol{{Name: "path"}},
					},
					&ir.DistilledClass{
						Name:       "TestClass",
						Visibility: ir.VisibilityPublic,
						Children: []ir.DistilledNode{
							&ir.DistilledField{
								Name: "count",
								Type: &ir.TypeRef{Name: "int"},
							},
							&ir.DistilledFunction{
								Name:       "__init__",
								Visibility: ir.VisibilityPublic,
							},
						},
					},
					&ir.DistilledFunction{
						Name:       "standalone_func",
						Visibility: ir.VisibilityPublic,
					},
				},
			},
			check: func(t *testing.T, data map[string]interface{}) {
				assert.Equal(t, "file", data["type"])
				assert.Equal(t, "example.py", data["path"])
				assert.Equal(t, "python", data["language"])
				assert.Equal(t, "3.9", data["version"])

				// Check structure
				structure := data["structure"].(map[string]interface{})

				// Packages
				packages := structure["packages"].([]interface{})
				assert.Len(t, packages, 1)
				pkg := packages[0].(map[string]interface{})
				assert.Equal(t, "mypackage", pkg["name"])

				// Imports
				imports := structure["imports"].([]interface{})
				assert.Len(t, imports, 1)
				imp := imports[0].(map[string]interface{})
				assert.Equal(t, "from", imp["type"])
				assert.Equal(t, "os", imp["module"])

				// Classes
				classes := structure["classes"].([]interface{})
				assert.Len(t, classes, 1)
				class := classes[0].(map[string]interface{})
				assert.Equal(t, "TestClass", class["name"])

				// Class members
				members := class["members"].(map[string]interface{})
				fields := members["fields"].([]interface{})
				assert.Len(t, fields, 1)
				methods := members["methods"].([]interface{})
				assert.Len(t, methods, 1)

				// Functions
				functions := structure["functions"].([]interface{})
				assert.Len(t, functions, 1)
				fn := functions[0].(map[string]interface{})
				assert.Equal(t, "standalone_func", fn["name"])

				// Stats
				stats := data["stats"].(map[string]interface{})
				assert.Equal(t, float64(1), stats["package"])
				assert.Equal(t, float64(1), stats["import"])
				assert.Equal(t, float64(1), stats["class"])
				assert.Equal(t, float64(2), stats["function"])
				assert.Equal(t, float64(1), stats["field"])
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
							Location: ir.Location{StartLine: 5, EndLine: 10},
						},
						Name: "main",
					},
				},
			},
			check: func(t *testing.T, data map[string]interface{}) {
				// File location
				fileLoc := data["location"].(map[string]interface{})
				assert.Equal(t, float64(1), fileLoc["start_line"])
				assert.Equal(t, float64(100), fileLoc["end_line"])

				// Function location
				structure := data["structure"].(map[string]interface{})
				functions := structure["functions"].([]interface{})
				fn := functions[0].(map[string]interface{})
				fnLoc := fn["location"].(map[string]interface{})
				assert.Equal(t, float64(5), fnLoc["start_line"])
				assert.Equal(t, float64(10), fnLoc["end_line"])
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
			check: func(t *testing.T, data map[string]interface{}) {
				// Metadata
				metadata := data["metadata"].(map[string]interface{})
				assert.Equal(t, float64(1234), metadata["size_bytes"])
				assert.Equal(t, "abc123", metadata["hash"])

				// Errors
				errors := data["errors"].([]interface{})
				assert.Len(t, errors, 1)
				err0 := errors[0].(map[string]interface{})
				assert.Equal(t, "error", err0["severity"])
				assert.Equal(t, "Syntax error", err0["message"])
				assert.Equal(t, "E001", err0["code"])
			},
		},
		{
			name: "compact mode",
			options: Options{
				Compact: true,
			},
			file: &ir.DistilledFile{
				Path:     "compact.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledFunction{
						Name:           "calculate",
						Implementation: "def calculate():\n    return 42",
					},
				},
			},
			check: func(t *testing.T, data map[string]interface{}) {
				structure := data["structure"].(map[string]interface{})
				functions := structure["functions"].([]interface{})
				fn := functions[0].(map[string]interface{})
				assert.Equal(t, true, fn["has_implementation"])
				assert.Nil(t, fn["implementation"])
			},
		},
		{
			name:    "complex class hierarchy",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "hierarchy.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledClass{
						Name:       "BaseClass",
						Visibility: ir.VisibilityPublic,
					},
					&ir.DistilledClass{
						Name:       "DerivedClass",
						Visibility: ir.VisibilityPublic,
						Extends:    []ir.TypeRef{{Name: "BaseClass"}},
						Implements: []ir.TypeRef{{Name: "Interface1"}, {Name: "Interface2"}},
						Modifiers:  []ir.Modifier{ir.ModifierAbstract},
					},
					&ir.DistilledInterface{
						Name:    "Interface1",
						Extends: []ir.TypeRef{{Name: "BaseInterface"}},
					},
				},
			},
			check: func(t *testing.T, data map[string]interface{}) {
				structure := data["structure"].(map[string]interface{})

				// Classes
				classes := structure["classes"].([]interface{})
				assert.Len(t, classes, 2)

				derived := classes[1].(map[string]interface{})
				assert.Equal(t, "DerivedClass", derived["name"])
				assert.Equal(t, []interface{}{"abstract"}, derived["modifiers"])
				assert.Equal(t, []interface{}{"BaseClass"}, derived["extends"])
				assert.Equal(t, []interface{}{"Interface1", "Interface2"}, derived["implements"])

				// Interfaces
				interfaces := structure["interfaces"].([]interface{})
				assert.Len(t, interfaces, 1)
				iface := interfaces[0].(map[string]interface{})
				assert.Equal(t, "Interface1", iface["name"])
				assert.Equal(t, []interface{}{"BaseInterface"}, iface["extends"])
			},
		},
		{
			name:    "all node types",
			options: Options{},
			file: &ir.DistilledFile{
				Path:     "all_types.py",
				Language: "python",
				Children: []ir.DistilledNode{
					&ir.DistilledPackage{Name: "pkg"},
					&ir.DistilledImport{Module: "sys"},
					&ir.DistilledClass{Name: "MyClass"},
					&ir.DistilledInterface{Name: "IService"},
					&ir.DistilledFunction{Name: "func"},
					&ir.DistilledField{Name: "field"},
					&ir.DistilledTypeAlias{
						Name: "ID",
						Type: ir.TypeRef{Name: "string"},
					},
				},
			},
			check: func(t *testing.T, data map[string]interface{}) {
				structure := data["structure"].(map[string]interface{})

				// Check all sections exist
				assert.NotNil(t, structure["packages"])
				assert.NotNil(t, structure["imports"])
				assert.NotNil(t, structure["classes"])
				assert.NotNil(t, structure["interfaces"])
				assert.NotNil(t, structure["functions"])
				assert.NotNil(t, structure["variables"])
				assert.NotNil(t, structure["types"])

				// Type aliases
				types := structure["types"].([]interface{})
				assert.Len(t, types, 1)
				ta := types[0].(map[string]interface{})
				assert.Equal(t, "ID", ta["name"])
				assert.Equal(t, "string", ta["type"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			formatter := NewJSONStructuredFormatter(tt.options)
			var buf bytes.Buffer

			err := formatter.Format(&buf, tt.file)
			require.NoError(t, err)

			var data map[string]interface{}
			err = json.Unmarshal(buf.Bytes(), &data)
			require.NoError(t, err, "Output should be valid JSON")

			tt.check(t, data)
		})
	}
}

func TestJSONStructuredFormatter_FormatMultiple(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})

	files := []*ir.DistilledFile{
		{
			Path:     "file1.py",
			Language: "python",
			Children: []ir.DistilledNode{
				&ir.DistilledClass{Name: "Class1"},
				&ir.DistilledFunction{Name: "func1"},
			},
		},
		{
			Path:     "file2.py",
			Language: "python",
			Children: []ir.DistilledNode{
				&ir.DistilledClass{Name: "Class2"},
				&ir.DistilledFunction{Name: "func2"},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.FormatMultiple(&buf, files)
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)

	// Check project structure
	assert.Equal(t, "project", data["type"])

	// Check files
	fileList := data["files"].([]interface{})
	assert.Len(t, fileList, 2)

	// Check total stats
	totalStats := data["total_stats"].(map[string]interface{})
	assert.Equal(t, float64(2), totalStats["class"])
	assert.Equal(t, float64(2), totalStats["function"])

	// Verify file contents
	file1 := fileList[0].(map[string]interface{})
	assert.Equal(t, "file1.py", file1["path"])

	file2 := fileList[1].(map[string]interface{})
	assert.Equal(t, "file2.py", file2["path"])
}

func TestJSONStructuredFormatter_Extension(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})
	assert.Equal(t, ".json", formatter.Extension())
}

func TestJSONStructuredFormatter_Parameters(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})

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

	var data map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)

	structure := data["structure"].(map[string]interface{})
	functions := structure["functions"].([]interface{})
	fn := functions[0].(map[string]interface{})

	assert.Equal(t, "dict", fn["returns"])

	params := fn["parameters"].([]interface{})
	assert.Len(t, params, 4)

	// Check parameter details
	p0 := params[0].(map[string]interface{})
	assert.Equal(t, "required", p0["name"])
	assert.Equal(t, "str", p0["type"])

	p1 := params[1].(map[string]interface{})
	assert.Equal(t, "optional", p1["name"])
	assert.Equal(t, true, p1["optional"])

	p2 := params[2].(map[string]interface{})
	assert.Equal(t, "default", p2["name"])
	assert.Equal(t, "True", p2["default"])

	p3 := params[3].(map[string]interface{})
	assert.Equal(t, "args", p3["name"])
	assert.Equal(t, true, p3["variadic"])
}

func TestJSONStructuredFormatter_EmptyStructure(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "empty.py",
		Language: "python",
		Children: []ir.DistilledNode{},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)

	// Should not have structure key if no children
	assert.Nil(t, data["structure"])

	// Should have empty stats
	stats, ok := data["stats"].(map[string]interface{})
	if ok {
		assert.Empty(t, stats)
	} else {
		// stats is nil or not present, which is also fine for empty files
		assert.Nil(t, data["stats"])
	}
}

func TestJSONStructuredFormatter_NestedClasses(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})

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
							&ir.DistilledField{
								Name: "inner_field",
								Type: &ir.TypeRef{Name: "str"},
							},
							&ir.DistilledFunction{
								Name: "inner_method",
							},
						},
					},
					&ir.DistilledField{
						Name: "outer_field",
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)

	// Outer class should contain inner class in its members
	structure := data["structure"].(map[string]interface{})
	classes := structure["classes"].([]interface{})
	assert.Len(t, classes, 1) // Only outer class at top level

	outerClass := classes[0].(map[string]interface{})
	assert.Equal(t, "OuterClass", outerClass["name"])

	// Check outer class members
	outerMembers := outerClass["members"].(map[string]interface{})
	outerFields := outerMembers["fields"].([]interface{})
	assert.Len(t, outerFields, 1)

	// Stats should count nested elements
	stats := data["stats"].(map[string]interface{})
	assert.Equal(t, float64(2), stats["class"])    // Outer and inner
	assert.Equal(t, float64(2), stats["field"])    // Both fields
	assert.Equal(t, float64(1), stats["function"]) // Inner method
}

func TestJSONStructuredFormatter_ImportSymbols(t *testing.T) {
	formatter := NewJSONStructuredFormatter(Options{})

	file := &ir.DistilledFile{
		Path:     "imports.py",
		Language: "python",
		Children: []ir.DistilledNode{
			&ir.DistilledImport{
				ImportType: "from",
				Module:     "collections",
				Symbols: []ir.ImportedSymbol{
					{Name: "defaultdict"},
					{Name: "OrderedDict", Alias: "ODict"},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := formatter.Format(&buf, file)
	require.NoError(t, err)

	var data map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &data)
	require.NoError(t, err)

	structure := data["structure"].(map[string]interface{})
	imports := structure["imports"].([]interface{})
	imp := imports[0].(map[string]interface{})

	symbols := imp["symbols"].([]interface{})
	assert.Len(t, symbols, 2)

	sym0 := symbols[0].(map[string]interface{})
	assert.Equal(t, "defaultdict", sym0["name"])
	assert.Nil(t, sym0["alias"])

	sym1 := symbols[1].(map[string]interface{})
	assert.Equal(t, "OrderedDict", sym1["name"])
	assert.Equal(t, "ODict", sym1["alias"])
}

func TestJSONStructuredFormatter_TypeAliasModifier(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, NewJSONStructuredFormatter(Options{}).Format(&buf, goTypeDeclsFile()))

	var out struct {
		Structure map[string][]map[string]interface{} `json:"structure"`
	}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	modifiers := map[string]interface{}{}
	for _, group := range []string{"types", "interfaces", "classes"} {
		for _, node := range out.Structure[group] {
			modifiers[node["name"].(string)] = node["modifiers"]
		}
	}
	assert.Len(t, modifiers, 4)
	assert.Nil(t, modifiers["ID"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Name"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Any"])
	assert.Equal(t, []interface{}{"type_alias"}, modifiers["Pair"])
}
