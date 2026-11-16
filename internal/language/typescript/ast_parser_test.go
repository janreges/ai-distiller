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

func TestTS_HashMethodsAndAccessorsArePrivate(t *testing.T) {
	f := parseTS(t, "class A {\n  #p(): void {}\n  get #g(): number { return 1; }\n  open(): void {}\n}\n")
	var cls *ir.DistilledClass
	for _, c := range f.Children {
		if k, ok := c.(*ir.DistilledClass); ok {
			cls = k
		}
	}
	require.NotNil(t, cls)
	vis := map[string]ir.Visibility{}
	for _, c := range cls.Children {
		if fn, ok := c.(*ir.DistilledFunction); ok {
			vis[fn.Name] = fn.Visibility
		}
	}
	assert.Equal(t, ir.VisibilityPrivate, vis["#p"])
	assert.Equal(t, ir.VisibilityPrivate, vis["get #g"])
	assert.Equal(t, ir.VisibilityPublic, vis["open"])
}

// Accessors and generators are named as the JavaScript processor names
// them (`get size`, `*iter`); type parameters keep their default.
func TestTS_AccessorsGeneratorsAndTypeParamDefaults(t *testing.T) {
	f := parseTS(t, "class K<T extends object = {}> {\n  get size(): number { return 0; }\n  set size(v: number) {}\n  async *iter() {}\n  get(key: string) {}\n}\n"+
		"function* gen() {}\n"+
		"interface I { get n(): number; }\n")
	k := f.Children[0].(*ir.DistilledClass)
	assert.Equal(t, []ir.TypeParam{{Name: "T", Constraints: []ir.TypeRef{{Name: "object"}}, Default: &ir.TypeRef{Name: "{}"}}}, k.TypeParams)
	var names []string
	for _, c := range k.Children {
		names = append(names, c.(*ir.DistilledFunction).Name)
	}
	assert.Equal(t, []string{"get size", "set size", "*iter", "get"}, names)
	assert.Contains(t, k.Children[2].(*ir.DistilledFunction).Modifiers, ir.ModifierAsync)
	assert.Equal(t, "*gen", f.Children[1].(*ir.DistilledFunction).Name)
	assert.Equal(t, "get n", f.Children[2].(*ir.DistilledInterface).Children[0].(*ir.DistilledFunction).Name)
}

func TestTS_Comments(t *testing.T) {
	f := parseTS(t, "// line c\n/** Doc c */\nexport class K {\n  // inner\n  /** m doc */\n  m(): void {}\n}\n")
	top0 := f.Children[0].(*ir.DistilledComment)
	assert.Equal(t, "line c", top0.Text)
	assert.Equal(t, ir.CommentLine, top0.Format)
	top1 := f.Children[1].(*ir.DistilledComment)
	assert.Equal(t, "Doc c", top1.Text)
	assert.True(t, top1.IsDoc())
	cls := f.Children[2].(*ir.DistilledClass)
	assert.Equal(t, "inner", cls.Children[0].(*ir.DistilledComment).Text)
	assert.True(t, cls.Children[1].(*ir.DistilledComment).IsDoc())
}

// The class body is reordered (fields, constructor, methods); a member's
// comments move with it, so a doc stays directly before what it documents.
func TestTS_ClassCommentsMoveWithTheirMember(t *testing.T) {
	src := "class K {\n  /** m doc */\n  m(): void {}\n  /** ctor doc */\n  constructor(private a: number) {}\n  /** f doc */\n  f = 1;\n  /** index doc */\n  [key: string]: unknown;\n  /** block doc */\n  static {}\n  // trailing\n}\n"
	cls := parseTS(t, src).Children[0].(*ir.DistilledClass)
	var got []string
	for _, c := range cls.Children {
		switch n := c.(type) {
		case *ir.DistilledComment:
			got = append(got, "/"+n.Text)
		case *ir.DistilledField:
			got = append(got, n.Name)
		case *ir.DistilledFunction:
			got = append(got, n.Name+"()")
		}
	}
	// The doc of the static block, which is not emitted, is dropped; an
	// index signature is a field and keeps its doc.
	assert.Equal(t, []string{"a", "/f doc", "f", "/index doc", "[key: string]", "/ctor doc", "constructor()", "/m doc", "m()", "/trailing"}, got)
}

func TestTS_FileDocIsInner(t *testing.T) {
	f := parseTS(t, "/**\n * @packageDocumentation\n * The app.\n */\n/** K doc */\nexport class K {}\n")
	assert.True(t, f.Children[0].(*ir.DistilledComment).Inner, "a file doc documents the file")
	assert.False(t, f.Children[1].(*ir.DistilledComment).Inner)
}

