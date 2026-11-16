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

	// afterComment is set while the last node written was a comment. A class
	// or interface then omits its leading blank line, so its doc comment stays
	// directly above it.
	afterComment bool
}

// NewTypeScriptFormatter creates a new TypeScript formatter
func NewTypeScriptFormatter() *TypeScriptFormatter {
	return &TypeScriptFormatter{}
}

// Reset clears the per-file state.
func (f *TypeScriptFormatter) Reset() {
	f.afterComment = false
}

// FormatNode formats a TypeScript node
func (f *TypeScriptFormatter) FormatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	err := f.formatNode(w, node, indent)
	_, f.afterComment = node.(*ir.DistilledComment)
	return err
}

func (f *TypeScriptFormatter) formatNode(w io.Writer, node ir.DistilledNode, indent int) error {
	switch n := node.(type) {
	case *ir.DistilledComment:
		writeCStyleComment(w, n, strings.Repeat("    ", indent))
		return nil
	case *ir.DistilledImport:
		return f.formatImport(w, n)
	case *ir.DistilledExport:
		fmt.Fprintf(w, "%s%s\n", strings.Repeat("    ", indent), esExportLine(n))
		return nil
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
	case *ir.DistilledEnum:
		return f.formatEnum(w, n, indent)
	case *ir.DistilledPackage:
		return f.formatNamespace(w, n, indent)
	default:
		// Fallback for unknown nodes
		reportUnhandled("typescript", node)
		return nil
	}
}

// formatNamespace prints `namespace X {` or `declare module "x" {` and the
// members indented one level deeper than the namespace itself.
func (f *TypeScriptFormatter) formatNamespace(w io.Writer, pkg *ir.DistilledPackage, indent int) error {
	pad := strings.Repeat("    ", indent)
	keyword := "namespace"
	if strings.HasPrefix(pkg.Name, `"`) || strings.HasPrefix(pkg.Name, "'") {
		keyword = "declare module"
	}
	fmt.Fprintf(w, "%s%s %s {\n", pad, keyword, pkg.Name)
	var buf strings.Builder
	for _, child := range pkg.Children {
		if err := f.FormatNode(&buf, child, 0); err != nil {
			return err
		}
	}
	for _, line := range strings.Split(buf.String(), "\n") {
		if line != "" {
			fmt.Fprintf(w, "%s    %s\n", pad, line)
		}
	}
	fmt.Fprintf(w, "%s}\n", pad)
	return nil
}

// formatImport prints the import as written (esImportLine).
func (f *TypeScriptFormatter) formatImport(w io.Writer, imp *ir.DistilledImport) error {
	fmt.Fprintln(w, esImportLine(imp))
	return nil
}

