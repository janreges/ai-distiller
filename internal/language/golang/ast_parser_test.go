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
