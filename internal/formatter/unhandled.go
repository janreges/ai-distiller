package formatter

import "github.com/janreges/ai-distiller/internal/ir"

// OnUnhandledNode, when non-nil, is called for every IR node that a text
// formatter has no rendering for and therefore drops from the output.
// It is nil by default; the CLI sets it in verbose mode and tests set it to
// detect silently skipped node kinds.
var OnUnhandledNode func(lang string, node ir.DistilledNode)

// reportUnhandled notifies OnUnhandledNode, if set, that node was skipped.
func reportUnhandled(lang string, node ir.DistilledNode) {
	if OnUnhandledNode != nil {
		OnUnhandledNode(lang, node)
	}
}
