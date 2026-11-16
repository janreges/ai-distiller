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
	require.NoError(t, f.(SiblingFormatter).FormatNodes(&b, nodes, 0))
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

func TestGoFormatter_ImportComments(t *testing.T) {
	span := func(line, col int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: line, StartColumn: col, EndLine: line, EndColumn: col + 5}}
	}
	comment := func(line, col int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: span(line, col), Text: text, Format: ir.CommentLine}
	}
	imp := func(line int, module string) *ir.DistilledImport {
		return &ir.DistilledImport{BaseNode: span(line, 2), Module: module, Symbols: []ir.ImportedSymbol{{Name: module}}}
	}

	// package p                 line 1
	// // decl doc               line 3
	// import (                  line 4
	// 	// doc of fmt            line 5
	// 	"fmt" // printing        line 6
	//
	// 	// free                  line 8
	//
	// 	"os"                     line 10
	// )
	// // after                  line 13
	// var V int                 line 15
	nodes := []ir.DistilledNode{
		&ir.DistilledPackage{BaseNode: span(1, 9), Name: "p"},
		comment(3, 1, "decl doc"),
		comment(5, 2, "doc of fmt"),
		comment(6, 8, "printing"),
		imp(6, "fmt"),
		comment(8, 2, "free"),
		imp(10, "os"),
		comment(13, 1, "after"),
		&ir.DistilledField{BaseNode: span(15, 5), Name: "V", Type: &ir.TypeRef{Name: "int"}},
	}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes, 0))
	// The comments directly above an import and those between imports go in
	// the block, the one at the end of an import's line stays there; a
	// comment away from the imports stays out of the block.
	assert.Equal(t, "package p\n// decl doc\n"+
		"\nimport (\n\t// doc of fmt\n\t\"fmt\" // printing\n\t// free\n\t\"os\"\n)\n"+
		"\n// after\n\nvar V int\n", b.String())

	// A file that ends with its imports closes the block.
	b.Reset()
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes[:5], 0))
	assert.True(t, strings.HasSuffix(b.String(), "\t\"fmt\" // printing\n)\n"), b.String())
}

func TestGoFormatter_SpecLineComments(t *testing.T) {
	span := func(from, to, col int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: from, StartColumn: col, EndLine: to, EndColumn: col + 10}}
	}
	comment := func(line, col int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: span(line, line, col), Text: text, Format: ir.CommentLine}
	}

	// const Max = 10 // the limit     line 1
	// // free                         line 2
	// type S struct {                 line 3
	// 	X int
	// } // after S                    line 5
	nodes := []ir.DistilledNode{
		comment(1, 16, "the limit"),
		&ir.DistilledField{BaseNode: span(1, 1, 7), Name: "Max", DefaultValue: "10", Modifiers: []ir.Modifier{ir.ModifierFinal}},
		comment(2, 1, "free"),
		comment(5, 3, "after S"),
		&ir.DistilledClass{BaseNode: span(3, 5, 8), Name: "S", Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "X", Type: &ir.TypeRef{Name: "int"}}}},
	}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes, 0))
	assert.Equal(t, "\nconst Max = 10 // the limit\n// free\n\ntype S struct {\n\tX int\n} // after S\n", b.String())
}

func TestGoFormatter_DefinedTypeAndAlias(t *testing.T) {
	format := func(n ir.DistilledNode) string {
		var b strings.Builder
		require.NoError(t, NewGoFormatter().FormatNode(&b, n, 0))
		return b.String()
	}
	alias := []ir.Modifier{ir.ModifierTypeAlias}
	assert.Equal(t, "\ntype Named int\n", format(&ir.DistilledTypeAlias{Name: "Named", Type: ir.TypeRef{Name: "int"}}))
	assert.Equal(t, "\ntype Alias = int\n", format(&ir.DistilledTypeAlias{Name: "Alias", Type: ir.TypeRef{Name: "int"},
		Modifiers: alias}))

	// An alias of a struct or interface literal keeps its "=" too.
	field := &ir.DistilledField{Name: "X", Type: &ir.TypeRef{Name: "int"}}
	assert.Equal(t, "\ntype Point struct {\n\tX int\n}\n",
		format(&ir.DistilledClass{Name: "Point", Children: []ir.DistilledNode{field}}))
	assert.Equal(t, "\ntype Pair = struct {\n\tX int\n}\n",
		format(&ir.DistilledClass{Name: "Pair", Modifiers: alias, Children: []ir.DistilledNode{field}}))
	assert.Equal(t, "\ntype Any = interface {}\n", format(&ir.DistilledInterface{Name: "Any", Modifiers: alias}))
}
