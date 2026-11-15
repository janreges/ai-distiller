package python

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/stripper"
)

// Processor implements the LanguageProcessor interface for Python
type Processor struct {
	processor.BaseProcessor
}

// NewProcessor creates a new Python language processor
func NewProcessor() *Processor {
	return &Processor{
		BaseProcessor: processor.NewBaseProcessor(
			"python",
			"1.0.0",
			[]string{".py", ".pyw", ".pyi"},
		),
	}
}

// Process parses Python source code and returns the IR representation
func (p *Processor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	// Default options
	opts := processor.DefaultProcessOptions()
	return p.ProcessWithOptions(ctx, reader, filename, opts)
}

// ProcessFile processes a file by path
func (p *Processor) ProcessFile(filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	source, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	return p.processSource(context.Background(), source, filename, opts)
}

// ProcessWithOptions parses with specific options
func (p *Processor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("python")
	defer dbg.Timing(debug.LevelDetailed, "ProcessWithOptions")()

	// Read source code
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	dbg.Logf(debug.LevelDetailed, "Processing %s (%d bytes)", filename, len(source))

	return p.processSource(ctx, source, filename, opts)
}

// processSource parses the source with tree-sitter and applies the stripper.
// A parse failure is returned to the caller; there is no fallback parser.
func (p *Processor) processSource(ctx context.Context, source []byte, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	tsProcessor, err := NewNativeTreeSitterProcessor()
	if err != nil {
		return nil, fmt.Errorf("failed to create tree-sitter processor: %w", err)
	}
	defer tsProcessor.parser.Close()

	file, err := tsProcessor.ProcessSource(ctx, source, filename)
	if err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", filename, err)
	}

	// Apply stripper only if there is something to strip
	stripperOpts := opts.ToStripperOptions()
	if stripperOpts.HasAnyOption() {
		s := stripper.New(stripperOpts)
		stripped := file.Accept(s)
		if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
			return strippedFile, nil
		}
	}

	return file, nil
}
