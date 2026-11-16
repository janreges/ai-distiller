package formatter

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/rust"
	"github.com/janreges/ai-distiller/internal/processor"
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
	assert.Equal(t, "\nimpl Shape for P {\n    fn area(&self) -> i32\n}\n", format(traitImpl))

	// A default method of a trait is not abstract and still takes no keyword.
	trait := &ir.DistilledClass{Name: "Shape", Visibility: ir.VisibilityPublic,
		Modifiers: []ir.Modifier{ir.ModifierAbstract}, Children: []ir.DistilledNode{area("{ 1 }")}}
	assert.Equal(t, "\npub trait Shape {\n    fn area(&self) -> i32 { 1 }\n}\n", format(trait))

	inherent := &ir.DistilledClass{Name: "impl P", Visibility: ir.VisibilityPublic,
		Children: []ir.DistilledNode{area("")}}
	assert.Contains(t, format(inherent), "    pub fn area(&self) -> i32\n")
}

func TestRustFormatter_AbstractFunctionEndsWithSemicolon(t *testing.T) {
	self := []ir.Parameter{{Name: "&self"}}
	abstract := []ir.Modifier{ir.ModifierAbstract}
	trait := &ir.DistilledClass{Name: "T", Visibility: ir.VisibilityPublic, Modifiers: abstract,
		Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "a", Visibility: ir.VisibilityPublic, Modifiers: abstract, Parameters: self},
			&ir.DistilledFunction{Name: "b", Visibility: ir.VisibilityPublic, Modifiers: abstract, Parameters: self,
				Returns: &ir.TypeRef{Name: "u8"}},
			&ir.DistilledFunction{Name: "c", Visibility: ir.VisibilityPublic, Parameters: self},
		}}
	var b strings.Builder
	require.NoError(t, NewRustFormatter().FormatNode(&b, trait, 0))
	lines := strings.Split(b.String(), "\n")
	assert.Contains(t, lines, "    fn a(&self);")
	assert.Contains(t, lines, "    fn b(&self) -> u8;")
	assert.Contains(t, lines, "    fn c(&self)")
}

func TestRustFormatter_InlineModPrintsEveryChild(t *testing.T) {
	format := func(children ...ir.DistilledNode) string {
		return formatRust(t, &ir.DistilledClass{Name: "inline_mod", Visibility: ir.VisibilityPublic, Children: children}, 0)
	}
	pub := ir.VisibilityPublic

	// A class without a kind modifier is a module, also when it holds only
	// functions (the old enum fallback) or only nested items and imports.
	assert.Equal(t, "\npub mod inline_mod {\n    pub fn inside() -> u8\n}\n", format(
		&ir.DistilledFunction{Name: "inside", Visibility: pub, Returns: &ir.TypeRef{Name: "u8"}},
	))
	assert.Equal(t, "\npub mod inline_mod {\n"+
		"    use super::*;\n"+
		"\n    /// Inner doc\n"+
		"    pub struct Inner;\n"+
		"\n    pub fn inside() -> u8\n"+
		"\n    pub type T = u8;\n"+
		"\n    mod deeper {\n"+
		"        pub const N: u8 = 1;\n"+
		"    }\n"+
		"}\n", format(
		&ir.DistilledImport{ImportType: "use", Module: "super::*"},
		&ir.DistilledComment{Text: "Inner doc", Format: ir.CommentDoc},
		&ir.DistilledClass{Name: "Inner", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierStruct}},
		&ir.DistilledFunction{Name: "inside", Visibility: pub, Returns: &ir.TypeRef{Name: "u8"}},
		&ir.DistilledField{Name: "T", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierTypeAlias}, Type: &ir.TypeRef{Name: "u8"}},
		&ir.DistilledClass{Name: "deeper", Visibility: ir.VisibilityPrivate, Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "N", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierFinal}, Type: &ir.TypeRef{Name: "u8"}, DefaultValue: "1"},
		}},
	))
	assert.Equal(t, "\npub mod inline_mod;\n", format())
}

