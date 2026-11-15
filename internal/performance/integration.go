package performance

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
)

// PerformanceMode defines different performance optimization modes
type PerformanceMode int

const (
	// ModeStandard uses regular processing without optimizations
	ModeStandard PerformanceMode = iota
	// ModeConcurrent uses concurrent processing for multiple files
	ModeConcurrent
	// ModeStreaming uses streaming for very large files
	ModeStreaming
	// ModeCached uses caching for repeated processing
	ModeCached
	// ModeOptimal automatically selects the best mode based on input
	ModeOptimal
)

// PerformanceProcessor combines all performance optimizations
type PerformanceProcessor struct {
	standardProcessor   *processor.Processor
	concurrentProcessor *ConcurrentProcessor
	streamingProcessor  *StreamingProcessor
	cachedProcessor     *CachedProcessor
	cacheDir           string
	config             *PerformanceConfig
}

// PerformanceConfig defines performance optimization settings
type PerformanceConfig struct {
	// Concurrent processing settings
	MaxWorkers         int
	BufferSize         int
	
	// Streaming settings
	ChunkSize          int
	StreamBufferSize   int
	MaxMemoryMB        int64
	
	// Cache settings
	CacheEnabled       bool
	CacheMaxSize       int64
	CacheMaxAge        time.Duration
	
	// Thresholds for mode selection
	LargeFileThresholdMB   int64  // Files larger than this use streaming
	ManyFilesThreshold     int    // File counts larger than this use concurrent
	EnableAutoOptimization bool
}

// DefaultPerformanceConfig returns optimized default settings
func DefaultPerformanceConfig() *PerformanceConfig {
	return &PerformanceConfig{
		MaxWorkers:             8,
		BufferSize:             1024,
		ChunkSize:              1000,
		StreamBufferSize:       64 * 1024,
		MaxMemoryMB:            512,
		CacheEnabled:           true,
		CacheMaxSize:           1024 * 1024 * 1024, // 1GB
		CacheMaxAge:            24 * time.Hour,
		LargeFileThresholdMB:   50,
		ManyFilesThreshold:     10,
		EnableAutoOptimization: true,
	}
}

// NewPerformanceProcessor creates a new performance-optimized processor
func NewPerformanceProcessor(cacheDir string) *PerformanceProcessor {
	config := DefaultPerformanceConfig()
	
	return &PerformanceProcessor{
		standardProcessor:   processor.New(),
		concurrentProcessor: NewConcurrentProcessor().WithWorkers(config.MaxWorkers).WithBufferSize(config.BufferSize),
		streamingProcessor:  NewStreamingProcessor().WithChunkSize(config.ChunkSize).WithBufferSize(config.StreamBufferSize).WithMemoryLimit(config.MaxMemoryMB),
		cachedProcessor:     NewCachedProcessor(cacheDir),
		cacheDir:           cacheDir,
		config:             config,
	}
}

// WithConfig sets custom performance configuration
func (p *PerformanceProcessor) WithConfig(config *PerformanceConfig) *PerformanceProcessor {
	p.config = config
	
	// Update sub-processors
	p.concurrentProcessor = p.concurrentProcessor.WithWorkers(config.MaxWorkers).WithBufferSize(config.BufferSize)
	p.streamingProcessor = p.streamingProcessor.WithChunkSize(config.ChunkSize).WithBufferSize(config.StreamBufferSize).WithMemoryLimit(config.MaxMemoryMB)
	
	if config.CacheEnabled {
		p.cachedProcessor = NewCachedProcessor(p.cacheDir)
		p.cachedProcessor.GetCache().WithMaxSize(config.CacheMaxSize).WithMaxAge(config.CacheMaxAge)
	}
	
	return p
}

// ProcessFile processes a single file with optimal performance mode
func (p *PerformanceProcessor) ProcessFile(
	ctx context.Context,
	filePath string,
	opts processor.ProcessOptions,
) (*ir.DistilledFile, error) {
	mode := p.selectOptimalMode([]string{filePath}, opts)
	return p.ProcessFileWithMode(ctx, filePath, opts, mode)
}

