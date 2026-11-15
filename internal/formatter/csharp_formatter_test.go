package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCSharpFormatter_Comments(t *testing.T) {
	doc := func(text string) *ir.DistilledComment { return &ir.DistilledComment{Text: text, Format: ir.CommentDoc} }
	intT := &ir.TypeRef{Name: "int"}
	pub := ir.VisibilityPublic
	class := &ir.DistilledClass{Name: "A", Visibility: pub, Children: []ir.DistilledNode{
		doc("m doc"),
		&ir.DistilledFunction{Name: "M", Visibility: pub, Returns: &ir.TypeRef{Name: "void"}},
		&ir.DistilledComment{Text: "line", Format: ir.CommentLine},
		&ir.DistilledComment{Text: "block", Format: ir.CommentBlock},
		doc("Doc of a delegate, which the formatter does not print."),
		&ir.DistilledTypeAlias{Name: "D"},
		doc("x doc\n\nafter a blank line"),
		&ir.DistilledField{Name: "X", Visibility: pub, Type: intT},
	}}
	record := &ir.DistilledClass{Name: "R", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierData}, Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "P", Type: intT, IsProperty: true, HasGetter: true, Modifiers: []ir.Modifier{ir.ModifierReadonly}},
		doc("Doc of a body property printed as a record parameter."),
		&ir.DistilledField{Name: "Q", Type: intT, IsProperty: true, HasGetter: true, Modifiers: []ir.Modifier{ir.ModifierReadonly}},
	}}
	enum := &ir.DistilledEnum{Name: "E", Visibility: pub, Children: []ir.DistilledNode{
		doc("a doc"),
		&ir.DistilledField{Name: "A", Visibility: pub, Type: &ir.TypeRef{Name: "E"}},
	}}
	got := formatFileAs(t, "csharp",
		&ir.DistilledComment{Text: "line c", Format: ir.CommentLine},
		doc("Doc of a using directive, which is no declaration."),
		&ir.DistilledImport{ImportType: "using", Module: "System"},
		doc("<summary>Doc A</summary>\nsecond"),
		class,
		doc("r doc"),
		record,
		enum,
		doc("Doc of nothing: the last node."),
	)
	assert.Equal(t, `// line c
using System;
/// <summary>Doc A</summary>
/// second
public class A {
    /// m doc
    public void M();
    // line
    /* block */
    /// x doc
    ///
    /// after a blank line
    public int X;
}
/// r doc
public record R(
    int P,
    int Q);
public enum E {
    /// a doc
    A
}
`, got)
}

func TestCSharpFormatter_EnumMembers(t *testing.T) {
	var b strings.Builder
	e := &ir.DistilledEnum{Name: "Color", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		&ir.DistilledField{Name: "Red"}, &ir.DistilledField{Name: "Green", DefaultValue: "2"},
	}}
	require.NoError(t, NewCSharpFormatter().FormatNode(&b, e, 0))
	assert.Equal(t, "public enum Color {\n    Red,\n    Green = 2\n}\n", b.String())
}
