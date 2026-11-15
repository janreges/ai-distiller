package language

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCanonical(t *testing.T) {
	for alias, want := range map[string]string{"py": "python", "JS": "javascript", "ts": "typescript", "golang": "go",
		"c++": "cpp", "C#": "csharp", "cs": "csharp", "rb": "ruby", "rs": "rust", "kt": "kotlin", "python": "python", "zig": "zig"} {
		assert.Equal(t, want, Canonical(alias), alias)
	}
}

func TestCanonical_EveryAliasHasAProcessor(t *testing.T) {
	for alias := range aliases {
		_, ok := GetProcessor(Canonical(alias))
		assert.True(t, ok, "alias %q maps to %q, which has no processor", alias, Canonical(alias))
	}
}
