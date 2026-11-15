package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKotlinFormatter_Bodies(t *testing.T) {
	format := func(fn *ir.DistilledFunction) string {
		var b strings.Builder
		require.NoError(t, NewKotlinFormatter().FormatNode(&b, fn, 0))
		return b.String()
	}
	base := ir.DistilledFunction{Name: "find", Visibility: ir.VisibilityPublic,
		Parameters: []ir.Parameter{{Name: "id", Type: ir.TypeRef{Name: "Int"}}}, Returns: &ir.TypeRef{Name: "String"}}

	block := base
	block.Implementation = "{\n        val x = load(id)\n        return x\n    }"
	assert.Equal(t, "fun find(id: Int): String {\n    val x = load(id)\n    return x\n}\n", format(&block))

	expr := base
	expr.Implementation = "= load(id)"
	assert.Equal(t, "fun find(id: Int): String = load(id)\n", format(&expr))
}