func TestRustFormatter_UseGroups(t *testing.T) {
	format := func(imp *ir.DistilledImport) string {
		var b strings.Builder
		require.NoError(t, NewRustFormatter().FormatNode(&b, imp, 0))
		return b.String()
	}
	assert.Equal(t, "use std::io::{self, Write as W};\n", format(&ir.DistilledImport{ImportType: "use", Module: "std::io",
		Symbols: []ir.ImportedSymbol{{Name: "self"}, {Name: "Write", Alias: "W"}}}))
	assert.Equal(t, "use {a::B, c::D};\n", format(&ir.DistilledImport{ImportType: "use",
		Symbols: []ir.ImportedSymbol{{Name: "a::B"}, {Name: "c::D"}}}))
	assert.Equal(t, "use super::*;\n", format(&ir.DistilledImport{ImportType: "use", Module: "super::*"}))
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

// formatRust formats node with the Rust formatter at indent.
func formatRust(t *testing.T, node ir.DistilledNode, indent int) string {
	t.Helper()
	var b strings.Builder
	require.NoError(t, NewRustFormatter().FormatNode(&b, node, indent))
	return b.String()
}

// distillRust runs the real Rust processor, with the stripper for opts, and
// the text formatter on src, and returns the lines between the <file> tags.
func distillRust(t *testing.T, src string, opts processor.ProcessOptions) []string {
	t.Helper()
	file, err := rust.NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "x.rs", opts)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, NewLanguageAwareTextFormatter(Options{}).Format(&b, file))
	lines := strings.Split(strings.TrimSuffix(b.String(), "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 2)
	return lines[1 : len(lines)-1]
}

// rustDefault are the default flags: public items with their docs, no
// bodies, no other comments.
var rustDefault = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
}

// rustAll keeps every item, without bodies.
var rustAll = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
	IncludePrivate: true,
}

func withRustExtensions(ext ir.RustExtensions) ir.BaseNode {
	return ir.BaseNode{Extensions: &ir.NodeExtensions{Rust: &ext}}
}

func TestRustFormatter_TupleStruct(t *testing.T) {
	pair := &ir.DistilledClass{BaseNode: withRustExtensions(ir.RustExtensions{TupleArity: 2}), Name: "Pair",
		Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierStruct}, Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "0", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "i32"}},
			&ir.DistilledField{Name: "1", Visibility: ir.VisibilityPrivate, Type: &ir.TypeRef{Name: "String"}},
		}}
	assert.Equal(t, "\npub struct Pair(pub i32, String);\n", formatRust(t, pair, 0))

	// A field the stripper removed prints as _, as in rustdoc.
	src := "pub struct Pair(pub i32, String);\npub struct Meters(f64);\n"
	assert.Equal(t, []string{"", "pub struct Pair(pub i32, _);", "", "pub struct Meters(_);"}, distillRust(t, src, rustDefault))
	assert.Equal(t, []string{"", "pub struct Pair(pub i32, String);", "", "pub struct Meters(f64);"}, distillRust(t, src, rustAll))
}

func TestRustFormatter_Union(t *testing.T) {
	union := &ir.DistilledClass{Name: "FFIData", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierUnion},
		Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "i", Visibility: ir.VisibilityPrivate, Type: &ir.TypeRef{Name: "u32"}},
			&ir.DistilledField{Name: "f", Visibility: ir.VisibilityPublic, Type: &ir.TypeRef{Name: "f32"}},
		}}
	assert.Equal(t, "\npub union FFIData {\n    i: u32,\n    pub f: f32,\n}\n", formatRust(t, union, 0))
}

