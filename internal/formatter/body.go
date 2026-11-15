package formatter

import "strings"

// dedentBody splits a body into lines with the source indentation removed.
// Parsers store a body from its first token, so line one has no indentation
// while later lines keep what they had in the file. The common indentation is
// the longest whitespace prefix shared by the non-blank lines after the first;
// line one is only left-trimmed. Leading and trailing blank lines are dropped.
func dedentBody(impl string) []string {
	lines := strings.Split(strings.ReplaceAll(impl, "\r\n", "\n"), "\n")
	lines[0] = strings.TrimLeft(lines[0], " \t")
	if len(lines) > 1 {
		copy(lines[1:], dedentLines(lines[1:]))
	}
	return trimBlankEdges(lines)
}

// dedentLines removes the longest whitespace prefix shared by all non-blank
// lines. Blank lines become empty.
func dedentLines(lines []string) []string {
	common := ""
	found := false
	for _, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		ws := l[:len(l)-len(strings.TrimLeft(l, " \t"))]
		if !found {
			common, found = ws, true
			continue
		}
		n := 0
		for n < len(common) && n < len(ws) && common[n] == ws[n] {
			n++
		}
		common = common[:n]
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			continue
		}
		out[i] = strings.TrimPrefix(l, common)
	}
	return out
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
	for _, l := range dedentLines(rest) {
		if l != "" {
			b.WriteString(indent + "    " + l)
		}
		b.WriteString("\n")
	}
	if closing != "" {
		b.WriteString(indent + closing + "\n")
	}
	return b.String()
}
