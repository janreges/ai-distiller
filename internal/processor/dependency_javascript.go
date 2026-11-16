package processor

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
)

// tsConfig is what --dependency-aware reads from a tsconfig.json or
// jsconfig.json: compilerOptions.baseUrl and compilerOptions.paths, with
// relative "extends" applied.
type tsConfig struct {
	baseURL  string              // absolute baseUrl, "" when unset
	paths    map[string][]string // compilerOptions.paths
	pathsDir string              // the directory of the config that declares paths
}

var (
	jsFromClause = regexp.MustCompile(`\bfrom\s*["']([^"'\n]+)["']`)
	jsSideEffect = regexp.MustCompile(`(?m)^\s*import\s*["']([^"'\n]+)["']`)
	jsCall       = regexp.MustCompile(`\b(?:import|require)\s*\(\s*["']([^"'\n]+)["']\s*\)`)
)

// jsExtensions are the extensions tried for an import without one, in
// TypeScript's order of preference.
var jsExtensions = []string{".ts", ".tsx", ".d.ts", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}

// extractJavaScriptImports returns the files a JavaScript or TypeScript file
// imports, in source order: the module of every "import ... from", "export
// ... from", side-effect import, dynamic import() and require() that is a
// relative path or matches compilerOptions.paths (or baseUrl) of the
// nearest tsconfig.json or jsconfig.json. Statements may span lines. Packages
// from node_modules and Node built-ins are not followed.
func (da *DependencyAnalyzer) extractJavaScriptImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	code := stripSlashComments(string(fileContent))

	type spec struct {
		pos  int
		name string
	}
	var specs []spec
	for _, re := range []*regexp.Regexp{jsFromClause, jsSideEffect, jsCall} {
		for _, m := range re.FindAllStringSubmatchIndex(code, -1) {
			specs = append(specs, spec{pos: m[2], name: code[m[2]:m[3]]})
		}
	}
	sort.SliceStable(specs, func(i, j int) bool { return specs[i].pos < specs[j].pos })

	var deps []string
	for _, s := range specs {
		if f := da.resolveJSModule(s.name, filePath); f != "" {
			deps = append(deps, f)
			dbg.Logf(debug.LevelDetailed, "JS import %s -> %s", s.name, f)
		}
	}
	return deps, nil
}

// resolveJSModule returns the file an import specifier names, or "".
func (da *DependencyAnalyzer) resolveJSModule(spec, fromFile string) string {
	if i := strings.IndexAny(spec, "?#"); i >= 0 {
		spec = spec[:i] // "./icon.svg?react", "#internal"
	}
	if spec == "" {
		return ""
	}
	if spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return resolveJSPath(filepath.Join(filepath.Dir(fromFile), filepath.FromSlash(spec)))
	}
	if strings.HasPrefix(spec, "/") || strings.Contains(spec, ":") {
		return "" // a URL path, "node:fs", "https://..."
	}
	cfg := da.tsConfigFor(filepath.Dir(fromFile))
	if cfg == nil {
		return ""
	}
	for _, target := range cfg.match(spec) {
		if f := resolveJSPath(target); f != "" {
			return f
		}
	}
	return ""
}

// resolveJSPath finds the file for an import path without its extension
// resolution: the path itself, the TypeScript source of a ".js" path (ESM
// imports name the compiled file), the path with each extension, then its
// index file.
func resolveJSPath(base string) string {
	if f := existingFile(base); f != "" {
		return f
	}
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	var sources []string
	switch filepath.Ext(base) {
	case ".js":
		sources = []string{".ts", ".tsx", ".d.ts"}
	case ".jsx":
		sources = []string{".tsx"}
	case ".mjs":
		sources = []string{".mts"}
	case ".cjs":
		sources = []string{".cts"}
	}
	for _, ext := range sources {
		if f := existingFile(stem + ext); f != "" {
			return f
		}
	}
	for _, ext := range jsExtensions {
		if f := existingFile(base + ext); f != "" {
			return f
		}
	}
	for _, ext := range jsExtensions {
		if f := existingFile(filepath.Join(base, "index"+ext)); f != "" {
			return f
		}
	}
	return ""
}

// match returns the paths a non-relative specifier maps to: the targets of
// the most specific compilerOptions.paths pattern (an exact pattern first,
// then the longest prefix before "*"), then the specifier under baseUrl.
func (c *tsConfig) match(spec string) []string {
	base := c.baseURL
	if base == "" {
		base = c.pathsDir
	}
	best, star, bestLen := "", "", -1
	if _, ok := c.paths[spec]; ok {
		best, bestLen = spec, len(spec)
	} else {
		for pattern := range c.paths {
			i := strings.Index(pattern, "*")
			if i < 0 {
				continue
			}
			prefix, suffix := pattern[:i], pattern[i+1:]
			if len(spec) < len(prefix)+len(suffix) || !strings.HasPrefix(spec, prefix) || !strings.HasSuffix(spec, suffix) {
				continue
			}
			if len(prefix) > bestLen || (len(prefix) == bestLen && pattern < best) {
				best, star, bestLen = pattern, spec[len(prefix):len(spec)-len(suffix)], len(prefix)
			}
		}
	}
	var targets []string
	if bestLen >= 0 {
		for _, target := range c.paths[best] {
			targets = append(targets, filepath.Join(base, filepath.FromSlash(strings.Replace(target, "*", star, 1))))
		}
	}
	if c.baseURL != "" {
		targets = append(targets, filepath.Join(c.baseURL, filepath.FromSlash(spec)))
	}
	return targets
}

