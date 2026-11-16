package formatter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/janreges/ai-distiller/internal/importfilter"
	"github.com/janreges/ai-distiller/internal/ir"
)

// LanguageAwareTextFormatter is a text formatter that uses language-specific formatters
type LanguageAwareTextFormatter struct {
	BaseFormatter
	formatters map[string]LanguageFormatter
	mu         sync.RWMutex
}

// NewLanguageAwareTextFormatter creates a new language-aware text formatter
func NewLanguageAwareTextFormatter(options Options) *LanguageAwareTextFormatter {
	f := &LanguageAwareTextFormatter{
		BaseFormatter: NewBaseFormatter(options),
		formatters:    make(map[string]LanguageFormatter),
	}

	// Register built-in language formatters
	f.RegisterLanguageFormatter("java", NewJavaFormatter())
	f.RegisterLanguageFormatter("go", NewGoFormatter())
	f.RegisterLanguageFormatter("typescript", NewTypeScriptFormatter())
	f.RegisterLanguageFormatter("python", NewPythonFormatter())
	f.RegisterLanguageFormatter("javascript", NewJavaScriptFormatter())
	f.RegisterLanguageFormatter("swift", NewSwiftFormatter())
	f.RegisterLanguageFormatter("ruby", NewRubyFormatter())
	f.RegisterLanguageFormatter("rust", NewRustFormatter())
	f.RegisterLanguageFormatter("csharp", NewCSharpFormatter())
	f.RegisterLanguageFormatter("c#", NewCSharpFormatter()) // Alias
	f.RegisterLanguageFormatter("kotlin", NewKotlinFormatter())
	f.RegisterLanguageFormatter("cpp", NewCppFormatter())
	f.RegisterLanguageFormatter("c++", NewCppFormatter()) // Alias
	f.RegisterLanguageFormatter("php", NewPHPFormatter())

	return f
}

// RegisterLanguageFormatter registers a language-specific formatter
func (f *LanguageAwareTextFormatter) RegisterLanguageFormatter(language string, formatter LanguageFormatter) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.formatters[language] = formatter
}

// Format implements formatter.Formatter
func (f *LanguageAwareTextFormatter) Format(w io.Writer, file *ir.DistilledFile) error {
	// Check if we should apply import filtering
	if f.options.ProcessingOptions.FilterImports && file.Language != "" {
		// Use buffered approach for filtering
		return f.formatWithFiltering(w, file)
	}

	// Direct formatting without filtering
	return f.formatDirect(w, file)
}

// formatDirect writes directly to the writer without filtering
func (f *LanguageAwareTextFormatter) formatDirect(w io.Writer, file *ir.DistilledFile) error {
	// Write file header
	fmt.Fprintf(w, "<file path=\"%s\">\n", escapeFilePath(file.Path))

	// Get language-specific formatter
	langFormatter := f.getLanguageFormatter(file.Language)

	// Reset formatter state for new file
	if langFormatter != nil {
		langFormatter.Reset()
	}

	// Write file contents
	if siblings, ok := langFormatter.(SiblingFormatter); ok {
		if err := siblings.FormatNodes(w, file.Children, 0); err != nil {
			return err
		}
	} else {
		for _, child := range file.Children {
			if langFormatter != nil {
				if err := langFormatter.FormatNode(w, child, 0); err != nil {
					return err
				}
			} else {
				// Fallback to generic formatting
				if err := f.formatNodeGeneric(w, child, 0); err != nil {
					return err
				}
			}
		}
	}

	// Write file footer
	fmt.Fprintln(w, "</file>")

	return nil
}

// formatWithFiltering buffers output and applies import filtering
func (f *LanguageAwareTextFormatter) formatWithFiltering(w io.Writer, file *ir.DistilledFile) error {
	var buf bytes.Buffer

	// Format to buffer
	if err := f.formatDirect(&buf, file); err != nil {
		return err
	}

	// Apply import filtering
	content := buf.String()
	src := importfilter.Source{Path: file.Path}
	if ext := file.Extensions; ext != nil {
		src.HasJSX = (ext.JavaScript != nil && ext.JavaScript.HasJSX) ||
			(ext.TypeScript != nil && ext.TypeScript.HasJSX)
	}
	filteredContent, removedImports, err := importfilter.FilterImportsOf(content, file.Language, f.options.DebugLevel, src)
	if err != nil {
		// On error, use original content
		fmt.Fprint(w, content)
		return nil
	}

	// Log removed imports in debug mode
	if len(removedImports) > 0 && f.options.DebugLevel >= 3 {
		for _, imp := range removedImports {
			fmt.Fprintf(os.Stderr, "[import filter] Removed unused import: %s\n", imp)
		}
	}

	// Write filtered content
	fmt.Fprint(w, filteredContent)
	return nil
}

