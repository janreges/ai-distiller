package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/janreges/ai-distiller/internal/testrunner"
)

func main() {
	// Find project root
	projectRoot := findProjectRoot()
	if projectRoot == "" {
		fmt.Fprintf(os.Stderr, "Error: could not find project root\n")
		os.Exit(1)
	}

	testDataDir := filepath.Join(projectRoot, "testdata")

	// Create runner for audit
	runner := testrunner.New(testDataDir, "")

	// Run audit
	result, err := runner.Audit()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error running audit: %v\n", err)
		os.Exit(1)
	}

	// Print results
	result.PrintAuditResult()

	// Exit with error code if issues found
	totalIssues := len(result.Issues) + len(result.MissingExpectedDirs) + len(result.EmptyExpectedDirs) +
		len(result.UnparsableFiles) + len(result.DuplicateScenarios) + len(result.InconsistentNaming)

	if totalIssues > 0 {
		os.Exit(1)
	}
}

// findProjectRoot looks for go.mod to determine project root
func findProjectRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