// ProcessFileWithMode processes a file with a specific performance mode
func (p *PerformanceProcessor) ProcessFileWithMode(
	ctx context.Context,
	filePath string,
	opts processor.ProcessOptions,
	mode PerformanceMode,
) (*ir.DistilledFile, error) {
	switch mode {
	case ModeStandard:
		return p.standardProcessor.ProcessFile(filePath, opts)
		
	case ModeCached:
		if p.config.CacheEnabled {
			return p.cachedProcessor.ProcessFile(filePath, opts)
		}
		return p.standardProcessor.ProcessFile(filePath, opts)
		
	case ModeStreaming:
		result, err := p.streamingProcessor.ProcessLargeFile(ctx, filePath, opts)
		if err != nil {
			return nil, err
		}
		return result.File, nil
		
	case ModeConcurrent:
		// For single files, concurrent mode falls back to cached/standard
		if p.config.CacheEnabled {
			return p.cachedProcessor.ProcessFile(filePath, opts)
		}
		return p.standardProcessor.ProcessFile(filePath, opts)
		
	case ModeOptimal:
		return p.ProcessFile(ctx, filePath, opts)
		
	default:
		return p.standardProcessor.ProcessFile(filePath, opts)
	}
}

// ProcessFiles processes multiple files with optimal performance
func (p *PerformanceProcessor) ProcessFiles(
	ctx context.Context,
	filePaths []string,
	opts processor.ProcessOptions,
) (*BatchResult, error) {
	mode := p.selectOptimalMode(filePaths, opts)
	return p.ProcessFilesWithMode(ctx, filePaths, opts, mode)
}

// ProcessFilesWithMode processes files with a specific performance mode
func (p *PerformanceProcessor) ProcessFilesWithMode(
	ctx context.Context,
	filePaths []string,
	opts processor.ProcessOptions,
	mode PerformanceMode,
) (*BatchResult, error) {
	switch mode {
	case ModeStandard:
		return p.processFilesStandard(ctx, filePaths, opts)
		
	case ModeConcurrent, ModeOptimal:
		return p.concurrentProcessor.ProcessFiles(ctx, filePaths, opts)
		
	case ModeCached:
		return p.processFilesCached(ctx, filePaths, opts)
		
	case ModeStreaming:
		return p.processFilesStreaming(ctx, filePaths, opts)
		
	default:
		return p.concurrentProcessor.ProcessFiles(ctx, filePaths, opts)
	}
}

// ProcessDirectory processes all files in a directory with optimal performance
func (p *PerformanceProcessor) ProcessDirectory(
	ctx context.Context,
	dirPath string,
	opts processor.ProcessOptions,
) (*BatchResult, error) {
	return p.concurrentProcessor.ProcessDirectory(ctx, dirPath, opts)
}

// selectOptimalMode automatically selects the best performance mode
func (p *PerformanceProcessor) selectOptimalMode(filePaths []string, opts processor.ProcessOptions) PerformanceMode {
	if !p.config.EnableAutoOptimization {
		return ModeStandard
	}

	// Check for many files -> use concurrent
	if len(filePaths) >= p.config.ManyFilesThreshold {
		return ModeConcurrent
	}

	// Check for large files -> use streaming
	for _, filePath := range filePaths {
		if info, err := os.Stat(filePath); err == nil {
			fileSizeMB := info.Size() / 1024 / 1024
			if fileSizeMB >= p.config.LargeFileThresholdMB {
				return ModeStreaming
			}
		}
	}

	// Default to cached mode for small files
	if p.config.CacheEnabled {
		return ModeCached
	}

	return ModeStandard
}

// processFilesStandard processes files sequentially without optimizations
func (p *PerformanceProcessor) processFilesStandard(
	ctx context.Context,
	filePaths []string,
	opts processor.ProcessOptions,
) (*BatchResult, error) {
	var files []*ir.DistilledFile
	var errors []error

	for _, filePath := range filePaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		file, err := p.standardProcessor.ProcessFile(filePath, opts)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to process %s: %w", filePath, err))
		} else {
			files = append(files, file)
		}
	}

	return &BatchResult{
		Files:   files,
		Errors:  errors,
		Metrics: &ProcessingMetrics{FilesProcessed: int64(len(files))},
	}, nil
}

