package swift

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

func parse(src string) *ir.DistilledFile { return NewLineParser([]byte(src), "t.swift").Parse() }

func method(t *testing.T, f *ir.DistilledFile, name string) *ir.DistilledFunction {
	for _, c := range f.Children[0].(*ir.DistilledClass).Children {
		if fn, ok := c.(*ir.DistilledFunction); ok && fn.Name == name {
			return fn
		}
	}
	t.Fatalf("method %s not found", name)
	return nil
}

func TestLineParser_MultiLineBody(t *testing.T) {
	f := parse("public class S {\n    public func get() -> Int {\n        let a: Int = 1\n        return a\n    }\n    public func other() -> Int {\n        return 2\n    }\n}\n")
	get := method(t, f, "get")
	assert.Equal(t, "{\n        let a: Int = 1\n        return a\n    }", get.Implementation)
	assert.Equal(t, "Int", get.Returns.Name, "return type must not include the brace")
	assert.Equal(t, "{\n        return 2\n    }", method(t, f, "other").Implementation)
	for _, c := range f.Children[0].(*ir.DistilledClass).Children {
		if fld, ok := c.(*ir.DistilledField); ok {
			t.Errorf("local variable %q was parsed as a class field", fld.Name)
		}
	}
}

func TestLineParser_OneLineBody(t *testing.T) {
	get := method(t, parse("public class S {\n    public func get() -> Int { return 1 }\n}\n"), "get")
	assert.Equal(t, "{ return 1 }", get.Implementation)
	assert.Equal(t, "Int", get.Returns.Name)
}

func TestLineParser_BraceInsideStringLiteral(t *testing.T) {
	f := parse("public class S {\n    public func a() {\n        print(\"}\") // }\n    }\n    public func b() {\n        return\n    }\n}\n")
	assert.Equal(t, "{\n        print(\"}\") // }\n    }", method(t, f, "a").Implementation)
	assert.NotNil(t, method(t, f, "b"))
}

func TestBraceDelta(t *testing.T) {
	assert.Equal(t, 1, braceDelta("func a() {"))
	assert.Equal(t, 0, braceDelta(`print("}") // }`))
	assert.Equal(t, -1, braceDelta("}"))
	assert.Equal(t, 0, braceDelta("func a() { return 1 }"))
}

func TestLineParser_PropertyDefault(t *testing.T) {
	f := parse("public struct P {\n    public var x: Int = 0\n    public var f: (Int) -> Bool = { $0 == 1 }\n}\n")
	p := f.Children[0].(*ir.DistilledClass)
	assert.Contains(t, p.Modifiers, ir.ModifierStruct)
	assert.Equal(t, "0", p.Children[0].(*ir.DistilledField).DefaultValue)
	assert.Equal(t, "{ $0 == 1 }", p.Children[1].(*ir.DistilledField).DefaultValue)
}

// distillSwift runs src through the processor and the text formatter with the
// CLI defaults (public members, no bodies, no comments, docstrings), changed
// by set when it is not nil.
func distillSwift(t *testing.T, src string, set func(*processor.ProcessOptions)) string {
	t.Helper()
	opts := processor.DefaultProcessOptions()
	opts.IncludePrivate = false
	opts.IncludeImplementation = false
	opts.IncludeComments = false
	if set != nil {
		set(&opts)
	}
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "t.swift", opts)
	require.NoError(t, err)
	var b strings.Builder
	require.NoError(t, formatter.NewLanguageAwareTextFormatter(formatter.Options{}).Format(&b, file))
	return b.String()
}

func TestSwift_LineAndBlockCommentsWithComments(t *testing.T) {
	src := "public struct S {\n    // note one\n    public var a: Int = 1\n    /* block one */\n    /* multi\n       line block */\n    public func f() -> Int { return 1 }\n    /// DOC_TEXT\n    public func g() -> Int { return 2 }\n}\n"
	def := distillSwift(t, src, nil)
	for _, hidden := range []string{"note one", "block one", "multi", "line block"} {
		assert.NotContains(t, def, hidden)
	}
	assert.Contains(t, def, "DOC_TEXT")

	with := distillSwift(t, src, func(o *processor.ProcessOptions) { o.IncludeComments = true })
	for _, want := range []string{"note one", "block one", "multi", "line block", "DOC_TEXT"} {
		assert.Contains(t, with, want)
	}
}

func TestSwift_EmptyBlockCommentPrintsNothing(t *testing.T) {
	src := "public struct S {\n    /**/\n    /* */\n    /***/\n    /** */\n    /* kept */\n    public var a: Int = 1\n}\n"
	out := distillSwift(t, src, func(o *processor.ProcessOptions) { o.IncludeComments = true })
	assert.Contains(t, out, "public var a: Int = 1")
	var comments []string
	for _, line := range strings.Split(out, "\n") {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "//") {
			comments = append(comments, trimmed)
		}
	}
	assert.Equal(t, []string{"// kept"}, comments, out)
}

func TestLineParser_CodeAfterBlockComment(t *testing.T) {
	f := parse("public struct S {\n    /* inline */ public var c: Int = 3\n    /* multi\n       line */ public func g() -> Int { return 1 }\n}\n")
	var names []string
	for _, c := range f.Children[0].(*ir.DistilledClass).Children {
		switch n := c.(type) {
		case *ir.DistilledField:
			names = append(names, n.Name)
		case *ir.DistilledFunction:
			names = append(names, n.Name)
		}
	}
	assert.Equal(t, []string{"c", "g"}, names)
}

func TestSwift_ExtensionConformanceOnce(t *testing.T) {
	out := distillSwift(t, "public extension Dictionary: Cacheable { }\n", nil)
	assert.Contains(t, out, "public extension Dictionary: Cacheable")
	assert.Equal(t, 1, strings.Count(out, "Cacheable"), out)
}
