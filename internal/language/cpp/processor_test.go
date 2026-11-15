package cpp

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessor_Basic(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		expected []string
	}{
		{
			name: "simple_class",
			code: `
class MyClass {
public:
    int getValue() { return value; }
private:
    int value;
};`,
			expected: []string{
				"MyClass",
				"getValue",
				"value",
			},
		},
		{
			name: "namespace",
			code: `
namespace MyNamespace {
    void myFunction() {}
}`,
			expected: []string{
				"MyNamespace",
				"myFunction",
			},
		},
		{
			name: "struct_with_methods",
			code: `
struct Point {
    int x, y;
    int distance() { return x + y; }
};`,
			expected: []string{
				"Point",
				"x",
				"y",
				"distance",
			},
		},
		{
			name: "virtual_functions",
			code: `
class Base {
public:
    virtual void doSomething() = 0;
    virtual void doAnother() {}
};`,
			expected: []string{
				"Base",
				"doSomething",
				"doAnother",
			},
		},
		{
			name: "template_function",
			code: `
template<typename T>
T max(T a, T b) {
    return a > b ? a : b;
}`,
			expected: []string{
				"max",
			},
		},
	}

	processor := NewProcessor()
	ctx := context.Background()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := strings.NewReader(tt.code)
			result, err := processor.Process(ctx, reader, "test.cpp")
			require.NoError(t, err)
			require.NotNil(t, result)

			// Convert result to string for easier checking
			content := convertToString(result)

			// Check that all expected elements are present
			for _, exp := range tt.expected {
				assert.Contains(t, content, exp, "Expected to find %s in output", exp)
			}
		})
	}
}

func TestProcessor_ComplexFeatures(t *testing.T) {
	code := `
#include <iostream>
#include <vector>

using namespace std;

// Template class
template<typename T>
class Container {
private:
    vector<T> items;
public:
    void add(T item) { items.push_back(item); }
    T get(int index) { return items[index]; }
};

// Abstract base class
class Shape {
public:
    virtual double area() = 0;
    virtual ~Shape() {}
};

// Derived class
class Circle : public Shape {
private:
    double radius;
public:
    Circle(double r) : radius(r) {}
    double area() override { return 3.14 * radius * radius; }
};

// Function with default parameter
void print(const string& msg = "Hello") {
    cout << msg << endl;
}

// Static member
class Counter {
    static int count;
public:
    Counter() { count++; }
    static int getCount() { return count; }
};

int Counter::count = 0;
`

	processor := NewProcessor()
	ctx := context.Background()

	reader := strings.NewReader(code)
	result, err := processor.Process(ctx, reader, "complex.cpp")
	require.NoError(t, err)
	require.NotNil(t, result)

	content := convertToString(result)

	// Check various C++ features
	assert.Contains(t, content, "Container")
	assert.Contains(t, content, "Shape")
	assert.Contains(t, content, "Circle")
	assert.Contains(t, content, "area")
	// assert.Contains(t, content, "override") // TODO: Add override modifier support
	assert.Contains(t, content, "static")
	assert.Contains(t, content, "count")
}

// Helper function to convert result to string representation
func convertToString(file *ir.DistilledFile) string {
	var sb strings.Builder

	// Recursively collect all names from the IR structure
	var collectNames func(node ir.DistilledNode)
	collectNames = func(node ir.DistilledNode) {
		switch n := node.(type) {
		case *ir.DistilledClass:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledStruct:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledFunction:
			sb.WriteString(n.Name + " ")
			for _, mod := range n.Modifiers {
				sb.WriteString(string(mod) + " ")
			}
		case *ir.DistilledField:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledPackage:
			sb.WriteString(n.Name + " ")
		case *ir.DistilledEnum:
			sb.WriteString(n.Name + " ")
		}

		// Process children
		if node != nil {
			for _, child := range node.GetChildren() {
				collectNames(child)
			}
		}
	}

	// Process all top-level nodes
	for _, node := range file.Children {
		collectNames(node)
	}

	return sb.String()
}

func parseCpp(t *testing.T, code string) *ir.DistilledFile {
	t.Helper()
	file, err := NewTreeSitterProcessor().ProcessSource(context.Background(), []byte(code), "test.cpp")
	require.NoError(t, err)
	return file
}

func findCppTopLevelFn(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledFunction {
	t.Helper()
	for _, child := range file.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok && fn.Name == name {
			return fn
		}
	}
	require.Failf(t, "function not found", "no top-level function named %q", name)
	return nil
}

