package conformance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AID_CONFINE_ROOT (set by the MCP server to the real path of AID_ROOT)
// confines what aid reads: no file whose real path lies outside it. Unset,
// it changes nothing.

// confineTree builds, under a temporary directory:
//
//	repo/.git/                   the repository top, above the confine root
//	repo/proj/                   the confine root
//	repo/proj/main.py, sub/inner.py
//	repo/proj/inlink.py       -> sub/inner.py (a symlink that stays inside)
//	repo/proj/leak.py         -> ../../outside/secret.py
//	repo/proj/out             -> ../../outside (a directory symlink)
//	repo/proj/dangling        -> ../../outside/nothere
//	repo/shared/lib.py           in the repository, outside the confine root
//	outside/secret.py
//
// and returns the temporary directory.
func confineTree(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	writeProject(t, tmp, map[string]string{
		"repo/.git/HEAD":         "ref: refs/heads/main\n",
		"repo/proj/main.py":      "def main_inside():\n    return 1\n",
		"repo/proj/sub/inner.py": "def inner_inside():\n    return 2\n",
		"repo/shared/lib.py":     "def shared_outside_the_confine_root():\n    return 3\n",
		"outside/secret.py":      "def secret_outside_the_repository():\n    return 4\n",
	})
	proj := filepath.Join(tmp, "repo", "proj")
	require.NoError(t, os.Symlink(filepath.Join("sub", "inner.py"), filepath.Join(proj, "inlink.py")))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "outside", "secret.py"), filepath.Join(proj, "leak.py")))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "outside"), filepath.Join(proj, "out")))
	require.NoError(t, os.Symlink(filepath.Join("..", "..", "outside", "nothere"), filepath.Join(proj, "dangling")))
	return tmp
}

func TestConfineRootMustBeAnExistingDirectory(t *testing.T) {
	tmp := confineTree(t)
	proj := filepath.Join(tmp, "repo", "proj")
	for _, value := range []string{filepath.Join(tmp, "missing"), filepath.Join(proj, "main.py")} {
		out, stderr, code := aidRunEnv(t, proj, aidEnv("AID_CONFINE_ROOT="+value), nil, "main.py", "--stdout", "--summary-type=off")
		assert.Equal(t, 1, code, "AID_CONFINE_ROOT=%s: %s", value, stderr)
		assert.Contains(t, stderr, "AID_CONFINE_ROOT="+value)
		assert.Empty(t, out)
	}

	// Empty is unset.
	out, stderr, code := aidRunEnv(t, proj, aidEnv("AID_CONFINE_ROOT="), nil, "main.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.py"}, filePaths(out))
}

// A path argument whose real path lies outside the confine root fails
// before anything is read, whether it leads out with "..", through a file
// symlink or through a directory symlink.
func TestConfineRootRefusesPathArgumentsOutsideIt(t *testing.T) {
	tmp := confineTree(t)
	proj := filepath.Join(tmp, "repo", "proj")
	real, err := filepath.EvalSymlinks(proj)
	require.NoError(t, err)
	env := aidEnv("AID_CONFINE_ROOT=" + proj)

	for _, arg := range []string{"../shared/lib.py", "../shared", "leak.py", "out", "out/secret.py", "sub/../../shared/lib.py", filepath.Join(tmp, "outside")} {
		out, stderr, code := aidRunEnv(t, proj, env, nil, arg, "--stdout", "--summary-type=off")
		assert.Equal(t, 1, code, "%s: %s", arg, stderr)
		assert.Contains(t, stderr, "Error: "+arg+" is outside AID_CONFINE_ROOT "+real, arg)
		assert.Empty(t, out, arg)
		assert.NotContains(t, stderr, "outside_the", arg)
	}
	// The error does not tell where a symlink inside the root points.
	_, stderr, _ := aidRunEnv(t, proj, env, nil, "leak.py", "--stdout")
	assert.NotContains(t, stderr, "secret.py")

	// Inside, also through a symlink that stays inside, and from a working
	// directory outside the root.
	for _, arg := range []string{"main.py", "inlink.py", "sub", "."} {
		_, stderr, code := aidRunEnv(t, proj, env, nil, arg, "--stdout", "--summary-type=off")
		assert.Equal(t, 0, code, "%s: %s", arg, stderr)
	}
	out, stderr, code := aidRunEnv(t, tmp, env, nil, filepath.Join("repo", "proj", "main.py"), "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.py"}, filePaths(out))
}

