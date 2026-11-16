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

// The --dependency-aware fixtures live in testdata/depaware/<lang>/: an entry
// file and the project files it imports, a few levels deep.

var fileHeader = regexp.MustCompile(`(?m)^<file path="([^"]+)">`)

// depAwareDir returns testdata/depaware, the working directory of these runs.
func depAwareDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(conformanceData(t), "depaware")
}

// filePaths lists the paths of the <file> blocks of a text output, in order.
func filePaths(out string) []string {
	var paths []string
	for _, m := range fileHeader.FindAllStringSubmatch(out, -1) {
		paths = append(paths, m[1])
	}
	return paths
}

// runDepAware runs aid with --dependency-aware on args from testdata/depaware
// and fails unless it exits 0.
func runDepAware(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	full := append(append([]string{}, args...), "--dependency-aware", "--stdout", "--summary-type=off")
	stdout, stderr, code := aidRun(t, depAwareDir(t), nil, full...)
	require.Equal(t, 0, code, "aid %s\nstderr: %s", strings.Join(full, " "), stderr)
	return stdout, stderr
}

const noImportsNote = "Note: --dependency-aware followed no imports"

func TestDependencyAwareFollowsImportsWithRelativePaths(t *testing.T) {
	out, stderr := runDepAware(t, "python/app/main.py")
	assert.NotContains(t, stderr, noImportsNote)
	// The entry file first, as without the flag, then the files it imports
	// (depth 1) and the file those import (depth 2), each once.
	assert.Equal(t, []string{"main.py", "models.py", "services/mailer.py", "services/templates.py"}, filePaths(out))
	assert.NotContains(t, out, depAwareDir(t), "paths must be relative")

	plain, _, code := aidRun(t, depAwareDir(t), nil, "python/app/main.py", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.True(t, strings.HasPrefix(out, plain), "the entry file's block must be the output without the flag\nwith: %s\nwithout: %s", out, plain)

	// Followed files are distilled like the entry file: signatures without
	// bodies by default, never source text in comments.
	assert.Contains(t, out, "render(name: str, value: str) -> str")
	assert.NotContains(t, out, "return name")
	assert.NotContains(t, out, "// ")
}

func TestDependencyAwareBodiesFollowImplementationFlag(t *testing.T) {
	out, _ := runDepAware(t, "python/app/main.py", "--implementation=1")
	assert.Contains(t, out, "\n    return name + \": \" + value\n")
	assert.Contains(t, out, "\n    body = render(\"welcome\", user.greeting())\n")
	assert.NotContains(t, out, "// ")
}

func TestDependencyAwareMaxDepthLimitsImportLevels(t *testing.T) {
	out, _ := runDepAware(t, "python/app/main.py", "--max-depth=1")
	assert.Equal(t, []string{"main.py", "models.py", "services/mailer.py"}, filePaths(out))

	_, stderr, code := aidRun(t, depAwareDir(t), nil, "python/app/main.py", "--dependency-aware", "--max-depth=0", "--stdout")
	assert.NotEqual(t, 0, code)
	assert.Contains(t, stderr, "--max-depth must be 1 or more with --dependency-aware (got 0)")
}

func TestDependencyAwareDirectoryAddsOnlyFilesOutsideIt(t *testing.T) {
	out, stderr := runDepAware(t, "python/app/services")
	assert.NotContains(t, stderr, noImportsNote)
	// The directory's own files once, then the file outside it they import.
	assert.Equal(t, []string{"mailer.py", "templates.py", "../models.py"}, filePaths(out))
}

// Go: the other files of the entry's package, then the packages of the
// module (go.mod's module path) and of a module go.mod replaces with a local
// directory; never _test.go files or a "package main" generator next to a
// library.
func TestDependencyAwareFollowsGoPackages(t *testing.T) {
	out, stderr := runDepAware(t, "go/shop/cmd/shop/main.go")
	assert.NotContains(t, stderr, noImportsNote)
	assert.Equal(t, []string{
		"main.go",
		"flags.go",                // same package
		"../../../kit/log/log.go", // replace example.com/kit => ../kit
		"../../store/cache.go",    // example.com/shop/store
		"../../store/store.go",    //
		"../../internal/db/db.go", // imported by store.go: depth 2
	}, filePaths(out))
	assert.Contains(t, out, "func Dial(dsn string) *Conn")

	out, _ = runDepAware(t, "go/shop/cmd/shop/main.go", "--max-depth=1")
	assert.NotContains(t, filePaths(out), "../../internal/db/db.go")
	assert.Len(t, filePaths(out), 5)
}

// Go imports are read by go/parser: an import block on one line counts, and
// a string literal after the imports is no import.
func TestDependencyAwareReadsGoImportsInAnyLayout(t *testing.T) {
	tmp := t.TempDir()
	writeProject(t, tmp, map[string]string{
		".git/HEAD": "ref: refs/heads/main\n",
		"go.mod":    "module m\n\ngo 1.22\n",
		"main.go":   "package main\n\nimport (\"fmt\"; \"m/a\")\n\nvar names = []string{\n\t\"m/b\",\n}\n\nfunc main() { fmt.Println(a.A(), names) }\n",
		"a/a.go":    "package a\n\nfunc A() int { return 1 }\n",
		"b/b.go":    "package b\n\nfunc B() int { return 2 }\n",
	})
	out, stderr, code := aidRun(t, tmp, nil, "main.go", "--dependency-aware", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.go", "a/a.go"}, filePaths(out))
}

// Rust: "mod x;" files (not #[cfg(test)] ones), and the module files named
// by crate::, super::, the crate's own library name and a path dependency's
// name, in use declarations and in expressions; not paths in doc comments.
func TestDependencyAwareFollowsRustModules(t *testing.T) {
	out, stderr := runDepAware(t, "rust/app/src/main.rs")
	assert.NotContains(t, stderr, noImportsNote)
	assert.Equal(t, []string{
		"main.rs",
		"cli.rs",               // mod cli;
		"store/mod.rs",         // use demo_app::store::Store
		"../../kit/src/lib.rs", // use util_kit::slug (path dependency)
		"report.rs",            // demo_app::report::summary(...)
		"store/disk.rs",        // mod disk; in store/mod.rs: depth 2
	}, filePaths(out))
	assert.Contains(t, out, "pub fn normalize(path: &str) -> String")

	out, _ = runDepAware(t, "rust/app/src/main.rs", "--max-depth=1")
	assert.Equal(t, []string{"main.rs", "cli.rs", "store/mod.rs", "../../kit/src/lib.rs", "report.rs"}, filePaths(out))

	// From a module file, super:: and crate:: lead back to the crate's files.
	out, _ = runDepAware(t, "rust/app/src/store/mod.rs", "--max-depth=1")
	assert.Equal(t, []string{"mod.rs", "disk.rs", "../report.rs"}, filePaths(out))
}

// TypeScript and JavaScript: import/export ... from (also over several
// lines), dynamic import() and require(), relative or through
// compilerOptions.paths of tsconfig.json (with comments, trailing commas and
// "extends"); "./x.js" finds x.ts, "./app" finds app/index.tsx. Not CSS, not
// npm packages, not an import inside a doc comment.
func TestDependencyAwareFollowsTypeScriptImports(t *testing.T) {
	out, stderr := runDepAware(t, "ts/src/main.tsx")
	assert.NotContains(t, stderr, noImportsNote)
	assert.Equal(t, []string{
		"main.tsx",
		"app/index.tsx",         // './app'
		"lib/format.ts",         // '@/lib/format', a multi-line import
		"lib/api.ts",            // export ... from '@/lib/api.js'
		"lazy/page.tsx",         // import('./lazy/page')
		"components/button.jsx", // imported by app/index.tsx: depth 2
		"lib/http.cjs",          // require('./http.cjs') in lib/api.ts: depth 2
	}, filePaths(out))
	assert.Contains(t, out, "export function formatPrice(price: Price): string")

	out, _ = runDepAware(t, "ts/src/main.tsx", "--max-depth=1")
	assert.Equal(t, []string{"main.tsx", "app/index.tsx", "lib/format.ts", "lib/api.ts", "lazy/page.tsx"}, filePaths(out))
}

// PHP: classes named in use imports (grouped, aliased), trait use, extends
// and implements, found through the PSR-4 autoload of every composer.json
// from the file up to the repository top (a package's own and the root's),
// and require_once __DIR__ . '...'. Not "use function", not vendor classes,
// not a commented-out import.
func TestDependencyAwareFollowsPhpClasses(t *testing.T) {
	out, stderr := runDepAware(t, "php/src/Models/User.php")
	assert.NotContains(t, stderr, noImportsNote)
	assert.Equal(t, []string{
		"User.php",
		"../Contracts/Arrayable.php",         // use App\Contracts\{Arrayable, ...}
		"../Contracts/Jsonable.php",          // ... Jsonable as Json}
		"../../packages/support/src/Str.php", // use App\Support\Str (autoload-dev, a list)
		"../bootstrap.php",                   // require_once __DIR__ . '/../bootstrap.php'
		"Model.php",                          // extends Model: the file's namespace
		"Concerns/HasName.php",               // use Concerns\HasName; inside the class
		"../Text/Inflector.php",              // Str.php's import, mapped by the root composer.json: depth 2
		"../Concerns/Timestamps.php",         // use \App\Concerns\Timestamps; in Model.php: depth 2
	}, filePaths(out))
	assert.Contains(t, out, "public static capitalize(string $value): string")

	out, _ = runDepAware(t, "php/src/Models/User.php", "--max-depth=1")
	assert.Len(t, filePaths(out), 7)
}

// writeProject creates files (slash path -> content) under root.
func writeProject(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
	}
}

// Following never leaves the repository (the nearest directory holding .git
// above the input): an import that resolves outside it, directly or through
// a symlink, is not read, -v says so, and the stderr note counts it.
func TestDependencyAwareStaysInsideTheRepository(t *testing.T) {
	tmp := t.TempDir()
	writeProject(t, tmp, map[string]string{
		"repo/.git/HEAD": "ref: refs/heads/main\n",
		"repo/app/main.js": "import { local } from './local.js';\n" +
			"import { secret } from '../../outside/secret.js';\n" +
			"import { linked } from './linked.js';\n" +
			"export function main() { return [local, secret, linked]; }\n",
		"repo/app/local.js": "export function local() { return 1; }\n",
		"repo/app/only.js":  "import { secret } from '../../outside/secret.js';\nexport function only() { return secret; }\n",
		"outside/secret.js": "export function secretOutsideTheRepository() { return 2; }\n",
		"outside/target.js": "export function linkedOutsideTheRepository() { return 3; }\n",
	})
	require.NoError(t, os.Symlink(filepath.Join(tmp, "outside", "target.js"), filepath.Join(tmp, "repo", "app", "linked.js")))
	repo := filepath.Join(tmp, "repo")

	out, stderr, code := aidRun(t, repo, nil, "app/main.js", "--dependency-aware", "--stdout", "--summary-type=off", "-v")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.js", "local.js"}, filePaths(out))
	assert.NotContains(t, out, "OutsideTheRepository")
	assert.Regexp(t, `Not following \S+/outside/secret\.js \(imported by \S+/app/main\.js\): outside the repository \S+/repo\n`, stderr)
	assert.Regexp(t, `Not following \S+/app/linked\.js \(imported by \S+/app/main\.js\): outside the repository`, stderr)

	// The only candidate lies outside: the entry alone and the note.
	out, stderr, code = aidRun(t, repo, nil, "app/only.js", "--dependency-aware", "--stdout", "--summary-type=off")
	require.Equal(t, 0, code)
	assert.Equal(t, []string{"only.js"}, filePaths(out))
	assert.Equal(t, noImportsNote+" of app/only.js: none resolved to a project file aid can distill"+
		" (skipped 1 outside the repository; -v lists them)\n", stderr)
}

// Without .git above the input, the boundary is the project root aid
// detects from the working directory (here package.json).
func TestDependencyAwareWithoutGitStaysInsideTheProjectRoot(t *testing.T) {
	tmp := t.TempDir()
	for d := tmp; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			t.Skipf("%s holds .git, so the temporary directory is inside a repository", d)
		}
		if filepath.Dir(d) == d {
			break
		}
	}
	writeProject(t, tmp, map[string]string{
		"proj/package.json": "{}\n",
		"proj/src/main.js":  "import { util } from './util.js';\nimport { shared } from '../../shared/lib.js';\nexport function main() { return [util, shared]; }\n",
		"proj/src/util.js":  "export function util() { return 1; }\n",
		"shared/lib.js":     "export function sharedOutsideTheProject() { return 2; }\n",
	})
	proj := filepath.Join(tmp, "proj")
	out, stderr, code := aidRun(t, proj, nil, "src/main.js", "--dependency-aware", "--stdout", "--summary-type=off", "-v")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.js", "util.js"}, filePaths(out))
	assert.NotContains(t, out, "OutsideTheProject")
	assert.Regexp(t, `Not following \S+/shared/lib\.js \(imported by \S+/src/main\.js\): outside the project root \S+/proj\n`, stderr)
}

