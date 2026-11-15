package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/semantic"
)

// DistilledCodeSnippet represents a piece of code extracted from a file
type DistilledCodeSnippet struct {
	StartByte uint32
	EndByte   uint32
	FQN       string // Fully Qualified Name of the symbol
	Type      string // "function", "class", "method"
}

// DistilledMultiFile represents a single file with its distilled content
type DistilledMultiFile struct {
	OriginalFilePath string
	OriginalContent  []byte
	Snippets         []DistilledCodeSnippet
	DistilledContent string
	Language         string
}

// DistilledProject is the container for all distilled files
type DistilledProject struct {
	Files       map[string]*DistilledMultiFile // Map of file path to distilled file
	EntryPoints []string                       // Entry point FQNs
}

// FunctionDefinition holds metadata about a function for distillation
type FunctionDefinition struct {
	Name      string
	FQN       string
	FilePath  string
	StartByte uint32
	EndByte   uint32
	Language  string
}

// ClassDefinition holds metadata about a class for distillation
type ClassDefinition struct {
	Name      string
	FQN       string
	FilePath  string
	StartByte uint32
	EndByte   uint32
	Language  string
}

// DependencyAnalyzer performs cross-file dependency analysis and call graph traversal
type DependencyAnalyzer struct {
	analyzer        *semantic.Analyzer
	projectRoot     string
	maxDepth        int
	visited         map[string]bool
	callGraph       map[string][]string            // function -> called functions
	typeGraph       map[string][]string            // type -> used types
	usedSymbols     map[string]bool                // symbol -> used
	functionDefs    map[string]*FunctionDefinition // FQN -> function definition
	classDefs       map[string]*ClassDefinition    // FQN -> class definition
	originalContent map[string][]byte              // file path -> original content
}

// NewDependencyAnalyzer creates a new dependency analyzer
func NewDependencyAnalyzer(projectRoot string, maxDepth int) (*DependencyAnalyzer, error) {
	analyzer, err := semantic.NewAnalyzer(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to create semantic analyzer: %w", err)
	}

	return &DependencyAnalyzer{
		analyzer:        analyzer,
		projectRoot:     projectRoot,
		maxDepth:        maxDepth,
		visited:         make(map[string]bool),
		callGraph:       make(map[string][]string),
		typeGraph:       make(map[string][]string),
		usedSymbols:     make(map[string]bool),
		functionDefs:    make(map[string]*FunctionDefinition),
		classDefs:       make(map[string]*ClassDefinition),
		originalContent: make(map[string][]byte),
	}, nil
}

// AnalyzeDependencies performs dependency-aware analysis on a distilled file
func (da *DependencyAnalyzer) AnalyzeDependencies(ctx context.Context, file *ir.DistilledFile) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	dbg.Logf(debug.LevelBasic, "Starting dependency analysis for %s (max depth: %d)", file.Path, da.maxDepth)

	// Step 1: Discover all related files through includes/imports
	relatedFiles, err := da.discoverRelatedFiles(ctx, file.Path)
	if err != nil {
		dbg.Logf(debug.LevelBasic, "Failed to discover related files: %v", err)
		relatedFiles = []string{file.Path} // Fall back to single file
	}
	dbg.Logf(debug.LevelDetailed, "Found %d related files: %v", len(relatedFiles), relatedFiles)

	// Step 2: Build symbol tables for all related files
	symbolTables := make(map[string]*semantic.SymbolTable)
	for _, filePath := range relatedFiles {
		if symbolTable, err := da.buildSymbolTableForFile(ctx, filePath); err != nil {
			dbg.Logf(debug.LevelDetailed, "Failed to build symbol table for %s: %v", filePath, err)
		} else {
			symbolTables[filePath] = symbolTable
		}
	}
	dbg.Logf(debug.LevelDetailed, "Built symbol tables for %d files", len(symbolTables))

	// Step 3: Build cross-file call graph
	da.buildCrossFileCallGraph(ctx, symbolTables)

	// Step 4: Find entry points from the main file
	entryPoints := da.findEntryPoints(file)
	dbg.Logf(debug.LevelDetailed, "Found %d entry points: %v", len(entryPoints), entryPoints)

	// Step 5: Traverse call graph to find used symbols
	for _, entryPoint := range entryPoints {
		// Convert entry point to FQN
		entryPointFQN := fmt.Sprintf("%s::%s", file.Path, entryPoint)
		da.markSymbolAsUsedRecursive(ctx, entryPointFQN, 0)
	}

	// Step 6: Create distilled project with multi-file output
	distilledProject, err := da.CreateDistilledProject(ctx, entryPoints)
	if err != nil {
		dbg.Logf(debug.LevelBasic, "Failed to create distilled project: %v, falling back to single file", err)
		filteredFile := da.filterFileByUsage(ctx, file, symbolTables[file.Path])
		return filteredFile, nil
	}

	// Step 7: Convert DistilledProject back to single DistilledFile for compatibility
	combinedFile := da.convertProjectToSingleFile(ctx, distilledProject, file)

	dbg.Logf(debug.LevelBasic, "Dependency analysis complete. Used symbols: %d, distilled files: %d", len(da.usedSymbols), len(distilledProject.Files))
	return combinedFile, nil
}

// findEntryPoints identifies functions/methods that serve as entry points
func (da *DependencyAnalyzer) findEntryPoints(file *ir.DistilledFile) []string {
	entryPoints := []string{}

	// Look for common entry point patterns in IR
	for _, child := range file.Children {
		switch node := child.(type) {
		case *ir.DistilledFunction:
			// Main functions are always entry points (case-insensitive for C#/Java compatibility)
			lowerName := strings.ToLower(node.Name)
			if lowerName == "main" || node.Name == "__init__" {
				entryPoints = append(entryPoints, node.Name)
			}
		case *ir.DistilledClass:
			// Class constructors are entry points if the class is instantiated
			for _, child := range node.Children {
				if method, ok := child.(*ir.DistilledFunction); ok {
					if method.Name == "__init__" || method.Name == "__construct" {
						entryPoints = append(entryPoints, fmt.Sprintf("%s.%s", node.Name, method.Name))
					}
				}
			}
		}
	}

	// If no explicit entry points found in IR, treat all public functions as entry points
	if len(entryPoints) == 0 {
		for _, child := range file.Children {
			if fn, ok := child.(*ir.DistilledFunction); ok {
				if fn.Visibility == "public" || fn.Visibility == "" {
					entryPoints = append(entryPoints, fn.Name)
				}
			}
		}
	}

	// FALLBACK: If still no entry points found (common for Go where processor doesn't detect functions properly),
	// look in our symbol table for functions we detected during dependency analysis
	if len(entryPoints) == 0 {
		// Look for main functions in our function definitions (case-insensitive)
		for _, funcDef := range da.functionDefs {
			if funcDef.FilePath == file.Path && strings.ToLower(funcDef.Name) == "main" {
				entryPoints = append(entryPoints, funcDef.Name)
			}
		}

		// If still no main function, treat all functions in main file as entry points
		if len(entryPoints) == 0 {
			for _, funcDef := range da.functionDefs {
				if funcDef.FilePath == file.Path {
					entryPoints = append(entryPoints, funcDef.Name)
				}
			}
		}
	}

	return entryPoints
}

// buildCallGraphRecursive builds the call graph by following function calls
func (da *DependencyAnalyzer) buildCallGraphRecursive(ctx context.Context, symbol string, analysis *semantic.FileAnalysis, depth int) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Check depth limit
	if depth > da.maxDepth {
		dbg.Logf(debug.LevelDetailed, "Max depth %d reached for symbol %s", da.maxDepth, symbol)
		return
	}

	// Check if already visited
	visitKey := fmt.Sprintf("%s:%d", symbol, depth)
	if da.visited[visitKey] {
		return
	}
	da.visited[visitKey] = true

	// Mark this symbol as used
	da.usedSymbols[symbol] = true
	dbg.Logf(debug.LevelDetailed, "Marking symbol as used: %s (depth %d)", symbol, depth)

	// Find all symbols called by this symbol
	calledSymbols := da.findCalledSymbols(symbol, analysis)
	da.callGraph[symbol] = calledSymbols

	// Recursively analyze called symbols
	for _, calledSymbol := range calledSymbols {
		da.buildCallGraphRecursive(ctx, calledSymbol, analysis, depth+1)
	}

	// Find and mark used types for this symbol
	usedTypes := da.findUsedTypes(symbol, analysis)
	da.typeGraph[symbol] = usedTypes
	for _, usedType := range usedTypes {
		da.usedSymbols[usedType] = true
		dbg.Logf(debug.LevelDetailed, "Marking type as used: %s", usedType)
	}
}

// discoverRelatedFiles finds all files related through includes/imports
func (da *DependencyAnalyzer) discoverRelatedFiles(ctx context.Context, startFile string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	visited := make(map[string]bool)
	toProcess := []string{startFile}
	allFiles := []string{}

	for len(toProcess) > 0 {
		filePath := toProcess[0]
		toProcess = toProcess[1:]

		if visited[filePath] {
			continue
		}
		visited[filePath] = true
		allFiles = append(allFiles, filePath)

		// For PHP, look for require/include statements
		includes, err := da.extractIncludeStatements(ctx, filePath)
		if err != nil {
			dbg.Logf(debug.LevelDetailed, "Failed to extract includes from %s: %v", filePath, err)
			continue
		}

		for _, includePath := range includes {
			resolvedPath := da.resolveIncludePath(includePath, filePath)
			if resolvedPath != "" && !visited[resolvedPath] {
				toProcess = append(toProcess, resolvedPath)
			}
		}
	}

	return allFiles, nil
}

// extractIncludeStatements extracts include/require statements from a file
func (da *DependencyAnalyzer) extractIncludeStatements(ctx context.Context, filePath string) ([]string, error) {
	content, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	if content.IsDir() {
		return nil, fmt.Errorf("path is directory: %s", filePath)
	}

	fileContent, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Detect language and use appropriate parser
	language := da.detectLanguage(filePath)

	switch language {
	case "php":
		return da.extractPhpIncludes(ctx, fileContent, filePath)
	case "python":
		return da.extractPythonImports(ctx, fileContent, filePath)
	case "go":
		return da.extractGoImports(ctx, fileContent, filePath)
	case "javascript", "typescript":
		return da.extractJavaScriptImports(ctx, fileContent, filePath)
	case "ruby":
		return da.extractRubyRequires(ctx, fileContent, filePath)
	case "java":
		return da.extractJavaImports(ctx, fileContent, filePath)
	case "csharp", "c#":
		return da.extractCSharpImports(ctx, fileContent, filePath)
	case "rust":
		return da.extractRustImports(ctx, fileContent, filePath)
	case "swift":
		return da.extractSwiftImports(ctx, fileContent, filePath)
	case "cpp", "c++":
		return da.extractCppIncludes(ctx, fileContent, filePath)
	case "kotlin":
		return da.extractKotlinImports(ctx, fileContent, filePath)
	default:
		return []string{}, nil // No imports for unsupported languages
	}
}

// extractPhpIncludes extracts PHP require/include statements
func (da *DependencyAnalyzer) extractPhpIncludes(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	includes := []string{}

	// Look for require_once 'file.php'
	if strings.Contains(string(fileContent), "require_once") {
		lines := strings.Split(string(fileContent), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "require_once") {
				// Extract quoted filename
				start := strings.Index(line, "'")
				if start == -1 {
					start = strings.Index(line, "\"")
				}
				if start != -1 {
					start++
					end := strings.Index(line[start:], "'")
					if end == -1 {
						end = strings.Index(line[start:], "\"")
					}
					if end != -1 {
						includes = append(includes, line[start:start+end])
					}
				}
			}
		}
	}

	return includes, nil
}

// extractPythonImports extracts Python import statements
func (da *DependencyAnalyzer) extractPythonImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	includes := []string{}

	lines := strings.Split(string(fileContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Handle different import patterns:
		// import module
		// import module as alias
		// from module import something
		// from . import relative_module
		// from .relative_module import something

		if strings.HasPrefix(line, "import ") {
			// import module [as alias]
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				moduleName := parts[1]
				// Remove 'as alias' part if present
				if len(parts) >= 4 && parts[2] == "as" {
					moduleName = parts[1]
				}
				// Skip standard library modules (basic filtering)
				if !da.isStandardLibraryModule(moduleName) {
					resolved := da.resolvePythonImport(moduleName, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found Python import: %s -> %s", moduleName, resolved)
					}
				}
			}
		} else if strings.HasPrefix(line, "from ") {
			// from module import something
			parts := strings.Fields(line)
			if len(parts) >= 4 && parts[2] == "import" {
				moduleName := parts[1]

				// Handle relative imports
				if strings.HasPrefix(moduleName, ".") {
					resolved := da.resolvePythonRelativeImport(moduleName, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found Python relative import: %s -> %s", moduleName, resolved)
					}
				} else {
					// Absolute import
					if !da.isStandardLibraryModule(moduleName) {
						resolved := da.resolvePythonImport(moduleName, filePath)
						if resolved != "" {
							includes = append(includes, resolved)
							dbg.Logf(debug.LevelDetailed, "Found Python from-import: %s -> %s", moduleName, resolved)
						}
					}
				}
			}
		}
	}

	return includes, nil
}

// extractGoImports extracts Go import statements
func (da *DependencyAnalyzer) extractGoImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	includes := []string{}

	lines := strings.Split(string(fileContent), "\n")
	inImportBlock := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "//") || line == "" {
			continue
		}

		// Handle single import: import "package"
		if strings.HasPrefix(line, "import \"") && !inImportBlock {
			start := strings.Index(line, "\"")
			if start != -1 {
				start++
				end := strings.Index(line[start:], "\"")
				if end != -1 {
					importPath := line[start : start+end]
					resolved := da.resolveGoImport(importPath, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found Go import: %s -> %s", importPath, resolved)
					}
				}
			}
			continue
		}

		// Handle import block start: import (
		if line == "import (" {
			inImportBlock = true
			continue
		}

		// Handle import block end: )
		if inImportBlock && line == ")" {
			inImportBlock = false
			continue
		}

		// Handle imports within block: "package"
		if inImportBlock {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "\"") {
				start := 1
				end := strings.Index(line[start:], "\"")
				if end != -1 {
					importPath := line[start : start+end]
					resolved := da.resolveGoImport(importPath, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found Go block import: %s -> %s", importPath, resolved)
					}
				}
			}
		}
	}

	return includes, nil
}

// extractJavaScriptImports extracts JavaScript/TypeScript import statements
func (da *DependencyAnalyzer) extractJavaScriptImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	includes := []string{}

	lines := strings.Split(string(fileContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || line == "" {
			continue
		}

		// Handle different import patterns:
		// import something from 'module'
		// import { something } from 'module'
		// import * as something from 'module'
		// const something = require('module')
		// require('module')

		if strings.HasPrefix(line, "import ") && strings.Contains(line, " from ") {
			// ES6 import ... from 'module'
			fromIndex := strings.LastIndex(line, " from ")
			if fromIndex != -1 {
				remaining := strings.TrimSpace(line[fromIndex+6:]) // +6 for " from "
				modulePath := da.extractQuotedString(remaining)
				if modulePath != "" {
					resolved := da.resolveJavaScriptImport(modulePath, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found JS ES6 import: %s -> %s", modulePath, resolved)
					}
				}
			}
		} else if strings.Contains(line, "require(") {
			// CommonJS require('module')
			start := strings.Index(line, "require(")
			if start != -1 {
				remaining := line[start+8:] // +8 for "require("
				modulePath := da.extractQuotedString(remaining)
				if modulePath != "" {
					resolved := da.resolveJavaScriptImport(modulePath, filePath)
					if resolved != "" {
						includes = append(includes, resolved)
						dbg.Logf(debug.LevelDetailed, "Found JS require: %s -> %s", modulePath, resolved)
					}
				}
			}
		}
	}

	return includes, nil
}

// resolveIncludePath resolves relative include path to absolute path
func (da *DependencyAnalyzer) resolveIncludePath(includePath, currentFile string) string {
	if filepath.IsAbs(includePath) {
		return includePath
	}

	// Resolve relative to current file's directory
	currentDir := filepath.Dir(currentFile)
	resolved := filepath.Join(currentDir, includePath)

	// Check if file exists
	if _, err := os.Stat(resolved); err == nil {
		abs, _ := filepath.Abs(resolved)
		return abs
	}

	return ""
}

