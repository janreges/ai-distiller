package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// GoFormatter implements language-specific formatting for Go
type GoFormatter struct {
	BaseLanguageFormatter
	isFirstImport bool
	lastWasImport bool
	// printedBody is the source span of the last function printed with its
	// body; comments inside it are already part of that body.
	printedBody ir.Location
}

// Reset resets the formatter state for a new file
func (f *GoFormatter) Reset() {
	f.isFirstImport = true
	f.lastWasImport = false
	f.printedBody = ir.Location{}
}

// NewGoFormatter creates a new Go formatter
func NewGoFormatter() LanguageFormatter {
	return &GoFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("go"),
		isFirstImport:         true,
	}
}

// goIndent returns the indentation of nesting level n. Go output indents with
// one tab per level, as gofmt does, so it is never larger than the source.
func goIndent(n int) string {
	return strings.Repeat("\t", n)
}

// FormatNode implements LanguageFormatter
func (f *GoFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	indentStr := goIndent(indent)

	// Close import block if we're moving to non-import
	// Close before doc comments that precede type declarations
	if f.lastWasImport && node.GetNodeKind() != ir.KindImport {
		// Check if this is a doc comment (not a build constraint or inline comment)
		if comment, ok := node.(*ir.DistilledComment); ok && comment.Format == "doc" {
			// This is a doc comment for a type/function - close import block
			fmt.Fprintln(w, ")")
			fmt.Fprintln(w, "")
			f.lastWasImport = false
		} else if node.GetNodeKind() != ir.KindComment {
			// Any non-comment, non-import node closes the import block
			fmt.Fprintln(w, ")")
			f.lastWasImport = false
		}
	}

	switch n := node.(type) {
	case *ir.DistilledPackage:
		return f.formatPackage(w, n, indentStr)
	case *ir.DistilledImport:
		f.lastWasImport = true
		return f.formatImport(w, n, indentStr)
	case *ir.DistilledComment:
		if f.insidePrintedBody(n.Location) {
			return nil // printed verbatim as part of the body
		}
		return f.formatComment(w, n, indentStr)
	case *ir.DistilledField:
		return f.formatField(w, n, indentStr)
	case *ir.DistilledClass:
		return f.formatStruct(w, n, indent)
	case *ir.DistilledInterface:
		return f.formatInterface(w, n, indent)
	case *ir.DistilledFunction:
		// Check if this is a method (has receiver) or regular function
		if f.isMethod(n) {
			receiverType := f.getReceiverTypeName(n)
			return f.formatMethod(w, n, receiverType, indentStr)
		} else {
			return f.formatFunction(w, n, indentStr)
		}
	case *ir.DistilledTypeAlias:
		return f.formatTypeAlias(w, n, indentStr)
	default:
		// Skip unknown nodes
		reportUnhandled("go", node)
		return nil
	}
}

func (f *GoFormatter) formatPackage(w io.Writer, pkg *ir.DistilledPackage, indent string) error {
	// Package comments are handled separately as DistilledComment nodes
	fmt.Fprintf(w, "%spackage %s\n", indent, pkg.Name)
	return nil
}

func (f *GoFormatter) formatImport(w io.Writer, imp *ir.DistilledImport, indent string) error {
	// Check if this is the first import to add import block
	if f.isFirstImport {
		fmt.Fprintf(w, "\n%simport (\n", indent)
		f.isFirstImport = false
	}

	// Format individual import
	if len(imp.Symbols) > 0 && imp.Symbols[0].Alias != "" {
		// Aliased import
		fmt.Fprintf(w, "%s\t%s \"%s\"", indent, imp.Symbols[0].Alias, imp.Module)
	} else if len(imp.Symbols) > 0 && imp.Symbols[0].Name == "." {
		// Dot import
		fmt.Fprintf(w, "%s\t. \"%s\"", indent, imp.Module)
	} else if len(imp.Symbols) > 0 && imp.Symbols[0].Name == "_" {
		// Blank import
		fmt.Fprintf(w, "%s\t_ \"%s\"", indent, imp.Module)
	} else {
		// Standard import
		fmt.Fprintf(w, "%s\t\"%s\"", indent, imp.Module)
	}

	// Comments are handled separately as DistilledComment nodes
	fmt.Fprintln(w)

	return nil
}

func (f *GoFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent string) error {
	// Handle build constraints specially
	if strings.HasPrefix(comment.Text, "@build_constraint(") {
		constraint := strings.TrimPrefix(comment.Text, "@build_constraint(")
		constraint = strings.TrimSuffix(constraint, ")")
		fmt.Fprintf(w, "//go:build %s\n", constraint)
		return nil
	}

	// Regular comment
	lines := strings.Split(comment.Text, "\n")
	for _, line := range lines {
		if line != "" {
			fmt.Fprintf(w, "%s// %s\n", indent, line)
		} else {
			fmt.Fprintf(w, "%s//\n", indent)
		}
	}
	return nil
}

