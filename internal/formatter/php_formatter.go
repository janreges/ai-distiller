package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// PHPFormatter formats IR nodes as PHP code
type PHPFormatter struct {
	BaseLanguageFormatter
}

// NewPHPFormatter creates a new PHP formatter
func NewPHPFormatter() *PHPFormatter {
	return &PHPFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("php"),
	}
}

// FormatNode formats an IR node as PHP code
func (f *PHPFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	switch n := node.(type) {
	case *ir.DistilledComment:
		_, err := fmt.Fprintln(w, f.formatComment(n, indent))
		return err
	case *ir.DistilledImport:
		_, err := fmt.Fprintln(w, f.formatImport(n))
		return err
	case *ir.DistilledClass:
		_, err := fmt.Fprintln(w, f.formatClass(n, indent))
		return err
	case *ir.DistilledInterface:
		_, err := fmt.Fprintln(w, f.formatInterface(n, indent))
		return err
	case *ir.DistilledEnum:
		_, err := fmt.Fprintln(w, f.formatEnum(n, indent))
		return err
	case *ir.DistilledFunction:
		_, err := fmt.Fprintln(w, f.formatFunction(n, indent))
		return err
	case *ir.DistilledField:
		// Top-level fields in PHP are constants or global variables
		if indent == 0 {
			_, err := fmt.Fprintln(w, f.formatGlobalField(n))
			return err
		}
		_, err := fmt.Fprintln(w, f.formatField(n, indent))
		return err
	default:
		// Report the node, then process any children recursively
		reportUnhandled("php", node)
		children := node.GetChildren()
		for _, child := range children {
			if err := f.FormatNode(w, child, indent); err != nil {
				return err
			}
		}
		return nil
	}
}

func (f *PHPFormatter) formatImport(imp *ir.DistilledImport) string {
	if imp.ImportType == "namespace" {
		return fmt.Sprintf("namespace %s;", imp.Module)
	}

	// Use statement with alias
	if len(imp.Symbols) == 1 && imp.Symbols[0].Alias != "" {
		return fmt.Sprintf("use %s\\%s as %s;", imp.Module, imp.Symbols[0].Name, imp.Symbols[0].Alias)
	}

	// Check for function/const imports
	if strings.Contains(imp.Module, "function ") {
		return fmt.Sprintf("use %s;", imp.Module)
	}
	if strings.Contains(imp.Module, "const ") {
		return fmt.Sprintf("use %s;", imp.Module)
	}

	return fmt.Sprintf("use %s;", imp.Module)
}

