package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPythonFormatter_DocCommentPlacement(t *testing.T) {
	format := func(nodes ...ir.DistilledNode) string {
		var b strings.Builder
		require.NoError(t, NewPythonFormatter().FormatNodes(&b, nodes, 0))
		return b.String()
	}
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	fn := func(impl string) *ir.DistilledFunction {
		return &ir.DistilledFunction{Name: "f", Visibility: ir.VisibilityPublic, Decorators: []string{"cache"}, Implementation: impl}
	}

	assert.Equal(t, "@cache\nf()\n    \"\"\"F doc.\n\n    More.\n    \"\"\"\n", format(doc("F doc.\n\nMore."), fn("")),
		"a function's doc goes under its signature, one level deeper")
	assert.Equal(t, "@cache\ndef f():\n    \"\"\"F doc.\"\"\"\n    return 1\n", format(doc("F doc."), fn("return 1")),
		"and before the body")

	inner := &ir.DistilledComment{Text: "Module doc.", Format: ir.CommentDoc, Inner: true}
	assert.Equal(t, "\"\"\"Module doc.\"\"\"\n@cache\nf()\n", format(inner, fn("")), "an inner doc stays where it is")

	field := &ir.DistilledField{Name: "X", Visibility: ir.VisibilityPublic, DefaultValue: "1"}
	assert.Equal(t, "\"\"\"Lone.\"\"\"\nX = 1\n", format(doc("Lone."), field), "a doc before something else prints as before")
	assert.Equal(t, "@cache\nf()\n\"\"\"Trailing.\"\"\"\n", format(fn(""), doc("Trailing.")), "a doc with nothing after it is still printed")

	var b strings.Builder
	require.NoError(t, NewPythonFormatter().FormatNode(&b, &ir.DistilledClass{Name: "C", Visibility: ir.VisibilityPublic,
		Children: []ir.DistilledNode{inner, doc("M doc."), &ir.DistilledFunction{Name: "m", Visibility: ir.VisibilityPublic}}}, 0))
	assert.Equal(t, "\nclass C:\n    \"\"\"Module doc.\"\"\"\n    m()\n        \"\"\"M doc.\"\"\"\n", b.String(), "the same inside a class")
}

// The "#!" interpreter line on the first line prints as written; a comment
// elsewhere keeps the "# " form.
func TestPythonFormatter_Shebang(t *testing.T) {
	comment := func(line int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: line, EndLine: line}}, Text: text, Format: ir.CommentLine}
	}
	var b strings.Builder
	require.NoError(t, NewPythonFormatter().FormatNodes(&b, []ir.DistilledNode{comment(1, "!/usr/bin/env python"), comment(2, "!not a shebang")}, 0))
	assert.Equal(t, "#!/usr/bin/env python\n# !not a shebang\n", b.String())
}
