package formatter

import (
	"io"

	"github.com/janreges/ai-distiller/internal/ir"
)

// Formatter defines the interface for output formatters
type Formatter interface {
	// Format writes the IR to the writer in the specific format
	Format(w io.Writer, file *ir.DistilledFile) error

	// FormatMultiple writes multiple files to the writer
	FormatMultiple(w io.Writer, files []*ir.DistilledFile) error

	// Extension returns the recommended file extension for this format
	Extension() string
}

// Options configures formatter behavior
type Options struct {
	// IncludeLocation includes source location information
	IncludeLocation bool

	// IncludeMetadata includes file metadata
	IncludeMetadata bool

	// Compact produces compact output (no extra whitespace)
	Compact bool

	// AbsolutePaths uses absolute paths instead of relative
	AbsolutePaths bool

	// SortNodes sorts nodes by type and name
	SortNodes bool

	// ProcessingOptions contains information about what was included/excluded during processing
	ProcessingOptions ProcessingInfo

	// DebugLevel for verbose output (0=none, 1=basic, 2=detailed, 3=trace)
	DebugLevel int
}

// ProcessingInfo contains information about what was included/excluded during processing
type ProcessingInfo struct {
	IncludePublic         bool
	IncludeProtected      bool
	IncludeInternal       bool
	IncludePrivate        bool
	IncludeImplementation bool
	IncludeComments       bool
	IncludeImports        bool
	IncludeDocstrings     bool
	IncludeAnnotations    bool
	FilterImports         bool // Whether to filter unused imports
}

// BaseFormatter provides common functionality for formatters
type BaseFormatter struct {
	options Options
}

// NewBaseFormatter creates a base formatter with options
func NewBaseFormatter(options Options) BaseFormatter {
	return BaseFormatter{options: options}
}

// GetOptions returns the formatter options
func (f *BaseFormatter) GetOptions() Options {
	return f.options
}
