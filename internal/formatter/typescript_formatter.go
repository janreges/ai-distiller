package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// TypeScriptFormatter is a language-specific formatter for TypeScript
type TypeScriptFormatter struct {
	BaseLanguageFormatter
}

// NewTypeScriptFormatter creates a new TypeScript formatter
func NewTypeScriptFormatter() *TypeScriptFormatter {
	return &TypeScriptFormatter{}
}

// FormatNode formats a TypeScript node
func (f *TypeScriptFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	switch n := node.(type) {
	case *ir.DistilledImport:
		return f.formatImport(w, n)
	case *ir.DistilledClass:
		return f.formatClass(w, n, indent)
	case *ir.DistilledInterface:
		return f.formatInterface(w, n, indent)
	case *ir.DistilledFunction:
		return f.formatFunction(w, n, indent)
	case *ir.DistilledField:
		return f.formatField(w, n, indent)
	case *ir.DistilledTypeAlias:
		return f.formatTypeAlias(w, n)
	default:
		// Fallback for unknown nodes
		reportUnhandled("typescript", node)
		return nil
	}
}

func (f *TypeScriptFormatter) formatImport(w io.Writer, imp *ir.DistilledImport) error {
	if imp.ImportType == "from" {
		symbols := make([]string, len(imp.Symbols))
		for i, sym := range imp.Symbols {
			if sym.Alias != "" {
				symbols[i] = fmt.Sprintf("%s as %s", sym.Name, sym.Alias)
			} else {
				symbols[i] = sym.Name
			}
		}
		fmt.Fprintf(w, "import { %s } from '%s'\n", strings.Join(symbols, ", "), imp.Module)
	} else {
		fmt.Fprintf(w, "import '%s'\n", imp.Module)
	}
	return nil
}

