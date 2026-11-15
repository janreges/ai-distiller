package processor

import (
	"context"
	"io"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test processor implementation
type testProcessor struct {
	BaseProcessor
}

func (t *testProcessor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	return &ir.DistilledFile{
		Path:     filename,
		Language: t.Language(),
	}, nil
}

func (t *testProcessor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts ProcessOptions) (*ir.DistilledFile, error) {
	return t.Process(ctx, reader, filename)
}

func TestNewRegistry(t *testing.T) {
	reg := NewRegistry()
	assert.NotNil(t, reg)

	// Should be empty initially
	assert.Empty(t, reg.List())
}

func TestRegistryRegister(t *testing.T) {
	reg := NewRegistry()

	// Create test processors
	goProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("go", "1.0.0", []string{".go"}),
	}
	pyProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("python", "1.0.0", []string{".py", ".pyw"}),
	}

	// Register processors
	err := reg.Register(goProc)
	assert.NoError(t, err)

	err = reg.Register(pyProc)
	assert.NoError(t, err)

	// Should have 2 languages
	langs := reg.List()
	assert.Len(t, langs, 2)
	assert.Contains(t, langs, "go")
	assert.Contains(t, langs, "python")

	// Test duplicate registration
	err = reg.Register(goProc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")

	// Test empty language
	emptyProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("", "1.0.0", []string{".empty"}),
	}
	err = reg.Register(emptyProc)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestRegistryGet(t *testing.T) {
	reg := NewRegistry()

	// Register a processor
	goProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("go", "1.0.0", []string{".go"}),
	}
	err := reg.Register(goProc)
	require.NoError(t, err)

	// Get existing processor
	proc, ok := reg.Get("go")
	assert.True(t, ok)
	assert.Equal(t, "go", proc.Language())

	// Get non-existing processor
	proc, ok = reg.Get("rust")
	assert.False(t, ok)
	assert.Nil(t, proc)
}

func TestRegistryGetByFilename(t *testing.T) {
	reg := NewRegistry()

	// Register processors
	goProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("go", "1.0.0", []string{".go"}),
	}
	pyProc := &testProcessor{
		BaseProcessor: NewBaseProcessor("python", "1.0.0", []string{".py", "py"}), // Test with and without dot
	}

	err := reg.Register(goProc)
	require.NoError(t, err)
	err = reg.Register(pyProc)
	require.NoError(t, err)

	// Test various filenames
	tests := []struct {
		filename string
		expected string
		found    bool
	}{
		{"main.go", "go", true},
		{"test.GO", "go", true}, // Case insensitive
		{"script.py", "python", true},
		{"script.PY", "python", true},
		{"app.js", "", false},
		{"README.md", "", false},
		{"noextension", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			proc, ok := reg.GetByFilename(tt.filename)
			assert.Equal(t, tt.found, ok)
			if tt.found {
				assert.Equal(t, tt.expected, proc.Language())
			} else {
				assert.Nil(t, proc)
			}
		})
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Clear default registry for testing
	defaultRegistry = NewRegistry()

	// Test Register
	proc := &testProcessor{
		BaseProcessor: NewBaseProcessor("test", "1.0.0", []string{".test"}),
	}
	err := Register(proc)
	assert.NoError(t, err)

	// Test Get
	p, ok := Get("test")
	assert.True(t, ok)
	assert.Equal(t, "test", p.Language())

	// Test GetByFilename
	p, ok = GetByFilename("file.test")
	assert.True(t, ok)
	assert.Equal(t, "test", p.Language())

	// Test List
	langs := List()
	assert.Contains(t, langs, "test")
}

func TestMustRegister(t *testing.T) {
	defaultRegistry = NewRegistry()

	// Should not panic with valid processor
	proc := &testProcessor{
		BaseProcessor: NewBaseProcessor("test2", "1.0.0", []string{".test2"}),
	}
	assert.NotPanics(t, func() {
		MustRegister(proc)
	})

	// Verify it was registered
	p, ok := Get("test2")
	assert.True(t, ok)
	assert.Equal(t, "test2", p.Language())

	// Should panic with duplicate
	assert.Panics(t, func() {
		MustRegister(proc)
	})
}