// docTexts returns the texts of the doc comments among nodes.
func docTexts(nodes []ir.DistilledNode) []string {
	var texts []string
	for _, n := range nodes {
		if c, ok := n.(*ir.DistilledComment); ok && c.IsDoc() {
			texts = append(texts, c.Text)
		}
	}
	return texts
}

// A doc before a statement that yields no declaration documented that
// statement; it must not end up above the next declaration. Line comments
// keep their place.
func TestTS_DocOfStatementWithoutDeclarationIsDropped(t *testing.T) {
	for name, stmt := range map[string]string{
		"exports.x":      "exports.add = function (a: number) { return a; };",
		"module.exports": "module.exports = { a: 1 };",
		"call":           "main();",
	} {
		t.Run(name, func(t *testing.T) {
			f := parseTS(t, "/** @deprecated Use bar. */\n// line kept\n"+stmt+"\n/** bar doc */\nexport function bar(): void {}\n")
			require.Len(t, f.Children, 3)
			assert.Equal(t, "line kept", f.Children[0].(*ir.DistilledComment).Text)
			assert.Equal(t, "bar doc", f.Children[1].(*ir.DistilledComment).Text)
			assert.Equal(t, "bar", f.Children[2].(*ir.DistilledFunction).Name)
		})
	}
	t.Run("namespace", func(t *testing.T) {
		ns := parseTS(t, "namespace N {\n  /** orphan */\n  main();\n  /** g doc */\n  export function g(): void {}\n}\n").Children[0].(*ir.DistilledPackage)
		assert.Equal(t, []string{"g doc"}, docTexts(ns.Children))
	})
	t.Run("export default class keeps its doc", func(t *testing.T) {
		f := parseTS(t, "/** Foo doc */\nexport default class Foo {}\n")
		assert.Equal(t, []string{"Foo doc"}, docTexts(f.Children))
	})
	// A re-export and `export default <expression>` are export nodes now:
	// the doc above them documents them.
	for name, stmt := range map[string]string{
		"re-export":      `export { foo } from "./foo";`,
		"export default": "export default { a: 1 };",
	} {
		t.Run(name+" keeps its doc", func(t *testing.T) {
			f := parseTS(t, "/** export doc */\n"+stmt+"\n")
			require.Len(t, f.Children, 2)
			assert.Equal(t, "export doc", f.Children[0].(*ir.DistilledComment).Text)
			assert.IsType(t, &ir.DistilledExport{}, f.Children[1])
		})
	}
}

// Every export statement form survives: re-exports and export lists as
// export nodes, exported declarations with their modifiers.
func TestTS_ExportStatements(t *testing.T) {
	src := "export * from \"./a\";\n" +
		"export * as ns from \"./b\";\n" +
		"export { x, y as z } from \"./c\";\n" +
		"export type { T } from \"./t\";\n" +
		"export { default as Btn, type P } from \"./btn\";\n" +
		"export = Foo;\n" +
		"export default function main(): void {}\n" +
		"export default class {}\n"
	f := parseTS(t, src)
	require.Len(t, f.Children, 8)
	exp := func(i int) *ir.DistilledExport { return f.Children[i].(*ir.DistilledExport) }
	assert.Equal(t, "./a", exp(0).Module)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "*"}}, exp(0).Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "*", Alias: "ns"}}, exp(1).Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "x"}, {Name: "y", Alias: "z"}}, exp(2).Symbols)
	assert.True(t, exp(3).IsType)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "default", Alias: "Btn"}, {Name: "P", IsType: true}}, exp(4).Symbols)
	assert.Equal(t, "Foo", exp(5).Value)
	assert.False(t, exp(5).IsDefault)
	main := f.Children[6].(*ir.DistilledFunction)
	assert.Equal(t, []ir.Modifier{ir.ModifierExport, ir.ModifierDefault}, main.Modifiers)
	anon := f.Children[7].(*ir.DistilledClass)
	assert.Equal(t, "", anon.Name)
	assert.Contains(t, anon.Modifiers, ir.ModifierDefault)
}

