package processor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/janreges/ai-distiller/internal/language"
)

func TestProcessFileRecordsSourceSize(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "python", "01_basic", "source.py")
	info, err := os.Stat(path)
	require.NoError(t, err)
	// registry_test.go (package processor) replaces the default registry, so
	// make sure the real language processors are registered whatever the order.
	if _, ok := processor.GetByFilename(path); !ok {
		require.NoError(t, language.RegisterAll())
	}
	file, err := processor.New().ProcessFile(path, processor.ProcessOptions{IncludeImports: true})
	require.NoError(t, err)
	require.NotNil(t, file.Metadata)
	assert.Equal(t, info.Size(), file.Metadata.Size)
}
