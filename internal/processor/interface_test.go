package processor

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/stretchr/testify/assert"
)

func TestDefaultProcessOptions(t *testing.T) {
	opts := DefaultProcessOptions()

	assert.True(t, opts.IncludeImplementation)
	assert.True(t, opts.IncludeComments)
	assert.True(t, opts.IncludeImports)
	assert.True(t, opts.IncludePrivate)
	assert.True(t, opts.IncludeLineNumbers)
}

func TestDefaultProcessOptionsDoesNotEnableDependencyAnalysis(t *testing.T) {
	opts := DefaultProcessOptions()
	assert.False(t, opts.SymbolResolution)
	assert.Equal(t, 2, opts.MaxDepth)
}

func TestProcessorError(t *testing.T) {
	tests := []struct {
		name     string
		err      ProcessorError
		expected string
	}{
		{
			name: "WithLineInfo",
			err: ProcessorError{
				File:     "test.go",
				Line:     10,
				Column:   5,
				Message:  "syntax error",
				Severity: "error",
				Code:     "E001",
			},
			expected: "test.go:10:5: syntax error",
		},
		{
			name: "WithoutLineInfo",
			err: ProcessorError{
				File:     "test.go",
				Message:  "file not found",
				Severity: "error",
			},
			expected: "test.go: file not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestMultiError(t *testing.T) {
	tests := []struct {
		name     string
		errors   []error
		expected string
	}{
		{
			name:     "NoErrors",
			errors:   []error{},
			expected: "no errors",
		},
		{
			name:     "SingleError",
			errors:   []error{errors.New("test error")},
			expected: "test error",
		},
		{
			name:     "MultipleErrors",
			errors:   []error{errors.New("error1"), errors.New("error2")},
			expected: "2 errors occurred",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := MultiError{Errors: tt.errors}
			assert.Equal(t, tt.expected, err.Error())
		})
	}
}

func TestBaseProcessor(t *testing.T) {
	proc := NewBaseProcessor("go", "1.0.0", []string{".go"})

	assert.Equal(t, "go", proc.Language())
	assert.Equal(t, "1.0.0", proc.Version())
	assert.Equal(t, []string{".go"}, proc.SupportedExtensions())

	// Test CanProcess
	assert.True(t, proc.CanProcess("main.go"))
	assert.True(t, proc.CanProcess("test.go"))
	assert.False(t, proc.CanProcess("main.py"))
	assert.False(t, proc.CanProcess("test.js"))
}

// Mock processor for testing
type mockProcessor struct {
	BaseProcessor
}

func (m *mockProcessor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	return &ir.DistilledFile{
		Path:     filename,
		Language: m.Language(),
		Version:  m.Version(),
	}, nil
}

func (m *mockProcessor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts ProcessOptions) (*ir.DistilledFile, error) {
	file, err := m.Process(ctx, reader, filename)
	if err != nil {
		return nil, err
	}

	// Simulate options affecting the result
	if !opts.IncludeComments {
		file.Children = filterComments(file.Children)
	}

	return file, nil
}

func filterComments(nodes []ir.DistilledNode) []ir.DistilledNode {
	filtered := make([]ir.DistilledNode, 0, len(nodes))
	for _, node := range nodes {
		if node.GetNodeKind() != ir.KindComment {
			filtered = append(filtered, node)
		}
	}
	return filtered
}

func TestMockProcessor(t *testing.T) {
	ctx := context.Background()
	proc := &mockProcessor{
		BaseProcessor: NewBaseProcessor("test", "1.0.0", []string{".test"}),
	}

	// Test Process
	reader := strings.NewReader("test content")
	file, err := proc.Process(ctx, reader, "test.test")

	assert.NoError(t, err)
	assert.NotNil(t, file)
	assert.Equal(t, "test.test", file.Path)
	assert.Equal(t, "test", file.Language)
	assert.Equal(t, "1.0.0", file.Version)

	// Test ProcessWithOptions
	opts := ProcessOptions{
		IncludeComments: false,
	}
	file2, err := proc.ProcessWithOptions(ctx, reader, "test2.test", opts)

	assert.NoError(t, err)
	assert.NotNil(t, file2)
}