// aidEnv returns the test's environment without AID_PROJECT_ROOT and
// AID_CONFINE_ROOT, plus set ("NAME=value").
func aidEnv(set ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "AID_PROJECT_ROOT=") || strings.HasPrefix(kv, "AID_CONFINE_ROOT=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, set...)
}

// skipUnlessNoMarkerAbove skips the test when dir or a directory above it
// holds a project-root marker, which would make aid find a project root.
func skipUnlessNoMarkerAbove(t *testing.T, dir string) {
	t.Helper()
	markers := []string{".aidrc", "go.mod", "package.json", "Cargo.toml", "pyproject.toml", "setup.py", "pom.xml", "build.gradle", ".git"}
	for d := dir; ; d = filepath.Dir(d) {
		for _, m := range markers {
			if _, err := os.Stat(filepath.Join(d, m)); err == nil {
				t.Skipf("%s holds %s, so aid would find a project root above the temporary directory", d, m)
			}
		}
		if filepath.Dir(d) == d {
			break
		}
	}
}

// Without .git and without a project-root marker, aid falls back to the
// working directory as the project root; that fallback is no boundary (run
// from "/", it would be the whole disk), so following stays in the input's
// directory. The output path asks for the project root first, so the
// default-output run checks the cached root too.
func TestDependencyAwareWithoutAnyMarkerStaysInTheInputDirectory(t *testing.T) {
	tmp := t.TempDir()
	skipUnlessNoMarkerAbove(t, tmp)
	writeProject(t, tmp, map[string]string{
		"loose/app/src/main.js": "import { util } from './util.js';\n" +
			"import { secret } from '../../../outside/secret.js';\n" +
			"export function main() { return [util, secret]; }\n",
		"loose/app/src/util.js": "export function util() { return 1; }\n",
		"outside/secret.js":     "export function secretOutsideTheInputDirectory() { return 2; }\n",
	})

	out, stderr, code := aidRunEnv(t, tmp, aidEnv(), nil, "loose/app/src/main.js", "--dependency-aware", "--stdout", "--summary-type=off", "-v")
	require.Equal(t, 0, code, stderr)
	assert.Equal(t, []string{"main.js", "util.js"}, filePaths(out))
	assert.NotContains(t, out, "OutsideTheInputDirectory")
	assert.Regexp(t, `Not following \S+/outside/secret\.js \(imported by \S+/src/main\.js\): outside the input directory \S+/loose/app/src\n`, stderr)

	_, stderr, code = aidRunEnv(t, tmp, aidEnv(), nil, "loose/app/src/main.js", "--dependency-aware", "--summary-type=off")
	require.Equal(t, 0, code, stderr)
	saved, err := os.ReadFile(filepath.Join(tmp, ".aid", "aid.main.js.txt"))
	require.NoError(t, err, stderr)
	assert.Equal(t, []string{"main.js", "util.js"}, filePaths(string(saved)))
}

