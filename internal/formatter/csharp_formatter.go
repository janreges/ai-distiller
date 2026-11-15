package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// CSharpFormatter formats IR nodes as C# code
type CSharpFormatter struct {
	BaseLanguageFormatter
}

// NewCSharpFormatter creates a new C# formatter
func NewCSharpFormatter() *CSharpFormatter {
	return &CSharpFormatter{
		BaseLanguageFormatter: NewBaseLanguageFormatter("csharp"),
	}
}

// FormatNode formats an IR node as C# code
func (f *CSharpFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	switch n := node.(type) {
	case *ir.DistilledImport:
		_, err := fmt.Fprintln(w, f.formatImport(n))
		return err
	case *ir.DistilledStruct:
		// Format struct declaration
		structDecl := fmt.Sprintf("%sstruct %s", strings.Repeat("    ", indent), n.Name)
		if len(n.TypeParams) > 0 {
			genericParams := []string{}
			for _, g := range n.TypeParams {
				genericParams = append(genericParams, g.Name)
			}
			structDecl += "<" + strings.Join(genericParams, ", ") + ">"
		}
		_, err := fmt.Fprintln(w, structDecl+" {")
		if err != nil {
			return err
		}
		// Format struct members
		if err := f.FormatNodes(w, n.Children, indent+1); err != nil {
			return err
		}
		// Closing brace
		indentStr := strings.Repeat("    ", indent)
		fmt.Fprintf(w, "%s}\n", indentStr)
		return nil
	case *ir.DistilledClass:
		// Format attributes/decorators
		indentStr := strings.Repeat("    ", indent)
		for _, decorator := range n.Decorators {
			fmt.Fprintf(w, "%s%s\n", indentStr, decorator)
		}
		// Format class declaration
		classDecl := f.formatClass(n, indent)

		// Check if this is a record with only parameters (no body)
		isRecord := false
		onlyProperties := true
		for _, mod := range n.Modifiers {
			if mod == ir.ModifierData {
				isRecord = true
				break
			}
		}

		members := 0
		if isRecord {
			// Check if all members are readonly properties (record parameters);
			// comments are not members.
			for _, child := range n.Children {
				if _, ok := child.(*ir.DistilledComment); ok {
					continue
				}
				members++
				if field, ok := child.(*ir.DistilledField); !ok || !field.IsProperty || field.HasSetter {
					onlyProperties = false
					break
				}
			}
		}

		if isRecord && onlyProperties && members > 0 {
			// Record with only parameters - don't print body
			fmt.Fprintln(w, classDecl+";")
			return nil
		}

		// Regular class or record with body
		_, err := fmt.Fprintln(w, classDecl+" {")
		if err != nil {
			return err
		}
		// Format class members (but skip record parameters - they're already
		// in the declaration)
		err = f.formatSiblings(w, n.Children, indent+1, func(child ir.DistilledNode) bool {
			field, ok := child.(*ir.DistilledField)
			return isRecord && ok && field.IsProperty && !field.HasSetter
		})
		if err != nil {
			return err
		}
		// Closing brace
		fmt.Fprintf(w, "%s}\n", indentStr)
		return nil
	case *ir.DistilledInterface:
		// Format interface declaration
		_, err := fmt.Fprintln(w, f.formatInterface(n, indent)+" {")
		if err != nil {
			return err
		}
		// Format interface members
		if err := f.FormatNodes(w, n.Children, indent+1); err != nil {
			return err
		}
		// Closing brace
		indentStr := strings.Repeat("    ", indent)
		fmt.Fprintf(w, "%s}\n", indentStr)
		return nil
	case *ir.DistilledEnum:
		// Format enum declaration
		_, err := fmt.Fprintln(w, f.formatEnum(n, indent)+" {")
		if err != nil {
			return err
		}
		// Format enum members
		if err := f.FormatNodes(w, n.Children, indent+1); err != nil {
			return err
		}
		// Closing brace
		indentStr := strings.Repeat("    ", indent)
		fmt.Fprintf(w, "%s}\n", indentStr)
		return nil
	case *ir.DistilledFunction:
		_, err := fmt.Fprintln(w, f.formatFunction(n, indent))
		return err
	case *ir.DistilledField:
		_, err := fmt.Fprintln(w, f.formatField(n, indent))
		return err
	case *ir.DistilledPackage:
		// Format package/namespace declaration
		indentStr := strings.Repeat("    ", indent)
		fmt.Fprintf(w, "%snamespace %s;\n", indentStr, n.Name)
		// Format package members
		return f.FormatNodes(w, n.Children, indent)
	case *ir.DistilledRawContent:
		// Output raw content as-is (e.g., #nullable directives)
		fmt.Fprint(w, n.Content)
		return nil
	case *ir.DistilledComment:
		writeCSharpComment(w, n, strings.Repeat("    ", indent))
		return nil
	default:
		// Report the node, then process any children recursively
		reportUnhandled("csharp", node)
		return f.FormatNodes(w, node.GetChildren(), indent)
	}
}