func TestCpp_OutOfClassDefinitionHasQualifiedName(t *testing.T) {
	file := parseCpp(t, "class Foo { public: int bar(); };\nint Foo::bar() { return 1; }\n")
	fn := findCppTopLevelFn(t, file, "Foo::bar")
	assert.Equal(t, "int", fn.Returns.Name)
	assert.Equal(t, ir.VisibilityPublic, fn.Visibility)
}

func TestCpp_OutOfClassDefinitionInheritsVisibility(t *testing.T) {
	file := parseCpp(t, "class Foo { int secret(); public: int open(); };\nint Foo::secret() { return 1; }\nint Foo::open() { return 2; }\n")
	assert.Equal(t, ir.VisibilityPrivate, findCppTopLevelFn(t, file, "Foo::secret").Visibility)
	assert.Equal(t, ir.VisibilityPublic, findCppTopLevelFn(t, file, "Foo::open").Visibility)
}

// findCppFn returns the function named name among nodes, searching namespace
// (package) children too, and fails the test when there is none.
func findCppFn(t *testing.T, nodes []ir.DistilledNode, name string) *ir.DistilledFunction {
	t.Helper()
	var walk func([]ir.DistilledNode) *ir.DistilledFunction
	walk = func(nodes []ir.DistilledNode) *ir.DistilledFunction {
		for _, n := range nodes {
			switch n := n.(type) {
			case *ir.DistilledFunction:
				if n.Name == name {
					return n
				}
			case *ir.DistilledPackage:
				if fn := walk(n.Children); fn != nil {
					return fn
				}
			}
		}
		return nil
	}
	fn := walk(nodes)
	require.NotNil(t, fn, "no function named %q", name)
	return fn
}

func TestCpp_OutOfClassDefinitionVisibilityLayouts(t *testing.T) {
	cases := []struct {
		name string
		code string
		want map[string]ir.Visibility
	}{
		{"class and definitions in a namespace",
			"namespace ns {\nclass Foo { int secret(); public: int open(); };\nint Foo::secret() { return 1; }\nint Foo::open() { return 2; }\n}\n",
			map[string]ir.Visibility{"Foo::secret": ir.VisibilityPrivate, "Foo::open": ir.VisibilityPublic}},
		{"namespace-qualified definition",
			"namespace ns { class Foo { protected: int prot(); public: int open(); }; }\nint ns::Foo::prot() { return 1; }\nint ns::Foo::open() { return 2; }\n",
			map[string]ir.Visibility{"ns::Foo::prot": ir.VisibilityProtected, "ns::Foo::open": ir.VisibilityPublic}},
		{"nested namespaces",
			"namespace a { namespace b { class Foo { int secret(); }; int Foo::secret() { return 1; } } }\n",
			map[string]ir.Visibility{"Foo::secret": ir.VisibilityPrivate}},
		{"nested class",
			"class Bar {\npublic:\n  class In { int deeph(); public: int deepp(); };\n};\nint Bar::In::deeph() { return 1; }\nint Bar::In::deepp() { return 2; }\n",
			map[string]ir.Visibility{"Bar::In::deeph": ir.VisibilityPrivate, "Bar::In::deepp": ir.VisibilityPublic}},
		{"member of a private nested class is no more open than the class",
			"class Bar {\n  struct Impl { int run(); };\n};\nint Bar::Impl::run() { return 1; }\n",
			map[string]ir.Visibility{"Bar::Impl::run": ir.VisibilityPrivate}},
		{"class template",
			"template <typename T>\nclass Tp { T get(); public: T put(); };\ntemplate <typename T>\nT Tp<T>::get() { return T(); }\ntemplate <typename T>\nT Tp<T>::put() { return T(); }\n",
			map[string]ir.Visibility{"Tp<T>::get": ir.VisibilityPrivate, "Tp<T>::put": ir.VisibilityPublic}},
		{"operator",
			"class Foo { bool operator<(const Foo& o) const; };\nbool Foo::operator<(const Foo& o) const { return true; }\n",
			map[string]ir.Visibility{"Foo::operator<": ir.VisibilityPrivate}},
		{"struct members are public by default",
			"struct S { int f(); private: int g(); };\nint S::f() { return 1; }\nint S::g() { return 2; }\n",
			map[string]ir.Visibility{"S::f": ir.VisibilityPublic, "S::g": ir.VisibilityPrivate}},
		{"no declaration keeps the definition public",
			"int Unknown::f() { return 1; }\n",
			map[string]ir.Visibility{"Unknown::f": ir.VisibilityPublic}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			file := parseCpp(t, tc.code)
			for fnName, want := range tc.want {
				assert.Equal(t, want, findCppFn(t, file.Children, fnName).Visibility, fnName)
			}
		})
	}
}

