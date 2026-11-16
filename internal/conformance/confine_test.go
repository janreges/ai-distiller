package conformance

import (
	"os"
	"path/filepath"
	"regexp"
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

// --dependency-aware stops at AID_CONFINE_ROOT when it lies below the
// repository top: for every language it resolves through project files, an
// import that leads out of the confine root (a relative path, a go.mod
// replace, a Cargo path dependency, tsconfig paths, PSR-4 roots) is not
// read and -v says so, and a go.mod or jsconfig.json above the confine root
// is not read either. Unset, the same imports are followed up to the
// repository top.
func TestConfineRootBoundsDependencyAware(t *testing.T) {
	tmp := t.TempDir()
	writeProject(t, tmp, map[string]string{
		"repo/.git/HEAD": "ref: refs/heads/main\n",

		"repo/proj/py/app.py":   "from .local import thing\nfrom ...pyshared import helper\n\ndef app():\n    return thing, helper\n",
		"repo/proj/py/local.py": "def thing():\n    return 1\n",
		"repo/pyshared.py":      "def py_outside_the_confine_root():\n    return 2\n",

		"repo/proj/go/go.mod":        "module example.com/app\n\ngo 1.22\n\nreplace example.com/kit => ../../shared/kit\n",
		"repo/proj/go/main.go":       "package main\n\nimport (\n\t\"example.com/app/util\"\n\t\"example.com/kit/log\"\n)\n\nfunc main() { util.Util(); log.Log() }\n",
		"repo/proj/go/util/util.go":  "package util\n\nfunc Util() {}\n",
		"repo/shared/kit/log/log.go": "package log\n\nfunc GoOutsideTheConfineRoot() {}\n",

		"repo/go.mod":                       "module example.com/repo\n\ngo 1.22\n",
		"repo/proj/goup/main.go":            "package main\n\nimport \"example.com/repo/proj/goup/inner\"\n\nfunc main() { inner.Inner() }\n",
		"repo/proj/goup/inner/inner.go":     "package inner\n\nfunc Inner() {}\n",
		"repo/proj/rust/Cargo.toml":         "[package]\nname = \"app\"\n\n[dependencies]\nkit = { path = \"../../shared/kit-rs\" }\n",
		"repo/proj/rust/src/main.rs":        "mod cli;\nuse kit::helper;\n\nfn main() {}\n",
		"repo/proj/rust/src/cli.rs":         "pub fn run() {}\n",
		"repo/shared/kit-rs/src/lib.rs":     "pub fn rust_outside_the_confine_root() {}\n",
		"repo/proj/ts/tsconfig.json":        `{"compilerOptions": {"baseUrl": ".", "paths": {"@shared/*": ["../../shared/ts/*"], "@/*": ["src/*"]}}}` + "\n",
		"repo/proj/ts/src/main.ts":          "import { local } from '@/local';\nimport { types } from '@shared/types';\nimport { rel } from '../../../shared/ts/rel';\nexport function main() { return [local, types, rel]; }\n",
		"repo/proj/ts/src/local.ts":         "export function local() { return 1; }\n",
		"repo/shared/ts/types.ts":           "export function tsOutsideTheConfineRoot() { return 2; }\n",
		"repo/shared/ts/rel.ts":             "export function relOutsideTheConfineRoot() { return 3; }\n",
		"repo/jsconfig.json":                `{"compilerOptions": {"baseUrl": ".", "paths": {"~/*": ["proj/js/*"]}}}` + "\n",
		"repo/proj/js/app.js":               "import { inside } from '~/lib/inside.js';\nimport { local } from './local.js';\nexport function app() { return [inside, local]; }\n",
		"repo/proj/js/local.js":             "export function local() { return 1; }\n",
		"repo/proj/js/lib/inside.js":        "export function inside() { return 2; }\n",
		"repo/proj/php/composer.json":       `{"autoload": {"psr-4": {"App\\": "src/", "Shared\\": "../../shared/php/"}}}` + "\n",
		"repo/proj/php/src/User.php":        "<?php\nnamespace App;\n\nuse App\\Models\\Post;\nuse Shared\\Secret;\n\nrequire_once __DIR__ . '/../../../shared/php/lib.php';\n\nclass User {}\n",
		"repo/proj/php/src/Models/Post.php": "<?php\nnamespace App\\Models;\n\nclass Post {}\n",
		"repo/shared/php/Secret.php":        "<?php\nnamespace Shared;\n\nclass PhpOutsideTheConfineRoot {}\n",
		"repo/shared/php/lib.php":           "<?php\nfunction lib_outside_the_confine_root() {}\n",
	})
	proj := filepath.Join(tmp, "repo", "proj")
	confined := aidEnv("AID_CONFINE_ROOT=" + proj)

	for _, tc := range []struct {
		entry      string
		confined   []string
		unset      []string
		outsideLog int // imports -v reports outside the confine root
	}{
		{"py/app.py", []string{"app.py", "local.py"}, []string{"app.py", "local.py", "../../pyshared.py"}, 1},
		{"go/main.go", []string{"main.go", "util/util.go"}, []string{"main.go", "util/util.go", "../../shared/kit/log/log.go"}, 1},
		{"goup/main.go", []string{"main.go"}, []string{"main.go", "inner/inner.go"}, 0},
		{"rust/src/main.rs", []string{"main.rs", "cli.rs"}, []string{"main.rs", "cli.rs", "../../../shared/kit-rs/src/lib.rs"}, 1},
		{"ts/src/main.ts", []string{"main.ts", "local.ts"}, []string{"main.ts", "local.ts", "../../../shared/ts/types.ts", "../../../shared/ts/rel.ts"}, 2},
		{"js/app.js", []string{"app.js", "local.js"}, []string{"app.js", "lib/inside.js", "local.js"}, 0},
		{"php/src/User.php", []string{"User.php", "Models/Post.php"}, []string{"User.php", "Models/Post.php", "../../../shared/php/Secret.php", "../../../shared/php/lib.php"}, 2},
	} {
		t.Run(tc.entry, func(t *testing.T) {
			out, stderr, code := aidRunEnv(t, proj, confined, nil, tc.entry, "--dependency-aware", "--stdout", "--summary-type=off", "-v")
			require.Equal(t, 0, code, stderr)
			assert.Equal(t, tc.confined, filePaths(out))
			assert.NotRegexp(t, `(?i)outside_?the_?confine_?root`, out)
			assert.Len(t, regexp.MustCompile(`Not following \S+ \(imported by \S+\): outside the AID_CONFINE_ROOT directory \S+/proj\n`).FindAllString(stderr, -1), tc.outsideLog, stderr)

			out, stderr, code = aidRunEnv(t, proj, aidEnv(), nil, tc.entry, "--dependency-aware", "--stdout", "--summary-type=off")
			require.Equal(t, 0, code, stderr)
			assert.Equal(t, tc.unset, filePaths(out))
		})
	}
}
