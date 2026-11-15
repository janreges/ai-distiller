package formatter

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// MarkdownFormatter formats IR as Markdown by wrapping text formatter output
type MarkdownFormatter struct {
	BaseFormatter
	textFormatter Formatter // Use interface instead of concrete type
}

// NewMarkdownFormatter creates a new Markdown formatter
func NewMarkdownFormatter(options Options) *MarkdownFormatter {
	return &MarkdownFormatter{
		BaseFormatter: NewBaseFormatter(options),
		textFormatter: NewLanguageAwareTextFormatter(options), // Use the same formatter as "text" format
	}
}

// Extension returns the file extension for Markdown
func (f *MarkdownFormatter) Extension() string {
	return ".md"
}

// Format writes a single file as Markdown
func (f *MarkdownFormatter) Format(w io.Writer, file *ir.DistilledFile) error {
	// First, format as text
	var textBuf bytes.Buffer
	if err := f.textFormatter.Format(&textBuf, file); err != nil {
		return err
	}
	
	// Convert text format to markdown
	text := textBuf.String()
	
	// Debug: Show what text formatter returned
	// fmt.Fprintf(os.Stderr, "DEBUG: Text formatter output:\n%s\n", text)
	
	// Extract file content from <file> tags using simple string operations
	// This preserves all formatting exactly as output by the text formatter
	startTag := fmt.Sprintf(`<file path="%s">`, escapeFilePath(file.Path))
	endTag := "</file>"
	
	startIdx := strings.Index(text, startTag)
	endIdx := strings.Index(text, endTag)
	
	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		// Extract content between tags, including the newline after opening tag
		contentStart := startIdx + len(startTag)
		if contentStart < len(text) && text[contentStart] == '\n' {
			contentStart++ // Skip the newline after opening tag
		}
		content := text[contentStart:endIdx]
		
		// Debug: print what we extracted
		// Temporarily disable debug to see actual output
		// fmt.Fprintf(os.Stderr, "NEW DEBUG FORMAT\n")
		
		// Write as markdown
		fmt.Fprintf(w, "### %s\n\n", file.Path)
		
		// Determine language for syntax highlighting
		lang := f.getLanguageIdentifier(file.Language)
		
		fmt.Fprintf(w, "```%s\n", lang)
		fmt.Fprint(w, content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "```")
	} else {
		// Fallback - just write the text as-is
		// fmt.Fprintf(os.Stderr, "DEBUG: Fallback path taken. StartIdx: %d, EndIdx: %d\n", startIdx, endIdx)
		// fmt.Fprintf(os.Stderr, "DEBUG: Text length: %d, First 100 chars: %q\n", len(text), text[:100])
		fmt.Fprint(w, text)
	}
	
	return nil
}

// FormatMultiple writes multiple files as Markdown
func (f *MarkdownFormatter) FormatMultiple(w io.Writer, files []*ir.DistilledFile) error {
	// First, get all files formatted as text
	var textBuf bytes.Buffer
	if err := f.textFormatter.FormatMultiple(&textBuf, files); err != nil {
		return err
	}
	
	text := textBuf.String()
	
	// Find all file blocks in the text output
	processed := 0
	for {
		// Find next <file path="..."> tag
		startIdx := strings.Index(text[processed:], "<file path=\"")
		if startIdx == -1 {
			break
		}
		startIdx += processed
		
		// Extract the path
		pathStart := startIdx + 12 // len(`<file path="`)
		pathEnd := strings.Index(text[pathStart:], `">`)
		if pathEnd == -1 {
			break
		}
		path := unescapeFilePath(text[pathStart : pathStart+pathEnd])
		
		// Find the closing tag
		tagEnd := pathStart + pathEnd + 2
		endIdx := strings.Index(text[tagEnd:], "</file>")
		if endIdx == -1 {
			break
		}
		endIdx += tagEnd
		
		// Extract content (skip newline after opening tag)
		contentStart := tagEnd
		if contentStart < len(text) && text[contentStart] == '\n' {
			contentStart++
		}
		content := text[contentStart:endIdx]
		
		// Write markdown
		if processed > 0 {
			fmt.Fprintln(w)
			fmt.Fprintln(w)
		}
		
		fmt.Fprintf(w, "### %s\n\n", path)
		
		// Determine language from file extension
		lang := f.getLanguageFromPath(path)
		
		fmt.Fprintf(w, "```%s\n", lang)
		fmt.Fprint(w, content)
		if !strings.HasSuffix(content, "\n") {
			fmt.Fprintln(w)
		}
		fmt.Fprintln(w, "```")
		
		processed = endIdx + 7 // len("</file>")
	}
	
	return nil
}

// getLanguageFromPath returns the language identifier based on file extension
func (f *MarkdownFormatter) getLanguageFromPath(path string) string {
	ext := ""
	if idx := strings.LastIndex(path, "."); idx != -1 {
		ext = path[idx+1:]
	}
	
	switch ext {
	case "py":
		return "python"
	case "go":
		return "go"
	case "ts", "tsx":
		return "typescript"
	case "js", "jsx":
		return "javascript"
	case "java":
		return "java"
	case "cs":
		return "csharp"
	case "cpp", "cc", "cxx":
		return "cpp"
	case "rb":
		return "ruby"
	case "rs":
		return "rust"
	case "swift":
		return "swift"
	case "kt", "kts":
		return "kotlin"
	case "php":
		return "php"
	default:
		return ext
	}
}

// getLanguageIdentifier returns the correct language identifier for syntax highlighting
func (f *MarkdownFormatter) getLanguageIdentifier(language string) string {
	// Map our language names to common markdown identifiers
	switch strings.ToLower(language) {
	case "python":
		return "python"
	case "go", "golang":
		return "go"
	case "typescript":
		return "typescript"
	case "javascript":
		return "javascript"
	case "java":
		return "java"
	case "csharp", "c#":
		return "csharp"
	case "cpp", "c++":
		return "cpp"
	case "ruby":
		return "ruby"
	case "rust":
		return "rust"
	case "swift":
		return "swift"
	case "kotlin":
		return "kotlin"
	case "php":
		return "php"
	default:
		return language
	}
}