func TestRustFormatter_ExternBlock(t *testing.T) {
	abstract := []ir.Modifier{ir.ModifierAbstract}
	block := &ir.DistilledClass{Name: `extern "C"`, Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierExtern},
		Children: []ir.DistilledNode{
			&ir.DistilledFunction{Name: "validate", Visibility: ir.VisibilityPrivate, Modifiers: abstract,
				Parameters: []ir.Parameter{{Name: "x", Type: ir.TypeRef{Name: "i32"}}}, Returns: &ir.TypeRef{Name: "bool"}},
			&ir.DistilledFunction{Name: "other", Visibility: ir.VisibilityPublic, Modifiers: abstract},
			&ir.DistilledField{Name: "environ", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierStatic},
				Type: &ir.TypeRef{Name: "*const u8"}},
		}}
	assert.Equal(t, "\nextern \"C\" {\n    fn validate(x: i32) -> bool;\n\n    pub fn other();\n\n    pub static environ: *const u8;\n}\n",
		formatRust(t, block, 0))

	// A block whose members were all removed is not printed, nor are the
	// docs above it, at the top level and in a module; a plain comment stays.
	src := "/// FFI doc.\nextern \"C\" {\n    fn abs(input: i32) -> i32;\n}\npub fn after() {}\n"
	assert.Equal(t, []string{"", "pub fn after()"}, distillRust(t, src, rustDefault))
	inMod := "pub mod m {\n    // note\n    /// FFI doc.\n    extern \"C\" {\n        fn abs();\n    }\n    pub fn after() {}\n}\n"
	withComments := rustDefault
	withComments.IncludeComments = true
	assert.Equal(t, []string{"", "pub mod m {", "    // note", "", "    pub fn after()", "}"}, distillRust(t, inMod, withComments))
	assert.Equal(t, []string{"", "/// FFI doc.", "extern \"C\" {", "    fn abs(input: i32) -> i32;", "}", "", "pub fn after()"},
		distillRust(t, src, rustAll))
}

func TestRustFormatter_MacroRules(t *testing.T) {
	macro := &ir.DistilledFunction{Name: "square_it", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierMacro},
		Parameters: []ir.Parameter{{Name: "($x:expr)"}, {Name: "($a:expr, $b:expr)"}}}
	assert.Equal(t, "\nmacro_rules! square_it {\n    ($x:expr) => ...;\n    ($a:expr, $b:expr) => ...;\n}\n", formatRust(t, macro, 0))

	macro.Implementation = "{\n    ($x:expr) => { $x * $x };\n}"
	assert.Equal(t, "\nmacro_rules! square_it {\n    ($x:expr) => { $x * $x };\n}\n", formatRust(t, macro, 0))

	inMod := &ir.DistilledClass{Name: "m", Visibility: ir.VisibilityPublic, Children: []ir.DistilledNode{
		&ir.DistilledFunction{Name: "e", Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierMacro},
			Parameters: []ir.Parameter{{Name: "()"}}},
	}}
	assert.Equal(t, "\npub mod m {\n    macro_rules! e {\n        () => ...;\n    }\n}\n", formatRust(t, inMod, 0))
}

func TestRustFormatter_ExternCrateAndPubUse(t *testing.T) {
	format := func(imp *ir.DistilledImport) string { return formatRust(t, imp, 0) }
	assert.Equal(t, "extern crate alloc;\n", format(&ir.DistilledImport{ImportType: "extern crate", Module: "alloc"}))
	assert.Equal(t, "extern crate foo as bar;\n", format(&ir.DistilledImport{ImportType: "extern crate", Module: "foo as bar"}))
	assert.Equal(t, "pub extern crate self as me;\n", format(&ir.DistilledImport{ImportType: "pub extern crate", Module: "self as me"}))
	assert.Equal(t, "pub use crate::inner::Thing;\n", format(&ir.DistilledImport{ImportType: "pub use", Module: "crate::inner::Thing"}))
	assert.Equal(t, "pub(crate) use a::{b, c};\n", format(&ir.DistilledImport{ImportType: "pub(crate) use", Module: "a",
		Symbols: []ir.ImportedSymbol{{Name: "b"}, {Name: "c"}}}))

	// pub use is an import: --imports decides, not visibility.
	src := "pub use crate::inner::Thing;\nextern crate alloc;\n"
	assert.Equal(t, []string{"pub use crate::inner::Thing;", "extern crate alloc;"}, distillRust(t, src, rustDefault))
	noImports := rustAll
	noImports.IncludeImports = false
	assert.Empty(t, distillRust(t, src, noImports))
}