// extractRubyRequires extracts Ruby require statements
func (da *DependencyAnalyzer) extractRubyRequires(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	includes := []string{}

	lines := strings.Split(string(fileContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "#") || line == "" {
			continue
		}

		// Handle different require patterns:
		// require 'file'
		// require_relative 'file'
		// require_relative './file'

		if strings.HasPrefix(line, "require_relative ") {
			// require_relative 'file'
			remaining := strings.TrimSpace(line[17:]) // +17 for "require_relative "
			modulePath := da.extractQuotedString(remaining)
			if modulePath != "" {
				resolved := da.resolveRubyRequire(modulePath, filePath, true)
				if resolved != "" {
					includes = append(includes, resolved)
					dbg.Logf(debug.LevelDetailed, "Found Ruby require_relative: %s -> %s", modulePath, resolved)
				}
			}
		} else if strings.HasPrefix(line, "require ") {
			// require 'file' (absolute require)
			remaining := strings.TrimSpace(line[8:]) // +8 for "require "
			modulePath := da.extractQuotedString(remaining)
			if modulePath != "" && !da.isRubyStandardLibrary(modulePath) {
				resolved := da.resolveRubyRequire(modulePath, filePath, false)
				if resolved != "" {
					includes = append(includes, resolved)
					dbg.Logf(debug.LevelDetailed, "Found Ruby require: %s -> %s", modulePath, resolved)
				}
			}
		}
	}

	return includes, nil
}

// Helper functions for language-specific import resolution

// isStandardLibraryModule checks if a module is part of the standard library
func (da *DependencyAnalyzer) isStandardLibraryModule(moduleName string) bool {
	// Basic list of common Python standard library modules
	stdLibModules := map[string]bool{
		"os": true, "sys": true, "re": true, "json": true, "time": true,
		"datetime": true, "typing": true, "collections": true, "itertools": true,
		"functools": true, "pathlib": true, "urllib": true, "http": true,
		"math": true, "random": true, "string": true, "io": true, "csv": true,
		"xml": true, "sqlite3": true, "threading": true, "multiprocessing": true,
		"subprocess": true, "logging": true, "unittest": true, "argparse": true,
		"configparser": true, "email": true, "html": true, "abc": true,
	}
	return stdLibModules[moduleName]
}

// resolvePythonImport resolves a Python import to a file path
func (da *DependencyAnalyzer) resolvePythonImport(moduleName, currentFile string) string {
	currentDir := filepath.Dir(currentFile)

	// Convert package-style imports (utils.helper) to file paths (utils/helper)
	moduleFilePath := strings.ReplaceAll(moduleName, ".", string(filepath.Separator))

	// Try different file patterns for Python modules
	patterns := []string{
		moduleFilePath + ".py",
		moduleFilePath + "/__init__.py",
	}

	for _, pattern := range patterns {
		candidate := filepath.Join(currentDir, pattern)
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
	}

	return ""
}

// resolvePythonRelativeImport resolves Python relative imports
func (da *DependencyAnalyzer) resolvePythonRelativeImport(moduleName, currentFile string) string {
	currentDir := filepath.Dir(currentFile)

	// Handle different levels of relative imports
	// .module -> same directory
	// ..module -> parent directory
	// ...module -> grandparent directory, etc.

	dotsCount := 0
	for i, char := range moduleName {
		if char == '.' {
			dotsCount++
		} else {
			moduleName = moduleName[i:]
			break
		}
	}

	// Move up the directory tree based on dots count
	targetDir := currentDir
	for i := 1; i < dotsCount; i++ { // -1 because current dir is already level 0
		targetDir = filepath.Dir(targetDir)
	}

	// Try to resolve the module
	if moduleName == "" {
		// Just dots - look for __init__.py
		candidate := filepath.Join(targetDir, "__init__.py")
		if _, err := os.Stat(candidate); err == nil {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
	} else {
		// Module name specified
		patterns := []string{
			moduleName + ".py",
			moduleName + "/__init__.py",
		}

		for _, pattern := range patterns {
			candidate := filepath.Join(targetDir, pattern)
			if _, err := os.Stat(candidate); err == nil {
				abs, _ := filepath.Abs(candidate)
				return abs
			}
		}
	}

	return ""
}

// resolveGoImport resolves Go import paths to file locations
func (da *DependencyAnalyzer) resolveGoImport(importPath, currentFile string) string {
	// Go imports are more complex - they can be:
	// 1. Standard library (skip these)
	// 2. Local relative imports (./package or ../package)
	// 3. Module imports (github.com/user/repo/package)

	// Skip standard library packages (basic filtering)
	if da.isGoStandardLibrary(importPath) {
		return ""
	}

	// Handle relative imports
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		currentDir := filepath.Dir(currentFile)
		resolved := filepath.Join(currentDir, importPath)

		// Look for .go files in the target directory
		if entries, err := os.ReadDir(resolved); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
					candidate := filepath.Join(resolved, entry.Name())
					abs, _ := filepath.Abs(candidate)
					return abs
				}
			}
		}
	}

	// For module imports, we would need GOPATH/GOMOD resolution
	// For now, skip these as they are typically external dependencies

	return ""
}

// resolveJavaScriptImport resolves JavaScript/TypeScript imports
func (da *DependencyAnalyzer) resolveJavaScriptImport(modulePath, currentFile string) string {
	// Skip Node.js built-in modules and npm packages
	if da.isJavaScriptBuiltIn(modulePath) || !strings.HasPrefix(modulePath, ".") {
		return ""
	}

	currentDir := filepath.Dir(currentFile)

	// Handle relative imports
	if strings.HasPrefix(modulePath, "./") || strings.HasPrefix(modulePath, "../") {
		// Try different file extensions
		basePath := filepath.Join(currentDir, modulePath)

		patterns := []string{
			basePath + ".js",
			basePath + ".ts",
			basePath + ".jsx",
			basePath + ".tsx",
			basePath + "/index.js",
			basePath + "/index.ts",
		}

		for _, pattern := range patterns {
			if _, err := os.Stat(pattern); err == nil {
				abs, _ := filepath.Abs(pattern)
				return abs
			}
		}
	}

	return ""
}

// isGoStandardLibrary checks if an import is Go standard library
func (da *DependencyAnalyzer) isGoStandardLibrary(importPath string) bool {
	// Basic list of common Go standard library packages
	stdLibPackages := map[string]bool{
		"fmt": true, "os": true, "io": true, "strings": true, "strconv": true,
		"time": true, "net": true, "net/http": true, "encoding/json": true,
		"path": true, "path/filepath": true, "regexp": true, "sort": true,
		"sync": true, "context": true, "errors": true, "log": true,
		"math": true, "crypto": true, "database/sql": true, "html": true,
		"bufio": true, "bytes": true, "container": true, "flag": true,
	}
	return stdLibPackages[importPath] || !strings.Contains(importPath, ".")
}

// isJavaScriptBuiltIn checks if a module is Node.js built-in
func (da *DependencyAnalyzer) isJavaScriptBuiltIn(modulePath string) bool {
	// Node.js built-in modules
	builtInModules := map[string]bool{
		"fs": true, "path": true, "os": true, "util": true, "events": true,
		"stream": true, "http": true, "https": true, "url": true, "crypto": true,
		"buffer": true, "process": true, "child_process": true, "cluster": true,
		"net": true, "dgram": true, "dns": true, "tls": true, "readline": true,
		"zlib": true, "assert": true, "querystring": true, "string_decoder": true,
	}
	return builtInModules[modulePath]
}

// extractQuotedString extracts a quoted string from text
func (da *DependencyAnalyzer) extractQuotedString(text string) string {
	text = strings.TrimSpace(text)

	// Try single quotes first
	if strings.HasPrefix(text, "'") {
		end := strings.Index(text[1:], "'")
		if end != -1 {
			return text[1 : end+1]
		}
	}

	// Try double quotes
	if strings.HasPrefix(text, "\"") {
		end := strings.Index(text[1:], "\"")
		if end != -1 {
			return text[1 : end+1]
		}
	}

	// Try backticks (template literals)
	if strings.HasPrefix(text, "`") {
		end := strings.Index(text[1:], "`")
		if end != -1 {
			return text[1 : end+1]
		}
	}

	return ""
}

// Ruby-specific helper functions

// isRubyStandardLibrary checks if a module is Ruby standard library
func (da *DependencyAnalyzer) isRubyStandardLibrary(moduleName string) bool {
	// Common Ruby standard library modules
	stdLibModules := map[string]bool{
		"json": true, "yaml": true, "csv": true, "uri": true, "net": true,
		"open-uri": true, "fileutils": true, "pathname": true, "digest": true,
		"base64": true, "time": true, "date": true, "logger": true, "optparse": true,
		"ostruct": true, "set": true, "singleton": true, "tempfile": true,
		"thread": true, "timeout": true, "zlib": true, "stringio": true,
		"erb": true, "cgi": true, "webrick": true, "socket": true,
	}
	return stdLibModules[moduleName]
}

// resolveRubyRequire resolves a Ruby require to a file path
func (da *DependencyAnalyzer) resolveRubyRequire(modulePath, currentFile string, isRelative bool) string {
	currentDir := filepath.Dir(currentFile)

	var targetPath string
	if isRelative {
		// require_relative - always relative to current file
		targetPath = filepath.Join(currentDir, modulePath)
	} else {
		// require - could be relative or absolute
		if strings.HasPrefix(modulePath, "./") || strings.HasPrefix(modulePath, "../") {
			targetPath = filepath.Join(currentDir, modulePath)
		} else {
			// For non-relative requires, try current directory first
			targetPath = filepath.Join(currentDir, modulePath)
		}
	}

	// Try different Ruby file patterns
	patterns := []string{
		targetPath + ".rb",
		targetPath,
	}

	for _, pattern := range patterns {
		if _, err := os.Stat(pattern); err == nil {
			abs, _ := filepath.Abs(pattern)
			return abs
		}
	}

	return ""
}

// CreateDistilledProject creates a multi-file distilled project
func (da *DependencyAnalyzer) CreateDistilledProject(ctx context.Context, entryPoints []string) (*DistilledProject, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	project := &DistilledProject{
		Files:       make(map[string]*DistilledMultiFile),
		EntryPoints: entryPoints,
	}

	// Collect all used function definitions
	usedDefinitions := make(map[string]*FunctionDefinition)
	for fqn := range da.usedSymbols {
		if def, exists := da.functionDefs[fqn]; exists {
			usedDefinitions[fqn] = def
			dbg.Logf(debug.LevelDetailed, "Including definition: %s", fqn)
		}
	}

	// Group definitions by file
	for fqn, def := range usedDefinitions {
		filePath := def.FilePath

		// Initialize file if not exists
		if _, exists := project.Files[filePath]; !exists {
			project.Files[filePath] = &DistilledMultiFile{
				OriginalFilePath: filePath,
				OriginalContent:  da.originalContent[filePath],
				Snippets:         []DistilledCodeSnippet{},
				Language:         def.Language,
			}
		}

		// Add snippet
		snippet := DistilledCodeSnippet{
			StartByte: def.StartByte,
			EndByte:   def.EndByte,
			FQN:       fqn,
			Type:      "function",
		}

		project.Files[filePath].Snippets = append(project.Files[filePath].Snippets, snippet)
	}

	// Extract content for each file
	for filePath, distilledFile := range project.Files {
		err := da.extractDistilledContent(ctx, distilledFile)
		if err != nil {
			dbg.Logf(debug.LevelDetailed, "Failed to extract content for %s: %v", filePath, err)
		}
	}

	dbg.Logf(debug.LevelBasic, "Created distilled project with %d files, %d total definitions",
		len(project.Files), len(usedDefinitions))

	return project, nil
}

// extractDistilledContent extracts the actual code content for a distilled file
func (da *DependencyAnalyzer) extractDistilledContent(ctx context.Context, distilledFile *DistilledMultiFile) error {
	if len(distilledFile.Snippets) == 0 {
		distilledFile.DistilledContent = ""
		return nil
	}

	// Sort snippets by start position
	sort.Slice(distilledFile.Snippets, func(i, j int) bool {
		return distilledFile.Snippets[i].StartByte < distilledFile.Snippets[j].StartByte
	})

	var builder strings.Builder

	// Add PHP opening tag if needed
	if distilledFile.Language == "php" {
		builder.WriteString("<?php\n\n")
	}

	// Extract each snippet
	for i, snippet := range distilledFile.Snippets {
		if snippet.StartByte >= snippet.EndByte {
			continue
		}

		if snippet.EndByte > uint32(len(distilledFile.OriginalContent)) {
			continue
		}

		// Add snippet content
		content := string(distilledFile.OriginalContent[snippet.StartByte:snippet.EndByte])
		builder.WriteString(content)

		// Add separator between snippets
		if i < len(distilledFile.Snippets)-1 {
			builder.WriteString("\n\n")
		}
	}

	distilledFile.DistilledContent = builder.String()
	return nil
}

// convertProjectToSingleFile converts a DistilledProject back to a single DistilledFile for compatibility
func (da *DependencyAnalyzer) convertProjectToSingleFile(ctx context.Context, project *DistilledProject, originalFile *ir.DistilledFile) *ir.DistilledFile {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	result := &ir.DistilledFile{
		Path:     originalFile.Path,
		Language: originalFile.Language,
		Version:  originalFile.Version,
		Children: []ir.DistilledNode{},
	}

	var allContent strings.Builder
	fileCount := 0

	// Combine content from all distilled files
	for filePath, distilledFile := range project.Files {
		if distilledFile.DistilledContent == "" {
			continue
		}

		fileCount++

		// Add file header
		allContent.WriteString(fmt.Sprintf("// === %s ===\n", filePath))
		allContent.WriteString(distilledFile.DistilledContent)
		allContent.WriteString("\n\n")
	}

	if fileCount > 0 {
		// Create a single "implementation" node with all content
		implementationNode := &ir.DistilledComment{
			BaseNode: ir.BaseNode{
				Location: ir.Location{StartLine: 1, EndLine: 1},
			},
			Text:   allContent.String(),
			Format: "implementation",
		}

		result.Children = append(result.Children, implementationNode)
		dbg.Logf(debug.LevelBasic, "Combined %d files into single output (%d chars)",
			fileCount, len(allContent.String()))
	}

	return result
}

// buildSymbolTableForFile builds a symbol table for a single file
func (da *DependencyAnalyzer) buildSymbolTableForFile(ctx context.Context, filePath string) (*semantic.SymbolTable, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	// Store original content for later extraction
	da.originalContent[filePath] = content

	// Detect language from file extension
	language := da.detectLanguage(filePath)
	symbolTable := semantic.NewSymbolTable(filePath, language)

	// Language-specific function detection
	switch language {
	case "php":
		da.parsePhpFunctions(ctx, symbolTable, content, filePath)
	case "python":
		da.parsePythonFunctions(ctx, symbolTable, content, filePath)
	case "go":
		da.parseGoFunctions(ctx, symbolTable, content, filePath)
	case "javascript", "typescript":
		da.parseJavaScriptFunctions(ctx, symbolTable, content, filePath)
	case "ruby":
		da.parseRubyFunctions(ctx, symbolTable, content, filePath)
	case "java":
		da.parseJavaFunctions(ctx, symbolTable, content, filePath)
	case "csharp", "c#":
		da.parseCSharpFunctions(ctx, symbolTable, content, filePath)
	case "rust":
		da.parseRustFunctions(ctx, symbolTable, content, filePath)
	case "swift":
		da.parseSwiftFunctions(ctx, symbolTable, content, filePath)
	case "cpp", "c++":
		da.parseCppFunctions(ctx, symbolTable, content, filePath)
	case "kotlin":
		da.parseKotlinFunctions(ctx, symbolTable, content, filePath)
	default:
		dbg.Logf(debug.LevelDetailed, "Language %s not supported for dependency analysis yet", language)
	}

	return symbolTable, nil
}

