package semantic

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// TypeScriptStrategy implements LanguageStrategy for TypeScript/JavaScript
type TypeScriptStrategy struct{}

// NewTypeScriptStrategy creates a new TypeScript language strategy
func NewTypeScriptStrategy() *TypeScriptStrategy {
	return &TypeScriptStrategy{}
}

// ResolveImport resolves TypeScript/JavaScript import paths to file paths
func (ts *TypeScriptStrategy) ResolveImport(importPath string, currentFilePath string, projectRoot string) (string, error) {
	// Handle different types of TypeScript/JavaScript imports

	// Remove quotes from import path
	cleanImportPath := strings.Trim(importPath, "\"'")

	// Skip external modules (no relative path indicators)
	if !strings.HasPrefix(cleanImportPath, ".") && !strings.HasPrefix(cleanImportPath, "/") {
		// This is likely an external package (e.g., "react", "lodash")
		return "", fmt.Errorf("external module '%s' not resolved", cleanImportPath)
	}

	currentDir := filepath.Dir(currentFilePath)
	var candidatePaths []string

	// Resolve relative imports
	if strings.HasPrefix(cleanImportPath, ".") {
		// Relative to current file's directory
		basePath := filepath.Join(currentDir, cleanImportPath)

		// Try different extensions
		extensions := []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"}
		for _, ext := range extensions {
			candidatePaths = append(candidatePaths, basePath+ext)
		}

		// Try index files in directory
		indexExtensions := []string{"index.ts", "index.tsx", "index.js", "index.jsx", "index.d.ts"}
		for _, indexFile := range indexExtensions {
			candidatePaths = append(candidatePaths, filepath.Join(basePath, indexFile))
		}
	} else if strings.HasPrefix(cleanImportPath, "/") {
		// Absolute path from project root
		if projectRoot != "" {
			basePath := filepath.Join(projectRoot, strings.TrimPrefix(cleanImportPath, "/"))
			extensions := []string{".ts", ".tsx", ".js", ".jsx", ".d.ts"}
			for _, ext := range extensions {
				candidatePaths = append(candidatePaths, basePath+ext)
			}
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
	return "", fmt.Errorf("module '%s' not found, tried paths: %v", cleanImportPath, candidatePaths)
}

// ResolveMemberAccess handles TypeScript member access like obj.method or Class.static_method
func (ts *TypeScriptStrategy) ResolveMemberAccess(containerSymbol *Symbol, memberName string, symbolTable *SymbolTable) (SymbolID, error) {
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

	// If container is an interface, look for method signatures
	if containerSymbol.Kind == SymbolKindInterface {
		for _, symbol := range symbolTable.Symbols {
			if symbol.Scope == containerSymbol.Name && symbol.Name == memberName {
				return symbol.ID, nil
			}
		}
	}

	// If container is a namespace/module, look for top-level symbols
	if containerSymbol.Kind == SymbolKindNamespace || containerSymbol.Kind == SymbolKindModule {
		for _, symbol := range symbolTable.Symbols {
			if symbol.Name == memberName && symbol.Scope == containerSymbol.Name {
				return symbol.ID, nil
			}
		}
	}

	return "", fmt.Errorf("member '%s' not found in %s '%s'", memberName, containerSymbol.Kind, containerSymbol.Name)
}

// DetermineScope determines the scope of a given AST node in TypeScript
func (ts *TypeScriptStrategy) DetermineScope(node *sitter.Node, fileAST *sitter.Tree, content []byte) string {
	current := node.Parent()

	for current != nil {
		switch current.Type() {
		case "function_declaration", "method_definition":
			if nameNode := ts.findNameNode(current); nameNode != nil {
				funcName := string(content[nameNode.StartByte():nameNode.EndByte()])

				// Check if this function is inside a class
				classScope := ts.findContainingClass(current, content)
				if classScope != "" {
					return classScope + "::" + funcName
				}
				return funcName
			}
		case "class_declaration":
			if nameNode := ts.findNameNode(current); nameNode != nil {
				className := string(content[nameNode.StartByte():nameNode.EndByte()])
				return className
			}
		case "interface_declaration":
			if nameNode := ts.findNameNode(current); nameNode != nil {
				interfaceName := string(content[nameNode.StartByte():nameNode.EndByte()])
				return interfaceName
			}
		case "module_declaration":
			if nameNode := ts.findNameNode(current); nameNode != nil {
				namespaceName := string(content[nameNode.StartByte():nameNode.EndByte()])
				return namespaceName
			}
		}
		current = current.Parent()
	}

	return "" // Module-level scope
}

// InferType attempts to infer the type of a symbol usage in TypeScript
func (ts *TypeScriptStrategy) InferType(symbolName string, context *ResolutionContext) (string, error) {
	// TypeScript has explicit type annotations, so we can often extract them directly

	// Look for the symbol in local scope first
	if symbol, exists := context.FileSymbols.GetSymbol(symbolName); exists {
		switch symbol.Kind {
		case SymbolKindClass:
			return symbol.Name, nil
		case SymbolKindInterface:
			return symbol.Name, nil
		case SymbolKindFunction:
			// Check if there's return type information in metadata
			if symbol.Metadata.ReturnType != "" {
				return symbol.Metadata.ReturnType, nil
			}
			return "Function", nil
		case SymbolKindVariable:
			// Try to extract type from TypeScript type annotations
			return ts.inferVariableType(symbol, context)
		case SymbolKindType:
			return symbol.Name, nil
		}
	}

	// Check imported symbols
	for moduleName, symbolTable := range context.ImportedSymbols {
		if symbol, exists := symbolTable.GetSymbol(symbolName); exists {
			if symbol.Kind == SymbolKindClass || symbol.Kind == SymbolKindInterface {
				return moduleName + "." + symbol.Name, nil
			}
		}
	}

	return "", fmt.Errorf("unable to infer type for symbol '%s'", symbolName)
}

// inferVariableType attempts to infer the type of a variable from TypeScript annotations
func (ts *TypeScriptStrategy) inferVariableType(symbol *Symbol, context *ResolutionContext) (string, error) {
	// In TypeScript, variables often have explicit type annotations
	// This would require analyzing the AST at the declaration location
	// For now, return a generic type
	return "any", nil
}

// findContainingClass finds the name of the class containing a node
func (ts *TypeScriptStrategy) findContainingClass(node *sitter.Node, content []byte) string {
	current := node.Parent()
	for current != nil {
		if current.Type() == "class_declaration" {
			if nameNode := ts.findNameNode(current); nameNode != nil {
				return string(content[nameNode.StartByte():nameNode.EndByte()])
			}
		}
		current = current.Parent()
	}
	return ""
}

// findNameNode finds the name node for different TypeScript constructs
func (ts *TypeScriptStrategy) findNameNode(node *sitter.Node) *sitter.Node {
	// Try different field names used in TypeScript tree-sitter grammar
	nameFields := []string{"name", "property", "key"}

	for _, field := range nameFields {
		if nameNode := node.ChildByFieldName(field); nameNode != nil {
			return nameNode
		}
	}

	// Fallback: look for identifier or type_identifier children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "type_identifier" || child.Type() == "property_identifier" {
			return child
		}
	}

	return nil
}

