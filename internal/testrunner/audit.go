package testrunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AuditResult represents issues found during audit
type AuditResult struct {
	Issues              []string
	MissingExpectedDirs []string
	EmptyExpectedDirs   []string
	UnparsableFiles     []string
	DuplicateScenarios  []string
	InconsistentNaming  []string
}

// Audit walks the testdata directory and checks for consistency issues
func (r *Runner) Audit() (*AuditResult, error) {
	result := &AuditResult{}

	scenarioNames := make(map[string][]string) // normalized name -> list of paths

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
			scenarioPath := filepath.Join(langDir, scenarioName)
			fullPath := fmt.Sprintf("%s/%s", language, scenarioName)

			// Check for naming consistency
			if err := r.checkNamingConsistency(scenarioName, result); err != nil {
				result.InconsistentNaming = append(result.InconsistentNaming, fmt.Sprintf("%s: %v", fullPath, err))
			}

			// Track for duplicate detection
			normalized := normalizeScenarioName(scenarioName)
			scenarioNames[normalized] = append(scenarioNames[normalized], fullPath)

			// Check for source file
			if _, err := findSourceFile(scenarioPath, language); err != nil {
				result.Issues = append(result.Issues, fmt.Sprintf("%s: no source file found", fullPath))
				continue
			}

			// Check expected directory
			expectedDir := filepath.Join(scenarioPath, "expected")
			if _, err := os.Stat(expectedDir); os.IsNotExist(err) {
				result.MissingExpectedDirs = append(result.MissingExpectedDirs, fullPath)
				continue
			}

			// Check if expected directory has files
			expectedFiles, err := os.ReadDir(expectedDir)
			if err != nil {
				result.Issues = append(result.Issues, fmt.Sprintf("%s: cannot read expected dir: %v", fullPath, err))
				continue
			}

			hasExpectedFiles := false
			for _, expFile := range expectedFiles {
				if !strings.HasSuffix(expFile.Name(), ".txt") {
					continue
				}
				hasExpectedFiles = true

				// Test if filename can be parsed
				flags := parseParametersFromFilename(expFile.Name())
				if len(flags) == 0 && expFile.Name() != "default.txt" {
					// Check if it's a simple alias we don't recognize
					name := strings.TrimSuffix(expFile.Name(), ".txt")
					simpleAliases := []string{"default", "public", "no_private", "no_impl", "public.no_impl", "no_private.no_impl"}
					isKnownAlias := false
					for _, alias := range simpleAliases {
						if name == alias {
							isKnownAlias = true
							break
						}
					}
					if !isKnownAlias && !strings.Contains(name, "=") {
						result.UnparsableFiles = append(result.UnparsableFiles, fmt.Sprintf("%s/%s: cannot parse filename", fullPath, expFile.Name()))
					}
				}
			}

			if !hasExpectedFiles {
				result.EmptyExpectedDirs = append(result.EmptyExpectedDirs, fullPath)
			}
		}
	}

	// Check for duplicates within the same language only
	languageScenarios := make(map[string]map[string][]string) // language -> normalized name -> paths
	for normalized, paths := range scenarioNames {
		for _, path := range paths {
			parts := strings.Split(path, "/")
			if len(parts) >= 2 {
				lang := parts[0]
				if languageScenarios[lang] == nil {
					languageScenarios[lang] = make(map[string][]string)
				}
				languageScenarios[lang][normalized] = append(languageScenarios[lang][normalized], path)
			}
		}
	}

	// Report duplicates within same language
	for lang, scenarios := range languageScenarios {
		for normalized, paths := range scenarios {
			if len(paths) > 1 {
				result.DuplicateScenarios = append(result.DuplicateScenarios, fmt.Sprintf("%s: %s -> %v", lang, normalized, paths))
			}
		}
	}

	// Sort results for consistent output
	sort.Strings(result.Issues)
	sort.Strings(result.MissingExpectedDirs)
	sort.Strings(result.EmptyExpectedDirs)
	sort.Strings(result.UnparsableFiles)
	sort.Strings(result.DuplicateScenarios)
	sort.Strings(result.InconsistentNaming)

	return result, nil
}

