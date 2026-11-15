package formatter

import (
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
)

// Line and block comments print as written; doc comments keep the Javadoc
// layout.
func TestJavaFormatter_Comments(t *testing.T) {
	class := &ir.DistilledClass{Name: "A", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		&ir.DistilledComment{Text: "line", Format: ir.CommentLine},
		&ir.DistilledComment{Text: "block", Format: ir.CommentBlock},
		&ir.DistilledComment{Text: "two\nlines", Format: ir.CommentBlock},
		&ir.DistilledComment{Text: "Doc.", Format: ir.CommentDoc},
		&ir.DistilledField{Name: "x", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "int"}},
	}}
	got := formatFileAs(t, "java", &ir.DistilledComment{Text: "top", Format: ir.CommentLine}, class)
	assert.Equal(t, `// top

public class A {
    // line
    /* block */
    /*
     * two
     * lines
     */
    /**
     * Doc.
     */
    public int x;
}
`, got)
}
