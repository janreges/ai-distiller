package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	typescript "tree-sitter-typescript"
)

// tsConfig is what --dependency-aware reads from a tsconfig.json or
// jsconfig.json: compilerOptions.baseUrl and compilerOptions.paths, with
// relative "extends" applied.
type tsConfig struct {
	baseURL  string              // absolute baseUrl, "" when unset
	paths    map[string][]string // compilerOptions.paths
	pathsDir string              // the directory of the config that declares paths
}

// jsExtensions are the extensions tried for an import without one, in
// TypeScript's order of preference.
var jsExtensions = []string{".ts", ".tsx", ".d.ts", ".js", ".jsx", ".mts", ".cts", ".mjs", ".cjs"}

// extractJavaScriptImports returns the files a JavaScript or TypeScript file
// imports, in source order: the module of every "import ... from", "export
// ... from", side-effect import, "import x = require()", dynamic import()
// and require() with a string literal that is a relative path or matches
// compilerOptions.paths (or baseUrl) of the nearest tsconfig.json or
// jsconfig.json. The specifiers come from the tree-sitter tree, so text in
// comments, strings and template literals never counts. Packages from
// node_modules and Node built-ins are not followed.
func (da *DependencyAnalyzer) extractJavaScriptImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	specs, err := jsImportSpecifiers(ctx, fileContent, filePath)
	if err != nil {
		return nil, err
	}
	var deps []string
	for _, spec := range specs {
		if f := da.resolveJSModule(spec, filePath); f != "" {
			deps = append(deps, f)
			dbg.Logf(debug.LevelDetailed, "JS import %s -> %s", spec, f)
		}
	}
	return deps, nil
}

// jsLanguage returns the tree-sitter grammar for a JavaScript or TypeScript
// file name: TSX for .tsx, TypeScript for .ts, .mts and .cts, else
// JavaScript (with JSX).
func jsLanguage(filePath string) (*sitter.Language, error) {
	var lang unsafe.Pointer
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".tsx":
		lang = typescript.LanguageTSX()
	case ".ts", ".mts", ".cts":
		lang = typescript.Language()
	default:
		lang = tree_sitter_javascript.Language()
	}
	if lang == nil {
		return nil, fmt.Errorf("no tree-sitter grammar for %s (cgo disabled)", filePath)
	}
	return sitter.NewLanguage(lang), nil
}

// jsImportSpecifiers returns the module specifiers of a JavaScript or
// TypeScript file in source order, read from its tree-sitter tree: the
// source string of import and export declarations and of "import x =
// require()", and the string argument of import() and require() calls.
func jsImportSpecifiers(ctx context.Context, content []byte, filePath string) ([]string, error) {
	lang, err := jsLanguage(filePath)
	if err != nil {
		return nil, err
	}
	parser, err := tsparse.NewParser(lang)
	if err != nil {
		return nil, err
	}
	defer parser.Close()
	tree, err := tsparse.Parse(ctx, parser, content)
	if err != nil {
		return nil, err
	}
	defer tree.Close()

	var specs []string
	add := func(str *sitter.Node) {
		if str == nil || str.Kind() != "string" {
			return // a template literal or an expression: not a static specifier
		}
		if text := str.Utf8Text(content); len(text) >= 2 {
			specs = append(specs, text[1:len(text)-1])
		}
	}
	var visit func(n *sitter.Node)
	visit = func(n *sitter.Node) {
		switch n.Kind() {
		case "import_statement", "export_statement", "import_require_clause":
			add(n.ChildByFieldName("source"))
		case "call_expression":
			fn := n.ChildByFieldName("function")
			if fn != nil && (fn.Kind() == "import" || fn.Kind() == "identifier" && fn.Utf8Text(content) == "require") {
				if args := n.ChildByFieldName("arguments"); args != nil && args.NamedChildCount() > 0 {
					add(args.NamedChild(0))
				}
			}
		}
		for i := uint(0); i < n.ChildCount(); i++ {
			visit(n.Child(i))
		}
	}
	visit(tree.RootNode())
	return specs, nil
}

