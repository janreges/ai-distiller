package processor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ignore"
	"github.com/janreges/ai-distiller/internal/project"
)

// followScope decides which imported files --dependency-aware may read and
// print: only files inside the boundary (the repository) that the file
// selection rules of a directory walk keep.
type followScope struct {
	boundary     string // absolute path of the directory following stays in
	realBoundary string // boundary with symlinks resolved
	kind         string // what boundary is: "repository", "project root", "input directory" or "AID_CONFINE_ROOT directory"
	inputDir     string // the input directory, or the entry file's directory
	include      []string
	exclude      []string
	aidignores   map[string]*ignore.IgnoreMatcher // directory -> matcher of its .aidignore, nil when it has none
}

// newFollowScope returns the scope of a run on inputDir (the input directory,
// or the directory of the entry file). With AID_CONFINE_ROOT set, the
// boundary is the deeper of dependencyBoundary and the confine root: both
// hold the input, so the one nearer to it.
func newFollowScope(inputDir string, opts ProcessOptions) *followScope {
	boundary, kind := dependencyBoundary(inputDir)
	real, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		real = boundary
	}
	if confine, err := project.ConfineRoot(); err == nil && confine != "" &&
		confine != real && pathInside(confine, real) {
		boundary, real, kind = confine, confine, project.EnvConfineRoot+" directory"
	}
	return &followScope{
		boundary:     boundary,
		realBoundary: real,
		kind:         kind,
		inputDir:     inputDir,
		include:      opts.IncludePatterns,
		exclude:      opts.ExcludePatterns,
		aidignores:   map[string]*ignore.IgnoreMatcher{},
	}
}

// dependencyBoundary returns the directory that --dependency-aware never
// leaves for a run on dir: the repository top (the nearest directory at or
// above dir that holds .git), else the project root aid detects
// (internal/project) when it holds dir and was found by a marker or named by
// AID_PROJECT_ROOT, else dir itself. The working directory that
// internal/project falls back to when it finds neither (IsFallback) is no
// boundary: run from "/", it would let following read the whole disk.
func dependencyBoundary(dir string) (boundary, kind string) {
	for d := dir; ; {
		if exists(filepath.Join(d, ".git")) {
			return d, "repository"
		}
		parent := filepath.Dir(d)
		if parent == d {
			break
		}
		d = parent
	}
	if root, err := project.FindRoot(); err == nil && root != nil && root.Path != "" && !root.IsFallback {
		if abs, err := filepath.Abs(root.Path); err == nil && pathInside(dir, abs) {
			return abs, "project root"
		}
	}
	return dir, "input directory"
}

