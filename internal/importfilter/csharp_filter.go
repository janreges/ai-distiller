package importfilter

import (
	"regexp"
	"strings"
)

// CSharpFilter handles import filtering for C# code
type CSharpFilter struct {
	BaseFilter
}

// NewCSharpFilter creates a new C# import filter
func NewCSharpFilter() ImportFilter {
	return &CSharpFilter{
		BaseFilter: NewBaseFilter("csharp"),
	}
}

// FilterUnusedImports removes unused imports from C# code
func (f *CSharpFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting C# import filtering")
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
		// Extract namespace or type name to search for
		searchName := f.extractSearchName(imp.Module)

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
		} else if searchName != "" {
			// Check if the namespace/type is used
			if f.SearchForUsage(code, searchName, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Found usage of '%s'", searchName)
				used = true
			}

			// Also check for common patterns in C#
			// For System.* namespaces, check common types
			if strings.HasPrefix(imp.Module, "System.") {
				if f.checkSystemNamespaceUsage(code, imp.Module, lastImportLine) {
					used = true
				}
			}
		}

		if !used {
			f.LogDebug(debugLevel, 2, "Removing unused using: %s", imp.FullLine)
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

// parseImports extracts import statements from C# code
func (f *CSharpFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Regex patterns for C# using statements
	usingRe := regexp.MustCompile(`^\s*using\s+([\w.]+)(?:\s*=\s*(\w+))?\s*;?\s*$`)
	usingStaticRe := regexp.MustCompile(`^\s*using\s+static\s+([\w.]+)\s*;?\s*$`)

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
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			i++
			continue
		}

		// Skip namespace declaration
		if strings.HasPrefix(trimmed, "namespace") {
			i++
			continue
		}

		// Stop parsing imports when we hit non-import code
		if !strings.HasPrefix(trimmed, "using") {
			// Check if this might be after imports
			if strings.Contains(trimmed, "class") || strings.Contains(trimmed, "interface") ||
				strings.Contains(trimmed, "struct") || strings.Contains(trimmed, "enum") ||
				strings.Contains(trimmed, "public") || strings.Contains(trimmed, "private") ||
				strings.Contains(trimmed, "internal") || strings.Contains(trimmed, "[") {
				// We've likely hit actual code, stop parsing imports
				if !inFileBlock {
					break
				}
			}
		}

		// Try to match using static
		if matches := usingStaticRe.FindStringSubmatch(line); matches != nil {
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

		// Try to match regular using
		if matches := usingRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[1],
				Aliases:   make(map[string]string),
			}

			// Check if there's an alias
			if matches[2] != "" {
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

// extractSearchName extracts the name to search for from a namespace/type
func (f *CSharpFilter) extractSearchName(module string) string {
	// For C#, we typically use the last part of the namespace
	// But for some common ones, we might use the full namespace
	parts := strings.Split(module, ".")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return module
}

// checkSystemNamespaceUsage checks for common usage patterns of System namespaces
func (f *CSharpFilter) checkSystemNamespaceUsage(code string, namespace string, afterLine int) bool {
	switch namespace {
	case "System":
		// Common System types
		return f.SearchForUsage(code, "Console", afterLine) ||
			f.SearchForUsage(code, "DateTime", afterLine) ||
			f.SearchForUsage(code, "String", afterLine) ||
			f.SearchForUsage(code, "Exception", afterLine) ||
			f.SearchForUsage(code, "Math", afterLine)
	case "System.Collections.Generic":
		return f.SearchForUsage(code, "List<", afterLine) ||
			f.SearchForUsage(code, "Dictionary<", afterLine) ||
			f.SearchForUsage(code, "HashSet<", afterLine) ||
			f.SearchForUsage(code, "Queue<", afterLine)
	case "System.Linq":
		return f.SearchForUsage(code, ".Where(", afterLine) ||
			f.SearchForUsage(code, ".Select(", afterLine) ||
			f.SearchForUsage(code, ".OrderBy(", afterLine) ||
			f.SearchForUsage(code, ".ToList(", afterLine)
	case "System.IO":
		return f.SearchForUsage(code, "File", afterLine) ||
			f.SearchForUsage(code, "Directory", afterLine) ||
			f.SearchForUsage(code, "Path", afterLine) ||
			f.SearchForUsage(code, "Stream", afterLine)
	case "System.Threading.Tasks":
		return f.SearchForUsage(code, "Task", afterLine) ||
			f.SearchForUsage(code, "async", afterLine) ||
			f.SearchForUsage(code, "await", afterLine)
	}

	return false
}

func init() {
	Register("csharp", NewCSharpFilter())
	Register("c#", NewCSharpFilter()) // Alias
}
