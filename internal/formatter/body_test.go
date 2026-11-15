package formatter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedentBody(t *testing.T) {
	cases := map[string]struct {
		in   string
		want []string
	}{
		"first line bare": {"a = 1\n        return a", []string{"a = 1", "return a"}},
		"nested":          {"String x = load(id);\n        if (x) {\n            y();\n        }\n        return x;", []string{"String x = load(id);", "if (x) {", "    y();", "}", "return x;"}},
		"blank lines":     {"\n    a\n\n    b\n", []string{"a", "", "b"}},
		"tabs":            {"a\n\t\tb\n\t\t\tc", []string{"a", "b", "\tc"}},
		"single line":     {"x", []string{"x"}},
		"empty":           {"  \n", nil},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, dedentBody(c.in)) })
	}
}

func TestRenderBody(t *testing.T) {
	cases := map[string]struct{ impl, indent, want string }{
		"one line":   {"{ return a + 1; }", "", " { return a + 1; }\n"},
		"block":      {"{\n        return a + 1;\n    }", "    ", " {\n        return a + 1;\n    }\n"},
		"relative":   {"{\n  const x = 1;\n  if (x) {\n    y();\n  }\n}", "", " {\n    const x = 1;\n    if (x) {\n      y();\n    }\n}\n"},
		"empty":      {"{}", "", " {}\n"},
		"blank impl": {"", "", "\n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, renderBody(c.impl, c.indent)) })
	}
}
