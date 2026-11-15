package ai

import (
	"fmt"
	"github.com/janreges/ai-distiller/internal/project"
	"path/filepath"
	"strings"
)

// ExpandTemplate expands template variables in a path template
func ExpandTemplate(template string, ctx *ActionContext) string {
	result := template

	// Replace ./.aid/ with project root .aid/
	if strings.HasPrefix(result, "./.aid/") || strings.HasPrefix(result, ".aid/") {
		aidDir, err := project.GetAidDir()
		if err == nil {
			if strings.HasPrefix(result, "./.aid/") {
				result = filepath.Join(aidDir, strings.TrimPrefix(result, "./.aid/"))
			} else {
				result = filepath.Join(aidDir, strings.TrimPrefix(result, ".aid/"))
			}
		}
	}

	// Time-based replacements
	now := ctx.Timestamp
	result = strings.ReplaceAll(result, "%YYYY%", fmt.Sprintf("%04d", now.Year()))
	result = strings.ReplaceAll(result, "%MM%", fmt.Sprintf("%02d", now.Month()))
	result = strings.ReplaceAll(result, "%DD%", fmt.Sprintf("%02d", now.Day()))
	result = strings.ReplaceAll(result, "%HH%", fmt.Sprintf("%02d", now.Hour()))
	result = strings.ReplaceAll(result, "%mm%", fmt.Sprintf("%02d", now.Minute()))
	result = strings.ReplaceAll(result, "%SS%", fmt.Sprintf("%02d", now.Second()))

	// Combined date/time formats
	result = strings.ReplaceAll(result, "%YYYY-MM-DD%", now.Format("2006-01-02"))
	result = strings.ReplaceAll(result, "%HH-MM-SS%", now.Format("15-04-05"))
	result = strings.ReplaceAll(result, "%YYYY-MM-DD.HH-MM-SS%", now.Format("2006-01-02.15-04-05"))

	// Project-based replacements
	baseName := ctx.BaseName
	if baseName == "" {
		baseName = "project"
	}
	result = strings.ReplaceAll(result, "%folder-basename%", baseName)
	result = strings.ReplaceAll(result, "%project%", baseName)

	// Clean up any double slashes
	result = filepath.Clean(result)

	return result
}

// ValidateOutputPath validates that an output path is safe
func ValidateOutputPath(path string, projectPath string) error {
	// Get absolute paths for comparison
	absProject, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute project path: %w", err)
	}

	absPath := path
	if !filepath.IsAbs(path) {
		// If relative, make it relative to project path
		absPath = filepath.Join(projectPath, path)
	}
	absPath, err = filepath.Abs(absPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute output path: %w", err)
	}

	// Ensure paths have consistent separators
	absProject = filepath.Clean(absProject)
	absPath = filepath.Clean(absPath)

	// Check if the output path is within the project directory
	// Add separator to ensure we're checking directory boundaries
	if !strings.HasPrefix(absPath, absProject+string(filepath.Separator)) && absPath != absProject {
		// Special case: if path is in .aid directory at project root, it's allowed
		projectRoot, _ := project.FindRoot()
		if projectRoot != nil && strings.HasPrefix(absPath, filepath.Join(projectRoot.Path, project.AidDirName)) {
			return nil
		}
		return fmt.Errorf("output path must be within project directory")
	}

	// Check for path traversal attempts
	if strings.Contains(path, "..") {
		cleaned := filepath.Clean(path)
		if strings.Contains(cleaned, "..") {
			return fmt.Errorf("output path cannot contain '..' for security reasons")
		}
	}

	return nil
}
