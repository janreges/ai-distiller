package formatter

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// RustFormatter implements language-specific formatting for Rust
//
// Visibility symbols in text format:
//   - (no prefix) = public visibility (pub)
//   - "-" = private visibility (module-private, no modifier)
//   - "*" = protected visibility (pub(super) or pub(in path))
//   - "~" = internal visibility (pub(crate))
type RustFormatter struct {
	BaseLanguageFormatter
}

// NewRustFormatter creates a new Rust formatter
func NewRustFormatter() LanguageFormatter {
	return &RustFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("rust"),
	}
}

// FormatNode implements LanguageFormatter. Every node is written through a
// docWriter: a call with any other writer formats node as a list of one
// (FormatNodes).
func (f *RustFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	if _, ok := w.(*docWriter); !ok {
		return f.FormatNodes(w, []ir.DistilledNode{node}, indent)
	}
	indentStr := strings.Repeat("    ", indent)

	switch n := node.(type) {
	case *ir.DistilledImport:
		return f.formatImport(w, n, indentStr)
	case *ir.DistilledClass:
		return f.formatStruct(w, n, indent)
	case *ir.DistilledInterface:
		return f.formatTrait(w, n, indent)
	case *ir.DistilledFunction:
		return f.formatFunction(w, n, indentStr, !hasModifier(n.Modifiers, ir.ModifierAbstract))
	case *ir.DistilledField:
		return f.formatField(w, n, indentStr)
	case *ir.DistilledTypeAlias:
		return f.formatTypeAlias(w, n, indentStr)
	case *ir.DistilledComment:
		return f.formatComment(w, n, indentStr)
	default:
		// Skip unknown nodes
		reportUnhandled("rust", node)
		return nil
	}
}

// FormatNodes implements SiblingFormatter: it formats sibling nodes with
// FormatNode, except the outer doc comments right before an item that prints
// nothing (an extern block whose members were all removed), which would
// otherwise appear to document the next item. The outermost call writes
// through a docWriter, so the comments before an item sit directly on it.
func (f *RustFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
	if _, nested := w.(*docWriter); !nested {
		dw := &docWriter{w: w}
		if err := f.FormatNodes(dw, nodes, indent); err != nil {
			return err
		}
		return dw.flush()
	}
	// From the end: skip[i] when nodes[i] is an outer doc and the first node
	// after it that is not a comment prints nothing.
	skip := make([]bool, len(nodes))
	unprinted := false
	for i := len(nodes) - 1; i >= 0; i-- {
		if c, ok := nodes[i].(*ir.DistilledComment); ok {
			skip[i] = unprinted && c.IsDoc() && !c.Inner
			continue
		}
		unprinted = printsNothing(nodes[i])
	}
	for i, node := range nodes {
		if skip[i] {
			continue
		}
		if printsNothing(node) {
			// The plain comments above it keep their place.
			if err := w.(*docWriter).flush(); err != nil {
				return err
			}
		}
		if err := f.FormatNode(w, node, indent); err != nil {
			return err
		}
	}
	return nil
}

// docWriter holds the comments written before an item until the item's first
// write. When that write starts with a blank line (the item is set off from
// the one before it), the blank line goes before the comments, so that they
// sit directly on the item they document instead of under the item before.
// Right after the "{" that opens a body (openBody) there is no item before,
// and that blank line is dropped.
type docWriter struct {
	w io.Writer
	// pending is the text of the held comments; plain is true when one of
	// them is not a doc comment.
	pending []byte
	plain   bool
	// opened is true while nothing has been written after the "{" line of a
	// body.
	opened bool
}

func (d *docWriter) Write(p []byte) (int, error) {
	n := len(p)
	if d.opened && n > 0 {
		d.opened = false
		if p[0] == '\n' {
			p = p[1:]
			if len(p) == 0 {
				return n, nil
			}
		}
	}
	if len(d.pending) == 0 || len(p) == 0 {
		if _, err := d.w.Write(p); err != nil {
			return 0, err
		}
		return n, nil
	}
	out := make([]byte, 0, len(d.pending)+len(p))
	if p[0] == '\n' {
		out = append(append(append(out, '\n'), d.pending...), p[1:]...)
	} else {
		out = append(append(out, d.pending...), p...)
	}
	d.pending, d.plain = d.pending[:0], false
	if _, err := d.w.Write(out); err != nil {
		return 0, err
	}
	return n, nil
}

