package processor

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
)

// rustCrate is what --dependency-aware reads from a Cargo.toml file.
type rustCrate struct {
	dir     string            // the directory holding Cargo.toml
	src     string            // dir/src, where the crate's module tree lives
	libName string            // the library's name in paths (hyphens as underscores)
	deps    map[string]string // path dependency name in paths -> its directory
}

var (
	rustModDecl    = regexp.MustCompile(`(?m)^[ \t]*((?:#\[[^\]\n]*\][ \t]*\n?[ \t]*)*)(?:pub(?:\s*\([^)]*\))?\s+)?mod\s+([A-Za-z_]\w*)\s*;`)
	rustUseStmt    = regexp.MustCompile(`\buse\s+([^;]+);`)
	rustPathExpr   = regexp.MustCompile(`\b([A-Za-z_]\w*)((?:\s*::\s*[A-Za-z_]\w*)+)`)
	rustUseAlias   = regexp.MustCompile(`\s+as\s+[A-Za-z_]\w*`)
	tomlTable      = regexp.MustCompile(`^\[\[?\s*([^\]]+?)\s*\]\]?$`)
	tomlString     = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*"([^"]*)"`)
	tomlInlinePath = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*\{[^}]*\bpath\s*=\s*"([^"]+)"`)
)

// extractRustImports returns the module files a Rust file uses: the files of
// its "mod x;" declarations (not under #[cfg(test)]), and the module file each
// path names that starts with crate::, super::, self::, the crate's library
// name, a path dependency's name or a module the file declares, in "use"
// declarations and in expressions. A path is followed to the deepest module
// that exists as x.rs or x/mod.rs; crates.io crates and std are not followed.
func (da *DependencyAnalyzer) extractRustImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	code := stripSlashComments(string(fileContent))
	crate := da.rustCrateFor(filepath.Dir(filePath))
	selfDir := rustModuleDir(filePath, crate)

	var deps []string
	declared := map[string]bool{}
	for _, m := range rustModDecl.FindAllStringSubmatch(code, -1) {
		if strings.Contains(m[1], "cfg(test)") {
			continue
		}
		declared[m[2]] = true
		if f := rustModuleFile(selfDir, m[2]); f != "" {
			deps = append(deps, f)
		}
	}

	var paths [][]string
	for _, m := range rustUseStmt.FindAllStringSubmatch(code, -1) {
		for _, p := range expandRustUseTree(m[1]) {
			paths = append(paths, strings.Split(p, "::"))
		}
	}
	for _, m := range rustPathExpr.FindAllStringSubmatch(code, -1) {
		segs := []string{m[1]}
		for _, s := range strings.Split(m[2], "::") {
			if s = strings.TrimSpace(s); s != "" {
				segs = append(segs, s)
			}
		}
		paths = append(paths, segs)
	}

	for _, segs := range paths {
		if f := da.resolveRustPath(segs, filePath, selfDir, crate, declared); f != "" && f != filePath {
			deps = append(deps, f)
			dbg.Logf(debug.LevelDetailed, "Rust path %s -> %s", strings.Join(segs, "::"), f)
		}
	}
	return deps, nil
}

// resolveRustPath returns the module file a path names, or "".
func (da *DependencyAnalyzer) resolveRustPath(segs []string, filePath, selfDir string, crate *rustCrate, declared map[string]bool) string {
	if len(segs) == 0 {
		return ""
	}
	first, rest := segs[0], segs[1:]
	switch {
	case first == "crate":
		root := crateRootDir(filePath, crate)
		return rustModulePath(root, rest, rustRootFile(root, filePath))
	case first == "self":
		return rustModulePath(selfDir, rest, "")
	case first == "super":
		dir := filepath.Dir(selfDir)
		for len(rest) > 0 && rest[0] == "super" {
			dir, rest = filepath.Dir(dir), rest[1:]
		}
		return rustModulePath(dir, rest, rustModuleFileAt(dir, crate, filePath))
	case declared[first]:
		return rustModulePath(selfDir, segs, "")
	case crate != nil && first == crate.libName:
		return rustModulePath(crate.src, rest, existingFile(filepath.Join(crate.src, "lib.rs")))
	case crate != nil && crate.deps[first] != "":
		src := filepath.Join(crate.deps[first], "src")
		return rustModulePath(src, rest, existingFile(filepath.Join(src, "lib.rs")))
	}
	return ""
}

// rustModulePath walks segs from the module directory dir as long as each
// segment is a module file (seg.rs or seg/mod.rs) and returns the last one
// found, or fallback when the first segment is not a module.
func rustModulePath(dir string, segs []string, fallback string) string {
	found := fallback
	for _, seg := range segs {
		if seg == "*" || seg == "self" {
			break
		}
		f := rustModuleFile(dir, seg)
		if f == "" {
			break
		}
		found = f
		dir = filepath.Join(dir, seg)
	}
	return found
}

// rustModuleFile returns dir/name.rs or dir/name/mod.rs, whichever exists.
func rustModuleFile(dir, name string) string {
	if f := existingFile(filepath.Join(dir, name+".rs")); f != "" {
		return f
	}
	return existingFile(filepath.Join(dir, name, "mod.rs"))
}

