package importfilter

import (
	"regexp"
	"strings"
)

// CppFilter handles import filtering for C++ code
type CppFilter struct {
	BaseFilter
}

// NewCppFilter creates a new C++ import filter
func NewCppFilter() ImportFilter {
	return &CppFilter{
		BaseFilter: NewBaseFilter("cpp"),
	}
}

// FilterUnusedImports removes unused imports from C++ code
func (f *CppFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting C++ import filtering")
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
		// For C++ system headers, be conservative
		if strings.HasPrefix(imp.Module, "<") && strings.HasSuffix(imp.Module, ">") {
			// System headers like <iostream>, <vector> etc.
			systemHeader := strings.Trim(imp.Module, "<>")

			// Common usage patterns for system headers
			used := false
			switch systemHeader {
			case "iostream":
				// Check for cout, cin, cerr, endl
				if f.SearchForUsage(code, "std::cout", lastImportLine) ||
					f.SearchForUsage(code, "std::cin", lastImportLine) ||
					f.SearchForUsage(code, "std::cerr", lastImportLine) ||
					f.SearchForUsage(code, "std::endl", lastImportLine) {
					used = true
				}
			case "string":
				if f.SearchForUsage(code, "std::string", lastImportLine) {
					used = true
				}
			case "vector":
				if f.SearchForUsage(code, "std::vector", lastImportLine) {
					used = true
				}
			case "map":
				if f.SearchForUsage(code, "std::map", lastImportLine) {
					used = true
				}
			case "algorithm":
				// Common algorithm functions
				if f.SearchForUsage(code, "std::sort", lastImportLine) ||
					f.SearchForUsage(code, "std::find", lastImportLine) ||
					f.SearchForUsage(code, "std::copy", lastImportLine) {
					used = true
				}
			default:
				// For other system headers, check if std:: is used at all
				if f.SearchForUsage(code, "std::", lastImportLine) {
					used = true // Conservative approach
				}
			}

			if !used {
				f.LogDebug(debugLevel, 2, "Removing unused include: %s", imp.FullLine)
				removedImports = append(removedImports, imp.FullLine)
				linesToRemove = append(linesToRemove, struct{ start, end int }{imp.StartLine, imp.EndLine})
			}
		} else if strings.HasPrefix(imp.Module, "\"") && strings.HasSuffix(imp.Module, "\"") {
			// User headers - be very conservative, usually keep them
			headerName := strings.Trim(imp.Module, "\"")
			// Extract base name without extension
			baseName := headerName
			if idx := strings.LastIndex(baseName, "/"); idx >= 0 {
				baseName = baseName[idx+1:]
			}
			if idx := strings.LastIndex(baseName, "."); idx >= 0 {
				baseName = baseName[:idx]
			}

			// Check if any identifier from the header might be used
			// This is very basic and conservative
			if !f.SearchForUsage(code, baseName, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Keeping user header (conservative): %s", imp.FullLine)
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

// parseImports extracts import statements from C++ code
func (f *CppFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for C++ includes
	includeRe := regexp.MustCompile(`^\s*#\s*include\s+([<"][^>"]+[>"])\s*$`)

	// Check if this is formatted output with file tags
	inFileBlock := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "<file path=") {
			inFileBlock = true
			break
		}
	}

	f.LogDebug(debugLevel, 2, "InFileBlock: %v, Total lines: %d", inFileBlock, len(lines))

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
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			i++
			continue
		}

		// Stop parsing includes when we hit non-include code
		if !strings.Contains(trimmed, "#include") && !strings.HasPrefix(trimmed, "#") {
			// Check if we've hit actual code
			if strings.Contains(trimmed, "namespace") || strings.Contains(trimmed, "class") ||
				strings.Contains(trimmed, "struct") || strings.Contains(trimmed, "void") ||
				strings.Contains(trimmed, "int") || strings.Contains(trimmed, "using") {
				if !inFileBlock {
					break
				}
			}
		}

		// Try to match include
		if matches := includeRe.FindStringSubmatch(line); matches != nil {
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

		i++
	}

	return imports, nil
}

func init() {
	Register("cpp", NewCppFilter())
	Register("c++", NewCppFilter()) // Alias
}