// Followed files pass the selection rules of a directory walk: --include,
// --exclude, every .aidignore from the repository top down to the file's
// directory, and the directories a walk skips by default. The patterns
// match the printed path without its leading "../" segments, so the
// directories that hold the input never match. The input itself is not
// filtered, as in a run without the flag.
func TestDependencyAwareFollowsOnlySelectedFiles(t *testing.T) {
	// select/src/main.js imports ../generated/api.js (select/.aidignore:
	// generated/), ./secret.js (select/src/.aidignore), ./vendor/helper.js
	// (vendor is skipped by default), ./util.js and ./util.test.js.
	out, stderr := runDepAware(t, "select/src/main.js", "-v")
	assert.Equal(t, []string{"main.js", "util.js", "util.test.js"}, filePaths(out))
	assert.Regexp(t, `Not following \S+/select/generated/api\.js \(imported by \S+\): excluded by \S*select/\.aidignore\n`, stderr)
	assert.Regexp(t, `Not following \S+/select/src/secret\.js \(imported by \S+\): excluded by \S*select/src/\.aidignore\n`, stderr)
	assert.Regexp(t, `Not following \S+/select/src/vendor/helper\.js \(imported by \S+\): excluded by the directory vendor`, stderr)

	out, _ = runDepAware(t, "select/src/main.js", "--exclude", "*.test.js")
	assert.Equal(t, []string{"main.js", "util.js"}, filePaths(out))
	out, _ = runDepAware(t, "select/src/main.js", "--include", "*.test.js")
	assert.Equal(t, []string{"main.js", "util.test.js"}, filePaths(out))

	// A directory: what its walk leaves out is not brought back by an import.
	out, stderr = runDepAware(t, "select/src")
	assert.Equal(t, []string{"main.js", "util.js", "util.test.js"}, filePaths(out))
	assert.Contains(t, stderr, noImportsNote+" out of select/src: none resolved to a file outside it that aid can distill"+
		" (skipped 3 excluded by --include, --exclude, .aidignore or a default-ignored directory; -v lists them)\n")
	out, _ = runDepAware(t, "python/app/services", "--exclude", "templates.py")
	assert.Equal(t, []string{"mailer.py", "../models.py"}, filePaths(out))

	// "store" matches ../../store/*.go; an excluded file is not read, so
	// db.go, which only store.go imports, is not reached. "*shop*" matches
	// only directories that hold the input, which never count.
	out, _ = runDepAware(t, "go/shop/cmd/shop/main.go", "--exclude", "store")
	assert.Equal(t, []string{"main.go", "flags.go", "../../../kit/log/log.go"}, filePaths(out))
	out, _ = runDepAware(t, "go/shop/cmd/shop/main.go", "--exclude", "*shop*")
	assert.Len(t, filePaths(out), 6)
}

func TestDependencyAwareSaysWhenNothingIsFollowed(t *testing.T) {
	// index.js imports only npm packages: nothing to follow, but the entry
	// file is still printed and the run succeeds.
	out, stderr := runDepAware(t, "js/index.js")
	assert.Equal(t, []string{"index.js"}, filePaths(out))
	assert.Contains(t, out, "mount(element)")
	assert.Contains(t, stderr, noImportsNote+" of js/index.js: none resolved to a project file aid can distill\n")
	assert.Equal(t, 1, strings.Count(stderr, "\n"), "the note is one line: %q", stderr)

	out, stderr = runDepAware(t, "js")
	assert.Equal(t, []string{"index.js"}, filePaths(out))
	assert.Contains(t, stderr, noImportsNote+" out of js:")
}
