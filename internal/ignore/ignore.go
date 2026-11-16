package ignore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/janreges/ai-distiller/internal/project"
)

// IgnoreMatcher handles .aidignore file parsing and matching
type IgnoreMatcher struct {
	patterns     []pattern
	baseDir      string
	mu           sync.RWMutex
	cache        map[string]bool
	includeCache map[string]bool
	submatchers  map[string]*IgnoreMatcher // For nested .aidignore files
}

// pattern represents a single ignore pattern
type pattern struct {
	raw        string
	isNegation bool
	isDir      bool
	pattern    string
}

// New creates a new IgnoreMatcher for the given directory
func New(baseDir string) (*IgnoreMatcher, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	matcher := &IgnoreMatcher{
		baseDir:      absBase,
		patterns:     []pattern{},
		cache:        make(map[string]bool),
		includeCache: make(map[string]bool),
		submatchers:  make(map[string]*IgnoreMatcher),
	}

	// Load .aidignore from base directory if it exists. With AID_CONFINE_ROOT
	// set, an .aidignore whose real path lies outside it is not read.
	ignoreFile := filepath.Join(absBase, ".aidignore")
	if !project.InConfineRoot(ignoreFile) {
		return matcher, nil
	}
	if err := matcher.loadIgnoreFile(ignoreFile); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to load .aidignore: %w", err)
	}

	return matcher, nil
}

// loadIgnoreFile loads patterns from a .aidignore file
func (m *IgnoreMatcher) loadIgnoreFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		p := parsePattern(line)
		m.patterns = append(m.patterns, p)
	}

	return scanner.Err()
}

// parsePattern parses a single ignore pattern
func parsePattern(line string) pattern {
	p := pattern{raw: line}

	// Check for negation
	if strings.HasPrefix(line, "!") {
		p.isNegation = true
		line = line[1:]
	}

	// Check if pattern is for directories only
	if strings.HasSuffix(line, "/") {
		p.isDir = true
		line = strings.TrimSuffix(line, "/")
	}

	p.pattern = line
	return p
}

// ShouldIgnore checks if a path should be ignored
func (m *IgnoreMatcher) ShouldIgnore(path string) bool {
	// Debug logging
	// fmt.Fprintf(os.Stderr, "[DEBUG] ShouldIgnore called for: %s\n", path)
	m.mu.RLock()
	// Check cache first
	if ignored, ok := m.cache[path]; ok {
		m.mu.RUnlock()
		return ignored
	}
	m.mu.RUnlock()

	// Make path absolute for consistent matching
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Get relative path from base directory
	relPath, err := filepath.Rel(m.baseDir, absPath)
	if err != nil {
		return false
	}

	// Debug: Print the relative path and patterns
	// fmt.Fprintf(os.Stderr, "[DEBUG] Relative path: %s, Base: %s\n", relPath, m.baseDir)
	// for _, p := range m.patterns {
	// 	fmt.Fprintf(os.Stderr, "[DEBUG] Pattern: %s (isDir: %v)\n", p.pattern, p.isDir)
	// }

	// First check our own patterns
	ignored := m.matches(relPath)

	// Then check if there's a more specific .aidignore file that might override
	dir := filepath.Dir(absPath)
	if dir != m.baseDir && strings.HasPrefix(dir, m.baseDir) {
		// Check if we need to load a nested .aidignore
		m.mu.Lock()
		if _, exists := m.submatchers[dir]; !exists {
			ignoreFile := filepath.Join(dir, ".aidignore")
			if _, err := os.Stat(ignoreFile); err == nil {
				if submatcher, err := New(dir); err == nil {
					m.submatchers[dir] = submatcher
				}
			} else {
				// Mark as checked even if no .aidignore exists
				m.submatchers[dir] = nil
			}
		}
		submatcher := m.submatchers[dir]
		m.mu.Unlock()

		// If there's a submatcher, let it potentially override our decision
		if submatcher != nil {
			// Get relative path from submatcher's perspective
			subRelPath, err := filepath.Rel(dir, absPath)
			if err == nil {
				// Check if submatcher has a specific match (positive or negative)
				hasMatch, subIgnored := submatcher.matchesWithInfo(subRelPath)
				if hasMatch {
					// Submatcher has a specific pattern that matches, use its decision
					ignored = subIgnored
				}
				// If no match in submatcher, keep the inherited decision
			}
		}
	}

	// Cache the result
	m.mu.Lock()
	m.cache[path] = ignored
	m.mu.Unlock()

	return ignored
}