// openBody writes the " {" that opens the body of an impl block, a trait, an
// extern block or a module, whose members each start with a blank line: the
// first member's blank line is dropped, so that the body starts on the next
// line.
func openBody(w io.Writer) {
	fmt.Fprintln(w, " {")
	if dw, ok := w.(*docWriter); ok {
		dw.opened = true
	}
}

// hold adds a comment's text to the held comments. A doc comment after a
// plain one (a file header above the doc of the first item) releases the
// plain one first, so that the header keeps its own place.
func (d *docWriter) hold(text string, doc bool) error {
	if doc && d.plain {
		if err := d.flush(); err != nil {
			return err
		}
	}
	d.pending = append(d.pending, text...)
	d.plain = d.plain || !doc
	return nil
}

// flush writes the held comments as they are.
func (d *docWriter) flush() error {
	if len(d.pending) == 0 {
		return nil
	}
	_, err := d.w.Write(d.pending)
	d.pending, d.plain, d.opened = d.pending[:0], false, false
	return err
}

// printsNothing reports whether node prints nothing: an extern block or an
// inherent impl block (not "impl Trait for Type") without members. Such a
// block says nothing; rustdoc does not show an empty inherent impl either. An
// empty trait impl prints: "impl Copy for X {}" states a fact.
func printsNothing(node ir.DistilledNode) bool {
	class, ok := node.(*ir.DistilledClass)
	if !ok || len(class.Children) > 0 {
		return false
	}
	switch rustItemKind(class) {
	case "extern":
		return true
	case "impl":
		return !strings.Contains(class.Name, " for ")
	}
	return false
}

// formatImport writes a use declaration, or an extern crate. ImportType is
// the keyword with the visibility of a re-export ("pub use", "extern crate");
// an empty one is "use".
func (f *RustFormatter) formatImport(w io.Writer, imp *ir.DistilledImport, indent string) error {
	keyword := imp.ImportType
	if keyword == "" {
		keyword = "use"
	}
	if strings.HasSuffix(keyword, "extern crate") {
		fmt.Fprintf(w, "%s%s %s;\n", indent, keyword, imp.Module)
		return nil
	}
	if len(imp.Symbols) > 0 {
		symbols := make([]string, len(imp.Symbols))
		for i, sym := range imp.Symbols {
			if sym.Alias != "" {
				symbols[i] = fmt.Sprintf("%s as %s", sym.Name, sym.Alias)
			} else {
				symbols[i] = sym.Name
			}
		}
		// A group without a path ("use {a::B, c::D};") has an empty Module
		prefix := ""
		if imp.Module != "" {
			prefix = imp.Module + "::"
		}
		fmt.Fprintf(w, "%s%s %s{%s};\n", indent, keyword, prefix, strings.Join(symbols, ", "))
	} else {
		fmt.Fprintf(w, "%s%s %s;\n", indent, keyword, imp.Module)
	}
	return nil
}

