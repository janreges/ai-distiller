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
