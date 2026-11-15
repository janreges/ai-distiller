package rust

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
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
				assert.Len(t, imp2.Symbols, 2, "Expected two imported symbols")
				assert.Equal(t, "Read", imp2.Symbols[0].Name)
				assert.Equal(t, "Write", imp2.Symbols[1].Name)
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

func TestRust_BraceDelta(t *testing.T) {
	assert.Equal(t, 1, braceDelta("pub fn a() {"))
	assert.Equal(t, -1, braceDelta("}"))
	assert.Equal(t, 0, braceDelta(`println!("}"); // }`))
	assert.Equal(t, 0, braceDelta(`let s = "\"{";`))
	// Char literals hold braces and quotes; lifetimes are not char literals.
	assert.Equal(t, 0, braceDelta(`'{' => depth += 1,`))
	assert.Equal(t, 1, braceDelta(`'}' => {`))
	assert.Equal(t, 1, braceDelta(`if c == '"' {`))
	assert.Equal(t, 1, braceDelta(`if c == '\'' {`))
	assert.Equal(t, 1, braceDelta(`fn f<'a>(x: &'a str) -> &'a str {`))
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

func TestRust_OneLineBodyOpenOnly(t *testing.T) {
	// A body that opens on the signature line but closes later is not a
	// one-line body.
	assert.Equal(t, "", oneLineBody("pub fn f() { let x = 1;"))
	assert.Equal(t, "", oneLineBody("pub fn f() -> i32;"))
	assert.Equal(t, "{ x }", oneLineBody("pub fn f(x: i32) -> i32 { x } // trailing"))
}
