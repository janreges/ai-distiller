package rust

import (
	"context"
	"fmt"
	"io"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/stripper"
)

// Processor handles Rust source code processing
type Processor struct {
	processor.BaseProcessor
}

// NewProcessor creates a new Rust processor
func NewProcessor() *Processor {
	return &Processor{
		BaseProcessor: processor.NewBaseProcessor(
			"rust",
			"1.0.0",
			[]string{".rs"},
		),
	}
}

// Process implements processor.LanguageProcessor
func (p *Processor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	// Read source
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	// A new tree-sitter processor per call keeps calls independent
	return NewTreeSitterProcessor().ProcessSource(ctx, source, filename)
}

// ProcessWithOptions implements processor.LanguageProcessor
func (p *Processor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	// Read source
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	tsp := NewTreeSitterProcessor()
	file, err := tsp.ProcessSource(ctx, source, filename)
	if err != nil {
		return nil, err
	}

	// Apply stripper options
	stripperOpts := opts.ToStripperOptions()

	// Only strip if there's something to strip
	if stripperOpts.HasAnyOption() {
		s := stripper.New(stripperOpts)
		// The impl blocks of a type the stripper removes go with it.
		hideImplsOfRemovedTypes(file, tsp.impls, tsp.uses, s)
		stripped := file.Accept(s)
		if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
			return strippedFile, nil
		}
	}

	return file, nil
}
