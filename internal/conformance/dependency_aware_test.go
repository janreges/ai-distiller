package conformance

import (
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
