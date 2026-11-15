package semantic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// PythonStrategy implements LanguageStrategy for Python
type PythonStrategy struct{}

// NewPythonStrategy creates a new Python language strategy
func NewPythonStrategy() *PythonStrategy {
	return &PythonStrategy{}
}

// ResolveImport resolves Python import paths to file paths
func (ps *PythonStrategy) ResolveImport(importPath string, currentFilePath string, projectRoot string) (string, error) {
	// Handle different types of Python imports

	// Remove any leading dots for relative imports
	cleanImportPath := strings.TrimLeft(importPath, ".")

	// Convert module path to file path
	// e.g., "utils.helper" -> "utils/helper.py"
	pathParts := strings.Split(cleanImportPath, ".")

	var candidatePaths []string

	// If it's a relative import (starts with .)
	if strings.HasPrefix(importPath, ".") {
		// Relative to current file's directory
		currentDir := filepath.Dir(currentFilePath)

		// Handle different levels of relative imports
		dotCount := 0
		for _, char := range importPath {
			if char == '.' {
				dotCount++
			} else {
				break
			}
		}

		// Move up directories based on dot count
		relativeDir := currentDir
		for i := 1; i < dotCount; i++ {
			relativeDir = filepath.Dir(relativeDir)
		}

		if len(pathParts) > 0 && pathParts[0] != "" {
			modulePath := filepath.Join(relativeDir, filepath.Join(pathParts...))
			candidatePaths = append(candidatePaths, modulePath+".py")
			candidatePaths = append(candidatePaths, filepath.Join(modulePath, "__init__.py"))
		} else {
			// Just "." means current package
			candidatePaths = append(candidatePaths, filepath.Join(relativeDir, "__init__.py"))
		}
	} else {
		// Absolute import - try multiple locations
		currentDir := filepath.Dir(currentFilePath)

		// 1. Relative to current file's directory
		modulePath := filepath.Join(currentDir, filepath.Join(pathParts...))
		candidatePaths = append(candidatePaths, modulePath+".py")
		candidatePaths = append(candidatePaths, filepath.Join(modulePath, "__init__.py"))

		// 2. Relative to project root
		if projectRoot != "" {
			modulePath = filepath.Join(projectRoot, filepath.Join(pathParts...))
			candidatePaths = append(candidatePaths, modulePath+".py")
			candidatePaths = append(candidatePaths, filepath.Join(modulePath, "__init__.py"))
		}

		// 3. Simple case: module name in same directory
		if len(pathParts) == 1 {
			candidatePaths = append(candidatePaths, filepath.Join(currentDir, pathParts[0]+".py"))
		}
	}

	// Check which candidate paths actually exist
	for _, candidatePath := range candidatePaths {
		if _, err := os.Stat(candidatePath); err == nil {
			// Make the path absolute
			absPath, err := filepath.Abs(candidatePath)
			if err != nil {
				return candidatePath, nil // Return relative path if abs fails
			}
			return absPath, nil
		}
	}

	// If no file found, return an error
	return "", fmt.Errorf("module '%s' not found, tried paths: %v", importPath, candidatePaths)
}

// ResolveMemberAccess handles Python member access like obj.method or Class.static_method
func (ps *PythonStrategy) ResolveMemberAccess(containerSymbol *Symbol, memberName string, symbolTable *SymbolTable) (SymbolID, error) {
	if containerSymbol == nil {
		return "", fmt.Errorf("container symbol is nil")
	}

	// If container is a class, look for methods/properties in that class
	if containerSymbol.Kind == SymbolKindClass {
		// Look for methods with the class as scope
		for _, symbol := range symbolTable.Symbols {
			if symbol.Scope == containerSymbol.Name && symbol.Name == memberName {
				return symbol.ID, nil
			}
		}
	}

	// If container is a module, look for top-level symbols
	if containerSymbol.Kind == SymbolKindModule {
		for _, symbol := range symbolTable.Symbols {
			if symbol.Name == memberName && symbol.Scope == "" {
				return symbol.ID, nil
			}
		}
	}

	return "", fmt.Errorf("member '%s' not found in %s '%s'", memberName, containerSymbol.Kind, containerSymbol.Name)
}