func TestRustFormatter_TraitConstsAndSupertraits(t *testing.T) {
	pub := ir.VisibilityPublic
	trait := &ir.DistilledClass{Name: "Animal", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierAbstract},
		Extends: []ir.TypeRef{{Name: "Display"}, {Name: "Clone"}},
		Children: []ir.DistilledNode{
			&ir.DistilledField{Name: "Food", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierTypeAlias}},
			&ir.DistilledField{Name: "LEGS", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierFinal}, Type: &ir.TypeRef{Name: "u32"}},
			&ir.DistilledField{Name: "NAME", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierFinal},
				Type: &ir.TypeRef{Name: "&'static str"}, DefaultValue: `"animal"`},
		}}
	assert.Equal(t, "\npub trait Animal: Display + Clone {\n    type Food;\n\n    const LEGS: u32;\n\n    const NAME: &'static str = \"animal\";\n}\n",
		formatRust(t, trait, 0))
}

func TestRustFormatter_WhereOnHeaders(t *testing.T) {
	where := func(clause string) ir.BaseNode { return withRustExtensions(ir.RustExtensions{Where: clause}) }
	pub := ir.VisibilityPublic
	value := &ir.DistilledField{Name: "value", Visibility: pub, Type: &ir.TypeRef{Name: "T"}}

	assert.Equal(t, "\npub struct Bounded<T> where T: Clone {\n    pub value: T,\n}\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: Clone"), Name: "Bounded<T>", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierStruct},
		Children: []ir.DistilledNode{value}}, 0))
	assert.Equal(t, "\npub struct S<T> where T: Clone;\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: Clone"), Name: "S<T>", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierStruct}}, 0))
	assert.Equal(t, "\npub struct W<T>(T) where T: Clone;\n", formatRust(t, &ir.DistilledClass{
		BaseNode: withRustExtensions(ir.RustExtensions{TupleArity: 1, Where: "where T: Clone"}), Name: "W<T>", Visibility: pub,
		Modifiers: []ir.Modifier{ir.ModifierStruct},
		Children:  []ir.DistilledNode{&ir.DistilledField{Name: "0", Visibility: ir.VisibilityPrivate, Type: &ir.TypeRef{Name: "T"}}}}, 0))
	assert.Equal(t, "\npub enum E<T> where T: Copy {\n    A(T),\n}\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: Copy"), Name: "E<T>", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierEnum},
		Children: []ir.DistilledNode{&ir.DistilledField{Name: "A", Visibility: pub, Type: &ir.TypeRef{Name: "(T)"}}}}, 0))
	assert.Equal(t, "\npub union U<T> where T: Copy {\n    pub value: T,\n}\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: Copy"), Name: "U<T>", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierUnion},
		Children: []ir.DistilledNode{value}}, 0))
	assert.Equal(t, "\npub trait Container<T>: Sized where T: Clone {\n}\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: Clone"), Name: "Container<T>", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierAbstract},
		Extends: []ir.TypeRef{{Name: "Sized"}}}, 0))
	assert.Equal(t, "\nimpl<T> Copy for Own<T> where T: ?Sized {\n}\n", formatRust(t, &ir.DistilledClass{
		BaseNode: where("where T: ?Sized"), Name: "impl<T> Copy for Own<T>", Visibility: pub}, 0))
}

