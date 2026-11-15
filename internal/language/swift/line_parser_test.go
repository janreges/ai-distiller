package swift

import (
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
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