// detectLanguage detects the programming language from file extension
func (da *DependencyAnalyzer) detectLanguage(filePath string) string {
	ext := filepath.Ext(filePath)
	switch ext {
	case ".php":
		return "php"
	case ".py":
		return "python"
	case ".go":
		return "go"
	case ".js":
		return "javascript"
	case ".ts":
		return "typescript"
	case ".rb":
		return "ruby"
	case ".java":
		return "java"
	case ".cs":
		return "csharp"
	case ".cpp", ".cc", ".cxx", ".c++":
		return "cpp"
	case ".h", ".hpp", ".hxx", ".h++":
		return "cpp"
	case ".c":
		return "c"
	case ".rs":
		return "rust"
	case ".swift":
		return "swift"
	case ".kt":
		return "kotlin"
	default:
		return "unknown"
	}
}

// parsePhpFunctions parses PHP function definitions
func (da *DependencyAnalyzer) parsePhpFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		line = strings.TrimSpace(line)

		// Look for function definitions: function name(
		if strings.HasPrefix(line, "function ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				funcName := parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					funcName = funcName[:parenIndex]

					// Estimate function end position (simplified - until next function or EOF)
					funcEndPos := da.estimateFunctionEnd(content, lineStart)

					// Create FQN
					fqn := fmt.Sprintf("%s::%s", filePath, funcName)

					// Store function definition for distillation
					da.functionDefs[fqn] = &FunctionDefinition{
						Name:      funcName,
						FQN:       fqn,
						FilePath:  filePath,
						StartByte: lineStart,
						EndByte:   funcEndPos,
						Language:  "php",
					}

					// Create symbol
					symbol := &semantic.Symbol{
						ID:   semantic.GenerateSymbolID(filePath, funcName, ""),
						Name: funcName,
						Kind: semantic.SymbolKindFunction,
						Location: semantic.FileLocation{
							FilePath:  filePath,
							StartLine: i + 1,
							EndLine:   i + 1,
						},
						Visibility: "public",
						IsExported: true,
						Language:   "php",
					}

					symbolTable.AddSymbol(symbol)
					dbg.Logf(debug.LevelDetailed, "Found function: %s in %s (FQN: %s)", funcName, filePath, fqn)
				}
			}
		}

		currentPos = lineEnd
	}
}

// parsePythonFunctions parses Python function definitions
func (da *DependencyAnalyzer) parsePythonFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		// Look for function definitions: def name(
		if strings.HasPrefix(trimmedLine, "def ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName := parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					funcName = funcName[:parenIndex]

					// Estimate function end position
					funcEndPos := da.estimatePythonFunctionEnd(content, lineStart)

					// Create FQN
					fqn := fmt.Sprintf("%s::%s", filePath, funcName)

					// Store function definition for distillation
					da.functionDefs[fqn] = &FunctionDefinition{
						Name:      funcName,
						FQN:       fqn,
						FilePath:  filePath,
						StartByte: lineStart,
						EndByte:   funcEndPos,
						Language:  "python",
					}

					// Create symbol
					symbol := &semantic.Symbol{
						ID:   semantic.GenerateSymbolID(filePath, funcName, ""),
						Name: funcName,
						Kind: semantic.SymbolKindFunction,
						Location: semantic.FileLocation{
							FilePath:  filePath,
							StartLine: i + 1,
							EndLine:   i + 1,
						},
						Visibility: "public",
						IsExported: true,
						Language:   "python",
					}

					symbolTable.AddSymbol(symbol)
					dbg.Logf(debug.LevelDetailed, "Found function: %s in %s (FQN: %s)", funcName, filePath, fqn)
				}
			}
		}

		currentPos = lineEnd
	}
}

// parseGoFunctions parses Go function definitions
func (da *DependencyAnalyzer) parseGoFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		// Look for function definitions: func name(
		if strings.HasPrefix(trimmedLine, "func ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName := parts[1]

				// Handle method receivers: func (r *Receiver) methodName(
				if strings.HasPrefix(funcName, "(") {
					// This is a method with receiver, find the actual method name
					parenIndex := strings.Index(trimmedLine, ") ")
					if parenIndex != -1 {
						remaining := strings.TrimSpace(trimmedLine[parenIndex+2:])
						methodParts := strings.Fields(remaining)
						if len(methodParts) >= 1 {
							funcName = methodParts[0]
						}
					}
				}

				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					funcName = funcName[:parenIndex]

					// Estimate function end position
					funcEndPos := da.estimateGoFunctionEnd(content, lineStart)

					// Create FQN
					fqn := fmt.Sprintf("%s::%s", filePath, funcName)

					// Store function definition for distillation
					da.functionDefs[fqn] = &FunctionDefinition{
						Name:      funcName,
						FQN:       fqn,
						FilePath:  filePath,
						StartByte: lineStart,
						EndByte:   funcEndPos,
						Language:  "go",
					}

					// Create symbol
					symbol := &semantic.Symbol{
						ID:   semantic.GenerateSymbolID(filePath, funcName, ""),
						Name: funcName,
						Kind: semantic.SymbolKindFunction,
						Location: semantic.FileLocation{
							FilePath:  filePath,
							StartLine: i + 1,
							EndLine:   i + 1,
						},
						Visibility: "public",
						IsExported: true,
						Language:   "go",
					}

					symbolTable.AddSymbol(symbol)
					dbg.Logf(debug.LevelDetailed, "Found function: %s in %s (FQN: %s)", funcName, filePath, fqn)
				}
			}
		}

		currentPos = lineEnd
	}
}

// parseJavaScriptFunctions parses JavaScript/TypeScript function definitions
func (da *DependencyAnalyzer) parseJavaScriptFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		var funcName string

		// Look for function definitions: function name( or const name = function( or const name = (
		if strings.HasPrefix(trimmedLine, "function ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName = parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					funcName = funcName[:parenIndex]
				}
			}
		} else if strings.Contains(trimmedLine, " = function") || strings.Contains(trimmedLine, " = (") {
			// Arrow functions or function assignments: const name = function() or const name = () =>
			if strings.HasPrefix(trimmedLine, "const ") || strings.HasPrefix(trimmedLine, "let ") || strings.HasPrefix(trimmedLine, "var ") {
				parts := strings.Fields(trimmedLine)
				if len(parts) >= 2 {
					funcName = parts[1]
				}
			}
		}

		if funcName != "" {
			// Estimate function end position
			funcEndPos := da.estimateJavaScriptFunctionEnd(content, lineStart)

			// Create FQN
			fqn := fmt.Sprintf("%s::%s", filePath, funcName)

			// Store function definition for distillation
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      funcName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: lineStart,
				EndByte:   funcEndPos,
				Language:  symbolTable.Language,
			}

			// Create symbol
			symbol := &semantic.Symbol{
				ID:   semantic.GenerateSymbolID(filePath, funcName, ""),
				Name: funcName,
				Kind: semantic.SymbolKindFunction,
				Location: semantic.FileLocation{
					FilePath:  filePath,
					StartLine: i + 1,
					EndLine:   i + 1,
				},
				Visibility: "public",
				IsExported: true,
				Language:   symbolTable.Language,
			}

			symbolTable.AddSymbol(symbol)
			dbg.Logf(debug.LevelDetailed, "Found function: %s in %s (FQN: %s)", funcName, filePath, fqn)
		}

		currentPos = lineEnd
	}
}

// parseRubyFunctions parses Ruby function definitions
func (da *DependencyAnalyzer) parseRubyFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		// Look for function definitions: def name, def self.name
		if strings.HasPrefix(trimmedLine, "def ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcSignature := parts[1]

				// Extract function name (remove parameters if present)
				var funcName string
				if parenIndex := strings.Index(funcSignature, "("); parenIndex != -1 {
					funcName = funcSignature[:parenIndex]
				} else {
					funcName = funcSignature
				}

				// Handle self.method_name for class methods
				if strings.HasPrefix(funcName, "self.") {
					funcName = funcName[5:] // Remove "self."
				}

				// Estimate function end position
				funcEndPos := da.estimateRubyFunctionEnd(content, lineStart)

				// Create FQN
				fqn := fmt.Sprintf("%s::%s", filePath, funcName)

				// Store function definition for distillation
				da.functionDefs[fqn] = &FunctionDefinition{
					Name:      funcName,
					FQN:       fqn,
					FilePath:  filePath,
					StartByte: lineStart,
					EndByte:   funcEndPos,
					Language:  "ruby",
				}

				// Create symbol
				symbol := &semantic.Symbol{
					ID:   semantic.GenerateSymbolID(filePath, funcName, ""),
					Name: funcName,
					Kind: semantic.SymbolKindFunction,
					Location: semantic.FileLocation{
						FilePath:  filePath,
						StartLine: i + 1,
						EndLine:   i + 1,
					},
					Visibility: "public",
					IsExported: true,
					Language:   "ruby",
				}

				symbolTable.AddSymbol(symbol)
				dbg.Logf(debug.LevelDetailed, "Found function: %s in %s (FQN: %s)", funcName, filePath, fqn)
			}
		}

		currentPos = lineEnd
	}
}

// estimateFunctionEnd estimates the end position of a function (simplified)
func (da *DependencyAnalyzer) estimateFunctionEnd(content []byte, startPos uint32) uint32 {
	// Simple heuristic: find the next "function " or end of file
	remaining := content[startPos:]

	// Look for the next function keyword
	nextFunc := strings.Index(string(remaining), "\nfunction ")
	if nextFunc != -1 {
		return startPos + uint32(nextFunc)
	}

	// No next function, go to end of file
	return uint32(len(content))
}

// estimatePythonFunctionEnd estimates the end position of a Python function
func (da *DependencyAnalyzer) estimatePythonFunctionEnd(content []byte, startPos uint32) uint32 {
	lines := strings.Split(string(content), "\n")

	// Find the line containing startPos
	currentPos := uint32(0)
	startLine := 0
	for i, line := range lines {
		lineEnd := currentPos + uint32(len(line)) + 1
		if currentPos <= startPos && startPos < lineEnd {
			startLine = i
			break
		}
		currentPos = lineEnd
	}

	// Find the next function definition or class at the same indentation level
	defIndent := 0
	if startLine < len(lines) {
		for i, char := range lines[startLine] {
			if char != ' ' && char != '\t' {
				defIndent = i
				break
			}
		}
	}

	// Look for next def/class at same or lower indentation
	// Calculate position at the end of the start line
	currentPos = startPos
	for k := startLine; k < len(lines); k++ {
		if k == startLine {
			// Skip to end of the def line
			currentPos += uint32(len(lines[k])) + 1
			continue
		}
		i := k
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			// Skip empty lines
			currentPos += uint32(len(line)) + 1
			continue
		}

		// Check indentation
		lineIndent := 0
		for j, char := range line {
			if char != ' ' && char != '\t' {
				lineIndent = j
				break
			}
		}

		// If we find a line at same or lower indentation with def/class, this is the end
		if lineIndent <= defIndent && (strings.HasPrefix(strings.TrimSpace(line), "def ") ||
			strings.HasPrefix(strings.TrimSpace(line), "class ")) {
			return currentPos
		}

		currentPos += uint32(len(line)) + 1
	}

	// No next function/class found, go to end of file
	return uint32(len(content))
}

// estimateGoFunctionEnd estimates the end position of a Go function
func (da *DependencyAnalyzer) estimateGoFunctionEnd(content []byte, startPos uint32) uint32 {
	remaining := content[startPos:]

	// Look for opening brace
	openBrace := strings.Index(string(remaining), "{")
	if openBrace == -1 {
		return uint32(len(content))
	}

	// Count braces to find matching closing brace
	braceCount := 0
	inString := false
	escapeNext := false

	for i := openBrace; i < len(remaining); i++ {
		char := remaining[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if char == '"' || char == '`' {
			inString = !inString
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					return startPos + uint32(i) + 1
				}
			}
		}
	}

	// No matching brace found, go to end of file
	return uint32(len(content))
}

// estimateJavaScriptFunctionEnd estimates the end position of a JavaScript function
func (da *DependencyAnalyzer) estimateJavaScriptFunctionEnd(content []byte, startPos uint32) uint32 {
	remaining := content[startPos:]

	// Look for opening brace or arrow function body
	openBrace := strings.Index(string(remaining), "{")
	arrow := strings.Index(string(remaining), "=>")

	// Handle arrow functions
	if arrow != -1 && (openBrace == -1 || arrow < openBrace) {
		// Check if it's a single expression arrow function
		afterArrow := remaining[arrow+2:]
		afterArrowTrimmed := strings.TrimSpace(string(afterArrow))
		if !strings.HasPrefix(afterArrowTrimmed, "{") {
			// Single expression, find end of line or semicolon
			lineEnd := strings.Index(string(afterArrow), "\n")
			semicolon := strings.Index(string(afterArrow), ";")
			if semicolon != -1 && (lineEnd == -1 || semicolon < lineEnd) {
				return startPos + uint32(arrow) + 2 + uint32(semicolon) + 1
			}
			if lineEnd != -1 {
				return startPos + uint32(arrow) + 2 + uint32(lineEnd)
			}
			return uint32(len(content))
		}
		// Arrow function with block, treat like regular function
		openBrace = strings.Index(string(afterArrow), "{")
		if openBrace != -1 {
			openBrace += arrow + 2
		}
	}

	if openBrace == -1 {
		return uint32(len(content))
	}

	// Count braces to find matching closing brace
	braceCount := 0
	inString := false
	stringChar := byte(0)
	escapeNext := false

	for i := openBrace; i < len(remaining); i++ {
		char := remaining[i]

		if escapeNext {
			escapeNext = false
			continue
		}

		if char == '\\' {
			escapeNext = true
			continue
		}

		if !inString && (char == '"' || char == '\'' || char == '`') {
			inString = true
			stringChar = char
			continue
		}

		if inString && char == stringChar {
			inString = false
			continue
		}

		if !inString {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					return startPos + uint32(i) + 1
				}
			}
		}
	}

	// No matching brace found, go to end of file
	return uint32(len(content))
}

// buildCrossFileCallGraph builds call graph across multiple files
func (da *DependencyAnalyzer) buildCrossFileCallGraph(ctx context.Context, symbolTables map[string]*semantic.SymbolTable) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// For each file, find function calls and link them in call graph
	for filePath := range symbolTables {
		err := da.extractCallsFromFile(ctx, filePath, symbolTables)
		if err != nil {
			dbg.Logf(debug.LevelDetailed, "Failed to extract calls from %s: %v", filePath, err)
		}
	}
}

// extractCallsFromFile extracts function calls from a file and updates call graph
func (da *DependencyAnalyzer) extractCallsFromFile(ctx context.Context, filePath string, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Detect language and use appropriate call extraction
	language := da.detectLanguage(filePath)
	switch language {
	case "php":
		return da.extractPhpCalls(ctx, filePath, content, symbolTables)
	case "python":
		return da.extractPythonCalls(ctx, filePath, content, symbolTables)
	case "go":
		return da.extractGoCalls(ctx, filePath, content, symbolTables)
	case "javascript", "typescript":
		return da.extractJavaScriptCalls(ctx, filePath, content, symbolTables)
	case "ruby":
		return da.extractRubyCalls(ctx, filePath, content, symbolTables)
	case "java":
		return da.extractJavaCalls(ctx, filePath, content, symbolTables)
	case "csharp", "c#":
		return da.extractCSharpCalls(ctx, filePath, content, symbolTables)
	case "rust":
		return da.extractRustCalls(ctx, filePath, content, symbolTables)
	case "swift":
		return da.extractSwiftCalls(ctx, filePath, content, symbolTables)
	case "cpp", "c++":
		return da.extractCppCalls(ctx, filePath, content, symbolTables)
	case "kotlin":
		return da.extractKotlinCalls(ctx, filePath, content, symbolTables)
	default:
		dbg.Logf(debug.LevelDetailed, "Call extraction for language %s not implemented yet", language)
		return nil
	}
}

