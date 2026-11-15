package importfilter

import (
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

// TestComprehensiveImportFiltering tests import filtering across all languages
func TestComprehensiveImportFiltering(t *testing.T) {
	testDataDir := "../../test-data/import-filtering"

	languages := []string{
		"python",
		"javascript",
		"typescript",
		"go",
		"java",
		"php",
		"ruby",
		"cpp",
		"csharp",
	}

	for _, lang := range languages {
		t.Run(lang, func(t *testing.T) {
			langDir := filepath.Join(testDataDir, lang)
			testLanguageImports(t, lang, langDir)
		})
	}
}

func testLanguageImports(t *testing.T, language, langDir string) {
	// Get the appropriate filter for the language
	filter, err := GetFilter(language)
	if err != nil {
		t.Skipf("No filter implemented for %s yet", language)
		return
	}

	// Read all test files for this language
	files, err := ioutil.ReadDir(langDir)
	if err != nil {
		t.Fatalf("Failed to read test directory %s: %v", langDir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		fileName := file.Name()
		if !hasCorrectExtension(fileName, language) {
			continue
		}

		t.Run(fileName, func(t *testing.T) {
			filePath := filepath.Join(langDir, fileName)
			testSingleFile(t, filter, filePath, language)
		})
	}
}

func testSingleFile(t *testing.T, filter ImportFilter, filePath, language string) {
	// Read the test file
	content, err := ioutil.ReadFile(filePath)
	if err != nil {
		t.Fatalf("Failed to read test file %s: %v", filePath, err)
	}

	code := string(content)

	// Test with implementation (default)
	t.Run("with_implementation", func(t *testing.T) {
		filtered, removed, err := filter.FilterUnusedImports(code, 2) // Debug level 2
		if err != nil {
			t.Fatalf("Filter error: %v", err)
		}

		// Log what was removed
		if len(removed) > 0 {
			t.Logf("Removed imports: %v", removed)
		}

		// Verify removed imports are not in the output
		for _, imp := range removed {
			if strings.Contains(filtered, imp) {
				t.Errorf("Removed import %q still appears in filtered output", imp)
			}
		}

		// Verify the output is valid code (basic check)
		if filtered == "" && code != "" {
			t.Error("Filter returned empty output for non-empty input")
		}
	})

	// Test without implementation (simulate --implementation=0)
	t.Run("without_implementation", func(t *testing.T) {
		// Remove function/method bodies to simulate no implementation
		codeWithoutImpl := removeImplementations(code, language)

		filtered, removed, err := filter.FilterUnusedImports(codeWithoutImpl, 1)
		if err != nil {
			t.Fatalf("Filter error: %v", err)
		}

		// Without implementation, more imports should be removed
		t.Logf("Removed imports (no impl): %v", removed)

		// In most cases, all imports should be removed when there's no implementation
		// unless they're used in type signatures, decorators, etc.
		if len(removed) == 0 && containsNonSideEffectImports(code, language) {
			t.Log("Warning: No imports removed even without implementation")
		}

		// Use filtered to avoid "declared and not used" error
		_ = filtered
	})
}

// removeImplementations removes function/method bodies to simulate --implementation=0
func removeImplementations(code, language string) string {
	// This is a simplified implementation
	// In reality, this would use the language parser to properly remove implementations

	switch language {
	case "python":
		// Simple approach: replace function bodies with pass
		lines := strings.Split(code, "\n")
		inFunction := false
		indentLevel := 0
		result := []string{}

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "def ") || strings.HasPrefix(trimmed, "async def ") {
				inFunction = true
				indentLevel = len(line) - len(strings.TrimLeft(line, " \t"))
				result = append(result, line)
				result = append(result, strings.Repeat(" ", indentLevel+4)+"pass")
			} else if inFunction && len(line) > indentLevel && strings.TrimSpace(line) != "" {
				// Skip function body
				continue
			} else {
				inFunction = false
				result = append(result, line)
			}
		}
		return strings.Join(result, "\n")

	case "go":
		// Replace function bodies with empty blocks
		// This is overly simplified
		return strings.ReplaceAll(code, "{", "{ }")

	default:
		// For other languages, return as-is for now
		return code
	}
}

