package processor

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// RawProcessor handles all text files without parsing
type RawProcessor struct {
	BaseProcessor
}

// NewRawProcessor creates a new raw processor
func NewRawProcessor() *RawProcessor {
	return &RawProcessor{
		BaseProcessor: NewBaseProcessor("raw", "1.0.0", []string{
			// Common text file extensions
			".txt", ".text", ".md", ".markdown", ".mdown",
			".json", ".jsonl", ".json5",
			".yaml", ".yml",
			".xml", ".html", ".htm",
			".csv", ".tsv",
			".ini", ".cfg", ".conf", ".config",
			".env", ".properties",
			".toml",
			".rst", ".asciidoc", ".adoc",
			".tex", ".latex",
			".log",
			".sh", ".bash", ".zsh", ".fish",
			".bat", ".cmd", ".ps1",
			".sql",
			".graphql", ".gql",
			".proto",
			".dockerfile", ".containerfile",
			".gitignore", ".dockerignore", ".npmignore",
			".editorconfig", ".prettierrc", ".eslintrc",
			// Also handle files without extension that are commonly text
			"Makefile", "makefile", "Dockerfile", "dockerfile",
			"README", "readme", "LICENSE", "license",
			"CHANGELOG", "changelog", "TODO", "todo",
		}),
	}
}

// CanProcessRaw checks if a file can be processed as raw text
func (p *RawProcessor) CanProcessRaw(filename string) bool {
	// Check by extension
	if p.CanProcess(filename) {
		return true
	}

	// Check by base filename (no extension)
	base := filepath.Base(filename)
	baseUpper := strings.ToUpper(base)

	// Common text files without extensions
	textFiles := []string{
		"README", "LICENSE", "CHANGELOG", "TODO",
		"MAKEFILE", "DOCKERFILE", "VAGRANTFILE",
		"GEMFILE", "RAKEFILE", "GULPFILE",
		".GITIGNORE", ".DOCKERIGNORE", ".NPMIGNORE",
		".EDITORCONFIG", ".PRETTIERRC", ".ESLINTRC",
		".BABELRC", ".GITATTRIBUTES", ".GITMODULES",
	}

	for _, tf := range textFiles {
		if baseUpper == tf || base == strings.ToLower(tf) {
			return true
		}
	}

	return false
}

// Process implements LanguageProcessor
func (p *RawProcessor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	// Check if this is likely a binary file by extension
	ext := strings.ToLower(filepath.Ext(filename))
	binaryExtensions := []string{
		".exe", ".dll", ".so", ".dylib", ".a", ".lib",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".ico", ".webp", ".svg",
		".mp3", ".mp4", ".avi", ".mkv", ".mov", ".flv", ".wmv",
		".zip", ".tar", ".gz", ".bz2", ".7z", ".rar",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".ttf", ".otf", ".woff", ".woff2", ".eot",
		".pyc", ".pyo", ".class", ".o", ".obj",
		".db", ".sqlite", ".mdb",
	}

	for _, binExt := range binaryExtensions {
		if ext == binExt {
			// Log warning and skip binary file
			fmt.Fprintf(os.Stderr, "Warning: skipping binary file: %s\n", filename)
			return nil, nil
		}
	}

	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   -1, // Will be set after reading
			},
		},
		Path:     filename,
		Language: "text",
		Children: []ir.DistilledNode{},
	}

	// Read entire content
	scanner := bufio.NewScanner(reader)
	var lines []string
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	file.BaseNode.Location.EndLine = lineNum

	// Create a single raw content node
	content := strings.Join(lines, "\n")
	// Debug: print content length
	// fmt.Fprintf(os.Stderr, "DEBUG: Raw content length: %d, lines: %d, content: %q\n", len(content), lineNum, content)
	rawNode := &ir.DistilledRawContent{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   lineNum,
			},
		},
		Content: content,
	}

	file.Children = append(file.Children, rawNode)

	return file, nil
}

// ProcessWithOptions implements LanguageProcessor
func (p *RawProcessor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts ProcessOptions) (*ir.DistilledFile, error) {
	// Raw mode doesn't apply any stripping options
	return p.Process(ctx, reader, filename)
}

// IsTextFile checks if a file might be a text file based on extension
func IsTextFile(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	// Check common text extensions
	textExtensions := []string{
		".txt", ".text", ".md", ".markdown",
		".json", ".yaml", ".yml", ".xml", ".html",
		".csv", ".ini", ".conf", ".config",
		".log", ".sh", ".bat", ".sql",
	}

	for _, textExt := range textExtensions {
		if ext == textExt {
			return true
		}
	}

	// Check filenames without extensions
	base := strings.ToLower(filepath.Base(filename))
	textFiles := []string{
		"readme", "license", "changelog", "todo",
		"makefile", "dockerfile", ".gitignore",
	}

	for _, tf := range textFiles {
		if base == tf {
			return true
		}
	}

	return false
}