func (f *GoFormatter) formatField(w io.Writer, field *ir.DistilledField, indent string) error {
	// Field comments are handled separately

	// Add blank line before top-level const/var
	if indent == "" {
		fmt.Fprintln(w)
	}

	// Determine if this is a const or var
	isConst := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierFinal {
			isConst = true
			break
		}
	}

	// Format the declaration
	if isConst {
		fmt.Fprintf(w, "%sconst %s", indent, field.Name)
	} else {
		fmt.Fprintf(w, "%svar %s", indent, field.Name)
	}

	// Add type if specified
	if field.Type != nil && field.Type.Name != "" {
		fmt.Fprintf(w, " %s", field.Type.Name)
	}

	// Add value if specified
	if field.DefaultValue != "" {
		fmt.Fprintf(w, " = %s", field.DefaultValue)
	}

	fmt.Fprintln(w)
	return nil
}

func (f *GoFormatter) formatStruct(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := goIndent(indent)

	// Class comments are handled separately

	// Format struct declaration
	fmt.Fprintf(w, "\n%stype %s struct", indentStr, class.Name)

	// Check if there are fields
	hasFields := false
	for _, child := range class.Children {
		if _, ok := child.(*ir.DistilledField); ok {
			hasFields = true
			break
		}
	}

	if hasFields {
		fmt.Fprintln(w, " {")
		// Format fields
		for _, child := range class.Children {
			if field, ok := child.(*ir.DistilledField); ok {
				f.formatStructField(w, field, indent+1)
			}
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else {
		// Empty struct
		fmt.Fprintln(w, " {}")
	}

	// Methods are now formatted as top-level functions in original order
	// No need to format them here within the struct

	return nil
}

func (f *GoFormatter) formatStructField(w io.Writer, field *ir.DistilledField, indent int) error {
	indentStr := goIndent(indent)

	// Check if this is an embedded field
	isEmbedded := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierEmbedded {
			isEmbedded = true
			break
		}
	}

	if isEmbedded {
		// Embedded field - just the type name
		fmt.Fprintf(w, "%s%s", indentStr, field.Name)
	} else {
		// Regular field
		fmt.Fprintf(w, "%s%s", indentStr, field.Name)
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, " %s", field.Type.Name)
		}
	}

	// TODO: Add struct tag support when Annotations are added to IR

	// Comments are handled separately

	fmt.Fprintln(w)
	return nil
}