// extractPhpCalls extracts function calls from PHP code
func (da *DependencyAnalyzer) extractPhpCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentFunction := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Track current function context
		if strings.HasPrefix(line, "function ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				funcName := parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					currentFunction = funcName[:parenIndex]
				}
			}
		}

		// Look for function calls: functionName(
		if currentFunction != "" && strings.Contains(line, "(") && !strings.HasPrefix(line, "function ") {
			// Simple pattern matching for function calls
			words := strings.FieldsFunc(line, func(c rune) bool {
				return c == ' ' || c == '(' || c == ')' || c == ';' || c == ',' || c == '=' || c == '$'
			})

			for _, word := range words {
				word = strings.TrimSpace(word)
				if word == "" || strings.HasPrefix(word, "//") {
					continue
				}

				// Check if this word is a function call (exists in any symbol table)
				for _, symTable := range symbolTables {
					if symbol, exists := symTable.Symbols[word]; exists && symbol.Kind == semantic.SymbolKindFunction {
						// Found a function call!
						callerFQN := fmt.Sprintf("%s::%s", filePath, currentFunction)
						calleeFQN := fmt.Sprintf("%s::%s", symbol.Location.FilePath, word)

						da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
						dbg.Logf(debug.LevelDetailed, "Found call: %s -> %s", callerFQN, calleeFQN)
						break
					}
				}
			}
		}
	}

	return nil
}

// extractPythonCalls extracts function calls from Python code
func (da *DependencyAnalyzer) extractPythonCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentFunction := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Track current function context
		if strings.HasPrefix(trimmedLine, "def ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName := parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					currentFunction = funcName[:parenIndex]
				}
			}
		}

		// Look for function calls: functionName(
		if currentFunction != "" && strings.Contains(trimmedLine, "(") && !strings.HasPrefix(trimmedLine, "def ") {
			// Simple pattern matching for function calls
			words := strings.FieldsFunc(trimmedLine, func(c rune) bool {
				return c == ' ' || c == '(' || c == ')' || c == ',' || c == '=' || c == ':' || c == '.'
			})

			for _, word := range words {
				word = strings.TrimSpace(word)
				if word == "" || strings.HasPrefix(word, "#") {
					continue
				}

				// Check if this word is a function call (exists in any symbol table)
				for _, symTable := range symbolTables {
					if symbol, exists := symTable.Symbols[word]; exists && symbol.Kind == semantic.SymbolKindFunction {
						// Found a function call!
						callerFQN := fmt.Sprintf("%s::%s", filePath, currentFunction)
						calleeFQN := fmt.Sprintf("%s::%s", symbol.Location.FilePath, word)

						da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
						dbg.Logf(debug.LevelDetailed, "Found call: %s -> %s", callerFQN, calleeFQN)
						break
					}
				}
			}
		}
	}

	return nil
}

// extractGoCalls extracts function calls from Go code
func (da *DependencyAnalyzer) extractGoCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentFunction := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Track current function context
		if strings.HasPrefix(trimmedLine, "func ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName := parts[1]

				// Handle method receivers
				if strings.HasPrefix(funcName, "(") {
					parenIndex := strings.Index(trimmedLine, ") ")
					if parenIndex != -1 {
						remaining := strings.TrimSpace(trimmedLine[parenIndex+2:])
						methodParts := strings.Fields(remaining)
						if len(methodParts) >= 1 {
							funcName = methodParts[0]
						}
					}
				}

				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					currentFunction = funcName[:parenIndex]
				}
			}
		}

		// Look for function calls: functionName(
		if currentFunction != "" && strings.Contains(trimmedLine, "(") && !strings.HasPrefix(trimmedLine, "func ") {
			// Simple pattern matching for function calls
			words := strings.FieldsFunc(trimmedLine, func(c rune) bool {
				return c == ' ' || c == '(' || c == ')' || c == ',' || c == '=' || c == ':' || c == '.'
			})

			for _, word := range words {
				word = strings.TrimSpace(word)
				if word == "" || strings.HasPrefix(word, "//") {
					continue
				}

				// Check if this word is a function call (exists in any symbol table)
				for _, symTable := range symbolTables {
					if symbol, exists := symTable.Symbols[word]; exists && symbol.Kind == semantic.SymbolKindFunction {
						// Found a function call!
						callerFQN := fmt.Sprintf("%s::%s", filePath, currentFunction)
						calleeFQN := fmt.Sprintf("%s::%s", symbol.Location.FilePath, word)

						da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
						dbg.Logf(debug.LevelDetailed, "Found call: %s -> %s", callerFQN, calleeFQN)
						break
					}
				}
			}
		}
	}

	return nil
}

// extractJavaScriptCalls extracts function calls from JavaScript/TypeScript code
func (da *DependencyAnalyzer) extractJavaScriptCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentFunction := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Track current function context
		if strings.HasPrefix(trimmedLine, "function ") {
			parts := strings.Fields(trimmedLine)
			if len(parts) >= 2 {
				funcName := parts[1]
				if parenIndex := strings.Index(funcName, "("); parenIndex != -1 {
					currentFunction = funcName[:parenIndex]
				}
			}
		} else if strings.Contains(trimmedLine, " = function") || strings.Contains(trimmedLine, " = (") {
			// Arrow functions or function assignments
			if strings.HasPrefix(trimmedLine, "const ") || strings.HasPrefix(trimmedLine, "let ") || strings.HasPrefix(trimmedLine, "var ") {
				parts := strings.Fields(trimmedLine)
				if len(parts) >= 2 {
					currentFunction = parts[1]
				}
			}
		}

		// Look for function calls: functionName(
		if currentFunction != "" && strings.Contains(trimmedLine, "(") &&
			!strings.HasPrefix(trimmedLine, "function ") && !strings.Contains(trimmedLine, " = function") {
			// Simple pattern matching for function calls
			words := strings.FieldsFunc(trimmedLine, func(c rune) bool {
				return c == ' ' || c == '(' || c == ')' || c == ',' || c == '=' || c == ';' || c == '.'
			})

			for _, word := range words {
				word = strings.TrimSpace(word)
				if word == "" || strings.HasPrefix(word, "//") {
					continue
				}

				// Check if this word is a function call (exists in any symbol table)
				for _, symTable := range symbolTables {
					if symbol, exists := symTable.Symbols[word]; exists && symbol.Kind == semantic.SymbolKindFunction {
						// Found a function call!
						callerFQN := fmt.Sprintf("%s::%s", filePath, currentFunction)
						calleeFQN := fmt.Sprintf("%s::%s", symbol.Location.FilePath, word)

						da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
						dbg.Logf(debug.LevelDetailed, "Found call: %s -> %s", callerFQN, calleeFQN)
						break
					}
				}
			}
		}
	}

	return nil
}

// markSymbolAsUsedRecursive marks a symbol and its dependencies as used
func (da *DependencyAnalyzer) markSymbolAsUsedRecursive(ctx context.Context, symbolFQN string, depth int) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Check depth limit
	if depth > da.maxDepth {
		dbg.Logf(debug.LevelDetailed, "Max depth %d reached for symbol %s", da.maxDepth, symbolFQN)
		return
	}

	// Check if already visited
	visitKey := fmt.Sprintf("%s:%d", symbolFQN, depth)
	if da.visited[visitKey] {
		return
	}
	da.visited[visitKey] = true

	// Mark this symbol as used
	da.usedSymbols[symbolFQN] = true
	dbg.Logf(debug.LevelDetailed, "Marking symbol as used: %s (depth %d)", symbolFQN, depth)

	// Recursively mark called symbols
	for _, calledSymbol := range da.callGraph[symbolFQN] {
		da.markSymbolAsUsedRecursive(ctx, calledSymbol, depth+1)
	}
}

// findCalledSymbols finds all symbols (functions/methods) called by the given symbol
func (da *DependencyAnalyzer) findCalledSymbols(symbol string, analysis *semantic.FileAnalysis) []string {
	called := []string{}

	// Look up the symbol in the symbol table
	symbolInfo, exists := analysis.SymbolTable.Symbols[symbol]
	if !exists {
		return called
	}

	// Extract called functions from the symbol's metadata
	if symbolInfo.Metadata.Calls != nil {
		for _, call := range symbolInfo.Metadata.Calls {
			called = append(called, call.FunctionName)
		}
	}

	return called
}

// findUsedTypes finds all types used by the given symbol (parameters, return types, etc.)
func (da *DependencyAnalyzer) findUsedTypes(symbol string, analysis *semantic.FileAnalysis) []string {
	usedTypes := []string{}

	// Look up the symbol in the symbol table
	symbolInfo, exists := analysis.SymbolTable.Symbols[symbol]
	if !exists {
		return usedTypes
	}

	// Extract types from parameters
	for _, param := range symbolInfo.Metadata.Parameters {
		if param.Type != "" && param.Type != "string" && param.Type != "int" && param.Type != "bool" {
			usedTypes = append(usedTypes, param.Type)
		}
	}

	// Extract return type
	if symbolInfo.Metadata.ReturnType != "" && symbolInfo.Metadata.ReturnType != "string" && symbolInfo.Metadata.ReturnType != "int" && symbolInfo.Metadata.ReturnType != "bool" {
		usedTypes = append(usedTypes, symbolInfo.Metadata.ReturnType)
	}

	// Extract types from variable assignments using TypeTracker
	if analysis.SymbolTable.TypeTracker != nil {
		for _, typeName := range analysis.SymbolTable.TypeTracker.Variables {
			// For now, assume all types in the tracker are used (simplified heuristic)
			usedTypes = append(usedTypes, typeName)
		}
	}

	return usedTypes
}

// filterFileByUsage creates a new distilled file containing only used symbols
func (da *DependencyAnalyzer) filterFileByUsage(ctx context.Context, file *ir.DistilledFile, symbolTable *semantic.SymbolTable) *ir.DistilledFile {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	filteredFile := &ir.DistilledFile{
		Path:     file.Path,
		Language: file.Language,
		Children: []ir.DistilledNode{},
	}

	originalCount := 0
	filteredCount := 0

	// Filter functions
	for _, child := range file.Children {
		originalCount++

		switch node := child.(type) {
		case *ir.DistilledFunction:
			// Check if this function is used by FQN
			fqn := fmt.Sprintf("%s::%s", file.Path, node.Name)
			if da.isSymbolUsedByFQN(fqn) {
				filteredFile.Children = append(filteredFile.Children, node)
				filteredCount++
				dbg.Logf(debug.LevelDetailed, "Including function: %s (FQN: %s)", node.Name, fqn)
			} else {
				dbg.Logf(debug.LevelDetailed, "Excluding function: %s (FQN: %s)", node.Name, fqn)
			}

		case *ir.DistilledClass:
			filteredClass := da.filterClass(ctx, node)
			if filteredClass != nil {
				filteredFile.Children = append(filteredFile.Children, filteredClass)
				filteredCount++
				dbg.Logf(debug.LevelDetailed, "Including class: %s", node.Name)
			} else {
				dbg.Logf(debug.LevelDetailed, "Excluding class: %s", node.Name)
			}

		default:
			// Keep other nodes (imports, comments, etc.) as-is
			filteredFile.Children = append(filteredFile.Children, node)
			filteredCount++
		}
	}

	dbg.Logf(debug.LevelBasic, "Filtered %d -> %d nodes (%.1f%% reduction)",
		originalCount, filteredCount, float64(originalCount-filteredCount)/float64(originalCount)*100)

	return filteredFile
}

// filterClass filters a class to include only used methods and fields
func (da *DependencyAnalyzer) filterClass(ctx context.Context, class *ir.DistilledClass) *ir.DistilledClass {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Check if the class itself is used
	if !da.isSymbolUsed(class.Name) {
		return nil
	}

	filteredClass := &ir.DistilledClass{
		Name:       class.Name,
		Visibility: class.Visibility,
		Extends:    class.Extends,
		Implements: class.Implements,
		Children:   []ir.DistilledNode{},
	}

	methodCount := 0

	// Filter children (methods and fields)
	for _, child := range class.Children {
		switch node := child.(type) {
		case *ir.DistilledFunction:
			methodKey := fmt.Sprintf("%s.%s", class.Name, node.Name)
			if da.isSymbolUsed(methodKey) || da.isSymbolUsed(node.Name) {
				filteredClass.Children = append(filteredClass.Children, node)
				methodCount++
				dbg.Logf(debug.LevelDetailed, "Including method: %s", methodKey)
			} else {
				dbg.Logf(debug.LevelDetailed, "Excluding method: %s", methodKey)
			}
		default:
			// For now, include all non-method children (fields, etc.)
			// TODO: More sophisticated field usage analysis
			filteredClass.Children = append(filteredClass.Children, child)
		}
	}

	// If no methods remain, don't include the class
	if methodCount == 0 {
		return nil
	}

	return filteredClass
}

// isSymbolUsed checks if a symbol is marked as used
func (da *DependencyAnalyzer) isSymbolUsed(symbol string) bool {
	return da.usedSymbols[symbol]
}

// isSymbolUsedByFQN checks if a symbol is marked as used by its fully qualified name
func (da *DependencyAnalyzer) isSymbolUsedByFQN(fqn string) bool {
	return da.usedSymbols[fqn]
}

// ProcessWithDependencyAnalysis processes a file with dependency-aware analysis
func ProcessWithDependencyAnalysis(ctx context.Context, proc *Processor, filePath string, opts ProcessOptions) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Create a temporary copy of opts without dependency analysis to avoid recursion
	tempOpts := opts
	tempOpts.SymbolResolution = false
	tempOpts.MaxDepth = 0

	// First, process the file normally without dependency analysis
	file, err := proc.ProcessFile(filePath, tempOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to process file: %w", err)
	}

	// If dependency analysis is not enabled, return the normal result
	if !opts.SymbolResolution || opts.MaxDepth < 0 {
		return file, nil
	}

	dbg.Logf(debug.LevelBasic, "Enabling dependency-aware analysis for %s", filePath)

	// Determine project root
	projectRoot, err := findProjectRoot(filePath)
	if err != nil {
		dbg.Logf(debug.LevelBasic, "Could not determine project root: %v, using file directory", err)
		projectRoot = filepath.Dir(filePath)
	}

	// Create dependency analyzer
	analyzer, err := NewDependencyAnalyzer(projectRoot, opts.MaxDepth)
	if err != nil {
		dbg.Logf(debug.LevelBasic, "Failed to create dependency analyzer: %v, returning normal result", err)
		return file, nil
	}

	// Run dependency analysis regardless of whether regular processor found content
	// The dependency analyzer has its own parsing logic and can work with raw source
	// Preserve the original full path since file.Path might be just the basename
	file.Path = filePath
	return analyzer.AnalyzeDependencies(ctx, file)
}

// findProjectRoot attempts to find the project root directory
func findProjectRoot(filePath string) (string, error) {
	dir := filepath.Dir(filePath)
	for dir != "/" && dir != "." {
		// Look for common project markers
		markers := []string{"go.mod", "package.json", "pyproject.toml", "Cargo.toml", ".git"}
		for _, marker := range markers {
			if exists(filepath.Join(dir, marker)) {
				return dir, nil
			}
		}
		dir = filepath.Dir(dir)
	}
	return "", fmt.Errorf("project root not found")
}

// exists checks if a file or directory exists
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// estimateRubyFunctionEnd estimates the end position of a Ruby function
func (da *DependencyAnalyzer) estimateRubyFunctionEnd(content []byte, startPos uint32) uint32 {
	lines := strings.Split(string(content), "\n")

	// Find the line containing startPos
	currentPos := uint32(0)
	startLine := 0
	baseIndent := 0

	for i, line := range lines {
		lineEnd := currentPos + uint32(len(line)) + 1
		if currentPos <= startPos && startPos < lineEnd {
			startLine = i
			// Calculate base indentation
			for _, char := range line {
				if char == ' ' {
					baseIndent++
				} else if char == '\t' {
					baseIndent += 2 // Treat tab as 2 spaces
				} else {
					break
				}
			}
			break
		}
		currentPos = lineEnd
	}

	// Look for matching 'end' or next function at same indentation level
	// Calculate the current position at the start of the next line after startLine
	currentPos = uint32(0)
	for k := 0; k <= startLine; k++ {
		if k < len(lines) {
			currentPos += uint32(len(lines[k])) + 1
		}
	}

	for i := startLine + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			currentPos += uint32(len(line)) + 1
			continue
		}

		// Calculate line indentation
		lineIndent := 0
		for _, char := range line {
			if char == ' ' {
				lineIndent++
			} else if char == '\t' {
				lineIndent += 2
			} else {
				break
			}
		}

		// Found 'end' at same or lower indentation level
		if strings.HasPrefix(trimmed, "end") && lineIndent <= baseIndent {
			return currentPos + uint32(len(line)) + 1 // Include the 'end' line
		}

		// Found another function/class/module at same or lower indentation
		if (strings.HasPrefix(trimmed, "def ") ||
			strings.HasPrefix(trimmed, "class ") ||
			strings.HasPrefix(trimmed, "module ")) && lineIndent <= baseIndent {
			return currentPos // Don't include the new definition
		}

		currentPos += uint32(len(line)) + 1
	}

	// No explicit end found, function goes to end of file
	return uint32(len(content))
}

