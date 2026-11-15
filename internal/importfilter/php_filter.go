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

// FilterUnusedImports removes unused imports from PHP code
func (f *PHPFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting PHP import filtering")
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
		// Check if the class/function/const is used
		used := false

		// For aliased imports, check the alias
		if len(imp.Aliases) > 0 {
			for _, alias := range imp.Aliases {
				if f.SearchForUsage(code, alias, lastImportLine) {
					f.LogDebug(debugLevel, 3, "Found usage of alias '%s'", alias)
					used = true
					break
				}
			}
		} else {
			// For non-aliased imports, extract the class/function/const name
			searchName := f.extractName(imp.Module, imp.FullLine)
			if searchName != "" && f.SearchForUsage(code, searchName, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Found usage of '%s'", searchName)
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

	f.LogDebug(debugLevel, 1, "Removed %d unused imports", len(removedImports))

	return result, removedImports, nil
}

// parseImports extracts import statements from PHP code
func (f *PHPFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for PHP use statements
	useClassRe := regexp.MustCompile(`^\s*use\s+([\w\\]+)(?:\s+as\s+(\w+))?;?\s*$`)
	useFunctionRe := regexp.MustCompile(`^\s*use\s+function\s+([\w\\]+)(?:\s+as\s+(\w+))?;?\s*$`)
	useConstRe := regexp.MustCompile(`^\s*use\s+const\s+([\w\\]+)(?:\s+as\s+(\w+))?;?\s*$`)
	// Group use statements
	useGroupStartRe := regexp.MustCompile(`^\s*use\s+([\w\\]+)\\{`)

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
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}

		// Skip namespace declaration
		if strings.HasPrefix(trimmed, "namespace") {
			i++
			continue
		}

		// Skip declare statements
		if strings.HasPrefix(trimmed, "declare") {
			i++
			continue
		}

		// Stop parsing imports when we hit non-import code
		if !strings.HasPrefix(trimmed, "use") {
			// Check if this might be after imports
			if !f.IsCommentLine(line) && trimmed != "" &&
				(strings.HasPrefix(trimmed, "class") || strings.HasPrefix(trimmed, "interface") ||
					strings.HasPrefix(trimmed, "trait") || strings.HasPrefix(trimmed, "function") ||
					strings.HasPrefix(trimmed, "abstract") || strings.HasPrefix(trimmed, "final")) {
				// We've likely hit actual code, stop parsing imports
				if !inFileBlock {
					break
				}
			}
		}

		// Try to match group use statement
		if useGroupStartRe.MatchString(line) {
			// Handle group use statements (not fully implemented for simplicity)
			// Skip to the closing brace
			for i < len(lines) && !strings.Contains(lines[i], "}") {
				i++
			}
			i++
			continue
		}

		// Try to match function use
		if matches := useFunctionRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			if matches[2] != "" {
				// Function with alias
				imp.Aliases[matches[1]] = matches[2]
			}

			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match const use
		if matches := useConstRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			if matches[2] != "" {
				// Const with alias
				imp.Aliases[matches[1]] = matches[2]
			}

			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match class use
		if matches := useClassRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			if matches[2] != "" {
				// Class with alias
				imp.Aliases[matches[1]] = matches[2]
			}

			imports = append(imports, imp)
			i++
			continue
		}

		i++
	}

	return imports, nil
}

// extractName extracts the class/function/const name from a fully qualified name
func (f *PHPFilter) extractName(fqn string, fullLine string) string {
	// Remove leading backslash if present
	fqn = strings.TrimPrefix(fqn, "\\")

	// For function/const imports, extract from the line
	if strings.Contains(fullLine, "use function") || strings.Contains(fullLine, "use const") {
		// Already have the full name, use the last component
		parts := strings.Split(fqn, "\\")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return fqn
	}

	// For class imports, use the last component
	parts := strings.Split(fqn, "\\")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return fqn
}

func init() {
	Register("php", NewPHPFilter())
}
