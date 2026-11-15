package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// writeCStyleComment renders a comment for C-family languages at the given indentation.
// line: "// " per text line. block: "/* text */". doc: "/** text */" for one line,
// otherwise "/**", " * " per line, " */".
//
// A multi-line block uses the doc layout with "/*". Blank lines get no
// trailing space, and "*/" inside a block or doc is written as "*\/" so it
// does not end the comment early.
func writeCStyleComment(w io.Writer, c *ir.DistilledComment, indent string) {
	lines := strings.Split(c.Text, "\n")
	if !c.IsDoc() && c.Format != ir.CommentBlock {
		for _, line := range lines {
			fmt.Fprintf(w, "%s%s\n", indent, strings.TrimRight("// "+line, " "))
		}
		return
	}

	open := "/*"
	if c.IsDoc() {
		open = "/**"
	}
	for i, line := range lines {
		lines[i] = strings.ReplaceAll(line, "*/", `*\/`)
	}
	if len(lines) == 1 {
		fmt.Fprintf(w, "%s%s */\n", indent, strings.TrimRight(open+" "+lines[0], " "))
		return
	}
	fmt.Fprintf(w, "%s%s\n", indent, open)
	for _, line := range lines {
		fmt.Fprintf(w, "%s%s\n", indent, strings.TrimRight(" * "+line, " "))
	}
	fmt.Fprintf(w, "%s */\n", indent)
}
