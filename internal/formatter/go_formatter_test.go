package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoFormatter_FunctionBody(t *testing.T) {
	format := func(fn *ir.DistilledFunction) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, fn, 0))
		return b.String()
	}
	body := "{\n\tif id == 0 {\n\t\treturn nil\n\t}\n\treturn &id\n}"
	ret := &ir.TypeRef{Name: "*int"}

	fn := &ir.DistilledFunction{Name: "Get", Returns: ret, Implementation: body,
		Parameters: []ir.Parameter{{Name: "id", Type: ir.TypeRef{Name: "int"}}}}
	assert.Equal(t, "\nfunc Get(id int) *int {\n    if id == 0 {\n        return nil\n    }\n    return &id\n}\n", format(fn))

	method := &ir.DistilledFunction{Name: "One", Modifiers: []ir.Modifier{ir.ModifierAbstract},
		Returns:        &ir.TypeRef{Name: "int"},
		Parameters:     []ir.Parameter{{Name: "w", Type: ir.TypeRef{Name: "*W"}}},
		Implementation: "{ return 1 }"}
	assert.Equal(t, "func (w *W) One() int { return 1 }\n", format(method))

	decl := &ir.DistilledFunction{Name: "Get", Returns: ret}
	assert.Equal(t, "\nfunc Get() *int\n", format(decl))
}

func TestGoFormatter_CommentInsidePrintedBody(t *testing.T) {
	// func Get() {     line 1
	// 	// inner        line 2
	// 	return
	// } // after       line 4
	loc := ir.Location{StartLine: 1, StartColumn: 1, EndLine: 4, EndColumn: 2}
	inner := &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: 2, StartColumn: 2}}, Text: "inner", Format: "inline"}
	after := &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: 4, StartColumn: 3}}, Text: "after", Format: "inline"}
	format := func(impl string) string {
		f := NewGoFormatter()
		fn := &ir.DistilledFunction{BaseNode: ir.BaseNode{Location: loc}, Name: "Get", Implementation: impl}
		var b strings.Builder
		for _, n := range []ir.DistilledNode{fn, inner, after} {
			require.NoError(t, f.FormatNode(&b, n, 0))
		}
		return b.String()
	}

	// The printed body already holds the inner comment; it is not repeated.
	assert.Equal(t, "\nfunc Get() {\n    // inner\n    return\n}\n// after\n", format("{\n\t// inner\n\treturn\n}"))
	// Without the body the comment node is the only place the text appears.
	assert.Equal(t, "\nfunc Get()\n// inner\n// after\n", format(""))
}

func TestGoFormatter_NestedBodyIndentation(t *testing.T) {
	body := "{\n\tfor i := 0; i < 3; i++ {\n\t\tif i == 1 {\n\t\t\tcontinue\n\t\t}\n\t}\n}"
	fn := &ir.DistilledFunction{Name: "Loop", Implementation: body}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().FormatNode(&b, fn, 0))
	out := b.String()
	assert.NotContains(t, out, "\t")
	assert.Contains(t, out, "\n            continue\n")
}
