package conformance

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	_ "github.com/janreges/ai-distiller/internal/language"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/testrunner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoUnhandledNodesInTestdata formats the raw IR of every testdata source
// and fails when a text formatter drops a node kind that is not listed in
// knownUnhandled.
func TestNoUnhandledNodesInTestdata(t *testing.T) {
	root, err := testrunner.ProjectRoot()
	require.NoError(t, err)

	seen := map[string]bool{}
	formatter.OnUnhandledNode = func(lang string, node ir.DistilledNode) {
		seen[lang+"/"+string(node.GetNodeKind())] = true
	}
	defer func() { formatter.OnUnhandledNode = nil }()

	sources, err := filepath.Glob(filepath.Join(root, "testdata", "*", "*", "source.*"))
	require.NoError(t, err)
	require.NotEmpty(t, sources)
	text, err := formatter.Get("text", formatter.Options{})
	require.NoError(t, err)

	for _, src := range sources {
		proc, ok := processor.GetByFilename(src)
		require.True(t, ok, src)
		f, err := os.Open(src)
		require.NoError(t, err)
		file, err := proc.Process(context.Background(), f, src) // raw IR, nothing stripped
		f.Close()
		require.NoError(t, err, src)
		require.NoError(t, text.Format(io.Discard, file))
	}

	for key := range seen {
		if _, ok := knownUnhandled[key]; !ok {
			t.Errorf("text formatter drops %s nodes without rendering them", key)
		}
	}
	for key := range knownUnhandled {
		assert.True(t, seen[key], "%s is rendered now: delete it from knownUnhandled", key)
	}
}

// knownUnhandled lists "<lang>/<node kind>" pairs that a text formatter
// silently drops today, each with the reason.
var knownUnhandled = map[string]string{
	"cpp/comment":       "the C++ formatter has no DistilledComment case, so comments are dropped",
	"cpp/package":       "the C++ formatter has no DistilledPackage case, so the namespace line is dropped (its children are still rendered)",
	"csharp/type_alias": "the C# formatter has no DistilledTypeAlias case (using aliases are not rendered)",
	"kotlin/package":    "the Kotlin formatter has no DistilledPackage case (package declaration is not rendered)",
}