// pathInside reports whether path is dir or lies below it.
func pathInside(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// contains reports whether the real path of path (symlinks resolved) lies
// inside the boundary and, with AID_CONFINE_ROOT set, inside the confine
// root (project.InConfineRoot). A path that cannot be resolved is not
// inside. A nil scope contains everything.
func (s *followScope) contains(path string) bool {
	if s == nil {
		return true
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return pathInside(real, s.realBoundary) && project.InConfineRoot(real)
}

// describe names the boundary for the log and the stderr note.
func (s *followScope) describe() string {
	return fmt.Sprintf("the %s %s", s.kind, s.boundary)
}

// excludedBy returns why the file selection rules of a directory walk leave
// out the followed file path, or "" when they keep it:
//   - an .aidignore file in a directory from the boundary down to the file's
//     directory ignores the file or a directory on its way, as a walk from
//     that directory would;
//   - the file lies in an .aid directory or a directory the walk skips by
//     default (node_modules, vendor, build, ...; an .aidignore "!" pattern
//     can bring it back), below the deepest directory that holds both the
//     input and the file, or its name holds ".aid.";
//   - --include and --exclude reject its path relative to that directory:
//     the printed path without its leading "../" segments, which for a file
//     inside the input directory is the walk's own relative path.
//
// --recursive=0 limits only the walk, not following.
func (s *followScope) excludedBy(path string) string {
	if s == nil {
		return ""
	}
	inputDir, boundary := s.inputDir, s.boundary
	if !pathInside(path, boundary) {
		// Reached through a symlink that resolves into the boundary.
		realPath, err1 := filepath.EvalSymlinks(path)
		realInput, err2 := filepath.EvalSymlinks(inputDir)
		if err1 != nil || err2 != nil {
			return "an unresolvable path"
		}
		path, inputDir, boundary = realPath, realInput, s.realBoundary
		if !pathInside(path, boundary) {
			return "a path outside " + s.describe()
		}
	}

	// .aidignore files from the boundary down to the file's directory.
	var matchers []*ignore.IgnoreMatcher
	var matcherDirs []string
	for _, dir := range dirsBetween(boundary, filepath.Dir(path)) {
		if m := s.aidignore(dir); m != nil {
			matchers = append(matchers, m)
			matcherDirs = append(matcherDirs, dir)
		}
	}
	for i := len(matchers) - 1; i >= 0; i-- { // the nearest .aidignore first, to name it
		m := matchers[i]
		for _, dir := range dirsBetween(matcherDirs[i], filepath.Dir(path))[1:] {
			if m.ShouldIgnore(dir) {
				return ignoreFileName(matcherDirs[i], boundary)
			}
		}
		if m.ShouldIgnore(path) {
			return ignoreFileName(matcherDirs[i], boundary)
		}
	}

	base := commonDir(inputDir, path)
	for _, dir := range dirsBetween(base, filepath.Dir(path))[1:] {
		name := filepath.Base(dir)
		if name == ".aid" {
			return "the .aid directory"
		}
		if isDefaultIgnoredDir(name) && !explicitlyIncluded(matchers, dir) {
			return fmt.Sprintf("the directory %s, which a directory walk skips by default", name)
		}
	}
	if strings.Contains(filepath.Base(path), ".aid.") {
		return "the name of an aid output file"
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		rel = path
	}
	if !matchPatterns(rel, path, s.include, s.exclude) {
		return "--include/--exclude"
	}
	return ""
}

// aidignore returns the matcher of dir/.aidignore, or nil when there is none.
func (s *followScope) aidignore(dir string) *ignore.IgnoreMatcher {
	if m, ok := s.aidignores[dir]; ok {
		return m
	}
	var m *ignore.IgnoreMatcher
	if info, err := os.Stat(filepath.Join(dir, ".aidignore")); err == nil && !info.IsDir() {
		if loaded, err := ignore.New(dir); err == nil {
			m = loaded
		}
	}
	s.aidignores[dir] = m
	return m
}

// explicitlyIncluded reports whether an .aidignore "!" pattern brings back a
// default-ignored directory or something in it, as the walk checks.
func explicitlyIncluded(matchers []*ignore.IgnoreMatcher, dir string) bool {
	for _, m := range matchers {
		if m.IsExplicitlyIncluded(dir) || m.MightContainExplicitIncludes(dir) {
			return true
		}
	}
	return false
}

// ignoreFileName names an .aidignore file for the log, relative to the
// boundary.
func ignoreFileName(dir, boundary string) string {
	if rel, err := filepath.Rel(boundary, dir); err == nil && rel != "." {
		return filepath.Join(rel, ".aidignore")
	}
	return ".aidignore"
}

// dirsBetween returns top, then each directory below it down to bottom, which
// must lie inside top.
func dirsBetween(top, bottom string) []string {
	dirs := []string{top}
	rel, err := filepath.Rel(top, bottom)
	if err != nil || rel == "." {
		return dirs
	}
	dir := top
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		dir = filepath.Join(dir, part)
		dirs = append(dirs, dir)
	}
	return dirs
}

// commonDir returns the deepest directory that holds both dir and path.
func commonDir(dir, path string) string {
	for d := dir; ; {
		if pathInside(path, d) {
			return d
		}
		parent := filepath.Dir(d)
		if parent == d {
			return d
		}
		d = parent
	}
}
