package semantic

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
)

// LanguageStrategy defines the interface for language-specific resolution logic
type LanguageStrategy interface {
	// ResolveImport turns an import string from a file into a canonical file path
	ResolveImport(importPath string, currentFilePath string, projectRoot string) (string, error)

	// ResolveMemberAccess handles expressions like object.method or Class.static_method
	ResolveMemberAccess(containerSymbol *Symbol, memberName string, symbolTable *SymbolTable) (SymbolID, error)

	// DetermineScope determines the scope of a given AST node
	DetermineScope(node *sitter.Node, fileAST *sitter.Tree, content []byte) string

	// InferType attempts to infer the type of a symbol usage
	InferType(symbolName string, context *ResolutionContext) (string, error)

	// GetBuiltinTypes returns builtin types for the language
	GetBuiltinTypes() map[string]string

	// IsBuiltinSymbol checks if a symbol is a language builtin
	IsBuiltinSymbol(symbolName string) bool

	// GetBuiltinSymbolID returns symbol ID for builtins
	GetBuiltinSymbolID(symbolName string) SymbolID
}

// ResolutionContext provides context for symbol resolution
type ResolutionContext struct {
	CurrentFile     string
	CurrentSymbol   *Symbol
	LocalScope      map[string]*Symbol
	FileSymbols     *SymbolTable
	ImportedSymbols map[string]*SymbolTable // import name -> symbol table
	DependencyGraph map[string][]string
	AllSymbolTables map[string]*SymbolTable // file path -> symbol table
}

// Resolver performs Pass 2 analysis - linking and resolving symbols
type Resolver struct {
	languageStrategies map[string]LanguageStrategy
	projectRoot        string
}

// NewResolver creates a new symbol resolver
func NewResolver(projectRoot string) *Resolver {
	resolver := &Resolver{
		languageStrategies: make(map[string]LanguageStrategy),
		projectRoot:        projectRoot,
	}

	// Register language strategies
	resolver.RegisterLanguageStrategy("python", NewPythonStrategy())
	resolver.RegisterLanguageStrategy("typescript", NewTypeScriptStrategy())
	resolver.RegisterLanguageStrategy("javascript", NewTypeScriptStrategy())

	return resolver
}

// RegisterLanguageStrategy registers a language-specific strategy
func (r *Resolver) RegisterLanguageStrategy(language string, strategy LanguageStrategy) {
	r.languageStrategies[language] = strategy
}