func (f *PHPFormatter) formatClass(class *ir.DistilledClass, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	var parts []string

	// Format decorators/attributes first
	for _, dec := range class.Decorators {
		parts = append(parts, fmt.Sprintf("%s#[%s]", indentStr, dec))
	}

	// Format API docblock if present
	if class.APIDocblock != "" {
		// Split and indent each line of the docblock
		lines := strings.Split(class.APIDocblock, "\n")
		for _, line := range lines {
			parts = append(parts, indentStr+line)
		}
	}

	// Check if this is an enum or trait
	isEnum := class.Extensions != nil && class.Extensions.PHP != nil && class.Extensions.PHP.IsEnum
	isTrait := class.Extensions != nil && class.Extensions.PHP != nil && class.Extensions.PHP.IsTrait

	// Class modifiers (but not for enums or traits)
	modifiers := []string{}
	if !isEnum && !isTrait {
		for _, mod := range class.Modifiers {
			if mod == ir.ModifierAbstract {
				modifiers = append(modifiers, "abstract")
			} else if mod == ir.ModifierFinal {
				modifiers = append(modifiers, "final")
			}
		}
	}

	// Class type
	classType := "class"
	if isEnum {
		classType = "enum"
	} else if isTrait {
		classType = "trait"
	}

	// Class declaration
	classDecl := ""
	if len(modifiers) > 0 {
		classDecl = strings.Join(modifiers, " ") + " "
	}
	classDecl += classType + " " + class.Name

	// Enum backing type
	if isEnum && class.Extensions.PHP.EnumBackingType != "" {
		classDecl += ": " + class.Extensions.PHP.EnumBackingType
	}

	// Inheritance
	if len(class.Extends) > 0 {
		// Classes can only extend one class
		classDecl += " extends " + class.Extends[0].Name
	}

	// Implements
	if len(class.Implements) > 0 {
		implements := []string{}
		for _, impl := range class.Implements {
			implements = append(implements, impl.Name)
		}
		classDecl += " implements " + strings.Join(implements, ", ")
	}

	parts = append(parts, indentStr+classDecl+" {")

	// Format trait usage
	if len(class.Mixins) > 0 && !isTrait {
		for _, trait := range class.Mixins {
			parts = append(parts, fmt.Sprintf("%s    use %s;", indentStr, trait.Name))
		}
	}

	// Format children (methods, properties, etc.)
	for _, child := range class.Children {
		switch n := child.(type) {
		case *ir.DistilledFunction:
			// Skip common magic methods - they're never called directly
			// https://www.php.net/manual/en/language.oop5.magic.php
			magicMethods := map[string]bool{
				"__construct":   false, // Keep this - constructor is important
				"__destruct":    true,  // Destructor - rarely needed
				"__get":         true,  // Property getter
				"__set":         true,  // Property setter
				"__isset":       true,  // isset() on inaccessible properties
				"__unset":       true,  // unset() on inaccessible properties
				"__call":        true,  // Method calls on inaccessible methods
				"__callStatic":  true,  // Static method calls on inaccessible methods
				"__sleep":       true,  // serialize() behavior
				"__wakeup":      true,  // unserialize() behavior
				"__serialize":   true,  // serialize() behavior (PHP 7.4+)
				"__unserialize": true,  // unserialize() behavior (PHP 7.4+)
				"__toString":    false, // Keep this - it defines string representation
				"__invoke":      false, // Keep this - makes object callable
				"__set_state":   true,  // var_export() behavior
				"__clone":       false, // Keep this - defines cloning behavior
				"__debugInfo":   true,  // var_dump() behavior
			}

			if skip, found := magicMethods[n.Name]; found && skip {
				continue
			}

			// Special case: skip __construct if it has no parameters
			if n.Name == "__construct" && len(n.Parameters) == 0 {
				continue
			}

			parts = append(parts, f.formatFunction(n, indent+1))
		case *ir.DistilledField:
			// Skip virtual fields from docblock - they're already shown in the docblock
			if n.Extensions != nil && n.Extensions.PHP != nil && n.Extensions.PHP.Origin == ir.FieldOriginDocblock {
				continue
			}
			parts = append(parts, f.formatField(n, indent+1))
		}
	}

	// Add closing brace
	parts = append(parts, indentStr+"}")

	return strings.Join(parts, "\n")
}

func (f *PHPFormatter) formatInterface(intf *ir.DistilledInterface, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	var parts []string

	// Interface declaration
	intfDecl := "interface " + intf.Name

	// Extends
	if len(intf.Extends) > 0 {
		extends := []string{}
		for _, ext := range intf.Extends {
			extends = append(extends, ext.Name)
		}
		intfDecl += " extends " + strings.Join(extends, ", ")
	}

	parts = append(parts, indentStr+intfDecl+" {")

	// Format children (methods)
	for _, child := range intf.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok {
			parts = append(parts, f.formatFunction(fn, indent+1))
		}
	}

	// Add closing brace
	parts = append(parts, indentStr+"}")

	return strings.Join(parts, "\n")
}

func (f *PHPFormatter) formatEnum(enum *ir.DistilledEnum, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Enum declaration (PHP 8.1+)
	enumDecl := "enum " + enum.Name

	// Base type (backed enum)
	if enum.Type != nil && enum.Type.Name != "" {
		enumDecl += ": " + enum.Type.Name
	}

	var parts []string
	parts = append(parts, indentStr+enumDecl+" {")

	// Format enum members and methods
	for _, child := range enum.Children {
		switch n := child.(type) {
		case *ir.DistilledFunction:
			parts = append(parts, f.formatFunction(n, indent+1))
		case *ir.DistilledField:
			// Enum cases are represented as fields
			caseStr := indentStr + "    case " + n.Name
			if n.DefaultValue != "" {
				caseStr += " = " + n.DefaultValue
			}
			caseStr += ";"
			parts = append(parts, caseStr)
		}
	}

	// Add closing brace
	parts = append(parts, indentStr+"}")

	return strings.Join(parts, "\n")
}

