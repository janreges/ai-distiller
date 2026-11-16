package golang

import (
	"context"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseGoFunctions(t *testing.T, src string) map[string]*ir.DistilledFunction {
	t.Helper()
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)
	fns := map[string]*ir.DistilledFunction{}
	for _, node := range file.Children {
		if fn, ok := node.(*ir.DistilledFunction); ok {
			fns[fn.Name] = fn
		}
	}
	return fns
}

func TestASTParser_BodyIsVerbatim(t *testing.T) {
	body := "{\n" +
		"\tif id == 0 {\n" +
		"\t\treturn nil, errNone\n" +
		"\t} else if id < 0 {\n" +
		"\t\tpanic(\"neg\") // never\n" +
		"\t} else {\n" +
		"\t\tid++\n" +
		"\t}\n" +
		"\tfor i := 0; i < 3; i++ {\n" +
		"\t\tgo work(i)\n" +
		"\t}\n" +
		"\treturn &id, nil\n" +
		"}"
	src := "package p\n\n" +
		"func Get(id int) (*int, error) " + body + "\n\n" +
		"func (w *W) One() int { return 1 }\n\n" +
		"func (w *W) Noop() {}\n\n" +
		"func Decl()\n"
	fns := parseGoFunctions(t, src)

	assert.Equal(t, body, fns["Get"].Implementation)
	assert.Equal(t, "{ return 1 }", fns["One"].Implementation)
	assert.Equal(t, "{}", fns["Noop"].Implementation)
	assert.Equal(t, "", fns["Decl"].Implementation, "a function without a body has no implementation")
}

func TestASTParser_InterfaceEmbeddedElements(t *testing.T) {
	src := "package p\n\n" +
		"type Two interface {\n\tio.Reader\n\tio.Closer\n\tExtra() int\n}\n\n" +
		"type Number interface {\n\t~int | ~int64 | float64\n}\n\n" +
		"type Mixed interface {\n\tfmt.Stringer\n\t~string | ~[]byte\n}\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)
	intfs := map[string]*ir.DistilledInterface{}
	for _, node := range file.Children {
		if intf, ok := node.(*ir.DistilledInterface); ok {
			intfs[intf.Name] = intf
		}
	}

	// Each embedded element is one entry, a type-set union stays whole, and
	// the methods are still the children.
	assert.Equal(t, []ir.TypeRef{{Name: "io.Reader"}, {Name: "io.Closer"}}, intfs["Two"].Extends)
	require.Len(t, intfs["Two"].Children, 1)
	assert.Equal(t, "Extra", intfs["Two"].Children[0].(*ir.DistilledFunction).Name)
	assert.Equal(t, []ir.TypeRef{{Name: "~int | ~int64 | float64"}}, intfs["Number"].Extends)
	assert.Equal(t, []ir.TypeRef{{Name: "fmt.Stringer"}, {Name: "~string | ~[]byte"}}, intfs["Mixed"].Extends)
}

func TestASTParser_DefinedTypeAndAlias(t *testing.T) {
	src := "package p\n\ntype Named int\ntype Alias = int\ntype List[T any] []T\ntype Same[T any] = List[T]\n" +
		"type Point struct{ X int }\ntype Pair = struct{ A, B int }\n" +
		"type Closer interface{ Close() error }\ntype Any = interface{}\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)
	mods := map[string][]ir.Modifier{}
	for _, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledTypeAlias:
			mods[n.Name] = n.Modifiers
		case *ir.DistilledClass:
			mods[n.Name] = n.Modifiers
		case *ir.DistilledInterface:
			mods[n.Name] = n.Modifiers
		}
	}

	// Only a declaration with "=" is an alias; a defined type is a new type.
	// A struct or interface literal is no exception.
	assert.Empty(t, mods["Named"])
	assert.Equal(t, []ir.Modifier{ir.ModifierTypeAlias}, mods["Alias"])
	assert.Empty(t, mods["List[T any]"])
	assert.Equal(t, []ir.Modifier{ir.ModifierTypeAlias}, mods["Same[T any]"])
	assert.Empty(t, mods["Point"])
	assert.Equal(t, []ir.Modifier{ir.ModifierTypeAlias}, mods["Pair"])
	assert.Empty(t, mods["Closer"])
	assert.Equal(t, []ir.Modifier{ir.ModifierTypeAlias}, mods["Any"])
}

func TestASTParser_FunctionTypesKeepTheirSignature(t *testing.T) {
	src := "package p\n\n" +
		"func F(a func(x, y int) (n int, err error), b func(), c ...func(func(int) bool) []func() error) func(string) {\n\treturn nil\n}\n\n" +
		"type H func(w io.Writer, n int) error\n\n" +
		"type S struct {\n\tf func(int) string\n}\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)
	var fn *ir.DistilledFunction
	var alias *ir.DistilledTypeAlias
	var field *ir.DistilledField
	for _, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledFunction:
			fn = n
		case *ir.DistilledTypeAlias:
			alias = n
		case *ir.DistilledClass:
			field = n.Children[0].(*ir.DistilledField)
		}
	}

	// Parameter, result, field and type definition all keep the whole
	// function type, names and grouping as written.
	require.Len(t, fn.Parameters, 3)
	assert.Equal(t, "func(x, y int) (n int, err error)", fn.Parameters[0].Type.Name)
	assert.Equal(t, "func()", fn.Parameters[1].Type.Name)
	assert.Equal(t, "...func(func(int) bool) []func() error", fn.Parameters[2].Type.Name)
	assert.Equal(t, "func(string)", fn.Returns.Name)
	assert.Equal(t, "func(w io.Writer, n int) error", alias.Type.Name)
	assert.Equal(t, "func(int) string", field.Type.Name)
}

