package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func jsFormat(t *testing.T, n ir.DistilledNode, indent int) string {
	var b strings.Builder
	require.NoError(t, NewJavaScriptFormatter().FormatNode(&b, n, indent))
	return b.String()
}

func TestJavaScriptFormatter_FunctionBody(t *testing.T) {
	params := []ir.Parameter{{Name: "a"}}
	t.Run("block", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Implementation: "{\n  return a + 1;\n}"}
		assert.Equal(t, "function f(a) {\n    return a + 1;\n}\n", jsFormat(t, fn, 0))
	})
	t.Run("arrow block", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierFinal}, Implementation: "{\n  return a + 1;\n}"}
		assert.Equal(t, "const f = (a) => {\n    return a + 1;\n}\n", jsFormat(t, fn, 0))
	})
	t.Run("expression", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierFinal}, Implementation: "a + 1"}
		out := jsFormat(t, fn, 0)
		assert.True(t, strings.HasSuffix(out, " => a + 1\n"), out)
	})
	t.Run("no body", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params}
		assert.Equal(t, "function f(a)\n", jsFormat(t, fn, 0))
	})
}

func TestJavaScriptFormatter_Comments(t *testing.T) {
	var b strings.Builder
	f := NewJavaScriptFormatter()
	cls := &ir.DistilledClass{Name: "K", Children: []ir.DistilledNode{
		&ir.DistilledComment{Text: "inner", Format: ir.CommentLine},
		&ir.DistilledComment{Text: "First.\nSecond.", Format: ir.CommentDoc},
		&ir.DistilledFunction{Name: "m"},
		&ir.DistilledComment{Text: "static initialization block", Format: ir.CommentBlock},
	}}
	for _, n := range []ir.DistilledNode{
		&ir.DistilledComment{Text: "K doc", Format: ir.CommentDoc},
		cls,
		&ir.DistilledComment{Text: "module.exports = K", Format: ir.CommentExport},
	} {
		require.NoError(t, f.FormatNode(&b, n, 0))
	}
	assert.Equal(t, "/** K doc */\nclass K:\n    // inner\n    /**\n     * First.\n     * Second.\n     */\n    m()\n    /* static initialization block */\n// module.exports = K\n", b.String())
}
