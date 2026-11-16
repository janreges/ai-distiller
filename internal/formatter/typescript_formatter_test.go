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
		assert.Equal(t, "const f = (a: number): number => {\n    return a + 1;\n}\n", tsFormat(t, fn, 0))
	})
	t.Run("expression", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret, Modifiers: []ir.Modifier{ir.ModifierFinal}, Implementation: "a + 1"}
		assert.Equal(t, "const f = (a: number): number => a + 1\n", tsFormat(t, fn, 0))
	})
	t.Run("no body", func(t *testing.T) {
		fn := &ir.DistilledFunction{Name: "f", Parameters: params, Returns: ret}
		assert.Equal(t, "function f(a: number): number\n", tsFormat(t, fn, 0))
	})
}

// A variable prints with the keyword it was declared with; `let` and `var`
// arrow functions keep their arrow-constant form.
func TestTypeScriptFormatter_DeclarationKeyword(t *testing.T) {
	params := []ir.Parameter{{Name: "a", Type: ir.TypeRef{Name: "number"}}}
	assert.Equal(t, "let f = (a: number) => a\n", tsFormat(t, &ir.DistilledFunction{Name: "f", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierLet}, Implementation: "a"}, 0))
	assert.Equal(t, "export var g = async (a: number) => {\n    return a;\n}\n",
		tsFormat(t, &ir.DistilledFunction{Name: "g", Parameters: params, Modifiers: []ir.Modifier{ir.ModifierVar, ir.ModifierAsync, ir.ModifierExport}, Implementation: "{\n  return a;\n}"}, 0))
	assert.Equal(t, "var v = 3\n", tsFormat(t, &ir.DistilledField{Name: "v", DefaultValue: "3", Modifiers: []ir.Modifier{ir.ModifierVar}}, 0))
	assert.Equal(t, "let w: number\n", tsFormat(t, &ir.DistilledField{Name: "w", Type: &ir.TypeRef{Name: "number"}, Modifiers: []ir.Modifier{ir.ModifierLet}}, 0))
	assert.Equal(t, "const c = 1\n", tsFormat(t, &ir.DistilledField{Name: "c", DefaultValue: "1", Modifiers: []ir.Modifier{ir.ModifierFinal}}, 0))
}

func TestTypeScriptFormatter_Enum(t *testing.T) {
	e := &ir.DistilledEnum{Name: "Color", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierExport}, Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "Red"},
		&ir.DistilledField{Name: "Green", DefaultValue: `"green"`},
	}}
	assert.Equal(t, "export enum Color {\n    Red\n    Green = \"green\"\n}\n", tsFormat(t, e, 0))
	// A public enum without the export modifier (exported by an export
	// list, or in an ambient module) prints no export keyword it lacks.
	e.Modifiers = nil
	assert.Equal(t, "enum Color {\n    Red\n    Green = \"green\"\n}\n", tsFormat(t, e, 0))
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

// Type parameters keep their default everywhere they print; a generator
// function prints `function*`, a generator method `*name`.
func TestTypeScriptFormatter_TypeParamsAndGenerators(t *testing.T) {
	tps := []ir.TypeParam{{Name: "T", Constraints: []ir.TypeRef{{Name: "object"}}, Default: &ir.TypeRef{Name: "{}"}}, {Name: "U"}}
	assert.Equal(t, "<T extends object = {}, U>", tsTypeParams(tps))
	assert.Equal(t, "", tsTypeParams(nil))
	assert.Equal(t, "\nclass K<T extends object = {}, U> {\n}\n", tsFormat(t, &ir.DistilledClass{Name: "K", TypeParams: tps}, 0))
	assert.Equal(t, "type A<T extends object = {}, U> = T\n", tsFormat(t, &ir.DistilledTypeAlias{Name: "A", TypeParams: tps, Type: ir.TypeRef{Name: "T"}}, 0))
	async := []ir.Modifier{ir.ModifierExport, ir.ModifierAsync}
	assert.Equal(t, "export async function* gen()\n", tsFormat(t, &ir.DistilledFunction{Name: "*gen", Modifiers: async}, 0))
	assert.Equal(t, "    public async *iter()\n", tsFormat(t, &ir.DistilledFunction{Name: "*iter", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierAsync}}, 1))
	assert.Equal(t, "    public get size(): number\n", tsFormat(t, &ir.DistilledFunction{Name: "get size", Visibility: ir.VisibilityPublic, Returns: &ir.TypeRef{Name: "number"}}, 1))
}

// Parameters print as written, in functions, methods and interface methods.
func TestTypeScriptFormatter_Parameters(t *testing.T) {
	params := []ir.Parameter{
		{Name: "{ a, b = 1 }", Type: ir.TypeRef{Name: "P"}},
		{Name: "c", Type: ir.TypeRef{Name: "string"}, IsOptional: true},
		{Name: "d", Type: ir.TypeRef{Name: "number"}, DefaultValue: "0"},
		{Name: "rest", Type: ir.TypeRef{Name: "U[]"}, IsVariadic: true},
	}
	want := "({ a, b = 1 }: P, c?: string, d: number = 0, ...rest: U[])"
	assert.Equal(t, "function f"+want+"\n", tsFormat(t, &ir.DistilledFunction{Name: "f", Parameters: params}, 0))
	intf := &ir.DistilledInterface{Name: "I", Children: []ir.DistilledNode{&ir.DistilledFunction{Name: "m", Parameters: params}}}
	assert.Contains(t, tsFormat(t, intf, 0), "    method m"+want+"\n")
}

// Optional members print their `?`, interface members their `readonly`, and
// an index signature prints as written.
func TestTypeScriptFormatter_OptionalReadonlyIndex(t *testing.T) {
	ro, opt := ir.ModifierReadonly, ir.ModifierOptional
	str := &ir.TypeRef{Name: "string"}
	intf := &ir.DistilledInterface{Name: "I", Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "id", Type: str, Modifiers: []ir.Modifier{ro}},
		&ir.DistilledField{Name: "tags", Type: &ir.TypeRef{Name: "string[]"}, Modifiers: []ir.Modifier{ro, opt}},
		&ir.DistilledField{Name: "[key: string]", Type: &ir.TypeRef{Name: "unknown"}},
		&ir.DistilledFunction{Name: "run", Modifiers: []ir.Modifier{opt}, Returns: &ir.TypeRef{Name: "void"}},
	}}
	assert.Equal(t, "\ninterface I {\n    property readonly id: string\n    property readonly tags?: string[]\n    property [key: string]: unknown\n    method run?(): void\n}\n", tsFormat(t, intf, 0))
	cls := &ir.DistilledClass{Name: "K", Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "name", Type: str, Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{opt}},
		&ir.DistilledField{Name: "h", Type: str, Visibility: ir.VisibilityPrivate, Modifiers: []ir.Modifier{ro, opt}},
		&ir.DistilledFunction{Name: "m", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{opt}},
	}}
	assert.Equal(t, "\nclass K {\n    public name?: string\n    private readonly h?: string\n    public m?()\n}\n", tsFormat(t, cls, 0))
}
