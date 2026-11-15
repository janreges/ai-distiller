package importfilter

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

// BaseFilter provides common functionality for all language filters
type BaseFilter struct {
	language string
}

// NewBaseFilter creates a new base filter
func NewBaseFilter(language string) BaseFilter {
	return BaseFilter{language: language}
}

// Language returns the language this filter handles
func (f *BaseFilter) Language() string {
	return f.language
}

// SearchForUsage checks if a given name is used in the code (excluding import section)
func (f *BaseFilter) SearchForUsage(code string, name string, importEndLine int) bool {
	lines := strings.Split(code, "\n")

	// Skip import section
	searchLines := lines
	if importEndLine > 0 && importEndLine < len(lines) {
		searchLines = lines[importEndLine:]
	}

	// Create pattern with word boundaries
	pattern := fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(name))
	re, err := regexp.Compile(pattern)
	if err != nil {
		// If regex fails, fall back to simple contains
		searchText := strings.Join(searchLines, "\n")
		return strings.Contains(searchText, name)
	}

	// Search in non-import lines
	for _, line := range searchLines {
		// Skip comments (simple heuristic)
		if strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") &&
			!strings.HasPrefix(strings.TrimSpace(line), "//") {
			if re.MatchString(line) {
				return true
			}
		}
	}

	return false
}

// RemoveLines removes lines from startLine to endLine (1-based)
func (f *BaseFilter) RemoveLines(code string, startLine, endLine int) string {
	lines := strings.Split(code, "\n")

	// Convert to 0-based indexing
	startIdx := startLine - 1
	endIdx := endLine - 1

	// Validate indices
	if startIdx < 0 || endIdx >= len(lines) || startIdx > endIdx {
		return code
	}

	// Create new slice without the specified lines
	result := make([]string, 0, len(lines)-(endIdx-startIdx+1))
	result = append(result, lines[:startIdx]...)
	result = append(result, lines[endIdx+1:]...)

	return strings.Join(result, "\n")
}

// LogDebug logs debug information if debug level is high enough
func (f *BaseFilter) LogDebug(debugLevel int, requiredLevel int, format string, args ...interface{}) {
	if debugLevel >= requiredLevel {
		msg := fmt.Sprintf(format, args...)
		fmt.Fprintf(os.Stderr, "[%s import filter] %s\n", f.language, msg)
	}
}

// IsCommentLine checks if a line is likely a comment
func (f *BaseFilter) IsCommentLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}

	// Common comment patterns across languages
	commentPrefixes := []string{"#", "//", "/*", "*", "*/"}
	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}

	return false
}

// ExtractStringContent extracts content between quotes
func (f *BaseFilter) ExtractStringContent(line string) []string {
	var contents []string

	// Match single and double quoted strings
	patterns := []string{`"([^"]*)"`, `'([^']*)'`}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			if len(match) > 1 {
				contents = append(contents, match[1])
			}
		}
	}

	return contents
}
