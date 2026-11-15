package formatter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDedentBody(t *testing.T) {
	cases := map[string]struct {
		in, prefix string
		want       []string
	}{
		"first line bare": {"a = 1\n        return a", "", []string{"a = 1", "return a"}},
		"nested":          {"String x = load(id);\n        if (x) {\n            y();\n        }\n        return x;", "", []string{"String x = load(id);", "if (x) {", "    y();", "}", "return x;"}},
		"blank lines":     {"\n    a\n\n    b\n", "", []string{"a", "", "b"}},
		"tabs":            {"a\n\t\tb\n\t\t\tc", "", []string{"a", "b", "\tc"}},
		"single line":     {"x", "", []string{"x"}},
		"empty":           {"  \n", "", nil},
		"prefix":          {"a\n    if b:\n\n        c", "  ", []string{"  a", "  if b:", "", "      c"}},
		// Content of a multi-line string at column 0 stays where it is and
		// does not stop the code lines from being dedented.
		"column 0 string": {"text = \"\"\"\nhello\n\"\"\"\n        return Service(None)", "    ", []string{"    text = \"\"\"", "hello", "\"\"\"", "    return Service(None)"}},
		// A nested block whose closing line is tab-indented too.
		"tabs with closing line": {"if (x) {\n\t\t\ty();\n\t\t}\n\t\treturn x;", "", []string{"if (x) {", "\ty();", "}", "return x;"}},
		// Tabs and spaces at the same depth: tabs expand to 4-column stops.
		"mixed tabs and spaces": {"a\n\tb\n    c\n\t  d\n\t\te", "", []string{"a", "b", "c", "  d", "    e"}},
		// No line is indented: the whole body is code and is re-indented.
		"unindented body": {"a\nb", "  ", []string{"  a", "  b"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, dedentBody(c.in, c.prefix)) })
	}
}

func TestRenderBody(t *testing.T) {
	cases := map[string]struct{ impl, indent, want string }{
		"one line":   {"{ return a + 1; }", "", " { return a + 1; }\n"},
		"block":      {"{\n        return a + 1;\n    }", "    ", " {\n        return a + 1;\n    }\n"},
		"relative":   {"{\n  const x = 1;\n  if (x) {\n    y();\n  }\n}", "", " {\n    const x = 1;\n    if (x) {\n      y();\n    }\n}\n"},
		"empty":      {"{}", "", " {}\n"},
		"blank impl": {"", "", "\n"},
		"tabs":       {"{\n\t\tconst hit = 1;\n\t\treturn hit;\n\t}", "    ", " {\n        const hit = 1;\n        return hit;\n    }\n"},
		// A template literal line at column 0 inside a tab-indented body.
		"column 0 template": {"{\n\t\tconst hit = `x\nat col0`;\n\t\treturn hit;\n\t}", "    ", " {\n        const hit = `x\nat col0`;\n        return hit;\n    }\n"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, renderBody(c.impl, c.indent)) })
	}
}
