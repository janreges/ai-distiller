//go:build ignore
// +build ignore

// This is a migration helper script to move existing tests to the new structure
// Run with: go run internal/testrunner/migrate.go

package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type Migration struct {
	Language   string
	OldPattern string
	NewPattern string
	Transform  func(oldPath string) (scenario, sourceFile string)
}

var migrations = []Migration{
	{
		Language:   "python",
		OldPattern: "test-data/python/construct_*",
		Transform: func(oldPath string) (string, string) {
			base := filepath.Base(oldPath)
			// construct_1_basic -> 01_basic
			parts := strings.Split(base, "_")
			if len(parts) >= 3 {
				num := parts[1]
				name := strings.Join(parts[2:], "_")
				return fmt.Sprintf("%02s_%s", num, name), "source.py"
			}
			return base, "source.py"
		},
	},
	{
		Language:   "typescript",
		OldPattern: "test-data/typescript/construct_*",
		Transform: func(oldPath string) (string, string) {
			base := filepath.Base(oldPath)
			parts := strings.Split(base, "_")
			if len(parts) >= 3 {
				num := parts[1]
				name := strings.Join(parts[2:], "_")
				return fmt.Sprintf("%02s_%s", num, name), "source.ts"
			}
			return base, "source.ts"
		},
	},
	{
		Language:   "go",
		OldPattern: "test-data/go/construct_*",
		Transform: func(oldPath string) (string, string) {
			base := filepath.Base(oldPath)
			parts := strings.Split(base, "_")
			if len(parts) >= 3 {
				num := parts[1]
				name := strings.Join(parts[2:], "_")
				return fmt.Sprintf("%02s_%s", num, name), "source.go"
			}
			return base, "source.go"
		},
	},
	{
		Language:   "java",
		OldPattern: "test-data/java/*.java",
		Transform: func(oldPath string) (string, string) {
			base := filepath.Base(oldPath)
			name := strings.TrimSuffix(base, ".java")
			// Map feature names to numbered scenarios
			scenarioMap := map[string]string{
				"Basic":                 "01_basic",
				"SimpleOOP":             "02_simple_oop",
				"GenericsAndInterfaces": "03_generics_interfaces",
				"Advanced":              "04_advanced",
				"ModernJava":            "05_modern_java",
			}

			if scenario, ok := scenarioMap[name]; ok {
				return scenario, "source.java"
			}
			return strings.ToLower(name), "source.java"
		},
	},
}

func main() {
	if err := migrate(); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Migration completed successfully!")
}

func migrate() error {
	// Create testdata directory if it doesn't exist
	if err := os.MkdirAll("testdata", 0755); err != nil {
		return fmt.Errorf("creating testdata dir: %w", err)
	}

	for _, m := range migrations {
		fmt.Printf("Migrating %s tests...\n", m.Language)

		// Create language directory
		langDir := filepath.Join("testdata", m.Language)
		if err := os.MkdirAll(langDir, 0755); err != nil {
			return fmt.Errorf("creating language dir %s: %w", langDir, err)
		}

		// Find old test directories/files
		matches, err := filepath.Glob(m.OldPattern)
		if err != nil {
			return fmt.Errorf("globbing pattern %s: %w", m.OldPattern, err)
		}

		for _, oldPath := range matches {
			scenario, sourceFile := m.Transform(oldPath)
			scenarioDir := filepath.Join(langDir, scenario)

			fmt.Printf("  %s -> %s\n", oldPath, scenarioDir)

			// Create scenario directory
			if err := os.MkdirAll(scenarioDir, 0755); err != nil {
				return fmt.Errorf("creating scenario dir %s: %w", scenarioDir, err)
			}

			// Create expected directory
			expectedDir := filepath.Join(scenarioDir, "expected")
			if err := os.MkdirAll(expectedDir, 0755); err != nil {
				return fmt.Errorf("creating expected dir %s: %w", expectedDir, err)
			}

			// Migrate files based on the structure
			if info, err := os.Stat(oldPath); err == nil && info.IsDir() {
				// It's a directory (construct pattern)
				if err := migrateConstructDir(oldPath, scenarioDir, sourceFile); err != nil {
					return fmt.Errorf("migrating construct dir %s: %w", oldPath, err)
				}
			} else {
				// It's a file (Java pattern)
				if err := migrateFeatureFile(oldPath, scenarioDir, sourceFile); err != nil {
					return fmt.Errorf("migrating feature file %s: %w", oldPath, err)
				}
			}
		}
	}

	return nil
}

func migrateConstructDir(oldDir, newDir, sourceFile string) error {
	// Find source file in old directory
	entries, err := os.ReadDir(oldDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		oldPath := filepath.Join(oldDir, name)

		switch {
		case strings.HasSuffix(name, ".py"), strings.HasSuffix(name, ".ts"),
			strings.HasSuffix(name, ".go"), strings.HasSuffix(name, ".js"):
			// Source file
			newPath := filepath.Join(newDir, sourceFile)
			if err := copyFile(oldPath, newPath); err != nil {
				return fmt.Errorf("copying source file: %w", err)
			}

		case strings.HasPrefix(name, "expected_"):
			// Expected file
			newName := transformExpectedName(name)
			newPath := filepath.Join(newDir, "expected", newName)
			if err := copyFile(oldPath, newPath); err != nil {
				return fmt.Errorf("copying expected file: %w", err)
			}
		}
	}

	return nil
}

func migrateFeatureFile(oldFile, newDir, sourceFile string) error {
	// Copy source file
	newSourcePath := filepath.Join(newDir, sourceFile)
	if err := copyFile(oldFile, newSourcePath); err != nil {
		return fmt.Errorf("copying source file: %w", err)
	}

	// Look for expected files in the same directory
	dir := filepath.Dir(oldFile)
	base := filepath.Base(oldFile)
	nameWithoutExt := strings.TrimSuffix(base, filepath.Ext(base))

	pattern := filepath.Join(dir, nameWithoutExt+"_expected_*.txt")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}

	for _, match := range matches {
		// Extract the mode from filename
		// e.g., Basic_expected_full.txt -> full
		parts := strings.Split(filepath.Base(match), "_")
		if len(parts) >= 3 {
			mode := strings.TrimSuffix(parts[len(parts)-1], ".txt")
			newName := transformModeName(mode) + ".txt"
			newPath := filepath.Join(newDir, "expected", newName)

			if err := copyFile(match, newPath); err != nil {
				return fmt.Errorf("copying expected file %s: %w", match, err)
			}
		}
	}

	return nil
}

func transformExpectedName(oldName string) string {
	// expected_full.txt -> default.txt
	// expected_no_impl.txt -> no_impl.txt
	// expected_no_private.txt -> public.txt

	name := strings.TrimPrefix(oldName, "expected_")
	name = strings.TrimSuffix(name, ".txt")

	switch name {
	case "full":
		return "default.txt"
	case "no_private":
		return "public.txt"
	default:
		return name + ".txt"
	}
}

func transformModeName(mode string) string {
	switch mode {
	case "full":
		return "default"
	case "no_private":
		return "public"
	default:
		return mode
	}
}

func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
