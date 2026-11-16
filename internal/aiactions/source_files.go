package aiactions

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/project"
)

// collectSourceFiles walks projectPath and returns the source files to analyze
// as sorted, slash-separated paths relative to projectPath. It is shared by the
// AI flow actions so --include/--exclude behave exactly like they do for the
// regular distillation (processor.MatchesPatterns).
func collectSourceFiles(projectPath string, includePatterns, excludePatterns []string) ([]string, error) {
	var sourceFiles []string

	// Load git submodules to skip them
	// gitSubmodules := make(map[string]bool) // TODO: Implement later

	// Define source file extensions
	sourceExtensions := map[string]bool{
		".py": true, ".js": true, ".ts": true, ".tsx": true, ".jsx": true,
		".go": true, ".java": true, ".kt": true, ".kts": true,
		".rs": true, ".swift": true, ".rb": true, ".php": true,
		".cpp": true, ".cc": true, ".cxx": true, ".c": true, ".h": true, ".hpp": true,
		".cs": true, ".fs": true, ".vb": true,
		".scala": true, ".clj": true, ".cljs": true,
		".vue": true, ".svelte": true, ".astro": true,
		".html": true, ".htm": true, ".css": true, ".scss": true, ".sass": true,
		".json": true, ".yaml": true, ".yml": true, ".toml": true,
		".md": true, ".mdx": true, ".rst": true, ".txt": true,
		".sh": true, ".bash": true, ".sql": true, ".graphql": true,
	}

	// Skip these directories
	skipDirs := map[string]bool{
		"node_modules": true, ".git": true, ".svn": true, ".hg": true,
		"__pycache__": true, ".pytest_cache": true,
		"target": true, "build": true, "dist": true, "out": true,
		".aid": true, "vendor": true, ".vscode": true, ".idea": true,
		"coverage": true, ".coverage": true, ".nyc_output": true,
		"grammars": true, "test-data": true, "testdata": true,
		"docs": true, "examples": true, "assets": true, "static": true,
	}

	err := filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// The walk root is what the user asked for: it is never skipped by name,
		// and a root that is a single file is that file (named by its base name).
		isRoot := path == projectPath

		if info.IsDir() {
			if isRoot {
				return nil
			}
			dirName := filepath.Base(path)

			// Skip directories
			if skipDirs[dirName] || strings.HasPrefix(dirName, ".") && dirName != "." {
				return filepath.SkipDir
			}

			// Check for tree-sitter related content
			relPath, _ := filepath.Rel(projectPath, path)
			if strings.Contains(relPath, "grammars") || strings.Contains(relPath, "tree-sitter") {
				return filepath.SkipDir
			}

			return nil
		}

		// Check if it's a source file; with AID_CONFINE_ROOT set, not one whose
		// real path lies outside it
		ext := strings.ToLower(filepath.Ext(path))
		if sourceExtensions[ext] && project.InConfineRoot(path) {
			relPath, err := filepath.Rel(projectPath, path)
			if isRoot {
				relPath, err = filepath.Base(path), nil
			}
			if err == nil {
				// Skip generated files (not when the file was named explicitly)
				fileName := filepath.Base(relPath)
				if !isRoot && (strings.HasPrefix(fileName, ".aid.") ||
					strings.HasPrefix(fileName, ".") ||
					strings.Contains(fileName, "generated")) {
					return nil
				}

				// Apply include/exclude filters
				if processor.MatchesPatterns(relPath, includePatterns, excludePatterns) {
					sourceFiles = append(sourceFiles, filepath.ToSlash(relPath))
				}
			}
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	sort.Strings(sourceFiles)
	return sourceFiles, nil
}
