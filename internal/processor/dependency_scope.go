package processor

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/project"
)

// followScope decides which imported files --dependency-aware may read and
// print: only files inside the boundary (the repository).
type followScope struct {
	boundary     string // absolute path of the directory following stays in
	realBoundary string // boundary with symlinks resolved
	kind         string // what boundary is: "repository", "project root" or "input directory"
}

// newFollowScope returns the scope of a run on inputDir (the input directory,
// or the directory of the entry file).
func newFollowScope(inputDir string) *followScope {
	boundary, kind := dependencyBoundary(inputDir)
	real, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		real = boundary
	}
	return &followScope{boundary: boundary, realBoundary: real, kind: kind}
}

// dependencyBoundary returns the directory that --dependency-aware never
// leaves for a run on dir: the repository top (the nearest directory at or
// above dir that holds .git), else the project root aid detects
// (internal/project) when it holds dir, else dir itself.
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
	if root, err := project.FindRoot(); err == nil && root != nil && root.Path != "" {
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
// inside the boundary. A path that cannot be resolved is not inside. A nil
// scope contains everything.
func (s *followScope) contains(path string) bool {
	if s == nil {
		return true
	}
	real, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	return pathInside(real, s.realBoundary)
}

// describe names the boundary for the log and the stderr note.
func (s *followScope) describe() string {
	return fmt.Sprintf("the %s %s", s.kind, s.boundary)
}
