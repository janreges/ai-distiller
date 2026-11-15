package processor

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRawProcessor_BinaryFiles(t *testing.T) {
	proc := NewRawProcessor()
	ctx := context.Background()

	binaryFiles := []string{
		"test.jpg",
		"test.png",
		"test.pdf",
		"test.exe",
		"test.zip",
		"test.mp4",
		"test.pyc",
	}

	for _, filename := range binaryFiles {
		t.Run(filename, func(t *testing.T) {
			reader := strings.NewReader("fake binary content")
			result, err := proc.Process(ctx, reader, filename)
			assert.NoError(t, err, "Should not error on binary file: %s", filename)
			assert.Nil(t, result, "Should return nil for binary file: %s", filename)
		})
	}
}

func TestRawProcessor_TextFiles(t *testing.T) {
	proc := NewRawProcessor()
	ctx := context.Background()

	textFiles := []string{
		"README.md",
		"config.json",
		"script.sh",
		"data.csv",
		"index.html",
		"Makefile",
		".gitignore",
	}

	for _, filename := range textFiles {
		t.Run(filename, func(t *testing.T) {
			content := "Hello\nWorld\nTest"
			reader := strings.NewReader(content)
			result, err := proc.Process(ctx, reader, filename)
			assert.NoError(t, err, "Should accept text file: %s", filename)
			assert.NotNil(t, result)
			assert.Equal(t, filename, result.Path)
			assert.Equal(t, "text", result.Language)
		})
	}
}
