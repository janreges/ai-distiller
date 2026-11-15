package ir

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommentIsDoc(t *testing.T) {
	for format, want := range map[string]bool{CommentDoc: true, "docblock": true, CommentLine: false, CommentBlock: false, CommentExport: false, "": false} {
		assert.Equal(t, want, (&DistilledComment{Format: format}).IsDoc(), format)
	}
}

// Parsers must use the constants, so the set of formats cannot drift again.
func TestParsersUseCommentFormatConstants(t *testing.T) {
	re := regexp.MustCompile(`Format:\s*"`)
	err := filepath.WalkDir("../language", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		for i, line := range strings.Split(string(src), "\n") {
			assert.False(t, re.MatchString(line), "%s:%d uses a string literal for a comment format", path, i+1)
		}
		return nil
	})
	require.NoError(t, err)
}