func TestRustFormatter_TraitImplAssociatedItems(t *testing.T) {
	pub := ir.VisibilityPublic
	members := []ir.DistilledNode{
		&ir.DistilledField{Name: "Item", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierTypeAlias}, Type: &ir.TypeRef{Name: "u32"}},
		&ir.DistilledField{Name: "X", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierFinal}, Type: &ir.TypeRef{Name: "u8"}, DefaultValue: "1"},
	}
	assert.Equal(t, "\nimpl Iterator for Counter {\n    type Item = u32;\n\n    const X: u8 = 1;\n}\n",
		formatRust(t, &ir.DistilledClass{Name: "impl Iterator for Counter", Visibility: pub, Children: members}, 0))
	assert.Equal(t, "\nimpl Counter {\n    pub type Item = u32;\n\n    pub const X: u8 = 1;\n}\n",
		formatRust(t, &ir.DistilledClass{Name: "impl Counter", Visibility: pub, Children: members}, 0))

	// In the default output, through the real processor and stripper.
	src := "impl Iterator for Counter {\n    type Item = u32;\n    fn next(&mut self) -> Option<u32> { None }\n}\n"
	assert.Equal(t, []string{"", "impl Iterator for Counter {", "    type Item = u32;", "",
		"    fn next(&mut self) -> Option<u32>", "}"}, distillRust(t, src, rustDefault))
}

func TestRustFormatter_FunctionQualifiers(t *testing.T) {
	src := "pub const async unsafe extern \"C\" fn all() {}\npub const fn c() -> u8 { 1 }\npub unsafe fn u() {}\n" +
		"pub extern fn bare() {}\npub async fn a() {}\nfn p() where u32: Copy {}\n" +
		"pub fn w<T>(x: T)\nwhere\n    T: Clone,\n{}\n"
	got := distillRust(t, src, rustAll)
	assert.Equal(t, []string{
		"", "pub const async unsafe extern \"C\" fn all()",
		"", "pub const fn c() -> u8",
		"", "pub unsafe fn u()",
		"", "pub extern fn bare()",
		"", "pub async fn a()",
		"", "fn p() where u32: Copy",
		"", "pub fn w<T>(x: T) where T: Clone",
	}, got)

	fn := &ir.DistilledFunction{BaseNode: withRustExtensions(ir.RustExtensions{IsUnsafe: true, ABI: "C"}), Name: "f",
		Visibility: ir.VisibilityPublic, Modifiers: []ir.Modifier{ir.ModifierConst, ir.ModifierAsync, ir.ModifierExtern}}
	assert.Equal(t, "\npub const async unsafe extern \"C\" fn f()\n", formatRust(t, fn, 0))
}

func TestRustFormatter_UnsafeImplAndTrait(t *testing.T) {
	src := "unsafe impl Send for X {}\npub unsafe trait Z {}\nunsafe extern \"C\" { fn f(); }\n"
	got := distillRust(t, src, rustAll)
	assert.Contains(t, got, "unsafe impl Send for X {")
	assert.Contains(t, got, "pub unsafe trait Z {")
	assert.Contains(t, got, "unsafe extern \"C\" {")
}

func TestRustFormatter_StaticMut(t *testing.T) {
	got := distillRust(t, "pub static mut C: u64 = 0;\n", rustDefault)
	assert.Equal(t, []string{"", "pub static mut C: u64 = 0;"}, got)
}

func TestRustFormatter_RestrictedVisibility(t *testing.T) {
	src := "pub(in crate::outer) fn a() {}\npub(self) fn b() {}\ncrate fn c() {}\n" +
		"pub(in crate::m) struct S { pub(in crate::m) x: u8, pub(self) y: u8 }\n" +
		"pub struct T(pub(in crate::m) u8);\n"
	assert.Equal(t, []string{
		"", "pub(in crate::outer) fn a()",
		"", "fn b()",
		"", "pub(crate) fn c()",
		"", "pub(in crate::m) struct S {", "    pub(in crate::m) x: u8,", "    y: u8,", "}",
		"", "pub struct T(pub(in crate::m) u8);",
	}, distillRust(t, src, rustAll))

	// pub(self) is private: hidden by default.
	assert.Equal(t, []string{"", "pub struct T(_);"}, distillRust(t, src, rustDefault))
}

