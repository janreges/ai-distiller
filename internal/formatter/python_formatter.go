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
		return f.formatFunction(w, n, indent, nil)
	case *ir.DistilledClass:
		return f.formatClass(w, n, indent)
	default:
		// Skip unknown nodes
		reportUnhandled("python", node)
		return nil
	}
}

// FormatNodes formats sibling nodes in order. The IR keeps a function's
// docstring as a doc comment right before the function; it is printed under
// the signature, where Python has it, so it cannot pass for the docstring of
// the enclosing class or module. Any other doc comment prints where it is.
func (f *PythonFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
	var docs []*ir.DistilledComment // doc comments waiting for the next node
	flush := func() error {
		for _, doc := range docs {
			if err := f.formatComment(w, doc, indent); err != nil {
				return err
			}
		}
		docs = nil
		return nil
	}
	for _, node := range nodes {
		switch n := node.(type) {
		case *ir.DistilledComment:
			if n.IsDoc() && !n.Inner {
				docs = append(docs, n)
				continue
			}
		case *ir.DistilledFunction:
			if err := f.formatFunction(w, n, indent, docs); err != nil {
				return err
			}
			docs = nil
			continue
		}
		if err := flush(); err != nil {
			return err
		}
		if err := f.FormatNode(w, node, indent); err != nil {
			return err
		}
	}
	return flush()
}

// formatComment writes a doc comment as a docstring and any other comment as
// "# " lines, all at the comment's indentation. The shebang line is written
// as it is in the source.
func (f *PythonFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent int) error {
	indentStr := strings.Repeat("    ", indent)
	lines := strings.Split(comment.Text, "\n")

	if isShebang(comment) {
		fmt.Fprintln(w, "#"+comment.Text)
		return nil
	}
	if !comment.IsDoc() {
		for _, line := range lines {
			fmt.Fprintln(w, strings.TrimRight(indentStr+"# "+line, " "))
		}
		return nil
	}

	if len(lines) == 1 {
		fmt.Fprintf(w, "%s\"\"\"%s\"\"\"\n", indentStr, lines[0])
		return nil
	}
	fmt.Fprintf(w, "%s\"\"\"%s\n", indentStr, lines[0])
	for _, line := range lines[1:] {
		if line == "" {
			fmt.Fprintln(w)
		} else {
			fmt.Fprintln(w, indentStr+line)
		}
	}
	fmt.Fprintf(w, "%s\"\"\"\n", indentStr)
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

// formatFunction writes fn; docs, the doc comments that document it, go under
// the signature, one level deeper, before the body.
func (f *PythonFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent int, docs []*ir.DistilledComment) error {
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
	hasBody := fn.Implementation != "" || fn.DocOnlyBody
	if hasBody {
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

	// End the signature line; only a function with a body gets the colon
	if hasBody {
		fmt.Fprintln(w, ":")
	} else {
		fmt.Fprintln(w)
	}

	for _, doc := range docs {
		if err := f.formatComment(w, doc, indent+1); err != nil {
			return err
		}
	}

	// Write implementation with proper indentation
	if fn.Implementation != "" {
		for _, line := range dedentBody(fn.Implementation, indentStr+"    ") {
			fmt.Fprintln(w, line)
		}
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

		// Group children by type for better formatting. A doc comment that
		// documents the next member (a method's docstring) stays with it, so
		// FormatNodes can print it under the method's signature; the class
		// docstring and other comments come first.
		var comments, fields, functions, classes, docs []ir.DistilledNode

		for _, child := range class.Children {
			switch c := child.(type) {
			case *ir.DistilledComment:
				if c.IsDoc() && !c.Inner {
					docs = append(docs, child)
					continue
				}
				comments = append(comments, child)
				continue
			case *ir.DistilledField:
				fields = append(append(fields, docs...), child)
			case *ir.DistilledFunction:
				functions = append(append(functions, docs...), child)
			case *ir.DistilledClass:
				classes = append(append(classes, docs...), child)
			default:
				continue
			}
			docs = nil
		}
		comments = append(comments, docs...)

		// Write the class docstring and the comments, then the fields, the
		// functions and the nested classes
		var err error
		for _, group := range [][]ir.DistilledNode{comments, fields, functions, classes} {
			if err = f.FormatNodes(w, group, indent+1); err != nil {
				break
			}
		}

		f.inClass = wasInClass
		if err != nil {
			return err
		}
	} else {
		fmt.Fprintf(w, "%s    pass\n", indentStr)
	}

	return nil
}