func TestASTParser_MemberCommentsStayInTheirType(t *testing.T) {
	src := "package p\n\n" +
		"type S struct {\n" +
		"\t// doc of A\n" +
		"\t//\n" +
		"\t// more\n" +
		"\tA int // end of A\n" +
		"\n" +
		"\t// free\n" +
		"\n" +
		"\tInner struct {\n\t\tX int // nested\n\t}\n" +
		"}\n\n" +
		"type I interface {\n" +
		"\tio.Reader // end of Reader\n" +
		"\t// doc of Close\n" +
		"\tClose() error // end of Close\n" +
		"}\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)

	type child struct{ kind, text string }
	describe := func(nodes []ir.DistilledNode) []child {
		var out []child
		for _, n := range nodes {
			switch n := n.(type) {
			case *ir.DistilledComment:
				out = append(out, child{"comment", n.Text})
			case *ir.DistilledField:
				out = append(out, child{"field", n.Name})
			case *ir.DistilledFunction:
				out = append(out, child{"method", n.Name})
			}
		}
		return out
	}
	var s *ir.DistilledClass
	var i *ir.DistilledInterface
	for _, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledClass:
			s = n
		case *ir.DistilledInterface:
			i = n
		case *ir.DistilledComment:
			t.Errorf("a comment of a type body is at file level: %q", n.Text)
		}
	}

	// The doc group is one comment above its field, the comment at the end
	// of a field's line comes right before that field, a free comment keeps
	// its place, and a comment inside a nested struct type is dropped.
	assert.Equal(t, []child{
		{"comment", "doc of A\n\nmore"}, {"comment", "end of A"}, {"field", "A"},
		{"comment", "free"}, {"field", "Inner"},
	}, describe(s.Children))
	assert.Equal(t, []child{
		{"comment", "end of Reader"}, {"comment", "doc of Close"}, {"comment", "end of Close"}, {"method", "Close"},
	}, describe(i.Children))

	// The comment at the end of A's line lies within A's span, so the
	// stripper removes it together with A.
	a, end := s.Children[2].GetLocation(), s.Children[1].GetLocation()
	assert.Equal(t, a.StartLine, end.StartLine)
	assert.LessOrEqual(t, end.EndLine, a.EndLine)
}

func TestASTParser_CommentsInsideADeclarationStayInIt(t *testing.T) {
	src := "package p\n\n" +
		"// Get doc\n" +
		"func Get(id int, // the id\n" +
		"\tname string) int {\n" +
		"\t// in the body\n" +
		"\treturn id /* inline */\n" +
		"} // after Get\n\n" +
		"var handler = func() {\n\t// in a literal\n}\n\n" +
		"type Fn func(\n\ta int, // in a type\n)\n\n" +
		"// free\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)

	var texts []string
	for _, node := range file.Children {
		if c, ok := node.(*ir.DistilledComment); ok {
			texts = append(texts, c.Text)
		}
	}
	// Only the doc and the comments outside every declaration are nodes; the
	// body keeps its comments verbatim.
	assert.Equal(t, []string{"Get doc", "after Get", "free"}, texts)
	assert.Contains(t, parseGoFunctions(t, src)["Get"].Implementation, "// in the body")
}

func TestASTParser_SpecLineCommentsBeforeTheirSpec(t *testing.T) {
	src := "package p\n\n" +
		"const (\n\tA = 1 // one\n\tB, C = 2, 3 // two and three\n)\n\n" +
		"var F = func() {\n} // after F\n\n" +
		"type ID int // an id\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)

	var got []string
	for i, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledComment:
			// Each comment is right before its declaration, inside its span,
			// so that the stripper removes it with the declaration.
			decl := file.Children[i+1].GetLocation()
			assert.Equal(t, decl.EndLine, n.Location.StartLine, n.Text)
			assert.GreaterOrEqual(t, n.Location.StartLine, decl.StartLine, n.Text)
			got = append(got, "// "+n.Text)
		case *ir.DistilledField:
			got = append(got, n.Name)
		case *ir.DistilledTypeAlias:
			got = append(got, n.Name)
		}
	}
	assert.Equal(t, []string{"// one", "A", "B", "// two and three", "C", "// after F", "F", "// an id", "ID"}, got)
}

func TestASTParser_PackageDocText(t *testing.T) {
	src := "// Package p does things.\n" +
		"//\n" +
		"//   - a list item\n" +
		"//\tcode()\n" +
		"package p\n"
	file, err := NewASTParser().ProcessSource(context.Background(), []byte(src), "p.go")
	require.NoError(t, err)

	var texts []string
	for _, node := range file.Children {
		if c, ok := node.(*ir.DistilledComment); ok {
			texts = append(texts, c.Text)
		}
	}
	// "//" and the one space after it go (the formatter prints "// "
	// again); further indentation stays.
	assert.Equal(t, []string{"Package p does things.", "", "  - a list item", "\tcode()"}, texts)
}
