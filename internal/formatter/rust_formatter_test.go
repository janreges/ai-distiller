package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRustFormatter_ConstKeepsItsName(t *testing.T) {
	var b strings.Builder
	fld := &ir.DistilledField{Name: "max_n", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "u32"},
		DefaultValue: "5", Modifiers: []ir.Modifier{ir.ModifierFinal}}
	require.NoError(t, NewRustFormatter().FormatNode(&b, fld, 0))
	assert.Contains(t, b.String(), "max_n")
	assert.NotContains(t, b.String(), "MAX_N")
}

func TestRustFormatter_TraitMembersHaveNoVisibility(t *testing.T) {
	format := func(class *ir.DistilledClass) string {
		var b strings.Builder
		require.NoError(t, NewRustFormatter().FormatNode(&b, class, 0))
		return b.String()
	}
	area := func(body string) *ir.DistilledFunction {
		return &ir.DistilledFunction{Name: "area", Visibility: ir.VisibilityPublic, Implementation: body,
			Parameters: []ir.Parameter{{Name: "&self"}}, Returns: &ir.TypeRef{Name: "i32"}}
	}

	traitImpl := &ir.DistilledClass{Name: "impl Shape for P", Visibility: ir.VisibilityPublic,
		Children: []ir.DistilledNode{area("")}}
	assert.Equal(t, "\nimpl Shape for P {\n\n    fn area(&self) -> i32\n}\n", format(traitImpl))

	// A default method of a trait is not abstract and still takes no keyword.
	trait := &ir.DistilledClass{Name: "Shape", Visibility: ir.VisibilityPublic,
		Modifiers: []ir.Modifier{ir.ModifierAbstract}, Children: []ir.DistilledNode{area("{ 1 }")}}
	assert.Equal(t, "\npub trait Shape {\n\n    fn area(&self) -> i32 { 1 }\n}\n", format(trait))

	inherent := &ir.DistilledClass{Name: "impl P", Visibility: ir.VisibilityPublic,
		Children: []ir.DistilledNode{area("")}}
	assert.Contains(t, format(inherent), "    pub fn area(&self) -> i32\n")
}

func TestRustFormatter_EnumVariantPayloadAndDiscriminant(t *testing.T) {
	enum := &ir.DistilledClass{Name: "E", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierEnum},
		Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "A", Visibility: ir.VisibilityPublic, DefaultValue: "1 << 2"},
			&ir.DistilledField{Name: "B", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "(Vec<(i32, i32)>)"}},
			&ir.DistilledField{Name: "C", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "{ x: i32 }"}},
			&ir.DistilledField{Name: "D", Visibility: ir.VisibilityPublic},
		}}
	var b strings.Builder
	require.NoError(t, NewRustFormatter().FormatNode(&b, enum, 0))
	assert.Equal(t, "\npub enum E {\n    A = 1 << 2,\n    B(Vec<(i32, i32)>),\n    C { x: i32 },\n    D,\n}\n", b.String())
}
