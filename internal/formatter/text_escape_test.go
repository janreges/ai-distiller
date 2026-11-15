package formatter

import (
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTextFormatter_EscapesFilePath(t *testing.T) {
	var b strings.Builder
	file := &ir.DistilledFile{Path: `dir/a"b<c>&d.py`, Language: "python"}
	require.NoError(t, NewLanguageAwareTextFormatter(Options{}).Format(&b, file))
	assert.Contains(t, b.String(), `<file path="dir/a&quot;b&lt;c&gt;&amp;d.py">`)
}

func TestEscapeFilePathRoundTrip(t *testing.T) {
	for _, p := range []string{"plain/path.go", `a"b<c>&d.py`, "ünï/cödé.rs", "a&amp;b.py"} {
		assert.Equal(t, p, unescapeFilePath(escapeFilePath(p)))
	}
	assert.Equal(t, "plain/path.go", escapeFilePath("plain/path.go"))
}