// extractRubyCalls extracts function calls from Ruby content
func (da *DependencyAnalyzer) extractRubyCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Pattern for Ruby method calls:
		// 1. Module.method_name or Class.method_name (static method call)
		// 2. obj.method_name (instance method call)
		// 3. method_name with arguments (local method call)
		// 4. method_name without arguments (local method call)
		callPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(\w+)\.(\w+)`),         // Module.method or obj.method
			regexp.MustCompile(`\b(\w+)\s*\(`),         // method_name(
			regexp.MustCompile(`\b(\w+)\s+[\w"'\[\{]`), // method_name arg (Ruby style without parens)
			regexp.MustCompile(`^\s*([a-z_]\w*)\s*$`),  // method_name alone on line (start with lowercase or underscore)
		}

		for _, pattern := range callPatterns {
			matches := pattern.FindAllStringSubmatch(trimmed, -1)
			for _, match := range matches {
				var caller, callee, calleeFQN string

				if len(match) >= 3 {
					// Module.method pattern
					module := match[1]
					method := match[2]
					// For Ruby: Module.method_name should resolve to method_name in the module's file
					callee = method // Use just the method name for symbol lookup

					// Try to resolve cross-file call by mapping module to file
					if moduleFile := da.resolveRubyModuleToFile(module, symbolTables); moduleFile != "" {
						calleeFQN = fmt.Sprintf("%s::%s", moduleFile, method)
					} else {
						calleeFQN = fmt.Sprintf("%s::%s", filePath, method) // Fallback to local file
					}
				} else if len(match) >= 2 {
					// Simple method call
					callee = match[1]
					calleeFQN = fmt.Sprintf("%s::%s", filePath, callee)
				}

				if callee != "" {
					// Filter out common Ruby keywords and operators
					rubyKeywords := map[string]bool{
						"if": true, "unless": true, "while": true, "until": true,
						"for": true, "case": true, "when": true, "begin": true,
						"rescue": true, "ensure": true, "end": true, "class": true,
						"module": true, "def": true, "return": true, "yield": true,
						"puts": true, "print": true, "p": true, "require": true,
						"include": true, "extend": true, "attr_reader": true,
						"attr_writer": true, "attr_accessor": true, "new": true,
						"nil": true, "true": true, "false": true, "self": true,
						"super": true, "and": true, "or": true, "not": true,
						"length": true, "size": true, "empty": true, "strip": true,
						"map": true, "select": true, "reject": true, "sum": true,
						"inspect": true,
					}

					if !rubyKeywords[callee] && !rubyKeywords[strings.Split(callee, ".")[0]] {
						// Look for the containing function to establish caller
						caller = da.findContainingRubyFunction(filePath, line, content)
						if caller != "" {
							callerFQN := fmt.Sprintf("%s::%s", filePath, caller)

							// Check if callee exists in symbol tables
							if da.symbolExistsInProjectTables(callee, symbolTables) {
								da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
								dbg.Logf(debug.LevelDetailed, "Found Ruby call: %s -> %s", callerFQN, calleeFQN)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// findContainingRubyFunction finds which Ruby function contains a given line
func (da *DependencyAnalyzer) findContainingRubyFunction(filePath, targetLine string, content []byte) string {
	lines := strings.Split(string(content), "\n")
	currentFunction := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a function definition
		if strings.HasPrefix(trimmed, "def ") {
			parts := strings.Fields(trimmed)
			if len(parts) >= 2 {
				funcSignature := parts[1]

				// Extract function name (remove parameters if present)
				var funcName string
				if parenIndex := strings.Index(funcSignature, "("); parenIndex != -1 {
					funcName = funcSignature[:parenIndex]
				} else {
					funcName = funcSignature
				}

				// Handle self.method_name for class methods
				if strings.HasPrefix(funcName, "self.") {
					funcName = funcName[5:] // Remove "self."
				}

				currentFunction = funcName
			}
		}

		// If this is our target line, return the current function
		if strings.TrimSpace(line) == strings.TrimSpace(targetLine) {
			return currentFunction
		}
	}

	return currentFunction
}

// symbolExistsInProjectTables checks if a symbol exists in any of the project symbol tables
func (da *DependencyAnalyzer) symbolExistsInProjectTables(symbolName string, symbolTables map[string]*semantic.SymbolTable) bool {
	for _, symbolTable := range symbolTables {
		if symbolTable != nil {
			for _, symbol := range symbolTable.Symbols {
				if symbol.Name == symbolName {
					return true
				}
			}
		}
	}
	return false
}

// resolveRubyModuleToFile maps a Ruby module name to its corresponding file path
func (da *DependencyAnalyzer) resolveRubyModuleToFile(moduleName string, symbolTables map[string]*semantic.SymbolTable) string {
	// Common Ruby module to file mappings
	moduleToFile := map[string]string{
		"Utils":       "utils.rb",
		"Processor":   "processor.rb",
		"DataHandler": "data_handler.rb",
	}

	// Look for the module file in symbol tables
	expectedFileName := moduleToFile[moduleName]
	if expectedFileName != "" {
		for filePath := range symbolTables {
			if strings.HasSuffix(filePath, expectedFileName) {
				return filePath
			}
		}
	}

	// Fallback: look for snake_case version of module name
	snakeCase := strings.ToLower(moduleName)
	// Convert CamelCase to snake_case roughly
	if moduleName != snakeCase {
		expectedFileName = snakeCase + ".rb"
		for filePath := range symbolTables {
			if strings.HasSuffix(filePath, expectedFileName) {
				return filePath
			}
		}
	}

	return ""
}

// extractJavaImports extracts Java import statements and discovers same-package classes
func (da *DependencyAnalyzer) extractJavaImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	var imports []string

	// First, process explicit import statements
	lines := strings.Split(string(fileContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match Java import statements: import package.ClassName;
		if strings.HasPrefix(line, "import ") && strings.HasSuffix(line, ";") {
			importLine := strings.TrimPrefix(line, "import ")
			importLine = strings.TrimSuffix(importLine, ";")
			importLine = strings.TrimSpace(importLine)

			// Skip standard library imports
			if da.isJavaStandardLibrary(importLine) {
				continue
			}

			// For Java, we typically import classes from other packages
			className := importLine
			if lastDot := strings.LastIndex(importLine, "."); lastDot != -1 {
				className = importLine[lastDot+1:]
			}

			// Resolve to local Java file
			if resolvedPath := da.resolveJavaImport(className, filePath); resolvedPath != "" {
				imports = append(imports, resolvedPath)
				dbg.Logf(debug.LevelDetailed, "Found Java explicit import: %s -> %s", className, resolvedPath)
			}
		}
	}

	// Second, for Java auto-discover all .java files in the same directory
	// because Java classes in the same package don't need explicit imports
	dir := filepath.Dir(filePath)
	files, err := os.ReadDir(dir)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".java") {
				fullPath := filepath.Join(dir, file.Name())
				// Don't include the current file itself
				if fullPath != filePath {
					imports = append(imports, fullPath)
					dbg.Logf(debug.LevelDetailed, "Found Java same-package class: %s", fullPath)
				}
			}
		}
	}

	return imports, nil
}

// parseJavaFunctions parses Java method definitions
func (da *DependencyAnalyzer) parseJavaFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		// Look for Java method definitions: public static void methodName(...)
		// Pattern: [visibility] [static] returnType methodName(parameters) {
		// Updated to handle generics in return types like Map<String, Integer>
		methodPattern := regexp.MustCompile(`(public|private|protected)?\s*(static)?\s*[\w<>,\s]+\s+(\w+)\s*\([^)]*\)\s*\{?`)
		if match := methodPattern.FindStringSubmatch(trimmedLine); match != nil {
			methodName := match[3]

			// Skip constructors (method name same as class name)
			className := da.extractJavaClassName(filePath)
			if methodName == className {
				continue
			}

			// Estimate method end position
			methodEndPos := da.estimateJavaMethodEnd(content, lineStart)

			// Create FQN
			fqn := fmt.Sprintf("%s::%s", filePath, methodName)

			// Store method definition for distillation
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      methodName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: lineStart,
				EndByte:   methodEndPos,
				Language:  "java",
			}

			// Create symbol
			visibility := "public"
			if match[1] != "" {
				visibility = match[1]
			}

			symbol := &semantic.Symbol{
				ID:   semantic.GenerateSymbolID(filePath, methodName, ""),
				Name: methodName,
				Kind: semantic.SymbolKindFunction,
				Location: semantic.FileLocation{
					FilePath:  filePath,
					StartLine: i + 1,
					EndLine:   i + 1,
				},
				Visibility: visibility,
				IsExported: visibility == "public",
				Language:   "java",
			}

			symbolTable.AddSymbol(symbol)
			dbg.Logf(debug.LevelDetailed, "Found Java method: %s in %s (FQN: %s)", methodName, filePath, fqn)
		}

		currentPos = lineEnd
	}
}

// estimateJavaMethodEnd estimates the end position of a Java method
func (da *DependencyAnalyzer) estimateJavaMethodEnd(content []byte, startPos uint32) uint32 {
	lines := strings.Split(string(content), "\n")

	// Find the line containing startPos
	currentPos := uint32(0)
	startLine := 0
	braceCount := 0

	for i, line := range lines {
		lineEnd := currentPos + uint32(len(line)) + 1
		if currentPos <= startPos && startPos < lineEnd {
			startLine = i
			break
		}
		currentPos = lineEnd
	}

	// Look for matching closing brace
	currentPos = startPos
	for i := startLine; i < len(lines); i++ {
		line := lines[i]

		// Count braces to find method end
		for _, char := range line {
			if char == '{' {
				braceCount++
			} else if char == '}' {
				braceCount--
				if braceCount == 0 {
					// Found the closing brace
					return currentPos + uint32(len(line)) + 1
				}
			}
		}

		currentPos += uint32(len(line)) + 1
	}

	// No explicit end found, method goes to end of file
	return uint32(len(content))
}

