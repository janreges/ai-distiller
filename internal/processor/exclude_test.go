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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldIncludeFile(tt.path, tt.include, tt.exclude)
			if got != tt.expected {
				t.Errorf("shouldIncludeFile(%q, %v, %v) = %v, want %v",
					tt.path, tt.include, tt.exclude, got, tt.expected)
			}
		})
	}
}
