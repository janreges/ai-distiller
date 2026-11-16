package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPHPFormatter_FunctionBody(t *testing.T) {
	format := func(impl string) string {
		fn := &ir.DistilledFunction{Name: "quick", Visibility: ir.VisibilityPublic,
			Returns: &ir.TypeRef{Name: "int"}, Implementation: impl}
		var b strings.Builder
		require.NoError(t, NewPHPFormatter().FormatNode(&b, fn, 1))
		return b.String()
	}

	// A body that opens and closes on the signature line stays on one line.
	assert.Equal(t, "    public quick(): int { return 1; }\n", format("{ return 1; }"))
	// A block body loses its brace lines and is re-indented under the method.
	assert.Equal(t, "    public quick(): int {\n        return 1;\n    }\n", format("{\n        return 1;\n    }"))
}

func TestPHPFormatter_SignaturePartsPrintAsWritten(t *testing.T) {
	fn := &ir.DistilledFunction{
		BaseNode:   ir.BaseNode{Extensions: &ir.NodeExtensions{PHP: &ir.PHPExtensions{ReturnsReference: true}}},
		Name:       "collect",
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierStatic},
		Parameters: []ir.Parameter{
			{Name: "arr", Type: ir.TypeRef{Name: "array"}, IsReference: true},
			{Name: "dnf", Type: ir.TypeRef{Name: "(A&B)|null"}, DefaultValue: "null"},
			{Name: "rest", Type: ir.TypeRef{Name: "int"}, IsReference: true, IsVariadic: true},
		},
		Returns: &ir.TypeRef{Name: "never"},
	}
	var b strings.Builder
	require.NoError(t, NewPHPFormatter().FormatNode(&b, fn, 1))
	assert.Equal(t, "    public static &collect(array &$arr, (A&B)|null $dnf = null, int &...$rest): never\n", b.String())
}

// A by-reference parameter is marked in the structured formats too.
func TestStructuredFormats_ReferenceParameter(t *testing.T) {
	file := &ir.DistilledFile{Path: "r.php", Language: "php", Children: []ir.DistilledNode{
		&ir.DistilledFunction{Name: "push", Parameters: []ir.Parameter{{Name: "arr", Type: ir.TypeRef{Name: "array"}, IsReference: true}}},
	}}
	for name, tc := range map[string]struct {
		f    Formatter
		want string
	}{
		"jsonl":           {NewJSONLFormatter(Options{}), `"reference":true`},
		"json-structured": {NewJSONStructuredFormatter(Options{}), `"reference": true`},
		"xml":             {NewXMLFormatter(Options{}), `<parameter name="arr" type="array" reference="true"/>`},
	} {
		var b strings.Builder
		require.NoError(t, tc.f.Format(&b, file), name)
		assert.Contains(t, b.String(), tc.want, name)
	}
}
