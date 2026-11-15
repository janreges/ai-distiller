// Package commentutil holds the comment handling shared by the parsers of
// C-family languages (TypeScript, JavaScript, C#, Rust, ...).
package commentutil

import (
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// StripCStyle classifies a raw C-family comment token and removes its delimiters.
// "// x" -> ("x", ir.CommentLine); "/* x */" -> ("x", ir.CommentBlock);
// "/** x */" and "/// x" -> ("x", ir.CommentDoc). Leading " * " gutters are removed.
//
// "/**/" is an empty block. "/***" opens a block (a banner), not a doc, and
// "////" a line comment. The text keeps its inner line breaks and the
// indentation of its lines relative to each other; leading and trailing blank
// lines are dropped.
func StripCStyle(raw string) (text string, format string) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "///") && !strings.HasPrefix(raw, "////"):
		return strings.TrimSpace(raw[3:]), ir.CommentDoc
	case strings.HasPrefix(raw, "//"):
		return strings.TrimSpace(strings.TrimLeft(raw, "/")), ir.CommentLine
	case raw == "/**/":
		return "", ir.CommentBlock
	case strings.HasPrefix(raw, "/**") && !strings.HasPrefix(raw, "/***"):
		return stripBlock(raw[3:]), ir.CommentDoc
	case strings.HasPrefix(raw, "/*"):
		return stripBlock(raw[2:]), ir.CommentBlock
	}
	return raw, ir.CommentLine
}

// StripJSStyle is StripCStyle for JavaScript and TypeScript, whose only doc
// comment is /** */: there "/// x" is a line comment (a triple-slash directive
// such as /// <reference types="node" />, or plain text).
func StripJSStyle(raw string) (text string, format string) {
	text, format = StripCStyle(raw)
	if format == ir.CommentDoc && strings.HasPrefix(strings.TrimSpace(raw), "///") {
		format = ir.CommentLine
	}
	return text, format
}

// AppendStatement appends to list the nodes parsed from one statement that is
// not itself a comment. A doc comment documents the statement after it; when
// that statement yields nothing but comments (a re-export, `module.exports =
// ...`, `export default {...}`, a call), the doc comments right before it and
// among its own nodes are dropped, so they do not appear to document the next
// declaration. Other comments keep their place.
func AppendStatement(list, parsed []ir.DistilledNode) []ir.DistilledNode {
	for _, node := range parsed {
		if !isComment(node) {
			return append(list, parsed...)
		}
	}
	start := len(list)
	for start > 0 && isComment(list[start-1]) {
		start--
	}
	kept := WithoutDocs(append(append([]ir.DistilledNode(nil), list[start:]...), parsed...))
	return append(list[:start], kept...)
}

// WithoutDocs returns the nodes that are not doc comments.
func WithoutDocs(nodes []ir.DistilledNode) []ir.DistilledNode {
	var kept []ir.DistilledNode
	for _, node := range nodes {
		if c, ok := node.(*ir.DistilledComment); ok && c.IsDoc() {
			continue
		}
		kept = append(kept, node)
	}
	return kept
}

func isComment(node ir.DistilledNode) bool {
	_, ok := node.(*ir.DistilledComment)
	return ok
}

// stripBlock returns the text of a block comment whose opening delimiter is
// already removed: without the closing "*/", the "*" gutters (or, when the
// lines have none, their common indentation) and blank edge lines.
func stripBlock(body string) string {
	// The closer with the banner stars glued to it: "*/", "**/", "***/".
	if strings.HasSuffix(body, "*/") {
		body = strings.TrimRight(strings.TrimSuffix(body, "/"), "*")
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	// Banner stars glued to the opener: "/*** x".
	lines[0] = strings.TrimSpace(strings.TrimLeft(lines[0], "*"))

	rest := lines[1:]
	if hasGutters(rest) {
		for i, line := range rest {
			line = strings.TrimLeft(line, " \t")
			line = strings.TrimPrefix(line, "*")
			rest[i] = strings.TrimPrefix(line, " ")
		}
	} else {
		dedent(rest)
	}

	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// hasGutters reports whether every non-blank line starts with a "*" gutter.
func hasGutters(lines []string) bool {
	found := false
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(trimmed, "*") {
			return false
		}
		found = true
	}
	return found
}

// dedent removes the indentation that all non-blank lines share.
func dedent(lines []string) {
	common := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		if common < 0 || indent < common {
			common = indent
		}
	}
	for i, line := range lines {
		if len(line) >= common && common > 0 {
			lines[i] = line[common:]
		}
	}
}

// fileTags are the JSDoc/TSDoc tags that make a doc comment document the
// whole file rather than the declaration after it.
var fileTags = []string{"@file", "@fileoverview", "@overview", "@module", "@packageDocumentation"}

// DocumentsFile reports whether a doc comment's text (delimiters already
// stripped) documents the file: one of its lines starts with @file,
// @fileoverview, @overview, @module or @packageDocumentation.
func DocumentsFile(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		for _, tag := range fileTags {
			if fields[0] == tag {
				return true
			}
		}
	}
	return false
}
