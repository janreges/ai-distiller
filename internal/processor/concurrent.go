package processor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/janreges/ai-distiller/internal/ignore"
	"github.com/janreges/ai-distiller/internal/ir"
)

// FileTask represents a file to be processed with its order index
type FileTask struct {
	Index           int
	Path            string
	FileInfo        os.FileInfo
	ExplicitInclude bool
}

// FileResult represents the result of processing a file
type FileResult struct {
	Index  int
	Result *ir.DistilledFile
	Error  error
}

// processDirectoryConcurrent processes directory using multiple workers
func (p *Processor) processDirectoryConcurrent(dir string, opts ProcessOptions) (*ir.DistilledDirectory, error) {
	// Create ignore matcher for the directory
	ignoreMatcher, ignoreErr := ignore.New(dir)
	if ignoreErr != nil {
		// Log warning but continue without ignore functionality
		fmt.Fprintf(os.Stderr, "Warning: failed to create ignore matcher: %v\n", ignoreErr)
		ignoreMatcher = nil
	}

	// Calculate number of workers
	numWorkers := opts.Workers
	if numWorkers < 0 {
		// Negative counts used to run serially; keep that as one worker.
		numWorkers = 1
	} else if numWorkers == 0 {
		// Default to 80% of CPU cores
		numWorkers = int(float64(runtime.NumCPU()) * 0.8)
		if numWorkers < 1 {
			numWorkers = 1
		}
	}

	// First, collect all files to process
	var files []FileTask
	fileIndex := 0
	
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Check if path should be ignored
		if ignoreMatcher != nil && ignoreMatcher.ShouldIgnore(path) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip directories
		if info.IsDir() {
			basename := filepath.Base(path)
			
			// Skip .aid directories completely
			if basename == ".aid" {
				return filepath.SkipDir
			}
			
			// Skip default ignored directories unless explicitly included in .aidignore
			// or unless they contain explicitly included files
			if isDefaultIgnoredDir(basename) && ignoreMatcher != nil {
				// Check if directory is explicitly included
				if ignoreMatcher.IsExplicitlyIncluded(path) {
					return nil // Don't skip, process the directory
				}
				// Check if any files within this directory might be explicitly included
				if !ignoreMatcher.MightContainExplicitIncludes(path) {
					return filepath.SkipDir
				}
			} else if isDefaultIgnoredDir(basename) && ignoreMatcher == nil {
				// No .aidignore, skip default ignored dirs
				return filepath.SkipDir
			}
			
			// If not recursive and not the root directory, skip subdirectories
			if !opts.Recursive && path != dir {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip files containing '.aid.' anywhere in filename
		basename := filepath.Base(path)
		if strings.Contains(basename, ".aid.") {
			return nil
		}

		// Check include/exclude patterns; directory names above the walk root never match
		relPath, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			relPath = path
		}
		if !matchPatterns(relPath, path, opts.IncludePatterns, opts.ExcludePatterns) {
			return nil
		}

		// Check if file is explicitly included via !pattern in .aidignore
		explicitlyIncluded := ignoreMatcher != nil && ignoreMatcher.IsExplicitlyIncluded(path)
		
		// Check if we can process this file
		if opts.RawMode {
			// In raw mode, process all files
			// Skip directories which are already filtered out above
		} else {
			// Normal mode - check if we have a processor
			_, hasProcessor := GetByFilename(path)
			
			if !hasProcessor && !explicitlyIncluded {
				return nil
			}
		}

		files = append(files, FileTask{
			Index:           fileIndex,
			Path:            path,
			FileInfo:        info,
			ExplicitInclude: explicitlyIncluded,
		})
		fileIndex++
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	// Calculate display path for directory
	displayPath := dir
	if opts.FilePathType == "relative" && opts.BasePath != "" {
		// Try to make path relative to base path
		absBase, err := filepath.Abs(opts.BasePath)
		if err == nil {
			relPath, err := filepath.Rel(absBase, dir)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				displayPath = relPath
				
				// Apply prefix if specified
				if opts.RelativePathPrefix != "" {
					prefix := opts.RelativePathPrefix
					// Ensure prefix ends with separator if not empty and doesn't already
					if !strings.HasSuffix(prefix, "/") && !strings.HasSuffix(prefix, string(filepath.Separator)) {
						prefix += "/"
					}
					displayPath = prefix + displayPath
				}
			}
		}
	}

	// If no files to process, return empty result
	if len(files) == 0 {
		return &ir.DistilledDirectory{
			BaseNode: ir.BaseNode{},
			Path:     displayPath,
			Children: []ir.DistilledNode{},
		}, nil
	}

	// Create channels for task distribution and result collection
	taskChan := make(chan FileTask, numWorkers*2)
	resultChan := make(chan FileResult, len(files))

	// Create wait group for workers
	var wg sync.WaitGroup

	// Start workers
	ctx := context.Background()
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			p.worker(ctx, workerID, taskChan, resultChan, opts)
		}(i)
	}

	// Send tasks to workers
	go func() {
		for _, task := range files {
			taskChan <- task
		}
		close(taskChan)
	}()

	// Wait for all workers to complete
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect results
	results := make([]FileResult, 0, len(files))
	for result := range resultChan {
		results = append(results, result)
	}

	// Sort results by index to maintain order
	sort.Slice(results, func(i, j int) bool {
		return results[i].Index < results[j].Index
	})

	// Build final directory result
	dirResult := &ir.DistilledDirectory{
		BaseNode: ir.BaseNode{},
		Path:     displayPath,
		Children: make([]ir.DistilledNode, 0, len(results)),
	}

	// Add successful results and report errors
	for _, result := range results {
		if result.Error != nil {
			// Log error but continue (same behavior as serial processing)
			fmt.Fprintf(os.Stderr, "Warning: failed to process %s: %v\n", files[result.Index].Path, result.Error)
		} else if result.Result != nil {
			dirResult.Children = append(dirResult.Children, result.Result)
		}
	}

	return dirResult, nil
}

// worker processes files from the task channel
func (p *Processor) worker(ctx context.Context, workerID int, tasks <-chan FileTask, results chan<- FileResult, opts ProcessOptions) {
	for task := range tasks {
		select {
		case <-ctx.Done():
			return
		default:
			// Process the file
			fileOpts := opts
			fileOpts.ExplicitInclude = task.ExplicitInclude
			file, err := p.ProcessFile(task.Path, fileOpts)
			results <- FileResult{
				Index:  task.Index,
				Result: file,
				Error:  err,
			}
		}
	}
}

// calculateWorkers determines the number of workers to use
func calculateWorkers(requested int) int {
	if requested > 0 {
		return requested
	}
	
	// Default to 80% of CPU cores
	numCPU := runtime.NumCPU()
	workers := int(float64(numCPU) * 0.8)
	if workers < 1 {
		workers = 1
	}
	return workers
}