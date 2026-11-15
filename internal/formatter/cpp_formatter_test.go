package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCppFormatter_FunctionBody(t *testing.T) {
	format := func(fn *ir.DistilledFunction, indent int) string {
		var b strings.Builder
		require.NoError(t, NewCppFormatter().FormatNode(&b, fn, indent))
		return b.String()
	}
	params := []ir.Parameter{{Name: "a", Type: ir.TypeRef{Name: "int"}}, {Name: "b", Type: ir.TypeRef{Name: "int"}}}
	ret := &ir.TypeRef{Name: "int"}

	withBody := &ir.DistilledFunction{Name: "add", Parameters: params, Returns: ret, Implementation: "{\n    return a + b;\n}"}
	assert.Equal(t, "int add(int a, int b) {\n    return a + b;\n}\n", format(withBody, 0))

	declOnly := &ir.DistilledFunction{Name: "add", Parameters: params, Returns: ret}
	assert.Equal(t, "int add(int a, int b);\n", format(declOnly, 0))
}