// A declaration that an export list or `export default <name>` exports is
// public, without an export modifier it does not have in the source.
func TestTS_ExportedNamesArePublic(t *testing.T) {
	f := parseTS(t, "const q = 1;\nconst hidden = 2;\nfunction _f(): void {}\nconst Inner = 3;\nexport { q, _f as f };\nexport default Inner;\n"+
		"namespace N {\n  const n = 1;\n  export { n };\n}\n")
	vis := map[string]ir.Visibility{}
	for _, c := range f.Children {
		switch d := c.(type) {
		case *ir.DistilledField:
			vis[d.Name] = d.Visibility
			assert.NotContains(t, d.Modifiers, ir.ModifierExport)
		case *ir.DistilledFunction:
			vis[d.Name] = d.Visibility
		case *ir.DistilledPackage:
			vis["N.n"] = d.Children[0].(*ir.DistilledField).Visibility
		}
	}
	assert.Equal(t, map[string]ir.Visibility{
		"q": ir.VisibilityPublic, "hidden": ir.VisibilityInternal, "_f": ir.VisibilityPublic,
		"Inner": ir.VisibilityPublic, "N.n": ir.VisibilityPublic,
	}, vis)
}

// Every import form keeps its bindings: a default import is the symbol
// "default" aliased to its local name, a namespace import "*" with its alias.
func TestTS_ImportForms(t *testing.T) {
	src := "import { type A, B as C } from \"./x\";\n" +
		"import D, { E } from \"./d\";\n" +
		"import type { Foo } from \"./foo\";\n" +
		"import * as z from \"zod\";\n" +
		"import \"./side\";\n" +
		"import F = require(\"f\");\n" +
		"import H, * as I from \"./h\";\n"
	f := parseTS(t, src)
	require.Len(t, f.Children, 7)
	imp := func(i int) *ir.DistilledImport { return f.Children[i].(*ir.DistilledImport) }

	assert.Equal(t, "./x", imp(0).Module)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "A", IsType: true}, {Name: "B", Alias: "C"}}, imp(0).Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "default", Alias: "D"}, {Name: "E"}}, imp(1).Symbols)
	assert.True(t, imp(2).IsType)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "Foo"}}, imp(2).Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "*", Alias: "z"}}, imp(3).Symbols)
	assert.Equal(t, "./side", imp(4).Module)
	assert.Empty(t, imp(4).Symbols)
	assert.Equal(t, "require", imp(5).ImportType)
	assert.Equal(t, "f", imp(5).Module)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "*", Alias: "F"}}, imp(5).Symbols)
	assert.Equal(t, []ir.ImportedSymbol{{Name: "default", Alias: "H"}, {Name: "*", Alias: "I"}}, imp(6).Symbols)
}

// `import G = ns.H` is an import alias, `export import` an exported one, and
// `export as namespace Lib` an export with a namespace.
func TestTS_ImportAliasAndExportAsNamespace(t *testing.T) {
	f := parseTS(t, "import G = ns.H;\nexport import E = ns.Inner.E;\nexport as namespace Lib;\n")
	require.Len(t, f.Children, 3)
	assert.Equal(t, &ir.DistilledImport{BaseNode: f.Children[0].(*ir.DistilledImport).BaseNode, ImportType: "alias", Module: "ns.H",
		Symbols: []ir.ImportedSymbol{{Name: "*", Alias: "G"}}}, f.Children[0])
	e := f.Children[1].(*ir.DistilledImport)
	assert.Equal(t, "ns.Inner.E", e.Module)
	assert.True(t, e.IsExport)
	assert.Equal(t, "Lib", f.Children[2].(*ir.DistilledExport).Namespace)
}

// typeNames returns the names of refs.
func typeNames(refs []ir.TypeRef) []string {
	var out []string
	for _, r := range refs {
		out = append(out, r.Name)
	}
	return out
}

// Every base in an extends or implements clause is kept as written: generic,
// qualified, both, or an expression.
func TestTS_HeritageAsWritten(t *testing.T) {
	src := "interface A extends B<string>, ns.C, D<\n  E,\n> {}\n" +
		"class K extends React.Component<{ x: number }> implements M<number>, ns.N {}\n" +
		"class L extends mixin(A, B) {}\n" +
		"class P extends Q {}\n"
	f := parseTS(t, src)
	require.Len(t, f.Children, 4)
	assert.Equal(t, []string{"B<string>", "ns.C", "D<E>"}, typeNames(f.Children[0].(*ir.DistilledInterface).Extends))
	k := f.Children[1].(*ir.DistilledClass)
	assert.Equal(t, []string{"React.Component<{ x: number }>"}, typeNames(k.Extends))
	assert.Equal(t, []string{"M<number>", "ns.N"}, typeNames(k.Implements))
	assert.Equal(t, []string{"mixin(A, B)"}, typeNames(f.Children[2].(*ir.DistilledClass).Extends))
	assert.Equal(t, []string{"Q"}, typeNames(f.Children[3].(*ir.DistilledClass).Extends))
}

