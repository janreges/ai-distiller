package esutil

import (
	"context"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "tree-sitter-typescript"
)

// firstStatement parses src with the TSX grammar (or JavaScript's when js is
// set) and calls fn with the program's first statement.
func firstStatement(t *testing.T, src string, js bool, fn func(stmt *sitter.Node, source []byte)) {
	t.Helper()
	lang := sitter.NewLanguage(typescript.LanguageTSX())
	if js {
		lang = sitter.NewLanguage(tree_sitter_javascript.Language())
	}
	parser, err := tsparse.NewParser(lang)
	require.NoError(t, err)
	defer parser.Close()
	tree, err := tsparse.Parse(context.Background(), parser, []byte(src))
	require.NoError(t, err)
	defer tree.Close()
	stmt := tree.RootNode().NamedChild(0)
	require.NotNil(t, stmt)
	fn(stmt, []byte(src))
}

// initializer returns the value of the first declarator of a const statement.
func initializer(stmt *sitter.Node) *sitter.Node {
	return stmt.NamedChild(0).ChildByFieldName("value")
}

func TestSignature(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"const a = forwardRef<HTMLButtonElement, ButtonProps>(({ variant = \"a\", ...props }, ref) => {\n  return <button ref={ref} {...props} />;\n});",
			`forwardRef<HTMLButtonElement, ButtonProps>(({ variant = "a", ...props }, ref))`},
		{"const a = memo(function List<T>({ items }: { items: T[] }) { return <ul/>; });",
			"memo(function List<T>({ items }: { items: T[] }))"},
		{"const a = <T,>(x: T): T => x;", "<T,>(x: T): T"},
		{"const a = async (endpoint: string) => { await get(endpoint); };", "async (endpoint: string)"},
		{"const a = x => x * 2;", "(x)"},
		{"const a = { setToken: _token => { token = _token; } };", "{ setToken: (_token) }"},
		{"const a = connect(async state => state.a, () => ({}))(View);", "connect(async (state), ())(View)"},
		{"const a = useCallback((e: Event) => {\n  go(e);\n}, [go]);", "useCallback((e: Event), [go])"},
		{"const a = connect(\n  mapState, // state\n  mapDispatch,\n)(function View() { return null; });",
			"connect(mapState, mapDispatch)(function View())"},
		{"const a = styled.div(({ theme }) => ({ color: theme.c }));", "styled.div(({ theme }))"},
		{"const a = { get() { return 1; }, b: 2 };", "{ get(), b: 2 }"},
		{"const a = tag(`line one\n  line two`, (x) => x);", "tag(`line one\n  line two`, (x))"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			firstStatement(t, tc.src, false, func(stmt *sitter.Node, source []byte) {
				assert.Equal(t, tc.want, Signature(initializer(stmt), source))
			})
		})
	}
}

func TestContainsJSX(t *testing.T) {
	for src, want := range map[string]bool{
		"const a = <div />;":                           true,
		"function f() { if (x) { return <A>t</A>; } }": true,
		"const b = <>x</>;":                            true,
		"const c = 1 < 2 && d > 3;":                    false,
	} {
		firstStatement(t, src, false, func(stmt *sitter.Node, _ []byte) {
			assert.Equal(t, want, ContainsJSX(stmt), src)
		})
	}
}

func TestDeclarationModifier(t *testing.T) {
	for src, want := range map[string]ir.Modifier{
		"const a = 1;":     ir.ModifierFinal,
		"let b = 2, c;":    ir.ModifierLet,
		"var d = () => 1;": ir.ModifierVar,
	} {
		for _, js := range []bool{false, true} {
			firstStatement(t, src, js, func(stmt *sitter.Node, _ []byte) {
				assert.Equal(t, want, DeclarationModifier(stmt), src)
			})
		}
	}
	assert.Equal(t, ir.Modifier(""), DeclarationModifier(nil))
}

func TestTakesFunction(t *testing.T) {
	for src, want := range map[string]bool{
		"const a = forwardRef((p, r) => null);":         true,
		"const a = memo(function X() {});":              true,
		"const a = connect(m)(function X() {});":        true,
		"const a = (memo((x) => x) as Typed);":          true,
		"const a = useMemo(() => 1, []);":               true,
		"const a = new Pool(() => ({ data: null }));":   true,
		"const a = new Map();":                          false,
		"const a = configure({ reducer: (s) => s });":   false,
		"const a = (x) => x;":                           false,
		"const a = compose(a, b);":                      false,
		"const a = [1, 2].map((n) => n * 2).join(',');": false,
	} {
		t.Run(src, func(t *testing.T) {
			firstStatement(t, src, false, func(stmt *sitter.Node, _ []byte) {
				assert.Equal(t, want, TakesFunction(initializer(stmt)))
			})
		})
	}
}

