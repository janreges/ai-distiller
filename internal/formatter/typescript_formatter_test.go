package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tsFormat(t *testing.T, n ir.DistilledNode, indent int) string {
	var b strings.Builder
	require.NoError(t, NewTypeScriptFormatter().FormatNode(&b, n, indent))
	return b.String()
}

func TestTypeScriptFormatter_FunctionBody(t *testing.T) {
	ret := &ir.TypeRef{Name: "number"}
	params := []ir.Parameter{{Name: "a", Type: ir.TypeRef{Name: "number"}}}
	t.Run("block", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret, Implementation: "{\n  return a + 1;\n}"}
		assert.Equal(t, "function f(a: number): number {\n    return a + 1;\n}\n", tsFormat(t, fn, 0))
	})
	t.Run("arrow block", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret, Modifiers: []ir.Modifier{ir.ModifierFinal}, Implementation: "{\n  return a + 1;\n}"}
		assert.Equal(t, "const f(a: number): number => {\n    return a + 1;\n}\n", tsFormat(t, fn, 0))
	})
	t.Run("expression", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret, Modifiers: []ir.Modifier{ir.ModifierFinal}, Implementation: "a + 1"}
		assert.Equal(t, "const f(a: number): number => a + 1\n", tsFormat(t, fn, 0))
	})
	t.Run("no body", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret}
		assert.Equal(t, "function f(a: number): number\n", tsFormat(t, fn, 0))
	})
}