func (f *TypeScriptFormatter) formatClass(w io.Writer, class *ir.DistilledClass, indent int) error {
	// Format class declaration
	modifiers := ""
	hasExport := false
	for _, mod := range class.Modifiers {
		if mod == ir.ModifierExport {
			hasExport = true
		} else if mod == ir.ModifierAbstract {
			modifiers += "abstract "
		}
	}

	if hasExport {
		fmt.Fprintf(w, "\nexport %sclass %s", modifiers, class.Name)
	} else {
		fmt.Fprintf(w, "\n%sclass %s", modifiers, class.Name)
	}

	// Add generic type parameters
	if len(class.TypeParams) > 0 {
		typeParams := make([]string, len(class.TypeParams))
		for i, param := range class.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += " extends " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Add extends clause
	if len(class.Extends) > 0 {
		extends := make([]string, len(class.Extends))
		for i, ext := range class.Extends {
			extends[i] = ext.Name
		}
		fmt.Fprintf(w, " extends %s", strings.Join(extends, ", "))
	}

	// Add implements clause
	if len(class.Implements) > 0 {
		implements := make([]string, len(class.Implements))
		for i, impl := range class.Implements {
			implements[i] = impl.Name
		}
		fmt.Fprintf(w, " implements %s", strings.Join(implements, ", "))
	}

	fmt.Fprintln(w, " {")

	// Format class members
	for _, child := range class.Children {
		f.FormatNode(w, child, indent+1)
	}

	fmt.Fprintln(w, "}")

	return nil
}

func (f *TypeScriptFormatter) formatInterface(w io.Writer, intf *ir.DistilledInterface, indent int) error {
	hasExport := false
	for _, mod := range intf.Modifiers {
		if mod == ir.ModifierExport {
			hasExport = true
			break
		}
	}

	if hasExport {
		fmt.Fprintf(w, "\nexport interface %s", intf.Name)
	} else {
		fmt.Fprintf(w, "\ninterface %s", intf.Name)
	}

	// Add generic type parameters
	if len(intf.TypeParams) > 0 {
		typeParams := make([]string, len(intf.TypeParams))
		for i, param := range intf.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += " extends " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Add extends clause
	if len(intf.Extends) > 0 {
		extends := make([]string, len(intf.Extends))
		for i, ext := range intf.Extends {
			extends[i] = ext.Name
		}
		fmt.Fprintf(w, " extends %s", strings.Join(extends, ", "))
	}

	fmt.Fprintln(w, " {")

	// Format interface members
	for _, child := range intf.Children {
		switch c := child.(type) {
		case *ir.DistilledField:
			// Interface properties
			fmt.Fprintf(w, "    property %s", c.Name)
			if c.Type != nil && c.Type.Name != "" {
				fmt.Fprintf(w, ": %s", c.Type.Name)
			}
			fmt.Fprintln(w)
		case *ir.DistilledFunction:
			// Interface methods
			fmt.Fprintf(w, "    method %s", c.Name)

			// Add generic type parameters if any
			if len(c.TypeParams) > 0 {
				typeParams := make([]string, len(c.TypeParams))
				for i, param := range c.TypeParams {
					typeParams[i] = param.Name
					if len(param.Constraints) > 0 {
						typeParams[i] += " extends " + param.Constraints[0].Name
					}
				}
				fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
			}

			// Parameters
			fmt.Fprintf(w, "(")
			params := make([]string, 0, len(c.Parameters))
			for _, param := range c.Parameters {
				if param.Name == "" {
					continue
				}
				paramStr := param.Name
				if param.Type.Name != "" {
					paramStr += ": " + param.Type.Name
				}
				params = append(params, paramStr)
			}
			fmt.Fprintf(w, "%s)", strings.Join(params, ", "))

			// Return type
			if c.Returns != nil && c.Returns.Name != "" {
				fmt.Fprintf(w, ": %s", c.Returns.Name)
			}
			fmt.Fprintln(w)
		}
	}

	fmt.Fprintln(w, "}")

	return nil
}

func (f *TypeScriptFormatter) formatFunction(w io.Writer, fn *ir.DistilledFunction, indent int) error {
	indentStr := strings.Repeat("    ", indent)

	// Format decorators/annotations
	for _, dec := range fn.Decorators {
		fmt.Fprintf(w, "%s@%s\n", indentStr, dec)
	}

	modifiers := ""
	isConst := false
	hasExport := false

	// Format visibility keyword (only for class methods, not top-level functions)
	if indent > 0 {
		visKeyword := f.getTypeScriptVisibilityKeyword(fn.Visibility)
		if visKeyword != "" {
			modifiers = visKeyword + " " + modifiers
		}
	}
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierExport {
			hasExport = true
		} else if mod == ir.ModifierAbstract {
			modifiers += "abstract "
		} else if mod == ir.ModifierAsync {
			modifiers += "async "
		} else if mod == ir.ModifierStatic {
			modifiers += "static "
		} else if mod == ir.ModifierFinal {
			isConst = true
		}
	}

	// Top-level functions don't have indentation
	if indent == 0 {
		indentStr = ""
		exportPrefix := ""
		if hasExport {
			exportPrefix = "export "
		}
		// Top-level const arrow functions should be formatted as "const Name"
		if isConst {
			fmt.Fprintf(w, "%s%sconst %s", indentStr, exportPrefix, fn.Name)
		} else {
			fmt.Fprintf(w, "%s%s%sfunction %s", indentStr, exportPrefix, modifiers, fn.Name)
		}
	} else {
		// Methods inside classes/interfaces - no "function" keyword needed
		fmt.Fprintf(w, "%s%s%s", indentStr, modifiers, fn.Name)
	}

	// Add generic type parameters
	if len(fn.TypeParams) > 0 {
		typeParams := make([]string, len(fn.TypeParams))
		for i, param := range fn.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += " extends " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	// Parameters
	fmt.Fprintf(w, "(")
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

	// Return type
	if fn.Returns != nil && fn.Returns.Name != "" {
		fmt.Fprintf(w, ": %s", fn.Returns.Name)
	}

	fmt.Fprintln(w)

	// Implementation
	if fn.Implementation != "" {
		fmt.Fprintf(w, "%s    // implementation\n", indentStr)
	}

	return nil
}

func (f *TypeScriptFormatter) formatField(w io.Writer, field *ir.DistilledField, indent int) error {
	// Handle top-level const/let/var declarations differently from class fields
	if indent == 0 {
		// Top-level variable declaration
		varType := "let"
		hasExport := false
		for _, mod := range field.Modifiers {
			if mod == ir.ModifierFinal {
				varType = "const"
			} else if mod == ir.ModifierExport {
				hasExport = true
			}
		}

		exportPrefix := ""
		if hasExport {
			exportPrefix = "export "
		}

		fmt.Fprintf(w, "%s%s %s", exportPrefix, varType, field.Name)
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, ": %s", field.Type.Name)
		}
		if field.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", field.DefaultValue)
		}
		fmt.Fprintln(w)
	} else {
		// Class field - use visibility keywords
		visKeyword := f.getTypeScriptVisibilityKeyword(field.Visibility)

		modifiers := ""
		if visKeyword != "" {
			modifiers = visKeyword + " "
		}
		for _, mod := range field.Modifiers {
			if mod == ir.ModifierReadonly {
				modifiers += "readonly "
			} else if mod == ir.ModifierStatic {
				modifiers += "static "
			}
		}

		fmt.Fprintf(w, "    %s%s", modifiers, field.Name)
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, ": %s", field.Type.Name)
		}
		if field.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", field.DefaultValue)
		}
		fmt.Fprintln(w)
	}

	return nil
}

func (f *TypeScriptFormatter) formatTypeAlias(w io.Writer, alias *ir.DistilledTypeAlias) error {
	hasExport := false
	for _, mod := range alias.Modifiers {
		if mod == ir.ModifierExport {
			hasExport = true
			break
		}
	}

	if hasExport {
		fmt.Fprintf(w, "export type %s", alias.Name)
	} else {
		fmt.Fprintf(w, "type %s", alias.Name)
	}

	// Add generic type parameters
	if len(alias.TypeParams) > 0 {
		typeParams := make([]string, len(alias.TypeParams))
		for i, param := range alias.TypeParams {
			typeParams[i] = param.Name
			if len(param.Constraints) > 0 {
				typeParams[i] += " extends " + param.Constraints[0].Name
			}
		}
		fmt.Fprintf(w, "<%s>", strings.Join(typeParams, ", "))
	}

	fmt.Fprintf(w, " = %s\n", alias.Type.Name)

	return nil
}

// getTypeScriptVisibilityKeyword returns the TypeScript visibility keyword for the given visibility
func (f *TypeScriptFormatter) getTypeScriptVisibilityKeyword(visibility ir.Visibility) string {
	switch visibility {
	case ir.VisibilityPublic:
		return "public"
	case ir.VisibilityPrivate:
		return "private"
	case ir.VisibilityProtected:
		return "protected"
	case ir.VisibilityInternal:
		// TypeScript doesn't have internal, use protected
		return "protected"
	default:
		return ""
	}
}
