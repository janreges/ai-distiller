package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
)

// The dependency-aware mode (--dependency-aware, experimental) distils the
// entry file, or every file of a directory, and then the project files they
// import: breadth-first, one level of imports at a time, up to
// ProcessOptions.MaxDepth levels. Every followed file gets the distillation it
// would get without the flag. Imports are found per language with regular
// expressions and resolved to files on disk; an import that does not resolve
// to an existing file aid has a processor for is not followed.

// ProcessWithDependencies distils path (a file, or every file a directory run
// selects) and then the files it imports, up to opts.MaxDepth levels. The
// result holds path's own files first, as a run without the flag prints them,
// then the followed files in the order they were found, with paths relative
// to the entry file's directory (or to the directory). followed is the number
// of followed files in the result.
func (p *Processor) ProcessWithDependencies(path string, opts ProcessOptions) (result *ir.DistilledDirectory, followed int, err error) {
	dbg := debug.FromContext(p.ctx).WithSubsystem("dependency")
	if opts.MaxDepth < 1 {
		return nil, 0, fmt.Errorf("--max-depth must be 1 or more with --dependency-aware, got %d", opts.MaxDepth)
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to resolve path: %w", err)
	}
	info, err := os.Stat(absPath)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to stat path: %w", err)
	}

	fileOpts := opts
	fileOpts.SymbolResolution = false

	var starts []string
	baseDir := absPath
	if info.IsDir() {
		tasks, err := p.walkDirectory(absPath, fileOpts)
		if err != nil {
			return nil, 0, err
		}
		result = p.processTasks(absPath, tasks, fileOpts)
		for _, task := range tasks {
			starts = append(starts, task.Path)
		}
	} else {
		baseDir = filepath.Dir(absPath)
		entry, err := p.ProcessFile(absPath, fileOpts)
		if err != nil {
			return nil, 0, err
		}
		result = &ir.DistilledDirectory{Path: baseDir, Children: []ir.DistilledNode{entry}}
		starts = []string{absPath}
	}

	analyzer := NewDependencyAnalyzer(p.ctx)
	deps := analyzer.Follow(starts, opts.MaxDepth)
	dbg.Logf(debug.LevelBasic, "Followed %d files from %s (max depth %d)", len(deps), absPath, opts.MaxDepth)
	for _, dep := range deps {
		file, err := p.ProcessFile(dep, fileOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to process %s: %v\n", dep, err)
			continue
		}
		file.Path = dependencyDisplayPath(dep, baseDir, opts)
		result.Children = append(result.Children, file)
		followed++
	}
	return result, followed, nil
}

// dependencyDisplayPath prints a followed file the way processFile prints the
// entry file: relative to baseDir (with "../" when it lies outside) and with
// --relative-path-prefix, or absolute with --file-path-type=absolute.
func dependencyDisplayPath(file, baseDir string, opts ProcessOptions) string {
	if opts.FilePathType != "relative" || opts.BasePath == "" {
		return file
	}
	rel, err := filepath.Rel(baseDir, file)
	if err != nil {
		return file
	}
	if opts.RelativePathPrefix != "" {
		prefix := opts.RelativePathPrefix
		if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, string(filepath.Separator)) {
			prefix += "/"
		}
		rel = prefix + rel
	}
	return rel
}

// DependencyAnalyzer finds the files a source file imports. It caches the
// project files it reads to resolve imports (go.mod, Cargo.toml,
// tsconfig.json, composer.json, package listings).
type DependencyAnalyzer struct {
	ctx context.Context

	goModules  map[string]*goModule  // go.mod path -> parsed module, nil when unreadable
	goPackages map[string][]goFile   // directory -> its non-test Go files
	rustCrates map[string]*rustCrate // Cargo.toml path -> parsed crate, nil when unreadable
	tsConfigs  map[string]*tsConfig  // tsconfig/jsconfig path -> its effective paths, nil when unreadable
	composers  map[string][]psr4Root // composer.json path -> its PSR-4 roots, longest prefix first, nil when unreadable
}

// NewDependencyAnalyzer returns an analyzer that logs to ctx's debugger.
func NewDependencyAnalyzer(ctx context.Context) *DependencyAnalyzer {
	return &DependencyAnalyzer{
		ctx:        ctx,
		goModules:  map[string]*goModule{},
		goPackages: map[string][]goFile{},
		rustCrates: map[string]*rustCrate{},
		tsConfigs:  map[string]*tsConfig{},
		composers:  map[string][]psr4Root{},
	}
}

// findUp returns the path of the nearest file called name in dir or one of
// its parents, or "" when there is none. It stops after a directory that
// holds .git, the top of a repository.
func findUp(dir, name string) string {
	for {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		if exists(filepath.Join(dir, ".git")) {
			return ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// blockCommentAtLineStart is a /* */ comment that starts a line: a doc or
// license block, not a "/*" inside a string such as a glob.
var blockCommentAtLineStart = regexp.MustCompile(`(?ms)^[ \t]*/\*.*?\*/`)

// stripSlashComments blanks the // and /* */ comments of C-like source that
// start a line, so the examples in doc comments do not count as imports.
func stripSlashComments(code string) string {
	code = blockCommentAtLineStart.ReplaceAllString(code, "")
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "//") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}

// Follow returns the files that starts import, breadth-first up to maxDepth
// levels (1: what starts import directly), each once, without starts
// themselves. Only existing files that aid has a processor for are returned.
func (da *DependencyAnalyzer) Follow(starts []string, maxDepth int) []string {
	dbg := debug.FromContext(da.ctx).WithSubsystem("dependency")
	seen := make(map[string]bool, len(starts))
	for _, s := range starts {
		seen[filepath.Clean(s)] = true
	}
	var followed []string
	frontier := starts
	for depth := 1; depth <= maxDepth && len(frontier) > 0; depth++ {
		var next []string
		for _, file := range frontier {
			for _, dep := range da.Imports(file) {
				dep = filepath.Clean(dep)
				if seen[dep] {
					continue
				}
				seen[dep] = true
				if info, err := os.Stat(dep); err != nil || info.IsDir() {
					continue
				}
				if _, ok := GetByFilename(dep); !ok {
					dbg.Logf(debug.LevelDetailed, "Not following %s: no processor", dep)
					continue
				}
				dbg.Logf(debug.LevelDetailed, "Following %s (depth %d, imported by %s)", dep, depth, file)
				followed = append(followed, dep)
				next = append(next, dep)
			}
		}
		frontier = next
	}
	return followed
}

// Imports returns the files that filePath imports and that resolve to an
// existing path, as absolute paths.
func (da *DependencyAnalyzer) Imports(filePath string) []string {
	dbg := debug.FromContext(da.ctx).WithSubsystem("dependency")
	includes, err := da.extractIncludeStatements(da.ctx, filePath)
	if err != nil {
		dbg.Logf(debug.LevelDetailed, "Failed to extract imports from %s: %v", filePath, err)
		return nil
	}
	var resolved []string
	for _, includePath := range includes {
		if r := da.resolveIncludePath(includePath, filePath); r != "" {
			if abs, err := filepath.Abs(r); err == nil {
				resolved = append(resolved, abs)
			}
		}
	}
	return resolved
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
		return da.extractPhpImports(ctx, fileContent, filePath)
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
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx", ".mts", ".cts":
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

// exists checks if a file or directory exists
func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

// ========== KOTLIN SUPPORT ==========

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