// commentsOf returns the format and text of every comment among nodes.
func commentsOf(nodes []ir.DistilledNode) [][2]string {
	var got [][2]string
	for _, c := range nodes {
		if cm, ok := c.(*ir.DistilledComment); ok {
			got = append(got, [2]string{cm.Format, cm.Text})
		}
	}
	return got
}

func TestCpp_Comments(t *testing.T) {
	file := parseCpp(t, "// line c\n\n/** Doc c */\nint f();\n")
	assert.Equal(t, [][2]string{{ir.CommentLine, "line c"}, {ir.CommentDoc, "Doc c"}}, commentsOf(file.Children))
}

// labels names every node: "doc:<text>" (with "inner " for an inner doc),
// "line:<text>" and "block:<text>" for comments, the name for the rest.
func labels(nodes []ir.DistilledNode) []string {
	var out []string
	for _, n := range nodes {
		switch n := n.(type) {
		case *ir.DistilledComment:
			prefix := n.Format + ":"
			if n.Inner {
				prefix = "inner " + prefix
			}
			out = append(out, prefix+n.Text)
		case *ir.DistilledFunction:
			out = append(out, n.Name)
		case *ir.DistilledField:
			out = append(out, n.Name)
		case *ir.DistilledClass:
			out = append(out, n.Name)
		case *ir.DistilledStruct:
			out = append(out, n.Name)
		case *ir.DistilledImport:
			out = append(out, "import "+n.Module)
		case *ir.DistilledPackage:
			out = append(out, "namespace "+n.Name)
		}
	}
	return out
}

// Doxygen reads ///, //!, /** */ and /*! */ as the doc of the next
// declaration; a run of /// lines is one doc.
func TestCpp_DoxygenDocs(t *testing.T) {
	file := parseCpp(t, "//! Qt line doc\nint a();\n/*! Qt block doc */\nint b();\n/// first\n/// second\nint c();\n/// alone\n\n/// apart\nint d();\n")
	assert.Equal(t, []string{"doc:Qt line doc", "a", "doc:Qt block doc", "b", "doc:first\nsecond", "c", "doc:alone", "doc:apart", "d"}, labels(file.Children))
}

func TestCpp_ClassBodyComments(t *testing.T) {
	file := parseCpp(t, "class K {\npublic:\n    /// member doc\n    int g();\n    // body line\n    /* body block */\n    int h();\n};\nstruct S {\n    /** s doc */\n    int f() { return 1; }\n    // s line\n    int n;\n};\n")
	k := file.Children[0].(*ir.DistilledClass)
	assert.Equal(t, []string{"doc:member doc", "g", "line:body line", "block:body block", "h"}, labels(k.Children))
	s := file.Children[1].(*ir.DistilledStruct)
	assert.Equal(t, []string{"doc:s doc", "f", "line:s line", "n"}, labels(s.Children))
}

// A trailing member doc (///<, //!<, /**<, /*!<) documents the member before
// it, so it is placed in front of that member.
func TestCpp_TrailingMemberDoc(t *testing.T) {
	file := parseCpp(t, "class P {\n    /** x doc */\n    double x_;  ///< X coordinate\n    double y_;  /**< Y coordinate */\n    int f(); //!< f doc\n};\nint top; ///< top doc\nstruct E {\n    ///< documents nothing\n    int a;\n};\n")
	p := file.Children[0].(*ir.DistilledClass)
	assert.Equal(t, []string{"doc:x doc", "doc:X coordinate", "x_", "doc:Y coordinate", "y_", "doc:f doc", "f"}, labels(p.Children))
	assert.Equal(t, []string{"P", "doc:top doc", "top", "E"}, labels(file.Children))
	e := file.Children[3].(*ir.DistilledStruct)
	assert.Equal(t, []string{"line:documents nothing", "a"}, labels(e.Children))
}

// A trailing doc after a member that adds no declaration (here a pointer
// field, which the parser leaves out) goes with that member; it does not
// document the member before.
func TestCpp_TrailingDocOfMemberWithoutDeclaration(t *testing.T) {
	file := parseCpp(t, "class R {\n    int get();\n    int* p_; ///< pointer\n};\n")
	assert.Equal(t, []string{"get"}, labels(file.Children[0].(*ir.DistilledClass).Children))
}

