package testrunner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pmezard/go-difflib/difflib"
)

// TestCase represents a single test case discovered from the filesystem
type TestCase struct {
	Language      string
	ScenarioName  string
	SourceFile    string
	ExpectedFile  string
	Flags         []string
}

// Runner is the main test runner that discovers and executes tests
type Runner struct {
	testDataDir string
	aidBinary   string
	updateMode  bool
}

// New creates a new test runner
func New(testDataDir, aidBinary string) *Runner {
	return &Runner{
		testDataDir: testDataDir,
		aidBinary:   aidBinary,
		updateMode:  os.Getenv("UPDATE_EXPECTED") == "true",
	}
}

// parseFlags extracts flags from expected filename based on parameter encoding
// e.g., "Test5-Complex.implementation=0.comments=0.public=1.expected" -> ["--implementation=0", "--comments=0", "--public=1"]
// Also supports simple aliases: "public.expected" -> ["--private=0"], "no_impl.expected" -> ["--implementation=0"]
func parseParametersFromFilename(filename string) []string {
	// Remove the .txt extension
	name := strings.TrimSuffix(filename, ".txt")
	
	// Handle simple aliases first (legacy naming support)
	simpleAliases := map[string][]string{
		"default":        {},
		"public":         {"--private=0"},
		"no_private":     {"--private=0"},
		"no_impl":        {"--implementation=0"},
		"public.no_impl": {"--private=0", "--implementation=0"},
		"no_private.no_impl": {"--private=0", "--implementation=0"},
		// Rust special cases
		"fixed_full":      {"--private=1", "--protected=1", "--internal=1", "--implementation=1"},
		"fixed_no_impl":   {"--private=1", "--protected=1", "--internal=1", "--implementation=0"},
		"fixed_no_private": {"--private=0", "--protected=0", "--internal=0", "--implementation=1"},
		// PHP special case
		"ideal_psr19_output": {"--private=0", "--protected=0", "--internal=0", "--implementation=0"},
	}
	
	if flags, exists := simpleAliases[name]; exists {
		// Sort flags for consistency
		sort.Strings(flags)
		return flags
	}
	
	// Parse parameters from filename
	// The filename can be in formats like:
	// - "default.txt" -> no flags
	// - "implementation=1.txt" -> single parameter
	// - "private=0,protected=0.txt" -> comma-separated parameters
	// - "private=1,protected=1,internal=1,implementation=0.txt" -> multiple parameters
	
	var flags []string
	
	// Handle comma-separated parameters
	if strings.Contains(name, ",") {
		// Split by comma first
		params := strings.Split(name, ",")
		for _, param := range params {
			param = strings.TrimSpace(param)
			if strings.Contains(param, "=") {
				flags = append(flags, "--"+param)
			}
		}
	} else if strings.Contains(name, "=") {
		// Single parameter
		flags = append(flags, "--"+name)
	}
	// else: no parameters (e.g., "default")
	
	// Sort flags so the order is deterministic
	sort.Strings(flags)
	return flags
}


// DiscoverTests walks the testdata directory and finds all test cases
func (r *Runner) DiscoverTests() ([]TestCase, error) {
	var tests []TestCase
	
	// Walk through each language directory
	entries, err := os.ReadDir(r.testDataDir)
	if err != nil {
		return nil, fmt.Errorf("reading testdata dir: %w", err)
	}
	
	for _, langEntry := range entries {
		if !langEntry.IsDir() {
			continue
		}
		
		language := langEntry.Name()
		langDir := filepath.Join(r.testDataDir, language)
		
		// Walk through each scenario directory
		scenarios, err := os.ReadDir(langDir)
		if err != nil {
			return nil, fmt.Errorf("reading language dir %s: %w", language, err)
		}
		
		for _, scenarioEntry := range scenarios {
			if !scenarioEntry.IsDir() {
				continue
			}
			
			scenarioName := scenarioEntry.Name()
			scenarioDir := filepath.Join(langDir, scenarioName)
			
			// Find source file
			sourceFile, err := findSourceFile(scenarioDir, language)
			if err != nil {
				continue // Skip scenarios without source files
			}
			
			// Find expected files - require expected/ directory
			expectedDir := filepath.Join(scenarioDir, "expected")
			if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
				if r.updateMode {
					// Create expected directory in update mode
					if err := os.MkdirAll(expectedDir, 0755); err != nil {
						return nil, fmt.Errorf("creating expected dir for %s/%s: %w", language, scenarioName, err)
					}
					// Create default expected file placeholder
					tests = append(tests, TestCase{
						Language:     language,
						ScenarioName: scenarioName,
						SourceFile:   sourceFile,
						ExpectedFile: filepath.Join(expectedDir, "default.txt"),
						Flags:        []string{},
					})
				}
				continue // Skip scenarios without expected directory unless in update mode
			}
			
			// Read all expected files
			expectedFiles, err := os.ReadDir(expectedDir)
			if err != nil {
				return nil, fmt.Errorf("reading expected dir for %s/%s: %w", language, scenarioName, err)
			}
			
			for _, expFile := range expectedFiles {
				if !strings.HasSuffix(expFile.Name(), ".txt") {
					continue
				}
				
				// Use new parameter-based parsing
				flags := parseParametersFromFilename(expFile.Name())
				tests = append(tests, TestCase{
					Language:     language,
					ScenarioName: scenarioName,
					SourceFile:   sourceFile,
					ExpectedFile: filepath.Join(expectedDir, expFile.Name()),
					Flags:        flags,
				})
			}
		}
	}
	
	return tests, nil
}

