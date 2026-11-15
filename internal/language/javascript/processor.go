package javascript

import (
	"context"
	"fmt"
	"io"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/stripper"
)

// Processor handles JavaScript source code processing
type Processor struct {
	processor.BaseProcessor
}

// NewProcessor creates a new JavaScript processor
func NewProcessor() *Processor {
	return &Processor{
		BaseProcessor: processor.NewBaseProcessor(
			"javascript",
			"1.0.0",
			[]string{".js", ".mjs", ".cjs", ".jsx"},
		),
	}
}

// Process implements processor.LanguageProcessor
func (p *Processor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	return p.ProcessWithOptions(ctx, reader, filename, processor.DefaultProcessOptions())
}

// ProcessWithOptions implements processor.LanguageProcessor. A parse failure is
// returned to the caller; there is no fallback parser.
func (p *Processor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	tsProcessor, err := NewTreeSitterProcessor()
	if err != nil {
		return nil, fmt.Errorf("failed to create tree-sitter processor: %w", err)
	}
	defer tsProcessor.Close()

	result, err := tsProcessor.ProcessSource(ctx, source, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	// Apply stripper only if there is something to strip
	stripperOpts := opts.ToStripperOptions()
	if stripperOpts.HasAnyOption() {
		s := stripper.New(stripperOpts)
		stripped := result.Accept(s)
		if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
			return strippedFile, nil
		}
	}

	return result, nil
}
