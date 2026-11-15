package importfilter

import (
	"regexp"
	"strings"
)

// JavaFilter handles import filtering for Java code
type JavaFilter struct {
	BaseFilter
}

// NewJavaFilter creates a new Java import filter
func NewJavaFilter() ImportFilter {
	return &JavaFilter{
		BaseFilter: NewBaseFilter("java"),
	}
}

// FilterUnusedImports removes unused imports from Java code
func (f *JavaFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting Java import filtering")
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
		// Always keep wildcard imports (generally discouraged but still used)
		if imp.IsWildcard {
			f.LogDebug(debugLevel, 3, "Keeping wildcard import: %s", imp.FullLine)
			continue
		}

		// Always keep static imports with wildcards
		if strings.Contains(imp.FullLine, "import static") && imp.IsWildcard {
			f.LogDebug(debugLevel, 3, "Keeping static wildcard import: %s", imp.FullLine)
			continue
		}

		// Extract the class name to search for
		searchName := f.extractClassName(imp.Module)

		// For static imports, also check the imported member name
		if strings.Contains(imp.FullLine, "import static") && len(imp.ImportedNames) > 0 {
			// For static imports, search for the imported method/field name
			used := false
			for _, name := range imp.ImportedNames {
				if f.SearchForUsage(code, name, lastImportLine) {
					f.LogDebug(debugLevel, 3, "Found usage of static import '%s'", name)
					used = true
					break
				}
			}
			if !used {
				f.LogDebug(debugLevel, 2, "Removing unused static import: %s", imp.FullLine)
				removedImports = append(removedImports, imp.FullLine)
				linesToRemove = append(linesToRemove, struct{ start, end int }{imp.StartLine, imp.EndLine})
			}
		} else if searchName != "" && f.SearchForUsage(code, searchName, lastImportLine) {
			f.LogDebug(debugLevel, 3, "Found usage of class '%s'", searchName)
		} else {
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

// parseImports extracts import statements from Java code
func (f *JavaFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for Java imports
	importRe := regexp.MustCompile(`^\s*import\s+(?:static\s+)?([\w.]+)(?:\.\*)?;?\s*$`)
	staticImportRe := regexp.MustCompile(`^\s*import\s+static\s+([\w.]+)\.([\w*]+);?\s*$`)

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
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") || strings.HasPrefix(trimmed, "*") {
			i++
			continue
		}

		// Skip package declaration
		if strings.HasPrefix(trimmed, "package") {
			i++
			continue
		}

		// Stop parsing imports when we hit non-import code
		if !strings.HasPrefix(trimmed, "import") {
			// Check if this might be after imports
			if !f.IsCommentLine(line) && trimmed != "" &&
				!strings.HasPrefix(trimmed, "@") && // Skip annotations
				(strings.HasPrefix(trimmed, "public") || strings.HasPrefix(trimmed, "private") ||
					strings.HasPrefix(trimmed, "protected") || strings.HasPrefix(trimmed, "class") ||
					strings.HasPrefix(trimmed, "interface") || strings.HasPrefix(trimmed, "enum") ||
					strings.HasPrefix(trimmed, "abstract")) {
				// We've likely hit actual code, stop parsing imports
				if !inFileBlock {
					break
				}
			}
		}

		// Try to match static import
		if matches := staticImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			// Check if it's a wildcard static import
			if matches[2] == "*" {
				imp.IsWildcard = true
			} else {
				imp.ImportedNames = []string{matches[2]}
			}

			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match regular import
		if matches := importRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			// Check if it's a wildcard import
			if strings.Contains(line, ".*") {
				imp.IsWildcard = true
			}

			imports = append(imports, imp)
			i++
			continue
		}

		i++
	}

	return imports, nil
}

// extractClassName extracts the class name from a fully qualified import
func (f *JavaFilter) extractClassName(importPath string) string {
	// For Java imports, use the last component
	parts := strings.Split(importPath, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return importPath
}

func init() {
	Register("java", NewJavaFilter())
}