// FormatNodes formats sibling nodes. A doc comment documents the declaration
// after it; when the formatter does not print that node (a delegate, which it
// cannot render yet) or the node is no declaration (a using directive, a
// preprocessor directive), the doc is dropped, so it does not appear to
// document whatever is printed next.
func (f *CSharpFormatter) FormatNodes(w io.Writer, nodes []ir.DistilledNode, indent int) error {
	return f.formatSiblings(w, nodes, indent, func(ir.DistilledNode) bool { return false })
}

// formatSiblings is FormatNodes for a body that leaves out the nodes skip
// reports, together with their doc comments.
func (f *CSharpFormatter) formatSiblings(w io.Writer, nodes []ir.DistilledNode, indent int, skip func(ir.DistilledNode) bool) error {
	parts := formatCommentedNodes(nodes, strings.Repeat("    ", indent), writeCSharpComment, func(node ir.DistilledNode) (string, bool) {
		if skip(node) {
			return "", false
		}
		var b strings.Builder
		_ = f.FormatNode(&b, node, indent) // a strings.Builder does not fail
		switch node.(type) {
		case *ir.DistilledClass, *ir.DistilledInterface, *ir.DistilledStruct, *ir.DistilledEnum,
			*ir.DistilledFunction, *ir.DistilledField, *ir.DistilledPackage:
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

// writeCSharpComment writes a doc comment as one "/// " line per text line,
// the C# convention, and any other comment as writeCStyleComment does.
func writeCSharpComment(w io.Writer, c *ir.DistilledComment, indent string) {
	if !c.IsDoc() {
		writeCStyleComment(w, c, indent)
		return
	}
	for _, line := range strings.Split(c.Text, "\n") {
		fmt.Fprintf(w, "%s%s\n", indent, strings.TrimRight("/// "+line, " "))
	}
}

func (f *CSharpFormatter) formatImport(imp *ir.DistilledImport) string {
	// Check if this is an aliased import via symbols
	if len(imp.Symbols) == 1 && imp.Symbols[0].Alias != "" {
		return fmt.Sprintf("using %s = %s.%s;", imp.Symbols[0].Alias, imp.Module, imp.Symbols[0].Name)
	}
	return fmt.Sprintf("using %s;", imp.Module)
}

func (f *CSharpFormatter) formatClass(class *ir.DistilledClass, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Build visibility keyword (not prefix)
	visibility := f.getVisibilityKeyword(class.Visibility)

	// Collect all modifiers
	modifiers := []string{}

	// Add visibility first if not default
	if visibility != "" {
		modifiers = append(modifiers, visibility)
	}

	// Check if this is a record and/or struct
	isRecord := false
	isStruct := false

	// Add other modifiers
	for _, mod := range class.Modifiers {
		if mod == ir.ModifierData {
			isRecord = true
		} else if mod == ir.ModifierStruct {
			isStruct = true
		} else if mod == ir.ModifierReadonly {
			modifiers = append(modifiers, "readonly")
		} else if mod == ir.ModifierAbstract {
			modifiers = append(modifiers, "abstract")
		} else if mod == ir.ModifierSealed {
			modifiers = append(modifiers, "sealed")
		} else if mod == ir.ModifierPartial {
			modifiers = append(modifiers, "partial")
		} else if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		}
	}

	// Class/Record declaration
	classDecl := indentStr
	if len(modifiers) > 0 {
		classDecl += strings.Join(modifiers, " ") + " "
	}

	if isRecord && isStruct {
		classDecl += "record struct " + class.Name
	} else if isRecord {
		classDecl += "record " + class.Name

		// For records, check if there are constructor parameters (properties)
		recordParams := []string{}
		for _, child := range class.Children {
			if field, ok := child.(*ir.DistilledField); ok && field.IsProperty && field.HasGetter && !field.HasSetter {
				// Skip ModifierReadonly check - record properties are readonly by nature
				hasReadonly := false
				for _, mod := range field.Modifiers {
					if mod == ir.ModifierReadonly {
						hasReadonly = true
						break
					}
				}

				if hasReadonly {
					// This is likely a record parameter
					paramStr := ""
					// Add attributes if any
					if len(field.Decorators) > 0 {
						for _, dec := range field.Decorators {
							paramStr += dec + " "
						}
					}
					if field.Type != nil {
						paramStr += f.formatTypeRef(field.Type) + " " + field.Name
					} else {
						paramStr += field.Name
					}
					recordParams = append(recordParams, paramStr)
				}
			}
		}

		if len(recordParams) > 0 {
			classDecl += "("
			if len(recordParams) == 1 {
				// Single parameter on same line
				classDecl += recordParams[0] + ")"
			} else if len(recordParams) == 2 {
				// Two parameters on same line
				classDecl += "\n    " + indentStr + recordParams[0] + ",\n    " + indentStr + recordParams[1] + ")"
			} else {
				// Multiple parameters on separate lines
				for i, param := range recordParams {
					classDecl += "\n    " + indentStr + param
					if i < len(recordParams)-1 {
						classDecl += ","
					}
				}
				classDecl += ")"
			}
		}
	} else if isStruct {
		classDecl += "struct " + class.Name
	} else {
		classDecl += "class " + class.Name
	}

	// Generics
	if len(class.TypeParams) > 0 {
		genericParams := []string{}
		for _, g := range class.TypeParams {
			genericParams = append(genericParams, g.Name)
		}
		classDecl += "<" + strings.Join(genericParams, ", ") + ">"
	}

	// Inheritance
	if len(class.Extends) > 0 || len(class.Implements) > 0 {
		bases := []string{}
		for _, ext := range class.Extends {
			bases = append(bases, ext.Name)
		}
		for _, impl := range class.Implements {
			bases = append(bases, impl.Name)
		}
		classDecl += " : " + strings.Join(bases, ", ")
	}

	// Add generic constraints with 'where' clauses if present
	if len(class.TypeParams) > 0 {
		for _, typeParam := range class.TypeParams {
			if len(typeParam.Constraints) > 0 {
				constraints := []string{}
				for _, constraint := range typeParam.Constraints {
					constraints = append(constraints, f.formatTypeRef(&constraint))
				}
				classDecl += " where " + typeParam.Name + " : " + strings.Join(constraints, ", ")
			}
		}
	}

	// Don't add opening brace here - let caller decide based on body content
	return classDecl
}

func (f *CSharpFormatter) formatInterface(intf *ir.DistilledInterface, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Build visibility keyword
	visibility := f.getVisibilityKeyword(intf.Visibility)

	// Interface declaration
	intfDecl := indentStr
	if visibility != "" {
		intfDecl += visibility + " "
	}
	intfDecl += "interface " + intf.Name

	// Generics
	if len(intf.TypeParams) > 0 {
		genericParams := []string{}
		for _, g := range intf.TypeParams {
			genericParams = append(genericParams, g.Name)
		}
		intfDecl += "<" + strings.Join(genericParams, ", ") + ">"
	}

	// Extends
	if len(intf.Extends) > 0 {
		extends := []string{}
		for _, ext := range intf.Extends {
			extends = append(extends, ext.Name)
		}
		intfDecl += " : " + strings.Join(extends, ", ")
	}

	// Add generic constraints with 'where' clauses if present
	if len(intf.TypeParams) > 0 {
		for _, typeParam := range intf.TypeParams {
			if len(typeParam.Constraints) > 0 {
				constraints := []string{}
				for _, constraint := range typeParam.Constraints {
					constraints = append(constraints, f.formatTypeRef(&constraint))
				}
				intfDecl += " where " + typeParam.Name + " : " + strings.Join(constraints, ", ")
			}
		}
	}

	return intfDecl
}

func (f *CSharpFormatter) formatEnum(enum *ir.DistilledEnum, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Build visibility keyword
	visibility := f.getVisibilityKeyword(enum.Visibility)

	// Enum declaration
	enumDecl := indentStr
	if visibility != "" {
		enumDecl += visibility + " "
	}
	enumDecl += "enum " + enum.Name

	// Base type
	if enum.Type != nil {
		enumDecl += " : " + enum.Type.Name
	}

	return enumDecl
}

func (f *CSharpFormatter) formatFunction(fn *ir.DistilledFunction, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Build visibility keyword
	visibility := f.getVisibilityKeyword(fn.Visibility)

	// Collect all modifiers
	modifiers := []string{}

	// Add visibility first if not default
	if visibility != "" {
		modifiers = append(modifiers, visibility)
	}

	// Add other modifiers
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierAbstract {
			modifiers = append(modifiers, "abstract")
		} else if mod == ir.ModifierAsync {
			modifiers = append(modifiers, "async")
		} else if mod == ir.ModifierVirtual {
			modifiers = append(modifiers, "virtual")
		} else if mod == ir.ModifierOverride {
			modifiers = append(modifiers, "override")
		} else if mod == ir.ModifierSealed {
			modifiers = append(modifiers, "sealed")
		}
	}

	// Function signature
	signature := indentStr
	if len(modifiers) > 0 {
		signature += strings.Join(modifiers, " ") + " "
	}

	// Add return type if present (constructors have no return type)
	if fn.Returns != nil && fn.Returns.Name != "" {
		signature += f.formatTypeRef(fn.Returns) + " "
	} else if fn.Returns == nil {
		// This might be a constructor - no return type specified
		// Don't add "void"
	} else {
		// Explicit void return
		signature += "void "
	}

	signature += fn.Name

	// Generics
	if len(fn.TypeParams) > 0 {
		genericParams := []string{}
		for _, g := range fn.TypeParams {
			genericParams = append(genericParams, g.Name)
		}
		signature += "<" + strings.Join(genericParams, ", ") + ">"
	}

	// Parameters
	params := []string{}
	for _, p := range fn.Parameters {
		param := ""
		if p.Type.Name != "" {
			param = f.formatTypeRef(&p.Type) + " " + p.Name
		} else {
			param = "dynamic " + p.Name
		}
		if p.DefaultValue != "" {
			param += " = " + p.DefaultValue
		}
		params = append(params, param)
	}
	signature += "(" + strings.Join(params, ", ") + ")"

	// Add generic constraints with 'where' clauses if present
	if len(fn.TypeParams) > 0 {
		for _, typeParam := range fn.TypeParams {
			if len(typeParam.Constraints) > 0 {
				constraints := []string{}
				for _, constraint := range typeParam.Constraints {
					constraints = append(constraints, f.formatTypeRef(&constraint))
				}
				signature += " where " + typeParam.Name + " : " + strings.Join(constraints, ", ")
			}
		}
	}

	// Render the method body when present, otherwise emit a bare declaration.
	if fn.Implementation == "" {
		return signature + ";"
	}

	impl := strings.TrimSpace(fn.Implementation)

	// Expression-bodied member: "=> expr"
	if strings.HasPrefix(impl, "=>") {
		if !strings.HasSuffix(impl, ";") {
			impl += ";"
		}
		return signature + " " + impl
	}

	// Block body. The parser captures the whole block including its braces, so
	// strip the outer braces and re-indent the inner statements to match the
	// nesting of the distilled output (preserving relative indentation).
	body := strings.Trim(strings.TrimSuffix(strings.TrimPrefix(impl, "{"), "}"), "\n")
	lines := strings.Split(body, "\n")

	minIndent := -1
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		lead := len(line) - len(strings.TrimLeft(line, " \t"))
		if minIndent == -1 || lead < minIndent {
			minIndent = lead
		}
	}
	if minIndent < 0 {
		minIndent = 0
	}

	var sb strings.Builder
	sb.WriteString(signature)
	sb.WriteString(" {")
	bodyIndent := strings.Repeat("    ", indent+1)
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) >= minIndent {
			line = line[minIndent:]
		}
		sb.WriteString("\n")
		sb.WriteString(bodyIndent)
		sb.WriteString(strings.TrimRight(line, " \t"))
	}
	sb.WriteString("\n")
	sb.WriteString(indentStr)
	sb.WriteString("}")

	return sb.String()
}