// matchesWithInfo checks if a relative path matches any pattern and returns whether there was a match
func (m *IgnoreMatcher) matchesWithInfo(relPath string) (hasMatch bool, ignored bool) {
	// Normalize path separators
	relPath = filepath.ToSlash(relPath)

	info, err := os.Stat(filepath.Join(m.baseDir, relPath))
	isDir := err == nil && info.IsDir()

	// Process patterns in order (last match wins for overlapping patterns)
	for _, p := range m.patterns {
		matched := m.matchPattern(relPath, p, isDir)
		if matched {
			hasMatch = true
			if p.isNegation {
				ignored = false
			} else {
				ignored = true
			}
		}
	}

	return hasMatch, ignored
}

// matches checks if a relative path matches any pattern
func (m *IgnoreMatcher) matches(relPath string) bool {
	_, ignored := m.matchesWithInfo(relPath)
	return ignored
}

// matchPattern checks if a path matches a single pattern
func (m *IgnoreMatcher) matchPattern(relPath string, p pattern, isDir bool) bool {
	matched := false
	pattern := p.pattern

	// For directory patterns, check if the path is within that directory
	if p.isDir {
		// Handle leading slash in pattern - remove it for comparison with relPath
		patternToMatch := pattern
		if strings.HasPrefix(pattern, "/") {
			patternToMatch = strings.TrimPrefix(pattern, "/")
		}

		if isDir && relPath == patternToMatch {
			// Directory itself matches
			matched = true
		} else if strings.HasPrefix(relPath, patternToMatch+"/") {
			// Path is inside the directory
			matched = true
		}
	} else {
		// Handle different pattern types
		if strings.Contains(pattern, "/") {
			// Path pattern
			if strings.HasPrefix(pattern, "/") {
				// Absolute pattern (relative to base dir)
				pattern = strings.TrimPrefix(pattern, "/")
				matched = matchPath(relPath, pattern)
			} else if strings.HasPrefix(pattern, "**/") {
				// Match anywhere in tree
				pattern = strings.TrimPrefix(pattern, "**/")
				matched = matchAnywhere(relPath, pattern)
			} else {
				// Relative pattern - can match at any level
				matched = matchRelative(relPath, pattern)
			}
		} else {
			// Simple name pattern - matches anywhere
			matched = matchName(relPath, pattern)

			// Also try to match as directory pattern (without trailing slash)
			// This handles cases like "bin" matching "bin/file.txt"
			if !matched {
				// Check if this could be a directory pattern
				if relPath == pattern {
					// Exact directory match
					matched = true
				} else if strings.HasPrefix(relPath, pattern+"/") {
					// Path is inside this directory
					matched = true
				}
			}
		}
	}

	return matched
}

// matchPath matches a path against a pattern
func matchPath(path, pattern string) bool {
	matched, _ := filepath.Match(pattern, path)
	if matched {
		return true
	}

	// Also check if path is under a matched directory
	if strings.HasSuffix(pattern, "/**") {
		dirPattern := strings.TrimSuffix(pattern, "/**")
		if strings.HasPrefix(path, dirPattern+"/") || path == dirPattern {
			return true
		}
	}

	// Handle directory matching
	dir := path
	for dir != "." && dir != "/" {
		if matched, _ := filepath.Match(pattern, dir); matched {
			return true
		}
		dir = filepath.Dir(dir)
	}

	return false
}

// matchAnywhere matches a pattern anywhere in the path
func matchAnywhere(path, pattern string) bool {
	// Try matching the full path
	if matched, _ := filepath.Match(pattern, path); matched {
		return true
	}

	// Try matching each component
	parts := strings.Split(path, "/")
	for i := range parts {
		subpath := strings.Join(parts[i:], "/")
		if matched, _ := filepath.Match(pattern, subpath); matched {
			return true
		}
	}

	return false
}

// matchRelative matches a relative pattern at any level
func matchRelative(path, pattern string) bool {
	// Check if the pattern matches the full path
	if matchPath(path, pattern) {
		return true
	}

	// Check if pattern matches any suffix of the path
	parts := strings.Split(path, "/")
	for i := range parts {
		subpath := strings.Join(parts[i:], "/")
		if matchPath(subpath, pattern) {
			return true
		}
	}

	return false
}

// matchName matches just the filename
func matchName(path, pattern string) bool {
	name := filepath.Base(path)
	matched, _ := filepath.Match(pattern, name)
	return matched
}

// Clear clears the cache
func (m *IgnoreMatcher) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cache = make(map[string]bool)
	m.includeCache = make(map[string]bool)
	m.submatchers = make(map[string]*IgnoreMatcher)
}

