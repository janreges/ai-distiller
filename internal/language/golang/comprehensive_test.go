package golang

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	formatterPkg "github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/processor"
)

func TestGoConstructs(t *testing.T) {
	tests := []struct {
		name         string
		construct    string
		options      processor.ProcessOptions
		expectedFile string
	}{
		// Basic construct tests
		{
			name:      "Basic_Default",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        false,
				IncludeImplementation: false,
				IncludeComments:       false,
				IncludeDocstrings:     true, // Fixed: default should include docstrings
				IncludeImports:        true,
				IncludeFields:         true, // Fixed: default should include fields
				IncludeMethods:        true, // Fixed: default should include methods
			},
			expectedFile: "default.txt",
		},
		{
			name:      "Basic_WithImplementation",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        false,
				IncludeImplementation: true,
				IncludeComments:       false, // Fixed: implementation=1.txt doesn't include comments
				IncludeDocstrings:     true,
				IncludeImports:        true,
				IncludeFields:         true, // Fixed: default should include fields
				IncludeMethods:        true, // Fixed: default should include methods
			},
			expectedFile: "implementation=1.txt",
		},
		{
			name:      "Basic_WithPrivate",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        true,
				IncludeImplementation: false,
				IncludeComments:       false, // Fixed: private=1,protected=1,internal=1,implementation=0.txt doesn't include comments
				IncludeDocstrings:     true,
				IncludeImports:        true,
				IncludeFields:         true, // Fixed: default should include fields
				IncludeMethods:        true, // Fixed: default should include methods
			},
			expectedFile: "private=1,protected=1,internal=1,implementation=0.txt",
		},
	}

	p := NewProcessor()
	textFormatter := formatterPkg.NewLanguageAwareTextFormatter(formatterPkg.Options{
		ProcessingOptions: formatterPkg.ProcessingInfo{
			FilterImports: true, // Enable import filtering like CLI does
		},
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read source file
			sourcePath := filepath.Join("../../../testdata/go", tt.construct, "source.go")
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				t.Fatalf("Failed to open source file: %v", err)
			}
			defer sourceFile.Close()

			// Process the file
			result, err := p.ProcessWithOptions(context.Background(), sourceFile, "source.go", tt.options)
			if err != nil {
				t.Fatalf("Processing failed: %v", err)
			}

			// Format the result
			var output strings.Builder
			if err := textFormatter.Format(&output, result); err != nil {
				t.Fatalf("Formatting failed: %v", err)
			}

			// Read expected output
			expectedPath := filepath.Join("../../../testdata/go", tt.construct, "expected", tt.expectedFile)
			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("Failed to read expected file: %v", err)
			}

			expected := strings.TrimSpace(string(expectedBytes))
			actual := strings.TrimSpace(output.String())

			if expected != actual {
				t.Errorf("Output mismatch for %s:\nExpected:\n%s\n\nActual:\n%s", tt.name, expected, actual)
			}
		})
	}
}