// findSourceFile finds the source file for a given language in a scenario directory
func findSourceFile(dir, language string) (string, error) {
	extensions := map[string][]string{
		"go":         {".go"},
		"python":     {".py"},
		"typescript": {".ts", ".tsx"},
		"javascript": {".js", ".jsx"},
		"java":       {".java"},
		"csharp":     {".cs"},
		"cpp":        {".cpp", ".cc", ".cxx", ".hpp", ".h"},
		"rust":       {".rs"},
		"swift":      {".swift"},
		"kotlin":     {".kt"},
		"php":        {".php"},
		"ruby":       {".rb"},
	}
	
	exts, ok := extensions[language]
	if !ok {
		return "", fmt.Errorf("unknown language: %s", language)
	}
	
	// Look for source.{ext} first
	for _, ext := range exts {
		sourcePath := filepath.Join(dir, "source"+ext)
		if _, err := os.Stat(sourcePath); err == nil {
			return sourcePath, nil
		}
	}
	
	// If not found, look for any file with the right extension
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		for _, ext := range exts {
			if strings.HasSuffix(entry.Name(), ext) {
				return filepath.Join(dir, entry.Name()), nil
			}
		}
	}
	
	return "", fmt.Errorf("no source file found for language %s in %s", language, dir)
}

// RunTest executes a single test case
func (r *Runner) RunTest(tc TestCase) error {
	// Make source file path relative to testdata directory
	relSourceFile, err := filepath.Rel(r.testDataDir, tc.SourceFile)
	if err != nil {
		// If we can't make it relative, just use the basename
		relSourceFile = filepath.Base(tc.SourceFile)
	}

	args := append([]string{relSourceFile}, tc.Flags...)
	args = append(args, "--format", "text", "--stdout", "--summary-type=off")

	cmd := exec.Command(r.aidBinary, args...)
	cmd.Dir = r.testDataDir

	// Capture output
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	
	// Run the command
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running aid: %w\nstderr: %s", err, stderr.String())
	}
	
	// Get actual output
	actual := stdout.Bytes()
	
	if r.updateMode {
		// Update expected file
		return os.WriteFile(tc.ExpectedFile, actual, 0644)
	}
	
	// Read expected output
	expected, err := os.ReadFile(tc.ExpectedFile)
	if err != nil {
		if os.IsNotExist(err) && r.updateMode {
			// Create the expected file in update mode
			return os.WriteFile(tc.ExpectedFile, actual, 0644)
		}
		return fmt.Errorf("reading expected file: %w", err)
	}
	
	// Compare outputs
	if !bytes.Equal(actual, expected) {
		return fmt.Errorf("%s", formatMismatch(tc, args, expected, actual))
	}
	
	return nil
}

// RunTests executes all discovered tests
func (r *Runner) RunTests(t *testing.T) {
	tests, err := r.DiscoverTests()
	if err != nil {
		t.Fatalf("discovering tests: %v", err)
	}
	
	if len(tests) == 0 {
		t.Log("No tests discovered")
		return
	}
	
	for _, tc := range tests {
		testName := fmt.Sprintf("%s/%s/%s", tc.Language, tc.ScenarioName, filepath.Base(tc.ExpectedFile))
		t.Run(testName, func(t *testing.T) {
			// Note: Previously skipped Swift empty output test - let's see what happens
			// if tc.Language == "swift" && tc.ScenarioName == "01_basic" && filepath.Base(tc.ExpectedFile) == "imports=0.txt" {
			//	t.Skip("Skipping Swift empty output test - needs fix for empty file handling")
			// }
			
			if err := r.RunTest(tc); err != nil {
				t.Errorf("test failed: %v", err)
			}
		})
	}
}

// formatMismatch builds a failure message with the expected file path, the
// command to reproduce the run, and a unified diff of expected vs actual.
func formatMismatch(tc TestCase, args []string, expected, actual []byte) string {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(string(expected)),
		B:        difflib.SplitLines(string(actual)),
		FromFile: "expected",
		ToFile:   "actual",
		Context:  3,
	})
	if err != nil {
		diff = fmt.Sprintf("(could not compute diff: %v)", err)
	}
	return fmt.Sprintf("output mismatch\nexpected file: %s\ncommand: (cd testdata && aid %s </dev/null)\n%s",
		tc.ExpectedFile, strings.Join(args, " "), diff)
}
