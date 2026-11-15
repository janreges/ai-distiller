package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPHPFormatter_FunctionBody(t *testing.T) {
	format := func(impl string) string {
		fn := &ir.DistilledFunction{Name: "quick", Visibility: ir.VisibilityPublic,
			Returns: &ir.TypeRef{Name: "int"}, Implementation: impl}
		var b strings.Builder
		require.NoError(t, NewPHPFormatter().FormatNode(&b, fn, 1))
		return b.String()
	}

	// A body that opens and closes on the signature line stays on one line.
	assert.Equal(t, "    public quick(): int { return 1; }\n", format("{ return 1; }"))
	// A block body loses its brace lines and is re-indented under the method.
	assert.Equal(t, "    public quick(): int {\n        return 1;\n    }\n", format("{\n        return 1;\n    }"))
}
