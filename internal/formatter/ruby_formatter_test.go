package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRubyFormatter_Comments(t *testing.T) {
	comment := func(format, text string) *ir.DistilledComment {
		return &ir.DistilledComment{Text: text, Format: format}
	}
	pub := ir.VisibilityPublic
	class := &ir.DistilledClass{Name: "A", Visibility: pub, Children: []ir.DistilledNode{
		comment(ir.CommentLine, "inner"),
		comment(ir.CommentDoc, "m doc"),
		&ir.DistilledFunction{Name: "m", Visibility: pub},
		comment(ir.CommentDoc, "Nested doc"),
		&ir.DistilledClass{Name: "Nested", Visibility: pub},
		comment(ir.CommentBlock, "block\n  more"),
	}}
	got := formatFileAs(t, "ruby",
		comment(ir.CommentLine, "line c"),
		comment(ir.CommentDoc, "Doc c\n\nsecond"),
		class,
		comment(ir.CommentLine, "between"),
		&ir.DistilledInterface{Name: "M", Visibility: pub},
	)
	assert.Equal(t, `# line c

# Doc c
#
# second
class A
    # inner
    # m doc
    def m

    # Nested doc
    class Nested
    # block
    #   more
# between

module M
`, got)
}

// The "#!" interpreter line on the first line prints as written; a comment
// elsewhere keeps the "# " form.
func TestRubyFormatter_Shebang(t *testing.T) {
	comment := func(line int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: line, EndLine: line}}, Text: text, Format: ir.CommentLine}
	}
	got := formatFileAs(t, "ruby", comment(1, "!/usr/bin/env ruby"), comment(2, "!not a shebang"))
	assert.Equal(t, "#!/usr/bin/env ruby\n# !not a shebang\n", got)
}

// The "# private" / "# protected" marker of a method goes above the method's
// doc, so it does not read as a line of the doc.
func TestRubyFormatter_VisibilityMarkerAboveDoc(t *testing.T) {
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	class := &ir.DistilledClass{Name: "A", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		doc("p doc"),
		&ir.DistilledFunction{Name: "p", Visibility: ir.VisibilityPrivate},
		&ir.DistilledFunction{Name: "q", Visibility: ir.VisibilityProtected},
		doc("r doc"),
		&ir.DistilledFunction{Name: "r", Visibility: ir.VisibilityPublic},
	}}
	assert.Equal(t, `
class A
    # private
    # p doc
    def p
    # protected
    def q
    # r doc
    def r
`, formatFileAs(t, "ruby", class))
}

func TestRubyFormatter_VariadicParameter(t *testing.T) {
	for _, name := range []string{"args", "*args"} {
		var b strings.Builder
		fn := &ir.DistilledFunction{Name: "pub", Visibility: ir.VisibilityPublic, Parameters: []ir.Parameter{{Name: name, IsVariadic: true}}}
		require.NoError(t, NewRubyFormatter().FormatNode(&b, fn, 0))
		assert.Equal(t, "def pub(*args)\n", b.String(), name)
	}
}