func (f *PHPFormatter) formatFunction(fn *ir.DistilledFunction, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	result := ""

	// Format decorators/attributes first
	for _, dec := range fn.Decorators {
		result += fmt.Sprintf("%s#[%s]\n", indentStr, dec)
	}

	// Check modifiers (but not visibility - that's handled by visibility prefix)
	modifiers := []string{}
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierAbstract {
			modifiers = append(modifiers, "abstract")
		} else if mod == ir.ModifierFinal {
			modifiers = append(modifiers, "final")
		}
	}

	// Function signature
	signature := ""
	if len(modifiers) > 0 {
		signature = strings.Join(modifiers, " ") + " "
	}
	signature += fn.Name

	// Parameters
	params := []string{}
	for _, p := range fn.Parameters {
		param := ""

		// Type hint
		if p.Type.Name != "" {
			typeName := p.Type.Name
			// Handle nullable types
			if p.IsOptional && !strings.HasPrefix(typeName, "?") {
				typeName = "?" + typeName
			}
			param = typeName + " "
		}

		// Parameter name (PHP variables start with $)
		paramName := p.Name
		if !strings.HasPrefix(paramName, "$") {
			paramName = "$" + paramName
		}
		param += paramName

		// Default value
		if p.DefaultValue != "" {
			param += " = " + p.DefaultValue
		}

		params = append(params, param)
	}
	signature += "(" + strings.Join(params, ", ") + ")"

	// Return type
	if fn.Returns != nil {
		returnType := fn.Returns.Name
		if returnType == "never" {
			returnType = "never"
		} else if returnType == "" {
			returnType = "void"
		}
		signature += ": " + returnType
	}

	// Add implementation if present
	if fn.Implementation != "" {
		// Check if implementation is just empty braces
		impl := strings.TrimSpace(fn.Implementation)
		if impl == "{}" || impl == "{\n}" {
			// Empty implementation - don't add anything
		} else {
			signature += " {\n"
			// Strip leading and trailing braces from implementation if present
			lines := strings.Split(fn.Implementation, "\n")

			// Find first and last non-empty lines
			firstNonEmpty := -1
			lastNonEmpty := -1
			for i, line := range lines {
				if strings.TrimSpace(line) != "" {
					if firstNonEmpty == -1 {
						firstNonEmpty = i
					}
					lastNonEmpty = i
				}
			}

			// Check if first and last lines are braces
			if firstNonEmpty >= 0 && lastNonEmpty >= 0 && firstNonEmpty < lastNonEmpty {
				firstLine := strings.TrimSpace(lines[firstNonEmpty])
				lastLine := strings.TrimSpace(lines[lastNonEmpty])
				if firstLine == "{" && lastLine == "}" {
					// Remove brace lines
					lines = lines[firstNonEmpty+1 : lastNonEmpty]
				}
			}

			// Join back and add
			impl = strings.Join(lines, "\n")
			impl = strings.TrimSpace(impl)

			// Add implementation with proper closing
			if impl != "" {
				signature += impl
				if !strings.HasSuffix(impl, "\n") {
					signature += "\n"
				}
			}
			signature += indentStr + "}"
		}
	}

	// Global functions (indent == 0) don't have visibility prefix
	if indent == 0 {
		return result + signature
	}
	// Add visibility keyword
	visKeyword := f.getPHPVisibilityKeyword(fn.Visibility)
	if visKeyword != "" {
		return result + indentStr + visKeyword + " " + signature
	}
	return result + indentStr + signature
}

