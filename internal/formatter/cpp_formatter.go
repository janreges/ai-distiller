package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// CppFormatter formats IR nodes as C++ code
type CppFormatter struct {
	BaseLanguageFormatter
}

// NewCppFormatter creates a new C++ formatter
func NewCppFormatter() *CppFormatter {
	return &CppFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("cpp"),
	}
}

// FormatNode formats an IR node as C++ code
func (f *CppFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	switch n := node.(type) {
	case *ir.DistilledImport:
		_, err := fmt.Fprintln(w, f.formatImport(n))
		return err
	case *ir.DistilledClass:
		_, err := fmt.Fprintln(w, f.formatClass(n, indent))
		return err
	case *ir.DistilledStruct:
		_, err := fmt.Fprintln(w, f.formatStruct(n, indent))
		return err
	case *ir.DistilledEnum:
		_, err := fmt.Fprintln(w, f.formatEnum(n, indent))
		return err
	case *ir.DistilledFunction:
		_, err := fmt.Fprintln(w, f.formatFunction(n, indent))
		return err
	case *ir.DistilledField:
		_, err := fmt.Fprintln(w, f.formatField(n, indent))
		return err
	case *ir.DistilledComment:
		writeCStyleComment(w, n, strings.Repeat("    ", indent))
		return nil
	default:
		// Report the node, then process any children recursively
		reportUnhandled("cpp", node)
		return f.FormatNodes(w, node.GetChildren(), indent)
	}
}

// FormatNodes formats sibling nodes. The doc comments of a node the formatter
// does not print (a namespace, whose line is left out while its members are
// printed) are dropped with it.
func (f *CppFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
	parts := formatCStyleNodes(nodes, strings.Repeat("    ", indent), func(node ir.DistilledNode) (string, bool) {
		var b strings.Builder
		_ = f.FormatNode(&b, node, indent) // a strings.Builder does not fail
		switch node.(type) {
		case *ir.DistilledImport, *ir.DistilledClass, *ir.DistilledStruct, *ir.DistilledEnum,
			*ir.DistilledFunction, *ir.DistilledField:
			return strings.TrimSuffix(b.String(), "\n"), true
		}
		return strings.TrimSuffix(b.String(), "\n"), false
	})
	for _, part := range parts {
		if _, err := fmt.Fprintln(w, part); err != nil {
			return err
		}
	}
	return nil
}

// formatMembers formats the members of a class or struct body: methods,
// fields and comments.
func (f *CppFormatter) formatMembers(members []ir.DistilledNode, indent int) []string {
	return formatCStyleNodes(members, strings.Repeat("    ", indent), func(node ir.DistilledNode) (string, bool) {
		switch n := node.(type) {
		case *ir.DistilledFunction:
			return f.formatFunction(n, indent), true
		case *ir.DistilledField:
			return f.formatField(n, indent), true
		}
		return "", false
	})
}

func (f *CppFormatter) formatImport(imp *ir.DistilledImport) string {
	// C++ uses #include
	if strings.HasPrefix(imp.Module, "<") && strings.HasSuffix(imp.Module, ">") {
		return fmt.Sprintf("#include %s", imp.Module)
	} else if strings.HasPrefix(imp.Module, "\"") && strings.HasSuffix(imp.Module, "\"") {
		return fmt.Sprintf("#include %s", imp.Module)
	} else {
		// Assume user header
		return fmt.Sprintf("#include \"%s\"", imp.Module)
	}
}

func (f *CppFormatter) formatClass(class *ir.DistilledClass, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	var parts []string

	// Class type is always "class" for DistilledClass
	classType := "class"

	// Template parameters
	if len(class.TypeParams) > 0 {
		templateParams := []string{}
		for _, g := range class.TypeParams {
			templateParam := "typename " + g.Name
			if len(g.Constraints) > 0 {
				// C++ concepts or requires clause
				templateParam = g.Constraints[0].Name + " " + g.Name
			}
			templateParams = append(templateParams, templateParam)
		}
		parts = append(parts, indentStr+"template<"+strings.Join(templateParams, ", ")+">")
	}

	// Class declaration
	// In C++, visibility is not used on class declarations, only on members
	classDecl := classType + " " + class.Name

	// Inheritance
	if len(class.Extends) > 0 || len(class.Implements) > 0 {
		bases := []string{}
		for _, ext := range class.Extends {
			// Assume public inheritance by default
			base := "public " + ext.Name
			bases = append(bases, base)
		}
		// C++ doesn't distinguish implements, all are base classes
		for _, impl := range class.Implements {
			bases = append(bases, "public "+impl.Name)
		}
		classDecl += " : " + strings.Join(bases, ", ")
	}

	parts = append(parts, indentStr+classDecl+" {")

	// Format children (methods, fields, comments)
	parts = append(parts, f.formatMembers(class.Children, indent+1)...)

	// Closing brace
	parts = append(parts, indentStr+"};")

	return strings.Join(parts, "\n")
}

