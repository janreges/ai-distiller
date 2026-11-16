package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGoFormatter_FunctionBody(t *testing.T) {
	format := func(fn *ir.DistilledFunction) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, fn, 0))
		return b.String()
	}
	body := "{\n\tif id == 0 {\n\t\treturn nil\n\t}\n\treturn &id\n}"
	ret := &ir.TypeRef{Name: "*int"}

	fn := &ir.DistilledFunction{Name: "Get", Returns: ret, Implementation: body,
		Parameters: []ir.Parameter{{Name: "id", Type: ir.TypeRef{Name: "int"}}}}
	assert.Equal(t, "\nfunc Get(id int) *int {\n\tif id == 0 {\n\t\treturn nil\n\t}\n\treturn &id\n}\n", format(fn))

	method := &ir.DistilledFunction{Name: "One", Modifiers: []ir.Modifier{ir.ModifierAbstract},
		Returns:        &ir.TypeRef{Name: "int"},
		Parameters:     []ir.Parameter{{Name: "w", Type: ir.TypeRef{Name: "*W"}}},
		Implementation: "{ return 1 }"}
	assert.Equal(t, "func (w *W) One() int { return 1 }\n", format(method))

	decl := &ir.DistilledFunction{Name: "Get", Returns: ret}
	assert.Equal(t, "\nfunc Get() *int\n", format(decl))
}

func TestGoFormatter_CommentInsidePrintedBody(t *testing.T) {
	// func Get() {     line 1
	// 	// inner        line 2
	// 	return
	// } // after       line 4
	loc := ir.Location{StartLine: 1, StartColumn: 1, EndLine: 4, EndColumn: 2}
	inner := &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: 2, StartColumn: 2}}, Text: "inner", Format: "inline"}
	after := &ir.DistilledComment{BaseNode: ir.BaseNode{Location: ir.Location{StartLine: 4, StartColumn: 3}}, Text: "after", Format: "inline"}
	format := func(impl string) string {
		f := NewGoFormatter()
		fn := &ir.DistilledFunction{BaseNode: ir.BaseNode{Location: loc}, Name: "Get", Implementation: impl}
		var b strings.Builder
		for _, n := range []ir.DistilledNode{fn, inner, after} {
			require.NoError(t, f.FormatNode(&b, n, 0))
		}
		return b.String()
	}

	// The printed body already holds the inner comment; it is not repeated.
	assert.Equal(t, "\nfunc Get() {\n\t// inner\n\treturn\n}\n// after\n", format("{\n\t// inner\n\treturn\n}"))
	// Without the body the comment node is the only place the text appears.
	assert.Equal(t, "\nfunc Get()\n// inner\n// after\n", format(""))
}

func TestGoFormatter_NestedBodyIndentation(t *testing.T) {
	body := "{\n\tfor i := 0; i < 3; i++ {\n\t\tif i == 1 {\n\t\t\tcontinue\n\t\t}\n\t}\n}"
	fn := &ir.DistilledFunction{Name: "Loop", Implementation: body}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().FormatNode(&b, fn, 0))
	// The body keeps its source tabs: the output indents with tabs, as
	// gofmt does, and is never larger than the source.
	assert.Equal(t, "\nfunc Loop() "+body+"\n", b.String())
}

func TestGoFormatter_IndentsWithTabs(t *testing.T) {
	var b strings.Builder
	f := NewGoFormatter()
	nodes := []ir.DistilledNode{
		&ir.DistilledImport{Module: "io", Symbols: []ir.ImportedSymbol{{Name: "io"}}},
		&ir.DistilledImport{Module: "math", Symbols: []ir.ImportedSymbol{{Name: "math", Alias: "m"}}},
		&ir.DistilledClass{Name: "S", Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "R", Type: &ir.TypeRef{Name: "io.Reader"}},
		}},
		&ir.DistilledInterface{Name: "I", Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "Close", Returns: &ir.TypeRef{Name: "error"}},
		}},
	}
	for _, n := range nodes {
		require.NoError(t, f.FormatNode(&b, n, 0))
	}
	assert.Equal(t, "\nimport (\n\t\"io\"\n\tm \"math\"\n)\n"+
		"\ntype S struct {\n\tR io.Reader\n}\n"+
		"\ntype I interface {\n\tClose() error\n}\n", b.String())
}