// FormatMultiple implements formatter.Formatter
func (f *LanguageAwareTextFormatter) FormatMultiple(w io.Writer, files []*ir.DistilledFile) error {
	for i, file := range files {
		if err := f.Format(w, file); err != nil {
			return err
		}
		if i < len(files)-1 {
			fmt.Fprintln(w) // Add blank line between files
		}
	}
	return nil
}

// Extension implements formatter.Formatter
func (f *LanguageAwareTextFormatter) Extension() string {
	return "txt"
}

// FormatError implements formatter.Formatter
func (f *LanguageAwareTextFormatter) FormatError(w io.Writer, err error) error {
	fmt.Fprintf(w, "ERROR: %v\n", err)
	return nil
}

func (f *LanguageAwareTextFormatter) getLanguageFormatter(language string) LanguageFormatter {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.formatters[language]
}

// formatNodeGeneric provides a generic fallback for unsupported languages
func (f *LanguageAwareTextFormatter) formatNodeGeneric(w io.Writer, node ir.DistilledNode, indent int) error {
	// This is a simplified generic formatter
	// In a real implementation, this could be more sophisticated

	switch n := node.(type) {
	case *ir.DistilledImport:
		fmt.Fprintf(w, "import %s\n", n.Module)
	case *ir.DistilledExport:
		fmt.Fprintln(w, esExportLine(n))
	case *ir.DistilledClass:
		// Format class with modifiers and extends
		modifiers := ""
		for _, mod := range n.Modifiers {
			if mod == ir.ModifierAbstract {
				modifiers += "abstract "
			}
		}
		fmt.Fprintf(w, "\n%sclass %s", modifiers, n.Name)

		// Add generic type parameters
		if len(n.TypeParams) > 0 {
			typeParams := make([]string, len(n.TypeParams))
			for i, param := range n.TypeParams {
				typeParams[i] = param.Name
				if len(param.Constraints) > 0 {
					typeParams[i] += " extends " + param.Constraints[0].Name
				}
			}
			fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
		}

		// Add extends clause
		if len(n.Extends) > 0 {
			extends := make([]string, len(n.Extends))
			for i, ext := range n.Extends {
				extends[i] = ext.Name
			}
			fmt.Fprintf(w, " extends %s", strings.Join(extends, ", "))
		}

		// Add implements clause
		if len(n.Implements) > 0 {
			implements := make([]string, len(n.Implements))
			for i, impl := range n.Implements {
				implements[i] = impl.Name
			}
			fmt.Fprintf(w, " implements %s", strings.Join(implements, ", "))
		}

		fmt.Fprintln(w, ":")
		for _, child := range n.Children {
			f.formatNodeGeneric(w, child, indent+1)
		}
	case *ir.DistilledFunction:
		// Format function with modifiers and parameters
		visPrefix := ""
		switch n.Visibility {
		case ir.VisibilityPrivate:
			visPrefix = "private "
		case ir.VisibilityProtected:
			visPrefix = "protected "
		case ir.VisibilityPublic:
			// Don't print "public" as it's the default
		}

		modifiers := ""
		for _, mod := range n.Modifiers {
			if mod == ir.ModifierAbstract {
				modifiers += "abstract "
			} else if mod == ir.ModifierAsync {
				modifiers += "async "
			} else if mod == ir.ModifierStatic {
				modifiers += "static "
			}
		}
		fmt.Fprintf(w, "    %s%sfunction %s", visPrefix, modifiers, n.Name)

		// Add generic type parameters
		if len(n.TypeParams) > 0 {
			typeParams := make([]string, len(n.TypeParams))
			for i, param := range n.TypeParams {
				typeParams[i] = param.Name
				if len(param.Constraints) > 0 {
					typeParams[i] += " extends " + param.Constraints[0].Name
				}
			}
			fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
		}

		fmt.Fprintf(w, "(")

		// Format parameters
		params := make([]string, 0, len(n.Parameters))
		for _, param := range n.Parameters {
			if param.Name == "" {
				continue
			}
			paramStr := param.Name
			if param.Type.Name != "" {
				paramStr += ": " + param.Type.Name
			}
			if param.DefaultValue != "" {
				paramStr += " = " + param.DefaultValue
			}
			params = append(params, paramStr)
		}
		fmt.Fprintf(w, "%s)", strings.Join(params, ", "))

		// Format return type
		if n.Returns != nil && n.Returns.Name != "" {
			fmt.Fprintf(w, " -> %s", n.Returns.Name)
		}

		fmt.Fprintln(w)

		if n.Implementation != "" {
			fmt.Fprintln(w, "        // implementation")
		}
	case *ir.DistilledField:
		// Format field with visibility and type
		visPrefix := ""
		switch n.Visibility {
		case ir.VisibilityPrivate:
			visPrefix = "private "
		case ir.VisibilityProtected:
			visPrefix = "protected "
		case ir.VisibilityPublic:
			visPrefix = "public "
		}

		modifiers := ""
		for _, mod := range n.Modifiers {
			if mod == ir.ModifierReadonly {
				modifiers += "readonly "
			} else if mod == ir.ModifierStatic {
				modifiers += "static "
			} else if mod == ir.ModifierFinal {
				modifiers += "const "
			}
		}

		// Top-level const variables should be shown differently from class fields
		if indent == 0 && strings.Contains(modifiers, "const") {
			// This is a top-level const variable
			fmt.Fprintf(w, "%s%s", modifiers, n.Name)
		} else {
			// Regular field inside a class/interface
			fmt.Fprintf(w, "    field %s%s%s", visPrefix, modifiers, n.Name)
		}
		if n.Type != nil && n.Type.Name != "" {
			fmt.Fprintf(w, ": %s", n.Type.Name)
		}
		if n.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", n.DefaultValue)
		}
		fmt.Fprintln(w)
	case *ir.DistilledComment:
		fmt.Fprintf(w, "// %s\n", n.Text)
	case *ir.DistilledTypeAlias:
		// Format type with generic parameters
		fmt.Fprintf(w, "type %s", n.Name)
		if len(n.TypeParams) > 0 {
			typeParams := make([]string, len(n.TypeParams))
			for i, param := range n.TypeParams {
				typeParams[i] = param.Name
				if len(param.Constraints) > 0 {
					typeParams[i] += " extends " + param.Constraints[0].Name
				}
			}
			fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
		}
		fmt.Fprintf(w, " = %s\n", n.Type.Name)
	case *ir.DistilledInterface:
		fmt.Fprintf(w, "\ninterface %s", n.Name)
		if len(n.TypeParams) > 0 {
			typeParams := make([]string, len(n.TypeParams))
			for i, param := range n.TypeParams {
				typeParams[i] = param.Name
				if len(param.Constraints) > 0 {
					typeParams[i] += " extends " + param.Constraints[0].Name
				}
			}
			fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
		}
		if len(n.Extends) > 0 {
			extends := make([]string, len(n.Extends))
			for i, ext := range n.Extends {
				extends[i] = ext.Name
			}
			fmt.Fprintf(w, " extends %s", strings.Join(extends, ", "))
		}
		fmt.Fprintln(w, ":")
		for _, child := range n.Children {
			// For interfaces, format members as properties/methods, not fields
			switch child.(type) {
			case *ir.DistilledField:
				field := child.(*ir.DistilledField)
				fmt.Fprintf(w, "    property %s", field.Name)
				if field.Type != nil && field.Type.Name != "" {
					fmt.Fprintf(w, ": %s", field.Type.Name)
				}
				fmt.Fprintln(w)
			case *ir.DistilledFunction:
				// Format as method for interfaces
				method := child.(*ir.DistilledFunction)
				fmt.Fprintf(w, "    method %s", method.Name)

				// Add generic type parameters if any
				if len(method.TypeParams) > 0 {
					typeParams := make([]string, len(method.TypeParams))
					for i, param := range method.TypeParams {
						typeParams[i] = param.Name
						if len(param.Constraints) > 0 {
							typeParams[i] += " extends " + param.Constraints[0].Name
						}
					}
					fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
				}

				fmt.Fprintf(w, "(")
				params := make([]string, 0, len(method.Parameters))
				for _, param := range method.Parameters {
					if param.Name == "" {
						continue
					}
					paramStr := param.Name
					if param.Type.Name != "" {
						paramStr += ": " + param.Type.Name
					}
					params = append(params, paramStr)
				}
				fmt.Fprintf(w, "%s)", strings.Join(params, ", "))

				if method.Returns != nil && method.Returns.Name != "" {
					fmt.Fprintf(w, ": %s", method.Returns.Name)
				}
				fmt.Fprintln(w)
			default:
				f.formatNodeGeneric(w, child, indent+1)
			}
		}
	case *ir.DistilledRawContent:
		// Output raw content as-is
		fmt.Fprint(w, n.Content)
		if len(n.Content) > 0 && n.Content[len(n.Content)-1] != '\n' {
			fmt.Fprintln(w)
		}
	default:
		// Skip unknown nodes
		reportUnhandled("generic", node)
	}

	return nil
}

var (
	filePathEscaper   = strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;")
	filePathUnescaper = strings.NewReplacer("&quot;", `"`, "&lt;", "<", "&gt;", ">", "&amp;", "&")
)

// escapeFilePath escapes a path for the path attribute of a <file> tag, so a
// path with a quote or angle bracket cannot break the tag. Ordinary paths are
// unchanged.
func escapeFilePath(p string) string { return filePathEscaper.Replace(p) }

// unescapeFilePath reverses escapeFilePath.
func unescapeFilePath(p string) string { return filePathUnescaper.Replace(p) }
