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

// A variable prints with the keyword it was declared with; `let` and `var`
// arrow functions keep their arrow-constant form.
func TestJavaScriptFormatter_DeclarationKeyword(t *testing.T) {
	params := []ir.Parameter{{Name: "a"}}
	assert.Equal(t, "let f = (a) => a\n", jsFormat(t, &ir.DistilledFunction{Name: "f", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierLet}, Implementation: "a"}, 0))
	assert.Equal(t, "export var g = async (a)\n", jsFormat(t, &ir.DistilledFunction{Name: "g", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierVar, ir.ModifierAsync, ir.ModifierExport}}, 0))
	assert.Equal(t, "var v = 3\n", jsFormat(t, &ir.DistilledField{Name: "v", DefaultValue: "3", Modifiers: []ir.Modifier{ir.ModifierVar}}, 0))
	assert.Equal(t, "let w = 4\n", jsFormat(t, &ir.DistilledField{Name: "w", DefaultValue: "4", Modifiers: []ir.Modifier{ir.ModifierLet}}, 0))
}

// The star of a generator method sits on its name, as written.
func TestJavaScriptFormatter_GeneratorMethods(t *testing.T) {
	assert.Equal(t, "    *gen()\n", jsFormat(t, &ir.DistilledFunction{Name: "*gen"}, 1))
	assert.Equal(t, "    async *agen()\n", jsFormat(t, &ir.DistilledFunction{Name: "*agen", Modifiers: []ir.Modifier{ir.ModifierAsync}}, 1))
	assert.Equal(t, "    static async *sgen(n)\n",
		jsFormat(t, &ir.DistilledFunction{Name: "*sgen", Modifiers: []ir.Modifier{ir.ModifierStatic, ir.ModifierAsync}, Parameters: []ir.Parameter{{Name: "n"}}}, 1))
}

// A variable whose value is a function expression prints `function`, not
// an arrow, and its body without `=>`.
func TestJavaScriptFormatter_FunctionExpression(t *testing.T) {
	expr := &ir.NodeExtensions{JavaScript: &ir.JavaScriptExtensions{IsFunctionExpression: true}}
	params := []ir.Parameter{{Name: "a"}}
	fe := func(name string, mods ...ir.Modifier) *ir.DistilledFunction {
		return &ir.DistilledFunction{BaseNode: ir.BaseNode{Extensions: expr}, Name: name, Parameters: params, Modifiers: mods}
	}
	assert.Equal(t, "const fe = function (a)\n", jsFormat(t, fe("fe", ir.ModifierFinal), 0))
	assert.Equal(t, "export let ge = function* (a)\n", jsFormat(t, fe("*ge", ir.ModifierLet, ir.ModifierExport), 0))
	assert.Equal(t, "var af = async function* (a)\n", jsFormat(t, fe("*af", ir.ModifierVar, ir.ModifierAsync), 0))
	withBody := fe("fe", ir.ModifierFinal)
	withBody.Implementation = "{\n  return a;\n}"
	assert.Equal(t, "const fe = function (a) {\n    return a;\n}\n", jsFormat(t, withBody, 0))
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

// An exported declaration prints `export` or `export default`, like
// TypeScript; a generator function prints `function*`.
func TestJavaScriptFormatter_Export(t *testing.T) {
	export := []ir.Modifier{ir.ModifierExport}
	exportDefault := []ir.Modifier{ir.ModifierExport, ir.ModifierDefault}
	assert.Equal(t, "\nexport class K:\n", jsFormat(t, &ir.DistilledClass{Name: "K", Modifiers: export}, 0))
	assert.Equal(t, "\nexport default class:\n", jsFormat(t, &ir.DistilledClass{Modifiers: exportDefault}, 0))
	assert.Equal(t, "export default function main()\n", jsFormat(t, &ir.DistilledFunction{Name: "main", Modifiers: exportDefault}, 0))
	assert.Equal(t, "export async function* gen(n)\n",
		jsFormat(t, &ir.DistilledFunction{Name: "*gen", Modifiers: []ir.Modifier{ir.ModifierAsync, ir.ModifierExport}, Parameters: []ir.Parameter{{Name: "n"}}}, 0))
	assert.Equal(t, "function* gen()\n", jsFormat(t, &ir.DistilledFunction{Name: "*gen"}, 0))
	assert.Equal(t, "export const c = 1\n", jsFormat(t, &ir.DistilledField{Name: "c", DefaultValue: "1", Modifiers: []ir.Modifier{ir.ModifierFinal, ir.ModifierExport}}, 0))
	assert.Equal(t, "export const f = (x)\n", jsFormat(t, &ir.DistilledFunction{Name: "f", Modifiers: []ir.Modifier{ir.ModifierFinal, ir.ModifierExport}, Parameters: []ir.Parameter{{Name: "x"}}}, 0))
}
