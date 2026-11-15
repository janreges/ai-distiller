package aiactions

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
	"time"

	"github.com/janreges/ai-distiller/internal/version"
)

//go:embed templates/*.md
var embeddedTemplates embed.FS

// TemplateData contains data for template rendering
type TemplateData struct {
	ProjectName  string
	AnalysisDate string
}

// LoadTemplate loads and renders a template from embedded files first, then filesystem fallback
func LoadTemplate(templateName string, data TemplateData) (string, error) {
	var content []byte
	var err error

	// Strategy 1: Try embedded templates first
	embeddedPath := fmt.Sprintf("templates/%s.md", templateName)
	content, err = embeddedTemplates.ReadFile(embeddedPath)
	if err != nil {
		// Strategy 2: Fall back to filesystem
		templatePath := findTemplatePath(templateName)
		if templatePath == "" {
			cwd, _ := os.Getwd()
			return "", fmt.Errorf("template file not found: %s (searched embedded and filesystem from %s)", templateName+".md", cwd)
		}

		content, err = os.ReadFile(templatePath)
		if err != nil {
			return "", fmt.Errorf("failed to read template file %s: %w", templatePath, err)
		}
	}

	// Create FuncMap with template functions
	funcMap := template.FuncMap{
		"VERSION":     func() string { return version.Version },
		"WEBSITE_URL": func() string { return version.WebsiteURL },
	}

	// Parse and execute template
	tmpl, err := template.New(templateName).Funcs(funcMap).Parse(string(content))
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	var buf strings.Builder
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// findTemplatePath searches for the template file in multiple locations
func findTemplatePath(templateName string) string {
	// Strategy 1: Use runtime.Caller to find the source file location
	if _, filename, _, ok := runtime.Caller(1); ok {
		// Go up from template_loader.go to aiactions dir, then to templates
		aiactionsDir := filepath.Dir(filename)
		templatePath := filepath.Join(aiactionsDir, "templates", templateName+".md")
		if _, err := os.Stat(templatePath); err == nil {
			return templatePath
		}
	}

	// Strategy 2: Get executable path to find templates directory
	if execPath, err := os.Executable(); err == nil {
		// Try relative to executable first (for production)
		templatesDir := filepath.Join(filepath.Dir(execPath), "internal", "aiactions", "templates")
		templatePath := filepath.Join(templatesDir, templateName+".md")
		if _, err := os.Stat(templatePath); err == nil {
			return templatePath
		}

		// Try going up from executable to find project root
		execDir := filepath.Dir(execPath)
		for i := 0; i < 5; i++ {
			templatesDir := filepath.Join(execDir, "internal", "aiactions", "templates")
			templatePath := filepath.Join(templatesDir, templateName+".md")
			if _, err := os.Stat(templatePath); err == nil {
				return templatePath
			}
			parent := filepath.Dir(execDir)
			if parent == execDir {
				break
			}
			execDir = parent
		}
	}

	// Strategy 3: Try relative to current directory (for development)
	cwd, _ := os.Getwd()
	currentDir := cwd
	for i := 0; i < 10; i++ {
		templatesDir := filepath.Join(currentDir, "internal", "aiactions", "templates")
		templatePath := filepath.Join(templatesDir, templateName+".md")
		if _, err := os.Stat(templatePath); err == nil {
			return templatePath
		}
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			break
		}
		currentDir = parent
	}

	return ""
}

// CreateTemplateData creates template data with common values
func CreateTemplateData(projectName string) TemplateData {
	return TemplateData{
		ProjectName:  projectName,
		AnalysisDate: time.Now().Format("2006-01-02"),
	}
}
