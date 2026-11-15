package formatter

import "github.com/janreges/ai-distiller/internal/ir"

// getVisibilityPrefix converts visibility to UML-style prefix
func getVisibilityPrefix(vis ir.Visibility) string {
	// Prefix style used by the Python formatter; the language-aware text
	// formatter uses full keywords instead.
	switch vis {
	case ir.VisibilityPublic:
		return "" // No prefix for public
	case ir.VisibilityPrivate:
		return "-" // Private
	case ir.VisibilityProtected:
		return "*" // Protected
	case ir.VisibilityInternal:
		return "~" // UML package/internal
	case ir.VisibilityFilePrivate:
		return "-" // Swift fileprivate -> similar to private
	case ir.VisibilityOpen:
		return "" // Swift open -> treat as public
	case ir.VisibilityProtectedInternal:
		return "*~" // C# protected internal -> combination
	case ir.VisibilityPrivateProtected:
		return "-*" // C# private protected -> combination
	default:
		return "" // Default to public (no prefix)
	}
}
