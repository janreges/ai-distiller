package importfilter

import (
	"regexp"
	"strings"
)

// GoFilter handles import filtering for Go code
type GoFilter struct {
	BaseFilter
}

// NewGoFilter creates a new Go import filter
func NewGoFilter() ImportFilter {
	return &GoFilter{
		BaseFilter: NewBaseFilter("go"),
	}
}

// FilterUnusedImports removes unused imports from Go code
func (f *GoFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 3, "Starting Go import filtering")

	// Parse imports
	imports, err := f.parseImports(code, debugLevel)
	if err != nil {
		return code, nil, err
	}

	if len(imports) == 0 {
		f.LogDebug(debugLevel, 3, "No imports found")
		return code, nil, nil
	}

	f.LogDebug(debugLevel, 3, "Found %d import statements", len(imports))

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
		// Always keep blank imports (side-effect imports)
		if imp.IsSideEffect {
			f.LogDebug(debugLevel, 3, "Keeping side-effect import: %s", imp.FullLine)
			continue
		}

		// Always keep "C" import (cgo)
		if imp.Module == "C" {
			f.LogDebug(debugLevel, 3, "Keeping C import for cgo")
			continue
		}

		// Check if the package is used
		used := false

		// Determine the package name to search for
		searchName := ""
		if len(imp.Aliases) > 0 {
			// Use alias if available
			for _, alias := range imp.Aliases {
				searchName = alias
				break
			}
		} else {
			// Extract package name from import path
			searchName = f.extractPackageName(imp.Module)
		}

		if searchName != "" && f.SearchForUsage(code, searchName, lastImportLine) {
			f.LogDebug(debugLevel, 3, "Found usage of package '%s'", searchName)
			used = true
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

// parseImports extracts import statements from Go code
func (f *GoFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for Go imports
	// Note: These patterns now handle inline comments
	singleImportRe := regexp.MustCompile(`^\s*import\s+(?:(\w+)\s+)?"([^"]+)"`)
	singleImportWithAlias := regexp.MustCompile(`^\s*import\s+(\w+)\s+"([^"]+)"`)
	blankImportRe := regexp.MustCompile(`^\s*import\s+_\s+"([^"]+)"`)
	importStartRe := regexp.MustCompile(`^\s*import\s*\(\s*$`)
	importLineRe := regexp.MustCompile(`^\s*(?:(\w+)\s+)?"([^"]+)"`)
	blankImportLineRe := regexp.MustCompile(`^\s*_\s+"([^"]+)"`)

	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			i++
			continue
		}

		// Stop parsing imports when we hit non-import code
		// But don't break on package declaration or type/const/var declarations before imports
		if !strings.HasPrefix(trimmed, "import") && !strings.Contains(line, `"`) {
			// Skip package declaration
			if strings.HasPrefix(trimmed, "package ") {
				i++
				continue
			}
			// Check if we're inside an import block
			if !f.isInsideImportBlock(lines, i) {
				// Only break if we see actual code (func, type, const, var declarations)
				if strings.HasPrefix(trimmed, "func ") || strings.HasPrefix(trimmed, "type ") ||
					strings.HasPrefix(trimmed, "const ") || strings.HasPrefix(trimmed, "var ") {
					break
				}
			}
		}

		// Check for blank import (side-effect)
		if matches := blankImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:     line,
				StartLine:    i + 1,
				EndLine:      i + 1,
				Module:       matches[1],
				IsSideEffect: true,
				Aliases:      make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		// Check for single import with alias
		if matches := singleImportWithAlias.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[2],
				Aliases:   make(map[string]string),
			}
			imp.Aliases[matches[2]] = matches[1]
			imports = append(imports, imp)
			i++
			continue
		}

		// Check for single import without alias
		if matches := singleImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[2],
				Aliases:   make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		// Check for import block start
		if importStartRe.MatchString(line) {
			// Parse import block
			i++

			for i < len(lines) && !strings.Contains(lines[i], ")") {
				blockLine := lines[i]
				blockTrimmed := strings.TrimSpace(blockLine)

				// Skip empty lines and comments
				if blockTrimmed == "" || strings.HasPrefix(blockTrimmed, "//") {
					i++
					continue
				}

				// Check for blank import line
				if matches := blankImportLineRe.FindStringSubmatch(blockLine); matches != nil {
					imp := ImportStatement{
						FullLine:     blockLine,
						StartLine:    i + 1,
						EndLine:      i + 1,
						Module:       matches[1],
						IsSideEffect: true,
						Aliases:      make(map[string]string),
					}
					imports = append(imports, imp)
					i++
					continue
				}

				// Check for regular import line
				if matches := importLineRe.FindStringSubmatch(blockLine); matches != nil {
					imp := ImportStatement{
						FullLine:  blockLine,
						StartLine: i + 1,
						EndLine:   i + 1,
						Module:    matches[2],
						Aliases:   make(map[string]string),
					}

					// Check if there's an alias
					if matches[1] != "" {
						imp.Aliases[matches[2]] = matches[1]
					}

					imports = append(imports, imp)
				}

				i++
			}

			// Skip the closing parenthesis
			if i < len(lines) {
				i++
			}
			continue
		}

		i++
	}

	return imports, nil
}

// extractPackageName extracts the package name from an import path
func (f *GoFilter) extractPackageName(importPath string) string {
	// Remove quotes if present
	importPath = strings.Trim(importPath, `"`)

	// Special case for standard library packages with different names
	specialCases := map[string]string{
		"encoding/json": "json",
		"encoding/xml":  "xml",
		"net/http":      "http",
		"net/url":       "url",
		"database/sql":  "sql",
		"html/template": "template",
		"text/template": "template",
		"math/rand":     "rand",
		"crypto/rand":   "rand",
	}

	if packageName, ok := specialCases[importPath]; ok {
		return packageName
	}

	// For regular paths, use the last component
	parts := strings.Split(importPath, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Remove version suffix if present (e.g., /v2, /v3)
		if strings.HasPrefix(lastPart, "v") && len(lastPart) > 1 {
			// Check if it's a version suffix by trying to parse as number
			versionStr := strings.TrimPrefix(lastPart, "v")
			isVersion := true
			for _, ch := range versionStr {
				if ch < '0' || ch > '9' {
					isVersion = false
					break
				}
			}
			if isVersion && len(parts) > 1 {
				return parts[len(parts)-2]
			}
		}
		return lastPart
	}

	return importPath
}

// isInsideImportBlock checks if we're currently inside an import block
func (f *GoFilter) isInsideImportBlock(lines []string, currentIdx int) bool {
	// Look backwards for "import ("
	for i := currentIdx - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "import") && strings.Contains(line, "(") {
			// Now check if we've seen the closing ")" after the import (
			for j := i + 1; j <= currentIdx; j++ {
				if strings.Contains(lines[j], ")") {
					return false
				}
			}
			return true
		}
		// If we hit actual code, we're not in an import block
		if line != "" && !strings.HasPrefix(line, "//") && !strings.HasPrefix(line, "import") {
			return false
		}
	}
	return false
}

func init() {
	Register("go", NewGoFilter())
}
