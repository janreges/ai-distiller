package typescript

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseTS parses src with the real TypeScript processor and returns the raw IR.
func parseTS(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	f, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "t.ts")
	require.NoError(t, err)
	return f
}

func TestTS_Namespace(t *testing.T) {
	f := parseTS(t, "export namespace NS {\n  export function f(): void {}\n  function hidden(): void {}\n}\n")
	ns, ok := f.Children[0].(*ir.DistilledPackage)
	require.True(t, ok, "namespace must be a DistilledPackage, got %T", f.Children[0])
	assert.Equal(t, "NS", ns.Name)
	require.Len(t, ns.Children, 2)
	assert.Equal(t, ir.VisibilityPublic, ns.Children[0].(*ir.DistilledFunction).Visibility)
	assert.Equal(t, ir.VisibilityInternal, ns.Children[1].(*ir.DistilledFunction).Visibility)
}

func TestTS_AmbientModule(t *testing.T) {
	f := parseTS(t, "declare module \"conf-x\" {\n  export function g(a: number): void;\n}\n")
	ns := f.Children[0].(*ir.DistilledPackage)
	assert.Equal(t, `"conf-x"`, ns.Name)
	g := ns.Children[0].(*ir.DistilledFunction)
	assert.Equal(t, "g", g.Name)
	assert.Empty(t, g.Implementation)
	assert.Len(t, g.Parameters, 1)
}

func TestTS_FunctionReturnType(t *testing.T) {
	f := parseTS(t, "export function f(a: number): Promise<number> { return a }\n")
	fn := f.Children[0].(*ir.DistilledFunction)
	require.NotNil(t, fn.Returns)
	assert.Equal(t, "Promise<number>", fn.Returns.Name)
}

func TestTS_ConstTypeAndValue(t *testing.T) {
	t.Run("single line", func(t *testing.T) {
		fld := parseTS(t, "export const x: number = 1;\n").Children[0].(*ir.DistilledField)
		assert.Equal(t, "number", fld.Type.Name)
		assert.Equal(t, "1", fld.DefaultValue)
	})
	t.Run("multiline", func(t *testing.T) {
		fld := parseTS(t, "export const cfg: Cfg = {\n  a: 1,\n  b: 2,\n};\n").Children[0].(*ir.DistilledField)
		assert.Equal(t, "Cfg", fld.Type.Name)
		assert.Empty(t, fld.DefaultValue, "multi-line initialisers are not printed")
	})
	t.Run("too long", func(t *testing.T) {
		fld := parseTS(t, "export const s = \""+strings.Repeat("x", 100)+"\";\n").Children[0].(*ir.DistilledField)
		assert.Empty(t, fld.DefaultValue)
	})
}

func TestTS_PrivateHashField(t *testing.T) {
	cls := parseTS(t, "export class K {\n  #secret = 1;\n}\n").Children[0].(*ir.DistilledClass)
	fld := cls.Children[0].(*ir.DistilledField)
	assert.Equal(t, "#secret", fld.Name)
	assert.Equal(t, ir.VisibilityPrivate, fld.Visibility)
}

func TestTS_ArrowExpressionBody(t *testing.T) {
	f := parseTS(t, "export const id = (x: number) => x;\nexport const inc = (x: number) => x + 1;\n")
	require.Len(t, f.Children, 2)
	id := f.Children[0].(*ir.DistilledFunction)
	assert.Equal(t, "x", id.Implementation)
	assert.Len(t, id.Parameters, 1)
	inc := f.Children[1].(*ir.DistilledFunction)
	assert.Equal(t, "x + 1", inc.Implementation)
}

