package importfilter

import (
	"regexp"
	"strings"
)

// RubyFilter handles import filtering for Ruby code
type RubyFilter struct {
	BaseFilter
}

// NewRubyFilter creates a new Ruby import filter
func NewRubyFilter() ImportFilter {
	return &RubyFilter{
		BaseFilter: NewBaseFilter("ruby"),
	}
}

// FilterUnusedImports removes unused imports from Ruby code
func (f *RubyFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting Ruby import filtering")
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
		// For Ruby, we need to be careful with require/require_relative
		// as they have side effects and might be needed even if not directly used

		// Always keep require statements that might have side effects
		if strings.Contains(imp.Module, "/") || strings.Contains(imp.Module, "-") {
			// Likely a gem or file path, keep it
			f.LogDebug(debugLevel, 3, "Keeping require with potential side effects: %s", imp.FullLine)
			continue
		}

		// For simple module names, check if they're used
		searchName := f.extractModuleName(imp.Module)

		if searchName != "" && f.SearchForUsage(code, searchName, lastImportLine) {
			f.LogDebug(debugLevel, 3, "Found usage of module '%s'", searchName)
		} else {
			// Be conservative with Ruby - only remove if we're very sure it's unused
			// Check for common Ruby patterns
			if f.isLikelyUnused(imp.Module, code, lastImportLine) {
				f.LogDebug(debugLevel, 2, "Removing likely unused require: %s", imp.FullLine)
				removedImports = append(removedImports, imp.FullLine)
				linesToRemove = append(linesToRemove, struct{ start, end int }{imp.StartLine, imp.EndLine})
			} else {
				f.LogDebug(debugLevel, 3, "Keeping require (might have side effects): %s", imp.FullLine)
			}
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

// parseImports extracts import statements from Ruby code
func (f *RubyFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for Ruby require statements
	requireRe := regexp.MustCompile(`^\s*require\s+['"]([^'"]+)['"]`)
	requireRelativeRe := regexp.MustCompile(`^\s*require_relative\s+['"]([^'"]+)['"]`)
	loadRe := regexp.MustCompile(`^\s*load\s+['"]([^'"]+)['"]`)
	autoloadRe := regexp.MustCompile(`^\s*autoload\s+:(\w+),\s*['"]([^'"]+)['"]`)

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

		// Skip shebang
		if strings.HasPrefix(trimmed, "#!/") {
			i++
			continue
		}

		// Stop parsing imports when we hit class/module definitions
		if strings.HasPrefix(trimmed, "class ") || strings.HasPrefix(trimmed, "module ") ||
			strings.HasPrefix(trimmed, "def ") {
			// We've likely hit actual code, stop parsing imports
			if !inFileBlock {
				break
			}
		}

		// Try to match require
		if matches := requireRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match require_relative
		if matches := requireRelativeRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match load
		if matches := loadRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:     line,
				StartLine:    i + 1,
				EndLine:      i + 1,
				Module:       matches[1],
				IsSideEffect: true, // load always has side effects
				Aliases:      make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match autoload
		if matches := autoloadRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:      line,
				StartLine:     i + 1,
				EndLine:       i + 1,
				Module:        matches[2],
				ImportedNames: []string{matches[1]},
				Aliases:       make(map[string]string),
			}
			imports = append(imports, imp)
			i++
			continue
		}

		i++
	}

	return imports, nil
}

// extractModuleName extracts a module name from a require statement
func (f *RubyFilter) extractModuleName(requirePath string) string {
	// Remove file extension
	requirePath = strings.TrimSuffix(requirePath, ".rb")

	// Handle common Ruby conventions
	// Convert snake_case to CamelCase for module names
	parts := strings.Split(requirePath, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Convert snake_case to CamelCase
		words := strings.Split(lastPart, "_")
		var camelCase string
		for _, word := range words {
			if len(word) > 0 {
				camelCase += strings.ToUpper(word[:1]) + word[1:]
			}
		}
		return camelCase
	}

	return requirePath
}

// isLikelyUnused checks if a require is likely unused based on Ruby conventions
func (f *RubyFilter) isLikelyUnused(module string, code string, afterLine int) bool {
	// Very conservative approach for Ruby
	// Only consider it unused if it's a simple module name that we can track

	// If it contains path separators, it might be loading files with side effects
	if strings.Contains(module, "/") {
		return false
	}

	// If it's a gem name (contains hyphen), be conservative
	if strings.Contains(module, "-") {
		return false
	}

	// Check for the module name in various forms
	moduleName := f.extractModuleName(module)
	if moduleName == "" {
		return false
	}

	// Check if module is referenced
	if f.SearchForUsage(code, moduleName, afterLine) {
		return false
	}

	// Check for common Ruby patterns where the module might be used indirectly
	lowerModule := strings.ToLower(moduleName)
	if f.SearchForUsage(code, lowerModule, afterLine) {
		return false
	}

	// If we get here, it's likely unused
	return true
}

func init() {
	Register("ruby", NewRubyFilter())
}
