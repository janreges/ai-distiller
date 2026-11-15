package ignore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIgnoreMatcher(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create test files and directories
	createTestStructure(t, tmpDir)

	// Create .aidignore file
	aidignoreContent := `# Test .aidignore file
*.log
*.tmp
temp/
build/
/secret.txt
node_modules/
**/*.bak
src/test_*
!important.log
*.min.js
dist/**
`

	err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(aidignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .aidignore: %v", err)
	}

	// Create matcher
	matcher, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		// Files that should be ignored
		{"test.log", true, "*.log pattern"},
		{"subdir/test.log", true, "*.log pattern in subdirectory"},
		{"file.tmp", true, "*.tmp pattern"},
		{"temp", true, "temp/ directory pattern"},
		{"temp/file.txt", true, "file in temp/ directory"},
		{"build", true, "build/ directory pattern"},
		{"build/output.exe", true, "file in build/ directory"},
		{"secret.txt", true, "/secret.txt absolute pattern"},
		{"node_modules", true, "node_modules/ directory"},
		{"node_modules/package/index.js", true, "file in node_modules/"},
		{"backup.bak", true, "**/*.bak pattern"},
		{"deep/nested/file.bak", true, "**/*.bak in nested directory"},
		{"src/test_file.go", true, "src/test_* pattern"},
		{"src/test_util.go", true, "src/test_* pattern"},
		{"script.min.js", true, "*.min.js pattern"},
		{"dist", true, "dist/** pattern"},
		{"dist/bundle.js", true, "file in dist/**"},
		{"dist/assets/style.css", true, "nested file in dist/**"},

		// Files that should NOT be ignored
		{"main.go", false, "regular file"},
		{"src/main.go", false, "regular file in src/"},
		{"important.log", false, "negated !important.log"},
		{"subdir/secret.txt", false, "/secret.txt doesn't match subdir"},
		{"src/helper.go", false, "doesn't match src/test_*"},
		{"regular.js", false, "doesn't match *.min.js"},
		{"README.md", false, "regular file"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			fullPath := filepath.Join(tmpDir, test.path)
			result := matcher.ShouldIgnore(fullPath)
			if result != test.expected {
				t.Errorf("ShouldIgnore(%s) = %v, want %v", test.path, result, test.expected)
			}
		})
	}
}

func TestNestedIgnoreFiles(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Create subdirectories
	subdir := filepath.Join(tmpDir, "subproject")
	os.MkdirAll(subdir, 0755)

	// Root .aidignore
	rootIgnore := `*.log
temp/
`
	err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(rootIgnore), 0644)
	if err != nil {
		t.Fatalf("Failed to create root .aidignore: %v", err)
	}

	// Nested .aidignore with additional patterns
	nestedIgnore := `*.tmp
local/
!debug.log
`
	err = os.WriteFile(filepath.Join(subdir, ".aidignore"), []byte(nestedIgnore), 0644)
	if err != nil {
		t.Fatalf("Failed to create nested .aidignore: %v", err)
	}

	// Create test files
	createFile(t, tmpDir, "root.log")
	createFile(t, tmpDir, "root.tmp")
	createFile(t, subdir, "sub.log")
	createFile(t, subdir, "sub.tmp")
	createFile(t, subdir, "debug.log")

	// Create matcher for root
	matcher, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	tests := []struct {
		path     string
		expected bool
		desc     string
	}{
		{"root.log", true, "root .aidignore matches *.log"},
		{"root.tmp", false, "root .aidignore doesn't have *.tmp"},
		{"subproject/sub.log", true, "inherited *.log pattern"},
		{"subproject/sub.tmp", true, "nested .aidignore adds *.tmp"},
		{"subproject/debug.log", false, "nested .aidignore negates debug.log"},
	}

	for _, test := range tests {
		t.Run(test.desc, func(t *testing.T) {
			fullPath := filepath.Join(tmpDir, test.path)
			result := matcher.ShouldIgnore(fullPath)
			if result != test.expected {
				t.Errorf("ShouldIgnore(%s) = %v, want %v", test.path, result, test.expected)
			}
		})
	}
}

func TestEmptyAndCommentLines(t *testing.T) {
	tmpDir := t.TempDir()

	// Create .aidignore with empty lines and comments
	aidignoreContent := `# This is a comment
*.log

# Another comment
  # Indented comment

*.tmp
  
# End comment`

	err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(aidignoreContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create .aidignore: %v", err)
	}

	matcher, err := New(tmpDir)
	if err != nil {
		t.Fatalf("Failed to create matcher: %v", err)
	}

	// Should only have 2 patterns (*.log and *.tmp)
	if len(matcher.patterns) != 2 {
		t.Errorf("Expected 2 patterns, got %d", len(matcher.patterns))
	}
}

// Helper functions

func createTestStructure(t *testing.T, root string) {
	t.Helper()

	// Create directories
	dirs := []string{
		"src",
		"temp",
		"build",
		"node_modules/package",
		"subdir",
		"deep/nested",
		"dist/assets",
		"subproject/local",
	}

	for _, dir := range dirs {
		err := os.MkdirAll(filepath.Join(root, dir), 0755)
		if err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create files
	files := []string{
		"main.go",
		"test.log",
		"file.tmp",
		"secret.txt",
		"important.log",
		"README.md",
		"src/main.go",
		"src/test_file.go",
		"src/test_util.go",
		"src/helper.go",
		"subdir/test.log",
		"subdir/secret.txt",
		"temp/file.txt",
		"build/output.exe",
		"node_modules/package/index.js",
		"backup.bak",
		"deep/nested/file.bak",
		"script.min.js",
		"regular.js",
		"dist/bundle.js",
		"dist/assets/style.css",
	}

	for _, file := range files {
		createFile(t, root, file)
	}
}

func createFile(t *testing.T, root, relPath string) {
	t.Helper()
	fullPath := filepath.Join(root, relPath)
	err := os.WriteFile(fullPath, []byte("test content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create file %s: %v", relPath, err)
	}
}
