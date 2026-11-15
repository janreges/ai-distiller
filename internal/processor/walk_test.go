package processor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ensureLanguages(t *testing.T) {
	t.Helper()
	if _, ok := processor.GetByFilename("x.py"); !ok {
		require.NoError(t, language.RegisterAll())
	}
}

func walkPaths(t *testing.T, root string, opts processor.ProcessOptions) []string {
	t.Helper()
	res, err := processor.New().ProcessPath(root, opts)
	require.NoError(t, err)
	var out []string
	for _, c := range res.(*ir.DistilledDirectory).Children {
		out = append(out, c.(*ir.DistilledFile).Path)
	}
	return out
}

func TestWorkerCountDoesNotChangeFilesOrOrder(t *testing.T) {
	ensureLanguages(t)
	root := filepath.Join("..", "..", "testdata", "python")
	paths := func(workers int) []string {
		return walkPaths(t, root, processor.ProcessOptions{Workers: workers, Recursive: true, IncludeImports: true})
	}
	one, many := paths(1), paths(4)
	require.NotEmpty(t, one)
	assert.Equal(t, one, many)
}

// TestWorkerCountDoesNotChangeFilteredWalk pins the walk rules (.aidignore,
// default-ignored dirs, .aid. names, patterns, non-recursive) for every worker count.
func TestWorkerCountDoesNotChangeFilteredWalk(t *testing.T) {
	ensureLanguages(t)
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
	write("a.py", "def a(): pass\n")
	write("b.py", "def b(): pass\n")
	write("skip_me.py", "def s(): pass\n")
	write("x.aid.py", "def x(): pass\n")
	write("sub/c.py", "def c(): pass\n")
	write("node_modules/d.py", "def d(): pass\n")
	write("vendor/keep.py", "def k(): pass\n")
	write(".aid/e.py", "def e(): pass\n")
	write("test_f.py", "def f(): pass\n")
	write(".aidignore", "skip_me.py\n!vendor/keep.py\n")

	for name, opts := range map[string]processor.ProcessOptions{
		"recursive":     {Recursive: true},
		"non-recursive": {Recursive: false},
		"exclude":       {Recursive: true, ExcludePatterns: []string{"test_*"}},
		"include":       {Recursive: true, IncludePatterns: []string{"sub/*.py", "a.py"}},
	} {
		t.Run(name, func(t *testing.T) {
			o1, o4 := opts, opts
			o1.Workers, o4.Workers = 1, 4
			one := walkPaths(t, root, o1)
			assert.Equal(t, one, walkPaths(t, root, o4))
			assert.NotContains(t, one, filepath.Join(root, "skip_me.py"))
			assert.NotContains(t, one, filepath.Join(root, "x.aid.py"))
			assert.NotContains(t, one, filepath.Join(root, "node_modules", "d.py"))
		})
	}

	one := walkPaths(t, root, processor.ProcessOptions{Workers: 1, Recursive: true})
	assert.Contains(t, one, filepath.Join(root, "vendor", "keep.py"))
}

// TestEmptyWalkKeepsRelativeDisplayPath: a directory with no matching files
// still reports the relative display path, whatever the worker count.
func TestEmptyWalkKeepsRelativeDisplayPath(t *testing.T) {
	ensureLanguages(t)
	base := t.TempDir()
	sub := filepath.Join(base, "empty")
	require.NoError(t, os.MkdirAll(sub, 0o755))
	for _, w := range []int{1, 4} {
		res, err := processor.New().ProcessPath(sub, processor.ProcessOptions{
			Workers: w, Recursive: true, FilePathType: "relative", BasePath: base,
		})
		require.NoError(t, err)
		assert.Equal(t, "empty", res.(*ir.DistilledDirectory).Path, "workers=%d", w)
	}
}

// TestNegativeWorkerCountRunsWithOneWorker: -w <negative> must not panic.
func TestNegativeWorkerCountRunsWithOneWorker(t *testing.T) {
	ensureLanguages(t)
	root := filepath.Join("..", "..", "testdata", "python")
	one := walkPaths(t, root, processor.ProcessOptions{Workers: 1, Recursive: true})
	require.NotEmpty(t, one)
	assert.Equal(t, one, walkPaths(t, root, processor.ProcessOptions{Workers: -1, Recursive: true}))
}

// TestPatternsIgnoreDirectoriesAboveTheWalkRoot: a pattern without "/" is
// matched below the walk root, so "*test*" keeps a project that lives in
// ".../my-tests/app", and a slash pattern that matches a directory covers
// every file in it.
func TestPatternsIgnoreDirectoriesAboveTheWalkRoot(t *testing.T) {
	ensureLanguages(t)
	root := filepath.Join(t.TempDir(), "my-tests", "app")
	for _, rel := range []string{"a.py", "sub/b.py", "tests/c.py", "db/migrations/m.py", "src/x/gen/g.py"} {
		p := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("def f(): pass\n"), 0o644))
	}
	got := walkPaths(t, root, processor.ProcessOptions{Workers: 2, Recursive: true,
		ExcludePatterns: []string{"*test*", "**/migrations", "src/**/gen"}})
	assert.Equal(t, []string{filepath.Join(root, "a.py"), filepath.Join(root, "sub", "b.py")}, got)
}
