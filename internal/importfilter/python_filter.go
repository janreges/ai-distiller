package importfilter

import (
	"regexp"
	"strings"
)

// PythonFilter handles import filtering for Python code
type PythonFilter struct {
	BaseFilter
}

// NewPythonFilter creates a new Python import filter
func NewPythonFilter() ImportFilter {
	return &PythonFilter{
		BaseFilter: NewBaseFilter("python"),
	}
}

// FilterUnusedImports removes unused imports from Python code
func (f *PythonFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting Python import filtering")
	f.LogDebug(debugLevel, 2, "Code length: %d bytes", len(code))

	// Parse imports
	imports, err := f.parseImports(code, debugLevel)
	if err != nil {
		return code, nil, err
	}

	if len(imports) == 0 {
		f.LogDebug(debugLevel, 1, "No imports found")
		return code, nil, nil
	}

	f.LogDebug(debugLevel, 1, "Found %d import statements", len(imports))

	// Find the last import line to know where to start searching for usage
	lastImportLine := 0
	for _, imp := range imports {
		if imp.EndLine > lastImportLine {
			lastImportLine = imp.EndLine
		}
	}

	// Check usage and collect unused imports
	var removedImports []string
	var linesToRemove []struct{ start, end int }

	for _, imp := range imports {
		// Always keep wildcard and side-effect imports
		if imp.IsWildcard {
			f.LogDebug(debugLevel, 3, "Keeping wildcard import: %s", imp.FullLine)
			continue
		}

		// Check if any imported name is used
		used := false
		for name, alias := range imp.Aliases {
			checkName := alias
			if alias == "" {
				checkName = name
			}

			if f.SearchForUsage(code, checkName, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Found usage of '%s'", checkName)
				used = true
				break
			}
		}

		// For module imports (import module), check module usage
		if len(imp.ImportedNames) == 0 && imp.Module != "" {
			if f.SearchForUsage(code, imp.Module, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Found usage of module '%s'", imp.Module)
				used = true
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

	f.LogDebug(debugLevel, 3, "Removed %d unused imports", len(removedImports))

	return result, removedImports, nil
}

// parseImports extracts import statements from Python code
func (f *PythonFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for Python imports
	simpleImportRe := regexp.MustCompile(`^\s*import\s+([\w.]+)(?:\s+as\s+(\w+))?\s*$`)
	fromImportRe := regexp.MustCompile(`^\s*from\s+([\w.]+)\s+import\s+(.+)\s*$`)

	// Check if this is formatted output with file tags
	inFileBlock := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "<file path=") {
			inFileBlock = true
			break
		}
	}

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip file tags
		if strings.HasPrefix(trimmed, "<file") || strings.HasPrefix(trimmed, "</file>") {
			i++
			continue
		}

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		// Stop parsing imports when we hit non-import code
		if !strings.HasPrefix(trimmed, "import") && !strings.HasPrefix(trimmed, "from") {
			// Check if this might be inside a docstring or after imports
			if !f.IsCommentLine(line) && trimmed != "" && !strings.HasPrefix(trimmed, "class") && !strings.HasPrefix(trimmed, "def") {
				// In formatted output, continue looking for imports
				if !inFileBlock {
					break
				}
			}
		}

		// Try to match simple import
		if matches := simpleImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			if matches[2] != "" {
				// import module as alias
				imp.Aliases[matches[1]] = matches[2]
			}

			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match from import
		if matches := fromImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			// Check for multiline import
			importPart := matches[2]
			if strings.Contains(importPart, "(") && !strings.Contains(importPart, ")") {
				// Multiline import
				fullImport := line
				startLine := i + 1
				i++

				for i < len(lines) && !strings.Contains(lines[i], ")") {
					fullImport += "\n" + lines[i]
					importPart += " " + strings.TrimSpace(lines[i])
					i++
				}

				if i < len(lines) {
					fullImport += "\n" + lines[i]
					importPart += " " + strings.TrimSpace(lines[i])
				}

				imp.FullLine = fullImport
				imp.EndLine = i + 1
				imp.StartLine = startLine
			} else {
				imp.EndLine = i + 1
			}

			// Parse imported names
			if strings.TrimSpace(importPart) == "*" {
				imp.IsWildcard = true
			} else {
				// Remove parentheses if present
				importPart = strings.Trim(importPart, "()")

				// Split by comma and parse each import
				parts := strings.Split(importPart, ",")
				for _, part := range parts {
					part = strings.TrimSpace(part)
					if part == "" {
						continue
					}

					// Check for alias
					if strings.Contains(part, " as ") {
						asparts := strings.Split(part, " as ")
						if len(asparts) == 2 {
							name := strings.TrimSpace(asparts[0])
							alias := strings.TrimSpace(asparts[1])
							imp.ImportedNames = append(imp.ImportedNames, name)
							imp.Aliases[name] = alias
						}
					} else {
						imp.ImportedNames = append(imp.ImportedNames, part)
						imp.Aliases[part] = "" // No alias
					}
				}
			}

			imports = append(imports, imp)
			i++
			continue
		}

		i++
	}

	return imports, nil
}

func init() {
	Register("python", NewPythonFilter())
}
