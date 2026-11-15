package processor

import "testing"

func TestShouldIncludeFile_ExcludeDirectoryByName(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		include  []string
		exclude  []string
		expected bool
	}{
		// "**/" spans any number of directories, including none.
		{name: "include **/*.go deep", path: "a/b/c/x.go", include: []string{"**/*.go"}, expected: true},
		{name: "include **/*.go top level", path: "x.go", include: []string{"**/*.go"}, expected: true},
		{name: "include **/*.go other ext", path: "a/b/x.py", include: []string{"**/*.go"}, expected: false},
		{name: "exclude vendor/** deep", path: "pkg/vendor/a/b/x.go", exclude: []string{"vendor/**"}, expected: false},
		// Issue #9: a bare glob like "*migrations*" must also exclude files that
		// live inside a directory named "migrations", not only files whose
		// basename matches.
		{
			name:     "exclude *migrations* hits dir segment",
			path:     "app/migrations/0001_init.py",
			exclude:  []string{"*migrations*"},
			expected: false,
		},
		{
			name:     "exclude *migrations* still hits basename",
			path:     "app/migrations_helper.py",
			exclude:  []string{"*migrations*"},
			expected: false,
		},
		{
			name:     "exclude bare dir name excludes its contents",
			path:     "src/app/migrations/models.py",
			exclude:  []string{"migrations"},
			expected: false,
		},
		// Regression: a basename-only glob must not exclude unrelated files.
		{
			name:     "exclude *_test.go does not drop non-test file",
			path:     "internal/foo.go",
			exclude:  []string{"*_test.go"},
			expected: true,
		},
		{
			name:     "exclude *_test.go drops test file",
			path:     "internal/foo_test.go",
			exclude:  []string{"*_test.go"},
			expected: false,
		},
		{
			name:     "no patterns includes everything",
			path:     "any/where/file.py",
			expected: true,
		},
		{
			name:     "slash pattern still works (suffix dir match)",
			path:     "project/vendor/lib/x.go",
			exclude:  []string{"vendor/**"},
			expected: false,
		},
		// A slash pattern that matches a directory covers every file below it.
		{name: "exclude **/migrations covers the directory", path: "app/migrations/0001_init.py", exclude: []string{"**/migrations"}, expected: false},
		{name: "exclude **/migrations top-level directory", path: "migrations/0001_init.py", exclude: []string{"**/migrations"}, expected: false},
		{name: "exclude src/**/gen deep", path: "src/x/gen/a.go", exclude: []string{"src/**/gen"}, expected: false},
		{name: "exclude src/**/gen directly below src", path: "src/gen/a.go", exclude: []string{"src/**/gen"}, expected: false},
		{name: "exclude src/**/gen keeps a longer name", path: "src/x/generated/a.go", exclude: []string{"src/**/gen"}, expected: true},
		{name: "exclude vendor/* covers subdirectories", path: "vendor/lib/x.go", exclude: []string{"vendor/*"}, expected: false},
		{name: "include src/**/gen deep", path: "src/x/gen/a.go", include: []string{"src/**/gen"}, expected: true},
		{name: "include src/**/gen other directory", path: "src/x/other/a.go", include: []string{"src/**/gen"}, expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MatchesPatterns(tt.path, tt.include, tt.exclude)
			if got != tt.expected {
				t.Errorf("MatchesPatterns(%q, %v, %v) = %v, want %v",
					tt.path, tt.include, tt.exclude, got, tt.expected)
			}
		})
	}
}

// A pattern without "/" is matched below the walk root only. A pattern with
// "/" still matches the end of the full path, and the directories it covers
// are the ones below the walk root.
func TestMatchPatternsBelowWalkRoot(t *testing.T) {
	const full, rel = "/home/u/my-tests/app/sub/a.py", "sub/a.py"
	for _, tt := range []struct {
		exclude string
		keep    bool
	}{
		{"*test*", true},        // directory above the walk root
		{"app", true},           // the walk root itself
		{"*sub*", false},        // directory below the walk root
		{"app/sub/a.py", false}, // slash pattern: suffix of the full path
		{"app/*", false},        // covers app/sub, which is below the walk root
		{"u/*", true},           // would cover u/my-tests, which is above it
	} {
		if got := matchPatterns(rel, full, nil, []string{tt.exclude}); got != tt.keep {
			t.Errorf("exclude %q: kept = %v, want %v", tt.exclude, got, tt.keep)
		}
	}
}
