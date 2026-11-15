package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCppFormatter_FunctionBody(t *testing.T) {
	format := func(fn *ir.DistilledFunction, indent int) string {
		var b strings.Builder
		require.NoError(t, NewCppFormatter().FormatNode(&b, fn, indent))
		return b.String()
	}
	params := []ir.Parameter{{Name: "a", Type: ir.TypeRef{Name: "int"}}, {Name: "b", Type: ir.TypeRef{Name: "int"}}}
	ret := &ir.TypeRef{Name: "int"}

	withBody := &ir.DistilledFunction{Name: "add", Parameters: params, Returns: ret, Implementation: "{\n    return a + b;\n}"}
	assert.Equal(t, "int add(int a, int b) {\n    return a + b;\n}\n", format(withBody, 0))

	declOnly := &ir.DistilledFunction{Name: "add", Parameters: params, Returns: ret}
	assert.Equal(t, "int add(int a, int b);\n", format(declOnly, 0))
}

// formatFileAs formats nodes as the children of one file of lang with the
// text formatter, without the <file> lines.
func formatFileAs(t *testing.T, lang string, nodes ...ir.DistilledNode) string {
	t.Helper()
	var b strings.Builder
	file := &ir.DistilledFile{Path: "f", Language: lang, Children: nodes}
	require.NoError(t, NewLanguageAwareTextFormatter(Options{}).Format(&b, file))
	out := strings.TrimPrefix(b.String(), "<file path=\"f\">\n")
	return strings.TrimSuffix(out, "</file>\n")
}

func TestCppFormatter_Comments(t *testing.T) {
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	intT := &ir.TypeRef{Name: "int"}
	class := &ir.DistilledClass{Name: "K", Children: []ir.DistilledNode{
		doc("Doc g."),
		&ir.DistilledFunction{Name: "g", Returns: intT},
		&ir.DistilledComment{Text: "line", Format: ir.CommentLine},
		doc("Doc of a nested class the class body does not print."),
		&ir.DistilledClass{Name: "Nested"},
		&ir.DistilledField{Name: "x", Type: intT},
	}}
	strct := &ir.DistilledStruct{Name: "S", Children: []ir.DistilledNode{
		&ir.DistilledComment{Text: "block", Format: ir.CommentBlock},
		&ir.DistilledField{Name: "y", Type: intT},
	}}
	namespace := &ir.DistilledPackage{Name: "n", Children: []ir.DistilledNode{
		doc("f doc"),
		&ir.DistilledFunction{Name: "f", Returns: &ir.TypeRef{Name: "void"}},
	}}
	got := formatFileAs(t, "cpp",
		&ir.DistilledComment{Text: "line c", Format: ir.CommentLine},
		doc("Class doc.\nSecond line."),
		class,
		strct,
		doc("Namespace doc: the namespace line is not printed, so neither is its doc."),
		namespace,
		&ir.DistilledComment{Text: "@file f.h", Format: ir.CommentDoc, Inner: true},
	)
	assert.Equal(t, `// line c
/**
 * Class doc.
 * Second line.
 */
class K {
    /** Doc g. */
    int g()
    // line
    int x;
};
struct S {
    /* block */
    int y;
};
/** f doc */
void f();
/** @file f.h */
`, got)
}
