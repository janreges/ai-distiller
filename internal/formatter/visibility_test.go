package formatter

import (
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
)

func TestGetVisibilityPrefix(t *testing.T) {
	tests := []struct {
		visibility ir.Visibility
		expected   string
	}{
		{ir.VisibilityPublic, ""},              // No prefix for public
		{ir.VisibilityPrivate, "-"},            // Private
		{ir.VisibilityProtected, "*"},          // Protected
		{ir.VisibilityInternal, "~"},           // UML package/internal
		{ir.VisibilityFilePrivate, "-"},        // Swift fileprivate -> similar to private
		{ir.VisibilityOpen, ""},                // Swift open -> treat as public
		{ir.VisibilityProtectedInternal, "*~"}, // C# protected internal -> combination
		{ir.VisibilityPrivateProtected, "-*"},  // C# private protected -> combination
		{"", ""},                               // Default to public (no prefix)
	}

	for _, tt := range tests {
		t.Run(string(tt.visibility), func(t *testing.T) {
			result := getVisibilityPrefix(tt.visibility)
			assert.Equal(t, tt.expected, result)
		})
	}
}