func TestRustFormatter_DocCommentStyles(t *testing.T) {
	src := "/// outer line\n" +
		"///\n" +
		"/// after an empty doc line\n" +
		"fn a() {}\n" +
		"/** outer block\n * second line\n *\n *   indented\n */\n" +
		"fn b() {}\n" +
		"//! inner line\n" +
		"/*! inner block\n * more\n */\n" +
		"// plain line\n" +
		"/* plain block */\n" +
		"fn c() {}\n"
	opts := rustAll
	opts.IncludeComments = true
	assert.Equal(t, []string{
		"",
		"/// outer line",
		"///",
		"/// after an empty doc line",
		"fn a()",
		"",
		"/// outer block",
		"/// second line",
		"///",
		"///   indented",
		"fn b()",
		"//! inner line",
		"//! inner block",
		"//! more",
		"",
		"// plain line",
		"// plain block",
		"fn c()",
	}, distillRust(t, src, opts))

	// An empty comment prints its marker alone, at the indent.
	assert.Equal(t, "    ///\n    /// x\n", formatRust(t, &ir.DistilledComment{Text: "\nx", Format: ir.CommentDoc}, 1))
	assert.Equal(t, "//!\n", formatRust(t, &ir.DistilledComment{Text: "", Format: ir.CommentDoc, Inner: true}, 0))
	assert.Equal(t, "//\n", formatRust(t, &ir.DistilledComment{Text: "", Format: ir.CommentLine}, 0))
}

func TestRustFormatter_AttributesAndBodyDocs(t *testing.T) {
	src := "/// pub doc\n#[derive(Debug)]\npub struct A;\n/// priv doc\n#[derive(Debug)]\nstruct B;\n"
	assert.Equal(t, []string{"", "/// pub doc", "#[derive(Debug)]", "pub struct A;"}, distillRust(t, src, rustDefault))

	src = "/// S doc\n#[derive(Debug, Clone)]\n#[repr(C)]\npub struct S {\n" +
		"    /// x doc\n    #[serde(rename = \"x\")]\n    pub x: u8,\n" +
		"    /// hidden doc\n    #[serde(skip)]\n    y: u8,\n" +
		"    /// z doc\n    pub z: u8,\n}\n" +
		"pub enum E {\n    /// a doc\n    #[default]\n    A,\n    /// b doc\n    B(u8),\n}\n" +
		"pub union U {\n    /// f doc\n    #[allow(dead_code)]\n    pub f: u32,\n}\n" +
		"pub struct T(#[serde(skip)] pub u8);\n"
	assert.Equal(t, []string{
		"", "/// S doc", "#[derive(Debug, Clone)]", "#[repr(C)]", "pub struct S {",
		"    /// x doc", "    #[serde(rename = \"x\")]", "    pub x: u8,",
		"    /// z doc", "    pub z: u8,", "    /* private fields */", "}",
		"", "pub enum E {",
		"    /// a doc", "    #[default]", "    A,", "    /// b doc", "    B(u8),", "}",
		"", "pub union U {", "    /// f doc", "    #[allow(dead_code)]", "    pub f: u32,", "}",
		"", "pub struct T(pub u8);",
	}, distillRust(t, src, rustDefault))

	// Plain comments in a body print only with comments on, above the
	// field they precede; the hidden field takes its doc and attribute along.
	body := "pub struct S {\n    // note\n    pub a: u8,\n    /// hidden\n    // also hidden\n    b: u8,\n    /// hidden too\n    #[x]\n    c: u8,\n    d: u8, // trailing\n}\n"
	assert.Equal(t, []string{"", "pub struct S {", "    pub a: u8,", "    /* private fields */", "}"}, distillRust(t, body, rustDefault))
	withComments := rustDefault
	withComments.IncludeComments = true
	assert.Equal(t, []string{"", "pub struct S {", "    // note", "    pub a: u8,", "    /* private fields */", "}"},
		distillRust(t, body, withComments))

	// Members, functions, consts and macros carry their attributes too.
	members := "pub trait T {\n    #[must_use]\n    fn f(&self) -> u8;\n    #[deprecated]\n    type X;\n}\n" +
		"impl S {\n    #[inline]\n    pub fn g() {}\n    #[allow(x)]\n    pub const C: u8 = 1;\n}\n" +
		"#[macro_export]\nmacro_rules! m { () => {}; }\n" +
		"#[cfg(test)]\nuse std::fmt;\n#![allow(dead_code)]\n"
	assert.Equal(t, []string{
		"", "pub trait T {", "    #[must_use]", "    fn f(&self) -> u8;", "    #[deprecated]", "    type X;", "}",
		"", "impl S {", "    #[inline]", "    pub fn g()", "", "    #[allow(x)]", "    pub const C: u8 = 1;", "}",
		"", "#[macro_export]", "macro_rules! m {", "    () => ...;", "}",
		"use std::fmt;",
	}, distillRust(t, members, rustAll))

	// Annotations off removes them.
	noAttrs := rustDefault
	noAttrs.IncludeAnnotations = false
	assert.Equal(t, []string{"", "/// pub doc", "pub struct A;"}, distillRust(t, "/// pub doc\n#[derive(Debug)]\npub struct A;\n", noAttrs))
}

