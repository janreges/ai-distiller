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
	return f.FormatNodes(w, []ir.DistilledNode{node}, indent)
}

// FormatNodes formats sibling nodes. A class or module is set off from what
// comes before it by a blank line; when a doc comment documents it, the blank
// line goes above the doc, so the doc stays next to the declaration. For the
// same reason the "# private" / "# protected" marker of a method goes above
// the method's doc, not between the doc and the def.
func (f *RubyFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
	indentStr := strings.Repeat("    ", indent)
	for i, node := range nodes {
		if opensRubyBlock(nodes, i) {
			fmt.Fprintln(w)
		}
		if fn := startsRubyMethod(nodes, i); fn != nil {
			if marker := f.getRubyVisibilityComment(fn.Visibility); marker != "" {
				fmt.Fprintf(w, "%s# %s\n", indentStr, marker)
			}
		}
		if err := f.formatNode(w, node, indent); err != nil {
			return err
		}
	}
	return nil
}

// startsRubyMethod returns the method that nodes[i] starts: the method itself,
// or the method that the doc comment nodes[i] documents. Otherwise nil.
func startsRubyMethod(nodes []ir.DistilledNode, i int) *ir.DistilledFunction {
	if fn, ok := nodes[i].(*ir.DistilledFunction); ok {
		if i == 0 || !isDocComment(nodes[i-1]) {
			return fn
		}
		return nil
	}
	if isDocComment(nodes[i]) && i+1 < len(nodes) {
		if fn, ok := nodes[i+1].(*ir.DistilledFunction); ok {
			return fn
		}
	}
	return nil
}

// opensRubyBlock reports whether nodes[i] starts a class or module block: the
// class or module itself, or the doc comment right before it.
func opensRubyBlock(nodes []ir.DistilledNode, i int) bool {
	if isRubyBlock(nodes[i]) {
		return i == 0 || !isDocComment(nodes[i-1])
	}
	return isDocComment(nodes[i]) && i+1 < len(nodes) && isRubyBlock(nodes[i+1])
}

func isRubyBlock(node ir.DistilledNode) bool {
	switch node.(type) {
	case *ir.DistilledClass, *ir.DistilledInterface:
		return true
	}
	return false
}

func isDocComment(node ir.DistilledNode) bool {
	c, ok := node.(*ir.DistilledComment)
	return ok && c.IsDoc()
}

func (f *RubyFormatter) formatNode(w io.Writer, node ir.DistilledNode, indent int) error {
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

// formatComment writes any comment, line, block (=begin/=end) or doc, as one
// "# " line per text line. The shebang line is written as it is in the source.
func (f *RubyFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent string) error {
	if isShebang(comment) {
		fmt.Fprintf(w, "#%s\n", comment.Text)
		return nil
	}
	for _, line := range strings.Split(comment.Text, "\n") {
		fmt.Fprintf(w, "%s%s\n", indent, strings.TrimRight("# "+line, " "))
	}
	return nil
}

func (f *RubyFormatter) formatClass(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Format class declaration (FormatNodes sets it off by a blank line)
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
	if err := f.FormatNodes(w, class.Children, indent+1); err != nil {
		return err
	}

	// No 'end' in text format

	return nil
}

func (f *RubyFormatter) formatModule(w io.Writer, mod *ir.DistilledInterface, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Format module declaration (FormatNodes sets it off by a blank line)
	fmt.Fprintf(w, "%smodule %s\n", indentStr, mod.Name)

	// Included modules (the parser records `include X` in a module as Extends)
	memberIndent := strings.Repeat("    ", indent+1)
	for _, mixin := range mod.Extends {
		fmt.Fprintf(w, "%sinclude %s\n", memberIndent, mixin.Name)
	}

	// Format module members
	if err := f.FormatNodes(w, mod.Children, indent+1); err != nil {
		return err
	}

	// No 'end' in text format

	return nil
}

// formatMethod writes a method. Its visibility marker ("# private") is
// written by FormatNodes.
func (f *RubyFormatter) formatMethod(w io.Writer, fn *ir.DistilledFunction, indent string) error {
	// Check for special method types
	isClassMethod := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierStatic {
			isClassMethod = true
			break
		}
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

		// Splat operator
		if param.IsVariadic {
			fmt.Fprintf(w, "*%s", strings.TrimPrefix(param.Name, "*"))
		} else {
			fmt.Fprintf(w, "%s", param.Name)
		}

		// Default value
		if param.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", param.DefaultValue)
		}

		// Keyword argument syntax would go here if supported
		// if param.IsKeywordOnly {
		//	fmt.Fprintf(w, ":")
		// }
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
