package processor

import (
	"testing"
)

func TestShouldIncludeFile_ExcludePatterns(t *testing.T) {
	tests := []struct {
		name            string
		filePath        string
		excludePatterns []string
		want            bool // true = include, false = exclude
	}{
		{
			name:            "migrations dir should be excluded by *migrations*",
			filePath:        "/project/migrations/001_initial.go",
			excludePatterns: []string{"*migrations*"},
			want:            false,
		},
		{
			name:            "db/migrations dir should be excluded by *migrations*",
			filePath:        "/project/db/migrations/001_initial.go",
			excludePatterns: []string{"*migrations*"},
			want:            false,
		},
		{
			name:            "regular file should not be excluded by *migrations*",
			filePath:        "/project/src/main.go",
			excludePatterns: []string{"*migrations*"},
			want:            true,
		},
		{
			name:            "file with migrations in name should be excluded",
			filePath:        "/project/models/migrations_users.go",
			excludePatterns: []string{"*migrations*"},
			want:            false,
		},
		{
			name:            "test file should be excluded by *test*",
			filePath:        "/project/src/components/Button.test.tsx",
			excludePatterns: []string{"*test*"},
			want:            false,
		},
		{
			name:            "test dir should be excluded by *test*",
			filePath:        "/project/src/tests/Button.tsx",
			excludePatterns: []string{"*test*"},
			want:            false,
		},
		{
			name:            "vendor file should be excluded",
			filePath:        "/project/vendor/package/lib.go",
			excludePatterns: []string{"vendor/**"},
			want:            false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shouldIncludeFile(tt.filePath, nil, tt.excludePatterns)
			if got != tt.want {
				t.Errorf("shouldIncludeFile(%q, nil, %v) = %v, want %v", 
					tt.filePath, tt.excludePatterns, got, tt.want)
			}
		})
	}
}
