package language

import (
	"sort"
	"strings"

	"github.com/janreges/ai-distiller/internal/processor"
)

// aliases maps alternative language names (as users type them for --lang)
// to the name a processor is registered under.
var aliases = map[string]string{
	"py":     "python",
	"js":     "javascript",
	"jsx":    "javascript",
	"ts":     "typescript",
	"tsx":    "typescript",
	"golang": "go",
	"c++":    "cpp",
	"cxx":    "cpp",
	"cc":     "cpp",
	"c#":     "csharp",
	"cs":     "csharp",
	"rb":     "ruby",
	"rs":     "rust",
	"kt":     "kotlin",
}

// Canonical maps a language alias to the name its processor is registered
// under. A name that is not an alias is returned lower-cased.
func Canonical(name string) string {
	lower := strings.ToLower(name)
	if canonical, ok := aliases[lower]; ok {
		return canonical
	}
	return lower
}

// Aliases returns the alternative names --lang accepts, sorted.
func Aliases() []string {
	names := make([]string, 0, len(aliases))
	for alias := range aliases {
		names = append(names, alias)
	}
	sort.Strings(names)
	return names
}

// RegisteredNames returns the names of all registered processors, sorted.
func RegisteredNames() []string {
	names := processor.List()
	sort.Strings(names)
	return names
}
