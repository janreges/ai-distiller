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

// psr4Root maps a namespace prefix to its directories, from the
// autoload.psr-4 and autoload-dev.psr-4 sections of composer.json.
type psr4Root struct {
	prefix string
	dirs   []string
}

var (
	phpNamespace = regexp.MustCompile(`(?m)^[ \t]*namespace\s+([A-Za-z_\\][\w\\]*)\s*[;{]`)
	phpUse       = regexp.MustCompile(`(?m)^([ \t]*)use\s+([^;]+);`)
	phpHeritage  = regexp.MustCompile(`\b(?:extends|implements)\s+(\\?[A-Za-z_][\w\\]*(?:\s*,\s*\\?[A-Za-z_][\w\\]*)*)`)
	phpInclude   = regexp.MustCompile(`\b(?:require|include)(?:_once)?\s*\(?\s*(__DIR__\s*\.\s*)?['"]([^'"\n]+)['"]`)
	phpAlias     = regexp.MustCompile(`(?i)^(\S+)\s+as\s+(\w+)$`)
	phpComment   = regexp.MustCompile(`(?ms)^[ \t]*/\*.*?\*/`)
	phpName      = regexp.MustCompile(`^\\?[A-Za-z_\x80-\x{10FFFF}][\w\x80-\x{10FFFF}]*(?:\\[A-Za-z_\x80-\x{10FFFF}][\w\x80-\x{10FFFF}]*)*$`)
)