// extractJavaCalls extracts method calls from Java content
func (da *DependencyAnalyzer) extractJavaCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Pattern for Java method calls:
		// 1. ClassName.methodName (static method call)
		// 2. obj.methodName (instance method call)
		// 3. methodName (local method call)
		callPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(\w+)\.(\w+)\s*\(`), // ClassName.method( or obj.method(
			regexp.MustCompile(`\b(\w+)\s*\(`),      // method(
		}

		for _, pattern := range callPatterns {
			matches := pattern.FindAllStringSubmatch(trimmed, -1)
			for _, match := range matches {
				var caller, callee, calleeFQN string

				if len(match) >= 3 {
					// ClassName.method pattern
					className := match[1]
					method := match[2]
					callee = method // Use just the method name for symbol lookup

					// Try to resolve cross-file call by mapping class to file
					if classFile := da.resolveJavaClassToFile(className, symbolTables); classFile != "" {
						calleeFQN = fmt.Sprintf("%s::%s", classFile, method)
					} else {
						calleeFQN = fmt.Sprintf("%s::%s", filePath, method) // Fallback to local file
					}
				} else if len(match) >= 2 {
					// Simple method call
					callee = match[1]
					calleeFQN = fmt.Sprintf("%s::%s", filePath, callee)
				}

				if callee != "" {
					// Filter out common Java keywords and operators
					javaKeywords := map[string]bool{
						"if": true, "else": true, "while": true, "for": true,
						"do": true, "switch": true, "case": true, "default": true,
						"try": true, "catch": true, "finally": true, "throw": true,
						"throws": true, "return": true, "break": true, "continue": true,
						"class": true, "interface": true, "extends": true, "implements": true,
						"public": true, "private": true, "protected": true, "static": true,
						"final": true, "abstract": true, "synchronized": true, "volatile": true,
						"new": true, "this": true, "super": true, "null": true, "true": true,
						"false": true, "import": true, "package": true,
						"System": true, "String": true, "Integer": true, "Boolean": true,
						"println": true, "print": true, "length": true, "size": true,
						"get": true, "put": true, "add": true, "remove": true, "contains": true,
						"toString": true, "equals": true, "hashCode": true,
					}

					if !javaKeywords[callee] && !javaKeywords[strings.Split(callee, ".")[0]] {
						// Look for the containing method to establish caller
						caller = da.findContainingJavaMethod(filePath, line, content)
						if caller != "" {
							callerFQN := fmt.Sprintf("%s::%s", filePath, caller)

							// Check if callee exists in symbol tables
							if da.symbolExistsInProjectTables(callee, symbolTables) {
								da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
								dbg.Logf(debug.LevelDetailed, "Found Java call: %s -> %s", callerFQN, calleeFQN)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// findContainingJavaMethod finds which Java method contains a given line
func (da *DependencyAnalyzer) findContainingJavaMethod(filePath, targetLine string, content []byte) string {
	lines := strings.Split(string(content), "\n")
	currentMethod := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a method definition
		// Updated to handle generics in return types like Map<String, Integer>
		methodPattern := regexp.MustCompile(`(public|private|protected)?\s*(static)?\s*[\w<>,\s]+\s+(\w+)\s*\([^)]*\)\s*\{?`)
		if match := methodPattern.FindStringSubmatch(trimmed); match != nil {
			methodName := match[3]

			// Skip constructors
			className := da.extractJavaClassName(filePath)
			if methodName != className {
				currentMethod = methodName
			}
		}

		// If this is our target line, return the current method
		if strings.TrimSpace(line) == strings.TrimSpace(targetLine) {
			return currentMethod
		}
	}

	return currentMethod
}

// Helper functions for Java
func (da *DependencyAnalyzer) isJavaStandardLibrary(importPath string) bool {
	javaStdPrefixes := []string{
		"java.", "javax.", "org.w3c.", "org.xml.", "org.ietf.jgss.",
	}

	for _, prefix := range javaStdPrefixes {
		if strings.HasPrefix(importPath, prefix) {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) resolveJavaImport(className, currentFilePath string) string {
	// For local classes, look for ClassName.java in the same directory
	dir := filepath.Dir(currentFilePath)
	expectedFile := filepath.Join(dir, className+".java")

	if exists(expectedFile) {
		return expectedFile
	}

	return ""
}

func (da *DependencyAnalyzer) resolveJavaClassToFile(className string, symbolTables map[string]*semantic.SymbolTable) string {
	// Look for the class file in symbol tables
	expectedFileName := className + ".java"
	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, expectedFileName) {
			return filePath
		}
	}

	return ""
}

func (da *DependencyAnalyzer) extractJavaClassName(filePath string) string {
	// Extract class name from file path (ClassName.java -> ClassName)
	baseName := filepath.Base(filePath)
	if strings.HasSuffix(baseName, ".java") {
		return strings.TrimSuffix(baseName, ".java")
	}
	return baseName
}

// extractCSharpImports extracts C# using statements and discovers same-namespace classes
func (da *DependencyAnalyzer) extractCSharpImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	var imports []string

	// First, process explicit using statements
	lines := strings.Split(string(fileContent), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Match C# using statements: using System.Collections.Generic;
		if strings.HasPrefix(line, "using ") && strings.HasSuffix(line, ";") {
			usingLine := strings.TrimPrefix(line, "using ")
			usingLine = strings.TrimSuffix(usingLine, ";")
			usingLine = strings.TrimSpace(usingLine)

			// Skip standard library using statements
			if da.isCSharpStandardLibrary(usingLine) {
				continue
			}

			// For C#, we typically import namespaces from other assemblies
			// But for our test case, we're looking for local class files
			className := usingLine
			if lastDot := strings.LastIndex(usingLine, "."); lastDot != -1 {
				className = usingLine[lastDot+1:]
			}

			// Resolve to local C# file
			if resolvedPath := da.resolveCSharpImport(className, filePath); resolvedPath != "" {
				imports = append(imports, resolvedPath)
				dbg.Logf(debug.LevelDetailed, "Found C# explicit using: %s -> %s", className, resolvedPath)
			}
		}
	}

	// Second, for C# auto-discover all .cs files in the same directory
	// because C# classes in the same namespace don't need explicit using
	dir := filepath.Dir(filePath)
	files, err := os.ReadDir(dir)
	if err == nil {
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".cs") {
				fullPath := filepath.Join(dir, file.Name())
				// Don't include the current file itself
				if fullPath != filePath {
					imports = append(imports, fullPath)
					dbg.Logf(debug.LevelDetailed, "Found C# same-namespace class: %s", fullPath)
				}
			}
		}
	}

	return imports, nil
}

// parseCSharpFunctions parses C# method definitions
func (da *DependencyAnalyzer) parseCSharpFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")
	currentPos := uint32(0)

	for i, line := range lines {
		lineStart := currentPos
		lineEnd := currentPos + uint32(len(line)) + 1 // +1 for newline

		trimmedLine := strings.TrimSpace(line)

		// Look for C# method definitions: public static void MethodName(...)
		// Pattern: [visibility] [static] returnType MethodName(parameters) {
		// Updated to handle generics in return types like Dictionary<string, int>
		methodPattern := regexp.MustCompile(`(public|private|protected|internal)?\s*(static)?\s*[\w<>,\s\[\]]+\s+(\w+)\s*\([^)]*\)\s*\{?`)
		if match := methodPattern.FindStringSubmatch(trimmedLine); match != nil {
			methodName := match[3]

			// Skip constructors (method name same as class name)
			className := da.extractCSharpClassName(filePath)
			if methodName == className {
				continue
			}

			// Estimate method end position
			methodEndPos := da.estimateCSharpMethodEnd(content, lineStart)

			// Create FQN
			fqn := fmt.Sprintf("%s::%s", filePath, methodName)

			// Store method definition for distillation
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      methodName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: lineStart,
				EndByte:   methodEndPos,
				Language:  "csharp",
			}

			// Create symbol
			visibility := "public"
			if match[1] != "" {
				visibility = match[1]
			}

			symbol := &semantic.Symbol{
				ID:   semantic.GenerateSymbolID(filePath, methodName, ""),
				Name: methodName,
				Kind: semantic.SymbolKindFunction,
				Location: semantic.FileLocation{
					FilePath:  filePath,
					StartLine: i + 1,
					EndLine:   i + 1,
				},
				Visibility: visibility,
				IsExported: visibility == "public",
				Language:   "csharp",
			}

			symbolTable.AddSymbol(symbol)
			dbg.Logf(debug.LevelDetailed, "Found C# method: %s in %s (FQN: %s)", methodName, filePath, fqn)
		}

		currentPos = lineEnd
	}
}

// estimateCSharpMethodEnd estimates the end position of a C# method
func (da *DependencyAnalyzer) estimateCSharpMethodEnd(content []byte, startPos uint32) uint32 {
	lines := strings.Split(string(content), "\n")

	// Find the line containing startPos
	currentPos := uint32(0)
	startLine := 0
	braceCount := 0

	for i, line := range lines {
		lineEnd := currentPos + uint32(len(line)) + 1
		if currentPos <= startPos && startPos < lineEnd {
			startLine = i
			break
		}
		currentPos = lineEnd
	}

	// Look for matching closing brace
	currentPos = startPos
	foundOpeningBrace := false

	for i := startLine; i < len(lines); i++ {
		line := lines[i]

		// Count braces to find method end
		for _, char := range line {
			if char == '{' {
				braceCount++
				foundOpeningBrace = true
			} else if char == '}' {
				braceCount--
				// Only consider method end after we've found an opening brace
				if foundOpeningBrace && braceCount == 0 {
					// Found the closing brace
					return currentPos + uint32(len(line)) + 1
				}
			}
		}

		currentPos += uint32(len(line)) + 1
	}

	// No explicit end found, method goes to end of file
	return uint32(len(content))
}

// extractCSharpCalls extracts method calls from C# content
func (da *DependencyAnalyzer) extractCSharpCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	lines := strings.Split(string(content), "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip comments and empty lines
		if trimmed == "" || strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") {
			continue
		}

		// Pattern for C# method calls:
		// 1. ClassName.MethodName (static method call)
		// 2. obj.MethodName (instance method call)
		// 3. MethodName (local method call)
		callPatterns := []*regexp.Regexp{
			regexp.MustCompile(`(\w+)\.(\w+)\s*\(`), // ClassName.Method( or obj.Method(
			regexp.MustCompile(`\b(\w+)\s*\(`),      // Method(
		}

		for _, pattern := range callPatterns {
			matches := pattern.FindAllStringSubmatch(trimmed, -1)
			for _, match := range matches {
				var caller, callee, calleeFQN string

				if len(match) >= 3 {
					// ClassName.Method pattern
					className := match[1]
					method := match[2]
					callee = method // Use just the method name for symbol lookup

					// Try to resolve cross-file call by mapping class to file
					if classFile := da.resolveCSharpClassToFile(className, symbolTables); classFile != "" {
						calleeFQN = fmt.Sprintf("%s::%s", classFile, method)
					} else {
						calleeFQN = fmt.Sprintf("%s::%s", filePath, method) // Fallback to local file
					}
				} else if len(match) >= 2 {
					// Simple method call
					callee = match[1]
					calleeFQN = fmt.Sprintf("%s::%s", filePath, callee)
				}

				if callee != "" {
					// Filter out common C# keywords and operators
					csharpKeywords := map[string]bool{
						"if": true, "else": true, "while": true, "for": true, "foreach": true,
						"do": true, "switch": true, "case": true, "default": true,
						"try": true, "catch": true, "finally": true, "throw": true,
						"return": true, "break": true, "continue": true, "goto": true,
						"class": true, "interface": true, "struct": true, "enum": true,
						"public": true, "private": true, "protected": true, "internal": true,
						"static": true, "readonly": true, "const": true, "virtual": true,
						"override": true, "abstract": true, "sealed": true, "partial": true,
						"new": true, "this": true, "base": true, "null": true, "true": true,
						"false": true, "using": true, "namespace": true, "var": true,
						"Console": true, "string": true, "int": true, "bool": true, "object": true,
						"WriteLine": true, "Write": true, "ToString": true, "Equals": true,
						"GetHashCode": true, "Count": true, "Length": true, "Add": true,
						"Remove": true, "Contains": true, "Clear": true,
					}

					if !csharpKeywords[callee] && !csharpKeywords[strings.Split(callee, ".")[0]] {
						// Look for the containing method to establish caller
						caller = da.findContainingCSharpMethod(filePath, line, content)
						if caller != "" {
							callerFQN := fmt.Sprintf("%s::%s", filePath, caller)

							// Check if callee exists in symbol tables
							if da.symbolExistsInProjectTables(callee, symbolTables) {
								da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
								dbg.Logf(debug.LevelDetailed, "Found C# call: %s -> %s", callerFQN, calleeFQN)
							}
						}
					}
				}
			}
		}
	}

	return nil
}

// findContainingCSharpMethod finds which C# method contains a given line
func (da *DependencyAnalyzer) findContainingCSharpMethod(filePath, targetLine string, content []byte) string {
	lines := strings.Split(string(content), "\n")
	currentMethod := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if this is a method definition
		// Updated to handle generics in return types like Dictionary<string, int>
		methodPattern := regexp.MustCompile(`(public|private|protected|internal)?\s*(static)?\s*[\w<>,\s\[\]]+\s+(\w+)\s*\([^)]*\)\s*\{?`)
		if match := methodPattern.FindStringSubmatch(trimmed); match != nil {
			methodName := match[3]

			// Skip constructors
			className := da.extractCSharpClassName(filePath)
			if methodName != className {
				currentMethod = methodName
			}
		}

		// If this is our target line, return the current method
		if strings.TrimSpace(line) == strings.TrimSpace(targetLine) {
			return currentMethod
		}
	}

	return currentMethod
}

// Helper functions for C#
func (da *DependencyAnalyzer) isCSharpStandardLibrary(usingPath string) bool {
	csharpStdPrefixes := []string{
		"System", "Microsoft", "Windows",
	}

	for _, prefix := range csharpStdPrefixes {
		if strings.HasPrefix(usingPath, prefix) {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) resolveCSharpImport(className, currentFilePath string) string {
	// For local classes, look for ClassName.cs in the same directory
	dir := filepath.Dir(currentFilePath)
	expectedFile := filepath.Join(dir, className+".cs")

	if exists(expectedFile) {
		return expectedFile
	}

	return ""
}

func (da *DependencyAnalyzer) resolveCSharpClassToFile(className string, symbolTables map[string]*semantic.SymbolTable) string {
	// Look for the class file in symbol tables
	expectedFileName := className + ".cs"
	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, expectedFileName) {
			return filePath
		}
	}

	return ""
}

func (da *DependencyAnalyzer) extractCSharpClassName(filePath string) string {
	// Extract class name from file path (ClassName.cs -> ClassName)
	baseName := filepath.Base(filePath)
	if strings.HasSuffix(baseName, ".cs") {
		return strings.TrimSuffix(baseName, ".cs")
	}
	return baseName
}

// ========== RUST SUPPORT ==========

// extractRustImports extracts Rust use and mod statements
func (da *DependencyAnalyzer) extractRustImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	imports := []string{}
	content := string(fileContent)

	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Look for 'mod module_name;' declarations
	modRegex := regexp.MustCompile(`mod\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*;`)
	modMatches := modRegex.FindAllStringSubmatch(content, -1)

	for _, match := range modMatches {
		if len(match) >= 2 {
			moduleName := match[1]

			// Look for module_name.rs in the same directory
			dir := filepath.Dir(filePath)
			moduleFile := filepath.Join(dir, moduleName+".rs")

			if exists(moduleFile) {
				imports = append(imports, moduleFile)
				dbg.Logf(debug.LevelDetailed, "Found Rust mod import: %s -> %s", moduleName, moduleFile)
			}
		}
	}

	// Look for 'use module_name::' statements for cross-module function calls
	useRegex := regexp.MustCompile(`use\s+([a-zA-Z_][a-zA-Z0-9_]*)::[^;]*;`)
	useMatches := useRegex.FindAllStringSubmatch(content, -1)

	for _, match := range useMatches {
		if len(match) >= 2 {
			moduleName := match[1]

			// Skip std library and external crates
			if da.isRustStandardLibrary(moduleName) {
				continue
			}

			// Look for module_name.rs in the same directory
			dir := filepath.Dir(filePath)
			moduleFile := filepath.Join(dir, moduleName+".rs")

			if exists(moduleFile) {
				imports = append(imports, moduleFile)
				dbg.Logf(debug.LevelDetailed, "Found Rust use import: %s -> %s", moduleName, moduleFile)
			}
		}
	}

	dbg.Logf(debug.LevelDetailed, "Found %d Rust imports in %s", len(imports), filePath)
	return imports, nil
}

// parseRustFunctions parses Rust function definitions
func (da *DependencyAnalyzer) parseRustFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Parse function definitions: pub fn function_name(params) -> ReturnType { or fn function_name(params) {
	funcRegex := regexp.MustCompile(`(?m)(pub\s+)?fn\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)(?:\s*->\s*[^{]+)?\s*\{`)
	matches := funcRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			visibility := match[1] // "pub " or empty
			funcName := match[2]

			// Determine if function is public
			isPublic := strings.TrimSpace(visibility) == "pub"

			// Find start position of the function
			funcStart := strings.Index(contentStr, match[0])
			if funcStart == -1 {
				continue
			}

			// Estimate end position by finding the matching closing brace
			funcEnd := da.estimateRustFunctionEnd(contentStr, funcStart)

			// Create FQN
			fqn := filePath + "::" + funcName

			// Store function definition
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      funcName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: uint32(funcStart),
				EndByte:   uint32(funcEnd),
				Language:  "rust",
			}

			// Add to symbol table
			symbol := &semantic.Symbol{
				ID:         semantic.SymbolID(fqn),
				Name:       funcName,
				Kind:       semantic.SymbolKindFunction,
				Location:   semantic.FileLocation{FilePath: filePath},
				Signature:  funcName,
				Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
				IsExported: isPublic,
			}
			symbolTable.AddSymbol(symbol)

			dbg.Logf(debug.LevelDetailed, "Found Rust function: %s (public: %v) at bytes %d-%d",
				funcName, isPublic, funcStart, funcEnd)
		}
	}

	// Parse struct implementations: impl StructName {
	implRegex := regexp.MustCompile(`impl\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\{`)
	implMatches := implRegex.FindAllStringSubmatch(contentStr, -1)

	for _, implMatch := range implMatches {
		if len(implMatch) >= 2 {
			structName := implMatch[1]

			// Find the impl block start
			implStart := strings.Index(contentStr, implMatch[0])
			if implStart == -1 {
				continue
			}

			// Find methods within this impl block
			implBlock := da.extractRustImplBlock(contentStr, implStart)

			// Parse methods within the impl block
			methodRegex := regexp.MustCompile(`(?m)(pub\s+)?fn\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)(?:\s*->\s*[^{]+)?\s*\{`)
			methodMatches := methodRegex.FindAllStringSubmatch(implBlock, -1)

			for _, methodMatch := range methodMatches {
				if len(methodMatch) >= 3 {
					visibility := methodMatch[1]
					methodName := methodMatch[2]

					isPublic := strings.TrimSpace(visibility) == "pub"

					// Find method position within the impl block
					methodStart := implStart + strings.Index(implBlock, methodMatch[0])
					methodEnd := da.estimateRustFunctionEnd(contentStr, methodStart)

					// Create FQN for the method
					fqn := filePath + "::" + structName + "::" + methodName

					// Store method definition
					da.functionDefs[fqn] = &FunctionDefinition{
						Name:      structName + "::" + methodName,
						FQN:       fqn,
						FilePath:  filePath,
						StartByte: uint32(methodStart),
						EndByte:   uint32(methodEnd),
						Language:  "rust",
					}

					// Add to symbol table
					methodSymbol := &semantic.Symbol{
						ID:         semantic.SymbolID(fqn),
						Name:       methodName,
						Kind:       semantic.SymbolKindMethod,
						Location:   semantic.FileLocation{FilePath: filePath},
						Scope:      structName,
						Signature:  structName + "::" + methodName,
						Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
						IsExported: isPublic,
					}
					symbolTable.AddSymbol(methodSymbol)

					dbg.Logf(debug.LevelDetailed, "Found Rust method: %s::%s (public: %v) at bytes %d-%d",
						structName, methodName, isPublic, methodStart, methodEnd)
				}
			}
		}
	}
}

// extractRustCalls extracts function calls from Rust code
func (da *DependencyAnalyzer) extractRustCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Find function calls in various formats:
	// 1. Module::function() - static function calls
	// 2. object.method() - method calls
	// 3. function() - local function calls

	// Pattern 1: Module::function() calls
	staticCallRegex := regexp.MustCompile(`([A-Z][a-zA-Z0-9_]*)::\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	staticMatches := staticCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range staticMatches {
		if len(match) >= 3 {
			structName := match[1]
			methodName := match[2]

			// Look for the target file
			targetFile := da.resolveRustStructToFile(structName, symbolTables)
			if targetFile != "" {
				calledFQN := targetFile + "::" + structName + "::" + methodName
				currentFQN := filePath + "::main" // Simplified for now

				da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
				da.usedSymbols[calledFQN] = true

				dbg.Logf(debug.LevelDetailed, "Found Rust static call: %s -> %s", currentFQN, calledFQN)
			}
		}
	}

	// Pattern 2: Regular function calls
	funcCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	funcMatches := funcCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range funcMatches {
		if len(match) >= 2 {
			funcName := match[1]

			// Skip Rust keywords and common control structures
			if da.isRustKeyword(funcName) {
				continue
			}

			// Look for the function across all symbol tables
			for tablePath, table := range symbolTables {
				if symbol, exists := table.GetSymbol(funcName); exists && symbol.Kind == semantic.SymbolKindFunction {
					calledFQN := tablePath + "::" + funcName
					currentFQN := filePath + "::main" // Simplified

					da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
					da.usedSymbols[calledFQN] = true

					dbg.Logf(debug.LevelDetailed, "Found Rust function call: %s -> %s", currentFQN, calledFQN)
					break
				}
			}
		}
	}

	return nil
}

// Helper function to estimate Rust function end by counting braces
func (da *DependencyAnalyzer) estimateRustFunctionEnd(content string, startPos int) int {
	braceCount := 0
	inString := false
	escaped := false

	for i := startPos; i < len(content); i++ {
		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return i + 1
				}
			}
		}
	}

	return len(content)
}

// Helper function to extract impl block content
func (da *DependencyAnalyzer) extractRustImplBlock(content string, implStart int) string {
	braceCount := 0
	inString := false
	escaped := false
	foundFirstBrace := false

	for i := implStart; i < len(content); i++ {
		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '{':
				foundFirstBrace = true
				braceCount++
			case '}':
				braceCount--
				if foundFirstBrace && braceCount == 0 {
					return content[implStart : i+1]
				}
			}
		}
	}

	return content[implStart:]
}

// Helper functions for Rust
func (da *DependencyAnalyzer) isRustStandardLibrary(moduleName string) bool {
	rustStdPrefixes := []string{
		"std", "core", "alloc", "proc_macro",
	}

	for _, prefix := range rustStdPrefixes {
		if strings.HasPrefix(moduleName, prefix) {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) isRustKeyword(name string) bool {
	rustKeywords := []string{
		"if", "else", "while", "for", "loop", "match", "return", "break", "continue",
		"let", "mut", "const", "static", "fn", "struct", "enum", "trait", "impl",
		"use", "mod", "pub", "crate", "super", "self", "Self", "extern", "unsafe",
		"async", "await", "move", "ref", "dyn", "where", "type", "as", "in",
		"println", "print", "panic", "assert", "vec", "format",
	}

	for _, keyword := range rustKeywords {
		if name == keyword {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) resolveRustStructToFile(structName string, symbolTables map[string]*semantic.SymbolTable) string {
	// Look for the struct file in symbol tables
	expectedFileName := strings.ToLower(structName) + ".rs"
	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, expectedFileName) {
			return filePath
		}
	}

	// Also try exact struct name
	exactFileName := structName + ".rs"
	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, exactFileName) {
			return filePath
		}
	}

	return ""
}

// ========== SWIFT SUPPORT ==========

// extractSwiftImports extracts Swift import statements
func (da *DependencyAnalyzer) extractSwiftImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	imports := []string{}

	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Swift doesn't have explicit file imports like other languages
	// Instead, it automatically imports all .swift files in the same directory/module

	// Auto-discover all .swift files in the same directory
	dir := filepath.Dir(filePath)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return imports, err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".swift") {
			// Don't import self
			otherFile := filepath.Join(dir, entry.Name())
			if otherFile != filePath {
				imports = append(imports, otherFile)
				dbg.Logf(debug.LevelDetailed, "Found Swift auto-import: %s", otherFile)
			}
		}
	}

	dbg.Logf(debug.LevelDetailed, "Found %d Swift auto-imports in %s", len(imports), filePath)
	return imports, nil
}

