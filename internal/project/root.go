package project

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"
)

const (
	// MaxSearchDepth limits upward directory traversal to prevent excessive searching
	MaxSearchDepth = 12

	// AidDirName is the name of the directory where aid stores its data
	AidDirName = ".aid"

	// EnvProjectRoot is the environment variable for overriding project root detection
	EnvProjectRoot = "AID_PROJECT_ROOT"
)

// rootMarkers defines project root indicators in priority order
var rootMarkers = []string{
	".aidrc",         // AI Distiller specific config (highest priority)
	"go.mod",         // Go projects
	"package.json",   // Node.js projects
	"Cargo.toml",     // Rust projects
	"pyproject.toml", // Modern Python projects
	"setup.py",       // Legacy Python projects
	"pom.xml",        // Java Maven projects
	"build.gradle",   // Java Gradle projects
	".git",           // Version control (lowest priority)
}

// rootCache caches the detected root for the process lifetime
var (
	rootCache     string
	rootCacheMu   sync.RWMutex
	rootCacheOnce sync.Once
)

// RootInfo contains information about the detected project root
type RootInfo struct {
	Path       string // Absolute path to the project root
	Marker     string // The marker file that identified this as root (e.g., "go.mod")
	IsFallback bool   // True if no markers were found and CWD was used
}

// FindRoot detects the project root directory using a hierarchical search strategy.
// It searches upward from the current working directory for project markers.
//
// Search priority:
// 1. Upward search for markers (.aidrc, go.mod, package.json, etc.)
// 2. AID_PROJECT_ROOT environment variable (fallback if no markers found)
// 3. Current working directory (final fallback)
func FindRoot() (*RootInfo, error) {
	// Check cache first
	rootCacheMu.RLock()
	if rootCache != "" {
		info := &RootInfo{Path: rootCache, Marker: "cached"}
		rootCacheMu.RUnlock()
		return info, nil
	}
	rootCacheMu.RUnlock()

	// Find root only once per process
	var info *RootInfo
	var err error
	rootCacheOnce.Do(func() {
		info, err = findRootUncached()
		if err == nil && info != nil {
			rootCacheMu.Lock()
			rootCache = info.Path
			rootCacheMu.Unlock()
		}
	})

	// For subsequent calls, return cached value
	if info == nil && err == nil {
		rootCacheMu.RLock()
		info = &RootInfo{Path: rootCache, Marker: "cached"}
		rootCacheMu.RUnlock()
	}

	return info, err
}

// findRootUncached performs the actual root detection without caching
func findRootUncached() (*RootInfo, error) {
	// 1. Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("cannot get current working directory: %w", err)
	}

	// 2. Check if we should stop at certain boundaries
	homeDir, _ := os.UserHomeDir()

	// 3. Search upward for project markers
	currentDir := cwd
	for depth := 0; depth < MaxSearchDepth; depth++ {
		// Check each marker in priority order
		for _, marker := range rootMarkers {
			markerPath := filepath.Join(currentDir, marker)
			if _, err := os.Stat(markerPath); err == nil {
				// Found a marker - this is our project root
				return &RootInfo{
					Path:   currentDir,
					Marker: marker,
				}, nil
			}
		}

		// Security boundary check: stop at home directory
		if homeDir != "" && currentDir == homeDir {
			// Don't traverse above home directory for security
			break
		}

		// Move to parent directory
		parent := filepath.Dir(currentDir)
		if parent == currentDir {
			// Reached filesystem root
			break
		}

		// Additional security check for Unix systems
		if runtime.GOOS != "windows" && parent == "/" {
			// Don't search in root directory on Unix systems
			break
		}

		currentDir = parent
	}

	// 4. Check environment variable as fallback (lower priority than markers)
	if envRoot := os.Getenv(EnvProjectRoot); envRoot != "" {
		absRoot, err := filepath.Abs(envRoot)
		if err == nil {
			if info, err := os.Stat(absRoot); err == nil && info.IsDir() {
				return &RootInfo{
					Path:   absRoot,
					Marker: "AID_PROJECT_ROOT",
				}, nil
			}
		}
		log.Printf("WARN: %s is set to '%s', but it is not a valid directory. Ignoring.", EnvProjectRoot, envRoot)
	}

	// 5. Fallback to current working directory
	return &RootInfo{
		Path:       cwd,
		IsFallback: true,
	}, nil
}

// GetAidDir returns the path to the .aid directory within the project root
func GetAidDir() (string, error) {
	info, err := FindRoot()
	if err != nil {
		return "", err
	}

	if info.IsFallback {
		log.Println("WARN: No project root found. Using current directory. " +
			"For consistent behavior, create an '.aidrc' file at your project root.")
	}

	return filepath.Join(info.Path, AidDirName), nil
}

// GetCacheDir returns the path to the cache directory within .aid
func GetCacheDir() (string, error) {
	aidDir, err := GetAidDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(aidDir, "cache"), nil
}

// EnsureAidDir creates the .aid directory if it doesn't exist
func EnsureAidDir() (string, error) {
	aidDir, err := GetAidDir()
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(aidDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create .aid directory: %w", err)
	}

	return aidDir, nil
}

// ResetCache clears the cached root (mainly for testing)
func ResetCache() {
	rootCacheMu.Lock()
	defer rootCacheMu.Unlock()
	rootCache = ""
	rootCacheOnce = sync.Once{}
}
