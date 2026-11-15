package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/stretchr/testify/assert"
)

func TestWriteCStyleComment(t *testing.T) {
	render := func(text, format, indent string) string {
		var b strings.Builder
		writeCStyleComment(&b, &ir.DistilledComment{Text: text, Format: format}, indent)
		return b.String()
	}
	assert.Equal(t, "// hello\n", render("hello", ir.CommentLine, ""))
	assert.Equal(t, "    // a\n    // b\n", render("a\nb", ir.CommentLine, "    "))
	assert.Equal(t, "/* block */\n", render("block", ir.CommentBlock, ""))
	assert.Equal(t, "/** Doc. */\n", render("Doc.", ir.CommentDoc, ""))
	assert.Equal(t, "    /**\n     * First.\n     * Second.\n     */\n", render("First.\nSecond.", ir.CommentDoc, "    "))

	// Blank lines carry no trailing space; a multi-line block keeps the doc layout.
	assert.Equal(t, "//\n", render("", ir.CommentLine, ""))
	assert.Equal(t, "/**\n * a\n *\n * b\n */\n", render("a\n\nb", ir.CommentDoc, ""))
	assert.Equal(t, "/*\n * a\n * b\n */\n", render("a\nb", ir.CommentBlock, ""))
	// "*/" inside a block or doc would end the comment early.
	assert.Equal(t, "/** a *\\/ b */\n", render("a */ b", ir.CommentDoc, ""))
	assert.Equal(t, "// a */ b\n", render("a */ b", ir.CommentLine, ""))
	// The legacy PHP docblock format is a doc too.
	assert.Equal(t, "/** Doc. */\n", render("Doc.", "docblock", ""))
}

// Stripping the delimiters and writing them again gives the comment back,
// without doubled delimiters or gutters.
func TestCStyleCommentRoundTrip(t *testing.T) {
	for _, raw := range []string{
		"// hello",
		"// a */ b",
		"/* block */",
		"/** Doc. */",
		"/**\n * First.\n *\n * Second.\n */",
		"/*\n * a\n * b\n */",
	} {
		text, format := commentutil.StripCStyle(raw)
		var b strings.Builder
		writeCStyleComment(&b, &ir.DistilledComment{Text: text, Format: format}, "")
		assert.Equal(t, raw+"\n", b.String(), raw)
	}
}
