package aiactions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/janreges/ai-distiller/internal/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeTree(t *testing.T, rels ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, rel := range rels {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte("x"), 0o644))
	}
	return dir
}

func TestCollectSourceFiles_UsesSharedPatternSemantics(t *testing.T) {
	dir := writeTree(t, "src/a.go", "src/migrations/c.go", "tests/b.go", "x.sql", "node_modules/z.js", "docs/d.md")

	all, err := collectSourceFiles(dir, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"src/a.go", "src/migrations/c.go", "tests/b.go", "x.sql"}, all)

	// A pattern without a slash matches any path segment (issue #9).
	filtered, err := collectSourceFiles(dir, nil, []string{"*tests*", "*migrations*"})
	require.NoError(t, err)
	assert.Equal(t, []string{"src/a.go", "x.sql"}, filtered)

	onlyGo, err := collectSourceFiles(dir, []string{"**/*.go"}, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"src/a.go", "src/migrations/c.go", "tests/b.go"}, onlyGo)
}

func TestMultiFileDocsFlow_UsesSharedSourceCollection(t *testing.T) {
	dir := writeTree(t, "src/a.go", "src/migrations/c.go", "node_modules/z.js", "web/app.vue")

	res, err := (&MultiFileDocsFlowAction{}).ExecuteFlow(&ai.ActionContext{
		ProjectPath:     dir,
		BaseName:        "proj",
		Timestamp:       time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		ExcludePatterns: []string{"*migrations*"},
	})
	require.NoError(t, err)

	var docs []string
	for p := range res.Files {
		if strings.Contains(p, "/files/") {
			docs = append(docs, filepath.Base(p))
		}
	}
	assert.ElementsMatch(t, []string{"src_a_go.md", "web_app_vue.md"}, docs)

	index := res.Files["DOCS-INDEX.proj.2026-01-02.md"]
	assert.Contains(t, index, "| src/a.go |")
	assert.NotContains(t, index, dir)
	assert.NotContains(t, index, "node_modules")
	assert.NotContains(t, index, "migrations")
}

func TestCollectSourceFiles_WalkRootIsNeverSkipped(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"docs", ".hidden", "vendor"} {
		root := filepath.Join(base, name)
		require.NoError(t, os.MkdirAll(filepath.Join(root, "sub"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("x"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(root, "sub", "b.go"), []byte("x"), 0o644))

		got, err := collectSourceFiles(root, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, []string{"a.go", "sub/b.go"}, got, name)
	}
}

func TestCollectSourceFiles_SingleFileRoot(t *testing.T) {
	dir := writeTree(t, "src/a.go", "src/b.go")
	got, err := collectSourceFiles(filepath.Join(dir, "src", "a.go"), nil, nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"a.go"}, got)

	got, err = collectSourceFiles(filepath.Join(dir, "src", "a.go"), nil, []string{"*.go"})
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A directory above the walk root never matches a pattern.
func TestCollectSourceFiles_ParentDirectoryNameDoesNotMatch(t *testing.T) {
	base := writeTree(t, "my-tests/app/a.py", "my-tests/app/sub/b.py", "my-tests/app/tests/c.py")
	got, err := collectSourceFiles(filepath.Join(base, "my-tests", "app"), nil, []string{"*test*"})
	require.NoError(t, err)
	assert.Equal(t, []string{"a.py", "sub/b.py"}, got)
}
