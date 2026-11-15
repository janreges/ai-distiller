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

func TestKotlinFormatter_Comments(t *testing.T) {
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	intT := &ir.TypeRef{Name: "Int"}
	class := &ir.DistilledClass{Name: "K", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		&ir.DistilledComment{Text: "line", Format: ir.CommentLine},
		doc("g doc"),
		&ir.DistilledFunction{Name: "g", Visibility: ir.VisibilityPublic, Returns: intT},
		doc("Doc of a nested interface the class body does not print."),
		&ir.DistilledInterface{Name: "Nested"},
		doc("Object doc."),
		&ir.DistilledClass{Name: "O", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierStatic}, Children: []ir.DistilledNode{
			doc("x doc"),
			&ir.DistilledField{Name: "x", Visibility: ir.VisibilityPublic, Type: intT, Modifiers: []ir.Modifier{ir.ModifierFinal}},
		}},
	}}
	iface := &ir.DistilledInterface{Name: "I", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		doc("p doc: interface properties are not printed"),
		&ir.DistilledField{Name: "p", Visibility: ir.VisibilityPublic, Type: intT},
		doc("h doc"),
		&ir.DistilledFunction{Name: "h", Visibility: ir.VisibilityPublic},
	}}
	got := formatFileAs(t, "kotlin",
		doc("Package doc: the package line is not printed, so neither is its doc."),
		&ir.DistilledPackage{Name: "a.b"},
		&ir.DistilledComment{Text: "line c", Format: ir.CommentLine},
		doc("Doc c"),
		&ir.DistilledFunction{Name: "f", Visibility: ir.VisibilityPublic, Returns: intT},
		&ir.DistilledComment{Text: "multi\nline", Format: ir.CommentBlock},
		class,
		iface,
	)
	assert.Equal(t, `// line c
/** Doc c */
fun f(): Int
/*
 * multi
 * line
 */
class K {
    // line
    /** g doc */
    fun g(): Int
    /** Object doc. */
    object O {
        /** x doc */
        val x: Int
    }
}
interface I {
    /** h doc */
    fun h()
}
`, got)
}