// GetBuiltinTypes returns a map of TypeScript/JavaScript builtin types and functions
func (ts *TypeScriptStrategy) GetBuiltinTypes() map[string]string {
	return map[string]string{
		// JavaScript built-ins
		"console":       "object",
		"Array":         "constructor",
		"Object":        "constructor",
		"String":        "constructor",
		"Number":        "constructor",
		"Boolean":       "constructor",
		"Date":          "constructor",
		"RegExp":        "constructor",
		"Error":         "constructor",
		"Promise":       "constructor",
		"JSON":          "object",
		"Math":          "object",
		"parseInt":      "function",
		"parseFloat":    "function",
		"isNaN":         "function",
		"isFinite":      "function",
		"setTimeout":    "function",
		"setInterval":   "function",
		"clearTimeout":  "function",
		"clearInterval": "function",

		// TypeScript specific
		"any":       "type",
		"unknown":   "type",
		"never":     "type",
		"void":      "type",
		"undefined": "value",
		"null":      "value",
	}
}

// IsBuiltinSymbol checks if a symbol is a TypeScript/JavaScript builtin
func (ts *TypeScriptStrategy) IsBuiltinSymbol(symbolName string) bool {
	builtins := ts.GetBuiltinTypes()
	_, exists := builtins[symbolName]
	return exists
}

// GetBuiltinSymbolID returns a symbol ID for a TypeScript/JavaScript builtin
func (ts *TypeScriptStrategy) GetBuiltinSymbolID(symbolName string) SymbolID {
	if ts.IsBuiltinSymbol(symbolName) {
		return SymbolID("<builtin>::" + symbolName)
	}
	return ""
}
