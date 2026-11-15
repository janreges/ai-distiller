package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSwiftFormatter_StructAndExtensionHeaders(t *testing.T) {
	render := func(c *ir.DistilledClass) string {
		var b strings.Builder
		require.NoError(t, NewSwiftFormatter().FormatNode(&b, c, 0))
		return b.String()
	}
	assert.Contains(t, render(&ir.DistilledClass{Name: "P", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierStruct}}), "public struct P")
	ext := render(&ir.DistilledClass{Name: "extension P: Codable", Visibility: ir.VisibilityInternal})
	assert.Contains(t, ext, "extension P: Codable")
	assert.NotContains(t, ext, "class")
}