// A doc or plain comment sits directly on the item it precedes, with the
// item's blank line above the comment; a file header followed by an item's
// doc keeps its own place, and so does an inner doc.
func TestRustFormatter_CommentsSitOnTheirItem(t *testing.T) {
	src := "// Copyright header\n\n/// Item doc\npub fn a() {}\n" +
		"impl S {\n    /// m doc\n    #[inline]\n    pub fn m() {}\n    // plain\n    pub fn n() {}\n}\n" +
		"pub mod inner {\n    //! Module doc\n    /// f doc\n    pub fn f() {}\n}\n"
	opts := rustAll
	opts.IncludeComments = true
	assert.Equal(t, []string{
		"// Copyright header",
		"",
		"/// Item doc",
		"pub fn a()",
		"",
		"impl S {",
		"    /// m doc",
		"    #[inline]",
		"    pub fn m()",
		"",
		"    // plain",
		"    pub fn n()",
		"}",
		"",
		"pub mod inner {",
		"    //! Module doc",
		"",
		"    /// f doc",
		"    pub fn f()",
		"}",
	}, distillRust(t, src, opts))
}

// A struct or union whose private fields the stripper removed prints
// "/* private fields */" for them, as rustdoc does, and a unit struct stays
// "pub struct X;".
func TestRustFormatter_RemovedPrivateFields(t *testing.T) {
	pub, priv := ir.VisibilityPublic, ir.VisibilityPrivate
	structWith := func(private int, fields ...ir.DistilledNode) *ir.DistilledClass {
		return &ir.DistilledClass{Name: "S", Visibility: pub, Modifiers: []ir.Modifier{ir.ModifierStruct},
			BaseNode: ir.BaseNode{Extensions: &ir.NodeExtensions{Rust: &ir.RustExtensions{PrivateFields: private}}},
			Children: fields}
	}
	field := func(name string, vis ir.Visibility) *ir.DistilledField {
		return &ir.DistilledField{Name: name, Visibility: vis, Type: &ir.TypeRef{Name: "u8"}}
	}
	assert.Equal(t, "\npub struct S { /* private fields */ }\n", formatRust(t, structWith(2), 0))
	assert.Equal(t, "\npub struct S {\n    pub a: u8,\n    /* private fields */\n}\n",
		formatRust(t, structWith(1, field("a", pub)), 0))
	assert.Equal(t, "\npub struct S {\n    pub a: u8,\n    b: u8,\n}\n",
		formatRust(t, structWith(1, field("a", pub), field("b", priv)), 0), "nothing was removed")
	assert.Equal(t, "\npub struct S;\n", formatRust(t, structWith(0), 0))

	src := "pub struct Config {\n    name: String,\n}\npub struct Mixed {\n    pub id: u32,\n    secret: String,\n}\n" +
		"pub struct Marker;\npub union U {\n    i: u32,\n}\n"
	assert.Equal(t, []string{
		"", "pub struct Config { /* private fields */ }",
		"", "pub struct Mixed {", "    pub id: u32,", "    /* private fields */", "}",
		"", "pub struct Marker;",
		"", "pub union U { /* private fields */ }",
	}, distillRust(t, src, rustDefault))
	assert.Equal(t, []string{
		"", "pub struct Config {", "    name: String,", "}",
		"", "pub struct Mixed {", "    pub id: u32,", "    secret: String,", "}",
		"", "pub struct Marker;",
		"", "pub union U {", "    i: u32,", "}",
	}, distillRust(t, src, rustAll))
}

