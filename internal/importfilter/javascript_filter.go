package importfilter

import (
	"regexp"
	"strings"
)

// JavaScriptFilter handles import filtering for JavaScript and TypeScript code
type JavaScriptFilter struct {
	BaseFilter
}

// NewJavaScriptFilter creates a new JavaScript/TypeScript import filter
func NewJavaScriptFilter() ImportFilter {
	return &JavaScriptFilter{
		BaseFilter: NewBaseFilter("javascript"),
	}
}

// FilterUnusedImports removes unused imports from JavaScript/TypeScript code
func (f *JavaScriptFilter) FilterUnusedImports(code string, debugLevel int) (string, []string, error) {
	f.LogDebug(debugLevel, 1, "Starting JavaScript/TypeScript import filtering")
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
		// Always keep side-effect imports (no imported names)
		if imp.IsSideEffect {
			f.LogDebug(debugLevel, 3, "Keeping side-effect import: %s", imp.FullLine)
			continue
		}

		// Check if any imported name is used
		used := false

		// For namespace imports (import * as ns from 'module')
		if imp.IsWildcard && len(imp.Aliases) > 0 {
			for _, alias := range imp.Aliases {
				if f.SearchForUsage(code, alias, lastImportLine) {
					f.LogDebug(debugLevel, 3, "Found usage of namespace '%s'", alias)
					used = true
					break
				}
			}
		} else if len(imp.ImportedNames) > 0 {
			// Check each imported name
			for _, name := range imp.ImportedNames {
				// Check if there's an alias
				checkName := name
				if alias, ok := imp.Aliases[name]; ok && alias != "" {
					checkName = alias
				}

				if f.SearchForUsage(code, checkName, lastImportLine) {
					f.LogDebug(debugLevel, 3, "Found usage of '%s'", checkName)
					used = true
					break
				}
			}
		}

		// For default imports
		if imp.Module != "" && len(imp.ImportedNames) == 0 && len(imp.Aliases) == 1 {
			// This is likely a default import
			for _, alias := range imp.Aliases {
				if f.SearchForUsage(code, alias, lastImportLine) {
					f.LogDebug(debugLevel, 3, "Found usage of default import '%s'", alias)
					used = true
					break
				}
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

// parseImports extracts import statements from JavaScript/TypeScript code
func (f *JavaScriptFilter) parseImports(code string, debugLevel int) ([]ImportStatement, error) {
	var imports []ImportStatement
	lines := strings.Split(code, "\n")

	// Check if this is formatted output with file tags
	inFileBlock := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "<file path=") {
			inFileBlock = true
			break
		}
	}

	f.LogDebug(debugLevel, 2, "InFileBlock: %v, Total lines: %d", inFileBlock, len(lines))

	// Regex patterns for JS/TS imports
	// ES6 imports - make semicolon optional and handle quotes better
	importRe := regexp.MustCompile(`^\s*import\s+(.+?)\s+from\s+['"` + "`" + `](.+?)['"` + "`" + `]\s*;?\s*$`)
	// Side-effect imports
	sideEffectRe := regexp.MustCompile(`^\s*import\s+['"` + "`" + `](.+?)['"` + "`" + `]\s*;?\s*$`)
	// Dynamic imports (we'll skip these as they're runtime)
	dynamicImportRe := regexp.MustCompile(`import\s*\(`)
	// CommonJS require (for completeness, though less common in modern code)
	requireRe := regexp.MustCompile(`^\s*(?:const|let|var)\s+(.+?)\s*=\s*require\s*\(\s*['"` + "`" + `](.+?)['"` + "`" + `]\s*\)\s*;?\s*$`)
	// Type imports (TypeScript)
	typeImportRe := regexp.MustCompile(`^\s*import\s+type\s+(.+?)\s+from\s+['"` + "`" + `](.+?)['"` + "`" + `]\s*;?\s*$`)

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

		// Stop parsing imports when we hit non-import code
		if !strings.HasPrefix(trimmed, "import") && !strings.Contains(line, "require(") {
			// Check if this might be a multiline comment or after imports
			if !f.IsCommentLine(line) && trimmed != "" && !strings.HasPrefix(trimmed, "*") {
				// We've likely hit actual code, stop parsing imports
				if !inFileBlock {
					break
				}
			}
		}

		// Skip dynamic imports
		if dynamicImportRe.MatchString(line) {
			i++
			continue
		}

		// Try to match side-effect import first
		if matches := sideEffectRe.FindStringSubmatch(line); matches != nil {
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

		// Try to match type import (TypeScript)
		if matches := typeImportRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[2],
				Aliases:   make(map[string]string),
			}

			// Parse the import specifier
			f.parseImportSpecifier(matches[1], &imp)
			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match ES6 import
		if matches := importRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				Module:    matches[2],
				Aliases:   make(map[string]string),
			}

			// Check for multiline import
			importSpec := matches[1]
			if strings.Contains(importSpec, "{") && !strings.Contains(importSpec, "}") {
				// Multiline import
				fullImport := line
				startLine := i + 1
				i++

				for i < len(lines) && !strings.Contains(lines[i], "}") {
					fullImport += "\n" + lines[i]
					importSpec += " " + strings.TrimSpace(lines[i])
					i++
				}

				if i < len(lines) {
					fullImport += "\n" + lines[i]
					importSpec += " " + strings.TrimSpace(lines[i])
				}

				imp.FullLine = fullImport
				imp.EndLine = i + 1
				imp.StartLine = startLine
			} else {
				imp.EndLine = i + 1
			}

			// Parse the import specifier
			f.parseImportSpecifier(importSpec, &imp)
			imports = append(imports, imp)
			i++
			continue
		}

		// Try to match CommonJS require
		if matches := requireRe.FindStringSubmatch(line); matches != nil {
			imp := ImportStatement{
				FullLine:  line,
				StartLine: i + 1,
				EndLine:   i + 1,
				Module:    matches[2],
				Aliases:   make(map[string]string),
			}

			// Parse the variable declaration
			varDecl := matches[1]
			if strings.Contains(varDecl, "{") {
				// Destructured require
				f.parseDestructuredRequire(varDecl, &imp)
			} else {
				// Simple require
				varName := strings.TrimSpace(varDecl)
				imp.Aliases["default"] = varName
			}

			imports = append(imports, imp)
			i++
			continue
		}

		i++
	}

	return imports, nil
}

