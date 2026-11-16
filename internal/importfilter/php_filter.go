package importfilter

import (
	"regexp"
	"strings"
)

// PHPFilter handles import filtering for PHP code
type PHPFilter struct {
	BaseFilter
}

// NewPHPFilter creates a new PHP import filter
func NewPHPFilter() ImportFilter {
	return &PHPFilter{
		BaseFilter: NewBaseFilter("php"),
	}
}

// phpUse is one use statement of the formatted text and the names the code
// refers to it by.
type phpUse struct {
	ImportStatement
	// names holds, per imported name, its alias or else its last segment.
	names []phpUsedName
}

// phpUsedName is a name the code uses for an import. Constant names are case
// sensitive in PHP, class and function names are not.
type phpUsedName struct {
	name          string
	caseSensitive bool
}

var (
	// phpUseStartRe matches the start of a use statement at the top level.
	// Indented `use` lines are trait uses inside a class body.
	phpUseStartRe = regexp.MustCompile(`^use\s`)
	// phpUseRe splits a complete use statement into its kind and the rest.
	phpUseRe = regexp.MustCompile(`^use\s+(?:(?i:(function|const))\s+)?(.+?)\s*;\s*$`)
	// phpUseNameRe matches one imported name with an optional kind and alias.
	phpUseNameRe = regexp.MustCompile(`^(?:(?i:(function|const))\s+)?([\w\\]+)(?:\s+(?i:as)\s+(\w+))?$`)
)

// FilterUnusedImports removes the use statements none of whose names occurs
// in the rest of the text.
func (f *PHPFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting PHP import filtering")
	f.LogDebug(debugLevel, 2, "Code length: %d bytes", len(code))

	lines := strings.Split(code, "\n")
	imports := f.parseImports(lines)
	if len(imports) == 0 {
		f.LogDebug(debugLevel, 1, "No imports found")
		return code, nil, nil
	}
	f.LogDebug(debugLevel, 1, "Found %d import statements", len(imports))

	// Every line outside the use statements is searched, so a name used
	// between two groups of imports (a file with several namespaces) counts.
	isImportLine := make(map[int]bool)
	for _, imp := range imports {
		for l := imp.StartLine; l <= imp.EndLine; l++ {
			isImportLine[l-1] = true
		}
	}
	var searchLines []string
	for i, line := range lines {
		if !isImportLine[i] && f.searchable(line) {
			searchLines = append(searchLines, line)
		}
	}
	text := strings.Join(searchLines, "\n")

	var removedImports []string
	var linesToRemove []struct{ start, end int }
	for _, imp := range imports {
		used := false
		for _, n := range imp.names {
			if phpNameUsed(text, n.name, n.caseSensitive) {
				f.LogDebug(debugLevel, 3, "Found usage of '%s'", n.name)
				used = true
				break
			}
		}
		if !used {
			f.LogDebug(debugLevel, 2, "Removing unused import: %s", imp.FullLine)
			removedImports = append(removedImports, imp.FullLine)
			linesToRemove = append(linesToRemove, struct{ start, end int }{imp.StartLine, imp.EndLine})
		}
	}

	// Remove unused imports (in reverse order to maintain line numbers)
	result := code
	for i := len(linesToRemove) - 1; i >= 0; i-- {
		result = f.RemoveLines(result, linesToRemove[i].start, linesToRemove[i].end)
	}

	f.LogDebug(debugLevel, 1, "Removed %d unused imports", len(removedImports))

	return result, removedImports, nil
}

// searchable reports whether a line outside the use statements can show a
// usage. Comment lines cannot; `#[...]` is an attribute, not a `#` comment.
// The file tags and namespace lines name paths, not imported names.
func (f *PHPFilter) searchable(line string) bool {
	trimmed := strings.TrimSpace(line)
	switch {
	case trimmed == "":
		return false
	case strings.HasPrefix(trimmed, "//"):
		return false
	case strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#["):
		return false
	case strings.HasPrefix(trimmed, "<file") || strings.HasPrefix(trimmed, "</file>"):
		return false
	case strings.HasPrefix(trimmed, "namespace ") || strings.HasPrefix(trimmed, "<?php"):
		return false
	}
	return true
}

// phpNameUsed reports whether name occurs as a whole word in text.
func phpNameUsed(text, name string, caseSensitive bool) bool {
	pattern := `\b` + regexp.QuoteMeta(name) + `\b`
	if !caseSensitive {
		pattern = `(?i)` + pattern
	}
	return regexp.MustCompile(pattern).MatchString(text)
}

// parseImports finds the use statements at the top level of the text: one
// name or several (`use A, B as C;`), a group (`use A\{B, C as D};`, also
// written over several lines), each optionally with `function` or `const`.
func (f *PHPFilter) parseImports(lines []string) []phpUse {
	var imports []phpUse
	for i := 0; i < len(lines); i++ {
		if !phpUseStartRe.MatchString(lines[i]) {
			continue
		}
		start := i
		statement := strings.TrimSpace(lines[i])
		// A group written over several lines ends at its closing brace.
		if strings.Contains(statement, "{") {
			for !strings.Contains(statement, "}") && i+1 < len(lines) {
				i++
				statement += " " + strings.TrimSpace(lines[i])
			}
		}
		imp, ok := parsePHPUse(statement)
		if !ok {
			continue
		}
		imp.FullLine = strings.Join(lines[start:i+1], "\n")
		imp.StartLine = start + 1
		imp.EndLine = i + 1
		imports = append(imports, imp)
	}
	return imports
}

// parsePHPUse reads one complete use statement. The name the code uses for
// an imported name is its alias, else its last segment.
func parsePHPUse(statement string) (phpUse, bool) {
	m := phpUseRe.FindStringSubmatch(statement)
	if m == nil {
		return phpUse{}, false
	}
	var imp phpUse
	imp.Module = m[2]
	body := m[2]
	if open := strings.Index(body, "{"); open >= 0 {
		end := strings.LastIndex(body, "}")
		if end < open {
			return phpUse{}, false
		}
		imp.Module = strings.TrimSpace(body[:open])
		body = body[open+1 : end]
	}
	for _, part := range strings.Split(body, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue // a trailing comma in a group
		}
		nm := phpUseNameRe.FindStringSubmatch(part)
		if nm == nil {
			return phpUse{}, false
		}
		name := nm[3]
		if name == "" {
			segments := strings.Split(nm[2], "\\")
			name = segments[len(segments)-1]
		}
		imp.ImportedNames = append(imp.ImportedNames, nm[2])
		imp.names = append(imp.names, phpUsedName{
			name:          name,
			caseSensitive: strings.EqualFold(m[1], "const") || strings.EqualFold(nm[1], "const"),
		})
	}
	return imp, len(imp.names) > 0
}

func init() {
	Register("php", NewPHPFilter())
}
