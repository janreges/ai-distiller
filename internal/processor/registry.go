package processor

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"
)

// registry is the default implementation of Registry
type registry struct {
	mu         sync.RWMutex
	processors map[string]LanguageProcessor
	byExt      map[string]LanguageProcessor
}

// NewRegistry creates a new processor registry
func NewRegistry() Registry {
	return &registry{
		processors: make(map[string]LanguageProcessor),
		byExt:      make(map[string]LanguageProcessor),
	}
}

// Register adds a processor to the registry
func (r *registry) Register(processor LanguageProcessor) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	lang := processor.Language()
	if lang == "" {
		return fmt.Errorf("processor language cannot be empty")
	}

	// Check for duplicate registration
	if _, exists := r.processors[lang]; exists {
		return fmt.Errorf("processor for language %q already registered", lang)
	}

	// Register by language
	r.processors[lang] = processor

	// Register by extensions
	for _, ext := range processor.SupportedExtensions() {
		if ext == "" {
			continue
		}
		// Normalize extension (ensure it starts with a dot)
		if !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		r.byExt[ext] = processor
	}

	return nil
}

// Get returns a processor for the given language
func (r *registry) Get(language string) (LanguageProcessor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	processor, ok := r.processors[language]
	return processor, ok
}

// GetByFilename returns a processor that can handle the file
func (r *registry) GetByFilename(filename string) (LanguageProcessor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ext := strings.ToLower(filepath.Ext(filename))
	processor, ok := r.byExt[ext]
	return processor, ok
}

// List returns all registered language identifiers
func (r *registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	languages := make([]string, 0, len(r.processors))
	for lang := range r.processors {
		languages = append(languages, lang)
	}
	return languages
}

// defaultRegistry is the global processor registry
var defaultRegistry = NewRegistry()

// Register adds a processor to the default registry
func Register(processor LanguageProcessor) error {
	return defaultRegistry.Register(processor)
}

// Get returns a processor from the default registry
func Get(language string) (LanguageProcessor, bool) {
	return defaultRegistry.Get(language)
}

// GetByFilename returns a processor from the default registry
func GetByFilename(filename string) (LanguageProcessor, bool) {
	return defaultRegistry.GetByFilename(filename)
}

// List returns all languages in the default registry
func List() []string {
	return defaultRegistry.List()
}

// MustRegister registers a processor and panics on error
func MustRegister(processor LanguageProcessor) {
	if err := Register(processor); err != nil {
		panic(fmt.Sprintf("failed to register processor: %v", err))
	}
}