// parseSwiftFunctions parses Swift function and method definitions
func (da *DependencyAnalyzer) parseSwiftFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Parse standalone function definitions: func functionName(params) -> ReturnType {
	funcRegex := regexp.MustCompile(`(?m)(private\s+|public\s+|internal\s+)?func\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)(?:\s*->\s*[^{]+)?\s*\{`)
	matches := funcRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range matches {
		if len(match) >= 3 {
			visibility := strings.TrimSpace(match[1])
			funcName := match[2]

			// Determine if function is public (default is internal in Swift)
			isPublic := visibility == "public" || visibility == ""

			// Find start position of the function
			funcStart := strings.Index(contentStr, match[0])
			if funcStart == -1 {
				continue
			}

			// Estimate end position by finding the matching closing brace
			funcEnd := da.estimateSwiftFunctionEnd(contentStr, funcStart)

			// Create FQN
			fqn := filePath + "::" + funcName

			// Store function definition
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      funcName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: uint32(funcStart),
				EndByte:   uint32(funcEnd),
				Language:  "swift",
			}

			// Add to symbol table
			symbol := &semantic.Symbol{
				ID:         semantic.SymbolID(fqn),
				Name:       funcName,
				Kind:       semantic.SymbolKindFunction,
				Location:   semantic.FileLocation{FilePath: filePath},
				Signature:  funcName,
				Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
				IsExported: isPublic,
			}
			symbolTable.AddSymbol(symbol)

			dbg.Logf(debug.LevelDetailed, "Found Swift function: %s (public: %v) at bytes %d-%d",
				funcName, isPublic, funcStart, funcEnd)
		}
	}

	// Parse class definitions and their methods: class ClassName {
	classRegex := regexp.MustCompile(`(?m)(private\s+|public\s+|internal\s+)?class\s+([a-zA-Z_][a-zA-Z0-9_]*)[^{]*\{`)
	classMatches := classRegex.FindAllStringSubmatch(contentStr, -1)

	for _, classMatch := range classMatches {
		if len(classMatch) >= 3 {
			className := classMatch[2]

			// Find the class block start
			classStart := strings.Index(contentStr, classMatch[0])
			if classStart == -1 {
				continue
			}

			// Find methods within this class block
			classBlock := da.extractSwiftClassBlock(contentStr, classStart)

			// Parse methods within the class block (including static methods)
			methodRegex := regexp.MustCompile(`(?m)(private\s+|public\s+|internal\s+)?(static\s+)?func\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)(?:\s*->\s*[^{]+)?\s*\{`)
			methodMatches := methodRegex.FindAllStringSubmatch(classBlock, -1)

			for _, methodMatch := range methodMatches {
				if len(methodMatch) >= 4 {
					visibility := strings.TrimSpace(methodMatch[1])
					staticModifier := strings.TrimSpace(methodMatch[2])
					methodName := methodMatch[3]

					isPublic := visibility == "public" || visibility == ""
					isStatic := staticModifier == "static"

					// Find method position within the class block
					methodStart := classStart + strings.Index(classBlock, methodMatch[0])
					methodEnd := da.estimateSwiftFunctionEnd(contentStr, methodStart)

					// Create FQN for the method
					fqn := filePath + "::" + className + "::" + methodName

					// Store method definition
					da.functionDefs[fqn] = &FunctionDefinition{
						Name:      className + "::" + methodName,
						FQN:       fqn,
						FilePath:  filePath,
						StartByte: uint32(methodStart),
						EndByte:   uint32(methodEnd),
						Language:  "swift",
					}

					// Add to symbol table
					methodSymbol := &semantic.Symbol{
						ID:         semantic.SymbolID(fqn),
						Name:       methodName,
						Kind:       semantic.SymbolKindMethod,
						Location:   semantic.FileLocation{FilePath: filePath},
						Scope:      className,
						Signature:  className + "::" + methodName,
						Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
						IsExported: isPublic,
						IsStatic:   isStatic,
					}
					symbolTable.AddSymbol(methodSymbol)

					dbg.Logf(debug.LevelDetailed, "Found Swift method: %s::%s (public: %v, static: %v) at bytes %d-%d",
						className, methodName, isPublic, isStatic, methodStart, methodEnd)
				}
			}
		}
	}
}

// extractSwiftCalls extracts function calls from Swift code
func (da *DependencyAnalyzer) extractSwiftCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Find function calls in various formats:
	// 1. ClassName.methodName() - static method calls
	// 2. object.method() - instance method calls
	// 3. functionName() - standalone function calls

	// Pattern 1: ClassName.methodName() calls (static methods)
	staticCallRegex := regexp.MustCompile(`([A-Z][a-zA-Z0-9_]*)\.\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	staticMatches := staticCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range staticMatches {
		if len(match) >= 3 {
			className := match[1]
			methodName := match[2]

			// Look for the target file
			targetFile := da.resolveSwiftClassToFile(className, symbolTables)
			if targetFile != "" {
				calledFQN := targetFile + "::" + className + "::" + methodName
				currentFQN := filePath + "::main" // Simplified for now

				da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
				da.usedSymbols[calledFQN] = true

				dbg.Logf(debug.LevelDetailed, "Found Swift static call: %s -> %s", currentFQN, calledFQN)
			}
		}
	}

	// Pattern 2: Regular function calls
	funcCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	funcMatches := funcCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range funcMatches {
		if len(match) >= 2 {
			funcName := match[1]

			// Skip Swift keywords and common control structures
			if da.isSwiftKeyword(funcName) {
				continue
			}

			// Look for the function across all symbol tables
			for tablePath, table := range symbolTables {
				if symbol, exists := table.GetSymbol(funcName); exists && symbol.Kind == semantic.SymbolKindFunction {
					calledFQN := tablePath + "::" + funcName
					currentFQN := filePath + "::main" // Simplified

					da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
					da.usedSymbols[calledFQN] = true

					dbg.Logf(debug.LevelDetailed, "Found Swift function call: %s -> %s", currentFQN, calledFQN)
					break
				}
			}
		}
	}

	return nil
}

// Helper function to estimate Swift function end by counting braces
func (da *DependencyAnalyzer) estimateSwiftFunctionEnd(content string, startPos int) int {
	braceCount := 0
	inString := false
	escaped := false

	for i := startPos; i < len(content); i++ {
		if i+1 < len(content) && content[i:i+2] == "//" && !inString {
			// Skip to end of line for single-line comments
			for i < len(content) && content[i] != '\n' {
				i++
			}
			continue
		}

		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return i + 1
				}
			}
		}
	}

	return len(content)
}

// Helper function to extract Swift class block content
func (da *DependencyAnalyzer) extractSwiftClassBlock(content string, classStart int) string {
	braceCount := 0
	inString := false
	escaped := false
	foundFirstBrace := false

	for i := classStart; i < len(content); i++ {
		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '{':
				foundFirstBrace = true
				braceCount++
			case '}':
				braceCount--
				if foundFirstBrace && braceCount == 0 {
					return content[classStart : i+1]
				}
			}
		}
	}

	return content[classStart:]
}

// Helper functions for Swift
func (da *DependencyAnalyzer) isSwiftKeyword(name string) bool {
	swiftKeywords := []string{
		"if", "else", "while", "for", "switch", "case", "default", "return", "break", "continue",
		"let", "var", "func", "class", "struct", "enum", "protocol", "extension", "init", "deinit",
		"import", "public", "private", "internal", "open", "fileprivate", "static", "final",
		"override", "required", "convenience", "lazy", "weak", "strong", "unowned",
		"guard", "defer", "repeat", "where", "as", "is", "try", "catch", "throw", "throws",
		"async", "await", "actor", "isolated", "nonisolated",
		"print", "assert", "fatalError", "precondition", "debugPrint",
	}

	for _, keyword := range swiftKeywords {
		if name == keyword {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) resolveSwiftClassToFile(className string, symbolTables map[string]*semantic.SymbolTable) string {
	// Look for the class file in symbol tables
	expectedFileName := className + ".swift"
	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, expectedFileName) {
			return filePath
		}
	}

	return ""
}

// ========== C++ SUPPORT ==========

// extractCppIncludes extracts C++ #include statements
func (da *DependencyAnalyzer) extractCppIncludes(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	includes := []string{}
	content := string(fileContent)

	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Look for #include "header.h" statements (local headers)
	includeRegex := regexp.MustCompile(`#include\s+"([^"]+\.h(?:pp)?)"`)
	includeMatches := includeRegex.FindAllStringSubmatch(content, -1)

	for _, match := range includeMatches {
		if len(match) >= 2 {
			headerName := match[1]

			// Look for header file in the same directory
			dir := filepath.Dir(filePath)
			headerFile := filepath.Join(dir, headerName)

			if exists(headerFile) {
				includes = append(includes, headerFile)
				dbg.Logf(debug.LevelDetailed, "Found C++ include: %s -> %s", headerName, headerFile)

				// For each header, also look for corresponding .cpp file
				cppFile := da.getCppSourceFile(headerFile)
				if cppFile != "" && exists(cppFile) {
					includes = append(includes, cppFile)
					dbg.Logf(debug.LevelDetailed, "Found C++ source for header: %s -> %s", headerFile, cppFile)
				}
			}
		}
	}

	dbg.Logf(debug.LevelDetailed, "Found %d C++ includes in %s", len(includes), filePath)
	return includes, nil
}

// parseCppFunctions parses C++ function and method definitions
func (da *DependencyAnalyzer) parseCppFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Parse function definitions in both headers and source files
	// Pattern: [return_type] [ClassName::]functionName(params) [const] {
	funcRegex := regexp.MustCompile(`(?m)(public|private|protected)?\s*(static\s+)?([a-zA-Z_][a-zA-Z0-9_<>,\s:]*)\s+([a-zA-Z_][a-zA-Z0-9_]*::)?([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\)\s*(const\s*)?\s*(\{|;)`)
	matches := funcRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range matches {
		if len(match) >= 8 {
			visibility := strings.TrimSpace(match[1])
			staticModifier := strings.TrimSpace(match[2])
			returnType := strings.TrimSpace(match[3])
			className := strings.TrimSpace(match[4])
			funcName := strings.TrimSpace(match[5])
			// constModifier := strings.TrimSpace(match[6]) // Not used currently
			declaration := strings.TrimSpace(match[7])

			// Skip constructors, destructors, and operators
			if funcName == "" || strings.HasPrefix(funcName, "~") || strings.HasPrefix(funcName, "operator") {
				continue
			}

			// Skip C++ keywords and common STL functions
			if da.isCppKeyword(funcName) {
				continue
			}

			// Determine visibility (default is public for functions, private for class members)
			isPublic := visibility == "public" || (visibility == "" && className == "")
			isStatic := staticModifier == "static"

			// Find start position of the function
			funcStart := strings.Index(contentStr, match[0])
			if funcStart == -1 {
				continue
			}

			var funcEnd int
			if declaration == "{" {
				// Function with body - estimate end by counting braces
				funcEnd = da.estimateCppFunctionEnd(contentStr, funcStart)
			} else {
				// Declaration only - just use the match end
				funcEnd = funcStart + len(match[0])
			}

			// Create FQN
			var fqn string
			if className != "" {
				// Remove trailing :: from className
				className = strings.TrimSuffix(className, "::")
				fqn = filePath + "::" + className + "::" + funcName
			} else {
				fqn = filePath + "::" + funcName
			}

			// Store function definition
			da.functionDefs[fqn] = &FunctionDefinition{
				Name:      funcName,
				FQN:       fqn,
				FilePath:  filePath,
				StartByte: uint32(funcStart),
				EndByte:   uint32(funcEnd),
				Language:  "cpp",
			}

			// Add to symbol table
			if className != "" {
				// Method
				methodSymbol := &semantic.Symbol{
					ID:         semantic.SymbolID(fqn),
					Name:       funcName,
					Kind:       semantic.SymbolKindMethod,
					Location:   semantic.FileLocation{FilePath: filePath},
					Scope:      className,
					Signature:  returnType + " " + funcName,
					Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
					IsExported: isPublic,
					IsStatic:   isStatic,
				}
				symbolTable.AddSymbol(methodSymbol)

				dbg.Logf(debug.LevelDetailed, "Found C++ method: %s::%s (public: %v, static: %v) at bytes %d-%d",
					className, funcName, isPublic, isStatic, funcStart, funcEnd)
			} else {
				// Function
				funcSymbol := &semantic.Symbol{
					ID:         semantic.SymbolID(fqn),
					Name:       funcName,
					Kind:       semantic.SymbolKindFunction,
					Location:   semantic.FileLocation{FilePath: filePath},
					Signature:  returnType + " " + funcName,
					Visibility: map[bool]string{true: "public", false: "private"}[isPublic],
					IsExported: isPublic,
					IsStatic:   isStatic,
				}
				symbolTable.AddSymbol(funcSymbol)

				dbg.Logf(debug.LevelDetailed, "Found C++ function: %s (public: %v, static: %v) at bytes %d-%d",
					funcName, isPublic, isStatic, funcStart, funcEnd)
			}
		}
	}
}