// formatComment writes a comment in its own style, one line per text line: an
// outer doc as "/// text", an inner doc as "//! text" and any other comment
// as "// text". An empty line has no trailing space. Through a docWriter, a
// comment other than an inner doc is held for the item after it; an inner doc
// belongs to the enclosing module and is written at once.
func (f *RustFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent string) error {
	marker := "//"
	if comment.IsDoc() {
		marker = "///"
		if comment.Inner {
			marker = "//!"
		}
	}
	var b strings.Builder
	for _, line := range strings.Split(comment.Text, "\n") {
		if line = strings.TrimRight(line, " \t\r"); line != "" {
			line = " " + line
		}
		fmt.Fprintf(&b, "%s%s%s\n", indent, marker, line)
	}
	if dw, ok := w.(*docWriter); ok {
		if !comment.Inner {
			return dw.hold(b.String(), comment.IsDoc())
		}
		if err := dw.flush(); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// formatDecorators writes the outer attributes of an item, field or variant,
// one per line.
func formatDecorators(w io.Writer, decorators []string, indent string) {
	for _, decorator := range decorators {
		fmt.Fprintf(w, "%s%s\n", indent, decorator)
	}
}

// rustItemKind returns the Rust item a DistilledClass stands for: "impl"
// (its name starts with impl), "struct", "union", "enum", "trait" (modifier
// abstract) or "extern" (an extern block) by its modifier, and "mod" for a
// class without one of these modifiers.
func rustItemKind(class *ir.DistilledClass) string {
	if strings.HasPrefix(class.Name, "impl ") || strings.HasPrefix(class.Name, "impl<") {
		return "impl"
	}
	for _, mod := range class.Modifiers {
		switch mod {
		case ir.ModifierStruct:
			return "struct"
		case ir.ModifierUnion:
			return "union"
		case ir.ModifierEnum:
			return "enum"
		case ir.ModifierAbstract:
			return "trait"
		case ir.ModifierExtern:
			return "extern"
		}
	}
	return "mod"
}

// rustExtensions returns the Rust extensions of node, or an empty value when
// it has none.
func rustExtensions(node *ir.BaseNode) ir.RustExtensions {
	if node.Extensions == nil || node.Extensions.Rust == nil {
		return ir.RustExtensions{}
	}
	return *node.Extensions.Rust
}

// formatStruct writes a struct, union, enum, trait, impl block, extern block
// or module, after a blank line. The header is the visibility, the kind and
// the name (an impl or extern block prints its name alone), a trait's
// supertraits and the where clause; then the body. An extern block whose
// members were all removed prints nothing.
func (f *RustFormatter) formatStruct(w io.Writer, class *ir.DistilledClass, indent int) error {
	if printsNothing(class) {
		return nil
	}
	kind := rustItemKind(class)
	indentStr := strings.Repeat("    ", indent)
	ext := rustExtensions(&class.BaseNode)

	fmt.Fprintln(w)
	formatDecorators(w, class.Decorators, indentStr)
	fmt.Fprint(w, indentStr)
	switch kind {
	case "impl", "extern":
		if ext.IsUnsafe {
			fmt.Fprint(w, "unsafe ")
		}
		fmt.Fprint(w, class.Name)
	default:
		if visKeyword := f.getRustVisibilityKeyword(class.Visibility, &class.BaseNode); visKeyword != "" {
			fmt.Fprintf(w, "%s ", visKeyword)
		}
		if ext.IsUnsafe && kind == "trait" {
			fmt.Fprint(w, "unsafe ")
		}
		fmt.Fprintf(w, "%s %s", kind, class.Name)
	}

	// Add generic type parameters
	if len(class.TypeParams) > 0 {
		typeParams := make([]string, len(class.TypeParams))
		for i, param := range class.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	if kind == "trait" && len(class.Extends) > 0 {
		supertraits := make([]string, len(class.Extends))
		for i, ext := range class.Extends {
			supertraits[i] = ext.Name
		}
		fmt.Fprintf(w, ": %s", strings.Join(supertraits, " + "))
	}

	// A tuple struct's where clause follows its fields.
	where := ""
	if ext.Where != "" {
		where = " " + ext.Where
	}
	if kind == "struct" && ext.TupleArity > 0 {
		fmt.Fprintf(w, "(%s)%s;\n", strings.Join(f.tupleFields(class, ext.TupleArity), ", "), where)
		return nil
	}
	fmt.Fprint(w, where)

	switch kind {
	case "impl", "trait", "extern":
		return f.formatMembers(w, class, kind, indent)
	case "enum":
		return f.formatEnumBody(w, class, indentStr)
	case "mod":
		// A module prints every item it holds; one without any is "mod x;".
		if len(class.Children) == 0 {
			fmt.Fprintln(w, ";")
			return nil
		}
		openBody(w)
		if err := f.FormatNodes(w, class.Children, indent+1); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	case "struct", "union":
		return f.formatStructBody(w, class, indent)
	}
	return nil
}

// tupleFields returns the arity fields of a tuple struct in order, each as
// its visibility keyword and type; a field the stripper removed is "_".
func (f *RustFormatter) tupleFields(class *ir.DistilledClass, arity int) []string {
	fields := make([]string, arity)
	for i := range fields {
		fields[i] = "_"
	}
	for _, child := range class.Children {
		field, ok := child.(*ir.DistilledField)
		if !ok {
			continue
		}
		i, err := strconv.Atoi(field.Name)
		if err != nil || i < 0 || i >= arity {
			continue
		}
		fields[i] = ""
		if field.Type != nil {
			fields[i] = field.Type.Name
		}
		if visKeyword := f.getRustVisibilityKeyword(field.Visibility, &field.BaseNode); visKeyword != "" {
			fields[i] = visKeyword + " " + fields[i]
		}
	}
	return fields
}

// formatMembers writes the body of a trait, an impl block or an extern
// block. The members of a trait and of "impl Trait for Type" take no
// visibility keyword: they are as visible as the trait. A trait's associated
// types print as "type X: Bound;" without a blank line before them. A
// function without a body has no keyword either, except in an extern block,
// where each foreign item has its own visibility.
func (f *RustFormatter) formatMembers(w io.Writer, class *ir.DistilledClass, kind string, indent int) error {
	indentStr := strings.Repeat("    ", indent)
	memberIndent := indentStr + "    "
	traitMembers := kind == "trait" || strings.Contains(class.Name, " for ")

	openBody(w)
	for _, child := range class.Children {
		var err error
		switch n := child.(type) {
		case *ir.DistilledField:
			if kind == "trait" && !hasModifier(n.Modifiers, ir.ModifierFinal) {
				formatDecorators(w, n.Decorators, memberIndent)
				fmt.Fprintf(w, "%stype %s", memberIndent, n.Name)
				if n.Type != nil && n.Type.Name != "" && n.Type.Name != "type" {
					fmt.Fprintf(w, ": %s", n.Type.Name)
				}
				fmt.Fprintln(w, ";")
			} else {
				err = f.formatFieldWithVisibility(w, n, memberIndent, !traitMembers)
			}
		case *ir.DistilledFunction:
			withVisibility := !traitMembers && (kind == "extern" || !hasModifier(n.Modifiers, ir.ModifierAbstract))
			err = f.formatFunction(w, n, memberIndent, withVisibility)
		default:
			err = f.FormatNode(w, child, indent+1)
		}
		if err != nil {
			return err
		}
	}
	fmt.Fprintf(w, "%s}\n", indentStr)
	return nil
}

// formatEnumBody writes the variants of an enum, one per line, or ";" for an
// enum without any.
func (f *RustFormatter) formatEnumBody(w io.Writer, class *ir.DistilledClass, indentStr string) error {
	variants := 0
	for _, child := range class.Children {
		if _, ok := child.(*ir.DistilledField); ok {
			variants++
		}
	}
	if variants == 0 {
		fmt.Fprintln(w, ";")
		return nil
	}
	fmt.Fprintln(w, " {")
	for _, child := range class.Children {
		field, ok := child.(*ir.DistilledField)
		if !ok {
			if comment, ok := child.(*ir.DistilledComment); ok {
				if err := f.formatComment(w, comment, indentStr+"    "); err != nil {
					return err
				}
			}
			continue
		}
		formatDecorators(w, field.Decorators, indentStr+"    ")
		fmt.Fprintf(w, "%s    %s", indentStr, field.Name)
		if field.Type != nil && field.Type.Name != "" {
			if strings.HasPrefix(field.Type.Name, "(") {
				// Tuple variant
				fmt.Fprint(w, field.Type.Name)
			} else if strings.HasPrefix(field.Type.Name, "{") {
				// Struct variant: the parser keeps its fields as written
				fmt.Fprintf(w, " %s", field.Type.Name)
			} else if field.Type.Name == "struct" {
				// Struct variant
				fmt.Fprintf(w, " { /* fields */ }")
			}
		}
		if field.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", field.DefaultValue)
		}
		fmt.Fprintln(w, ",")
	}
	fmt.Fprintf(w, "%s}\n", indentStr)
	return nil
}

// formatStructBody writes the named fields of a struct or union, or ";" when
// it has none, and the methods of a struct in an impl block after it. When
// the stripper removed private fields, "/* private fields */" stands for
// them, as in rustdoc: on the header line when no field is left, else after
// the fields left.
func (f *RustFormatter) formatStructBody(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := strings.Repeat("    ", indent)
	hasFields := false
	privateLeft := 0
	for _, child := range class.Children {
		if field, ok := child.(*ir.DistilledField); ok {
			hasFields = true
			if field.Visibility != ir.VisibilityPublic {
				privateLeft++
			}
		}
	}
	privateRemoved := privateLeft < rustExtensions(&class.BaseNode).PrivateFields

	if !hasFields && privateRemoved {
		fmt.Fprintln(w, " { /* private fields */ }")
	} else if hasFields {
		fmt.Fprintln(w, " {")
		for _, child := range class.Children {
			var err error
			switch n := child.(type) {
			case *ir.DistilledField:
				err = f.formatStructField(w, n, indent+1)
			case *ir.DistilledComment:
				err = f.formatComment(w, n, indentStr+"    ")
			}
			if err != nil {
				return err
			}
		}
		if privateRemoved {
			fmt.Fprintf(w, "%s    /* private fields */\n", indentStr)
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else {
		// Unit struct
		fmt.Fprintln(w, ";")
	}

	// Format impl blocks (methods)
	var methods []*ir.DistilledFunction
	for _, child := range class.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok {
			methods = append(methods, fn)
		}
	}

	if len(methods) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintf(w, "%simpl %s", indentStr, class.Name)

		// Add generic type parameters for impl
		if len(class.TypeParams) > 0 {
			typeParams := make([]string, len(class.TypeParams))
			for i, param := range class.TypeParams {
				typeParams[i] = param.Name
			}
			fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
		}

		fmt.Fprintln(w, " {")
		for _, method := range methods {
			if err := f.formatImplMethod(w, method, indent+1); err != nil {
				return err
			}
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	}
	return nil
}

func (f *RustFormatter) formatStructField(w io.Writer, field *ir.DistilledField, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(field.Visibility, &field.BaseNode)

	typeName := ""
	if field.Type != nil {
		typeName = field.Type.Name
	}
	formatDecorators(w, field.Decorators, indentStr)
	if visKeyword != "" {
		fmt.Fprintf(w, "%s%s %s: %s,\n", indentStr, visKeyword, field.Name, typeName)
	} else {
		fmt.Fprintf(w, "%s%s: %s,\n", indentStr, field.Name, typeName)
	}
	return nil
}

func (f *RustFormatter) formatTrait(w io.Writer, trait *ir.DistilledInterface, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Add blank line before trait
	fmt.Fprintln(w)

	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(trait.Visibility, &trait.BaseNode)

	// Format trait declaration
	if visKeyword != "" {
		fmt.Fprintf(w, "%s%s trait %s", indentStr, visKeyword, trait.Name)
	} else {
		fmt.Fprintf(w, "%strait %s", indentStr, trait.Name)
	}

	// Add generic type parameters
	if len(trait.TypeParams) > 0 {
		typeParams := make([]string, len(trait.TypeParams))
		for i, param := range trait.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Add supertrait bounds
	if len(trait.Extends) > 0 {
		extends := make([]string, len(trait.Extends))
		for i, ext := range trait.Extends {
			extends[i] = ext.Name
		}
		fmt.Fprintf(w, ": %s", strings.Join(extends, " + "))
	}

	fmt.Fprintln(w, " {")

	// Format trait members
	for _, child := range trait.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok {
			f.formatTraitMethod(w, fn, indent+1)
		}
	}

	fmt.Fprintf(w, "%s}\n", indentStr)

	return nil
}

// formatFunction writes fn, or a macro_rules! when fn has the modifier
// macro. withVisibility false leaves out the visibility keyword, as for the
// members of a trait or a trait impl and for a function without a body
// outside an extern block. An abstract function (declared without a body,
// like a required trait method) ends with ";".
func (f *RustFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent string, withVisibility bool) error {
	if hasModifier(fn.Modifiers, ir.ModifierMacro) {
		return f.formatMacro(w, fn, indent)
	}

	isAbstract := hasModifier(fn.Modifiers, ir.ModifierAbstract)
	ext := rustExtensions(&fn.BaseNode)

	// The signature starts with the visibility, then the qualifiers in
	// Rust's order: const async unsafe extern "ABI".
	var head []string
	if withVisibility {
		if visKeyword := f.getRustVisibilityKeyword(fn.Visibility, &fn.BaseNode); visKeyword != "" {
			head = append(head, visKeyword)
		}
	}
	if hasModifier(fn.Modifiers, ir.ModifierConst) {
		head = append(head, "const")
	}
	if hasModifier(fn.Modifiers, ir.ModifierAsync) {
		head = append(head, "async")
	}
	if ext.IsUnsafe {
		head = append(head, "unsafe")
	}
	if hasModifier(fn.Modifiers, ir.ModifierExtern) {
		head = append(head, "extern")
		if ext.ABI != "" {
			head = append(head, strconv.Quote(ext.ABI))
		}
	}
	head = append(head, "fn")
	fmt.Fprintln(w)
	formatDecorators(w, fn.Decorators, indent)
	fmt.Fprintf(w, "%s%s %s", indent, strings.Join(head, " "), fn.Name)

	// Add generic type parameters
	if len(fn.TypeParams) > 0 {
		typeParams := make([]string, len(fn.TypeParams))
		for i, param := range fn.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Parameters
	fmt.Fprintf(w, "(")
	f.formatParameters(w, fn.Parameters)
	fmt.Fprintf(w, ")")

	// Return type and/or where clause
	if fn.Returns != nil && fn.Returns.Name != "" && fn.Returns.Name != "()" {
		// Check if it's just a where clause (starts with "where")
		if strings.HasPrefix(strings.TrimSpace(fn.Returns.Name), "where ") {
			fmt.Fprintf(w, " %s", fn.Returns.Name)
		} else {
			fmt.Fprintf(w, " -> %s", fn.Returns.Name)
		}
	}

	// Implementation
	if fn.Implementation != "" {
		fmt.Fprint(w, renderBody(fn.Implementation, indent))
	} else if isAbstract {
		fmt.Fprintln(w, ";")
	} else {
		fmt.Fprintln(w)
	}

	return nil
}

// formatMacro writes a macro_rules! definition: its body when the
// implementation is kept, otherwise one line per rule, its matcher followed
// by "=> ...;". A macro takes no visibility keyword (#[macro_export] makes it
// public).
func (f *RustFormatter) formatMacro(w io.Writer, macro *ir.DistilledFunction, indent string) error {
	fmt.Fprintln(w)
	formatDecorators(w, macro.Decorators, indent)
	fmt.Fprintf(w, "%smacro_rules! %s", indent, macro.Name)
	if macro.Implementation != "" {
		fmt.Fprint(w, renderBody(macro.Implementation, indent))
		return nil
	}
	fmt.Fprintln(w, " {")
	for _, rule := range macro.Parameters {
		fmt.Fprintf(w, "%s    %s => ...;\n", indent, rule.Name)
	}
	fmt.Fprintf(w, "%s}\n", indent)
	return nil
}

func (f *RustFormatter) formatImplMethod(w io.Writer, fn *ir.DistilledFunction, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(fn.Visibility, &fn.BaseNode)

	// Check for async
	isAsync := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			isAsync = true
			break
		}
	}

	// Format method signature
	formatDecorators(w, fn.Decorators, indentStr)
	if visKeyword != "" {
		if isAsync {
			fmt.Fprintf(w, "%s%s async fn %s", indentStr, visKeyword, fn.Name)
		} else {
			fmt.Fprintf(w, "%s%s fn %s", indentStr, visKeyword, fn.Name)
		}
	} else {
		if isAsync {
			fmt.Fprintf(w, "%sasync fn %s", indentStr, fn.Name)
		} else {
			fmt.Fprintf(w, "%sfn %s", indentStr, fn.Name)
		}
	}

	// Add generic type parameters
	if len(fn.TypeParams) > 0 {
		typeParams := make([]string, len(fn.TypeParams))
		for i, param := range fn.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Parameters
	fmt.Fprintf(w, "(")
	f.formatParameters(w, fn.Parameters)
	fmt.Fprintf(w, ")")

	// Return type and/or where clause
	if fn.Returns != nil && fn.Returns.Name != "" && fn.Returns.Name != "()" {
		// Check if it's just a where clause (starts with "where")
		if strings.HasPrefix(strings.TrimSpace(fn.Returns.Name), "where ") {
			fmt.Fprintf(w, " %s", fn.Returns.Name)
		} else {
			fmt.Fprintf(w, " -> %s", fn.Returns.Name)
		}
	}

	// Implementation
	if fn.Implementation != "" {
		fmt.Fprint(w, renderBody(fn.Implementation, indentStr))
	} else {
		fmt.Fprintln(w)
	}

	return nil
}

func (f *RustFormatter) formatTraitMethod(w io.Writer, fn *ir.DistilledFunction, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Check for async
	isAsync := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			isAsync = true
			break
		}
	}

	// Format method signature
	if isAsync {
		fmt.Fprintf(w, "%sasync fn %s", indentStr, fn.Name)
	} else {
		fmt.Fprintf(w, "%sfn %s", indentStr, fn.Name)
	}

	// Add generic type parameters
	if len(fn.TypeParams) > 0 {
		typeParams := make([]string, len(fn.TypeParams))
		for i, param := range fn.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Parameters
	fmt.Fprintf(w, "(")
	f.formatParameters(w, fn.Parameters)
	fmt.Fprintf(w, ")")

	// Return type and/or where clause
	if fn.Returns != nil && fn.Returns.Name != "" && fn.Returns.Name != "()" {
		// Check if it's just a where clause (starts with "where")
		if strings.HasPrefix(strings.TrimSpace(fn.Returns.Name), "where ") {
			fmt.Fprintf(w, " %s", fn.Returns.Name)
		} else {
			fmt.Fprintf(w, " -> %s", fn.Returns.Name)
		}
	}

	// Default implementation or just signature
	if fn.Implementation != "" {
		fmt.Fprint(w, renderBody(fn.Implementation, indentStr))
	} else {
		fmt.Fprintln(w, ";")
	}

	return nil
}

func (f *RustFormatter) formatField(w io.Writer, field *ir.DistilledField, indent string) error {
	return f.formatFieldWithVisibility(w, field, indent, true)
}

// formatFieldWithVisibility writes a const, static, type alias or variable;
// withVisibility false leaves out the visibility keyword, as for the members
// of a trait or a trait impl.
func (f *RustFormatter) formatFieldWithVisibility(w io.Writer, field *ir.DistilledField, indent string, withVisibility bool) error {
	visKeyword := ""
	if withVisibility {
		visKeyword = f.getRustVisibilityKeyword(field.Visibility, &field.BaseNode)
	}

	// Check if it's a constant, static, or type alias
	isConst := false
	isStatic := false
	isTypeAlias := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierFinal {
			isConst = true
		}
		if mod == ir.ModifierStatic {
			isStatic = true
		}
		if mod == ir.ModifierTypeAlias {
			isTypeAlias = true
		}
	}

	// Format field/constant
	typeName := ""
	if field.Type != nil {
		typeName = field.Type.Name
	}
	if isTypeAlias || isConst || isStatic {
		fmt.Fprintln(w)
	}
	formatDecorators(w, field.Decorators, indent)
	if isTypeAlias {
		if visKeyword != "" {
			fmt.Fprintf(w, "%s%s type %s", indent, visKeyword, field.Name)
		} else {
			fmt.Fprintf(w, "%stype %s", indent, field.Name)
		}
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, " = %s", field.Type.Name)
		}
	} else if isConst {
		if visKeyword != "" {
			fmt.Fprintf(w, "%s%s const %s: %s", indent, visKeyword, field.Name, typeName)
		} else {
			fmt.Fprintf(w, "%sconst %s: %s", indent, field.Name, typeName)
		}
	} else if isStatic {
		static := "static"
		if hasModifier(field.Modifiers, ir.ModifierMutable) {
			static = "static mut"
		}
		if visKeyword != "" {
			fmt.Fprintf(w, "%s%s %s %s: %s", indent, visKeyword, static, field.Name, typeName)
		} else {
			fmt.Fprintf(w, "%s%s %s: %s", indent, static, field.Name, typeName)
		}
	} else {
		// Regular field (shouldn't appear at top level in Rust)
		if visKeyword != "" {
			fmt.Fprintf(w, "%s%s let %s", indent, visKeyword, field.Name)
		} else {
			fmt.Fprintf(w, "%slet %s", indent, field.Name)
		}
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, ": %s", field.Type.Name)
		}
	}

	// Add value if specified
	if field.DefaultValue != "" {
		fmt.Fprintf(w, " = %s", field.DefaultValue)
	}

	fmt.Fprintln(w, ";")
	return nil
}

