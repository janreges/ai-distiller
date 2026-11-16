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

		// The import is used when any name it binds occurs in the code
		names := jsLocalNames(imp)
		if len(names) == 0 {
			f.LogDebug(debugLevel, 3, "Keeping import without a recognised name: %s", imp.FullLine)
			continue
		}
		used := false
		for _, name := range names {
			if f.SearchForUsage(code, name, lastImportLine) {
				f.LogDebug(debugLevel, 3, "Found usage of '%s'", name)
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
	// CommonJS require, and TypeScript's `import x = require('m')`
	requireRe := regexp.MustCompile(`^\s*(?:const|let|var|import)\s+(.+?)\s*=\s*require\s*\(\s*['"` + "`" + `](.+?)['"` + "`" + `]\s*\)\s*;?\s*$`)
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

// jsLocalNames returns the names an import binds in the code: the default
// binding, the namespace alias, and each named import's alias or, without
// one, its name.
func jsLocalNames(imp ImportStatement) []string {
	var names []string
	if d := imp.Aliases["default"]; d != "" {
		names = append(names, d)
	}
	if ns := imp.Aliases["*"]; ns != "" {
		names = append(names, ns)
	}
	for _, name := range imp.ImportedNames {
		if alias := imp.Aliases[name]; alias != "" {
			names = append(names, alias)
		} else {
			names = append(names, name)
		}
	}
	return names
}

// jsNamespaceRe matches a namespace binding, `* as name`.
var jsNamespaceRe = regexp.MustCompile(`^\*\s+as\s+([\w$]+)$`)

// parseImportSpecifier reads the clause of an import between `import` (and
// `type`) and `from`, in every form the formatters print: `D`, `* as ns`,
// `{ A, B as C, type T }`, `D, { A }` and `D, * as ns`. The default binding
// is recorded as Aliases["default"], a namespace binding as IsWildcard and
// Aliases["*"], each named import as an ImportedNames entry with its alias
// (empty without one) in Aliases.
func (f *JavaScriptFilter) parseImportSpecifier(spec string, imp *ImportStatement) {
	for _, part := range f.splitImportParts(strings.TrimSpace(spec)) {
		part = strings.TrimSpace(part)
		switch {
		case part == "":
		case strings.HasPrefix(part, "{"):
			inner := strings.TrimSuffix(strings.TrimPrefix(part, "{"), "}")
			for _, named := range strings.Split(inner, ",") {
				named = strings.TrimSpace(named)
				named = strings.TrimSpace(strings.TrimPrefix(named, "type "))
				if named == "" {
					continue
				}
				name, alias := named, ""
				if i := strings.Index(named, " as "); i >= 0 {
					name, alias = strings.TrimSpace(named[:i]), strings.TrimSpace(named[i+len(" as "):])
				}
				imp.ImportedNames = append(imp.ImportedNames, name)
				imp.Aliases[name] = alias
			}
		case strings.HasPrefix(part, "*"):
			if m := jsNamespaceRe.FindStringSubmatch(part); m != nil {
				imp.IsWildcard = true
				imp.Aliases["*"] = m[1]
			}
		default:
			imp.Aliases["default"] = part
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