func TestExport(t *testing.T) {
	sym := func(name, alias string) ir.ImportedSymbol { return ir.ImportedSymbol{Name: name, Alias: alias} }
	for _, tc := range []struct {
		src  string
		want *ir.DistilledExport // nil: not an export node
	}{
		{`export * from "./a";`, &ir.DistilledExport{Module: "./a", Symbols: []ir.ImportedSymbol{sym("*", "")}}},
		{`export * as ns from "./b";`, &ir.DistilledExport{Module: "./b", Symbols: []ir.ImportedSymbol{sym("*", "ns")}}},
		{`export { x, y as z } from './c';`, &ir.DistilledExport{Module: "./c", Symbols: []ir.ImportedSymbol{sym("x", ""), sym("y", "z")}}},
		{`export { q };`, &ir.DistilledExport{Symbols: []ir.ImportedSymbol{sym("q", "")}}},
		{`export type { T } from "./t";`, &ir.DistilledExport{Module: "./t", IsType: true, Symbols: []ir.ImportedSymbol{sym("T", "")}}},
		{`export { type P, V };`, &ir.DistilledExport{Symbols: []ir.ImportedSymbol{{Name: "P", IsType: true}, sym("V", "")}}},
		{`export default Inner;`, &ir.DistilledExport{IsDefault: true, Value: "Inner"}},
		{"export default connect(\n  a,\n  b,\n)(View);", &ir.DistilledExport{IsDefault: true, Value: "connect(a, b)(View)"}},
		{"export default memo(function L() {\n  return 1;\n});", &ir.DistilledExport{IsDefault: true, Value: "memo(function L())"}},
		{`export = Foo;`, &ir.DistilledExport{Value: "Foo"}},
		{`export as namespace Lib;`, &ir.DistilledExport{Namespace: "Lib"}},
		// Declarations and class expressions are the processor's; a long
		// value without a function is left out.
		{`export class K {}`, nil},
		{`export default function main() {}`, nil},
		{`export default class {}`, nil},
		{"export default {\n  a: 'aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa',\n  b: 'bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb',\n};", nil},
		// `export =` has the same cap.
		{"export = {\n  alpha: 'aaaaaaaaaaaaaaaaaaaa',\n  beta: 'bbbbbbbbbbbbbbbbbbbbbbbbbb',\n  gamma: 'cccccccccccccccccccccccccc',\n  delta: [1, 2, 3, 4, 5, 6, 7, 8, 9, 10],\n};", nil},
		{"export = connect(\n  aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa,\n  bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb,\n)(function V() {});",
			&ir.DistilledExport{Value: "connect(aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa, bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb)(function V())"}},
	} {
		t.Run(tc.src, func(t *testing.T) {
			firstStatement(t, tc.src, false, func(stmt *sitter.Node, source []byte) {
				got := Export(stmt, source)
				if tc.want == nil {
					assert.Nil(t, got)
					return
				}
				require.NotNil(t, got)
				got.BaseNode = ir.BaseNode{}
				assert.Equal(t, tc.want, got)
			})
		})
	}
	// The JavaScript grammar gives the same nodes.
	firstStatement(t, `export { x as y } from "./c";`, true, func(stmt *sitter.Node, source []byte) {
		got := Export(stmt, source)
		require.NotNil(t, got)
		assert.Equal(t, []ir.ImportedSymbol{sym("x", "y")}, got.Symbols)
	})
}

func TestLocalNames(t *testing.T) {
	assert.Equal(t, []string{"q", "r"}, LocalNames(&ir.DistilledExport{Symbols: []ir.ImportedSymbol{{Name: "q"}, {Name: "r", Alias: "s"}}}))
	assert.Empty(t, LocalNames(&ir.DistilledExport{Module: "./m", Symbols: []ir.ImportedSymbol{{Name: "q"}}}))
	assert.Equal(t, []string{"Inner"}, LocalNames(&ir.DistilledExport{IsDefault: true, Value: "Inner"}))
	assert.Empty(t, LocalNames(&ir.DistilledExport{IsDefault: true, Value: "connect(a)(B)"}))
}