// tsConfigFor returns the effective paths of the nearest tsconfig.json or
// jsconfig.json at or above dir, or nil.
func (da *DependencyAnalyzer) tsConfigFor(dir string) *tsConfig {
	path := ""
	for d := dir; ; {
		if f := existingFile(filepath.Join(d, "tsconfig.json")); f != "" {
			path = f
			break
		}
		if f := existingFile(filepath.Join(d, "jsconfig.json")); f != "" {
			path = f
			break
		}
		parent := filepath.Dir(d)
		if parent == d || exists(filepath.Join(d, ".git")) {
			break
		}
		d = parent
	}
	if path == "" {
		return nil
	}
	if cfg, ok := da.tsConfigs[path]; ok {
		return cfg
	}
	cfg := loadTSConfig(path, map[string]bool{})
	if cfg != nil && len(cfg.paths) == 0 && cfg.baseURL == "" {
		cfg = nil
	}
	da.tsConfigs[path] = cfg
	return cfg
}

// loadTSConfig reads a tsconfig with its relative "extends" chain. When it
// sets neither baseUrl nor paths, the first project in "references" that
// does (a Vite tsconfig.app.json) is used instead.
func loadTSConfig(path string, seen map[string]bool) *tsConfig {
	if seen[path] || len(seen) > 16 {
		return nil
	}
	seen[path] = true
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var raw struct {
		Extends         json.RawMessage `json:"extends"`
		CompilerOptions struct {
			BaseURL *string             `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
		References []struct {
			Path string `json:"path"`
		} `json:"references"`
	}
	if err := json.Unmarshal(stripJSONC(content), &raw); err != nil {
		return nil
	}
	dir := filepath.Dir(path)

	cfg := &tsConfig{}
	var extends []string
	var one string
	if json.Unmarshal(raw.Extends, &one) == nil {
		extends = []string{one}
	} else {
		_ = json.Unmarshal(raw.Extends, &extends)
	}
	for _, ext := range extends {
		if !strings.HasPrefix(ext, ".") {
			continue // a package such as @tsconfig/node20
		}
		basePath := filepath.Join(dir, filepath.FromSlash(ext))
		if existingFile(basePath) == "" && !strings.HasSuffix(basePath, ".json") {
			basePath += ".json"
		}
		if base := loadTSConfig(basePath, seen); base != nil {
			*cfg = *base
		}
	}
	if raw.CompilerOptions.BaseURL != nil {
		cfg.baseURL = filepath.Join(dir, filepath.FromSlash(*raw.CompilerOptions.BaseURL))
	}
	if raw.CompilerOptions.Paths != nil {
		cfg.paths = raw.CompilerOptions.Paths
		cfg.pathsDir = dir
	}
	if len(cfg.paths) == 0 && cfg.baseURL == "" {
		for _, ref := range raw.References {
			refPath := filepath.Join(dir, filepath.FromSlash(ref.Path))
			if info, err := os.Stat(refPath); err == nil && info.IsDir() {
				refPath = filepath.Join(refPath, "tsconfig.json")
			}
			if r := loadTSConfig(refPath, seen); r != nil && (len(r.paths) > 0 || r.baseURL != "") {
				return r
			}
		}
	}
	return cfg
}

// stripJSONC turns tsconfig's JSON with comments and trailing commas into
// JSON: it drops // and /* */ comments, then a comma before } or ], both
// outside strings.
func stripJSONC(src []byte) []byte {
	return scanJSON(scanJSON(src, true), false)
}

// scanJSON copies src outside strings through one rule: comments dropped
// (comments true) or trailing commas dropped (comments false).
func scanJSON(src []byte, comments bool) []byte {
	out := make([]byte, 0, len(src))
	inString := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if inString {
			out = append(out, c)
			if c == '\\' && i+1 < len(src) {
				i++
				out = append(out, src[i])
			} else if c == '"' {
				inString = false
			}
			continue
		}
		switch {
		case c == '"':
			inString = true
			out = append(out, c)
		case comments && c == '/' && i+1 < len(src) && src[i+1] == '/':
			for i+1 < len(src) && src[i+1] != '\n' {
				i++
			}
		case comments && c == '/' && i+1 < len(src) && src[i+1] == '*':
			i += 2
			for i+1 < len(src) && (src[i] != '*' || src[i+1] != '/') {
				i++
			}
			i++
		case !comments && c == ',':
			j := i + 1
			for j < len(src) && (src[j] == ' ' || src[j] == '\t' || src[j] == '\n' || src[j] == '\r') {
				j++
			}
			if j < len(src) && (src[j] == '}' || src[j] == ']') {
				continue // a trailing comma
			}
			out = append(out, c)
		default:
			out = append(out, c)
		}
	}
	return out
}