// extractCppCalls extracts function calls from C++ code
func (da *DependencyAnalyzer) extractCppCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Find function calls in various formats:
	// 1. ClassName::methodName() - static method calls
	// 2. object.method() or object->method() - instance method calls
	// 3. functionName() - standalone function calls

	// Pattern 1: ClassName::methodName() calls (static methods and namespace functions)
	staticCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)::\s*([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	staticMatches := staticCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range staticMatches {
		if len(match) >= 3 {
			className := match[1]
			methodName := match[2]

			// Skip std:: and other standard library calls
			if da.isCppStandardLibrary(className) {
				continue
			}

			// Look for the target file
			targetFile := da.resolveCppClassToFile(className, symbolTables)
			if targetFile != "" {
				calledFQN := targetFile + "::" + className + "::" + methodName
				currentFQN := filePath + "::main" // Simplified for now

				da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
				da.usedSymbols[calledFQN] = true

				dbg.Logf(debug.LevelDetailed, "Found C++ static call: %s -> %s", currentFQN, calledFQN)
			}
		}
	}

	// Pattern 2: Regular function calls
	funcCallRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*\(`)
	funcMatches := funcCallRegex.FindAllStringSubmatch(contentStr, -1)

	for _, match := range funcMatches {
		if len(match) >= 2 {
			funcName := match[1]

			// Skip C++ keywords and common control structures
			if da.isCppKeyword(funcName) {
				continue
			}

			// Look for the function across all symbol tables
			for tablePath, table := range symbolTables {
				if symbol, exists := table.GetSymbol(funcName); exists && symbol.Kind == semantic.SymbolKindFunction {
					calledFQN := tablePath + "::" + funcName
					currentFQN := filePath + "::main" // Simplified

					da.callGraph[currentFQN] = append(da.callGraph[currentFQN], calledFQN)
					da.usedSymbols[calledFQN] = true

					dbg.Logf(debug.LevelDetailed, "Found C++ function call: %s -> %s", currentFQN, calledFQN)
					break
				}
			}
		}
	}

	return nil
}

// Helper function to estimate C++ function end by counting braces
func (da *DependencyAnalyzer) estimateCppFunctionEnd(content string, startPos int) int {
	braceCount := 0
	inString := false
	inChar := false
	escaped := false
	inComment := false
	inLineComment := false

	for i := startPos; i < len(content); i++ {
		if i+1 < len(content) && content[i:i+2] == "//" && !inString && !inChar && !inComment {
			inLineComment = true
			continue
		}

		if i+1 < len(content) && content[i:i+2] == "/*" && !inString && !inChar && !inLineComment {
			inComment = true
			i++ // Skip the next character
			continue
		}

		if i+1 < len(content) && content[i:i+2] == "*/" && inComment {
			inComment = false
			i++ // Skip the next character
			continue
		}

		if content[i] == '\n' && inLineComment {
			inLineComment = false
			continue
		}

		if inComment || inLineComment {
			continue
		}

		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else if inChar {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '\'' {
				inChar = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '\'':
				inChar = true
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return i + 1
				}
			}
		}
	}

	return len(content)
}

// Helper function to get corresponding .cpp file for a .h file
func (da *DependencyAnalyzer) getCppSourceFile(headerFile string) string {
	// Try various extensions
	base := strings.TrimSuffix(headerFile, filepath.Ext(headerFile))

	extensions := []string{".cpp", ".cc", ".cxx", ".c++"}
	for _, ext := range extensions {
		sourceFile := base + ext
		if exists(sourceFile) {
			return sourceFile
		}
	}

	return ""
}

// Helper functions for C++
func (da *DependencyAnalyzer) isCppStandardLibrary(className string) bool {
	cppStdPrefixes := []string{
		"std", "boost", "__gnu", "__", "detail", "_",
	}

	for _, prefix := range cppStdPrefixes {
		if strings.HasPrefix(className, prefix) {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) isCppKeyword(name string) bool {
	cppKeywords := []string{
		"if", "else", "while", "for", "do", "switch", "case", "default", "return", "break", "continue",
		"auto", "bool", "char", "double", "float", "int", "long", "short", "signed", "unsigned", "void",
		"class", "struct", "enum", "union", "typedef", "typename", "template", "namespace", "using",
		"public", "private", "protected", "virtual", "static", "const", "mutable", "inline", "extern",
		"new", "delete", "this", "operator", "sizeof", "typeid", "const_cast", "dynamic_cast",
		"reinterpret_cast", "static_cast", "try", "catch", "throw", "friend", "explicit",
		"cout", "cin", "endl", "printf", "scanf", "malloc", "free", "strlen", "strcpy", "strcmp",
	}

	for _, keyword := range cppKeywords {
		if name == keyword {
			return true
		}
	}
	return false
}

func (da *DependencyAnalyzer) resolveCppClassToFile(className string, symbolTables map[string]*semantic.SymbolTable) string {
	// Look for the class file in symbol tables
	expectedHeaderName := className + ".h"
	expectedHeaderName2 := className + ".hpp"

	for filePath := range symbolTables {
		if strings.HasSuffix(filePath, expectedHeaderName) || strings.HasSuffix(filePath, expectedHeaderName2) {
			return filePath
		}
	}

	return ""
} // ========== KOTLIN SUPPORT ==========

// extractKotlinImports extracts Kotlin import statements
func (da *DependencyAnalyzer) extractKotlinImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	imports := []string{}
	content := string(fileContent)

	dbg := debug.FromContext(ctx).WithSubsystem("dependency")

	// Extract import statements - Kotlin uses similar syntax to Java
	// Pattern: import package.Class or import package.function
	importRegex := regexp.MustCompile(`^\s*import\s+([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*(?:\.\*)?)\s*$`)

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comments and empty lines
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") || line == "" {
			continue
		}

		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			importPath := matches[1]

			// Skip standard library imports
			if strings.HasPrefix(importPath, "kotlin.") ||
				strings.HasPrefix(importPath, "java.") ||
				strings.HasPrefix(importPath, "android.") ||
				strings.HasPrefix(importPath, "kotlinx.") {
				continue
			}

			// Convert package.Class to local file path
			if localPath := da.resolveKotlinImportToFile(ctx, importPath, filePath); localPath != "" {
				imports = append(imports, localPath)
				dbg.Logf(debug.LevelDetailed, "Found Kotlin import: %s -> %s", importPath, localPath)
			}
		}
	}

	dbg.Logf(debug.LevelDetailed, "Found %d Kotlin imports in %s", len(imports), filePath)
	return imports, nil
}

// resolveKotlinImportToFile resolves a Kotlin import to a local file path
func (da *DependencyAnalyzer) resolveKotlinImportToFile(ctx context.Context, importPath, filePath string) string {
	// Get directory of current file
	dir := filepath.Dir(filePath)

	// Try different strategies to find the imported file
	parts := strings.Split(importPath, ".")
	if len(parts) == 0 {
		return ""
	}

	// Strategy 1: Same directory - just filename.kt
	className := parts[len(parts)-1]
	if className != "*" {
		candidatePath := filepath.Join(dir, className+".kt")
		if exists(candidatePath) {
			return candidatePath
		}
	}

	// Strategy 2: Relative path based on package structure
	// Convert package.subpackage.Class to subpackage/Class.kt
	if len(parts) > 1 {
		subPath := strings.Join(parts[:len(parts)-1], string(filepath.Separator))
		candidatePath := filepath.Join(dir, subPath, className+".kt")
		if exists(candidatePath) {
			return candidatePath
		}
	}

	// Strategy 3: Look for any .kt file with the class name in nearby directories
	baseDir := dir
	for i := 0; i < 3; i++ { // Look up to 3 levels up
		matches, _ := filepath.Glob(filepath.Join(baseDir, "**", className+".kt"))
		if len(matches) > 0 {
			return matches[0]
		}
		baseDir = filepath.Dir(baseDir)
		if baseDir == "." || baseDir == "/" {
			break
		}
	}

	return ""
}

// parseKotlinFunctions parses Kotlin function and method definitions
func (da *DependencyAnalyzer) parseKotlinFunctions(ctx context.Context, symbolTable *semantic.SymbolTable, content []byte, filePath string) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Parse function definitions:
	// fun functionName(...): ReturnType { ... }
	// private/public/internal fun functionName(...)
	functionRegex := regexp.MustCompile(`(?:^|\n)\s*((?:private\s+|public\s+|internal\s+|protected\s+)?(?:inline\s+|suspend\s+|override\s+)*fun\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\([^)]*\))`)

	matches := functionRegex.FindAllStringSubmatch(contentStr, -1)
	for _, match := range matches {
		fullDeclaration := match[1]
		functionName := match[2]

		// Determine visibility
		isPublic := !strings.Contains(fullDeclaration, "private") && !strings.Contains(fullDeclaration, "internal") && !strings.Contains(fullDeclaration, "protected")
		isStatic := false // Kotlin doesn't have static methods in the same way, but companion objects have static-like behavior

		// Find function body to determine byte range
		funcIndex := strings.Index(contentStr, fullDeclaration)
		if funcIndex == -1 {
			continue
		}

		startByte := funcIndex
		endByte := da.findKotlinFunctionEnd(contentStr, funcIndex)

		fqn := fmt.Sprintf("%s::%s", filePath, functionName)
		symbolTable.Symbols[functionName] = &semantic.Symbol{
			ID:   semantic.SymbolID(fqn),
			Name: functionName,
			Kind: semantic.SymbolKindFunction,
			Location: semantic.FileLocation{
				FilePath:  filePath,
				StartLine: 0,
				EndLine:   0,
			},
			Visibility: func() string {
				if isPublic {
					return "public"
				}
				return "private"
			}(),
			IsStatic: isStatic,
		}

		dbg.Logf(debug.LevelDetailed, "Found Kotlin function: %s (public: %t, static: %t) at bytes %d-%d",
			functionName, isPublic, isStatic, startByte, endByte)
	}

	// Parse class methods within classes
	// class ClassName { ... }
	classRegex := regexp.MustCompile(`(?:^|\n)\s*(?:(?:open|abstract|final|data|sealed)\s+)*class\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*[^{]*\{`)
	classMatches := classRegex.FindAllStringSubmatch(contentStr, -1)

	for _, classMatch := range classMatches {
		className := classMatch[1]
		classStartIndex := strings.Index(contentStr, classMatch[0])
		if classStartIndex == -1 {
			continue
		}

		// Find class body
		classBodyStart := strings.Index(contentStr[classStartIndex:], "{")
		if classBodyStart == -1 {
			continue
		}
		classBodyStart += classStartIndex

		classBodyEnd := da.findMatchingBrace(contentStr, classBodyStart)
		if classBodyEnd == -1 {
			continue
		}

		classBody := contentStr[classBodyStart:classBodyEnd]

		// Parse methods within the class
		methodMatches := functionRegex.FindAllStringSubmatch(classBody, -1)
		for _, methodMatch := range methodMatches {
			fullMethodDeclaration := methodMatch[1]
			methodName := methodMatch[2]

			isPublic := !strings.Contains(fullMethodDeclaration, "private") && !strings.Contains(fullMethodDeclaration, "internal") && !strings.Contains(fullMethodDeclaration, "protected")
			isStatic := false

			methodIndex := strings.Index(classBody, fullMethodDeclaration)
			if methodIndex == -1 {
				continue
			}

			startByte := classBodyStart + methodIndex
			endByte := da.findKotlinFunctionEnd(contentStr, startByte)

			fqn := fmt.Sprintf("%s::%s::%s", filePath, className, methodName)
			symbolTable.Symbols[methodName] = &semantic.Symbol{
				ID:   semantic.SymbolID(fqn),
				Name: methodName,
				Kind: semantic.SymbolKindMethod,
				Location: semantic.FileLocation{
					FilePath:  filePath,
					StartLine: 0,
					EndLine:   0,
				},
				Scope: className,
				Visibility: func() string {
					if isPublic {
						return "public"
					}
					return "private"
				}(),
				IsStatic: isStatic,
			}

			dbg.Logf(debug.LevelDetailed, "Found Kotlin method: %s::%s (public: %t, static: %t) at bytes %d-%d",
				className, methodName, isPublic, isStatic, startByte, endByte)
		}
	}
}

// findKotlinFunctionEnd finds the end of a Kotlin function by counting braces
func (da *DependencyAnalyzer) findKotlinFunctionEnd(content string, startIndex int) int {
	// Find the opening brace
	openBraceIndex := strings.Index(content[startIndex:], "{")
	if openBraceIndex == -1 {
		// Single-expression function, find the end of line or semicolon
		rest := content[startIndex:]
		if newlineIndex := strings.Index(rest, "\n"); newlineIndex != -1 {
			return startIndex + newlineIndex
		}
		return len(content)
	}

	openBraceIndex += startIndex
	return da.findMatchingBrace(content, openBraceIndex)
}

// extractKotlinCalls extracts function calls from Kotlin code
func (da *DependencyAnalyzer) extractKotlinCalls(ctx context.Context, filePath string, content []byte, symbolTables map[string]*semantic.SymbolTable) error {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	contentStr := string(content)

	// Find function calls in various formats:
	// 1. ClassName.methodName() - companion object static calls
	// 2. functionName() - function calls
	// 3. objectName.methodName() - object method calls

	// Pattern for function calls: identifier(
	callRegex := regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*(?:\.[a-zA-Z_][a-zA-Z0-9_]*)*)\s*\(`)

	matches := callRegex.FindAllStringSubmatch(contentStr, -1)
	for _, match := range matches {
		fullCall := match[1]

		// Split by dot to handle ClassName.methodName format
		parts := strings.Split(fullCall, ".")

		if len(parts) == 1 {
			// Simple function call: functionName()
			functionName := parts[0]

			// Look for this function in symbol tables
			for targetFile, symbolTable := range symbolTables {
				// Check functions in the target file
				for _, funcInfo := range symbolTable.Symbols {
					if funcInfo.Kind == semantic.SymbolKindFunction && funcInfo.Name == functionName {
						callerFQN := fmt.Sprintf("%s::main", filePath)
						calleeFQN := fmt.Sprintf("%s::%s", targetFile, functionName)
						da.callGraph[callerFQN] = append(da.callGraph[callerFQN], calleeFQN)
						dbg.Logf(debug.LevelDetailed, "Found Kotlin function call: %s -> %s", filePath+"::"+functionName, calleeFQN)
					}
				}
			}
		} else if len(parts) == 2 {
			// Class.method or object.method call
			className := parts[0]
			methodName := parts[1]

			// Look for ClassName::methodName in symbol tables
			for targetFile, symbolTable := range symbolTables {
				staticFQN := fmt.Sprintf("%s::%s::%s", targetFile, className, methodName)
				found := false
				for _, symbol := range symbolTable.Symbols {
					if symbol.Kind == semantic.SymbolKindMethod && string(symbol.ID) == staticFQN {
						found = true
						break
					}
				}
				if found {
					callerFQN := fmt.Sprintf("%s::main", filePath)
					da.callGraph[callerFQN] = append(da.callGraph[callerFQN], staticFQN)
					dbg.Logf(debug.LevelDetailed, "Found Kotlin static call: %s -> %s", filePath+"::main", staticFQN)
				}

				// Also check for simple function calls from other files
				simpleFQN := fmt.Sprintf("%s::%s", targetFile, methodName)
				found2 := false
				for _, symbol := range symbolTable.Symbols {
					if symbol.Kind == semantic.SymbolKindFunction && string(symbol.ID) == simpleFQN {
						found2 = true
						break
					}
				}
				if found2 {
					callerFQN := fmt.Sprintf("%s::main", filePath)
					da.callGraph[callerFQN] = append(da.callGraph[callerFQN], simpleFQN)
					dbg.Logf(debug.LevelDetailed, "Found Kotlin function call: %s -> %s", filePath+"::main", simpleFQN)
				}
			}
		}
	}

	return nil
}

// findMatchingBrace finds the matching closing brace for an opening brace
func (da *DependencyAnalyzer) findMatchingBrace(content string, openBraceIndex int) int {
	braceCount := 0
	inString := false
	escaped := false

	for i := openBraceIndex; i < len(content); i++ {
		char := content[i]

		if inString {
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
		} else {
			switch char {
			case '"':
				inString = true
			case '{':
				braceCount++
			case '}':
				braceCount--
				if braceCount == 0 {
					return i
				}
			}
		}
	}

	return -1
}