func (f *PHPFormatter) formatField(field *ir.DistilledField, indent int) string {
	indentStr := strings.Repeat("    ", indent)
	result := ""

	// Format decorators/attributes first
	for _, dec := range field.Decorators {
		result += fmt.Sprintf("%s#[%s]\n", indentStr, dec)
	}

	// Check if this is an enum case
	if field.Extensions != nil && field.Extensions.PHP != nil && field.Extensions.PHP.IsEnumCase {
		// Format as enum case
		caseDecl := "case " + field.Name
		if field.DefaultValue != "" {
			caseDecl += " = " + field.DefaultValue
		}
		return result + indentStr + caseDecl + ";"
	}

	// Check if this is a magic property from PHP docblock
	isPropertyFromDocblock := false
	var accessMode string
	if field.Extensions != nil && field.Extensions.PHP != nil {
		if field.Extensions.PHP.Origin == ir.FieldOriginDocblock {
			isPropertyFromDocblock = true
			switch field.Extensions.PHP.AccessMode {
			case ir.FieldAccessReadOnly:
				accessMode = "property-read "
			case ir.FieldAccessWriteOnly:
				accessMode = "property-write "
			default:
				accessMode = "property "
			}
		}
	}

	// For magic properties, return special format
	if isPropertyFromDocblock {
		fieldDecl := field.Name
		if field.Type != nil && field.Type.Name != "" {
			fieldDecl = field.Type.Name + " $" + field.Name
		} else {
			fieldDecl = "$" + field.Name
		}

		if field.Description != "" {
			fieldDecl += " " + field.Description
		}

		return result + indentStr + accessMode + fieldDecl
	}

	// Check if this is a constant (has both static and final modifiers)
	isConst := false
	hasStatic := false
	hasFinal := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierStatic {
			hasStatic = true
		}
		if mod == ir.ModifierFinal {
			hasFinal = true
		}
	}
	isConst = hasStatic && hasFinal

	// For constants, use const syntax
	if isConst {
		fieldDecl := "const " + field.Name
		if field.DefaultValue != "" {
			fieldDecl += " = " + field.DefaultValue
		}
		fieldDecl += ";"

		// Add visibility keyword
		visKeyword := f.getPHPVisibilityKeyword(field.Visibility)
		if visKeyword != "" {
			return result + indentStr + visKeyword + " " + fieldDecl
		}
		return result + indentStr + fieldDecl
	}

	// Check modifiers (but not visibility - that's handled by visibility prefix)
	modifiers := []string{}
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierReadonly {
			modifiers = append(modifiers, "readonly")
		}
	}

	// Field declaration
	fieldDecl := ""
	if len(modifiers) > 0 {
		fieldDecl = strings.Join(modifiers, " ") + " "
	}

	// Type (PHP 7.4+ typed properties)
	if field.Type != nil && field.Type.Name != "" {
		fieldDecl += field.Type.Name + " "
	}

	// Property name (PHP variables start with $)
	fieldName := field.Name
	if !strings.HasPrefix(fieldName, "$") {
		fieldName = "$" + fieldName
	}
	fieldDecl += fieldName

	// Initializer
	if field.DefaultValue != "" {
		fieldDecl += " = " + field.DefaultValue
	}

	fieldDecl += ";"

	// Add visibility keyword
	visKeyword := f.getPHPVisibilityKeyword(field.Visibility)
	if visKeyword != "" {
		return result + indentStr + visKeyword + " " + fieldDecl
	}
	return result + indentStr + fieldDecl
}

func (f *PHPFormatter) formatGlobalField(field *ir.DistilledField) string {
	// Global fields are usually constants in PHP
	isConst := false
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierStatic || mod == ir.ModifierFinal {
			isConst = true
			break
		}
	}

	fieldDecl := ""
	if isConst {
		fieldDecl = "const " + field.Name
	} else {
		// Global variable with $ prefix
		fieldName := field.Name
		if !strings.HasPrefix(fieldName, "$") {
			fieldName = "$" + fieldName
		}
		fieldDecl = fieldName
	}

	// Add value if specified
	if field.DefaultValue != "" {
		fieldDecl += " = " + field.DefaultValue
	}

	return fieldDecl
}

func (f *PHPFormatter) getPHPVisibilityKeyword(visibility ir.Visibility) string {
	switch visibility {
	case ir.VisibilityPublic:
		return "public"
	case ir.VisibilityPrivate:
		return "private"
	case ir.VisibilityProtected:
		return "protected"
	case ir.VisibilityInternal:
		// PHP doesn't have internal, use protected
		return "protected"
	default:
		return "public" // Default is public in PHP
	}
}

// formatComment formats a comment node
func (f *PHPFormatter) formatComment(n *ir.DistilledComment, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	switch n.Format {
	case "docblock", "doc":
		// Format as docblock
		lines := strings.Split(n.Text, "\n")
		if len(lines) == 1 {
			// Single line docblock
			return fmt.Sprintf("%s/** %s */", indentStr, lines[0])
		}
		// Multi-line docblock
		result := indentStr + "/**\n"
		for _, line := range lines {
			if line == "" {
				result += indentStr + " *\n"
			} else {
				result += fmt.Sprintf("%s * %s\n", indentStr, line)
			}
		}
		result += indentStr + " */"
		return result

	case "block":
		// Format as block comment
		lines := strings.Split(n.Text, "\n")
		if len(lines) == 1 {
			// Single line block comment
			return fmt.Sprintf("%s/* %s */", indentStr, lines[0])
		}
		// Multi-line block comment
		result := indentStr + "/*\n"
		for _, line := range lines {
			result += fmt.Sprintf("%s%s\n", indentStr, line)
		}
		result += indentStr + "*/"
		return result

	default:
		// Format as line comment (default)
		lines := strings.Split(n.Text, "\n")
		result := make([]string, len(lines))
		for i, line := range lines {
			result[i] = fmt.Sprintf("%s// %s", indentStr, line)
		}
		return strings.Join(result, "\n")
	}
}
