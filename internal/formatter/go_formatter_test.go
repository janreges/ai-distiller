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
	assert.Equal(t, "\nfunc (w *W) One() int { return 1 }\n", format(method))

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
	declDoc := comment(3, 1, "decl doc")
	declDoc.Extensions = &ir.NodeExtensions{Go: &ir.GoExtensions{ImportDoc: true}}
	nodes := []ir.DistilledNode{
		&ir.DistilledPackage{BaseNode: span(1, 9), Name: "p"},
		declDoc,
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
	// the block, the one at the end of an import's line stays there; the
	// declaration's doc sits on the block, and a comment away from the
	// imports stays out of it.
	assert.Equal(t, "package p\n\n// decl doc\n"+
		"import (\n\t// doc of fmt\n\t\"fmt\" // printing\n\t// free\n\t\"os\"\n)\n"+
		"\n// after\n\nvar V int\n", b.String())

	// A file that ends with its imports closes the block.
	b.Reset()
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes[:5], 0))
	assert.True(t, strings.HasSuffix(b.String(), "\t\"fmt\" // printing\n)\n"), b.String())

	// A comment before the imports that is not the declaration's doc (a
	// file header set off by a blank line) stays a free comment, with its
	// blank line, and is set off from the doc after it.
	//   without a doc:          with one:
	//   package p     line 1    package p      line 1
	//   // header     line 3    // header      line 3
	//   import "fmt"  line 5    // decl doc    line 5
	//                           import (       line 6
	//                           	"fmt"       line 7
	for _, doc := range []bool{false, true} {
		nodes = []ir.DistilledNode{&ir.DistilledPackage{BaseNode: span(1, 9), Name: "p"}, comment(3, 1, "header")}
		want := "package p\n\n// header\n\nimport (\n\t\"fmt\"\n)\n"
		if doc {
			declDoc = comment(5, 1, "decl doc")
			declDoc.Extensions = &ir.NodeExtensions{Go: &ir.GoExtensions{ImportDoc: true}}
			nodes = append(nodes, declDoc, imp(7, "fmt"))
			want = "package p\n\n// header\n\n// decl doc\nimport (\n\t\"fmt\"\n)\n"
		} else {
			nodes = append(nodes, &ir.DistilledImport{BaseNode: span(5, 8), Module: "fmt", Symbols: []ir.ImportedSymbol{{Name: "fmt"}}})
		}
		b.Reset()
		require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes, 0))
		assert.Equal(t, want, b.String(), "with an import doc: %v", doc)
	}
}

func TestGoFormatter_SpecLineComments(t *testing.T) {
	span := func(from, to, col int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: from, StartColumn: col, EndLine: to, EndColumn: col + 10}}
	}
	comment := func(line, col int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: span(line, line, col), Text: text, Format: ir.CommentLine}
	}

	// const Max = 10 // the limit     line 1
	//
	// // free                         line 3
	//
	// type S struct {                 line 5
	// 	X int
	// } // after S                    line 7
	nodes := []ir.DistilledNode{
		comment(1, 16, "the limit"),
		&ir.DistilledField{BaseNode: span(1, 1, 7), Name: "Max", DefaultValue: "10", Modifiers: []ir.Modifier{ir.ModifierFinal}},
		comment(3, 1, "free"),
		comment(7, 3, "after S"),
		&ir.DistilledClass{BaseNode: span(5, 7, 8), Name: "S", Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "X", Type: &ir.TypeRef{Name: "int"}}}},
	}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes, 0))
	assert.Equal(t, "\nconst Max = 10 // the limit\n\n// free\n\ntype S struct {\n\tX int\n} // after S\n", b.String())
}

func TestGoFormatter_DocSitsOnItsDeclaration(t *testing.T) {
	at := func(line int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: line, StartColumn: 1, EndLine: line, EndColumn: 10}}
	}
	doc := func(line int, text string) *ir.DistilledComment {
		return &ir.DistilledComment{BaseNode: at(line), Text: text, Format: ir.CommentDoc}
	}
	free := &ir.DistilledComment{BaseNode: at(9), Text: "free", Format: ir.CommentLine}
	method := &ir.DistilledFunction{BaseNode: at(7), Name: "M", Modifiers: []ir.Modifier{ir.ModifierAbstract},
		Parameters: []ir.Parameter{{Name: "r", Type: ir.TypeRef{Name: "*R"}}}}

	// package p            line 1
	// // F doc             line 3
	// func F()             line 4
	// // M doc             line 6
	// func (r *R) M()      line 7
	// // free              line 9
	// // T doc             line 11
	// type T int           line 12
	nodes := []ir.DistilledNode{
		&ir.DistilledPackage{BaseNode: at(1), Name: "p"},
		doc(3, "F doc"), &ir.DistilledFunction{BaseNode: at(4), Name: "F"},
		doc(6, "M doc"), method,
		free,
		doc(11, "T doc"), &ir.DistilledTypeAlias{BaseNode: at(12), Name: "T", Type: ir.TypeRef{Name: "int"}},
	}
	var b strings.Builder
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, nodes, 0))
	// One blank line between declarations, a method included, above the doc;
	// none between a doc and its declaration.
	assert.Equal(t, "package p\n\n// F doc\nfunc F()\n\n// M doc\nfunc (r *R) M()\n\n// free\n\n// T doc\ntype T int\n", b.String())

	// The comment at the end of the package clause's or a one-line
	// function's line (the parser puts it right before the node) prints at
	// the end of that line, not as the next function's doc.
	// package p // import "x"     line 1
	// func A() int { ... } // a   line 3
	// func B()                    line 4
	at3 := func(line, col int) ir.BaseNode {
		return ir.BaseNode{Location: ir.Location{StartLine: line, StartColumn: col, EndLine: line, EndColumn: col + 20}}
	}
	b.Reset()
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b, []ir.DistilledNode{
		&ir.DistilledComment{BaseNode: at3(1, 11), Text: "import \"x\"", Format: ir.CommentLine},
		&ir.DistilledPackage{BaseNode: at3(1, 9), Name: "p"},
		&ir.DistilledComment{BaseNode: at3(3, 28), Text: "a", Format: ir.CommentLine},
		&ir.DistilledFunction{BaseNode: at3(3, 1), Name: "A", Returns: &ir.TypeRef{Name: "int"}},
		&ir.DistilledFunction{BaseNode: at3(4, 1), Name: "B"},
	}, 0))
	assert.Equal(t, "package p // import \"x\"\n\nfunc A() int // a\n\nfunc B()\n", b.String())

	// A comment set off from the package doc by a blank line (a copyright
	// header) stays set off from it.
	// // Copyright           line 1
	// // Package p doc       line 3
	// package p              line 4
	header := &ir.DistilledComment{BaseNode: at(1), Text: "Copyright", Format: ir.CommentLine}
	pkgDoc := &ir.DistilledComment{BaseNode: at(3), Text: "Package p doc"}
	b.Reset()
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b,
		[]ir.DistilledNode{header, pkgDoc, &ir.DistilledPackage{BaseNode: at(4), Name: "p"}}, 0))
	assert.Equal(t, "// Copyright\n\n// Package p doc\npackage p\n", b.String())

	// Without a package doc, too: the header does not stand on the package
	// clause as if it were its doc.
	b.Reset()
	require.NoError(t, NewGoFormatter().(SiblingFormatter).FormatNodes(&b,
		[]ir.DistilledNode{header, &ir.DistilledPackage{BaseNode: at(3), Name: "p"}}, 0))
	assert.Equal(t, "// Copyright\n\npackage p\n", b.String())
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