// rustModuleFileAt returns the file of the module whose directory is dir:
// dir.rs, dir/mod.rs or, for the crate root, its main.rs or lib.rs.
func rustModuleFileAt(dir string, crate *rustCrate, filePath string) string {
	if f := existingFile(dir + ".rs"); f != "" {
		return f
	}
	if f := existingFile(filepath.Join(dir, "mod.rs")); f != "" {
		return f
	}
	if dir == crateRootDir(filePath, crate) {
		return rustRootFile(dir, filePath)
	}
	return ""
}

// rustModuleDir returns the directory of filePath's own child modules:
// the file's directory for a crate root or a mod.rs, else dir/stem.
func rustModuleDir(filePath string, crate *rustCrate) string {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	if base == "mod.rs" || dir == crateRootDir(filePath, crate) && (base == "main.rs" || base == "lib.rs") {
		return dir
	}
	if crate == nil || !isWithin(filePath, crate.src) || isWithin(filePath, filepath.Join(crate.src, "bin")) {
		return dir // a crate root of its own: examples/, tests/, benches/, src/bin/, build.rs
	}
	return filepath.Join(dir, strings.TrimSuffix(base, ".rs"))
}

// crateRootDir returns the directory crate:: paths start from: the crate's
// src for a file in its module tree, else the file's own directory (a file
// in examples/, tests/ or src/bin/ is the root of a crate of its own).
func crateRootDir(filePath string, crate *rustCrate) string {
	if crate != nil && isWithin(filePath, crate.src) && !isWithin(filePath, filepath.Join(crate.src, "bin")) {
		return crate.src
	}
	return filepath.Dir(filePath)
}

// rustRootFile returns the crate root file in root that crate:: refers to
// from filePath: main.rs for main.rs itself, else lib.rs, else main.rs.
func rustRootFile(root, filePath string) string {
	if filepath.Dir(filePath) == root && filepath.Base(filePath) == "main.rs" {
		return filePath
	}
	if f := existingFile(filepath.Join(root, "lib.rs")); f != "" {
		return f
	}
	return existingFile(filepath.Join(root, "main.rs"))
}

// expandRustUseTree turns a use tree into its paths:
// "crate::a::{b, c::{D, E as F}}" -> crate::a::b, crate::a::c::D, crate::a::c::E.
func expandRustUseTree(tree string) []string {
	tree = rustUseAlias.ReplaceAllString(tree, "")
	tree = strings.Join(strings.Fields(tree), "")
	tree = strings.TrimPrefix(tree, "::")
	return expandRustUsePart("", tree)
}

func expandRustUsePart(prefix, part string) []string {
	open := strings.Index(part, "{")
	if open < 0 {
		return []string{strings.TrimSuffix(prefix+part, "::self")}
	}
	closing := strings.LastIndex(part, "}")
	if closing < open {
		return nil
	}
	head := prefix + part[:open]
	var paths []string
	depth, start := 0, open+1
	for i := open + 1; i <= closing; i++ {
		switch part[i] {
		case '{':
			depth++
		case '}', ',':
			if part[i] == '}' && depth > 0 {
				depth--
				continue
			}
			if depth == 0 {
				if item := part[start:i]; item != "" {
					paths = append(paths, expandRustUsePart(head, item)...)
				}
				start = i + 1
			}
		}
	}
	return paths
}

// rustCrateFor parses the nearest Cargo.toml at or above dir, once per file.
func (da *DependencyAnalyzer) rustCrateFor(dir string) *rustCrate {
	manifest := da.findUp(dir, "Cargo.toml")
	if manifest == "" {
		return nil
	}
	if c, ok := da.rustCrates[manifest]; ok {
		return c
	}
	c := parseCargoToml(manifest)
	da.rustCrates[manifest] = c
	return c
}

// parseCargoToml reads [package] name, [lib] name and the dependencies
// declared with a local path, line by line.
func parseCargoToml(manifest string) *rustCrate {
	content, err := os.ReadFile(manifest)
	if err != nil {
		return nil
	}
	dir := filepath.Dir(manifest)
	c := &rustCrate{dir: dir, src: filepath.Join(dir, "src"), deps: map[string]string{}}
	var table, pkgName, libName string
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if m := tomlTable.FindStringSubmatch(line); m != nil {
			table = m[1]
			continue
		}
		isDepTable := table == "dependencies" || table == "dev-dependencies" || table == "build-dependencies"
		switch {
		case table == "package":
			if m := tomlString.FindStringSubmatch(line); m != nil && m[1] == "name" {
				pkgName = m[2]
			}
		case table == "lib":
			if m := tomlString.FindStringSubmatch(line); m != nil && m[1] == "name" {
				libName = m[2]
			}
		case isDepTable:
			if m := tomlInlinePath.FindStringSubmatch(line); m != nil {
				c.deps[rustIdent(m[1])] = filepath.Join(dir, filepath.FromSlash(m[2]))
			}
		case strings.HasPrefix(table, "dependencies."):
			if m := tomlString.FindStringSubmatch(line); m != nil && m[1] == "path" {
				c.deps[rustIdent(strings.TrimPrefix(table, "dependencies."))] = filepath.Join(dir, filepath.FromSlash(m[2]))
			}
		}
	}
	if libName == "" {
		libName = pkgName
	}
	c.libName = rustIdent(libName)
	return c
}

// rustIdent is a crate name as Rust code writes it.
func rustIdent(name string) string {
	return strings.ReplaceAll(name, "-", "_")
}

// existingFile returns path when it is an existing regular file, else "".
func existingFile(path string) string {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return path
	}
	return ""
}

// isWithin reports whether path lies inside dir.
func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != "." && !strings.HasPrefix(rel, "..")
}
