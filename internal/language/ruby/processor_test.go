package ruby

import (
	"context"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
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

func TestRuby_VisibilitySections(t *testing.T) {
	file := parseRuby(t, "class A\n  def a; end\n  private\n  def b; end\n  attr_reader :r\n  protected\n  def c; end\n  public\n  def d; end\n  private def e; end\n  def f; end\n  def g; end\n  private :g\nend\n\nclass B\n  def h; end\nend\n")
	vis := map[string]ir.Visibility{}
	for _, name := range []string{"A", "B"} {
		for _, c := range findRubyClass(t, file, name).Children {
			if fn, ok := c.(*ir.DistilledFunction); ok {
				vis[fn.Name] = fn.Visibility
			}
		}
	}
	assert.Equal(t, map[string]ir.Visibility{
		"a": ir.VisibilityPublic, "b": ir.VisibilityPrivate, "r": ir.VisibilityPrivate,
		"c": ir.VisibilityProtected, "d": ir.VisibilityPublic, "e": ir.VisibilityPrivate,
		"f": ir.VisibilityPublic, "g": ir.VisibilityPrivate, "h": ir.VisibilityPublic,
	}, vis)
}

func TestRuby_SingletonMethodsStayPublic(t *testing.T) {
	file := parseRuby(t, "class S\n  private\n  def self.a; end\n  def b; end\n  private def self.c; end\n  def self.d; end\n  private :d\nend\n")
	vis := map[string]ir.Visibility{}
	for _, c := range findRubyClass(t, file, "S").Children {
		if fn, ok := c.(*ir.DistilledFunction); ok {
			vis[fn.Name] = fn.Visibility
		}
	}
	assert.Equal(t, map[string]ir.Visibility{
		"a": ir.VisibilityPublic, "b": ir.VisibilityPrivate, "c": ir.VisibilityPublic, "d": ir.VisibilityPublic,
	}, vis)
}

func TestRuby_ModuleInclude(t *testing.T) {
	file := parseRuby(t, "module M\n  include Comparable\n  def x; end\nend\n")
	var mod *ir.DistilledInterface
	for _, c := range file.Children {
		if m, ok := c.(*ir.DistilledInterface); ok {
			mod = m
		}
	}
	require.NotNil(t, mod)
	require.Len(t, mod.Extends, 1)
	assert.Equal(t, "Comparable", mod.Extends[0].Name)
}

func TestRuby_Comments(t *testing.T) {
	file := parseRuby(t, "# line c\n\n# Doc c\n# second\nclass A\n  # inner\n\n  # m doc\n  def m; end\nend\n")
	top := []*ir.DistilledComment{}
	for _, c := range file.Children {
		if cm, ok := c.(*ir.DistilledComment); ok {
			top = append(top, cm)
		}
	}
	require.Len(t, top, 2)
	assert.Equal(t, [2]string{ir.CommentLine, "line c"}, [2]string{top[0].Format, top[0].Text})
	assert.Equal(t, [2]string{ir.CommentDoc, "Doc c\nsecond"}, [2]string{top[1].Format, top[1].Text})
	cls := findRubyClass(t, file, "A")
	assert.Equal(t, ir.CommentLine, cls.Children[0].(*ir.DistilledComment).Format)
	assert.Equal(t, ir.CommentDoc, cls.Children[1].(*ir.DistilledComment).Format)
}

// rubyOutline returns "<format>:<text>" for each comment of nodes and the name
// of every declaration.
func rubyOutline(nodes []ir.DistilledNode) []string {
	var out []string
	for _, n := range nodes {
		switch v := n.(type) {
		case *ir.DistilledComment:
			out = append(out, v.Format+":"+v.Text)
		case *ir.DistilledClass:
			out = append(out, v.Name)
		case *ir.DistilledInterface:
			out = append(out, v.Name)
		case *ir.DistilledFunction:
			out = append(out, v.Name)
		case *ir.DistilledField:
			out = append(out, v.Name)
		case *ir.DistilledImport:
			out = append(out, "require "+v.Module)
		}
	}
	return out
}

// Only a run of "#" lines right above a class, module or def is a doc: a
// comment after code is a line comment and is not part of a run, and a
// comment above anything else is a line comment.
func TestRuby_DocCommentRules(t *testing.T) {
	src := `#!/usr/bin/env ruby
# frozen_string_literal: true

# above require
require 'json'
=begin
block
  indented
=end
# M doc
module M # trailing on the module line
  # only a comment in this body
end
class A
  # Example:
  #
  #   A.new.m
  def m; end # after m
  # name attr
  attr_reader :name
  # MAX const
  MAX = 10
  # private def doc
  private def p1; end
  # dropped: private_class_method adds nothing
  private_class_method def self.hidden; end
  def after; end
  # singleton doc
  class << self
    # s doc
    def s; end
  end
end
`
	file := parseRuby(t, src)
	assert.Equal(t, []string{
		"line:!/usr/bin/env ruby", "line:frozen_string_literal: true", "line:above require", "require json",
		"block:block\n  indented", "doc:M doc", "M", "A",
	}, rubyOutline(file.Children))
	assert.Equal(t, []string{"line:trailing on the module line", "line:only a comment in this body"},
		rubyOutline(file.Children[6].(*ir.DistilledInterface).Children))
	cls := findRubyClass(t, file, "A")
	assert.Equal(t, []string{
		"doc:Example:\n\n  A.new.m", "m", "line:after m",
		"line:name attr", "name", "@name",
		"line:MAX const", "MAX",
		"doc:private def doc", "p1",
		"after",
		"doc:singleton doc", "<<self>>",
	}, rubyOutline(cls.Children))
	assert.Equal(t, []string{"doc:s doc", "s"}, rubyOutline(cls.Children[len(cls.Children)-1].(*ir.DistilledClass).Children))
}

// The "#!" line is a line comment of its own, never a line of the doc below it.
func TestRuby_ShebangIsNotADocLine(t *testing.T) {
	file := parseRuby(t, "#!/usr/bin/env ruby\n# K doc\nclass K; end\n")
	assert.Equal(t, []string{"line:!/usr/bin/env ruby", "doc:K doc", "K"}, rubyOutline(file.Children))
	file = parseRuby(t, "#!/usr/bin/env ruby\nclass K; end\n")
	assert.Equal(t, []string{"line:!/usr/bin/env ruby", "K"}, rubyOutline(file.Children))
}

func TestRuby_ScopedClassName(t *testing.T) {
	file := parseRuby(t, "class Foo::Bar < Base\n  def m; end\nend\n")
	assert.Equal(t, "Foo::Bar", findRubyClass(t, file, "Foo::Bar").Name)
}

// distillRuby runs src through the processor and the text formatter with the
// CLI defaults (public members only), changed by set when it is not nil.
func distillRuby(t *testing.T, src string, set func(*processor.ProcessOptions)) string {
	t.Helper()
	opts := processor.DefaultProcessOptions()
	opts.IncludePrivate = false
	opts.IncludeImplementation = false
	opts.IncludeComments = false
	if set != nil {
		set(&opts)
	}
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "test.rb", opts)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, formatter.NewLanguageAwareTextFormatter(formatter.Options{}).Format(&b, file))
	return b.String()
}

func TestRuby_PrivateClassMethodAndPrivateAttr(t *testing.T) {
	src := "class A\n  def self.s; end\n  def self.t; end\n  private_class_method :s\n  private attr_reader :x\n  attr_reader :y\nend\n"
	def := distillRuby(t, src, nil)
	assert.NotContains(t, def, "def self.s")
	assert.NotContains(t, def, "def x")
	assert.Contains(t, def, "def self.t")
	assert.Contains(t, def, "def y")

	all := distillRuby(t, src, func(o *processor.ProcessOptions) { o.IncludePrivate = true })
	assert.Contains(t, all, "def self.s")
	assert.Contains(t, all, "def x")
	assert.Contains(t, all, "def self.t")
}