// containsNonSideEffectImports checks if code has imports that aren't just side-effects
func containsNonSideEffectImports(code, language string) bool {
	switch language {
	case "javascript", "typescript":
		// Check for non-side-effect imports
		return strings.Contains(code, "from '") && !strings.Contains(code, "import '")
	case "python":
		return strings.Contains(code, "import ") || strings.Contains(code, "from ")
	case "go":
		// Check for non-blank imports
		return strings.Contains(code, "import ") && !strings.Contains(code, "import _ ")
	default:
		return strings.Contains(code, "import") || strings.Contains(code, "using")
	}
}

func hasCorrectExtension(fileName, language string) bool {
	extensions := map[string][]string{
		"python":     {".py"},
		"javascript": {".js"},
		"typescript": {".ts"},
		"go":         {".go"},
		"java":       {".java"},
		"php":        {".php"},
		"ruby":       {".rb"},
		"cpp":        {".cpp", ".cc", ".cxx"},
		"csharp":     {".cs"},
	}

	langExts, ok := extensions[language]
	if !ok {
		return false
	}

	for _, ext := range langExts {
		if strings.HasSuffix(fileName, ext) {
			return true
		}
	}

	return false
}

// Specific test cases for edge cases

func TestPythonTYPE_CHECKING(t *testing.T) {
	filter := NewPythonFilter()

	code := `
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from .models import User
    
def process(user: 'User') -> None:
    print("Processing user")
`

	filtered, removed, err := filter.FilterUnusedImports(code, 2)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}

	// TYPE_CHECKING imports should be kept when used in type annotations
	if strings.Contains(strings.Join(removed, " "), "User") {
		t.Error("User import from TYPE_CHECKING block was incorrectly removed")
	}

	// Use filtered to avoid "declared and not used" error
	_ = filtered
}

func TestJavaScriptSideEffectImports(t *testing.T) {
	filter := NewJavaScriptFilter()

	code := `
import React from 'react';
import './styles.css';  // Side-effect import
import 'core-js/stable';  // Polyfill side-effect
import unused from './unused';

function App() {
    return React.createElement('div', null, 'Hello');
}
`

	filtered, removed, err := filter.FilterUnusedImports(code, 2)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}

	// Side-effect imports should never be removed
	if strings.Contains(filtered, "./styles.css") == false {
		t.Error("Side-effect import './styles.css' was incorrectly removed")
	}

	if strings.Contains(filtered, "core-js/stable") == false {
		t.Error("Side-effect import 'core-js/stable' was incorrectly removed")
	}

	// Unused regular import should be removed
	if strings.Contains(filtered, "unused") {
		t.Error("Unused import 'unused' was not removed")
	}

	// Log what was removed for debugging
	t.Logf("Removed imports: %v", removed)
}

func TestGoBlankImports(t *testing.T) {
	filter := NewGoFilter()

	code := `package main

import (
    "fmt"
    _ "net/http/pprof"  // Blank import for side effects
    "strings"  // Unused
)

func main() {
    fmt.Println("Hello")
}
`

	filtered, removed, err := filter.FilterUnusedImports(code, 2)
	if err != nil {
		t.Fatalf("Filter error: %v", err)
	}

	// Blank imports should never be removed
	if !strings.Contains(filtered, `_ "net/http/pprof"`) {
		t.Error("Blank import was incorrectly removed")
	}

	// Unused regular import should be removed
	if strings.Contains(filtered, `"strings"`) && !strings.Contains(filtered, `_ "strings"`) {
		t.Error("Unused import 'strings' was not removed")
	}

	// Log what was removed for debugging
	t.Logf("Removed imports: %v", removed)
}