// processFilesCached processes files with caching
func (p *PerformanceProcessor) processFilesCached(
	ctx context.Context,
	filePaths []string,
	opts processor.ProcessOptions,
) (*BatchResult, error) {
	var files []*ir.DistilledFile
	var errors []error

	for _, filePath := range filePaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		file, err := p.cachedProcessor.ProcessFile(filePath, opts)
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to process %s: %w", filePath, err))
		} else {
			files = append(files, file)
		}
	}

	// Get cache metrics
	var metrics *ProcessingMetrics
	if cacheStats := p.cachedProcessor.GetCache().Stats(); cacheStats != nil {
		metrics = &ProcessingMetrics{
			FilesProcessed: int64(len(files)),
			CacheHits:      cacheStats.Hits,
			CacheMisses:    cacheStats.Misses,
		}
	} else {
		metrics = &ProcessingMetrics{FilesProcessed: int64(len(files))}
	}

	return &BatchResult{
		Files:   files,
		Errors:  errors,
		Metrics: metrics,
	}, nil
}

// processFilesStreaming processes files using streaming for large files
func (p *PerformanceProcessor) processFilesStreaming(
	ctx context.Context,
	filePaths []string,
	opts processor.ProcessOptions,
) (*BatchResult, error) {
	var files []*ir.DistilledFile
	var errors []error

	for _, filePath := range filePaths {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Check file size
		if info, err := os.Stat(filePath); err == nil {
			fileSizeMB := info.Size() / 1024 / 1024
			
			if fileSizeMB >= p.config.LargeFileThresholdMB {
				// Use streaming for large files
				result, err := p.streamingProcessor.ProcessLargeFile(ctx, filePath, opts)
				if err != nil {
					errors = append(errors, fmt.Errorf("failed to stream process %s: %w", filePath, err))
				} else {
					files = append(files, result.File)
				}
			} else {
				// Use standard processing for small files
				file, err := p.standardProcessor.ProcessFile(filePath, opts)
				if err != nil {
					errors = append(errors, fmt.Errorf("failed to process %s: %w", filePath, err))
				} else {
					files = append(files, file)
				}
			}
		} else {
			errors = append(errors, fmt.Errorf("failed to stat %s: %w", filePath, err))
		}
	}

	return &BatchResult{
		Files:   files,
		Errors:  errors,
		Metrics: &ProcessingMetrics{FilesProcessed: int64(len(files))},
	}, nil
}

// GetMetrics returns comprehensive performance metrics
func (p *PerformanceProcessor) GetMetrics() *PerformanceMetrics {
	metrics := &PerformanceMetrics{
		ConcurrentMetrics: &ProcessingMetrics{},
		CacheMetrics:      &CacheMetrics{},
	}

	// Concurrent processor metrics
	if concurrentMetrics := p.concurrentProcessor.GetMetrics(); concurrentMetrics != nil {
		metrics.ConcurrentMetrics = concurrentMetrics
	}

	// Cache metrics
	if p.config.CacheEnabled {
		if cacheStats := p.cachedProcessor.GetCache().Stats(); cacheStats != nil {
			metrics.CacheMetrics = cacheStats
		}
	}

	return metrics
}

// PerformanceMetrics combines all performance metrics
type PerformanceMetrics struct {
	ConcurrentMetrics *ProcessingMetrics
	CacheMetrics      *CacheMetrics
}

// String returns formatted performance metrics
func (m *PerformanceMetrics) String() string {
	return fmt.Sprintf(
		"=== Performance Metrics ===\n\n%s\n\n%s",
		m.ConcurrentMetrics.String(),
		m.CacheMetrics.String(),
	)
}

// ResetMetrics resets all performance metrics
func (p *PerformanceProcessor) ResetMetrics() {
	p.concurrentProcessor.ResetMetrics()
	if p.config.CacheEnabled {
		// Cache metrics are reset by clearing the cache
		_ = p.cachedProcessor.GetCache().Clear()
	}
}