func (f *CSharpFormatter) formatField(field *ir.DistilledField, indent int) string {
	indentStr := strings.Repeat("    ", indent)

	// Build visibility keyword
	visibility := f.getVisibilityKeyword(field.Visibility)

	// Collect all modifiers
	modifiers := []string{}

	// Add visibility first if not default
	if visibility != "" {
		modifiers = append(modifiers, visibility)
	}

	// Add other modifiers
	for _, mod := range field.Modifiers {
		if mod == ir.ModifierStatic {
			modifiers = append(modifiers, "static")
		} else if mod == ir.ModifierReadonly && !field.IsProperty {
			// Only add readonly for fields, not properties
			modifiers = append(modifiers, "readonly")
		} else if mod == ir.ModifierConst {
			modifiers = append(modifiers, "const")
		}
	}

	// Field declaration
	fieldDecl := indentStr
	if len(modifiers) > 0 {
		fieldDecl += strings.Join(modifiers, " ") + " "
	}

	// Type
	if field.Type != nil && field.Type.Name != "" {
		fieldDecl += f.formatTypeRef(field.Type) + " "
	} else {
		fieldDecl += "dynamic "
	}

	fieldDecl += field.Name

	// Initializer
	if field.DefaultValue != "" {
		fieldDecl += " = " + field.DefaultValue
	}

	// Properties have different syntax
	if field.IsProperty {
		// For properties, show them with { get; set; } or { get; }
		if field.HasGetter && field.HasSetter {
			fieldDecl += " { get; set; }"
		} else if field.HasGetter {
			fieldDecl += " { get; }"
		} else if field.HasSetter {
			fieldDecl += " { set; }"
		} else {
			// Default for properties
			fieldDecl += " { get; set; }"
		}
	} else {
		// Regular field
		fieldDecl += ";"
	}

	return fieldDecl
}

// getVisibilityKeyword converts visibility enum to C# keyword
func (f *CSharpFormatter) getVisibilityKeyword(visibility ir.Visibility) string {
	switch visibility {
	case ir.VisibilityPublic:
		return "public"
	case ir.VisibilityPrivate:
		return "private"
	case ir.VisibilityProtected:
		return "protected"
	case ir.VisibilityInternal:
		return "internal"
	case ir.VisibilityProtectedInternal:
		return "protected internal"
	case ir.VisibilityPrivateProtected:
		return "private protected"
	default:
		return "" // Default visibility (internal in C#)
	}
}

// formatTypeRef formats a type reference including generics and nullability
func (f *CSharpFormatter) formatTypeRef(typeRef *ir.TypeRef) string {
	if typeRef == nil {
		return ""
	}

	result := typeRef.Name

	// Add generic type arguments if present
	if len(typeRef.TypeArgs) > 0 {
		genericArgs := []string{}
		for _, arg := range typeRef.TypeArgs {
			genericArgs = append(genericArgs, f.formatTypeRef(&arg))
		}
		result += "<" + strings.Join(genericArgs, ", ") + ">"
	}

	// Add nullable indicator if present
	if typeRef.IsNullable {
		result += "?"
	}

	return result
}