func (f *GoFormatter) formatInterface(w io.Writer, intf *ir.DistilledInterface, indent int) error {
	indentStr := goIndent(indent)

	// Interface comments are handled separately

	fmt.Fprintf(w, "\n%stype %s interface", indentStr, intf.Name)

	// Check for type constraints or embedded interfaces
	if len(intf.Extends) > 0 {
		fmt.Fprintln(w, " {")
		// Format type constraints (for generic constraints)
		for i, ext := range intf.Extends {
			if i > 0 {
				fmt.Fprintf(w, " | ")
			} else {
				fmt.Fprintf(w, "%s\t", indentStr)
			}
			fmt.Fprintf(w, "%s", ext.Name)
		}
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else if len(intf.Children) > 0 {
		fmt.Fprintln(w, " {")
		// Format methods
		for _, child := range intf.Children {
			if fn, ok := child.(*ir.DistilledFunction); ok {
				f.formatInterfaceMethod(w, fn, indent+1)
			}
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else {
		// Empty interface
		fmt.Fprintln(w, " {}")
	}

	return nil
}

func (f *GoFormatter) formatInterfaceMethod(w io.Writer, fn *ir.DistilledFunction, indent int) error {
	indentStr := goIndent(indent)

	// Method comments are handled separately

	// Format method signature
	fmt.Fprintf(w, "%s%s(", indentStr, fn.Name)
	f.formatParameters(w, fn.Parameters)
	fmt.Fprintf(w, ")")

	// Format return type
	if fn.Returns != nil {
		fmt.Fprintf(w, " %s", fn.Returns.Name)
	}

	fmt.Fprintln(w)
	return nil
}

func (f *GoFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent string) error {
	// Function comments are handled separately

	// Format compiler directives
	for _, dec := range fn.Decorators {
		fmt.Fprintf(w, "%s%s\n", indent, dec)
	}

	// Format function signature
	fmt.Fprintf(w, "\n%sfunc %s", indent, fn.Name)

	// Add generic type parameters if present
	if fn.TypeParams != nil && len(fn.TypeParams) > 0 {
		fmt.Fprintf(w, "[")
		for i, tp := range fn.TypeParams {
			if i > 0 {
				fmt.Fprintf(w, ", ")
			}
			fmt.Fprintf(w, "%s", tp.Name)
			if len(tp.Constraints) > 0 {
				fmt.Fprintf(w, " %s", tp.Constraints[0].Name)
			}
		}
		fmt.Fprintf(w, "]")
	}

	// Parameters
	fmt.Fprintf(w, "(")
	f.formatParameters(w, fn.Parameters)
	fmt.Fprintf(w, ")")

	// Return type
	if fn.Returns != nil && fn.Returns.Name != "" {
		fmt.Fprintf(w, " %s", fn.Returns.Name)
	}

	// Comments are handled separately

	// Implementation: the verbatim body, braces included
	if fn.Implementation != "" {
		fmt.Fprint(w, renderBodyVerbatim(fn.Implementation, indent))
		f.printedBody = fn.Location
	} else {
		fmt.Fprintln(w)
	}

	return nil
}

func (f *GoFormatter) formatMethod(w io.Writer, fn *ir.DistilledFunction, receiverType string, indent string) error {
	// Function comments are handled separately

	// Format compiler directives
	for _, dec := range fn.Decorators {
		fmt.Fprintf(w, "%s%s\n", indent, dec)
	}

	// Determine receiver from first parameter (if it exists)
	receiverStr := ""
	startParam := 0
	if len(fn.Parameters) > 0 {
		// Check if first parameter is the receiver
		// Methods have ModifierAbstract (though it might be removed)
		// or the first parameter might have special naming
		receiver := fn.Parameters[0]
		receiverName := receiver.Name
		if receiverName == "receiver" && len(fn.Parameters) > 1 {
			// Use second parameter name if available
			receiverName = "r"
		}
		receiverStr = fmt.Sprintf("(%s %s) ", receiverName, receiver.Type.Name)
		startParam = 1
	}

	// Format method signature
	fmt.Fprintf(w, "%sfunc %s%s(", indent, receiverStr, fn.Name)

	// Parameters (skip receiver)
	params := fn.Parameters[startParam:]
	f.formatParameters(w, params)
	fmt.Fprintf(w, ")")

	// Return type
	if fn.Returns != nil && fn.Returns.Name != "" {
		fmt.Fprintf(w, " %s", fn.Returns.Name)
	}

	// Comments are handled separately

	// Implementation: the verbatim body, braces included
	if fn.Implementation != "" {
		fmt.Fprint(w, renderBodyVerbatim(fn.Implementation, indent))
		f.printedBody = fn.Location
	} else {
		fmt.Fprintln(w)
	}

	return nil
}

// insidePrintedBody reports whether a comment starting at loc lies inside
// the last function printed with its body (after the function's start and
// before the position just past its closing brace).
func (f *GoFormatter) insidePrintedBody(loc ir.Location) bool {
	body := f.printedBody
	if body.EndLine == 0 {
		return false
	}
	afterStart := loc.StartLine > body.StartLine ||
		loc.StartLine == body.StartLine && loc.StartColumn > body.StartColumn
	beforeEnd := loc.StartLine < body.EndLine ||
		loc.StartLine == body.EndLine && loc.StartColumn < body.EndColumn
	return afterStart && beforeEnd
}

func (f *GoFormatter) formatTypeAlias(w io.Writer, alias *ir.DistilledTypeAlias, indent string) error {
	// Type alias comments are handled separately

	fmt.Fprintf(w, "\n%stype %s = %s\n", indent, alias.Name, alias.Type.Name)
	return nil
}

func (f *GoFormatter) formatParameters(w io.Writer, params []ir.Parameter) {
	for i, param := range params {
		if i > 0 {
			fmt.Fprintf(w, ", ")
		}
		if param.Name != "" {
			fmt.Fprintf(w, "%s", param.Name)
			if param.Type.Name != "" {
				fmt.Fprintf(w, " %s", param.Type.Name)
			}
		} else if param.Type.Name != "" {
			// Unnamed parameter
			fmt.Fprintf(w, "%s", param.Type.Name)
		}
	}
}

// isMethod checks if a function is a method (has receiver)
func (f *GoFormatter) isMethod(fn *ir.DistilledFunction) bool {
	// Check if function has AbstractMethod modifier (indicating it has a receiver)
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAbstract {
			return true
		}
	}
	return false
}

// getReceiverTypeName extracts the receiver type name from a method
func (f *GoFormatter) getReceiverTypeName(fn *ir.DistilledFunction) string {
	if f.isMethod(fn) && len(fn.Parameters) > 0 {
		receiverType := fn.Parameters[0].Type.Name
		// Strip pointer if present (*User -> User)
		if strings.HasPrefix(receiverType, "*") {
			receiverType = receiverType[1:]
		}
		// Extract base type name for generic types (Cache[K,V] -> Cache)
		if bracketIndex := strings.Index(receiverType, "["); bracketIndex != -1 {
			return receiverType[:bracketIndex]
		}
		return receiverType
	}
	return ""
}