// A directory walk skips a file whose real path lies outside the confine
// root (a symlink out of the tree) and says so with -v; symlinks that stay
// inside are read as before. An .aidignore that is a symlink out of the root
// is not read either.
func TestConfineRootWalkSkipsFilesOutsideIt(t *testing.T) {
	tmp := confineTree(t)
	proj := filepath.Join(tmp, "repo", "proj")
	env := aidEnv("AID_CONFINE_ROOT=" + proj)

	out, stderr, code := aidRunEnv(t, proj, env, nil, ".", "--stdout", "--summary-type=off", "-v")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"inlink.py", "main.py", "sub/inner.py"}, filePaths(out))
	assert.NotContains(t, out, "outside_the")
	assert.Regexp(t, `Skipping \S+/proj/leak\.py: it does not resolve to a file inside AID_CONFINE_ROOT\n`, stderr)

	writeProject(t, tmp, map[string]string{"outside/ignore-main": "main.py\n"})
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside", "ignore-main"), filepath.Join(proj, ".aidignore")))
	out, stderr, code = aidRunEnv(t, proj, env, nil, ".", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, filePaths(out), "main.py", "the .aidignore outside the root must not be read")
}

// Unset, AID_CONFINE_ROOT changes nothing: a path outside the working
// directory and a symlink out of the tree are read as in v1.5.0 (on the
// command line the user chooses what to run on).
func TestConfineRootUnsetChangesNothing(t *testing.T) {
	tmp := confineTree(t)
	proj := filepath.Join(tmp, "repo", "proj")

	out, stderr, code := aidRunEnv(t, proj, aidEnv(), nil, ".", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"inlink.py", "leak.py", "main.py", "sub/inner.py"}, filePaths(out))
	assert.Contains(t, out, "secret_outside_the_repository()")

	out, stderr, code = aidRunEnv(t, proj, aidEnv(), nil, "../shared/lib.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, out, "shared_outside_the_confine_root()")

	// The output file goes to the repository top, as without the variable.
	_, stderr, code = aidRunEnv(t, proj, aidEnv(), nil, "main.py", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(tmp, "repo", ".aid", "aid.main.py.txt"))
}

// With AID_CONFINE_ROOT set, the project root (where .aid/ goes) is searched
// for no higher than the confine root, and the confine root is the fallback:
// output files and AI-action files land inside it although .git is above.
// AID_PROJECT_ROOT counts only inside it.
func TestConfineRootKeepsTheAidDirectoryInsideIt(t *testing.T) {
	tmp := confineTree(t)
	proj := filepath.Join(tmp, "repo", "proj")
	real, err := filepath.EvalSymlinks(proj)
	require.NoError(t, err)
	env := aidEnv("AID_CONFINE_ROOT=" + proj)

	_, stderr, code := aidRunEnv(t, filepath.Join(proj, "sub"), env, nil, "inner.py", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(real, ".aid", "aid.inner.py.txt"))
	assert.NoDirExists(t, filepath.Join(tmp, "repo", ".aid"))

	// From a working directory outside the root.
	_, stderr, code = aidRunEnv(t, tmp, env, nil, filepath.Join("repo", "proj", "main.py"), "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(real, ".aid", "aid.main.py.txt"))
	assert.NoDirExists(t, filepath.Join(tmp, ".aid"))

	// An AI action's default output (./.aid/...) too.
	_, stderr, code = aidRunEnv(t, proj, env, nil, ".", "--ai-action", "prompt-for-bug-hunting")
	require.Equal(t, 0, code, stderr)
	matches, err := filepath.Glob(filepath.Join(real, ".aid", "BUG-HUNTING-ANALYSIS.*.md"))
	require.NoError(t, err)
	assert.Len(t, matches, 1, stderr)
	assert.NoDirExists(t, filepath.Join(tmp, "repo", ".aid"))

	// AID_PROJECT_ROOT outside the root is ignored, inside it is used.
	_, stderr, code = aidRunEnv(t, tmp, append(env, "AID_PROJECT_ROOT="+tmp), nil, filepath.Join("repo", "proj", "sub", "inner.py"), "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Contains(t, stderr, "outside AID_CONFINE_ROOT. Ignoring.")
	assert.NoDirExists(t, filepath.Join(tmp, ".aid"))
	_, stderr, code = aidRunEnv(t, tmp, append(env, "AID_PROJECT_ROOT="+filepath.Join(proj, "sub")), nil, filepath.Join("repo", "proj", "main.py"), "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.FileExists(t, filepath.Join(proj, "sub", ".aid", "aid.main.py.txt"))
	assert.False(t, strings.Contains(stderr, "Ignoring"), stderr)
}