// The docs before a statement that adds no declaration (a concept, a macro, a
// static_assert, a using declaration in a namespace, a friend) are dropped, so
// they do not document the next declaration; line comments keep their place.
func TestCpp_DocOfStatementWithoutDeclarationIsDropped(t *testing.T) {
	file := parseCpp(t, "/** concept doc */\ntemplate<typename T>\nconcept C = true;\n/** macro doc */\n#define MAX 10\n// kept line\n/** f doc */\nint f();\nnamespace n {\n/** using doc */\nusing std::string;\n/** g doc */\nint g();\n}\nclass K {\n    /** assert doc */\n    static_assert(true, \"x\");\n    /** friend doc */\n    friend class F;\n    /** h doc */\n    int h();\n};\n")
	assert.Equal(t, []string{"line:kept line", "doc:f doc", "f", "import std::string", "namespace n", "K"}, labels(file.Children))
	assert.Equal(t, []string{"doc:g doc", "g"}, labels(file.Children[4].(*ir.DistilledPackage).Children))
	assert.Equal(t, []string{"doc:h doc", "h"}, labels(file.Children[5].(*ir.DistilledClass).Children))
}

// A friend is not a member and not a comment: it adds nothing, and the body
// of a friend function never reaches the output.
func TestCpp_FriendAddsNothing(t *testing.T) {
	file := parseCpp(t, "class V {\n    /** out doc */\n    friend std::ostream& operator<<(std::ostream& os, const V& v) {\n        os << v.n;\n        return os;\n    }\n    friend bool operator==(const V&, const V&);\n    friend class W;\npublic:\n    int n;\n};\nstruct S {\n    friend class W;\n    int m;\n};\n")
	assert.Equal(t, []string{"n"}, labels(file.Children[0].(*ir.DistilledClass).Children))
	assert.Equal(t, []string{"m"}, labels(file.Children[1].(*ir.DistilledStruct).Children))
}

// Include guards, #ifdef/#else branches and extern "C" blocks hold
// declarations of the file: the docs of a #define inside them are dropped as
// at file level, the docs of real declarations (also prototypes inside
// extern "C") stay, and a @file doc before the include guard stays the
// file's doc.
func TestCpp_DocsInsidePreprocessorAndExternCBlocks(t *testing.T) {
	src := "/** @file w.h */\n#ifndef W_H\n#define W_H\n/** Max size */\n#define MAX 10\n/** W doc */\nclass W {};\n" +
		"#ifdef A\n/** a doc */\nint a();\n/** a macro */\n#define AM 1\n#else\n/** b macro */\n#define BM 1\n/** b doc */\nint b();\n#endif\n" +
		"#if defined(X) && X > 1\n/** x macro */\n#define XM 1\n/** x doc */\nint x();\n#elif defined(Y)\n/** y macro */\n#define YM 1\n/** y doc */\nint y();\n#endif\n" +
		"#ifdef __cplusplus\nextern \"C\" {\n#endif\n/** c doc */\nint c(void);\n/** d macro */\n#define D 1\n/** e doc */\nint e(void);\n#ifdef __cplusplus\n}\n#endif\n" +
		"extern \"C\" {\n/** m macro */\n#define M 1\n/** g doc */\nint g(void);\n}\n#endif\n"
	file := parseCpp(t, src)
	assert.Equal(t, []string{"inner doc:@file w.h", "doc:W doc", "W", "doc:a doc", "a", "doc:b doc", "b",
		"doc:x doc", "x", "doc:y doc", "y", "doc:c doc", "c", "doc:e doc", "e", "doc:g doc", "g"}, labels(file.Children))
}

// A doc with @file or \file documents the file, also when a #pragma comes
// next; it is not the doc of the first declaration.
func TestCpp_FileDocIsInner(t *testing.T) {
	file := parseCpp(t, "/**\n * @file widget.h\n * Widget API.\n */\n#pragma once\n/** W doc */\nclass W {};\n")
	assert.Equal(t, []string{"inner doc:@file widget.h\nWidget API.", "doc:W doc", "W"}, labels(file.Children))
	file = parseCpp(t, "/*! \\file widget.h */\nclass W {};\n")
	assert.Equal(t, []string{"inner doc:\\file widget.h", "W"}, labels(file.Children))
	// A header of macros only: the file doc does not wait for a declaration,
	// so a statement that adds none (a #define, the guard) does not drop it.
	file = parseCpp(t, "/** @file m.h */\n#define M 1\n")
	assert.Equal(t, []string{"inner doc:@file m.h"}, labels(file.Children))
	file = parseCpp(t, "/** @file m.h */\n#ifndef M_H\n#define M_H\n/** M doc */\n#define M 1\n#endif\n")
	assert.Equal(t, []string{"inner doc:@file m.h"}, labels(file.Children))
}
