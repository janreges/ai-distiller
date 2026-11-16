package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	confineOnce sync.Once
	confineRoot string
	confineErr  error
)

// ConfineRoot returns the real path of the directory that AID_CONFINE_ROOT
// (EnvConfineRoot) names, or "" when the variable is unset or empty. It
// returns an error when the value is not an existing directory: a caller must
// then refuse to run rather than read without the limit. The variable is read
// once per process.
func ConfineRoot() (string, error) {
	confineOnce.Do(func() {
		confineRoot, confineErr = readConfineRoot()
	})
	return confineRoot, confineErr
}

func readConfineRoot() (string, error) {
	value := os.Getenv(EnvConfineRoot)
	if value == "" {
		return "", nil
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("%s=%s: %w", EnvConfineRoot, value, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("%s=%s is not an existing directory: %w", EnvConfineRoot, value, err)
	}
	info, err := os.Stat(real)
	if err != nil {
		return "", fmt.Errorf("%s=%s is not an existing directory: %w", EnvConfineRoot, value, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s=%s is not a directory", EnvConfineRoot, value)
	}
	return real, nil
}

// InConfineRoot reports whether aid may read path: true when AID_CONFINE_ROOT
// is unset, else whether the real path of path (symlinks resolved) lies in
// the confine root. A path that cannot be resolved, or an invalid variable,
// gives false.
func InConfineRoot(path string) bool {
	root, err := ConfineRoot()
	if err != nil {
		return false
	}
	if root == "" {
		return true
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return false
	}
	return Within(real, root)
}

// Within reports whether path is dir or lies below it; both are compared as
// given, without resolving symlinks.
func Within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resetConfineRoot makes the next ConfineRoot read the variable again (tests).
func resetConfineRoot() {
	confineOnce = sync.Once{}
	confineRoot, confineErr = "", nil
}