// DetermineScope determines the scope of a given AST node in Python
func (ps *PythonStrategy) DetermineScope(node *sitter.Node, fileAST *sitter.Tree, content []byte) string {
	current := node.Parent()

	for current != nil {
		switch current.Kind() {
		case "function_definition":
			if nameNode := current.ChildByFieldName("name"); nameNode != nil {
				funcName := string(content[nameNode.StartByte():nameNode.EndByte()])

				// Check if this function is inside a class
				classScope := ps.findContainingClass(current, content)
				if classScope != "" {
					return classScope + "::" + funcName
				}
				return funcName
			}
		case "class_definition":
			if nameNode := current.ChildByFieldName("name"); nameNode != nil {
				className := string(content[nameNode.StartByte():nameNode.EndByte()])
				return className
			}
		}
		current = current.Parent()
	}

	return "" // Module-level scope
}

// InferType attempts to infer the type of a symbol usage in Python
func (ps *PythonStrategy) InferType(symbolName string, context *ResolutionContext) (string, error) {
	// This is a simplified type inference for Python
	// A full implementation would need to track assignments and function returns

	// Look for the symbol in local scope first
	if symbol, exists := context.FileSymbols.GetSymbol(symbolName); exists {
		switch symbol.Kind {
		case SymbolKindClass:
			return symbol.Name, nil
		case SymbolKindFunction:
			// Check if there's return type information in metadata
			if symbol.Metadata.ReturnType != "" {
				return symbol.Metadata.ReturnType, nil
			}
			return "function", nil
		case SymbolKindVariable:
			// Try to infer from assignment context
			return ps.inferVariableType(symbol, context)
		}
	}

	// Check imported symbols
	for moduleName, symbolTable := range context.ImportedSymbols {
		if symbol, exists := symbolTable.GetSymbol(symbolName); exists {
			if symbol.Kind == SymbolKindClass {
				return moduleName + "." + symbol.Name, nil
			}
		}
	}

	return "", fmt.Errorf("unable to infer type for symbol '%s'", symbolName)
}

// inferVariableType attempts to infer the type of a variable from its assignment
func (ps *PythonStrategy) inferVariableType(symbol *Symbol, context *ResolutionContext) (string, error) {
	// This would require analyzing the AST at the assignment location
	// For now, return a generic type
	return "Any", nil
}

// findContainingClass finds the name of the class containing a node
func (ps *PythonStrategy) findContainingClass(node *sitter.Node, content []byte) string {
	current := node.Parent()
	for current != nil {
		if current.Kind() == "class_definition" {
			if nameNode := current.ChildByFieldName("name"); nameNode != nil {
				return string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}
		current = current.Parent()
	}
	return ""
}

// GetBuiltinTypes returns a map of Python builtin types and functions
func (ps *PythonStrategy) GetBuiltinTypes() map[string]string {
	return map[string]string{
		"int":   "int",
		"str":   "str",
		"float": "float",
		"bool":  "bool",
		"list":  "list",
		"dict":  "dict",
		"set":   "set",
		"tuple": "tuple",
		"len":   "function",
		"print": "function",
		"type":  "function",
		"range": "function",
		"open":  "function",
		"input": "function",
	}
}

// IsBuiltinSymbol checks if a symbol is a Python builtin
func (ps *PythonStrategy) IsBuiltinSymbol(symbolName string) bool {
	builtins := ps.GetBuiltinTypes()
	_, exists := builtins[symbolName]
	return exists
}

// GetBuiltinSymbolID returns a symbol ID for a Python builtin
func (ps *PythonStrategy) GetBuiltinSymbolID(symbolName string) SymbolID {
	if ps.IsBuiltinSymbol(symbolName) {
		return SymbolID("<builtin>::" + symbolName)
	}
	return ""
}
