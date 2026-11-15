package ruby

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseRuby returns the raw IR of src (no stripping).
func parseRuby(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	file, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "test.rb")
	require.NoError(t, err)
	return file
}

// findRubyClass returns the class called name, searching the top level of file
// and the children of modules (and of classes, for nested classes). The test
// fails when there is none.
func findRubyClass(t *testing.T, file *ir.DistilledFile, name string) *ir.DistilledClass {
	t.Helper()
	var search func(children []ir.DistilledNode) *ir.DistilledClass
	search = func(children []ir.DistilledNode) *ir.DistilledClass {
		for _, c := range children {
			switch n := c.(type) {
			case *ir.DistilledClass:
				if n.Name == name {
					return n
				}
				if found := search(n.Children); found != nil {
					return found
				}
			case *ir.DistilledInterface:
				if found := search(n.Children); found != nil {
					return found
				}
			}
		}
		return nil
	}
	cls := search(file.Children)
	if cls == nil {
		t.Fatalf("class %q not found", name)
	}
	return cls
}

func TestRuby_RequireAttrInclude(t *testing.T) {
	file := parseRuby(t, "require 'json'\nrequire_relative 'lib/x'\n\nclass A\n  include Comparable\n  include Foo::Bar\n  attr_reader :name\n  attr_accessor :age, :city\nend\n")

	var modules []string
	for _, c := range file.Children {
		if imp, ok := c.(*ir.DistilledImport); ok {
			modules = append(modules, imp.ImportType+" "+imp.Module)
		}
	}
	assert.Equal(t, []string{"require json", "require_relative lib/x"}, modules)

	cls := findRubyClass(t, file, "A")
	var impl []string
	for _, r := range cls.Implements {
		impl = append(impl, r.Name)
	}
	assert.Equal(t, []string{"Comparable", "Foo::Bar"}, impl)

	var fns []string
	for _, c := range cls.Children {
		if fn, ok := c.(*ir.DistilledFunction); ok {
			fns = append(fns, fn.Name)
		}
	}
	assert.Equal(t, []string{"name", "age", "age=", "city", "city="}, fns)
}

func TestRuby_KeywordCallsNeedNoReceiver(t *testing.T) {
	file := parseRuby(t, "YAML.load(\"a.yml\")\nFile.extend Bar\nclass A\n  Foo.include Bar\n  extend Baz\n  prepend Qux\nend\n")

	for _, c := range file.Children {
		_, isImport := c.(*ir.DistilledImport)
		assert.False(t, isImport, "YAML.load must not become an import")
	}
	cls := findRubyClass(t, file, "A")
	assert.Empty(t, cls.Implements, "receiver include, extend and prepend must not become mixins")
}

// rubyImports returns "<keyword> <module>" for every import of file.
func rubyImports(file *ir.DistilledFile) []string {
	var modules []string
	for _, c := range file.Children {
		if imp, ok := c.(*ir.DistilledImport); ok {
			modules = append(modules, imp.ImportType+" "+imp.Module)
		}
	}
	return modules
}

func TestRuby_RequireNeedsAPlainStringLiteral(t *testing.T) {
	src := "require File.expand_path('../boot', __FILE__)\n" +
		"require \"dyn#{x}\"\n" +
		"require ENV['X']\n" +
		"require %q(pct)\n" +
		"require(\"paren\")\n" +
		"load 'tasks.rb'\n"
	assert.Equal(t, []string{"require pct", "require paren", "load tasks.rb"}, rubyImports(parseRuby(t, src)))
}

func TestRuby_SelfReceiverKeywordCalls(t *testing.T) {
	cls := findRubyClass(t, parseRuby(t, "class A\n  self.include Comparable\n  self.attr_reader :name\nend\n"), "A")
	require.Len(t, cls.Implements, 1)
	assert.Equal(t, "Comparable", cls.Implements[0].Name)
	var fns []string
	for _, c := range cls.Children {
		if fn, ok := c.(*ir.DistilledFunction); ok {
			fns = append(fns, fn.Name)
		}
	}
	assert.Equal(t, []string{"name"}, fns)
}