// Parameters keep what the source says: destructuring patterns, rest,
// optional, defaults and `this`.
func TestTS_ParametersAsWritten(t *testing.T) {
	f := parseTS(t, "function f({ a, b = 1 }: P, [x,\n  y]: [number, number], c?: string, d: number = 0, ...rest: U[]): void {}\n"+
		"class K { m(this: K, e = () => { return 1; }) {} }\n")
	params := f.Children[0].(*ir.DistilledFunction).Parameters
	assert.Equal(t, []ir.Parameter{
		{Name: "{ a, b = 1 }", Type: ir.TypeRef{Name: "P"}},
		{Name: "[x, y]", Type: ir.TypeRef{Name: "[number, number]"}},
		{Name: "c", Type: ir.TypeRef{Name: "string"}, IsOptional: true},
		{Name: "d", Type: ir.TypeRef{Name: "number"}, DefaultValue: "0"},
		{Name: "rest", Type: ir.TypeRef{Name: "U[]"}, IsVariadic: true},
	}, params)
	m := f.Children[1].(*ir.DistilledClass).Children[0].(*ir.DistilledFunction)
	assert.Equal(t, []ir.Parameter{
		{Name: "this", Type: ir.TypeRef{Name: "K"}},
		{Name: "e", DefaultValue: "()"},
	}, m.Parameters)
}

// A constructor parameter with an accessibility modifier or readonly is a
// field with that visibility and readonly.
func TestTS_ParameterProperties(t *testing.T) {
	f := parseTS(t, "class K { constructor(private readonly s: S, readonly r: number, protected p: P, plain: Q, public o?: O) {} }\n")
	fields := map[string]*ir.DistilledField{}
	for _, c := range f.Children[0].(*ir.DistilledClass).Children {
		if fd, ok := c.(*ir.DistilledField); ok {
			fields[fd.Name] = fd
		}
	}
	require.Len(t, fields, 4)
	assert.Equal(t, []ir.Modifier{ir.ModifierOptional}, fields["o"].Modifiers)
	assert.Equal(t, ir.VisibilityPrivate, fields["s"].Visibility)
	assert.Equal(t, []ir.Modifier{ir.ModifierReadonly}, fields["s"].Modifiers)
	assert.Equal(t, ir.VisibilityPublic, fields["r"].Visibility)
	assert.Equal(t, []ir.Modifier{ir.ModifierReadonly}, fields["r"].Modifiers)
	assert.Equal(t, "number", fields["r"].Type.Name)
	assert.Equal(t, ir.VisibilityProtected, fields["p"].Visibility)
	assert.Empty(t, fields["p"].Modifiers)
}

// The `?` of an optional member is the modifier optional, `readonly` is
// kept on interface members too, and an index signature is a field named by
// its brackets as written.
func TestTS_OptionalReadonlyAndIndexSignatures(t *testing.T) {
	f := parseTS(t, "interface I {\n  readonly id: string;\n  email?: string;\n  [key: string]: unknown;\n  readonly [i: number]: string;\n  run?(a: number): void;\n}\n"+
		"class K {\n  [k: string]: any;\n  name?: string;\n  private readonly h?: boolean;\n  m?(): void;\n}\n")
	type member struct {
		mods []ir.Modifier
		typ  string
	}
	members := func(children []ir.DistilledNode) map[string]member {
		got := map[string]member{}
		for _, c := range children {
			switch n := c.(type) {
			case *ir.DistilledField:
				got[n.Name] = member{n.Modifiers, n.Type.Name}
			case *ir.DistilledFunction:
				got[n.Name+"()"] = member{n.Modifiers, ""}
			}
		}
		return got
	}
	assert.Equal(t, map[string]member{
		"id":            {[]ir.Modifier{ir.ModifierReadonly}, "string"},
		"email":         {[]ir.Modifier{ir.ModifierOptional}, "string"},
		"[key: string]": {[]ir.Modifier{}, "unknown"},
		"[i: number]":   {[]ir.Modifier{ir.ModifierReadonly}, "string"},
		"run()":         {[]ir.Modifier{ir.ModifierOptional}, ""},
	}, members(f.Children[0].(*ir.DistilledInterface).Children))
	assert.Equal(t, map[string]member{
		"[k: string]": {[]ir.Modifier{}, "any"},
		"name":        {[]ir.Modifier{ir.ModifierOptional}, "string"},
		"h":           {[]ir.Modifier{ir.ModifierReadonly, ir.ModifierOptional}, "boolean"},
		"m()":         {[]ir.Modifier{ir.ModifierOptional}, ""},
	}, members(f.Children[1].(*ir.DistilledClass).Children))
}

