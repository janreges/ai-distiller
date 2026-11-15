package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// RubyFormatter implements language-specific formatting for Ruby
type RubyFormatter struct {
	BaseLanguageFormatter
}

// NewRubyFormatter creates a new Ruby formatter
func NewRubyFormatter() LanguageFormatter {
	return &RubyFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("ruby"),
	}
}

// FormatNode implements LanguageFormatter
func (f *RubyFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	switch n := node.(type) {
	case *ir.DistilledImport:
		return f.formatImport(w, n, indentStr)
	case *ir.DistilledClass:
		return f.formatClass(w, n, indent)
	case *ir.DistilledInterface:
		return f.formatModule(w, n, indent)
	case *ir.DistilledFunction:
		return f.formatMethod(w, n, indentStr)
	case *ir.DistilledField:
		return f.formatField(w, n, indentStr)
	case *ir.DistilledComment:
		return f.formatComment(w, n, indentStr)
	default:
		// Skip unknown nodes
		reportUnhandled("ruby", node)
		return nil
	}
}

func (f *RubyFormatter) formatImport(w io.Writer, imp *ir.DistilledImport, indent string) error {
	// Ruby uses require, require_relative or load
	keyword := imp.ImportType
	if keyword != "require_relative" && keyword != "load" {
		keyword = "require"
	}
	fmt.Fprintf(w, "%s%s '%s'\n", indent, keyword, imp.Module)
	return nil
}

func (f *RubyFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent string) error {
	lines := strings.Split(comment.Text, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Fprintf(w, "%s# %s\n", indent, line)
		} else {
			fmt.Fprintf(w, "%s#\n", indent)
		}
	}
	return nil
}

func (f *RubyFormatter) formatClass(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Add blank line before class
	fmt.Fprintln(w)

	// Format class declaration
	fmt.Fprintf(w, "%sclass %s", indentStr, class.Name)

	// Add inheritance
	if len(class.Extends) > 0 {
		fmt.Fprintf(w, " < %s", class.Extends[0].Name)
	}

	fmt.Fprintln(w)

	// Included modules (the parser records `include X` as Implements)
	memberIndent := strings.Repeat("    ", indent+1)
	for _, mixin := range class.Implements {
		fmt.Fprintf(w, "%sinclude %s\n", memberIndent, mixin.Name)
	}

	// Format class members
	for _, child := range class.Children {
		f.FormatNode(w, child, indent+1)
	}

	// No 'end' in text format

	return nil
}

func (f *RubyFormatter) formatModule(w io.Writer, mod *ir.DistilledInterface, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Add blank line before module
	fmt.Fprintln(w)

	// Format module declaration
	fmt.Fprintf(w, "%smodule %s\n", indentStr, mod.Name)

	// Format module members
	for _, child := range mod.Children {
		f.FormatNode(w, child, indent+1)
	}

	// No 'end' in text format

	return nil
}

func (f *RubyFormatter) formatMethod(w io.Writer, fn *ir.DistilledFunction, indent string) error {
	// Ruby uses visibility method calls (private, protected, public)
	// For text format, we'll show visibility as a comment
	visComment := f.getRubyVisibilityComment(fn.Visibility)

	// Check for special method types
	isClassMethod := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierStatic {
			isClassMethod = true
			break
		}
	}

	// Add visibility comment if not public
	if visComment != "" {
		fmt.Fprintf(w, "%s# %s\n", indent, visComment)
	}

	// Format method declaration
	if isClassMethod {
		fmt.Fprintf(w, "%sdef self.%s", indent, fn.Name)
	} else {
		fmt.Fprintf(w, "%sdef %s", indent, fn.Name)
	}

	// Parameters
	if len(fn.Parameters) > 0 {
		fmt.Fprintf(w, "(")
		f.formatParameters(w, fn.Parameters)
		fmt.Fprintf(w, ")")
	}

	// Implementation
	if fn.Implementation != "" {
		fmt.Fprintln(w)
		for _, line := range dedentBody(fn.Implementation, indent+"    ") {
			fmt.Fprintln(w, line)
		}
		// No 'end' in text format
	} else {
		fmt.Fprintln(w)
		// No 'end' in text format
	}

	return nil
}

func (f *RubyFormatter) formatField(w io.Writer, field *ir.DistilledField, indent string) error {
	// Ruby doesn't have visibility keywords for fields
	// Instance/class variables are always private

	// Check if it's a constant
	isConstant := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierFinal {
			isConstant = true
			break
		}
	}

	if isConstant {
		// Ruby constants are uppercase
		fmt.Fprintf(w, "%s%s", indent, strings.ToUpper(field.Name))
	} else {
		// Instance or class variables
		if strings.HasPrefix(field.Name, "@@") {
			// Class variable
			fmt.Fprintf(w, "%s%s", indent, field.Name)
		} else if strings.HasPrefix(field.Name, "@") {
			// Instance variable
			fmt.Fprintf(w, "%s%s", indent, field.Name)
		} else {
			// Add @ prefix for instance variables
			fmt.Fprintf(w, "%s@%s", indent, field.Name)
		}
	}

	// Add default value if specified
	if field.DefaultValue != "" {
		fmt.Fprintf(w, " = %s", field.DefaultValue)
	}

	fmt.Fprintln(w)
	return nil
}

func (f *RubyFormatter) formatParameters(w io.Writer, params []ir.Parameter) {
	for i, param := range params {
		if i > 0 {
			fmt.Fprintf(w, ", ")
		}

		// Parameter name
		fmt.Fprintf(w, "%s", param.Name)

		// Default value
		if param.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", param.DefaultValue)
		}

		// Keyword argument syntax would go here if supported
		// if param.IsKeywordOnly {
		//	fmt.Fprintf(w, ":")
		// }

		// Splat operator
		if param.IsVariadic {
			fmt.Fprintf(w, "*")
		}
	}
}

func (f *RubyFormatter) getRubyVisibilityComment(visibility ir.Visibility) string {
	switch visibility {
	case ir.VisibilityPrivate:
		return "private"
	case ir.VisibilityProtected:
		return "protected"
	case ir.VisibilityPublic:
		return "" // No comment for public
	case ir.VisibilityInternal:
		return "private" // Ruby doesn't have internal
	default:
		return ""
	}
}
