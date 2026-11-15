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

func TestTypeScriptFormatter_Enum(t *testing.T) {
	e := &ir.DistilledEnum{Name: "Color", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "Red"},
		&ir.DistilledField{Name: "Green", DefaultValue: `"green"`},
	}}
	assert.Equal(t, "export enum Color {\n    Red\n    Green = \"green\"\n}\n", tsFormat(t, e, 0))
}

func TestTypeScriptFormatter_Namespace(t *testing.T) {
	fn := &ir.DistilledFunction{Name: "f", Modifiers: []ir.Modifier{ir.ModifierExport}, Returns: &ir.TypeRef{Name: "void"}}
	assert.Equal(t, "namespace NS {\n    export function f(): void\n}\n",
		tsFormat(t, &ir.DistilledPackage{Name: "NS", Children: []ir.DistilledNode{fn}}, 0))
	assert.Equal(t, "declare module \"x\" {\n    export function f(): void\n}\n",
		tsFormat(t, &ir.DistilledPackage{Name: `"x"`, Children: []ir.DistilledNode{fn}}, 0))
}

// tsFormatAll formats sibling nodes with one formatter, as a file is formatted.
func tsFormatAll(t *testing.T, nodes ...ir.DistilledNode) string {
	var b strings.Builder
	f := NewTypeScriptFormatter()
	for _, n := range nodes {
		require.NoError(t, f.FormatNode(&b, n, 0))
	}
	return b.String()
}

func TestTypeScriptFormatter_Comments(t *testing.T) {
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	line := func(text string) *ir.DistilledComment {
		return &ir.DistilledComment{Text: text, Format: ir.CommentLine}
	}
	method := &ir.DistilledFunction{Name: "m", Visibility: ir.VisibilityPublic, Returns: &ir.TypeRef{Name: "void"}}
	t.Run("class body", func(t *testing.T) {
		cls := &ir.DistilledClass{Name: "K", Modifiers: []ir.Modifier{ir.ModifierExport}, Children: []ir.DistilledNode{
			line("inner"), doc("First.\nSecond."), method,
		}}
		assert.Equal(t, "// top\n/** K doc */\nexport class K {\n    // inner\n    /**\n     * First.\n     * Second.\n     */\n    public m(): void\n}\n",
			tsFormatAll(t, line("top"), doc("K doc"), cls))
	})
	t.Run("a doc sits on the declaration it documents", func(t *testing.T) {
		// Without a comment before it a class or interface opens with a blank
		// line; a comment ending the previous class body does not count.
		k := &ir.DistilledClass{Name: "K", Children: []ir.DistilledNode{method, line("end")}}
		assert.Equal(t, "\nclass K {\n    public m(): void\n    // end\n}\n\nclass L {\n}\n/** I doc */\ninterface I {\n}\n",
			tsFormatAll(t, k, &ir.DistilledClass{Name: "L"}, doc("I doc"), &ir.DistilledInterface{Name: "I"}))
	})
	t.Run("namespace", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Modifiers: []ir.Modifier{ir.ModifierExport}, Returns: &ir.TypeRef{Name: "void"}}
		ns := &ir.DistilledPackage{Name: "NS", Children: []ir.DistilledNode{doc("A.\nB."), fn}}
		assert.Equal(t, "namespace NS {\n    /**\n     * A.\n     * B.\n     */\n    export function f(): void\n}\n", tsFormat(t, ns, 0))
	})
}
