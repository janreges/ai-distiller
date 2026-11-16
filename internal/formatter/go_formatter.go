package formatter

import (
	"fmt"
	"io"
	"slices"
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

	if len(class.Children) == 0 {
		fmt.Fprintln(w, " {}")
		return nil
	}
	// Fields with their comments; methods print at file level, in source
	// order, as top-level functions
	fmt.Fprintln(w, " {")
	f.formatMembers(w, class.Children, indent+1)
	fmt.Fprintf(w, "%s}\n", indentStr)
	return nil
}

func (f *GoFormatter) formatInterface(w io.Writer, intf *ir.DistilledInterface, indent int) error {
	indentStr := goIndent(indent)

	// Interface comments are handled separately

	fmt.Fprintf(w, "\n%stype %s interface", indentStr, intf.Name)

	if len(intf.Extends) == 0 && len(intf.Children) == 0 {
		fmt.Fprintln(w, " {}")
		return nil
	}
	fmt.Fprintln(w, " {")
	// Embedded elements first, one per line: an embedded interface
	// (io.Reader) or a type set (~int | ~string), each as written, under
	// the comments written among them; then the methods with theirs.
	split := 0
	if len(intf.Extends) > 0 {
		split = embedCommentsEnd(intf.Children)
	}
	f.formatMembers(w, intf.Children[:split], indent+1)
	for _, ext := range intf.Extends {
		fmt.Fprintf(w, "%s%s\n", goIndent(indent+1), ext.Name)
	}
	f.formatMembers(w, intf.Children[split:], indent+1)
	fmt.Fprintf(w, "%s}\n", indentStr)
	return nil
}

// formatMembers writes the children of a struct or interface body, one line
// each at indent: a field, an interface method, or a comment, which keeps
// its place among them. A comment that ends a member's line in the source
// (the parser puts it right before that member) is written at the end of
// that member's line instead.
func (f *GoFormatter) formatMembers(w io.Writer, children []ir.DistilledNode, indent int) {
	indentStr := goIndent(indent)
	var trailing []string
	for i, child := range children {
		switch n := child.(type) {
		case *ir.DistilledComment:
			if i+1 < len(children) && endsLineOf(n, children[i+1]) {
				trailing = append(trailing, n.Text)
				continue
			}
			f.formatComment(w, n, indentStr)
			continue
		case *ir.DistilledField:
			fmt.Fprint(w, indentStr+goFieldLine(n))
		case *ir.DistilledFunction:
			fmt.Fprint(w, indentStr+f.interfaceMethodLine(n))
		default:
			continue
		}
		for _, text := range trailing {
			fmt.Fprintf(w, " // %s", text)
		}
		trailing = nil
		fmt.Fprintln(w)
	}
}

// endsLineOf reports whether comment c is written at the end of member's
// (last) line: one line of text that starts on that line, after the member
// starts.
func endsLineOf(c *ir.DistilledComment, member ir.DistilledNode) bool {
	switch member.(type) {
	case *ir.DistilledField, *ir.DistilledFunction:
	default:
		return false
	}
	at, m := c.Location, member.GetLocation()
	return at.StartLine > 0 && !strings.Contains(c.Text, "\n") && at.StartLine == m.EndLine &&
		(at.StartLine > m.StartLine || at.StartColumn > m.StartColumn)
}

// embedCommentsEnd returns where the comments written among the embedded
// types of an interface end in its children: before the first method and the
// comments that belong to it, the one at the end of its line and the group
// directly above it at its column. Without a method, every child is such a
// comment. The embedded types themselves are in Extends, without a position.
func embedCommentsEnd(children []ir.DistilledNode) int {
	first := slices.IndexFunc(children, func(n ir.DistilledNode) bool {
		_, ok := n.(*ir.DistilledFunction)
		return ok
	})
	if first < 0 {
		return len(children)
	}
	end := first
	if c, ok := commentAt(children, end-1); ok && endsLineOf(c, children[first]) {
		end--
	}
	method := children[first].GetLocation()
	if c, ok := commentAt(children, end-1); ok &&
		c.Location.EndLine+1 == method.StartLine && c.Location.StartColumn == method.StartColumn {
		end--
	}
	return end
}

// commentAt returns children[i] when it is a comment.
func commentAt(children []ir.DistilledNode, i int) (*ir.DistilledComment, bool) {
	if i < 0 || i >= len(children) {
		return nil, false
	}
	c, ok := children[i].(*ir.DistilledComment)
	return c, ok
}

// goFieldLine returns a struct field as it prints, without indentation: an
// embedded field as its type, any other as its name and type.
func goFieldLine(field *ir.DistilledField) string {
	// TODO: Add struct tag support when Annotations are added to IR
	if slices.Contains(field.Modifiers, ir.ModifierEmbedded) || field.Type == nil || field.Type.Name == "" {
		return field.Name
	}
	return field.Name + " " + field.Type.Name
}

// interfaceMethodLine returns an interface method as it prints, without
// indentation: its name, parameters and results.
func (f *GoFormatter) interfaceMethodLine(fn *ir.DistilledFunction) string {
	var b strings.Builder
	b.WriteString(fn.Name + "(")
	f.formatParameters(&b, fn.Parameters)
	b.WriteString(")")
	if fn.Returns != nil && fn.Returns.Name != "" {
		b.WriteString(" " + fn.Returns.Name)
	}
	return b.String()
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

// formatTypeAlias prints a type declaration that is neither a struct nor an
// interface: an alias (modifier type_alias) as "type Alias = int", a defined
// type, which is a new type and not another name for its underlying one, as
// "type Named int".
func (f *GoFormatter) formatTypeAlias(w io.Writer, alias *ir.DistilledTypeAlias, indent string) error {
	// Type alias comments are handled separately

	if slices.Contains(alias.Modifiers, ir.ModifierTypeAlias) {
		fmt.Fprintf(w, "\n%stype %s = %s\n", indent, alias.Name, alias.Type.Name)
	} else {
		fmt.Fprintf(w, "\n%stype %s %s\n", indent, alias.Name, alias.Type.Name)
	}
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
