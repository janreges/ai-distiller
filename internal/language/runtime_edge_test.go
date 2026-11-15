package language

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/require"
)

// edgeLanguage is one tree-sitter language with the inputs that need its
// syntax: a string literal holding invalid UTF-8, and one unit of valid code
// that is repeated into a large file.
type edgeLanguage struct {
	name     string
	filename string
	badUTF8  string
	unit     func(i int) string
}

var edgeLanguages = []edgeLanguage{
	{"python", "edge.py", "s = \"a\xff\xfe\xc3(b\"\n",
		func(i int) string {
			return fmt.Sprintf("class C%d:\n    \"\"\"Doc %d.\"\"\"\n\n    def m(self, x: int) -> int:\n        return x + %d\n\n", i, i, i)
		}},
	{"typescript", "edge.ts", "const s: string = \"a\xff\xfe\xc3(b\";\n",
		func(i int) string {
			return fmt.Sprintf("/** Doc %d. */\nexport class C%d {\n  private x: number = %d;\n  m(y: number): number { return this.x + y; }\n}\n", i, i, i)
		}},
	// The doc comment sits in the class body: the JavaScript processor looks
	// for the code after each /** comment among all of the comment's
	// siblings, which takes minutes for thousands of top-level /** comments.
	{"javascript", "edge.js", "const s = \"a\xff\xfe\xc3(b\";\n",
		func(i int) string {
			return fmt.Sprintf("class C%d {\n  /** Doc %d. @param {number} y */\n  m(y) { return this.x + y; }\n  constructor() { this.x = %d; }\n}\n", i, i, i)
		}},
	{"ruby", "edge.rb", "s = \"a\xff\xfe\xc3(b\"\n",
		func(i int) string {
			return fmt.Sprintf("# Doc %d.\nclass C%d\n  def m(x)\n    x + %d\n  end\n\n  private\n\n  def h; end\nend\n", i, i, i)
		}},
	{"java", "Edge.java", "class Edge { String s = \"a\xff\xfe\xc3(b\"; }\n",
		func(i int) string {
			return fmt.Sprintf("/** Doc %d. */\nclass C%d {\n    private int x = %d;\n    public int m(int y) { return x + y; }\n}\n", i, i, i)
		}},
	{"csharp", "Edge.cs", "class Edge { string s = \"a\xff\xfe\xc3(b\"; }\n",
		func(i int) string {
			return fmt.Sprintf("/// <summary>Doc %d.</summary>\npublic class C%d\n{\n    private int x = %d;\n    public int M(int y) { return x + y; }\n}\n", i, i, i)
		}},
	{"kotlin", "Edge.kt", "val s = \"a\xff\xfe\xc3(b\"\n",
		func(i int) string {
			return fmt.Sprintf("/** Doc %d. */\nclass C%d {\n    private val x = %d\n    fun m(y: Int): Int = x + y\n}\n", i, i, i)
		}},
	{"cpp", "edge.cpp", "const char* s = \"a\xff\xfe\xc3(b\";\n",
		func(i int) string {
			return fmt.Sprintf("/// Doc %d.\nclass C%d {\npublic:\n    int m(int y) { return x + y; }\nprivate:\n    int x = %d;\n};\n", i, i, i)
		}},
	// Sixteen classes per namespace block, docs in the class bodies: the PHP
	// processor searches all siblings of a /** comment and of a class, which
	// takes minutes for thousands of top-level classes.
	{"php", "edge.php", "<?php\n$s = \"a\xff\xfe\xc3(b\";\n",
		func(i int) string {
			var sb strings.Builder
			fmt.Fprintf(&sb, "namespace N%d {\n", i)
			for k := 0; k < 16; k++ {
				fmt.Fprintf(&sb, "class C%d\n{\n    /** Doc %d. @var int */\n    private int $x = %d;\n    public function m(int $y): int { return $this->x + $y; }\n}\n", k, k, k)
			}
			sb.WriteString("}\n")
			return sb.String()
		}},
}

// TestRuntime_EdgeInputs distils inputs that stress the tree-sitter runtime
// through each real processor: nothing may panic. An error or an empty result
// is accepted, except for the large valid file, which must be distilled.
func TestRuntime_EdgeInputs(t *testing.T) {
	random := make([]byte, 4096)
	rand.New(rand.NewSource(11)).Read(random)

	for _, lang := range edgeLanguages {
		t.Run(lang.name, func(t *testing.T) {
			// aid's workers share each processor too.
			t.Parallel()
			proc, ok := GetProcessor(lang.name)
			require.True(t, ok, "no processor for %s", lang.name)

			large := largeSource(lang)
			inputs := []struct {
				name string
				src  []byte
			}{
				{"empty", nil},
				{"random-4KB", random},
				{"invalid-utf8-string", []byte(lang.badUTF8)},
				{"generated-2MB", large},
			}
			for _, in := range inputs {
				file, err := proc.ProcessWithOptions(context.Background(), bytes.NewReader(in.src), lang.filename, processor.DefaultProcessOptions())
				if in.name == "generated-2MB" {
					require.NoError(t, err, in.name)
					require.NotEmpty(t, file.Children, in.name)
					continue
				}
				if err != nil {
					t.Logf("%s: error (accepted): %v", in.name, err)
				}
			}
		})
	}
}

// largeSource repeats lang's unit until the source is at least 2 MB.
func largeSource(lang edgeLanguage) []byte {
	var sb strings.Builder
	if lang.name == "php" {
		sb.WriteString("<?php\n")
	}
	for i := 0; sb.Len() < 2<<20; i++ {
		sb.WriteString(lang.unit(i))
	}
	return []byte(sb.String())
}
