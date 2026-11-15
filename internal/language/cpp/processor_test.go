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
