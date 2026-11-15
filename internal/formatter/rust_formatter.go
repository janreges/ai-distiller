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

// FormatNode implements LanguageFormatter
func (f *RustFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
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
// otherwise appear to document the next item.
func (f *RustFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
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
		if err := f.FormatNode(w, node, indent); err != nil {
			return err
		}
	}
	return nil
}

// printsNothing reports whether node prints nothing: an extern block without
// members.
func printsNothing(node ir.DistilledNode) bool {
	class, ok := node.(*ir.DistilledClass)
	return ok && rustItemKind(class) == "extern" && len(class.Children) == 0
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

func (f *RustFormatter) formatComment(w io.Writer, comment *ir.DistilledComment, indent string) error {
	lines := strings.Split(comment.Text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "///") || strings.HasPrefix(line, "//!") {
			// Doc comments
			fmt.Fprintf(w, "%s%s\n", indent, line)
		} else if line != "" {
			fmt.Fprintf(w, "%s// %s\n", indent, line)
		} else {
			fmt.Fprintf(w, "%s//\n", indent)
		}
	}
	return nil
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
	fmt.Fprint(w, indentStr)
	switch kind {
	case "impl", "extern":
		fmt.Fprint(w, class.Name)
	default:
		if visKeyword := f.getRustVisibilityKeyword(class.Visibility); visKeyword != "" {
			fmt.Fprintf(w, "%s ", visKeyword)
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
		f.formatMembers(w, class, kind, indent)
	case "enum":
		f.formatEnumBody(w, class, indentStr)
	case "mod":
		// A module prints every item it holds; one without any is "mod x;".
		if len(class.Children) == 0 {
			fmt.Fprintln(w, ";")
			return nil
		}
		fmt.Fprintln(w, " {")
		if err := f.FormatNodes(w, class.Children, indent+1); err != nil {
			return err
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	case "struct", "union":
		f.formatStructBody(w, class, indent)
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
		if visKeyword := f.getRustVisibilityKeyword(field.Visibility); visKeyword != "" {
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
func (f *RustFormatter) formatMembers(w io.Writer, class *ir.DistilledClass, kind string, indent int) {
	indentStr := strings.Repeat("    ", indent)
	memberIndent := indentStr + "    "
	traitMembers := kind == "trait" || strings.Contains(class.Name, " for ")

	fmt.Fprintln(w, " {")
	for _, child := range class.Children {
		switch n := child.(type) {
		case *ir.DistilledField:
			if kind == "trait" && !hasModifier(n.Modifiers, ir.ModifierFinal) {
				fmt.Fprintf(w, "%stype %s", memberIndent, n.Name)
				if n.Type != nil && n.Type.Name != "" && n.Type.Name != "type" {
					fmt.Fprintf(w, ": %s", n.Type.Name)
				}
				fmt.Fprintln(w, ";")
			} else {
				f.formatFieldWithVisibility(w, n, memberIndent, !traitMembers)
			}
		case *ir.DistilledFunction:
			withVisibility := !traitMembers && (kind == "extern" || !hasModifier(n.Modifiers, ir.ModifierAbstract))
			f.formatFunction(w, n, memberIndent, withVisibility)
		default:
			f.FormatNode(w, child, indent+1)
		}
	}
	fmt.Fprintf(w, "%s}\n", indentStr)
}

// formatEnumBody writes the variants of an enum, one per line, or ";" for an
// enum without any.
func (f *RustFormatter) formatEnumBody(w io.Writer, class *ir.DistilledClass, indentStr string) {
	if len(class.Children) == 0 {
		fmt.Fprintln(w, ";")
		return
	}
	fmt.Fprintln(w, " {")
	for _, child := range class.Children {
		field, ok := child.(*ir.DistilledField)
		if !ok {
			continue
		}
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
}

// formatStructBody writes the named fields of a struct or union, or ";" when
// none is left, and the methods of a struct in an impl block after it.
func (f *RustFormatter) formatStructBody(w io.Writer, class *ir.DistilledClass, indent int) {
	indentStr := strings.Repeat("    ", indent)
	hasFields := false
	for _, child := range class.Children {
		if _, ok := child.(*ir.DistilledField); ok {
			hasFields = true
			break
		}
	}

	if hasFields {
		fmt.Fprintln(w, " {")
		for _, child := range class.Children {
			if field, ok := child.(*ir.DistilledField); ok {
				f.formatStructField(w, field, indent+1)
			}
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
			f.formatImplMethod(w, method, indent+1)
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	}
}

func (f *RustFormatter) formatStructField(w io.Writer, field *ir.DistilledField, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(field.Visibility)

	typeName := ""
	if field.Type != nil {
		typeName = field.Type.Name
	}
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
	visKeyword := f.getRustVisibilityKeyword(trait.Visibility)

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

	// Check for async
	isAsync := false
	isAbstract := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			isAsync = true
		} else if mod == ir.ModifierAbstract {
			isAbstract = true
		}
	}

	visKeyword := ""
	if withVisibility {
		visKeyword = f.getRustVisibilityKeyword(fn.Visibility)
	}

	// Format function signature
	if visKeyword != "" {
		if isAsync {
			fmt.Fprintf(w, "\n%s%s async fn %s", indent, visKeyword, fn.Name)
		} else {
			fmt.Fprintf(w, "\n%s%s fn %s", indent, visKeyword, fn.Name)
		}
	} else {
		if isAsync {
			fmt.Fprintf(w, "\n%sasync fn %s", indent, fn.Name)
		} else {
			fmt.Fprintf(w, "\n%sfn %s", indent, fn.Name)
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
	fmt.Fprintf(w, "\n%smacro_rules! %s", indent, macro.Name)
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
	visKeyword := f.getRustVisibilityKeyword(fn.Visibility)

	// Check for async
	isAsync := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			isAsync = true
			break
		}
	}

	// Format method signature
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
		visKeyword = f.getRustVisibilityKeyword(field.Visibility)
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
	if isTypeAlias {
		if visKeyword != "" {
			fmt.Fprintf(w, "\n%s%s type %s", indent, visKeyword, field.Name)
		} else {
			fmt.Fprintf(w, "\n%stype %s", indent, field.Name)
		}
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, " = %s", field.Type.Name)
		}
	} else if isConst {
		if visKeyword != "" {
			fmt.Fprintf(w, "\n%s%s const %s: %s", indent, visKeyword, field.Name, typeName)
		} else {
			fmt.Fprintf(w, "\n%sconst %s: %s", indent, field.Name, typeName)
		}
	} else if isStatic {
		if visKeyword != "" {
			fmt.Fprintf(w, "\n%s%s static %s: %s", indent, visKeyword, field.Name, typeName)
		} else {
			fmt.Fprintf(w, "\n%sstatic %s: %s", indent, field.Name, typeName)
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
	visKeyword := f.getRustVisibilityKeyword(alias.Visibility)

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

func (f *RustFormatter) getRustVisibilityKeyword(visibility ir.Visibility) string {
	switch visibility {
	case ir.VisibilityPrivate:
		return "" // No keyword for private in Rust
	case ir.VisibilityProtected:
		return "pub(super)"
	case ir.VisibilityPublic:
		return "pub"
	case ir.VisibilityInternal:
		return "pub(crate)"
	default:
		return ""
	}
}