// extractPhpImports returns, in source order, the files of the classes a PHP
// file names in "use" imports (grouped ones too, not "use function" or "use
// const"), in trait "use" inside a class, and after extends and implements,
// mapped to files through the PSR-4 autoload sections of the nearest
// composer.json, plus the files of require/include with a literal path
// (relative to the file, also as __DIR__ . '/x.php'). A name without a
// leading backslash is resolved against the imports and then the file's
// namespace. Classes of installed packages (vendor/) are not followed.
func (da *DependencyAnalyzer) extractPhpImports(ctx context.Context, fileContent []byte, filePath string) ([]string, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("dependency")
	code := stripPhpComments(string(fileContent))
	dir := filepath.Dir(filePath)

	namespace := ""
	if m := phpNamespace.FindStringSubmatch(code); m != nil {
		namespace = strings.Trim(m[1], `\`)
	}

	type ref struct {
		pos   int
		names []string // class names to try, in order
		file  string   // a literal include path
	}
	var refs []ref
	aliases := map[string]string{}

	for _, m := range phpUse.FindAllStringSubmatchIndex(code, -1) {
		indent, body := code[m[2]:m[3]], strings.TrimSpace(code[m[4]:m[5]])
		if indent == "" {
			for _, fqn := range phpImportedClasses(body, aliases) {
				refs = append(refs, ref{pos: m[0], names: []string{fqn}})
			}
			continue
		}
		// An indented "use" is a trait use inside a class body.
		if i := strings.Index(body, "{"); i >= 0 {
			body = body[:i]
		}
		for _, name := range strings.Split(body, ",") {
			if name = strings.TrimSpace(name); isPhpName(name) {
				refs = append(refs, ref{pos: m[0], names: phpCandidates(name, namespace, aliases)})
			}
		}
	}
	for _, m := range phpHeritage.FindAllStringSubmatchIndex(code, -1) {
		for _, name := range strings.Split(code[m[2]:m[3]], ",") {
			if name = strings.TrimSpace(name); isPhpName(name) {
				refs = append(refs, ref{pos: m[0], names: phpCandidates(name, namespace, aliases)})
			}
		}
	}
	for _, m := range phpInclude.FindAllStringSubmatchIndex(code, -1) {
		refs = append(refs, ref{pos: m[0], file: filepath.Join(dir, filepath.FromSlash(code[m[4]:m[5]]))})
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].pos < refs[j].pos })

	roots := da.psr4RootsFor(dir)
	var deps []string
	for _, r := range refs {
		if r.file != "" {
			if f := existingFile(r.file); f != "" {
				deps = append(deps, f)
			}
			continue
		}
		for _, name := range r.names {
			if f := resolvePsr4(name, roots); f != "" {
				deps = append(deps, f)
				dbg.Logf(debug.LevelDetailed, "PHP class %s -> %s", name, f)
				break
			}
		}
	}
	return deps, nil
}

// phpImportedClasses returns the classes of a top-level "use" body and
// records their aliases: "A\B", "A\B as C", "A\B, C\D", "A\{B, C as D}".
// "use function" and "use const" import no class.
func phpImportedClasses(body string, aliases map[string]string) []string {
	lower := strings.ToLower(body)
	if strings.HasPrefix(lower, "function ") || strings.HasPrefix(lower, "const ") {
		return nil
	}
	prefix, items := "", body
	if open := strings.Index(body, "{"); open >= 0 {
		closing := strings.LastIndex(body, "}")
		if closing < open {
			return nil
		}
		prefix, items = strings.TrimSpace(body[:open]), body[open+1:closing]
	}
	var classes []string
	for _, item := range strings.Split(items, ",") {
		item = strings.TrimSpace(item)
		lowerItem := strings.ToLower(item)
		if item == "" || strings.HasPrefix(lowerItem, "function ") || strings.HasPrefix(lowerItem, "const ") {
			continue
		}
		name, alias := item, ""
		if m := phpAlias.FindStringSubmatch(item); m != nil {
			name, alias = m[1], m[2]
		}
		fqn := strings.Trim(prefix+name, `\`)
		if !isPhpName(fqn) {
			continue
		}
		if alias == "" {
			alias = fqn[strings.LastIndex(fqn, `\`)+1:]
		}
		aliases[strings.ToLower(alias)] = fqn
		classes = append(classes, fqn)
	}
	return classes
}

// phpCandidates returns the fully qualified names a class name in code can
// mean: "\A\B" as written; else with its first segment replaced by an
// import; else in the file's namespace, then as written.
func phpCandidates(name, namespace string, aliases map[string]string) []string {
	if strings.HasPrefix(name, `\`) {
		return []string{strings.TrimPrefix(name, `\`)}
	}
	first, rest := name, ""
	if i := strings.Index(name, `\`); i >= 0 {
		first, rest = name[:i], name[i:]
	}
	if fqn, ok := aliases[strings.ToLower(first)]; ok {
		return []string{fqn + rest}
	}
	if namespace != "" {
		return []string{namespace + `\` + name, name}
	}
	return []string{name}
}

// isPhpName reports whether s looks like a (qualified) class name.
func isPhpName(s string) bool {
	return phpName.MatchString(s)
}

// resolvePsr4 returns the file of a fully qualified class under the first
// PSR-4 root, in order, whose prefix it starts with and where the file
// exists.
func resolvePsr4(fqn string, roots []psr4Root) string {
	for _, root := range roots {
		if !strings.HasPrefix(fqn, root.prefix) {
			continue
		}
		rel := filepath.FromSlash(strings.ReplaceAll(fqn[len(root.prefix):], `\`, "/")) + ".php"
		for _, dir := range root.dirs {
			if f := existingFile(filepath.Join(dir, rel)); f != "" {
				return f
			}
		}
	}
	return ""
}

// psr4RootsFor returns the PSR-4 roots of every composer.json at or above
// dir up to the top of the repository, the nearest first: a monorepo
// package (laravel's src/Illuminate/Database) maps only its own namespace,
// the root composer.json maps the others. Each composer.json is read once.
func (da *DependencyAnalyzer) psr4RootsFor(dir string) []psr4Root {
	var roots []psr4Root
	for {
		composer := da.findUp(dir, "composer.json")
		if composer == "" {
			return roots
		}
		parsed, ok := da.composers[composer]
		if !ok {
			parsed = parseComposerPsr4(composer)
			da.composers[composer] = parsed
		}
		roots = append(roots, parsed...)
		dir = filepath.Dir(composer)
		parent := filepath.Dir(dir)
		if parent == dir || exists(filepath.Join(dir, ".git")) {
			return roots
		}
		dir = parent
	}
}

// parseComposerPsr4 reads autoload.psr-4 and autoload-dev.psr-4, whose
// values are a directory or a list of directories.
func parseComposerPsr4(composer string) []psr4Root {
	content, err := os.ReadFile(composer)
	if err != nil {
		return nil
	}
	var raw struct {
		Autoload struct {
			Psr4 map[string]json.RawMessage `json:"psr-4"`
		} `json:"autoload"`
		AutoloadDev struct {
			Psr4 map[string]json.RawMessage `json:"psr-4"`
		} `json:"autoload-dev"`
	}
	if json.Unmarshal(content, &raw) != nil {
		return nil
	}
	base := filepath.Dir(composer)
	var roots []psr4Root
	for _, section := range []map[string]json.RawMessage{raw.Autoload.Psr4, raw.AutoloadDev.Psr4} {
		for prefix, value := range section {
			var dirs []string
			var one string
			if json.Unmarshal(value, &one) == nil {
				dirs = []string{one}
			} else if json.Unmarshal(value, &dirs) != nil {
				continue
			}
			root := psr4Root{prefix: strings.TrimLeft(prefix, `\`)}
			for _, d := range dirs {
				root.dirs = append(root.dirs, filepath.Join(base, filepath.FromSlash(d)))
			}
			roots = append(roots, root)
		}
	}
	sort.SliceStable(roots, func(i, j int) bool {
		if len(roots[i].prefix) != len(roots[j].prefix) {
			return len(roots[i].prefix) > len(roots[j].prefix)
		}
		return roots[i].prefix < roots[j].prefix
	})
	return roots
}

// stripPhpComments blanks the docblocks and the //, # and /* */ comments
// that start a line, so names in documentation do not count.
func stripPhpComments(code string) string {
	code = phpComment.ReplaceAllString(code, "")
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "#[") {
			lines[i] = ""
		}
	}
	return strings.Join(lines, "\n")
}
