package processor

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/stripper"
)

// LanguageProcessor defines the interface for language-specific processors
type LanguageProcessor interface {
	// Language returns the language identifier (e.g., "go", "python", "javascript")
	Language() string

	// Version returns the processor version
	Version() string

	// SupportedExtensions returns file extensions this processor handles
	SupportedExtensions() []string

	// CanProcess checks if this processor can handle the given file
	CanProcess(filename string) bool

	// Process parses source code and returns the IR representation
	Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error)

	// ProcessWithOptions parses with specific options
	ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts ProcessOptions) (*ir.DistilledFile, error)
}

// ProcessOptions configures the processing behavior
type ProcessOptions struct {
	// IncludeImplementation includes function/method bodies
	IncludeImplementation bool

	// IncludeComments includes comment nodes
	IncludeComments bool

	// IncludeImports includes import statements
	IncludeImports bool

	// IncludePrivate includes private/internal declarations (legacy - removes both private and protected)
	IncludePrivate bool
	
	// RemovePrivateOnly removes only private members (not protected)
	RemovePrivateOnly bool
	
	// RemoveProtectedOnly removes only protected members (not private)
	RemoveProtectedOnly bool
	
	// RemoveInternalOnly removes only internal/package-private members
	RemoveInternalOnly bool
	
	// IncludePublic includes public members (for formatter instructions)
	IncludePublic bool
	
	// IncludeProtected includes protected members (for formatter instructions)
	IncludeProtected bool
	
	// IncludeInternal includes internal/package-private members (for formatter instructions)
	IncludeInternal bool
	
	// IncludeDocstrings includes documentation comments (when false, removes them)
	IncludeDocstrings bool
	
	// IncludeAnnotations includes decorators/annotations (when false, removes them)
	IncludeAnnotations bool
	
	// IncludeFields includes class fields/properties (when false, removes them)
	IncludeFields bool
	
	// IncludeMethods includes methods/functions (when false, removes them)
	IncludeMethods bool

	// ExpandSymbols holds glob patterns; functions/methods whose name matches
	// keep their implementation even when implementations are otherwise stripped.
	ExpandSymbols []string

	// MaxDepth limits the depth of nested structures
	MaxDepth int

	// SymbolResolution enables symbol cross-referencing
	SymbolResolution bool

	// IncludeLineNumbers adds line number information
	IncludeLineNumbers bool
	
	// Workers specifies the number of parallel workers for processing
	// 0 = auto (80% of CPU cores), 1 = serial processing
	Workers int
	
	// RawMode processes all text files without parsing
	RawMode bool
	
	// Recursive controls whether to process directories recursively
	Recursive bool
	
	// BasePath is the original path provided by the user (for relative path calculation)
	BasePath string
	
	// FilePathType controls how paths appear in output: "relative" or "absolute"
	FilePathType string
	
	// RelativePathPrefix is a custom prefix for relative paths (e.g., "src/")
	RelativePathPrefix string
	
	// IncludePatterns are file patterns to include (e.g., "*.go", "*.py")
	IncludePatterns []string
	
	// ExcludePatterns are file patterns to exclude (e.g., "*test*", "*.json")
	ExcludePatterns []string
	
	// ExplicitInclude indicates this file was explicitly included via !pattern
	ExplicitInclude bool
}

// DefaultProcessOptions returns default processing options
func DefaultProcessOptions() ProcessOptions {
	return ProcessOptions{
		IncludeImplementation: true,
		IncludeComments:       true,
		IncludeImports:        true,
		IncludePrivate:        true,
		RemovePrivateOnly:     false,
		RemoveProtectedOnly:   false,
		RemoveInternalOnly:    false,
		IncludeDocstrings:     true,
		IncludeAnnotations:    true,
		IncludeFields:         true,
		IncludeMethods:        true,
		MaxDepth:              100,
		SymbolResolution:      true,
		IncludeLineNumbers:    true,
		Workers:               0, // 0 = auto (80% of CPU cores)
		Recursive:             true,
		FilePathType:          "relative",
		RelativePathPrefix:    "",
		BasePath:              "",
	}
}

// Registry manages language processors
type Registry interface {
	// Register adds a processor to the registry
	Register(processor LanguageProcessor) error

	// Get returns a processor for the given language
	Get(language string) (LanguageProcessor, bool)

	// GetByFilename returns a processor that can handle the file
	GetByFilename(filename string) (LanguageProcessor, bool)

	// List returns all registered language identifiers
	List() []string
}

// ProcessorError represents a processing error with context
type ProcessorError struct {
	File     string
	Line     int
	Column   int
	Message  string
	Severity string // "error", "warning", "info"
	Code     string // Error code for tooling
}

func (e ProcessorError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
	}
	return fmt.Sprintf("%s: %s", e.File, e.Message)
}

// MultiError represents multiple processing errors
type MultiError struct {
	Errors []error
}

func (e MultiError) Error() string {
	if len(e.Errors) == 0 {
		return "no errors"
	}
	if len(e.Errors) == 1 {
		return e.Errors[0].Error()
	}
	return fmt.Sprintf("%d errors occurred", len(e.Errors))
}

// BaseProcessor provides common functionality for language processors
type BaseProcessor struct {
	language   string
	version    string
	extensions []string
}

// NewBaseProcessor creates a new base processor
func NewBaseProcessor(language, version string, extensions []string) BaseProcessor {
	return BaseProcessor{
		language:   language,
		version:    version,
		extensions: extensions,
	}
}

// Language implements LanguageProcessor
func (p BaseProcessor) Language() string {
	return p.language
}

// Version implements LanguageProcessor
func (p BaseProcessor) Version() string {
	return p.version
}

// SupportedExtensions implements LanguageProcessor
func (p BaseProcessor) SupportedExtensions() []string {
	return p.extensions
}

// CanProcess implements LanguageProcessor
func (p BaseProcessor) CanProcess(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	for _, supported := range p.extensions {
		if ext == supported {
			return true
		}
	}
	return false
}

// ToStripperOptions converts ProcessOptions to stripper.Options
func (opts ProcessOptions) ToStripperOptions() stripper.Options {
	return stripper.Options{
		RemovePrivate:         !opts.IncludePrivate,
		RemovePrivateOnly:     opts.RemovePrivateOnly,
		RemoveProtectedOnly:   opts.RemoveProtectedOnly,
		RemoveInternalOnly:    opts.RemoveInternalOnly,
		RemoveImplementations: !opts.IncludeImplementation,
		RemoveComments:        !opts.IncludeComments,
		RemoveImports:         !opts.IncludeImports,
		RemoveDocstrings:      !opts.IncludeDocstrings,
		RemoveAnnotations:     !opts.IncludeAnnotations,
		RemoveFields:          !opts.IncludeFields,
		RemoveMethods:         !opts.IncludeMethods,
		ExpandSymbols:         opts.ExpandSymbols,
	}
}