func TestGoFormatter_InterfaceEmbedsAndMethods(t *testing.T) {
	format := func(intf *ir.DistilledInterface) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, intf, 0))
		return b.String()
	}
	extra := &ir.DistilledFunction{Name: "Extra", Returns: &ir.TypeRef{Name: "int"}}

	// Embedded elements one per line, then the methods; none is lost.
	assert.Equal(t, "\ntype Two interface {\n\tio.Reader\n\tio.Closer\n\tExtra() int\n}\n",
		format(&ir.DistilledInterface{Name: "Two",
			Extends:  []ir.TypeRef{{Name: "io.Reader"}, {Name: "io.Closer"}},
			Children: []ir.DistilledNode{extra}}))
	// A type-set union is one element and keeps its "|".
	assert.Equal(t, "\ntype Number interface {\n\t~int | ~string\n}\n",
		format(&ir.DistilledInterface{Name: "Number", Extends: []ir.TypeRef{{Name: "~int | ~string"}}}))
	assert.Equal(t, "\ntype Any interface {}\n", format(&ir.DistilledInterface{Name: "Any"}))
}

func TestGoFormatter_MemberComments(t *testing.T) {
	at := func(line, col int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: line, StartColumn: col, EndLine: line, EndColumn: col + 10}}
	}
	comment := func(line, col int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: at(line, col), Text: text, Format: ir.CommentLine}
	}
	format := func(n ir.DistilledNode) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, n, 0))
		return b.String()
	}

	// type S struct {          line 1
	// 	// doc of A            line 2
	// 	A int // end of A      line 3
	// 	// free                line 4
	// }
	s := &ir.DistilledClass{Name: "S", Children: []ir.DistilledNode{
		comment(2, 2, "doc of A"),
		comment(3, 8, "end of A"),
		&ir.DistilledField{BaseNode: at(3, 2), Name: "A", Type: &ir.TypeRef{Name: "int"}},
		comment(4, 2, "free"),
	}}
	assert.Equal(t, "\ntype S struct {\n\t// doc of A\n\tA int // end of A\n\t// free\n}\n", format(s))

	// type I interface {           line 1
	// 	// doc of Reader           line 2
	// 	io.Reader                  line 3
	// 	io.Writer // end of Writer line 4
	// 	// doc of Close            line 5
	// 	Close() error // end       line 6
	// }
	// The comments among the embedded types go above them, the method's
	// own comments stay with it.
	i := &ir.DistilledInterface{Name: "I",
		Extends: []ir.TypeRef{{Name: "io.Reader"}, {Name: "io.Writer"}},
		Children: []ir.DistilledNode{
			comment(2, 2, "doc of Reader"),
			comment(4, 12, "end of Writer"),
			comment(5, 2, "doc of Close"),
			comment(6, 16, "end"),
			&ir.DistilledFunction{BaseNode: at(6, 2), Name: "Close", Returns: &ir.TypeRef{Name: "error"}},
		}}
	assert.Equal(t, "\ntype I interface {\n\t// doc of Reader\n\t// end of Writer\n\tio.Reader\n\tio.Writer\n"+
		"\t// doc of Close\n\tClose() error // end\n}\n", format(i))
}

func TestGoFormatter_DefinedTypeAndAlias(t *testing.T) {
	format := func(a *ir.DistilledTypeAlias) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, a, 0))
		return b.String()
	}
	assert.Equal(t, "\ntype Named int\n", format(&ir.DistilledTypeAlias{Name: "Named", Type: ir.TypeRef{Name: "int"}}))
	assert.Equal(t, "\ntype Alias = int\n", format(&ir.DistilledTypeAlias{Name: "Alias", Type: ir.TypeRef{Name: "int"},
		Modifiers: []ir.Modifier{ir.ModifierTypeAlias}}))
}