// checkNamingConsistency validates scenario naming conventions
func (r *Runner) checkNamingConsistency(scenarioName string, result *AuditResult) error {
	// Check for dots in directory names (except 04b_modern_decorators style)
	if strings.Contains(scenarioName, ".") && !strings.HasPrefix(scenarioName, "04b_") {
		return fmt.Errorf("directory name contains dots")
	}

	// Check for file extensions in directory names
	extensions := []string{".go", ".py", ".ts", ".java", ".js", ".rs", ".swift", ".kt", ".php", ".rb", ".cs", ".cpp"}
	for _, ext := range extensions {
		if strings.HasSuffix(scenarioName, ext) {
			return fmt.Errorf("directory name has file extension %s", ext)
		}
	}

	return nil
}

// normalizeScenarioName converts scenario name to normalized form for duplicate detection
func normalizeScenarioName(name string) string {
	// Remove file extensions
	extensions := []string{".go", ".py", ".ts", ".java", ".js", ".rs", ".swift", ".kt", ".php", ".rb", ".cs", ".cpp"}
	for _, ext := range extensions {
		name = strings.TrimSuffix(name, ext)
	}

	// Convert to lowercase for case-insensitive comparison
	return strings.ToLower(name)
}

// PrintAuditResult prints a human-readable audit report
func (r *AuditResult) PrintAuditResult() {
	fmt.Println("=== AI Distiller Test Structure Audit ===")

	totalIssues := len(r.Issues) + len(r.MissingExpectedDirs) + len(r.EmptyExpectedDirs) +
		len(r.UnparsableFiles) + len(r.DuplicateScenarios) + len(r.InconsistentNaming)

	if totalIssues == 0 {
		fmt.Println("✅ No issues found! Test structure is consistent.")
		return
	}

	fmt.Printf("Found %d issues:\n\n", totalIssues)

	if len(r.InconsistentNaming) > 0 {
		fmt.Printf("❌ Inconsistent Naming (%d):\n", len(r.InconsistentNaming))
		for _, issue := range r.InconsistentNaming {
			fmt.Printf("  - %s\n", issue)
		}
		fmt.Println()
	}

	if len(r.DuplicateScenarios) > 0 {
		fmt.Printf("❌ Duplicate Scenarios (%d):\n", len(r.DuplicateScenarios))
		for _, dup := range r.DuplicateScenarios {
			fmt.Printf("  - %s\n", dup)
		}
		fmt.Println()
	}

	if len(r.MissingExpectedDirs) > 0 {
		fmt.Printf("❌ Missing Expected Directories (%d):\n", len(r.MissingExpectedDirs))
		for _, missing := range r.MissingExpectedDirs {
			fmt.Printf("  - %s\n", missing)
		}
		fmt.Println()
	}

	if len(r.EmptyExpectedDirs) > 0 {
		fmt.Printf("❌ Empty Expected Directories (%d):\n", len(r.EmptyExpectedDirs))
		for _, empty := range r.EmptyExpectedDirs {
			fmt.Printf("  - %s\n", empty)
		}
		fmt.Println()
	}

	if len(r.UnparsableFiles) > 0 {
		fmt.Printf("❌ Unparsable Expected Files (%d):\n", len(r.UnparsableFiles))
		for _, unparsable := range r.UnparsableFiles {
			fmt.Printf("  - %s\n", unparsable)
		}
		fmt.Println()
	}

	if len(r.Issues) > 0 {
		fmt.Printf("❌ Other Issues (%d):\n", len(r.Issues))
		for _, issue := range r.Issues {
			fmt.Printf("  - %s\n", issue)
		}
		fmt.Println()
	}
}
