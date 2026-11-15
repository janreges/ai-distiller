package ir

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocation(t *testing.T) {
	loc := Location{
		StartLine:   10,
		StartColumn: 5,
		EndLine:     15,
		EndColumn:   20,
		StartByte:   100,
		EndByte:     200,
	}

	assert.Equal(t, 10, loc.StartLine)
	assert.Equal(t, 5, loc.StartColumn)
	assert.Equal(t, 15, loc.EndLine)
	assert.Equal(t, 20, loc.EndColumn)
}

func TestSymbolRef(t *testing.T) {
	ref := SymbolRef{
		ID:        "TestFunc_10_5",
		Name:      "TestFunc",
		Package:   "main",
		IsBuiltin: false,
	}

	assert.Equal(t, SymbolID("TestFunc_10_5"), ref.ID)
	assert.Equal(t, "TestFunc", ref.Name)
	assert.Equal(t, "main", ref.Package)
	assert.False(t, ref.IsBuiltin)
}

func TestBaseNode(t *testing.T) {
	symbolID := SymbolID("test_symbol")
	node := BaseNode{
		Location: Location{
			StartLine: 1,
			EndLine:   5,
		},
		SymbolID: &symbolID,
		Extensions: &NodeExtensions{
			Go: &GoExtensions{
				IsMethod: true,
			},
		},
	}

	assert.Equal(t, 1, node.GetLocation().StartLine)
	assert.NotNil(t, node.GetSymbolID())
	assert.Equal(t, symbolID, *node.GetSymbolID())
	assert.True(t, node.Extensions.Go.IsMethod)
}

func TestDistilledFile(t *testing.T) {
	file := &DistilledFile{
		BaseNode: BaseNode{
			Location: Location{StartLine: 1, EndLine: 100},
		},
		Path:     "test/main.go",
		Language: "go",
		Version:  "2.0.0",
		Children: []DistilledNode{},
		Errors:   []DistilledError{},
	}

	assert.Equal(t, KindFile, file.GetNodeKind())
	assert.Equal(t, "test/main.go", file.Path)
	assert.Equal(t, "go", file.Language)
	assert.Empty(t, file.GetChildren())
}

func TestDistilledError(t *testing.T) {
	err := &DistilledError{
		BaseNode: BaseNode{
			Location: Location{StartLine: 10, EndLine: 10},
		},
		Message:  "Syntax error: unexpected token",
		Severity: "error",
		Code:     "E001",
	}

	assert.Equal(t, KindError, err.GetNodeKind())
	assert.Equal(t, "Syntax error: unexpected token", err.Message)
	assert.Equal(t, "error", err.Severity)
	assert.Nil(t, err.GetChildren())
}

func TestNodeKindConstants(t *testing.T) {
	// Test that all constants are unique
	kinds := []NodeKind{
		KindFile, KindPackage, KindImport, KindClass,
		KindInterface, KindStruct, KindEnum, KindFunction,
		KindField, KindTypeAlias, KindComment, KindError,
	}

	seen := make(map[NodeKind]bool)
	for _, kind := range kinds {
		assert.False(t, seen[kind], "Duplicate NodeKind: %s", kind)
		seen[kind] = true
	}
}

func TestVisibilityConstants(t *testing.T) {
	// Test visibility constants
	visibilities := []Visibility{
		VisibilityPublic, VisibilityPrivate, VisibilityProtected,
		VisibilityInternal, VisibilityPackage, VisibilityFilePrivate,
		VisibilityOpen, VisibilityFriend,
	}

	seen := make(map[Visibility]bool)
	for _, vis := range visibilities {
		assert.False(t, seen[vis], "Duplicate Visibility: %s", vis)
		seen[vis] = true
	}
}

func TestModifierConstants(t *testing.T) {
	// Test modifier constants
	modifiers := []Modifier{
		ModifierStatic, ModifierFinal, ModifierAbstract, ModifierAsync,
		ModifierConst, ModifierReadonly, ModifierOverride, ModifierVirtual,
		ModifierInline, ModifierExtern, ModifierSealed, ModifierData,
		ModifierReified, ModifierMutable, ModifierPartial, ModifierVolatile,
		ModifierTransient,
	}

	seen := make(map[Modifier]bool)
	for _, mod := range modifiers {
		assert.False(t, seen[mod], "Duplicate Modifier: %s", mod)
		seen[mod] = true
	}
}

func TestDistilledFileJSON(t *testing.T) {
	file := &DistilledFile{
		BaseNode: BaseNode{
			Location: Location{
				StartLine:   1,
				StartColumn: 1,
				EndLine:     50,
				EndColumn:   10,
			},
		},
		Path:     "main.go",
		Language: "go",
		Version:  "2.0.0",
		Children: []DistilledNode{},
		Errors: []DistilledError{
			{
				BaseNode: BaseNode{
					Location: Location{StartLine: 10, EndLine: 10},
				},
				Message:  "Test error",
				Severity: "warning",
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(file)
	require.NoError(t, err)

	// Verify JSON contains expected fields
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "file", result["kind"])
	assert.Equal(t, "main.go", result["path"])
	assert.Equal(t, "go", result["language"])
	assert.Equal(t, "2.0.0", result["version"])
}

func TestDistilledErrorJSON(t *testing.T) {
	errNode := &DistilledError{
		BaseNode: BaseNode{
			Location: Location{StartLine: 42, EndLine: 42},
		},
		Message:  "Undefined variable",
		Severity: "error",
		Code:     "E100",
	}

	// Marshal to JSON
	data, err := json.Marshal(errNode)
	require.NoError(t, err)

	// Verify JSON contains expected fields
	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "error", result["kind"])
	assert.Equal(t, "Undefined variable", result["message"])
	assert.Equal(t, "error", result["severity"])
	assert.Equal(t, "E100", result["code"])
}

func TestLanguageExtensions(t *testing.T) {
	t.Run("GoExtensions", func(t *testing.T) {
		ext := &GoExtensions{
			IsChannel:        true,
			ChannelDirection: "send",
			IsMethod:         true,
			ReceiverType:     "*Server",
		}
		assert.True(t, ext.IsChannel)
		assert.Equal(t, "send", ext.ChannelDirection)
	})

	t.Run("PythonExtensions", func(t *testing.T) {
		ext := &PythonExtensions{
			IsGenerator: true,
			IsCoroutine: true,
			IsDataclass: true,
			Metaclass:   "ABCMeta",
		}
		assert.True(t, ext.IsGenerator)
		assert.True(t, ext.IsCoroutine)
		assert.True(t, ext.IsDataclass)
		assert.Equal(t, "ABCMeta", ext.Metaclass)
	})

	t.Run("JavaScriptExtensions", func(t *testing.T) {
		ext := &JavaScriptExtensions{
			IsArrowFunction:     true,
			IsGeneratorFunction: true,
		}
		assert.True(t, ext.IsArrowFunction)
		assert.True(t, ext.IsGeneratorFunction)
	})
}