// resolveJSModule returns the file an import specifier names, or "".
func (da *DependencyAnalyzer) resolveJSModule(spec, fromFile string) string {
	if i := strings.IndexAny(spec, "?#"); i >= 0 {
		spec = spec[:i] // "./icon.svg?react", "#internal"
	}
	if spec == "" {
		return ""
	}
	// "./dir/", "." and ".." name a directory, never a file next to it.
	dirOnly := spec == "." || spec == ".." || strings.HasSuffix(spec, "/") ||
		strings.HasSuffix(spec, "/.") || strings.HasSuffix(spec, "/..")
	if spec == "." || spec == ".." || strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		return da.resolveJSPath(filepath.Join(filepath.Dir(fromFile), filepath.FromSlash(spec)), dirOnly)
	}
	if strings.HasPrefix(spec, "/") || strings.Contains(spec, ":") {
		return "" // a URL path, "node:fs", "https://..."
	}
	cfg := da.tsConfigFor(filepath.Dir(fromFile))
	if cfg == nil {
		return ""
	}
	for _, target := range cfg.match(spec) {
		if f := da.resolveJSPath(target, dirOnly); f != "" {
			return f
		}
	}
	return ""
}

// resolveJSPath finds the file for an import path the way TypeScript and
// Node do: as a file (resolveJSFile), then as a directory (resolveJSDir).
// With dirOnly (the specifier ends in "/" or is "." or ".."), only as a
// directory, so "./dir/" never finds dir.ts.
func (da *DependencyAnalyzer) resolveJSPath(base string, dirOnly bool) string {
	if !dirOnly {
		if f := resolveJSFile(base); f != "" {
			return f
		}
	}
	return da.resolveJSDir(base, 0)
}

// resolveJSDir finds the file a directory import names: the file that
// "main" of the directory's package.json names (resolved like an import
// path), else its index file. depth bounds a chain of package.json files
// whose main names another directory.
func (da *DependencyAnalyzer) resolveJSDir(dir string, depth int) string {
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return ""
	}
	if main := da.packageMain(dir); main != "" && depth < 4 {
		target := filepath.Join(dir, filepath.FromSlash(main))
		if f := resolveJSFile(target); f != "" {
			return f
		}
		if f := da.resolveJSDir(target, depth+1); f != "" {
			return f
		}
	}
	for _, ext := range jsExtensions {
		if f := existingFile(filepath.Join(dir, "index"+ext)); f != "" {
			return f
		}
	}
	return ""
}

// resolveJSFile finds the file for an import path without its extension
// resolution: the path itself, the TypeScript source of a ".js" path (ESM
// imports name the compiled file), then the path with each extension.
func resolveJSFile(base string) string {
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
	return ""
}

// packageMain returns the "main" field of dir/package.json, or "" when there
// is none, it cannot be read or it lies outside the scope's boundary.
func (da *DependencyAnalyzer) packageMain(dir string) string {
	manifest := filepath.Join(dir, "package.json")
	if existingFile(manifest) == "" || !da.scope.contains(manifest) {
		return ""
	}
	content, err := os.ReadFile(manifest)
	if err != nil {
		return ""
	}
	var pkg struct {
		Main string `json:"main"`
	}
	if json.Unmarshal(content, &pkg) != nil {
		return ""
	}
	return pkg.Main
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
	for d := dir; da.scope.contains(d); {
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
	cfg := da.loadTSConfig(path, map[string]bool{})
	if cfg != nil && len(cfg.paths) == 0 && cfg.baseURL == "" {
		cfg = nil
	}
	da.tsConfigs[path] = cfg
	return cfg
}

// loadTSConfig reads a tsconfig with its relative "extends" chain. When it
// sets neither baseUrl nor paths, the first project in "references" that
// does (a Vite tsconfig.app.json) is used instead. A config outside the
// scope's boundary is not read.
func (da *DependencyAnalyzer) loadTSConfig(path string, seen map[string]bool) *tsConfig {
	if seen[path] || len(seen) > 16 || !da.scope.contains(path) {
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
		if base := da.loadTSConfig(basePath, seen); base != nil {
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
			if r := da.loadTSConfig(refPath, seen); r != nil && (len(r.paths) > 0 || r.baseURL != "") {
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
