package testrunner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoverTests_FailsOnIncompleteScenario(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "python", "01_no_expected"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "python", "01_no_expected", "source.py"), []byte("x = 1\n"), 0o644))
	_, err := New(dir, "unused").DiscoverTests()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "01_no_expected")

	dir2 := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir2, "python", "02_no_source", "expected"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir2, "python", "02_no_source", "expected", "default.txt"), nil, 0o644))
	_, err = New(dir2, "unused").DiscoverTests()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "02_no_source")
}

func TestDiscoverTests_RejectsMalformedExpectedName(t *testing.T) {
	for _, name := range []string{"implementation.txt", "private=1,protected.txt", "foo.txt"} {
		dir := t.TempDir()
		sc := filepath.Join(dir, "python", "01_bad")
		require.NoError(t, os.MkdirAll(filepath.Join(sc, "expected"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(sc, "source.py"), []byte("x = 1\n"), 0o644))
		require.NoError(t, os.WriteFile(filepath.Join(sc, "expected", name), nil, 0o644))
		_, err := New(dir, "unused").DiscoverTests()
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), name)
	}
}

func TestParseParametersFromFilename(t *testing.T) {
	assert.Empty(t, parseParametersFromFilename("default.txt"))
	assert.Equal(t, []string{"--implementation=1"}, parseParametersFromFilename("implementation=1.txt"))
	assert.Equal(t, []string{"--implementation=0", "--internal=1", "--private=1", "--protected=1"},
		parseParametersFromFilename("private=1,protected=1,internal=1,implementation=0.txt"))
	assert.Empty(t, parseParametersFromFilename("public.txt"), "legacy aliases are gone")
}