// A const initialised with an untyped arrow function is a function with its
// type parameters; a typed arrow function, a function expression and a call
// that takes a function are fields whose DefaultValue is the signature and
// whose Implementation is the initializer as written.
func TestTS_FunctionValuedVariables(t *testing.T) {
	f := parseTS(t, "export const fn = <T,>(x: T): T => x;\n"+
		"const h: H = (req) => req;\n"+
		"const fe = function (a: number) { return a; };\n"+
		"export const B = forwardRef<E, P>(({ v = 1 }, ref) => {\n  return v;\n});\n"+
		"const plain = compose(a, b);\n")
	fn := f.Children[0].(*ir.DistilledFunction)
	assert.Equal(t, "fn", fn.Name)
	assert.Equal(t, []ir.TypeParam{{Name: "T"}}, fn.TypeParams)
	field := func(i int) *ir.DistilledField { return f.Children[i].(*ir.DistilledField) }
	assert.Equal(t, "H", field(1).Type.Name)
	assert.Equal(t, "(req)", field(1).DefaultValue)
	assert.Equal(t, "(req) => req", field(1).Implementation)
	assert.Equal(t, "function (a: number)", field(2).DefaultValue)
	assert.Equal(t, "forwardRef<E, P>(({ v = 1 }, ref))", field(3).DefaultValue)
	assert.Equal(t, "forwardRef<E, P>(({ v = 1 }, ref) => {\n  return v;\n})", field(3).Implementation)
	assert.Equal(t, "compose(a, b)", field(4).DefaultValue)
	assert.Equal(t, "", field(4).Implementation)
}

// A variable keeps the keyword it was declared with: const is final, let and
// var are their own modifiers, for arrow functions and fields alike.
func TestTS_DeclarationKeyword(t *testing.T) {
	f := parseTS(t, "let f = (a: number) => a;\nvar g = () => 1;\nconst h = () => 2;\nlet v = 3;\nvar w = 4;\n")
	mods := map[string][]ir.Modifier{}
	for _, c := range f.Children {
		switch d := c.(type) {
		case *ir.DistilledFunction:
			mods["fn "+d.Name] = d.Modifiers
		case *ir.DistilledField:
			mods[d.Name] = d.Modifiers
		}
	}
	assert.Equal(t, map[string][]ir.Modifier{
		"fn f": {ir.ModifierLet},
		"fn g": {ir.ModifierVar},
		"fn h": {ir.ModifierFinal},
		"v":    {ir.ModifierLet},
		"w":    {ir.ModifierVar},
	}, mods)
}

// A type annotation spread over lines is kept as written on one line, for
// parameters, return types and variables alike.
func TestTS_MultiLineTypesOnOneLine(t *testing.T) {
	f := parseTS(t, "function f({ children }: {\n  children: React.ReactNode;\n}): {\n  value: T;\n} { return x; }\n"+
		"const c: {\n  a: string; // the a\n  b: number;\n} = make();\n")
	fn := f.Children[0].(*ir.DistilledFunction)
	assert.Equal(t, "{ children: React.ReactNode; }", fn.Parameters[0].Type.Name)
	assert.Equal(t, "{ value: T; }", fn.Returns.Name)
	assert.Equal(t, "{ a: string; b: number; }", f.Children[1].(*ir.DistilledField).Type.Name)
}

// A .tsx file whose source holds JSX is marked HasJSX; a .tsx file without
// JSX and a .ts file are not.
func TestTS_HasJSX(t *testing.T) {
	hasJSX := func(src, name string) bool {
		f, err := NewProcessor().Process(context.Background(), strings.NewReader(src), name)
		require.NoError(t, err)
		return f.Extensions != nil && f.Extensions.TypeScript != nil && f.Extensions.TypeScript.HasJSX
	}
	assert.True(t, hasJSX("export function C() { return <div />; }\n", "c.tsx"))
	assert.True(t, hasJSX("export const F = () => <><b>x</b></>;\n", "f.tsx"))
	assert.False(t, hasJSX("export function C(): number { return 1; }\n", "c.tsx"))
	assert.False(t, hasJSX("export const n = <number>x;\n", "n.ts"))
}