// ResolveProject performs Pass 2 analysis on all files in a project
func (r *Resolver) ResolveProject(ctx context.Context, semanticGraph *SemanticGraph) error {
	// Phase 2a: Build the File Dependency Graph
	if err := r.buildFileDependencyGraph(semanticGraph); err != nil {
		return fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Phase 2b: Resolve symbols for each file
	if err := r.resolveSymbols(ctx, semanticGraph); err != nil {
		return fmt.Errorf("failed to resolve symbols: %w", err)
	}

	return nil
}

// buildFileDependencyGraph builds the complete file dependency graph
func (r *Resolver) buildFileDependencyGraph(semanticGraph *SemanticGraph) error {
	for filePath, symbolTable := range semanticGraph.FileSymbolTables {
		strategy, ok := r.languageStrategies[symbolTable.Language]
		if !ok {
			continue // Skip unsupported languages
		}

		// Resolve each dependency to a concrete file path
		var resolvedDeps []string
		for _, depPath := range symbolTable.Dependencies {
			resolved, err := strategy.ResolveImport(depPath, filePath, r.projectRoot)
			if err != nil {
				// Log warning but continue - some imports might be external libraries
				fmt.Printf("Warning: failed to resolve import '%s' in %s: %v\n", depPath, filePath, err)
				continue
			}
			resolvedDeps = append(resolvedDeps, resolved)
		}

		semanticGraph.DependencyGraph[filePath] = resolvedDeps
	}

	return nil
}

// resolveSymbols resolves symbol usages to their definitions
func (r *Resolver) resolveSymbols(ctx context.Context, semanticGraph *SemanticGraph) error {
	// Process each file
	for filePath, symbolTable := range semanticGraph.FileSymbolTables {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		strategy, ok := r.languageStrategies[symbolTable.Language]
		if !ok {
			continue // Skip unsupported languages
		}

		// Create resolution context for this file
		resolutionCtx := r.createResolutionContext(filePath, semanticGraph)

		// Resolve call sites for this file
		if err := r.resolveCallSites(filePath, strategy, resolutionCtx, semanticGraph); err != nil {
			return fmt.Errorf("failed to resolve call sites for %s: %w", filePath, err)
		}
	}

	return nil
}

// createResolutionContext creates a resolution context for a file
func (r *Resolver) createResolutionContext(filePath string, semanticGraph *SemanticGraph) *ResolutionContext {
	symbolTable := semanticGraph.FileSymbolTables[filePath]
	dependencies := semanticGraph.DependencyGraph[filePath]

	// Build imported symbols map
	importedSymbols := make(map[string]*SymbolTable)
	for _, depPath := range dependencies {
		if depSymbolTable, exists := semanticGraph.FileSymbolTables[depPath]; exists {
			// Use the module name as the key (last part of path without extension)
			moduleName := strings.TrimSuffix(filepath.Base(depPath), filepath.Ext(depPath))
			importedSymbols[moduleName] = depSymbolTable
		}
	}

	return &ResolutionContext{
		CurrentFile:     filePath,
		FileSymbols:     symbolTable,
		ImportedSymbols: importedSymbols,
		DependencyGraph: semanticGraph.DependencyGraph,
		AllSymbolTables: semanticGraph.FileSymbolTables,
		LocalScope:      make(map[string]*Symbol),
	}
}

// resolveCallSites resolves call sites for a specific file
func (r *Resolver) resolveCallSites(filePath string, strategy LanguageStrategy, resCtx *ResolutionContext, semanticGraph *SemanticGraph) error {
	// Find call sites that originated from this file
	var callSitesToResolve []int
	for i, callSite := range semanticGraph.CallSites {
		if callSite.Location.FilePath == filePath {
			callSitesToResolve = append(callSitesToResolve, i)
		}
	}

	// Resolve each call site
	for _, index := range callSitesToResolve {
		callSite := &semanticGraph.CallSites[index]

		resolvedCalleeID, err := r.resolveCallSite(callSite, strategy, resCtx)
		if err != nil {
			// Log warning but continue - some calls might be to external libraries
			fmt.Printf("Warning: failed to resolve call to '%s' at %s:%d: %v\n",
				callSite.CalleeName, callSite.Location.FilePath, callSite.Location.StartLine, err)
			continue
		}

		// Update the call site with resolved information
		callSite.CalleeID = resolvedCalleeID
		callSite.IsResolved = true

		// Add to call graph
		semanticGraph.AddCallSite(*callSite)
	}

	return nil
}

// resolveCallSite resolves a single call site
func (r *Resolver) resolveCallSite(callSite *CallSite, strategy LanguageStrategy, resCtx *ResolutionContext) (SymbolID, error) {
	calleeName := callSite.CalleeName

	// Try different resolution strategies in order

	// 0. Check for builtin symbols first (language-specific)
	if builtinID := r.resolveBuiltinSymbol(calleeName, strategy); builtinID != "" {
		return builtinID, nil
	}

	// 1. Local scope (function parameters, local variables)
	if symbol := r.findInLocalScope(calleeName, resCtx); symbol != nil {
		return symbol.ID, nil
	}

	// 2. File/Module scope (functions, classes defined in the same file)
	if symbol, exists := resCtx.FileSymbols.GetSymbol(calleeName); exists {
		return symbol.ID, nil
	}

	// 3. Imported symbols
	if resolvedID := r.findInImportedSymbols(calleeName, resCtx); resolvedID != "" {
		return resolvedID, nil
	}

	// 4. Handle method calls and member access
	if resolvedID := r.resolveMemberAccess(callSite, strategy, resCtx); resolvedID != "" {
		return resolvedID, nil
	}

	// 5. Try type inference for complex expressions
	if resolvedID := r.resolveWithTypeInference(callSite, strategy, resCtx); resolvedID != "" {
		return resolvedID, nil
	}

	return "", fmt.Errorf("symbol '%s' not found", calleeName)
}

// findInLocalScope searches for a symbol in the local scope
func (r *Resolver) findInLocalScope(symbolName string, resCtx *ResolutionContext) *Symbol {
	// This would need to be populated based on the current function context
	// For now, we'll return nil as local scope resolution requires more AST analysis
	return nil
}

// findInImportedSymbols searches for a symbol in imported modules
func (r *Resolver) findInImportedSymbols(symbolName string, resCtx *ResolutionContext) SymbolID {
	for moduleName, symbolTable := range resCtx.ImportedSymbols {
		if symbol, exists := symbolTable.GetSymbol(symbolName); exists {
			return symbol.ID
		}

		// Also check if the symbol name matches the module name (import alias)
		if moduleName == symbolName {
			// This represents importing the entire module
			return GenerateSymbolID(symbolTable.FilePath, "<module>", "")
		}
	}
	return ""
}

// ResolveFilePair is a simplified function for resolving two files (for testing)
func (r *Resolver) ResolveFilePair(mainAnalysis, utilsAnalysis *FileAnalysis) (*SemanticGraph, error) {
	// Create a minimal semantic graph with two files
	semanticGraph := NewSemanticGraph(r.projectRoot)
	semanticGraph.AddSymbolTable(mainAnalysis.SymbolTable)
	semanticGraph.AddSymbolTable(utilsAnalysis.SymbolTable)

	// Add dependencies and call sites
	for _, dep := range mainAnalysis.Dependencies {
		semanticGraph.AddDependency(dep)
	}
	for _, dep := range utilsAnalysis.Dependencies {
		semanticGraph.AddDependency(dep)
	}

	for _, call := range mainAnalysis.CallSites {
		semanticGraph.AddCallSite(call)
	}
	for _, call := range utilsAnalysis.CallSites {
		semanticGraph.AddCallSite(call)
	}

	// Run the resolution
	ctx := context.Background()
	if err := r.ResolveProject(ctx, semanticGraph); err != nil {
		return nil, err
	}

	return semanticGraph, nil
}

// resolveBuiltinSymbol resolves language builtin symbols
func (r *Resolver) resolveBuiltinSymbol(symbolName string, strategy LanguageStrategy) SymbolID {
	if strategy.IsBuiltinSymbol(symbolName) {
		return strategy.GetBuiltinSymbolID(symbolName)
	}
	return ""
}

// resolveMemberAccess resolves member access patterns like obj.method or Class.staticMethod
func (r *Resolver) resolveMemberAccess(callSite *CallSite, strategy LanguageStrategy, resCtx *ResolutionContext) SymbolID {
	// Handle different member access patterns
	calleeName := callSite.CalleeName

	// Check if this is a qualified name (contains dots)
	if strings.Contains(calleeName, ".") {
		parts := strings.Split(calleeName, ".")
		if len(parts) >= 2 {
			containerName := parts[0]
			memberName := strings.Join(parts[1:], ".")

			// Find the container symbol
			var containerSymbol *Symbol

			// Look in local scope first
			containerSymbol = r.findInLocalScope(containerName, resCtx)

			// Look in file scope
			if containerSymbol == nil {
				if symbol, exists := resCtx.FileSymbols.GetSymbol(containerName); exists {
					containerSymbol = symbol
				}
			}

			// Look in imported symbols
			if containerSymbol == nil {
				for _, symbolTable := range resCtx.ImportedSymbols {
					if symbol, exists := symbolTable.GetSymbol(containerName); exists {
						containerSymbol = symbol
						break
					}
				}
			}

			// If we found the container, resolve the member access
			if containerSymbol != nil {
				if memberID, err := strategy.ResolveMemberAccess(containerSymbol, memberName, resCtx.FileSymbols); err == nil {
					return memberID
				}

				// Also try in imported symbol tables
				for _, symbolTable := range resCtx.ImportedSymbols {
					if memberID, err := strategy.ResolveMemberAccess(containerSymbol, memberName, symbolTable); err == nil {
						return memberID
					}
				}
			}
		}
	}

	// Check if the caller context suggests this is a method call
	if callSite.CallerID != "" && strings.Contains(string(callSite.CallerID), "::") {
		// Extract class name from caller ID
		callerParts := strings.Split(string(callSite.CallerID), "::")
		if len(callerParts) >= 2 {
			className := callerParts[len(callerParts)-2]

			// Look for the class symbol
			if classSymbol, exists := resCtx.FileSymbols.GetSymbol(className); exists {
				if memberID, err := strategy.ResolveMemberAccess(classSymbol, calleeName, resCtx.FileSymbols); err == nil {
					return memberID
				}
			}
		}
	}

	return ""
}

// resolveWithTypeInference attempts advanced type inference resolution
func (r *Resolver) resolveWithTypeInference(callSite *CallSite, strategy LanguageStrategy, resCtx *ResolutionContext) SymbolID {
	calleeName := callSite.CalleeName

	// Try to infer the type and resolve based on type information
	if inferredType, err := strategy.InferType(calleeName, resCtx); err == nil {
		// Look for symbols matching the inferred type
		for _, symbolTable := range resCtx.AllSymbolTables {
			for _, symbol := range symbolTable.Symbols {
				if symbol.Name == calleeName && (symbol.Kind == SymbolKindFunction || symbol.Kind == SymbolKindMethod) {
					// Additional type matching could be implemented here
					if symbol.Metadata.ReturnType == inferredType || inferredType == "" {
						return symbol.ID
					}
				}
			}
		}
	}

	return ""
}
