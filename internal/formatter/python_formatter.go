package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// PythonFormatter is a language-specific formatter for Python
type PythonFormatter struct {
	BaseLanguageFormatter
	inClass bool
}

// NewPythonFormatter creates a new Python formatter
func NewPythonFormatter() *PythonFormatter {
	return &PythonFormatter{}
}

// FormatNode formats a Python node
func (f *PythonFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	// DEBUG
	// fmt.Fprintf(os.Stderr, "DEBUG: PythonFormatter.FormatNode called with node type %T\n", node)
	switch n := node.(type) {
	case *ir.DistilledComment:
		return f.formatComment(w, n, indent)
	case *ir.DistilledImport:
		return f.formatImport(w, n)
	case *ir.DistilledField:
		return f.formatField(w, n, indent)
	case *ir.DistilledFunction:
		return f.formatFunction(w, n, indent)
	case *ir.DistilledClass:
		return f.formatClass(w, n, indent)
	default:
		// Skip unknown nodes
		reportUnhandled("python", node)
		return nil
	}
}

func (f *PythonFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent int) error {
	text := comment.Text
	indentStr := strings.Repeat("    ", indent)

	// Special handling for module-level docstrings
	if indent == 0 && comment.Format == "docstring" {
		// Module docstrings should be wrapped in triple quotes
		if !strings.HasPrefix(text, `"""`) && !strings.HasPrefix(text, "'''") {
			fmt.Fprintf(w, `"""%s"""`, text)
			fmt.Fprintln(w)
		} else {
			fmt.Fprintln(w, text)
		}
		return nil
	}

	// If it's a // comment from the generic parser
	if strings.HasPrefix(text, "//") {
		text = strings.TrimPrefix(text, "//")
		text = strings.TrimSpace(text)
	}

	// For non-module level comments, format as single-line comments
	if !strings.HasPrefix(text, "#") {
		fmt.Fprintf(w, "%s# %s\n", indentStr, text)
	} else {
		fmt.Fprintf(w, "%s%s\n", indentStr, text)
	}

	return nil
}

func (f *PythonFormatter) formatImport(w io.Writer, imp *ir.DistilledImport) error {
	if imp.ImportType == "from" {
		if len(imp.Symbols) > 0 {
			symbols := make([]string, len(imp.Symbols))
			for i, sym := range imp.Symbols {
				if sym.Alias != "" {
					symbols[i] = fmt.Sprintf("%s as %s", sym.Name, sym.Alias)
				} else {
					symbols[i] = sym.Name
				}
			}
			fmt.Fprintf(w, "from %s import %s\n", imp.Module, strings.Join(symbols, ", "))
		} else {
			fmt.Fprintf(w, "from %s import *\n", imp.Module)
		}
	} else {
		// Simple import
		if len(imp.Symbols) > 0 && imp.Symbols[0].Alias != "" {
			fmt.Fprintf(w, "import %s as %s\n", imp.Module, imp.Symbols[0].Alias)
		} else {
			fmt.Fprintf(w, "import %s\n", imp.Module)
		}
	}
	return nil
}

func (f *PythonFormatter) formatField(w io.Writer, field *ir.DistilledField, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Get visibility prefix
	visPrefix := getVisibilityPrefix(field.Visibility)

	// Format field declaration
	fmt.Fprintf(w, "%s%s%s", indentStr, visPrefix, field.Name)

	// Add type annotation if present
	if field.Type != nil && field.Type.Name != "" {
		fmt.Fprintf(w, ": %s", field.Type.Name)
	}

	// Add default value if present
	if field.DefaultValue != "" {
		fmt.Fprintf(w, " = %s", field.DefaultValue)
	}

	fmt.Fprintln(w)
	return nil
}

func (f *PythonFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Format decorators
	for _, dec := range fn.Decorators {
		fmt.Fprintf(w, "%s@%s\n", indentStr, dec)
	}

	// Get visibility prefix
	visPrefix := getVisibilityPrefix(fn.Visibility)
	// DEBUG: Print visibility info
	// fmt.Fprintf(os.Stderr, "DEBUG: Function %s, Visibility=%v, Prefix=%q\n", fn.Name, fn.Visibility, visPrefix)

	// Check for special modifiers
	modifiers := ""
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			modifiers = "async "
		}
	}

	// Format function signature
	if fn.Implementation != "" {
		// Full function with implementation
		fmt.Fprintf(w, "%s%s%sdef %s(", indentStr, visPrefix, modifiers, fn.Name)
	} else {
		// Just signature without def keyword
		fmt.Fprintf(w, "%s%s%s%s(", indentStr, visPrefix, modifiers, fn.Name)
	}

	// Format parameters
	params := make([]string, 0, len(fn.Parameters))
	for _, param := range fn.Parameters {
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
	if fn.Returns != nil && fn.Returns.Name != "" {
		fmt.Fprintf(w, " -> %s", fn.Returns.Name)
	}

	// Handle implementation
	if fn.Implementation != "" {
		fmt.Fprintln(w, ":")

		// Write implementation with proper indentation
		for _, line := range dedentBody(fn.Implementation, indentStr+"    ") {
			fmt.Fprintln(w, line)
		}
	} else {
		// No implementation - just signature
		fmt.Fprintln(w)
	}

	return nil
}

func (f *PythonFormatter) formatClass(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Add blank line before class
	if !f.inClass && len(indentStr) == 0 {
		fmt.Fprintln(w)
	}

	// Format decorators
	for _, dec := range class.Decorators {
		fmt.Fprintf(w, "%s@%s\n", indentStr, dec)
	}

	// Format class declaration
	fmt.Fprintf(w, "%sclass %s", indentStr, class.Name)

	// Add base classes if any
	if len(class.Extends) > 0 {
		bases := make([]string, len(class.Extends))
		for i, base := range class.Extends {
			bases[i] = base.Name
		}
		fmt.Fprintf(w, "(%s)", strings.Join(bases, ", "))
	}

	fmt.Fprintln(w, ":")

	// Format class body
	if len(class.Children) > 0 {
		wasInClass := f.inClass
		f.inClass = true

		// Group children by type for better formatting
		var comments, fields, functions, classes []ir.DistilledNode

		for _, child := range class.Children {
			switch child.(type) {
			case *ir.DistilledComment:
				comments = append(comments, child)
			case *ir.DistilledField:
				fields = append(fields, child)
			case *ir.DistilledFunction:
				functions = append(functions, child)
			case *ir.DistilledClass:
				classes = append(classes, child)
			}
		}

		// Write all comments (docstrings and regular comments)
		for _, comment := range comments {
			f.FormatNode(w, comment, indent+1)
		}

		// Write fields
		for _, field := range fields {
			f.FormatNode(w, field, indent+1)
		}

		// Write functions
		for _, function := range functions {
			f.FormatNode(w, function, indent+1)
		}

		// Write nested classes
		for _, nestedClass := range classes {
			f.FormatNode(w, nestedClass, indent+1)
		}

		f.inClass = wasInClass
	} else {
		fmt.Fprintf(w, "%s    pass\n", indentStr)
	}

	return nil
}
