package formatter

import "strings"

// dedentBody splits a body into lines re-indented under prefix, ready to
// print. Parsers store a body from its first token, so line one has no
// indentation while later lines keep what they had in the file: line one is
// only left-trimmed and prefixed, the rest go through dedentLines. Leading and
// trailing blank lines are dropped; blank lines inside are "".
func dedentBody(impl, prefix string) []string {
	lines := strings.Split(strings.ReplaceAll(impl, "\r\n", "\n"), "\n")
	first := strings.TrimLeft(lines[0], " \t")
	if strings.TrimSpace(first) != "" {
		first = prefix + first
	} else {
		first = ""
	}
	return trimBlankEdges(append([]string{first}, dedentLines(lines[1:], prefix)...))
}

// dedentLines replaces the indentation the lines of a body share with prefix.
// The shared indentation is the longest whitespace prefix of the non-blank
// lines that are indented at all. When those lines disagree on tabs versus
// spaces (no line's indentation is a prefix of all the others), their tabs
// are first expanded to 4-column stops. A non-blank line at column 0 inside
// an indented body is kept verbatim, without prefix: it is almost always the
// content of a multi-line string, template literal or heredoc, which must not
// move. When no line is indented, every line is code and gets prefix. Blank
// lines become "".
func dedentLines(lines []string, prefix string) []string {
	var indents []string // indentation of the indented non-blank lines
	for _, l := range lines {
		if ws := leadingSpace(l); ws != "" && strings.TrimSpace(l) != "" {
			indents = append(indents, ws)
		}
	}
	common, expand := sharedIndent(indents)

	out := make([]string, len(lines))
	for i, l := range lines {
		ws := leadingSpace(l)
		switch {
		case strings.TrimSpace(l) == "":
			// blank: stays ""
		case ws == "" && len(indents) > 0:
			out[i] = l
		default:
			code := l[len(ws):]
			if expand {
				ws = expandTabs(ws)
			}
			out[i] = prefix + strings.TrimPrefix(ws, common) + code
		}
	}
	return out
}

// sharedIndent returns the longest prefix shared by indents, and whether tabs
// have to be expanded first. That is the case when the indents mix tabs and
// spaces so that even the shortest one is not a prefix of all the others; the
// prefix is then the one shared by the expanded indents.
func sharedIndent(indents []string) (common string, expand bool) {
	if len(indents) == 0 {
		return "", false
	}
	common, shortest := indents[0], len(indents[0])
	for _, ws := range indents[1:] {
		n := 0
		for n < len(common) && n < len(ws) && common[n] == ws[n] {
			n++
		}
		common = common[:n]
		shortest = min(shortest, len(ws))
	}
	if len(common) == shortest {
		return common, false
	}
	expanded := make([]string, len(indents))
	for i, ws := range indents {
		expanded[i] = expandTabs(ws)
	}
	common, _ = sharedIndent(expanded)
	return common, true
}

// leadingSpace returns the run of spaces and tabs that starts l.
func leadingSpace(l string) string {
	return l[:len(l)-len(strings.TrimLeft(l, " \t"))]
}

// expandTabs turns the tabs of an indentation into spaces up to the next
// 4-column tab stop.
func expandTabs(ws string) string {
	col := 0
	for _, c := range ws {
		if c == '\t' {
			col += 4 - col%4
		} else {
			col++
		}
	}
	return strings.Repeat(" ", col)
}

func trimBlankEdges(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	if start == end {
		return nil
	}
	return lines[start:end]
}

// renderBodyVerbatim renders a body that carries its own braces ("{ ... }")
// as the text that follows a signature, keeping every line as the source
// wrote it: the first line goes on the signature line after a space, and the
// lines after it keep their own indentation, tabs included, after indent (the
// signature line's indentation). Go uses it: its functions are declared at
// file level, so the source indentation of a body is already right under the
// signature, and its output indents with tabs, as gofmt does. Blank lines
// print empty. The result always ends with "\n".
func renderBodyVerbatim(impl, indent string) string {
	raw := strings.Split(strings.ReplaceAll(impl, "\r\n", "\n"), "\n")
	raw = trimBlankEdges(raw)
	if len(raw) == 0 {
		return "\n"
	}
	var b strings.Builder
	b.WriteString(" " + strings.TrimSpace(raw[0]) + "\n")
	for _, l := range raw[1:] {
		if strings.TrimSpace(l) == "" {
			b.WriteString("\n")
			continue
		}
		b.WriteString(indent + l + "\n")
	}
	return b.String()
}

// renderBody renders a body that carries its own braces ("{ ... }") as the
// text that follows a signature. indent is the signature line's indentation.
// The result always ends with "\n".
func renderBody(impl, indent string) string {
	raw := strings.Split(strings.ReplaceAll(impl, "\r\n", "\n"), "\n")
	raw = trimBlankEdges(raw)
	if len(raw) == 0 {
		return "\n"
	}
	var b strings.Builder
	b.WriteString(" " + strings.TrimSpace(raw[0]) + "\n")
	if len(raw) == 1 {
		return b.String()
	}
	rest := raw[1:]
	closing := ""
	if strings.HasPrefix(strings.TrimSpace(rest[len(rest)-1]), "}") {
		closing = strings.TrimSpace(rest[len(rest)-1])
		rest = rest[:len(rest)-1]
	}
	for _, l := range dedentLines(rest, indent+"    ") {
		b.WriteString(l + "\n")
	}
	if closing != "" {
		b.WriteString(indent + closing + "\n")
	}
	return b.String()
}
