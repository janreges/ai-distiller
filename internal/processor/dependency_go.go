package processor

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
)

// goModule is what --dependency-aware reads from a go.mod file.
type goModule struct {
	dir      string            // the directory holding go.mod
	path     string            // the module path
	replaces map[string]string // replaced module path -> local directory
}

// goFile is a non-test Go file of a directory and its package clause.
type goFile struct {
	path string
	pkg  string
}

var (
	goModuleLine   = regexp.MustCompile(`(?m)^\s*module\s+"?([^"\s]+)"?`)
	goReplaceSpec  = regexp.MustCompile(`^(\S+)(?:\s+\S+)?\s+=>\s+(\S+)(?:\s+\S+)?$`)
	goPackageLine  = regexp.MustCompile(`(?m)^package\s+([A-Za-z_]\w*)`)
	goImportSingle = regexp.MustCompile(`^import\s+(?:[\w.]+\s+)?"([^"]+)"`)
	goImportSpec   = regexp.MustCompile(`^(?:[\w.]+\s+)?"([^"]+)"`)
)

// extractGoImports returns the files a Go file depends on: the other non-test
// files of its package (same directory, same package clause), then the
// non-test files of each imported package that belongs to the file's module
// (the module path of the nearest go.mod) or to a module that go.mod
// replaces with a local directory. The standard library and other modules are
// not followed.
func (da *DependencyAnalyzer) extractGoImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	var deps []string

	dir := filepath.Dir(filePath)
	if m := goPackageLine.FindSubmatch(fileContent); m != nil {
		pkg := string(m[1])
		for _, f := range da.goPackageFiles(dir) {
			if f.pkg == pkg && f.path != filePath {
				deps = append(deps, f.path)
			}
		}
	}

	mod := da.goModuleFor(dir)
	if mod == nil {
		return deps, nil
	}
	for _, importPath := range goImportPaths(fileContent) {
		pkgDir := mod.resolve(importPath)
		if pkgDir == "" {
			continue
		}
		if !da.scope.contains(pkgDir) {
			// Not read: Follow reports the directory as outside.
			deps = append(deps, pkgDir)
			continue
		}
		files := da.goPackageFiles(pkgDir)
		pkg := dominantGoPackage(files)
		for _, f := range files {
			if f.pkg == pkg {
				deps = append(deps, f.path)
			}
		}
		dbg.Logf(debug.LevelDetailed, "Go import %s -> %s (%d files)", importPath, pkgDir, len(files))
	}
	return deps, nil
}

// goImportPaths returns the import paths of a Go file's import declarations.
func goImportPaths(content []byte) []string {
	var paths []string
	inBlock := false
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case inBlock && strings.HasPrefix(line, ")"):
			inBlock = false
		case inBlock:
			if m := goImportSpec.FindStringSubmatch(line); m != nil {
				paths = append(paths, m[1])
			}
		case line == "import (" || strings.HasPrefix(line, "import ("):
			inBlock = true
		case strings.HasPrefix(line, "import "):
			if m := goImportSingle.FindStringSubmatch(line); m != nil {
				paths = append(paths, m[1])
			}
		}
	}
	return paths
}

// resolve returns the directory of importPath when it lies in the module or
// in a module replaced by a local directory, else "".
func (m *goModule) resolve(importPath string) string {
	if rest, ok := goPathWithin(importPath, m.path); ok {
		return filepath.Join(m.dir, rest)
	}
	best, bestDir := "", ""
	for old, local := range m.replaces {
		if _, ok := goPathWithin(importPath, old); ok && len(old) > len(best) {
			best, bestDir = old, local
		}
	}
	if best == "" {
		return ""
	}
	rest, _ := goPathWithin(importPath, best)
	return filepath.Join(bestDir, rest)
}

// goPathWithin reports whether importPath is modulePath or a package below
// it, and returns the part below it.
func goPathWithin(importPath, modulePath string) (string, bool) {
	if importPath == modulePath {
		return "", true
	}
	if strings.HasPrefix(importPath, modulePath+"/") {
		return filepath.FromSlash(importPath[len(modulePath)+1:]), true
	}
	return "", false
}

// goModuleFor parses the nearest go.mod at or above dir, once per go.mod.
func (da *DependencyAnalyzer) goModuleFor(dir string) *goModule {
	modFile := da.findUp(dir, "go.mod")
	if modFile == "" {
		return nil
	}
	if m, ok := da.goModules[modFile]; ok {
		return m
	}
	m := parseGoMod(modFile)
	da.goModules[modFile] = m
	return m
}

// parseGoMod reads the module path and the replace directives that point to
// a local directory (a path starting with ./, ../ or /).
func parseGoMod(modFile string) *goModule {
	content, err := os.ReadFile(modFile)
	if err != nil {
		return nil
	}
	match := goModuleLine.FindSubmatch(content)
	if match == nil {
		return nil
	}
	m := &goModule{dir: filepath.Dir(modFile), path: string(match[1]), replaces: map[string]string{}}
	inBlock := false
	for _, line := range strings.Split(string(content), "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		var spec string
		switch {
		case inBlock && line == ")":
			inBlock = false
			continue
		case inBlock:
			spec = line
		case strings.HasPrefix(line, "replace") && strings.TrimSpace(strings.TrimPrefix(line, "replace")) == "(":
			inBlock = true
			continue
		case strings.HasPrefix(line, "replace "):
			spec = strings.TrimSpace(strings.TrimPrefix(line, "replace "))
		default:
			continue
		}
		r := goReplaceSpec.FindStringSubmatch(spec)
		if r == nil {
			continue
		}
		target := r[2]
		if !strings.HasPrefix(target, "./") && !strings.HasPrefix(target, "../") && !filepath.IsAbs(target) {
			continue // replaced by another module version, not by a directory
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(m.dir, filepath.FromSlash(target))
		}
		m.replaces[r[1]] = target
	}
	return m
}

// goPackageFiles lists the non-test .go files of dir, sorted, with their
// package clauses, once per directory. It lists no directory and reads no
// file outside the scope's boundary.
func (da *DependencyAnalyzer) goPackageFiles(dir string) []goFile {
	if files, ok := da.goPackages[dir]; ok {
		return files
	}
	var files []goFile
	var entries []os.DirEntry
	var err error
	if da.scope.contains(dir) {
		entries, err = os.ReadDir(dir)
	}
	if err == nil {
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			path := filepath.Join(dir, name)
			if !da.scope.contains(path) {
				continue // a symlink out of the boundary
			}
			content, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			if m := goPackageLine.FindSubmatch(content); m != nil {
				files = append(files, goFile{path: path, pkg: string(m[1])})
			}
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].path < files[j].path })
	da.goPackages[dir] = files
	return files
}

// dominantGoPackage returns the package clause most files of a directory
// share, so a stray "package main" generator (//go:build ignore) next to a
// library is not followed.
func dominantGoPackage(files []goFile) string {
	counts := map[string]int{}
	best := ""
	for _, f := range files {
		counts[f.pkg]++
		if counts[f.pkg] > counts[best] || (counts[f.pkg] == counts[best] && f.pkg < best) {
			best = f.pkg
		}
	}
	return best
}
