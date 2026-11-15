package processor

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ignore"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/stripper"
)


// Processor processes files and directories
type Processor struct {
	ctx context.Context
}

// New creates a new processor
func New() *Processor {
	return &Processor{
		ctx: context.Background(),
	}
}

// NewWithContext creates a new processor with a context
func NewWithContext(ctx context.Context) *Processor {
	return &Processor{
		ctx: ctx,
	}
}

// ProcessPath processes a file or directory
func (p *Processor) ProcessPath(path string, opts ProcessOptions) (ir.DistilledNode, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		return p.processDirectory(path, opts)
	}
	return p.ProcessFile(path, opts)
}

// ProcessFile processes a single file
func (p *Processor) ProcessFile(filename string, opts ProcessOptions) (file *ir.DistilledFile, err error) {
	// Record the measured source size on every result, whichever path produced it.
	defer func() {
		if file == nil || err != nil || (file.Metadata != nil && file.Metadata.Size > 0) {
			return
		}
		if info, statErr := os.Stat(filename); statErr == nil {
			if file.Metadata == nil {
				file.Metadata = &ir.FileMetadata{}
			}
			file.Metadata.Size = info.Size()
		}
	}()
	return p.processFile(filename, opts)
}

func (p *Processor) processFile(filename string, opts ProcessOptions) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(p.ctx).WithSubsystem("processor")
	defer dbg.Timing(debug.LevelDetailed, fmt.Sprintf("ProcessFile %s", filepath.Base(filename)))()
	
	dbg.Logf(debug.LevelDetailed, "Processing file: %s", filename)
	
	// Calculate display path based on options
	displayPath := filename
	if opts.FilePathType == "relative" && opts.BasePath != "" {
		// Check if basePath is a file or directory
		baseInfo, err := os.Stat(opts.BasePath)
		var baseDir string
		if err == nil && !baseInfo.IsDir() {
			// If basePath is a file, use its directory
			baseDir = filepath.Dir(opts.BasePath)
		} else {
			// Otherwise use basePath as is
			baseDir = opts.BasePath
		}
		
		// Try to make path relative to base directory
		absBase, err := filepath.Abs(baseDir)
		if err == nil {
			relPath, err := filepath.Rel(absBase, filename)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				displayPath = relPath
				
				// Apply prefix if specified
				if opts.RelativePathPrefix != "" {
					prefix := opts.RelativePathPrefix
					// Ensure prefix ends with separator if not empty and doesn't already
					if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, string(filepath.Separator)) {
						prefix += "/"
					}
					displayPath = prefix + displayPath
				}
			}
		}
	}
	var proc LanguageProcessor
	var ok bool
	
	// In raw mode, use RawProcessor for all text files
	if opts.RawMode {
		rawProc := NewRawProcessor()
		// In raw mode, always use the raw processor
		proc = rawProc
		ok = true
	} else {
		// Normal mode - get processor for file
		proc, ok = GetByFilename(filename)
		
		// If no processor found, check if file is explicitly included
		if !ok && opts.ExplicitInclude {
			// Use RawProcessor for explicitly included files
			rawProc := NewRawProcessor()
			proc = rawProc
			ok = true
		}
	}
	
	if !ok && !opts.RawMode {
		return nil, fmt.Errorf("no processor found for file: %s", filename)
	}
	
	dbg.Logf(debug.LevelDetailed, "Using %s processor for %s", proc.Language(), filename)

	// Open file
	file, err := os.Open(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Process file using our debug-enabled context
	// Check if processor supports ProcessWithOptions
	if procWithOpts, ok := proc.(interface {
		ProcessWithOptions(context.Context, io.Reader, string, ProcessOptions) (*ir.DistilledFile, error)
	}); ok {
		result, err := procWithOpts.ProcessWithOptions(p.ctx, file, displayPath, opts)
		if err != nil {
			return nil, fmt.Errorf("failed to process file: %w", err)
		}
		
		// Dump the IR structure at trace level
		debug.Lazy(p.ctx, debug.LevelTrace, func(d debug.Debugger) {
			d.Dump(debug.LevelTrace, fmt.Sprintf("IR for %s", filepath.Base(filename)), result)
		})
		
		// Apply dependency-aware analysis if enabled
		if opts.SymbolResolution && opts.MaxDepth >= 0 {
			dependencyResult, err := ProcessWithDependencyAnalysis(p.ctx, p, filename, opts)
			if err != nil {
				dbg.Logf(debug.LevelBasic, "Dependency analysis failed, using normal result: %v", err)
				return result, nil
			}
			return dependencyResult, nil
		}
		
		return result, nil
	}
	
	// Fallback to regular Process method
	result, err := proc.Process(p.ctx, file, displayPath)
	if err != nil {
		return nil, fmt.Errorf("failed to process file: %w", err)
	}
	
	// Dump the IR structure at trace level
	debug.Lazy(p.ctx, debug.LevelTrace, func(d debug.Debugger) {
		d.Dump(debug.LevelTrace, fmt.Sprintf("IR for %s", filepath.Base(filename)), result)
	})

	// Apply stripping options (but not in raw mode)
	if !opts.RawMode {
		stripOpts := stripper.Options{
			RemovePrivate:         !opts.IncludePrivate,
			RemovePrivateOnly:     opts.RemovePrivateOnly,
			RemoveProtectedOnly:   opts.RemoveProtectedOnly,
			RemoveImplementations: !opts.IncludeImplementation,
			RemoveComments:        !opts.IncludeComments,
			RemoveImports:         !opts.IncludeImports,
		}

		if stripOpts.RemovePrivate || stripOpts.RemovePrivateOnly || stripOpts.RemoveProtectedOnly || 
		   stripOpts.RemoveImplementations || stripOpts.RemoveComments || stripOpts.RemoveImports {
			dbg.Logf(debug.LevelDetailed, "Applying stripper with options: %+v", stripOpts)
			
			s := stripper.New(stripOpts)
			strippedNode := result.Accept(s)
			if file, ok := strippedNode.(*ir.DistilledFile); ok {
				// Dump stripped result at trace level
				debug.Lazy(p.ctx, debug.LevelTrace, func(d debug.Debugger) {
					d.Dump(debug.LevelTrace, fmt.Sprintf("Stripped IR for %s", filepath.Base(filename)), file)
				})
				
				// Apply dependency-aware analysis if enabled
				if opts.SymbolResolution && opts.MaxDepth >= 0 {
					dependencyResult, err := ProcessWithDependencyAnalysis(p.ctx, p, filename, opts)
					if err != nil {
						dbg.Logf(debug.LevelBasic, "Dependency analysis failed, using normal result: %v", err)
						return file, nil
					}
					return dependencyResult, nil
				}
				
				return file, nil
			}
			return nil, fmt.Errorf("unexpected node type after stripping")
		}
	}

	// Apply dependency-aware analysis if enabled
	if opts.SymbolResolution && opts.MaxDepth >= 0 {
		dependencyResult, err := ProcessWithDependencyAnalysis(p.ctx, p, filename, opts)
		if err != nil {
			dbg.Logf(debug.LevelBasic, "Dependency analysis failed, using normal result: %v", err)
			return result, nil
		}
		return dependencyResult, nil
	}

	return result, nil
}

