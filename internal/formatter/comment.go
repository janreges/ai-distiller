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

// formatCStyleNodes formats sibling nodes as parts, one per comment or node,
// for formatters that build their output from parts. Comments are written by
// writeCStyleComment at indent; format returns the text of any other node and
// whether it prints that node itself. A run of comments waits for the node
// after it: when the formatter does not print that node (a namespace or
// package line it leaves out, a member kind it skips), the doc comments of the
// run are dropped with it, so they do not appear to document whatever is
// printed next. Line and block comments and inner docs keep their place.
func formatCStyleNodes(nodes []ir.DistilledNode, indent string, format func(ir.DistilledNode) (text string, printed bool)) []string {
	var parts []string
	var run []*ir.DistilledComment
	flush := func(keepDocs bool) {
		for _, c := range run {
			if c.IsDoc() && !c.Inner && !keepDocs {
				continue
			}
			var b strings.Builder
			writeCStyleComment(&b, c, indent)
			parts = append(parts, strings.TrimSuffix(b.String(), "\n"))
		}
		run = nil
	}
	for _, node := range nodes {
		if c, ok := node.(*ir.DistilledComment); ok {
			run = append(run, c)
			continue
		}
		text, printed := format(node)
		flush(printed)
		if text != "" {
			parts = append(parts, text)
		}
	}
	flush(false)
	return parts
}