// IsExplicitlyIncluded checks if a path is explicitly included via a negation pattern
func (m *IgnoreMatcher) IsExplicitlyIncluded(path string) bool {
	m.mu.RLock()
	// Check cache first
	if included, ok := m.includeCache[path]; ok {
		m.mu.RUnlock()
		return included
	}
	m.mu.RUnlock()

	// Make path absolute for consistent matching
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}

	// Get relative path from base directory
	relPath, err := filepath.Rel(m.baseDir, absPath)
	if err != nil {
		return false
	}

	// Check if there's a more specific .aidignore file in a parent directory
	dir := filepath.Dir(absPath)
	if dir != m.baseDir && strings.HasPrefix(dir, m.baseDir) {
		// Check if we need to load a nested .aidignore
		m.mu.Lock()
		if _, exists := m.submatchers[dir]; !exists {
			ignoreFile := filepath.Join(dir, ".aidignore")
			if _, err := os.Stat(ignoreFile); err == nil {
				if submatcher, err := New(dir); err == nil {
					m.submatchers[dir] = submatcher
				}
			} else {
				// Mark as checked even if no .aidignore exists
				m.submatchers[dir] = nil
			}
		}
		submatcher := m.submatchers[dir]
		m.mu.Unlock()

		// If there's a submatcher, check it first
		if submatcher != nil {
			if submatcher.IsExplicitlyIncluded(absPath) {
				m.mu.Lock()
				m.includeCache[path] = true
				m.mu.Unlock()
				return true
			}
		}
	}

	// Check patterns for explicit inclusion
	included := m.hasExplicitInclude(relPath)

	// Cache the result
	m.mu.Lock()
	m.includeCache[path] = included
	m.mu.Unlock()

	return included
}

// hasExplicitInclude checks if a path has an explicit include pattern (negation)
func (m *IgnoreMatcher) hasExplicitInclude(relPath string) bool {
	// Normalize path separators
	relPath = filepath.ToSlash(relPath)

	info, err := os.Stat(filepath.Join(m.baseDir, relPath))
	isDir := err == nil && info.IsDir()

	// Process patterns to find explicit includes
	for _, p := range m.patterns {
		if !p.isNegation {
			continue // Skip non-negation patterns
		}

		matched := false
		pattern := p.pattern

		// For directory patterns, check if the path is within that directory
		if p.isDir {
			if isDir && relPath == pattern {
				// Directory itself matches
				matched = true
			} else if strings.HasPrefix(relPath, pattern+"/") {
				// Path is inside the directory
				matched = true
			}
		} else {
			// Handle different pattern types
			if strings.Contains(pattern, "/") {
				// Path pattern
				if strings.HasPrefix(pattern, "/") {
					// Absolute pattern (relative to base dir)
					pattern = strings.TrimPrefix(pattern, "/")
					matched = matchPath(relPath, pattern)
				} else if strings.HasPrefix(pattern, "**/") {
					// Match anywhere in tree
					pattern = strings.TrimPrefix(pattern, "**/")
					matched = matchAnywhere(relPath, pattern)
				} else {
					// Relative pattern - can match at any level
					matched = matchRelative(relPath, pattern)
				}
			} else {
				// Simple name pattern - matches anywhere
				matched = matchName(relPath, pattern)
			}
		}

		if matched {
			return true
		}
	}

	return false
}

// MightContainExplicitIncludes checks if a directory might contain explicitly included files
func (m *IgnoreMatcher) MightContainExplicitIncludes(dirPath string) bool {
	// Make path absolute for consistent matching
	absPath, err := filepath.Abs(dirPath)
	if err != nil {
		return false
	}

	// Get relative path from base directory
	relPath, err := filepath.Rel(m.baseDir, absPath)
	if err != nil {
		return false
	}

	// Normalize path separators
	relPath = filepath.ToSlash(relPath)
	if !strings.HasSuffix(relPath, "/") {
		relPath += "/"
	}

	// Check all negation patterns to see if any might match files in this directory
	for _, p := range m.patterns {
		if !p.isNegation {
			continue // Skip non-negation patterns
		}

		pattern := p.pattern

		// Simple check: if pattern contains this directory in its path, we might need to look inside
		if strings.Contains(pattern, "/") {
			// Remove leading slash if present
			if strings.HasPrefix(pattern, "/") {
				pattern = strings.TrimPrefix(pattern, "/")
			}

			// Check if pattern starts with or contains this directory
			if strings.HasPrefix(pattern, relPath) {
				return true // Pattern definitely refers to something in this directory
			}

			// Also check if the directory name appears in the pattern
			// This handles cases like "!node_modules/package/index.js" when we're checking "node_modules"
			dirName := filepath.Base(strings.TrimSuffix(relPath, "/"))
			if strings.Contains(pattern, dirName+"/") {
				return true
			}
		}
	}

	// Check submatchers
	for subDir, submatcher := range m.submatchers {
		if submatcher != nil && strings.HasPrefix(subDir, absPath) {
			if submatcher.MightContainExplicitIncludes(dirPath) {
				return true
			}
		}
	}

	return false
}

// matchSegment checks if a path segment matches a pattern segment
func matchSegment(segment, pattern string) bool {
	if pattern == "**" {
		return true
	}
	matched, _ := filepath.Match(pattern, segment)
	return matched
}