// Process processes a reader
func (p *Processor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	// Get processor for file
	proc, ok := GetByFilename(filename)
	if !ok {
		return nil, fmt.Errorf("no processor found for file: %s", filename)
	}
	

	return proc.Process(ctx, reader, filename)
}

func (p *Processor) processDirectory(dir string, opts ProcessOptions) (*ir.DistilledDirectory, error) {
	// Use concurrent processing if workers > 1
	if opts.Workers == 0 || opts.Workers > 1 {
		return p.processDirectoryConcurrent(dir, opts)
	}

	// Create ignore matcher for the directory
	ignoreMatcher, ignoreErr := ignore.New(dir)
	if ignoreErr != nil {
		// Log warning but continue without ignore functionality
		fmt.Fprintf(os.Stderr, "Warning: failed to create ignore matcher: %v\n", ignoreErr)
		ignoreMatcher = nil
	}

	// Calculate display path for directory
	displayPath := dir
	if opts.FilePathType == "relative" && opts.BasePath != "" {
		// Try to make path relative to base path
		absBase, err := filepath.Abs(opts.BasePath)
		if err == nil {
			relPath, err := filepath.Rel(absBase, dir)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				displayPath = relPath
				
				// Apply prefix if specified
				if opts.RelativePathPrefix != "" {
					prefix := opts.RelativePathPrefix
					// Ensure prefix ends with separator if not empty and doesn't already
					if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, string(filepath.Separator)) {
						prefix += "/"
					}
					displayPath = prefix + displayPath
				}
			}
		}
	}

	// Serial processing (workers == 1)
	result := &ir.DistilledDirectory{
		BaseNode: ir.BaseNode{},
		Path:     displayPath,
		Children: []ir.DistilledNode{},
	}

	// Walk directory
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if path should be ignored
		if ignoreMatcher != nil && ignoreMatcher.ShouldIgnore(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories
		if info.IsDir() {
			basename := filepath.Base(path)
			
			// Skip .aid directories completely
			if basename == ".aid" {
				return filepath.SkipDir
			}
			
			// Skip default ignored directories unless explicitly included in .aidignore
			// or unless they contain explicitly included files
			if isDefaultIgnoredDir(basename) && ignoreMatcher != nil {
				// Check if directory is explicitly included
				if ignoreMatcher.IsExplicitlyIncluded(path) {
					return nil // Don't skip, process the directory
				}
				// Check if any files within this directory might be explicitly included
				if !ignoreMatcher.MightContainExplicitIncludes(path) {
					return filepath.SkipDir
				}
			} else if isDefaultIgnoredDir(basename) && ignoreMatcher == nil {
				// No .aidignore, skip default ignored dirs
				return filepath.SkipDir
			}
			
			// If not recursive and not the root directory, skip subdirectories
			if !opts.Recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files containing '.aid.' anywhere in filename
		basename := filepath.Base(path)
		if strings.Contains(basename, ".aid.") {
			return nil
		}

		// Check include/exclude patterns
		if !MatchesPatterns(path, opts.IncludePatterns, opts.ExcludePatterns) {
			return nil
		}

		// Check if we can process this file
		if opts.RawMode {
			// In raw mode, process all files
			// Skip directories which are already filtered out above
		} else {
			// Normal mode - check if we have a processor
			_, hasProcessor := GetByFilename(path)
			
			// Check if file is explicitly included via !pattern in .aidignore
			explicitlyIncluded := ignoreMatcher != nil && ignoreMatcher.IsExplicitlyIncluded(path)
			
			if !hasProcessor && !explicitlyIncluded {
				return nil
			}
		}

		// Process file
		fileOpts := opts
		// Check if file is explicitly included
		if ignoreMatcher != nil && ignoreMatcher.IsExplicitlyIncluded(path) {
			fileOpts.ExplicitInclude = true
		}
		file, err := p.ProcessFile(path, fileOpts)
		if err != nil {
			// Log error but continue
			fmt.Fprintf(os.Stderr, "Warning: failed to process %s: %v\n", path, err)
			return nil
		}

		// Skip nil files (e.g., binary files in RawMode)
		if file != nil {
			result.Children = append(result.Children, file)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return result, nil
}

// GetSupportedExtensions returns all supported file extensions
func (p *Processor) GetSupportedExtensions() []string {
	var extensions []string
	for _, lang := range List() {
		if proc, ok := Get(lang); ok {
			extensions = append(extensions, proc.SupportedExtensions()...)
		}
	}
	return extensions
}

// CanProcess checks if a file can be processed
func (p *Processor) CanProcess(filename string) bool {
	_, ok := GetByFilename(filename)
	return ok
}

// GetLanguage returns the language for a file
func (p *Processor) GetLanguage(filename string) string {
	proc, ok := GetByFilename(filename)
	if !ok {
		return ""
	}
	return proc.Language()
}

// MatchesPatterns reports whether a file passes the include/exclude patterns; it is the single matcher used by --include/--exclude everywhere.
// filePath is the path below the walk root (see matchPatterns).
func MatchesPatterns(filePath string, includePatterns, excludePatterns []string) bool {
	return matchPatterns(filePath, filePath, includePatterns, excludePatterns)
}

// matchPatterns applies the include/exclude patterns to one file. relPath is
// the file's path below the walk root; fullPath is the path as walked, which
// ends with relPath (pass relPath twice when there is nothing else).
//
// A pattern without "/" is matched against the segments of relPath (exclude)
// or its base name (include), so the names of the walk root and of the
// directories above it never match. A pattern with "/" is matched against the
// end of fullPath, and a pattern that matches a directory below the walk root
// covers every file in that directory.
func matchPatterns(relPath, fullPath string, includePatterns, excludePatterns []string) bool {
	// Normalize path separators for consistent matching
	normalizedRel := filepath.ToSlash(relPath)
	normalizedPath := filepath.ToSlash(fullPath)
	
	// Check exclude patterns first
	for _, pattern := range excludePatterns {
		if pattern == "" {
			continue
		}
		
		// Normalize pattern for cross-platform compatibility
		normalizedPattern := filepath.ToSlash(pattern)
		
		// Check for directory exclusion patterns (e.g., "vendor/**", "*/test/*")
		if strings.Contains(normalizedPattern, "/") {
			if matchesSlashPattern(normalizedRel, normalizedPath, normalizedPattern) {
				return false
			}
		} else {
			// Simple pattern without a path separator. Match it against every
			// path segment below the walk root (the basename is the last
			// segment) so that a pattern like "*migrations*" also excludes files
			// inside a directory named "migrations", not only files whose
			// basename matches (issue #9).
			for _, segment := range strings.Split(normalizedRel, "/") {
				if segment == "" {
					continue
				}
				if matched, _ := filepath.Match(pattern, segment); matched {
					return false
				}
			}
		}
	}
	
	// If include patterns specified, file must match at least one
	if len(includePatterns) > 0 {
		for _, pattern := range includePatterns {
			if pattern == "" {
				continue
			}
			
			// Normalize pattern
			normalizedPattern := filepath.ToSlash(pattern)
			
			// Check for path patterns
			if strings.Contains(normalizedPattern, "/") {
				if matchesSlashPattern(normalizedRel, normalizedPath, normalizedPattern) {
					return true
				}
			} else {
				// Simple filename pattern
				if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
					return true
				}
			}
		}
		return false
	}
	
	return true
}

// matchesSlashPattern reports whether a pattern containing "/" matches the file
// (fullPath) or one of the directories it lies in below the walk root.
func matchesSlashPattern(relPath, fullPath, pattern string) bool {
	if matchesPathSuffix(fullPath, pattern) {
		return true
	}
	base := len(fullPath) - len(relPath)
	if base < 0 || fullPath[base:] != relPath || base > 0 && fullPath[base-1] != '/' {
		fullPath, base = relPath, 0
	}
	for i := len(relPath) - 1; i > 0; i-- {
		if relPath[i] == '/' && matchesPathSuffix(fullPath[:base+i], pattern) {
			return true
		}
	}
	return false
}

// matchesPathSuffix matches a pattern containing "/" against path. A relative
// pattern may match any trailing part of the path that starts at a "/", so
// "internal/parser/grammars/*" matches
// "/home/user/project/internal/parser/grammars/file.go"; a pattern that starts
// with "/" or "**" must match the whole path.
func matchesPathSuffix(path, pattern string) bool {
	if strings.HasPrefix(pattern, "/") || strings.HasPrefix(pattern, "**") {
		return matchesPathPattern(path, pattern)
	}
	pathParts := strings.Split(path, "/")
	for i := 0; i < len(pathParts); i++ {
		if matchesPathPattern(strings.Join(pathParts[i:], "/"), pattern) {
			return true
		}
	}
	return false
}

// matchesPathPattern checks if a path matches a pattern that may contain directory components
func matchesPathPattern(path, pattern string) bool {
	// Handle ** for recursive directory matching
	if strings.Contains(pattern, "**") {
		return globRegexp(pattern).MatchString(path)
	}
	
	// Handle patterns with specific directory paths
	if strings.HasSuffix(pattern, "/*") {
		// Pattern like "vendor/*" - check if path is directly under this directory
		dir := strings.TrimSuffix(pattern, "/*")
		return strings.HasPrefix(path, dir+"/") && !strings.Contains(strings.TrimPrefix(path, dir+"/"), "/")
	}
	
	// Handle exact directory prefix patterns
	if strings.HasSuffix(pattern, "/") {
		return strings.HasPrefix(path, pattern)
	}
	
	// Try standard glob matching on full path
	matched, _ := filepath.Match(pattern, path)
	return matched
}

// globRegexps caches the compiled "**" patterns: they are matched once per
// walked file and directory level.
var globRegexps sync.Map

// globRegexp returns the anchored regular expression for a glob containing "**".
func globRegexp(pattern string) *regexp.Regexp {
	if re, ok := globRegexps.Load(pattern); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile("^" + globToRegexp(pattern) + "$")
	globRegexps.Store(pattern, re)
	return re
}

// globToRegexp converts a glob containing "**" into a regular expression body
// (without anchors). "**/" matches zero or more directories, "**" matches any
// characters including "/", "*" matches within one segment, "?" one character,
// and every other character is matched literally.
func globToRegexp(pattern string) string {
	var sb strings.Builder
	for i := 0; i < len(pattern); i++ {
		switch c := pattern[i]; c {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				if i+2 < len(pattern) && pattern[i+2] == '/' {
					sb.WriteString("(?:.*/)?")
					i += 2
				} else {
					sb.WriteString(".*")
					i++
				}
			} else {
				sb.WriteString("[^/]*")
			}
		case '?':
			sb.WriteString("[^/]")
		default:
			sb.WriteString(regexp.QuoteMeta(string(c)))
		}
	}
	return sb.String()
}

// isDefaultIgnoredDir checks if a directory should be ignored by default
func isDefaultIgnoredDir(dirname string) bool {
	defaultIgnored := []string{
		"node_modules",     // JavaScript/TypeScript
		"vendor",           // Go, PHP
		"target",           // Rust
		"build",            // Various
		"dist",             // JavaScript/TypeScript build output
		".gradle",          // Java/Kotlin
		"gradle",           // Java/Kotlin
		"__pycache__",      // Python
		".pytest_cache",    // Python
		"venv",             // Python virtual environment
		".venv",            // Python virtual environment
		"env",              // Python virtual environment
		".env",             // Python virtual environment
		"Pods",             // Swift/iOS
		".bundle",          // Ruby
		"bin",              // Various compiled binaries
		"obj",              // C#/.NET
		".vs",              // Visual Studio
		".idea",            // JetBrains IDEs
		".vscode",          // VS Code
		"coverage",         // Test coverage
		".nyc_output",      // JavaScript test coverage
		"bower_components", // Legacy JavaScript
		".terraform",       // Terraform
		".git",             // Git repository
		".svn",             // SVN
		".hg",              // Mercurial
	}
	
	for _, ignored := range defaultIgnored {
		if dirname == ignored {
			return true
		}
	}
	return false
}