func TestTS_TypeGuardReturnTypes(t *testing.T) {
	src := "export function typed(x: number): x is number { return true; }\n" +
		"export function check(x: unknown): asserts x is string {}\n" +
		"export function ok(x: unknown): asserts x {}\n" +
		"export declare function sig(x: unknown): x is Date;\n" +
		"export const arrow = (x: any): x is number => true;\n" +
		"export class C { isC(): this is C { return true; } }\n" +
		"export interface I { isS(v: any): v is string; }\n"
	f := parseTS(t, src)
	returns := func(fn *ir.DistilledFunction) string {
		if fn.Returns == nil {
			return "<none>"
		}
		return fn.Returns.Name
	}
	assert.Equal(t, "x is number", returns(f.Children[0].(*ir.DistilledFunction)))
	assert.Equal(t, "asserts x is string", returns(f.Children[1].(*ir.DistilledFunction)))
	assert.Equal(t, "asserts x", returns(f.Children[2].(*ir.DistilledFunction)))
	assert.Equal(t, "x is Date", returns(f.Children[3].(*ir.DistilledFunction)))
	assert.Equal(t, "x is number", returns(f.Children[4].(*ir.DistilledFunction)))
	assert.Equal(t, "this is C", returns(f.Children[5].(*ir.DistilledClass).Children[0].(*ir.DistilledFunction)))
	assert.Equal(t, "v is string", returns(f.Children[6].(*ir.DistilledInterface).Children[0].(*ir.DistilledFunction)))
}

func TestTS_EnumInitialisers(t *testing.T) {
	e := parseTS(t, "export enum E { A = -1, B = 1 << 2, C = Other.X, D = \"d\", F = 3, G }\n").Children[0].(*ir.DistilledEnum)
	var got []string
	for _, c := range e.Children {
		fld := c.(*ir.DistilledField)
		got = append(got, fld.Name+"="+fld.DefaultValue)
	}
	assert.Equal(t, []string{"A=-1", "B=1 << 2", "C=Other.X", `D="d"`, "F=3", "G="}, got)
}

func TestTS_AmbientMembersArePublic(t *testing.T) {
	src := "declare namespace DN {\n  function inDn(a: string): void;\n  const v: number;\n  function _under(): void;\n  class K {}\n  interface Opts {}\n  type T = string;\n  enum E { A }\n  namespace Inner { function deep(): void; }\n}\n" +
		"declare module \"m\" {\n  function implicit(): void;\n  interface MOpts {}\n}\n" +
		"namespace Plain {\n  function hidden(): void {}\n}\n"
	f := parseTS(t, src)
	require.Len(t, f.Children, 3)

	visibility := func(n ir.DistilledNode) ir.Visibility {
		switch n := n.(type) {
		case *ir.DistilledFunction:
			return n.Visibility
		case *ir.DistilledField:
			return n.Visibility
		case *ir.DistilledClass:
			return n.Visibility
		case *ir.DistilledInterface:
			return n.Visibility
		case *ir.DistilledTypeAlias:
			return n.Visibility
		case *ir.DistilledEnum:
			return n.Visibility
		}
		return ""
	}
	dn := f.Children[0].(*ir.DistilledPackage)
	require.Len(t, dn.Children, 8)
	for _, member := range dn.Children[:7] {
		assert.Equal(t, ir.VisibilityPublic, visibility(member), "declare namespace member %T", member)
	}
	inner := dn.Children[7].(*ir.DistilledPackage)
	assert.Equal(t, ir.VisibilityPublic, visibility(inner.Children[0]), "member of a namespace nested in declare namespace")

	m := f.Children[1].(*ir.DistilledPackage)
	for _, member := range m.Children {
		assert.Equal(t, ir.VisibilityPublic, visibility(member), "declare module member %T", member)
	}
	for _, member := range m.Children {
		if fn, ok := member.(*ir.DistilledFunction); ok {
			assert.NotContains(t, fn.Modifiers, ir.ModifierExport, "no export keyword was written")
		}
	}

	plain := f.Children[2].(*ir.DistilledPackage)
	assert.Equal(t, ir.VisibilityInternal, visibility(plain.Children[0]), "a plain namespace keeps non-exported members internal")
}