func (f *CppFormatter) formatStruct(strct *ir.DistilledStruct, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	var parts []string

	// Template parameters
	if len(strct.TypeParams) > 0 {
		templateParams := []string{}
		for _, g := range strct.TypeParams {
			templateParam := "typename " + g.Name
			if len(g.Constraints) > 0 {
				templateParam = g.Constraints[0].Name + " " + g.Name
			}
			templateParams = append(templateParams, templateParam)
		}
		parts = append(parts, indentStr+"template<"+strings.Join(templateParams, ", ")+">")
	}

	// Struct declaration
	// In C++, visibility is not used on struct declarations, only on members
	structDecl := "struct " + strct.Name

	parts = append(parts, indentStr+structDecl+" {")

	// Format children (methods, fields, comments)
	parts = append(parts, f.formatMembers(strct.Children, indent+1)...)

	// Closing brace
	parts = append(parts, indentStr+"};")

	return strings.Join(parts, "\n")
}

func (f *CppFormatter) formatEnum(enum *ir.DistilledEnum, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Enum declaration
	// In C++, visibility is not used on enum declarations, only on members
	enumDecl := "enum class " + enum.Name

	// Base type
	if enum.Type != nil {
		enumDecl += " : " + enum.Type.Name
	}

	var parts []string
	parts = append(parts, indentStr+enumDecl+" {")

	// Format enum values
	for _, child := range enum.Children {
		if field, ok := child.(*ir.DistilledField); ok {
			valueStr := indentStr + "    " + field.Name
			if field.DefaultValue != "" {
				valueStr += " = " + field.DefaultValue
			}
			parts = append(parts, valueStr)
		}
	}

	// Closing brace
	parts = append(parts, indentStr+"};")

	return strings.Join(parts, "\n")
}

func (f *CppFormatter) formatFunction(fn *ir.DistilledFunction, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// C++ doesn't use visibility keywords on standalone functions
	// Access specifiers are used in class/struct scope

	// Template parameters
	var templateLine string
	if len(fn.TypeParams) > 0 {
		templateParams := []string{}
		for _, g := range fn.TypeParams {
			templateParam := "typename " + g.Name
			if len(g.Constraints) > 0 {
				templateParam = g.Constraints[0].Name + " " + g.Name
			}
			templateParams = append(templateParams, templateParam)
		}
		templateLine = indentStr + "template<" + strings.Join(templateParams, ", ") + ">\n"
	}

	// Function modifiers
	modifiers := []string{}
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierVirtual {
			modifiers = append(modifiers, "virtual")
		} else if mod == ir.ModifierInline {
			modifiers = append(modifiers, "inline")
		} else if mod == ir.ModifierExtern {
			modifiers = append(modifiers, "extern")
		}
	}

	// Return type - constructors and destructors don't have return types
	var returnType string
	hasReturnType := true

	// Check if this is a constructor or destructor by examining the parent context
	// In C++, constructors have the same name as the class and no return type
	// Destructors start with ~ and have no return type
	// Since we don't have parent context here, we check if Returns is nil
	if fn.Returns == nil {
		hasReturnType = false
	} else {
		returnType = fn.Returns.Name
	}

	// Function signature
	signature := indentStr
	if len(modifiers) > 0 {
		signature += strings.Join(modifiers, " ") + " "
	}
	if hasReturnType {
		signature += returnType + " "
	}
	signature += fn.Name

	// Parameters
	params := []string{}
	for _, p := range fn.Parameters {
		param := ""
		if p.Type.Name != "" {
			param = p.Type.Name + " " + p.Name
		} else {
			param = "auto " + p.Name
		}
		if p.DefaultValue != "" {
			param += " = " + p.DefaultValue
		}
		params = append(params, param)
	}
	signature += "(" + strings.Join(params, ", ") + ")"

	// Function qualifiers - check modifiers
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierConst {
			signature += " const"
		} else if mod == ir.ModifierOverride {
			signature += " override"
		} else if mod == ir.ModifierFinal {
			signature += " final"
		} else if mod == ir.ModifierAbstract {
			signature += " = 0"
		}
	}

	// A function with a body prints it after the signature; the stripper has
	// already removed bodies that were not requested. A declaration without a
	// body gets a semicolon at top level.
	if fn.Implementation != "" {
		signature += strings.TrimRight(renderBody(fn.Implementation, indentStr), "\n")
	} else if indent == 0 {
		signature += ";"
	}

	if templateLine != "" {
		return templateLine + signature
	}
	return signature
}

func (f *CppFormatter) formatField(field *ir.DistilledField, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Field modifiers
	modifiers := []string{}
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierConst || mod == ir.ModifierReadonly {
			modifiers = append(modifiers, "const")
		} else if mod == ir.ModifierMutable {
			modifiers = append(modifiers, "mutable")
		} else if mod == ir.ModifierVolatile {
			modifiers = append(modifiers, "volatile")
		}
	}

	// Field declaration
	fieldDecl := ""
	if len(modifiers) > 0 {
		fieldDecl = strings.Join(modifiers, " ") + " "
	}

	// Type
	if field.Type != nil && field.Type.Name != "" {
		fieldDecl += field.Type.Name + " "
	} else {
		fieldDecl += "auto "
	}

	fieldDecl += field.Name

	// Initializer
	if field.DefaultValue != "" {
		fieldDecl += " = " + field.DefaultValue
	}

	// Add semicolon
	fieldDecl += ";"

	// In C++, visibility is controlled by access specifier sections (public:, private:, protected:)
	// For text format, we'll just show the field without prefix
	return indentStr + fieldDecl
}