// The body of an impl block, a trait, an extern block and a module starts on
// the line after its "{", also when its first member has a doc, an attribute
// or a plain comment; the members after it keep their blank line.
func TestRustFormatter_BodyStartsOnTheLineAfterItsBrace(t *testing.T) {
	src := "pub struct W;\nimpl W {\n    pub const fn new() -> Self { W }\n    pub fn len(&self) -> usize { 0 }\n}\n" +
		"impl Display for W {\n    fn fmt(&self) {}\n}\n" +
		"pub trait T {\n    fn f(&self);\n}\n" +
		"pub mod sub { pub fn f() {} fn g() {} pub mod deeper { pub fn h() {} } }\n" +
		"extern \"C\" {\n    pub fn abs(x: i32) -> i32;\n}\n"
	assert.Equal(t, []string{
		"", "pub struct W;",
		"", "impl W {", "    pub const fn new() -> Self", "", "    pub fn len(&self) -> usize", "}",
		"", "impl Display for W {", "    fn fmt(&self)", "}",
		"", "pub trait T {", "    fn f(&self);", "}",
		"", "pub mod sub {", "    pub fn f()", "", "    pub mod deeper {", "        pub fn h()", "    }", "}",
		"", "extern \"C\" {", "    pub fn abs(x: i32) -> i32;", "}",
	}, distillRust(t, src, rustDefault))

	commented := "impl W {\n    // plain\n    pub fn a() {}\n}\npub mod m {\n    /// doc\n    #[inline]\n    pub fn b() {}\n}\n"
	opts := rustDefault
	opts.IncludeComments = true
	assert.Equal(t, []string{
		"", "impl W {", "    // plain", "    pub fn a()", "}",
		"", "pub mod m {", "    /// doc", "    #[inline]", "    pub fn b()", "}",
	}, distillRust(t, commented, opts))
}

// An inherent impl block without members prints nothing, nor does the doc
// above it, as in rustdoc; a trait impl without members states a fact and
// prints.
func TestRustFormatter_EmptyInherentImplPrintsNothing(t *testing.T) {
	src := "pub struct S;\n/// S impl doc\nimpl S {\n    fn helper() {}\n}\nimpl Copy for S {}\nimpl S {}\npub fn after() {}\n"
	assert.Equal(t, []string{"", "pub struct S;", "", "impl Copy for S {", "}", "", "pub fn after()"},
		distillRust(t, src, rustDefault))
	assert.Equal(t, []string{"", "pub struct S;", "", "/// S impl doc", "impl S {", "    fn helper()", "}",
		"", "impl Copy for S {", "}", "", "pub fn after()"}, distillRust(t, src, rustAll))
}