// parseImportSpecifier parses the import specifier part of an ES6 import
func (f *JavaScriptFilter) parseImportSpecifier(spec string, imp *ImportStatement) {
	spec = strings.TrimSpace(spec)

	// Handle namespace import: * as name
	if strings.HasPrefix(spec, "*") {
		if matches := regexp.MustCompile(`\*\s+as\s+(\w+)`).FindStringSubmatch(spec); matches != nil {
			imp.IsWildcard = true
			imp.Aliases["*"] = matches[1]
			return
		}
	}

	// Remove outer braces if present
	hadBraces := false
	if strings.HasPrefix(spec, "{") && strings.HasSuffix(spec, "}") {
		spec = strings.TrimPrefix(spec, "{")
		spec = strings.TrimSuffix(spec, "}")
		hadBraces = true
	}

	// Split by comma
	parts := f.splitImportParts(spec)

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for default import (no braces originally)
		if !hadBraces && !strings.Contains(part, " as ") && len(parts) == 1 {
			// This is a default import
			imp.Aliases["default"] = part
			continue
		}

		// Check for alias
		if strings.Contains(part, " as ") {
			asParts := strings.Split(part, " as ")
			if len(asParts) == 2 {
				name := strings.TrimSpace(asParts[0])
				alias := strings.TrimSpace(asParts[1])
				imp.ImportedNames = append(imp.ImportedNames, name)
				imp.Aliases[name] = alias
			}
		} else {
			// No alias
			imp.ImportedNames = append(imp.ImportedNames, part)
			imp.Aliases[part] = "" // No alias
		}
	}
}

// parseDestructuredRequire parses destructured CommonJS require
func (f *JavaScriptFilter) parseDestructuredRequire(varDecl string, imp *ImportStatement) {
	// Remove braces
	varDecl = strings.TrimPrefix(varDecl, "{")
	varDecl = strings.TrimSuffix(varDecl, "}")

	// Split by comma
	parts := strings.Split(varDecl, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for alias (property: alias)
		if strings.Contains(part, ":") {
			colonParts := strings.Split(part, ":")
			if len(colonParts) == 2 {
				name := strings.TrimSpace(colonParts[0])
				alias := strings.TrimSpace(colonParts[1])
				imp.ImportedNames = append(imp.ImportedNames, name)
				imp.Aliases[name] = alias
			}
		} else {
			// No alias
			imp.ImportedNames = append(imp.ImportedNames, part)
			imp.Aliases[part] = "" // No alias
		}
	}
}

// splitImportParts splits import parts by comma, handling nested braces
func (f *JavaScriptFilter) splitImportParts(spec string) []string {
	var parts []string
	var current strings.Builder
	braceLevel := 0

	for _, ch := range spec {
		if ch == '{' {
			braceLevel++
		} else if ch == '}' {
			braceLevel--
		}

		if ch == ',' && braceLevel == 0 {
			parts = append(parts, current.String())
			current.Reset()
		} else {
			current.WriteRune(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

func init() {
	Register("javascript", NewJavaScriptFilter())
	Register("typescript", NewJavaScriptFilter()) // Same filter for both
}