func (f *TypeScriptFormatter) formatClass(w io.Writer, class *ir.DistilledClass, indent int) error {
	// Format class declaration
	modifiers := ""
	for _, mod := range class.Modifiers {
		if mod == ir.ModifierAbstract {
			modifiers += "abstract "
		}
	}

	f.writeBlankLine(w)
	fmt.Fprintf(w, "%s%sclass", esExportPrefix(class.Modifiers), modifiers)
	if class.Name != "" {
		fmt.Fprintf(w, " %s", class.Name)
	}

	fmt.Fprint(w, tsTypeParams(class.TypeParams))

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

// writeBlankLine writes the blank line that opens a class or interface,
// unless a comment was just written: that comment documents the declaration.
func (f *TypeScriptFormatter) writeBlankLine(w io.Writer) {
	if !f.afterComment {
		fmt.Fprintln(w)
	}
}

func (f *TypeScriptFormatter) formatInterface(w io.Writer, intf *ir.DistilledInterface, indent int) error {
	f.writeBlankLine(w)
	fmt.Fprintf(w, "%sinterface %s", esExportPrefix(intf.Modifiers), intf.Name)

	fmt.Fprint(w, tsTypeParams(intf.TypeParams))

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

			fmt.Fprint(w, tsTypeParams(c.TypeParams))

			fmt.Fprintf(w, "(%s)", tsParams(c.Parameters))

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
	// A function declared as a variable (`const f = () => ...`) prints with
	// its keyword: const, let or var.
	declKeyword := esDeclarationKeyword(fn.Modifiers)

	// Format visibility keyword (only for class methods, not top-level functions)
	if indent > 0 {
		visKeyword := f.getTypeScriptVisibilityKeyword(fn.Visibility)
		if visKeyword != "" {
			modifiers = visKeyword + " " + modifiers
		}
	}
	for _, mod := range fn.Modifiers {
		if mod == ir.ModifierAbstract {
			modifiers += "abstract "
		} else if mod == ir.ModifierAsync {
			modifiers += "async "
		} else if mod == ir.ModifierStatic {
			modifiers += "static "
		}
	}

	// Top-level functions don't have indentation
	if indent == 0 {
		indentStr = ""
		exportPrefix := esExportPrefix(fn.Modifiers)
		// A top-level arrow function constant prints as written without its
		// body: `const name = async <T>(params): R`, `let f = (a: number)`.
		if declKeyword != "" {
			async := ""
			if hasModifier(fn.Modifiers, ir.ModifierAsync) {
				async = "async "
			}
			fmt.Fprintf(w, "%s%s%s %s = %s", indentStr, exportPrefix, declKeyword, fn.Name, async)
		} else {
			keyword, name := "function ", fn.Name
			if strings.HasPrefix(name, "*") {
				keyword, name = "function* ", name[1:]
			}
			fmt.Fprintf(w, "%s%s%s%s%s", indentStr, exportPrefix, modifiers, keyword, name)
		}
	} else {
		// Methods inside classes/interfaces - no "function" keyword needed
		fmt.Fprintf(w, "%s%s%s", indentStr, modifiers, fn.Name)
	}

	fmt.Fprint(w, tsTypeParams(fn.TypeParams))

	fmt.Fprintf(w, "(%s)", tsParams(fn.Parameters))

	// Return type
	if fn.Returns != nil && fn.Returns.Name != "" {
		fmt.Fprintf(w, ": %s", fn.Returns.Name)
	}

	impl := strings.TrimSpace(fn.Implementation)
	switch {
	case impl == "":
		fmt.Fprintln(w)
	case strings.HasPrefix(impl, "{"):
		if indent == 0 && declKeyword != "" {
			fmt.Fprint(w, " =>")
		}
		fmt.Fprint(w, renderBody(fn.Implementation, indentStr))
	default:
		fmt.Fprintf(w, " => %s\n", impl)
	}

	return nil
}

// tsTypeParams prints type parameters as written, `<T extends object = {}, U>`,
// or nothing when there are none.
func tsTypeParams(params []ir.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	out := make([]string, len(params))
	for i, param := range params {
		out[i] = param.Name
		if len(param.Constraints) > 0 {
			out[i] += " extends " + param.Constraints[0].Name
		}
		if param.Default != nil && param.Default.Name != "" {
			out[i] += " = " + param.Default.Name
		}
	}
	return "<" + strings.Join(out, ", ") + ">"
}

// tsParams prints parameters as written, comma-separated: `...rest: U[]`,
// `c?: string`, `{ a, b = 1 }: P`, `d: number = 0`.
func tsParams(params []ir.Parameter) string {
	out := make([]string, 0, len(params))
	for _, param := range params {
		if param.Name == "" {
			continue
		}
		s := param.Name
		if param.IsVariadic {
			s = "..." + s
		}
		if param.IsOptional {
			s += "?"
		}
		if param.Type.Name != "" {
			s += ": " + param.Type.Name
		}
		if param.DefaultValue != "" {
			s += " = " + param.DefaultValue
		}
		out = append(out, s)
	}
	return strings.Join(out, ", ")
}

func (f *TypeScriptFormatter) formatField(w io.Writer, field *ir.DistilledField, indent int) error {
	// Handle top-level const/let/var declarations differently from class fields
	if indent == 0 {
		// Top-level variable declaration, with its keyword as written
		varType := esDeclarationKeyword(field.Modifiers)
		if varType == "" {
			varType = "let"
		}

		fmt.Fprintf(w, "%s%s %s", esExportPrefix(field.Modifiers), varType, field.Name)
		if field.Type != nil && field.Type.Name != "" {
			fmt.Fprintf(w, ": %s", field.Type.Name)
		}
		writeInitializer(w, field)
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
	fmt.Fprintf(w, "%stype %s", esExportPrefix(alias.Modifiers), alias.Name)

	fmt.Fprint(w, tsTypeParams(alias.TypeParams))

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

// formatEnum prints a TypeScript enum with one member per line. A
// DistilledEnum has no modifiers, so "export" follows its visibility.
func (f *TypeScriptFormatter) formatEnum(w io.Writer, enum *ir.DistilledEnum, indent int) error {
	pad := strings.Repeat("    ", indent)
	fmt.Fprintf(w, "%s%senum %s {\n", pad, esExportPrefix(enum.Modifiers), enum.Name)
	for _, child := range enum.Children {
		member, ok := child.(*ir.DistilledField)
		if !ok {
			continue
		}
		fmt.Fprintf(w, "%s    %s", pad, member.Name)
		if member.DefaultValue != "" {
			fmt.Fprintf(w, " = %s", member.DefaultValue)
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintf(w, "%s}\n", pad)
	return nil
}
