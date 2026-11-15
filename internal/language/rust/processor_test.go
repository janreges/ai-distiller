package rust

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_Process(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		validate func(t *testing.T, result *ir.DistilledFile)
	}{
		{
			name: "basic struct",
			source: `pub struct Person {
				pub name: String,
				age: u32,
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one top-level node")

				classNode, ok := result.Children[0].(*ir.DistilledClass)
				require.True(t, ok, "Expected a DistilledClass node")
				assert.Equal(t, "Person", classNode.Name)
				assert.Equal(t, ir.VisibilityPublic, classNode.Visibility)
				require.Len(t, classNode.Children, 2, "Expected two fields")

				field1, ok := classNode.Children[0].(*ir.DistilledField)
				require.True(t, ok, "Expected first child to be a field")
				assert.Equal(t, "name", field1.Name)
				assert.Equal(t, ir.VisibilityPublic, field1.Visibility)
				assert.NotNil(t, field1.Type)
				assert.Equal(t, "String", field1.Type.Name)

				field2, ok := classNode.Children[1].(*ir.DistilledField)
				require.True(t, ok, "Expected second child to be a field")
				assert.Equal(t, "age", field2.Name)
				assert.Equal(t, ir.VisibilityPrivate, field2.Visibility)
				assert.NotNil(t, field2.Type)
				assert.Equal(t, "u32", field2.Type.Name)
			},
		},
		{
			name: "basic function",
			source: `pub fn greet(name: &str) -> String {
				format!("Hello, {}!", name)
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one function")

				fnNode, ok := result.Children[0].(*ir.DistilledFunction)
				require.True(t, ok, "Expected a DistilledFunction node")
				assert.Equal(t, "greet", fnNode.Name)
				assert.Equal(t, ir.VisibilityPublic, fnNode.Visibility)

				require.Len(t, fnNode.Parameters, 1, "Expected one parameter")
				assert.Equal(t, "name", fnNode.Parameters[0].Name)
				assert.Equal(t, "&str", fnNode.Parameters[0].Type.Name)

				assert.NotNil(t, fnNode.Returns)
				assert.Equal(t, "String", fnNode.Returns.Name)
			},
		},
		{
			name: "use statements",
			source: `use std::collections::HashMap;
use std::io::{self, Read, Write};`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 2, "Expected two import statements")

				imp1, ok := result.Children[0].(*ir.DistilledImport)
				require.True(t, ok, "Expected first to be import")
				assert.Equal(t, "use", imp1.ImportType)
				assert.Equal(t, "std::collections::HashMap", imp1.Module)

				imp2, ok := result.Children[1].(*ir.DistilledImport)
				require.True(t, ok, "Expected second to be import")
				assert.Equal(t, "use", imp2.ImportType)
				assert.Equal(t, "std::io", imp2.Module)
				assert.Equal(t, []ir.ImportedSymbol{{Name: "self"}, {Name: "Read"}, {Name: "Write"}}, imp2.Symbols)
			},
		},
		{
			name: "enum",
			source: `pub enum Status {
				Active,
				Inactive,
				Pending(String),
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one enum")

				enumNode, ok := result.Children[0].(*ir.DistilledClass)
				require.True(t, ok, "Expected a DistilledClass node")
				assert.Equal(t, "Status", enumNode.Name)
				assert.Equal(t, ir.VisibilityPublic, enumNode.Visibility)
				require.Len(t, enumNode.Children, 3, "Expected three variants")

				// Check variants
				for i, expectedName := range []string{"Active", "Inactive", "Pending"} {
					variant, ok := enumNode.Children[i].(*ir.DistilledField)
					require.True(t, ok, "Expected variant to be a field")
					assert.Equal(t, expectedName, variant.Name)
					assert.Equal(t, ir.VisibilityPublic, variant.Visibility)
				}

				// Check Pending has tuple type
				pending := enumNode.Children[2].(*ir.DistilledField)
				assert.NotNil(t, pending.Type)
				assert.Equal(t, "(String)", pending.Type.Name)
			},
		},
		{
			name: "impl block",
			source: `impl Person {
				pub fn new(name: String) -> Self {
					Person { name }
				}
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one impl block")

				implNode, ok := result.Children[0].(*ir.DistilledClass)
				require.True(t, ok, "Expected a DistilledClass node")
				assert.Equal(t, "impl Person", implNode.Name)
				assert.Equal(t, ir.VisibilityPublic, implNode.Visibility)
				require.Len(t, implNode.Children, 1, "Expected one method")

				method, ok := implNode.Children[0].(*ir.DistilledFunction)
				require.True(t, ok, "Expected a function")
				assert.Equal(t, "new", method.Name)
				assert.Equal(t, ir.VisibilityPublic, method.Visibility)
				require.Len(t, method.Parameters, 1)
				assert.Equal(t, "name", method.Parameters[0].Name)
				assert.NotNil(t, method.Returns)
				assert.Equal(t, "Self", method.Returns.Name)
			},
		},
		{
			name: "function with lifetime parameters",
			source: `pub fn analyze<'a, S>(source: &'a S) -> Result<String, Error> where S: DataSource + ?Sized {
				Ok("result".to_string())
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one function")

				fnNode, ok := result.Children[0].(*ir.DistilledFunction)
				require.True(t, ok, "Expected a DistilledFunction node")
				assert.Equal(t, "analyze<'a, S>", fnNode.Name)
				assert.Equal(t, ir.VisibilityPublic, fnNode.Visibility)

				require.Len(t, fnNode.Parameters, 1, "Expected one parameter")
				assert.Equal(t, "source", fnNode.Parameters[0].Name)
				assert.Equal(t, "&'a S", fnNode.Parameters[0].Type.Name)

				assert.NotNil(t, fnNode.Returns)
				// The return type should include the where clause
				assert.Equal(t, "Result<String, Error> where S: DataSource + ?Sized", fnNode.Returns.Name)
			},
		},
		{
			name: "complex lifetime parameters",
			source: `fn compare_sources<'a, 'b, S1, S2>(source1: &'a S1, source2: &'b S2) -> Result<bool, Error> where S1: DataSource + Debug, S2: DataSource + Debug {
				Ok(true)
			}`,
			validate: func(t *testing.T, result *ir.DistilledFile) {
				require.Len(t, result.Children, 1, "Expected one function")

				fnNode, ok := result.Children[0].(*ir.DistilledFunction)
				require.True(t, ok, "Expected a DistilledFunction node")
				assert.Equal(t, "compare_sources<'a, 'b, S1, S2>", fnNode.Name)

				require.Len(t, fnNode.Parameters, 2, "Expected two parameters")
				assert.Equal(t, "source1", fnNode.Parameters[0].Name)
				assert.Equal(t, "&'a S1", fnNode.Parameters[0].Type.Name)
				assert.Equal(t, "source2", fnNode.Parameters[1].Name)
				assert.Equal(t, "&'b S2", fnNode.Parameters[1].Type.Name)

				assert.NotNil(t, fnNode.Returns)
				assert.Equal(t, "Result<bool, Error> where S1: DataSource + Debug, S2: DataSource + Debug", fnNode.Returns.Name)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			processor := NewProcessor()
			ctx := context.Background()
			reader := strings.NewReader(tt.source)

			result, err := processor.Process(ctx, reader, "test.rs")
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, "rust", result.Language)
			assert.Equal(t, "2021", result.Version)
			assert.Empty(t, result.Errors, "Expected no parsing errors")

			// Run specific validation
			tt.validate(t, result)
		})
	}
}

func TestProcessor_SupportedExtensions(t *testing.T) {
	processor := NewProcessor()
	extensions := processor.SupportedExtensions()

	assert.Contains(t, extensions, ".rs")
}

func TestProcessor_CanProcess(t *testing.T) {
	processor := NewProcessor()

	tests := []struct {
		filename string
		expected bool
	}{
		{"main.rs", true},
		{"lib.rs", true},
		{"mod.rs", true},
		{"test.py", false},
		{"main.go", false},
		{"file.js", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			assert.Equal(t, tt.expected, processor.CanProcess(tt.filename))
		})
	}
}

// parseRust runs the real processor on src with implementations and private
// items kept.
func parseRust(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	file, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "test.rs")
	require.NoError(t, err)
	return file
}

// findRustFn returns the function called name, searching the top level of file
// and the members of its classes (structs, impls, traits, modules). The test
// fails when there is none.
func findRustFn(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledFunction {
	t.Helper()
	for _, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledFunction:
			if n.Name == name {
				return n
			}
		case *ir.DistilledClass:
			for _, child := range n.Children {
				if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == name {
					return fn
				}
			}
		}
	}
	require.Failf(t, "function not found", "no function %q in the parsed file", name)
	return nil
}

// findRustClass returns the top-level class (struct, enum, trait, impl or
// module) called name. The test fails when there is none.
func findRustClass(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledClass {
	t.Helper()
	for _, node := range file.Children {
		if class, ok := node.(*ir.DistilledClass); ok && class.Name == name {
			return class
		}
	}
	require.Failf(t, "class not found", "no class %q in the parsed file", name)
	return nil
}

// findRustFnIn returns the function called name among the members of the
// top-level class called container. The test fails when there is none.
func findRustFnIn(t *testing.T, file *ir.DistilledFile, container, name string) *ir.DistilledFunction {
	t.Helper()
	for _, child := range findRustClass(t, file, container).Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == name {
			return fn
		}
	}
	require.Failf(t, "function not found", "no function %q in %q", name, container)
	return nil
}

// childNames returns the names of the fields, functions and classes among
// class's members, in order; comments are skipped.
func childNames(class *ir.DistilledClass) []string {
	names := []string{}
	for _, child := range class.Children {
		switch n := child.(type) {
		case *ir.DistilledField:
			names = append(names, n.Name)
		case *ir.DistilledFunction:
			names = append(names, n.Name)
		case *ir.DistilledClass:
			names = append(names, n.Name)
		}
	}
	return names
}

func TestRust_ImplementationStartsAtBrace(t *testing.T) {
	file := parseRust(t, "pub struct P {\n    pub x: i32,\n}\n\nimpl P {\n    pub fn get(&self) -> i32 {\n        self.x\n    }\n}\n")
	get := findRustFn(t, file, "get")
	assert.Equal(t, "{\n        self.x\n    }", get.Implementation)
}

func TestRust_ImplementationBraceOnOwnLine(t *testing.T) {
	file := parseRust(t, "pub fn f<T>(x: T) -> T\nwhere\n    T: Clone,\n{\n    x\n}\n")
	assert.Equal(t, "{\n    x\n}", findRustFn(t, file, "f<T>").Implementation)
}

func TestRust_BraceInsideStringLiteral(t *testing.T) {
	file := parseRust(t, "pub fn a() {\n    println!(\"}\"); // }\n}\n\npub fn b() {\n}\n")
	assert.Equal(t, "{\n    println!(\"}\"); // }\n}", findRustFn(t, file, "a").Implementation)
	assert.NotNil(t, findRustFn(t, file, "b"))
}

func TestRust_OneLineBody(t *testing.T) {
	src := "pub struct S {\n    items: Vec<i32>,\n}\n\nimpl S {\n" +
		"    pub fn quick(&self) -> usize { self.items.len() }\n" +
		"    pub fn empty() {}\n" +
		"    pub fn brace(&self) -> char { if self.items.is_empty() { '}' } else { '{' } } // }\n" +
		"    pub fn after(&self) -> i32 {\n        1\n    }\n}\n\n" +
		"pub fn top() -> i32 { 42 }\n"
	file := parseRust(t, src)
	assert.Equal(t, "{ self.items.len() }", findRustFn(t, file, "quick").Implementation)
	assert.Equal(t, "usize", findRustFn(t, file, "quick").Returns.Name)
	assert.Equal(t, "{}", findRustFn(t, file, "empty").Implementation)
	assert.Equal(t, "{ if self.items.is_empty() { '}' } else { '{' } }", findRustFn(t, file, "brace").Implementation)
	assert.Equal(t, "{\n        1\n    }", findRustFn(t, file, "after").Implementation)
	assert.Equal(t, "{ 42 }", findRustFn(t, file, "top").Implementation)
}

func TestRust_TraitImplMethodsArePublic(t *testing.T) {
	file := parseRust(t, "pub struct P;\n\npub trait Shape {\n    fn area(&self) -> i32;\n}\n\nimpl Shape for P {\n    fn area(&self) -> i32 {\n        1\n    }\n}\n\nimpl P {\n    fn helper(&self) {}\n}\n")
	assert.Equal(t, ir.VisibilityPublic, findRustFnIn(t, file, "impl Shape for P", "area").Visibility)
	assert.Equal(t, ir.VisibilityPrivate, findRustFnIn(t, file, "impl P", "helper").Visibility)
}

func TestRust_OneLineEnumAndStruct(t *testing.T) {
	file := parseRust(t, "pub enum E { A, B(i32), C { x: i32 } }\npub struct P { pub x: i32, y: u8 }\n")
	assert.Equal(t, []string{"A", "B", "C"}, childNames(findRustClass(t, file, "E")))
	p := findRustClass(t, file, "P")
	require.Equal(t, []string{"x", "y"}, childNames(p))
	assert.Equal(t, ir.VisibilityPublic, p.Children[0].(*ir.DistilledField).Visibility)
	assert.Equal(t, ir.VisibilityPrivate, p.Children[1].(*ir.DistilledField).Visibility)
}

func TestRust_OneLineStructWithGenerics(t *testing.T) {
	p := findRustClass(t, parseRust(t, "pub struct P<T> { pub x: HashMap<String, Vec<T>>, pub y: (i32, i32) }\n"), "P<T>")
	assert.Equal(t, []string{"x", "y"}, childNames(p))
}

func TestRust_TraitDefaultMethodKeepsLaterMembers(t *testing.T) {
	src := "pub trait T {\n" +
		"    fn required(&self) -> i32;\n" +
		"    fn provided(&self) -> i32 {\n" +
		"        if self.required() > 0 {\n" +
		"            let open = '{';\n" +
		"            let close = \"}\";\n" +
		"        }\n" +
		"        1\n" +
		"    }\n" +
		"    fn quick(&self) -> i32 { 2 }\n" +
		"    type Item;\n" +
		"    fn after(&self);\n" +
		"}\n\n" +
		"pub fn tail() {}\n"
	file := parseRust(t, src)
	assert.Equal(t, []string{"required", "provided", "quick", "Item", "after"}, childNames(findRustClass(t, file, "T")))

	provided := findRustFnIn(t, file, "T", "provided")
	assert.Equal(t, "{\n        if self.required() > 0 {\n            let open = '{';\n            let close = \"}\";\n        }\n        1\n    }", provided.Implementation)
	assert.NotContains(t, provided.Modifiers, ir.ModifierAbstract)
	assert.Equal(t, "{ 2 }", findRustFnIn(t, file, "T", "quick").Implementation)
	assert.NotContains(t, findRustFnIn(t, file, "T", "quick").Modifiers, ir.ModifierAbstract)

	required := findRustFnIn(t, file, "T", "required")
	assert.Empty(t, required.Implementation)
	assert.Contains(t, required.Modifiers, ir.ModifierAbstract)
	assert.Contains(t, findRustFnIn(t, file, "T", "after").Modifiers, ir.ModifierAbstract)

	assert.Equal(t, "{}", findRustFn(t, file, "tail").Implementation)
}

func TestRust_BlockBracesInLiterals(t *testing.T) {
	// A brace inside a string or char literal ends no block.
	trait := "pub trait T {\n    const OPEN: &'static str = \"{\";\n    fn after(&self);\n}\n"
	assert.Equal(t, []string{"OPEN", "after"}, childNames(findRustClass(t, parseRust(t, trait), "T")))

	impl := "pub struct S;\n\nimpl S {\n" +
		"    pub fn long(\n        &self,\n    ) -> &'static str {\n        \"}\"\n    }\n" +
		"    pub fn after(&self) {}\n}\n"
	assert.Equal(t, []string{"long", "after"}, childNames(findRustClass(t, parseRust(t, impl), "impl S")))
}

// variant returns the enum variant called name among class's fields. The test
// fails when there is none.
func variant(t *testing.T, class *ir.DistilledClass, name string) *ir.DistilledField {
	t.Helper()
	for _, child := range class.Children {
		if f, ok := child.(*ir.DistilledField); ok && f.Name == name {
			return f
		}
	}
	require.Failf(t, "variant not found", "no variant %q in %q", name, class.Name)
	return nil
}

func TestRust_EnumDiscriminantsAndPayloads(t *testing.T) {
	t.Run("one line", func(t *testing.T) {
		file := parseRust(t, "pub enum Code { Ok = 0, Err = 1 << 2, Big = -1 }\npub enum P { V(Vec<(i32, i32)>), S { a: (u8, u8) }, U }\n")
		code := findRustClass(t, file, "Code")
		assert.Equal(t, []string{"Ok", "Err", "Big"}, childNames(code))
		assert.Equal(t, "0", variant(t, code, "Ok").DefaultValue)
		assert.Equal(t, "1 << 2", variant(t, code, "Err").DefaultValue)
		assert.Equal(t, "-1", variant(t, code, "Big").DefaultValue)
		p := findRustClass(t, file, "P")
		assert.Equal(t, []string{"V", "S", "U"}, childNames(p))
		assert.Equal(t, "(Vec<(i32, i32)>)", variant(t, p, "V").Type.Name)
		assert.Equal(t, "{ a: (u8, u8) }", variant(t, p, "S").Type.Name)
		assert.Nil(t, variant(t, p, "U").Type)
	})
	t.Run("multi line", func(t *testing.T) {
		src := "pub enum M {\n" +
			"    /// The first one.\n" +
			"    A = 1, // trailing comment\n" +
			"    #[serde(rename = \"b, c\")]\n" +
			"    B(Vec<(i32, i32)>),\n" +
			"    C {\n" +
			"        x: i32,\n" +
			"        y: (u8, u8),\n" +
			"    },\n" +
			"    D = 1 << 3,\n" +
			"    E\n" +
			"}\n" +
			"pub fn after() {}\n"
		file := parseRust(t, src)
		m := findRustClass(t, file, "M")
		assert.Equal(t, []string{"A", "B", "C", "D", "E"}, childNames(m))
		assert.Equal(t, "1", variant(t, m, "A").DefaultValue)
		assert.Equal(t, 3, variant(t, m, "A").Location.StartLine)
		assert.Equal(t, "(Vec<(i32, i32)>)", variant(t, m, "B").Type.Name)
		assert.Equal(t, 5, variant(t, m, "B").Location.StartLine)
		assert.Equal(t, "{ x: i32, y: (u8, u8), }", variant(t, m, "C").Type.Name)
		assert.Equal(t, "1 << 3", variant(t, m, "D").DefaultValue)
		findRustFn(t, file, "after")
	})
}

func TestRust_InnerDocCommentsAreMarked(t *testing.T) {
	file := parseRust(t, "//! Crate doc.\n/*! Block crate doc. */\n/// Outer doc.\npub fn api() {}\n")
	var inner []bool
	for _, node := range file.Children {
		if c, ok := node.(*ir.DistilledComment); ok {
			require.Equal(t, "doc", c.Format, c.Text)
			inner = append(inner, c.Inner)
		}
	}
	assert.Equal(t, []bool{true, true, false}, inner)
}

// rustPublicOnly matches the CLI defaults: public members with their doc
// comments, no other comments, no bodies.
var rustPublicOnly = processor.ProcessOptions{
	IncludeImports: true, IncludeDocstrings: true, IncludeAnnotations: true, IncludeFields: true, IncludeMethods: true,
}

// distillRustTop runs the processor with opts and returns the top-level
// comments and functions that survive, in order.
func distillRustTop(t *testing.T, src string, opts processor.ProcessOptions) []string {
	t.Helper()
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.rs", opts)
	require.NoError(t, err)
	var out []string
	for _, node := range file.Children {
		switch n := node.(type) {
		case *ir.DistilledComment:
			out = append(out, "doc:"+n.Text)
		case *ir.DistilledFunction:
			out = append(out, "fn:"+n.Name)
		}
	}
	return out
}

func TestRust_DocOfHiddenFirstFunctionIsHidden(t *testing.T) {
	src := "/// helper doc.\nfn helper() {}\n\n/// Public api.\npub fn api() {}\n"
	assert.Equal(t, []string{"doc:Public api.", "fn:api"}, distillRustTop(t, src, rustPublicOnly))
}

func TestRust_CrateDocStaysOuterDocLeavesWithItsFunction(t *testing.T) {
	src := "//! Crate doc.\n//! More crate doc.\n\n/// helper doc.\nfn helper() {}\n\npub fn api() {}\n"
	assert.Equal(t, []string{"doc:Crate doc.", "doc:More crate doc.", "fn:api"}, distillRustTop(t, src, rustPublicOnly))
}

func TestRust_PublicOffDropsTheDocOfPublicFunction(t *testing.T) {
	opts := rustPublicOnly
	opts.RemovePublic, opts.IncludePrivate = true, true
	src := "/// Public api.\npub fn api() {}\n\n/// helper doc.\nfn helper() {}\n"
	assert.Equal(t, []string{"doc:helper doc.", "fn:helper"}, distillRustTop(t, src, opts))
}

// walkRust calls visit for every node of the IR below file, depth first.
func walkRust(file *ir.DistilledFile, visit func(ir.DistilledNode)) {
	var walk func(nodes []ir.DistilledNode)
	walk = func(nodes []ir.DistilledNode) {
		for _, node := range nodes {
			visit(node)
			if class, ok := node.(*ir.DistilledClass); ok {
				walk(class.Children)
			}
		}
	}
	walk(file.Children)
}

// declNames returns the names of the classes, functions and fields among
// nodes, in order; comments and imports are skipped.
func declNames(nodes []ir.DistilledNode) []string {
	names := []string{}
	for _, node := range nodes {
		switch n := node.(type) {
		case *ir.DistilledClass:
			names = append(names, n.Name)
		case *ir.DistilledFunction:
			names = append(names, n.Name)
		case *ir.DistilledField:
			names = append(names, n.Name)
		}
	}
	return names
}

// allNames returns the names of the classes, functions and fields anywhere in
// file's IR.
func allNames(file *ir.DistilledFile) []string {
	var names []string
	walkRust(file, func(node ir.DistilledNode) {
		names = append(names, declNames([]ir.DistilledNode{node})...)
	})
	return names
}

const multiLineSignature = "pub fn f(\n    a: u32,\n    b: u32,\n) -> bool\nwhere\n    u32: Copy,\n{ true }"

func TestRust_MultiLineSignature(t *testing.T) {
	file := parseRust(t, multiLineSignature)
	require.Equal(t, []string{"f"}, declNames(file.Children))
	f := findRustFn(t, file, "f")
	assert.Equal(t, []ir.Parameter{{Name: "a", Type: ir.TypeRef{Name: "u32"}}, {Name: "b", Type: ir.TypeRef{Name: "u32"}}}, f.Parameters)
	require.NotNil(t, f.Returns)
	assert.Equal(t, "bool where u32: Copy,", f.Returns.Name)
}

func TestRust_ImplGenericsInName(t *testing.T) {
	file := parseRust(t, "impl<T: Clone> Container<T> for Vec<T> {}\n")
	findRustClass(t, file, "impl<T: Clone> Container<T> for Vec<T>")
}

func TestRust_WhereBeforeBraceKeepsMembers(t *testing.T) {
	src := "pub struct Bounded<T>\nwhere\n    T: Clone,\n{\n    pub value: T,\n    count: usize,\n}\n\n" +
		"impl<T> Bounded<T>\nwhere\n    T: Clone + Default,\n{\n    pub fn reset(&mut self) {}\n}\n\n" +
		"pub trait Container<T>\nwhere\n    T: Clone,\n{\n    fn get(&self, i: usize) -> Option<&T>;\n}\n"
	file := parseRust(t, src)
	assert.Equal(t, []string{"Bounded<T>", "impl<T> Bounded<T>", "Container<T>"}, declNames(file.Children))
	assert.Len(t, file.Children, 3)
	assert.Equal(t, []string{"value", "count"}, childNames(findRustClass(t, file, "Bounded<T>")))
	assert.Equal(t, []string{"reset"}, childNames(findRustClass(t, file, "impl<T> Bounded<T>")))
	assert.Equal(t, []string{"get"}, childNames(findRustClass(t, file, "Container<T>")))
}

func TestRust_TraitAsyncUnsafeMembers(t *testing.T) {
	file := parseRust(t, "trait T { async fn a(&self); unsafe fn b(&self); fn c(&self) {} }\n")
	assert.Equal(t, []string{"a", "b", "c"}, childNames(findRustClass(t, file, "T")))
	assert.Contains(t, findRustFnIn(t, file, "T", "a").Modifiers, ir.ModifierAbstract)
	assert.Contains(t, findRustFnIn(t, file, "T", "b").Modifiers, ir.ModifierAbstract)
	assert.NotContains(t, findRustFnIn(t, file, "T", "c").Modifiers, ir.ModifierAbstract)
}

func TestRust_TrailingCommentNotInType(t *testing.T) {
	s := findRustClass(t, parseRust(t, "pub struct S {\n    content: String, // note\n}\n"), "S")
	require.Len(t, s.Children, 1)
	field, ok := s.Children[0].(*ir.DistilledField)
	require.True(t, ok)
	assert.Equal(t, "content", field.Name)
	assert.Equal(t, "String", field.Type.Name)
}

func TestRust_DocsOfSkippedStatementsAreDropped(t *testing.T) {
	file := parseRust(t, "/// d\nthread_local! { static X: u8 = 0; }\npub struct S;\n")
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			assert.NotEqual(t, "d", c.Text)
		}
	})
	findRustClass(t, file, "S")
}

func TestRust_UseTrees(t *testing.T) {
	tests := []struct {
		src     string
		module  string
		symbols []ir.ImportedSymbol
	}{
		{"use std::io::{self, Read, Write as W};", "std::io", []ir.ImportedSymbol{{Name: "self"}, {Name: "Read"}, {Name: "Write", Alias: "W"}}},
		{"use std::fmt::Display as Disp;", "std::fmt::Display as Disp", nil},
		{"use std::{\n    fs::File,\n    path::{Path, PathBuf},\n};", "std", []ir.ImportedSymbol{{Name: "fs::File"}, {Name: "path::{Path, PathBuf}"}}},
		{"use super::*;", "super::*", nil},
	}
	for _, tt := range tests {
		t.Run(tt.src, func(t *testing.T) {
			file := parseRust(t, tt.src)
			require.Len(t, file.Children, 1)
			imp, ok := file.Children[0].(*ir.DistilledImport)
			require.True(t, ok)
			assert.Equal(t, "use", imp.ImportType)
			assert.Equal(t, tt.module, imp.Module)
			if tt.symbols == nil {
				assert.Empty(t, imp.Symbols)
			} else {
				assert.Equal(t, tt.symbols, imp.Symbols)
			}
		})
	}
}

func TestRust_CommentLines(t *testing.T) {
	file := parseRust(t, "/// a\n/// b\nfn f() {}\n")
	require.Len(t, file.Children, 3)
	for i, want := range []string{"a", "b"} {
		c, ok := file.Children[i].(*ir.DistilledComment)
		require.True(t, ok)
		assert.Equal(t, want, c.Text)
		assert.Equal(t, ir.CommentDoc, c.Format)
		assert.Equal(t, i+1, c.Location.StartLine, want)
		assert.Equal(t, i+1, c.Location.EndLine, want)
	}
}

func TestRust_ErrorRecovery(t *testing.T) {
	file, err := NewProcessor().Process(context.Background(), strings.NewReader("pub fn a() {}\nfn b( {\npub fn c() {}\n"), "test.rs")
	require.NoError(t, err)
	assert.Empty(t, file.Errors)
	names := allNames(file)
	assert.Contains(t, names, "c")
	seen := map[string]bool{}
	for _, name := range names {
		assert.False(t, seen[name], "%q is present twice", name)
		seen[name] = true
	}
}

func TestRust_DeepNesting(t *testing.T) {
	const depth = 200
	var sb strings.Builder
	for i := 0; i < depth; i++ {
		fmt.Fprintf(&sb, "mod m%d { ", i)
	}
	sb.WriteString("pub fn leaf() {}")
	sb.WriteString(strings.Repeat(" }", depth))

	file, err := NewProcessor().Process(context.Background(), strings.NewReader(sb.String()), "test.rs")
	require.NoError(t, err)
	nodes := file.Children
	for i := 0; i < depth; i++ {
		require.Len(t, nodes, 1, "level %d", i)
		mod, ok := nodes[0].(*ir.DistilledClass)
		require.True(t, ok, "level %d", i)
		require.Equal(t, fmt.Sprintf("m%d", i), mod.Name)
		nodes = mod.Children
	}
	assert.Equal(t, []string{"leaf"}, declNames(nodes))
}

func TestRust_DocAboveAttributeFollowsItem(t *testing.T) {
	src := "/// pub doc\n#[derive(Debug)]\npub struct A;\n/// priv doc\n#[derive(Debug)]\nstruct B;\n"
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.rs", rustPublicOnly)
	require.NoError(t, err)
	var out []string
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			out = append(out, "doc:"+c.Text)
		}
		out = append(out, declNames([]ir.DistilledNode{node})...)
	})
	assert.Equal(t, []string{"doc:pub doc", "A"}, out)
}

func TestRust_CRLFAndUnicode(t *testing.T) {
	src := strings.ReplaceAll(multiLineSignature+"\n/// příliš\npub struct Žluť { pub ü: u8 }\n", "\n", "\r\n")
	file := parseRust(t, src)
	walkRust(file, func(node ir.DistilledNode) {
		var texts []string
		switch n := node.(type) {
		case *ir.DistilledFunction:
			texts = append(texts, n.Name)
			if n.Returns != nil {
				texts = append(texts, n.Returns.Name)
			}
			for _, p := range n.Parameters {
				texts = append(texts, p.Name, p.Type.Name)
			}
		case *ir.DistilledClass:
			texts = append(texts, n.Name)
		case *ir.DistilledField:
			texts = append(texts, n.Name)
			if n.Type != nil {
				texts = append(texts, n.Type.Name)
			}
		case *ir.DistilledComment:
			texts = append(texts, n.Text)
		}
		for _, text := range texts {
			assert.NotContains(t, text, "\r")
		}
	})

	assert.Equal(t, "bool where u32: Copy,", findRustFn(t, file, "f").Returns.Name)
	s := findRustClass(t, file, "Žluť")
	assert.Equal(t, []string{"ü"}, childNames(s))
	var comments []string
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			comments = append(comments, c.Text)
		}
	})
	assert.Equal(t, []string{"příliš"}, comments)
}

func TestRust_ItemsInsideBodiesAreNotDistilled(t *testing.T) {
	file := parseRust(t, "fn main() { struct L; impl L {} }\nconst _: () = { struct H; };\nmacro_rules! m { () => { struct M; } }\n"+
		"fn main2() { s! { struct N; } }\n")
	assert.Equal(t, []string{"main", "_", "m", "main2"}, declNames(file.Children))
	assert.Len(t, file.Children, 4)
	for _, name := range allNames(file) {
		assert.NotContains(t, []string{"L", "H", "M", "N", "impl L"}, name)
	}
}

func TestRust_ItemsInMacroCalls(t *testing.T) {
	src := "s! {\n    /// d\n    pub struct A { pub x: u8 }\n}\n" + // lines 1-4
		"cfg_if::cfg_if! {\n" + // line 5
		"    if #[cfg(unix)] {\n        pub fn on_unix() {}\n" + // lines 6-7
		"    } else {\n        pub fn elsewhere() {}\n    }\n}\n" + // lines 8-11
		"lazy_static! { static ref X: u8 = 0; }\n" // line 12
	file := parseRust(t, src)

	require.GreaterOrEqual(t, len(file.Children), 2)
	doc, ok := file.Children[0].(*ir.DistilledComment)
	require.True(t, ok, "the doc inside the macro body comes first")
	assert.Equal(t, "d", doc.Text)
	assert.Equal(t, 2, doc.Location.StartLine)
	assert.Equal(t, 2, doc.Location.EndLine)
	assert.Equal(t, 5, doc.Location.StartColumn)

	a := findRustClass(t, file, "A")
	assert.Same(t, a, file.Children[1], "the doc stands right before A")
	assert.Equal(t, 3, a.Location.StartLine)
	assert.Equal(t, 5, a.Location.StartColumn)
	assert.Equal(t, []string{"x"}, childNames(a))

	assert.Equal(t, 7, findRustFn(t, file, "on_unix").Location.StartLine)
	assert.Equal(t, 9, findRustFn(t, file, "elsewhere").Location.StartLine)
	assert.NotContains(t, allNames(file), "ref")

	var x *ir.DistilledField
	walkRust(file, func(node ir.DistilledNode) {
		if f, ok := node.(*ir.DistilledField); ok && f.Name == "X" {
			x = f
		}
	})
	require.NotNil(t, x, "the static inside lazy_static! is distilled")
	assert.Equal(t, 12, x.Location.StartLine)
	assert.Equal(t, 16, x.Location.StartColumn, "a body that starts on the line of its brace keeps the column")
}

// A macro body with an error keeps the items the parser recognised around
// it; the doc before the ERROR node is dropped.
func TestRust_MacroBodyWithErrors(t *testing.T) {
	file := parseRust(t, "m! {\n    pub fn kept(x: u8) -> u8;\n    /// gone\n    $x:ident => 1;\n    pub type T = u8;\n}\n")
	assert.Equal(t, []string{"kept", "T"}, declNames(file.Children))
	assert.Equal(t, 2, findRustFn(t, file, "kept").Location.StartLine)
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			assert.NotEqual(t, "gone", c.Text)
		}
	})
}

// An error inside one item of a macro body (Rust 2024 safe fn, libc's
// {const} fn) keeps the items the parser recognised, and a function body is
// never read as items.
func TestRust_MacroBodyWithAnErrorInsideAnItem(t *testing.T) {
	src := "f! {\n    pub safe fn a() -> bool {\n        // inside\n        true\n    }\n    pub fn b() {}\n" + // lines 1-6
		"    pub {const} fn c(len: usize) -> usize {\n        // also inside\n        struct Local;\n        len\n    }\n}\n" // lines 7-12
	file := parseRust(t, src)
	assert.Equal(t, []string{"a", "b", "c"}, declNames(file.Children))
	assert.Len(t, file.Children, 3, "nothing else reaches item level")
	assert.Equal(t, 2, findRustFn(t, file, "a").Location.StartLine)
	assert.Equal(t, 6, findRustFn(t, file, "b").Location.StartLine)
	assert.Equal(t, 7, findRustFn(t, file, "c").Location.StartLine)
	for _, name := range allNames(file) {
		assert.NotEqual(t, "Local", name)
	}
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			assert.Fail(t, "a comment of a function body reached item level", c.Text)
		}
	})
}

// The {} blocks inside a statement the parser could not read
// (if #[cfg(..)] { .. }) are expanded in place, between the items around it;
// the blocks of recognised items are not.
func TestRust_MacroBodyMixesItemsAndCfgBlocks(t *testing.T) {
	src := "m! {\n    pub fn first() {}\n    if #[cfg(unix)] {\n        pub fn branch() {}\n    }\n" + // lines 1-5
		"    impl S {\n        pub fn inner() {}\n    }\n    pub fn last() {}\n}\n" // lines 6-10
	file := parseRust(t, src)
	assert.Equal(t, []string{"first", "branch", "impl S", "last"}, declNames(file.Children))
	assert.Equal(t, 4, findRustFn(t, file, "branch").Location.StartLine)
	assert.Equal(t, 7, findRustFnIn(t, file, "impl S", "inner").Location.StartLine)
}

// A function the parser cannot read, whose header and body lie in one ERROR
// node (hyper's ffi_fn! { fn f() -> T { .. } ?= default }), does not let its
// body reach item level.
func TestRust_MacroBodyNeverReadsAFunctionBody(t *testing.T) {
	src := "ffi_fn! {\n    /// h docs\n    fn h() -> u8 {\n        // body note\n        struct Local;\n        0\n    } ?= 0\n}\n"
	file := parseRust(t, src)
	for _, name := range allNames(file) {
		assert.NotEqual(t, "Local", name)
	}
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			assert.NotEqual(t, "body note", c.Text)
		}
	})
	assert.Empty(t, file.Children, "the doc before the unreadable function is dropped too")
}

// A body that does not parse at all (its root is an ERROR node, as for an
// attribute before tokio's cfg_metrics_variant! arms stable: { .. },
// unstable: { .. }) yields only the items of its {} blocks.
func TestRust_MacroBodyThatDoesNotParse(t *testing.T) {
	src := "impl M {\n    cfg_metrics_variant! {\n        #[cfg(x)] stable: {\n            fn s(m: &W, now: Option<I>) -> B {\n                B { a: 0 }\n            }\n        },\n" + // lines 1-7
		"        unstable: {\n            fn u(m: &W, now: Option<I>) -> B {\n                let t = now.map(|n| P { s: n });\n                // in u\n                B { a: 0, t }\n            }\n        }\n    }\n}\n" // lines 8-16
	file := parseRust(t, src)
	assert.Equal(t, 4, findRustFnIn(t, file, "impl M", "s").Location.StartLine)
	assert.Equal(t, 9, findRustFnIn(t, file, "impl M", "u").Location.StartLine)
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			assert.Fail(t, "a comment of a function body reached item level", c.Text)
		}
	})
}

// Macro bodies nested deeper than maxMacroDepth are not distilled.
func TestRust_MacroBodyDepthLimit(t *testing.T) {
	src := ""
	for level := 1; level <= maxMacroDepth+1; level++ {
		src += fmt.Sprintf("m! { pub struct L%d; ", level)
	}
	src += strings.Repeat("}", maxMacroDepth+1) + "\n"
	names := declNames(parseRust(t, src).Children)
	assert.Len(t, names, maxMacroDepth)
	assert.Contains(t, names, fmt.Sprintf("L%d", maxMacroDepth))
	assert.NotContains(t, names, fmt.Sprintf("L%d", maxMacroDepth+1))
}

// An item of a macro body whose name the parser reads across an error
// (bitflags!, paste!) yields no IR, with its docs, and nothing from inside it
// reaches the enclosing list; an error after the name keeps the item.
func TestRust_MacroBodyItemNamedAcrossAnError(t *testing.T) {
	tests := []struct {
		name, src string
		want      []string
	}{
		{"bitflags with a path type", "bitflags::bitflags! {\n    /// Mode doc.\n    #[repr(transparent)]\n    pub struct Mode: libc::mode_t {\n        const S_IRWXU = libc::S_IRWXU;\n        const X = Self::S_IRWXU.bits() | Self::S_IRUSR.bits();\n    }\n}\n", nil},
		{"bitflags with a primitive type", "bitflags! {\n    pub struct Flags: u32 {\n        const A = 0b0001;\n        const AB = Self::A.bits() | Self::B.bits();\n    }\n}\n", []string{"Flags"}},
		{"paste function", "paste! {\n    /// Getter doc.\n    pub fn [<get_ $name>](&self) -> u8 { struct InBody; 1 }\n}\n", nil},
		{"paste struct and impl", "paste! {\n    pub struct [<Foo Bar>] { pub a: u8 }\n    impl [<Foo Bar>] { pub fn new() -> Self { todo!() } }\n}\n", nil},
		{"syntax of the macro after the name", "ast_struct! {\n    pub struct ExprArray #full {\n        pub elems: u8,\n    }\n}\n", []string{"ExprArray", "elems"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file := parseRust(t, tt.src)
			if tt.want == nil {
				assert.Empty(t, allNames(file), "an item named across an error yields nothing, its members included")
				assert.Empty(t, file.Children, "its docs go too")
				return
			}
			assert.Equal(t, tt.want[:1], declNames(file.Children))
			assert.Equal(t, tt.want, allNames(file))
		})
	}
}

// A macro body is parsed as items only when its first token, after comments,
// starts an item: an item keyword, an attribute, the if of cfg_if! or a macro
// call. Any other body yields no IR.
func TestRust_MacroBodyMustStartLikeItems(t *testing.T) {
	read := map[string]string{
		"doc then pub":       "/// d\n    pub fn a() {}",
		"attribute":          "#[cfg(unix)] fn a() {}",
		"inner attribute":    "#![cfg(unix)] fn a() {}",
		"cfg_if":             "if #[cfg(unix)] { fn a() {} }",
		"macro call":         "inner! { fn a() {} }",
		"macro call by path": "cfg_if::cfg_if! { if #[cfg(unix)] { fn a() {} } }",
		"extern":             "extern \"C\" { fn x(); } fn a() {}",
		"macro_rules":        "macro_rules! m { () => {} } fn a() {}",
		"union":              "union U { x: u8 } fn a() {}",
		"unsafe":             "unsafe impl Send for X {} fn a() {}",
		"default":            "default fn a() {}",
		"async":              "async fn a() {}",
	}
	for name, body := range read {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, allNames(parseRust(t, "m! {\n    "+body+"\n}\n")), "a")
		})
	}
	skipped := map[string]string{
		"tokio arms":   "stable: {\n        fn s(m: &W) -> B {\n            B { a: 0 }\n        }\n    },\n    unstable: {\n        fn u(m: &W) -> B {\n            B { a: 0 }\n        }\n    }",
		"select arms":  "v = rx.recv() => { fn s() {} }",
		"table":        "users (id) { id -> Integer, }",
		"block":        "{ fn a() {} }",
		"metavariable": "$vis struct S;",
	}
	for name, body := range skipped {
		t.Run(name, func(t *testing.T) {
			file := parseRust(t, "m! {\n    "+body+"\n}\n")
			assert.Empty(t, allNames(file))
			assert.Empty(t, file.Children, "no comment either")
		})
	}

	// A body of comments only (an empty cfg_if! branch) keeps them.
	file := parseRust(t, "m! {\n    // Unknown target_arch\n}\n")
	require.Len(t, file.Children, 1)
	assert.Equal(t, "Unknown target_arch", file.Children[0].(*ir.DistilledComment).Text)
}

// tableMacro returns a macro call whose body holds lead and then n table
// entries x0 => { pub fn f0() {} }, ... on lines of their own.
func tableMacro(lead string, n int) string {
	var b strings.Builder
	b.WriteString("m! {\n" + lead)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "    x%d => { pub fn f%d() {} },\n", i, i)
	}
	b.WriteString("}\npub fn after() {}\n")
	return b.String()
}

// A long table in a macro body is not parsed as items: tree-sitter's error
// recovery is quadratic on text that is not items (20,000 entries took 15 s).
func TestRust_MacroBodyTableIsNotParsed(t *testing.T) {
	src := tableMacro("", 20000)
	start := time.Now()
	file := parseRust(t, src)
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.Equal(t, []string{"after"}, allNames(file))
}

// A body that starts like items and goes on as a long table stops at the
// parse budget instead of running for seconds; it yields at most its leading
// items.
func TestRust_MacroBodyParseHasABudget(t *testing.T) {
	src := tableMacro("    pub fn lead() {}\n", 20000)
	start := time.Now()
	file := parseRust(t, src)
	assert.Less(t, time.Since(start), 5*time.Second)
	names := allNames(file)
	assert.Contains(t, names, "after")
	assert.Subset(t, []string{"lead", "after"}, names)
}

func TestRust_TrailingCommentsYieldNoIR(t *testing.T) {
	src := "#[derive(Debug)] // attr note\n" +
		"pub struct A;\n" +
		"pub const X: u8 = 1; // const note\n" +
		"pub struct S {\n    pub x: u8, // field note\n}\n" +
		"impl T for A {\n    type C = u8; // assoc note\n    fn f(&self) {} // fn note\n}\n" +
		"// own line\n" +
		"pub fn g() {}\n"
	file := parseRust(t, src)
	var comments []string
	walkRust(file, func(node ir.DistilledNode) {
		if c, ok := node.(*ir.DistilledComment); ok {
			comments = append(comments, c.Text)
		}
	})
	assert.Equal(t, []string{"own line"}, comments)
	assert.Equal(t, []string{"A", "X", "S", "impl T for A", "g"}, declNames(file.Children))
}

// rustExt returns the Rust extensions of class, or an empty value when it has
// none.
func rustExt(class *ir.DistilledClass) ir.RustExtensions {
	if class.Extensions == nil || class.Extensions.Rust == nil {
		return ir.RustExtensions{}
	}
	return *class.Extensions.Rust
}

// fieldsOf returns the fields among class's members, in order.
func fieldsOf(class *ir.DistilledClass) []*ir.DistilledField {
	var fields []*ir.DistilledField
	for _, child := range class.Children {
		if field, ok := child.(*ir.DistilledField); ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func TestRust_TupleStructFields(t *testing.T) {
	src := "pub struct Pair(pub i32, String);\npub struct Meters(f64);\n" +
		"pub struct Multi(\n    #[serde(skip)] pub(crate) u8, // c\n    (\n        u16,\n        u32,\n    ),\n);\npub struct Unit;\n"
	file := parseRust(t, src)

	pair := findRustClass(t, file, "Pair")
	assert.Equal(t, []ir.Modifier{ir.ModifierStruct}, pair.Modifiers)
	assert.Equal(t, 2, rustExt(pair).TupleArity)
	fields := fieldsOf(pair)
	require.Len(t, fields, 2)
	assert.Equal(t, "0", fields[0].Name)
	assert.Equal(t, ir.VisibilityPublic, fields[0].Visibility)
	assert.Equal(t, "i32", fields[0].Type.Name)
	assert.Equal(t, "1", fields[1].Name)
	assert.Equal(t, ir.VisibilityPrivate, fields[1].Visibility)
	assert.Equal(t, "String", fields[1].Type.Name)

	meters := findRustClass(t, file, "Meters")
	assert.Equal(t, 1, rustExt(meters).TupleArity)
	assert.Equal(t, []string{"0"}, childNames(meters))

	multi := findRustClass(t, file, "Multi")
	assert.Equal(t, 2, rustExt(multi).TupleArity)
	fields = fieldsOf(multi)
	require.Len(t, fields, 2)
	assert.Equal(t, ir.VisibilityInternal, fields[0].Visibility, "the attribute before the visibility is skipped")
	assert.Equal(t, "u8", fields[0].Type.Name)
	assert.Equal(t, 4, fields[0].Location.StartLine)
	assert.Equal(t, 20, fields[0].Location.StartColumn, "a field starts at its visibility")
	assert.Equal(t, "(u16, u32)", fields[1].Type.Name, "a type over several lines is normalised")

	assert.Zero(t, rustExt(findRustClass(t, file, "Unit")).TupleArity)
}

func TestRust_InlineModKeepsNestedItems(t *testing.T) {
	src := "pub mod inline_mod {\n    use super::*;\n    pub fn inside() -> u8 { 1 }\n    pub struct Inner;\n" +
		"    mod deeper {}\n    pub type T = u8;\n    pub union U { a: u8 }\n    extern \"C\" { fn c(); }\n}\nmod external;\n"
	file := parseRust(t, src)

	mod := findRustClass(t, file, "inline_mod")
	assert.Empty(t, mod.Modifiers, "a module has no kind modifier")
	require.Len(t, mod.Children, 7)
	_, isImport := mod.Children[0].(*ir.DistilledImport)
	assert.True(t, isImport, "use inside a mod is a child of the mod")
	assert.Equal(t, []string{"inside", "Inner", "deeper", "T", "U", `extern "C"`}, childNames(mod))

	external := findRustClass(t, file, "external")
	assert.Empty(t, external.Modifiers)
	assert.Empty(t, external.Children)
}

func TestRust_Union(t *testing.T) {
	src := "/// Doc.\npub union FFIData<T> where T: Copy {\n    i: u32,\n    pub f: f32,\n}\n"
	file := parseRust(t, src)
	require.Len(t, file.Children, 2, "the doc stays with the union")
	union := findRustClass(t, file, "FFIData<T>")
	assert.Equal(t, []ir.Modifier{ir.ModifierUnion}, union.Modifiers)
	assert.Equal(t, ir.VisibilityPublic, union.Visibility)
	fields := fieldsOf(union)
	require.Len(t, fields, 2)
	assert.Equal(t, "i", fields[0].Name)
	assert.Equal(t, ir.VisibilityPrivate, fields[0].Visibility)
	assert.Equal(t, "u32", fields[0].Type.Name)
	assert.Equal(t, "f", fields[1].Name)
	assert.Equal(t, ir.VisibilityPublic, fields[1].Visibility)
	assert.Equal(t, "where T: Copy", rustExt(union).Where)
}

func TestRust_ExternBlock(t *testing.T) {
	src := "/// FFI.\nextern \"C\" {\n    fn validate(x: i32) -> bool;\n    pub fn other();\n    pub static environ: *const u8;\n}\n" +
		"extern {\n    fn bare();\n    type Opaque;\n}\n"
	file := parseRust(t, src)
	require.Len(t, file.Children, 3, "the doc stays with the block")

	block := findRustClass(t, file, `extern "C"`)
	assert.Equal(t, []ir.Modifier{ir.ModifierExtern}, block.Modifiers)
	assert.Equal(t, ir.VisibilityPublic, block.Visibility)
	require.Len(t, block.Children, 3)

	validate, ok := block.Children[0].(*ir.DistilledFunction)
	require.True(t, ok)
	assert.Equal(t, "validate", validate.Name)
	assert.Equal(t, ir.VisibilityPrivate, validate.Visibility)
	assert.Equal(t, []ir.Modifier{ir.ModifierAbstract}, validate.Modifiers)
	assert.Equal(t, "bool", validate.Returns.Name)

	other, ok := block.Children[1].(*ir.DistilledFunction)
	require.True(t, ok)
	assert.Equal(t, ir.VisibilityPublic, other.Visibility)
	assert.Contains(t, other.Modifiers, ir.ModifierAbstract)

	environ, ok := block.Children[2].(*ir.DistilledField)
	require.True(t, ok)
	assert.Equal(t, "environ", environ.Name)
	assert.Equal(t, ir.VisibilityPublic, environ.Visibility)
	assert.Equal(t, []ir.Modifier{ir.ModifierStatic}, environ.Modifiers)
	assert.Equal(t, "*const u8", environ.Type.Name)

	bare := findRustClass(t, file, "extern")
	assert.Equal(t, []ir.Modifier{ir.ModifierExtern}, bare.Modifiers)
	assert.Equal(t, []string{"bare", "Opaque"}, childNames(bare))
	assert.Equal(t, ir.VisibilityPrivate, fieldsOf(bare)[0].Visibility, "an extern type has its own visibility")
}

func TestRust_MacroRules(t *testing.T) {
	src := "/// Squares.\nmacro_rules! square_it {\n    ($x:expr) => { $x * $x };\n    ( $a:expr,\n      $b:expr $(,)? ) => ( $a * $b );\n}\n\n" +
		"#[macro_export]\n// note\nmacro_rules! exported {\n    () => {};\n}\n" +
		"macro_rules! paren ( [] => {}; );\n"
	file := parseRust(t, src)

	square := findRustFn(t, file, "square_it")
	assert.Equal(t, []ir.Modifier{ir.ModifierMacro}, square.Modifiers)
	assert.Equal(t, ir.VisibilityInternal, square.Visibility)
	assert.Equal(t, []ir.Parameter{{Name: "($x:expr)"}, {Name: "($a:expr, $b:expr $(,)?)"}}, square.Parameters,
		"each rule's matcher, normalised; the commas the macro matches stay")
	assert.Equal(t, "{\n    ($x:expr) => { $x * $x };\n    ( $a:expr,\n      $b:expr $(,)? ) => ( $a * $b );\n}", square.Implementation)
	doc, ok := file.Children[0].(*ir.DistilledComment)
	require.True(t, ok, "the doc of a macro stays with it")
	assert.Equal(t, "Squares.", doc.Text)

	exported := findRustFn(t, file, "exported")
	assert.Equal(t, ir.VisibilityPublic, exported.Visibility, "#[macro_export] makes a macro public")
	assert.Equal(t, []ir.Parameter{{Name: "()"}}, exported.Parameters)

	paren := findRustFn(t, file, "paren")
	assert.Equal(t, "( [] => {}; )", paren.Implementation, "the body ends at its closing delimiter")
	assert.Equal(t, []ir.Parameter{{Name: "[]"}}, paren.Parameters)
}

func TestRust_ExternCrate(t *testing.T) {
	file := parseRust(t, "extern crate alloc;\nextern crate foo as bar;\npub extern crate self as me;\n")
	require.Len(t, file.Children, 3)
	want := []ir.DistilledImport{
		{ImportType: "extern crate", Module: "alloc"},
		{ImportType: "extern crate", Module: "foo as bar"},
		{ImportType: "pub extern crate", Module: "self as me"},
	}
	for i, w := range want {
		imp, ok := file.Children[i].(*ir.DistilledImport)
		require.True(t, ok)
		assert.Equal(t, w.ImportType, imp.ImportType)
		assert.Equal(t, w.Module, imp.Module)
		assert.Empty(t, imp.Symbols)
	}
}

func TestRust_PubUse(t *testing.T) {
	file := parseRust(t, "pub use crate::inner::Thing;\npub(crate) use a::{b, c};\npub( in crate::x ) use y;\nuse z;\n")
	require.Len(t, file.Children, 4)
	var types []string
	for _, child := range file.Children {
		imp, ok := child.(*ir.DistilledImport)
		require.True(t, ok)
		types = append(types, imp.ImportType)
	}
	assert.Equal(t, []string{"pub use", "pub(crate) use", "pub(in crate::x) use", "use"}, types)
	assert.Equal(t, "crate::inner::Thing", file.Children[0].(*ir.DistilledImport).Module)
}

func TestRust_UseInsideMod(t *testing.T) {
	file := parseRust(t, "mod tests {\n    use super::*;\n    pub use crate::x::{A, B};\n    fn t() {}\n}\n")
	mod := findRustClass(t, file, "tests")
	require.Len(t, mod.Children, 3)
	first, ok := mod.Children[0].(*ir.DistilledImport)
	require.True(t, ok)
	assert.Equal(t, "use", first.ImportType)
	assert.Equal(t, "super::*", first.Module)
	second, ok := mod.Children[1].(*ir.DistilledImport)
	require.True(t, ok)
	assert.Equal(t, "pub use", second.ImportType)
	assert.Equal(t, "crate::x", second.Module)
}

func TestRust_TraitConsts(t *testing.T) {
	src := "pub trait Animal {\n    type Food;\n    const LEGS: u32;\n    const NAME: &'static str = \"animal\";\n    fn name(&self) -> String;\n}\n"
	trait := findRustClass(t, parseRust(t, src), "Animal")
	assert.Equal(t, []string{"Food", "LEGS", "NAME", "name"}, childNames(trait))
	fields := fieldsOf(trait)
	require.Len(t, fields, 3)
	legs, name := fields[1], fields[2]
	assert.Equal(t, []ir.Modifier{ir.ModifierFinal}, legs.Modifiers)
	assert.Equal(t, ir.VisibilityPublic, legs.Visibility)
	assert.Equal(t, "u32", legs.Type.Name)
	assert.Empty(t, legs.DefaultValue)
	assert.Equal(t, []ir.Modifier{ir.ModifierFinal}, name.Modifiers)
	assert.Equal(t, ir.VisibilityPublic, name.Visibility)
	assert.Equal(t, "&'static str", name.Type.Name)
	assert.Equal(t, `"animal"`, name.DefaultValue)
}

func TestRust_Supertraits(t *testing.T) {
	src := "pub trait Animal: Display + Clone {}\n" +
		"pub trait Wide<T>: ?Sized + for<'a> Fn(\n    &'a T,\n) + Iterator<Item = T>\n    + 'static {}\n" +
		"pub trait Plain {}\n"
	file := parseRust(t, src)
	assert.Equal(t, []ir.TypeRef{{Name: "Display"}, {Name: "Clone"}}, findRustClass(t, file, "Animal").Extends)
	assert.Equal(t, []ir.TypeRef{{Name: "?Sized"}, {Name: "for<'a> Fn(&'a T)"}, {Name: "Iterator<Item = T>"}, {Name: "'static"}},
		findRustClass(t, file, "Wide<T>").Extends)
	assert.Empty(t, findRustClass(t, file, "Plain").Extends)
}

func TestRust_WhereOnHeaders(t *testing.T) {
	src := "pub struct Bounded<T>\nwhere\n    T: Clone,\n{\n    pub value: T,\n}\n" +
		"pub struct W<T>(T) where T: Clone;\n" +
		"pub enum E<T> where T: Copy { A(T) }\n" +
		"pub union U<T> where T: Copy { a: T }\n" +
		"pub trait Container<T>: Sized where T: Clone {}\n" +
		"impl<T> Copy for Own<T> where T: ?Sized {}\n" +
		"impl<T> Bounded<T>\nwhere\n    T: Clone\n        + Default,\n{\n    pub fn reset(&mut self) {}\n}\n" +
		"pub struct Plain;\n"
	file := parseRust(t, src)
	// Normalised, and without the trailing comma of a clause over several
	// lines: the header goes on with " {" or ";".
	for name, where := range map[string]string{
		"Bounded<T>":              "where T: Clone",
		"W<T>":                    "where T: Clone",
		"E<T>":                    "where T: Copy",
		"U<T>":                    "where T: Copy",
		"Container<T>":            "where T: Clone",
		"impl<T> Copy for Own<T>": "where T: ?Sized",
		"impl<T> Bounded<T>":      "where T: Clone + Default",
		"Plain":                   "",
	} {
		assert.Equal(t, where, rustExt(findRustClass(t, file, name)).Where, name)
	}
	assert.Equal(t, []string{"value"}, childNames(findRustClass(t, file, "Bounded<T>")))
	assert.Equal(t, []string{"reset"}, childNames(findRustClass(t, file, "impl<T> Bounded<T>")))
}

func TestRust_TraitImplAssociatedItemsArePublic(t *testing.T) {
	src := "impl Iterator for Counter {\n    type Item = u32;\n    const X: u8 = 1;\n    fn next(&mut self) -> Option<u32> { None }\n}\n" +
		"impl Counter {\n    const Y: u8 = 2;\n    type Z = u8;\n}\n"
	file := parseRust(t, src)
	traitImpl := findRustClass(t, file, "impl Iterator for Counter")
	fields := fieldsOf(traitImpl)
	require.Len(t, fields, 2)
	assert.Equal(t, "Item", fields[0].Name)
	assert.Equal(t, ir.VisibilityPublic, fields[0].Visibility)
	assert.Equal(t, []ir.Modifier{ir.ModifierTypeAlias}, fields[0].Modifiers)
	assert.Equal(t, "X", fields[1].Name)
	assert.Equal(t, ir.VisibilityPublic, fields[1].Visibility)

	for _, field := range fieldsOf(findRustClass(t, file, "impl Counter")) {
		assert.Equal(t, ir.VisibilityPrivate, field.Visibility, "members of an inherent impl keep their own visibility: %s", field.Name)
	}
}