func (f *RustFormatter) formatTypeAlias(w io.Writer, alias *ir.DistilledTypeAlias, indent string) error {
	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(alias.Visibility, &alias.BaseNode)

	if visKeyword != "" {
		fmt.Fprintf(w, "\n%s%s type %s", indent, visKeyword, alias.Name)
	} else {
		fmt.Fprintf(w, "\n%stype %s", indent, alias.Name)
	}

	// Add generic type parameters
	if len(alias.TypeParams) > 0 {
		typeParams := make([]string, len(alias.TypeParams))
		for i, param := range alias.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += ": " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	fmt.Fprintf(w, " = %s;\n", alias.Type.Name)

	return nil
}

func (f *RustFormatter) formatParameters(w io.Writer, params []ir.Parameter) {
	for i, param := range params {
		if i > 0 {
			fmt.Fprintf(w, ", ")
		}

		// Check for self parameter - must be exact match or with reference
		if param.Name == "self" || param.Name == "&self" || param.Name == "&mut self" {
			// self, &self, &mut self are stored in Name
			fmt.Fprintf(w, "%s", param.Name)
		} else {
			// Regular parameter
			if param.Type.Name != "" {
				fmt.Fprintf(w, "%s: %s", param.Name, param.Type.Name)
			} else {
				// Edge case: parameter without type (shouldn't happen in valid Rust)
				fmt.Fprintf(w, "%s", param.Name)
			}
		}
	}
}

// hasModifier reports whether modifiers holds m.
func hasModifier(modifiers []ir.Modifier, m ir.Modifier) bool {
	for _, mod := range modifiers {
		if mod == m {
			return true
		}
	}
	return false
}

// getRustVisibilityKeyword returns the keyword of visibility, as written in
// the source: pub(in path) when base records a path, otherwise pub(super).
func (f *RustFormatter) getRustVisibilityKeyword(visibility ir.Visibility, base *ir.BaseNode) string {
	switch visibility {
	case ir.VisibilityPrivate:
		return "" // No keyword for private in Rust
	case ir.VisibilityProtected:
		if path := rustExtensions(base).VisibilityPath; path != "" {
			return "pub(in " + path + ")"
		}
		return "pub(super)"
	case ir.VisibilityPublic:
		return "pub"
	case ir.VisibilityInternal:
		return "pub(crate)"
	default:
		return ""
	}
}
