package formatter

import (
	"fmt"
	"io"
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
		return f.formatFunction(w, n, indentStr)
	case *ir.DistilledField:
		return f.formatField(w, n, indentStr)
	case *ir.DistilledTypeAlias:
		return f.formatTypeAlias(w, n, indentStr)
	case *ir.DistilledComment:
		return f.formatComment(w, n, indentStr)
	default:
		// Skip unknown nodes
		return nil
	}
}

func (f *RustFormatter) formatImport(w io.Writer, imp *ir.DistilledImport, indent string) error {
	// Rust uses "use" for imports
	if len(imp.Symbols) > 0 {
		symbols := make([]string, len(imp.Symbols))
		for i, sym := range imp.Symbols {
			if sym.Alias != "" {
				symbols[i] = fmt.Sprintf("%s as %s", sym.Name, sym.Alias)
			} else {
				symbols[i] = sym.Name
			}
		}
		fmt.Fprintf(w, "%suse %s::{%s};\n", indent, imp.Module, strings.Join(symbols, ", "))
	} else {
		fmt.Fprintf(w, "%suse %s;\n", indent, imp.Module)
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

func (f *RustFormatter) formatStruct(w io.Writer, class *ir.DistilledClass, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Add blank line before struct
	fmt.Fprintln(w)

	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(class.Visibility)

	// Determine the type of declaration based on modifiers and name
	declarationType := "mod" // default
	isStruct := false
	isEnum := false
	isTrait := false
	isImpl := false

	// Check modifiers
	for _, mod := range class.Modifiers {
		if mod == ir.ModifierStruct {
			declarationType = "struct"
			isStruct = true
			break
		} else if mod == ir.ModifierAbstract {
			declarationType = "trait"
			isTrait = true
			break
		} else if mod == ir.ModifierEnum {
			declarationType = "enum"
			isEnum = true
			break
		}
	}

	// Check if it's an enum based on children (all fields without types)
	// This is a fallback for when ModifierEnum is not set
	if !isStruct && !isTrait && !isEnum && len(class.Children) > 0 {
		allFieldsNoType := true
		for _, child := range class.Children {
			if field, ok := child.(*ir.DistilledField); ok {
				// Enum variants might have tuple types like "(String)"
				if field.Type != nil && field.Type.Name != "" &&
					!strings.HasPrefix(field.Type.Name, "(") &&
					field.Type.Name != "struct" {
					allFieldsNoType = false
					break
				}
			}
		}
		if allFieldsNoType {
			declarationType = "enum"
			isEnum = true
		}
	}

	// Check if it's an impl block
	if strings.HasPrefix(class.Name, "impl ") {
		declarationType = ""
		isImpl = true
	}

	// Format declaration
	if isImpl {
		fmt.Fprintf(w, "%s%s", indentStr, class.Name) // impl blocks already have "impl" in name
	} else if declarationType != "" {
		if visKeyword != "" {
			fmt.Fprintf(w, "%s%s %s %s", indentStr, visKeyword, declarationType, class.Name)
		} else {
			fmt.Fprintf(w, "%s%s %s", indentStr, declarationType, class.Name)
		}
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

	// Handle different declaration types
	if isImpl || isTrait {
		// Impl blocks and traits always have a body
		fmt.Fprintln(w, " {")
		for _, child := range class.Children {
			switch n := child.(type) {
			case *ir.DistilledField:
				if isTrait {
					// Associated types in traits
					fmt.Fprintf(w, "%s    type %s", indentStr, n.Name)
					if n.Type != nil && n.Type.Name != "" && n.Type.Name != "type" {
						fmt.Fprintf(w, ": %s", n.Type.Name)
					}
					fmt.Fprintln(w, ";")
				} else {
					f.formatField(w, n, indentStr+"    ")
				}
			case *ir.DistilledFunction:
				f.formatFunction(w, n, indentStr+"    ")
			case *ir.DistilledComment:
				f.formatComment(w, n, indentStr+"    ")
			}
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else if isEnum {
		// Format enum variants
		if len(class.Children) > 0 {
			fmt.Fprintln(w, " {")
			for _, child := range class.Children {
				if field, ok := child.(*ir.DistilledField); ok {
					fmt.Fprintf(w, "%s    %s", indentStr, field.Name)
					if field.Type != nil && field.Type.Name != "" {
						if strings.HasPrefix(field.Type.Name, "(") {
							// Tuple variant
							fmt.Fprint(w, field.Type.Name)
						} else if field.Type.Name == "struct" {
							// Struct variant
							fmt.Fprintf(w, " { /* fields */ }")
						}
					}
					fmt.Fprintln(w, ",")
				}
			}
			fmt.Fprintf(w, "%s}\n", indentStr)
		} else {
			fmt.Fprintln(w, ";")
		}
	} else if declarationType == "mod" {
		// Modules have a body with various items
		if len(class.Children) > 0 {
			fmt.Fprintln(w, " {")
			// Format all children in the module
			for _, child := range class.Children {
				switch n := child.(type) {
				case *ir.DistilledField:
					f.formatField(w, n, indentStr+"    ")
				case *ir.DistilledFunction:
					f.formatFunction(w, n, indentStr+"    ")
				case *ir.DistilledComment:
					f.formatComment(w, n, indentStr+"    ")
				}
			}
			fmt.Fprintf(w, "%s}\n", indentStr)
		} else {
			fmt.Fprintln(w, ";")
		}
	} else if isStruct {
		// Regular struct handling
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

	return nil
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

func (f *RustFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent string) error {
	// Check for async
	isAsync := false
	isTrait := false
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAsync {
			isAsync = true
		} else if mod == ir.ModifierAbstract {
			isTrait = true
		}
	}

	// Get visibility keyword - trait methods don't use pub
	visKeyword := ""
	if !isTrait {
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
		fmt.Fprintln(w, " {")
		impl := strings.TrimSpace(fn.Implementation)
		for _, line := range strings.Split(impl, "\n") {
			fmt.Fprintf(w, "%s    %s\n", indent, line)
		}
		fmt.Fprintf(w, "%s}\n", indent)
	} else {
		fmt.Fprintln(w)
	}

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
		fmt.Fprintln(w, " {")
		impl := strings.TrimSpace(fn.Implementation)
		for _, line := range strings.Split(impl, "\n") {
			fmt.Fprintf(w, "%s    %s\n", indentStr, line)
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
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
		fmt.Fprintln(w, " {")
		impl := strings.TrimSpace(fn.Implementation)
		for _, line := range strings.Split(impl, "\n") {
			fmt.Fprintf(w, "%s    %s\n", indentStr, line)
		}
		fmt.Fprintf(w, "%s}\n", indentStr)
	} else {
		fmt.Fprintln(w, ";")
	}

	return nil
}

func (f *RustFormatter) formatField(w io.Writer, field *ir.DistilledField, indent string) error {
	// Get visibility keyword
	visKeyword := f.getRustVisibilityKeyword(field.Visibility)

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
			fmt.Fprintf(w, "\n%s%s const %s: %s", indent, visKeyword, strings.ToUpper(field.Name), typeName)
		} else {
			fmt.Fprintf(w, "\n%sconst %s: %s", indent, strings.ToUpper(field.Name), typeName)
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
