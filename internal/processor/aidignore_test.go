package processor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
)

func TestAidignoreDefaultIgnoredDirs(t *testing.T) {
	// Test that default ignored directories are properly ignored
	tests := []struct {
		name     string
		dirname  string
		expected bool
	}{
		{"node_modules", "node_modules", true},
		{"vendor", "vendor", true},
		{"build", "build", true},
		{"dist", "dist", true},
		{"__pycache__", "__pycache__", true},
		{".vscode", ".vscode", true},
		{"regular src", "src", false},
		{"regular lib", "lib", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isDefaultIgnoredDir(tt.dirname); got != tt.expected {
				t.Errorf("isDefaultIgnoredDir(%q) = %v, want %v", tt.dirname, got, tt.expected)
			}
		})
	}
}

func TestAidignoreIntegration(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create test directory structure
	dirs := []string{
		"src",
		"vendor/lib",
		"node_modules/package",
		"build",
		"docs",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create test files
	files := map[string]string{
		"main.go":                       "package main",
		"src/app.go":                    "package src",
		"vendor/lib/vendor.go":          "package vendor",
		"node_modules/package/index.js": "export default {}",
		"build/output.go":               "package build",
		"README.md":                     "# README",
		"docs/API.md":                   "# API",
	}
	for file, content := range files {
		path := filepath.Join(tmpDir, file)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	tests := []struct {
		name             string
		aidignoreContent string
		expectedFiles    []string
	}{
		{
			name:             "Default behavior - vendor and node_modules ignored",
			aidignoreContent: "",
			expectedFiles:    []string{"README.md", "docs/API.md", "main.go", "src/app.go"},
		},
		{
			name:             "Include vendor with !vendor/",
			aidignoreContent: "!vendor/\n",
			expectedFiles:    []string{".aidignore", "README.md", "docs/API.md", "main.go", "src/app.go", "vendor/lib/vendor.go"},
		},
		{
			name:             "Include node_modules and exclude src",
			aidignoreContent: "!node_modules/\nsrc/\n",
			expectedFiles:    []string{".aidignore", "README.md", "docs/API.md", "main.go", "node_modules/package/index.js"},
		},
		{
			name:             "Include markdown files",
			aidignoreContent: "!*.md\n!**/*.md\n",
			expectedFiles:    []string{".aidignore", "README.md", "docs/API.md", "main.go", "src/app.go"},
		},
		{
			name:             "Include all default ignored directories",
			aidignoreContent: "!vendor/\n!node_modules/\n!build/\n",
			expectedFiles:    []string{".aidignore", "README.md", "build/output.go", "docs/API.md", "main.go", "node_modules/package/index.js", "src/app.go", "vendor/lib/vendor.go"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Write .aidignore file
			aidignorePath := filepath.Join(tmpDir, ".aidignore")
			if tt.aidignoreContent != "" {
				if err := os.WriteFile(aidignorePath, []byte(tt.aidignoreContent), 0644); err != nil {
					t.Fatalf("Failed to write .aidignore: %v", err)
				}
			} else {
				// Remove .aidignore if it exists
				os.Remove(aidignorePath)
			}

			// Process directory
			proc := NewWithContext(context.Background())
			opts := ProcessOptions{
				Recursive: true,
				RawMode:   true, // Use RawProcessor for all files
				// Limit to text files only for safety
				IncludePatterns: []string{"*.go", "*.js", "*.tsx", "*.md", ".aidignore"},
			}

			result, err := proc.processDirectory(tmpDir, opts)
			if err != nil {
				t.Fatalf("Failed to process directory: %v", err)
			}

			// Collect processed files
			var processedFiles []string
			collectFiles(result, &processedFiles, tmpDir)

			// Debug: print the result structure
			if result != nil {
				t.Logf("Result type: %T, Path: %s, Children: %d", result, result.Path, len(result.Children))
			}

			// Check if expected files match
			if !stringSlicesEqual(processedFiles, tt.expectedFiles) {
				t.Errorf("Processed files = %v, want %v", processedFiles, tt.expectedFiles)
			}
		})
	}
}

func TestAidignoreNestedFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create nested structure
	dirs := []string{
		"src/components",
		"src/tests",
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(tmpDir, dir), 0755); err != nil {
			t.Fatalf("Failed to create directory %s: %v", dir, err)
		}
	}

	// Create files
	files := map[string]string{
		"main.go":                   "package main",
		"src/app.go":                "package src",
		"src/components/Button.tsx": "export class Button {}",
		"src/tests/Button.test.tsx": "test('Button', () => {})",
		"test_main.go":              "package main",
	}
	for file, content := range files {
		path := filepath.Join(tmpDir, file)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	// Test with nested .aidignore
	// Root .aidignore
	rootIgnore := "# Root ignore\n"
	if err := os.WriteFile(filepath.Join(tmpDir, ".aidignore"), []byte(rootIgnore), 0644); err != nil {
		t.Fatalf("Failed to write root .aidignore: %v", err)
	}

	// Nested .aidignore in src/
	srcIgnore := "tests/\n*.test.*\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "src", ".aidignore"), []byte(srcIgnore), 0644); err != nil {
		t.Fatalf("Failed to write src/.aidignore: %v", err)
	}

	// Process directory
	proc := NewWithContext(context.Background())
	opts := ProcessOptions{
		Recursive: true,
		RawMode:   true, // Use RawProcessor for all files
		// Limit to text files only for safety
		IncludePatterns: []string{"*.go", "*.js", "*.tsx", "*.md", ".aidignore"},
	}

	result, err := proc.processDirectory(tmpDir, opts)
	if err != nil {
		t.Fatalf("Failed to process directory: %v", err)
	}

	// Collect processed files
	var processedFiles []string
	collectFiles(result, &processedFiles, tmpDir)

	expectedFiles := []string{".aidignore", "main.go", "src/.aidignore", "src/app.go", "src/components/Button.tsx", "test_main.go"}
	if !stringSlicesEqual(processedFiles, expectedFiles) {
		t.Errorf("Processed files = %v, want %v", processedFiles, expectedFiles)
	}
}

// Helper functions

func collectFiles(node ir.DistilledNode, files *[]string, baseDir string) {
	switch n := node.(type) {
	case *ir.DistilledFile:
		// Get relative path from base directory
		relPath, err := filepath.Rel(baseDir, n.Path)
		if err != nil {
			// If we can't get relative path, use the original path
			relPath = n.Path
		}
		*files = append(*files, relPath)
	case *ir.DistilledDirectory:
		for _, child := range n.Children {
			collectFiles(child, files, baseDir)
		}
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// Create maps for comparison
	aMap := make(map[string]bool)
	for _, s := range a {
		aMap[s] = true
	}

	for _, s := range b {
		if !aMap[s] {
			return false
		}
	}

	return true
}
