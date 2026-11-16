package formatter

import (
	"fmt"
	"io"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// esImportLine returns a JavaScript or TypeScript import as the source wrote
// it, with the module in single quotes and no semicolon:
//
//	import './polyfill'                  (no symbols: a side-effect import)
//	import D from 'm'                    (the symbol "default" aliased D)
//	import D, { A, B as C } from 'm'
//	import * as ns from 'm'              (the symbol "*" aliased ns)
//	import type { T } from 'm'           (IsType)
//	import { type T, V } from 'm'        (a symbol's IsType)
//	import x = require('m')              (import type "require", TypeScript)
//	import G = ns.H                      (import type "alias", TypeScript)
//	export import G = ns.H               (an alias with IsExport)
//
// A default import is recognised only as the first symbol, where the clause
// writes it; a later "default" symbol prints as `default as X` in braces.
func esImportLine(imp *ir.DistilledImport) string {
	if imp.ImportType == "require" || imp.ImportType == "alias" {
		local := ""
		if len(imp.Symbols) > 0 {
			local = imp.Symbols[0].Alias
		}
		if imp.ImportType == "require" {
			return fmt.Sprintf("import %s = require('%s')", local, imp.Module)
		}
		export := ""
		if imp.IsExport {
			export = "export "
		}
		return fmt.Sprintf("%simport %s = %s", export, local, imp.Module)
	}

	var clause []string
	var named []string
	for i, sym := range imp.Symbols {
		switch {
		case i == 0 && sym.Name == "default" && sym.Alias != "" && !sym.IsType:
			clause = append(clause, sym.Alias)
		case sym.Name == "*" && sym.Alias != "":
			clause = append(clause, "* as "+sym.Alias)
		case sym.Name != "":
			named = append(named, esSpecifier(sym))
		}
	}
	if len(named) > 0 {
		clause = append(clause, "{ "+strings.Join(named, ", ")+" }")
	}

	if len(clause) == 0 {
		return fmt.Sprintf("import '%s'", imp.Module)
	}
	typePrefix := ""
	if imp.IsType {
		typePrefix = "type "
	}
	return fmt.Sprintf("import %s%s from '%s'", typePrefix, strings.Join(clause, ", "), imp.Module)
}

// esExportLine returns an export statement that declares nothing as the
// source wrote it, the module in single quotes like an import's:
//
//	export * from './a'
//	export * as ns from './b'
//	export { x, y as z } from './c'
//	export { q }
//	export type { T } from './t'
//	export default Inner
//	export = Foo
//	export as namespace Lib
func esExportLine(exp *ir.DistilledExport) string {
	switch {
	case exp.Namespace != "":
		return "export as namespace " + exp.Namespace
	case exp.IsDefault:
		return "export default " + exp.Value
	case exp.Value != "":
		return "export = " + exp.Value
	}
	typePrefix := ""
	if exp.IsType {
		typePrefix = "type "
	}
	var clause string
	if len(exp.Symbols) == 1 && exp.Symbols[0].Name == "*" {
		clause = "*"
		if exp.Symbols[0].Alias != "" {
			clause += " as " + exp.Symbols[0].Alias
		}
	} else {
		named := make([]string, 0, len(exp.Symbols))
		for _, sym := range exp.Symbols {
			named = append(named, esSpecifier(sym))
		}
		clause = "{ " + strings.Join(named, ", ") + " }"
	}
	if exp.Module == "" {
		return "export " + typePrefix + clause
	}
	return fmt.Sprintf("export %s%s from '%s'", typePrefix, clause, exp.Module)
}

// esExportPrefix returns "export ", "export default " or "" for a
// declaration's modifiers.
func esExportPrefix(mods []ir.Modifier) string {
	export, def := false, false
	for _, m := range mods {
		switch m {
		case ir.ModifierExport:
			export = true
		case ir.ModifierDefault:
			def = true
		}
	}
	switch {
	case export && def:
		return "export default "
	case export:
		return "export "
	}
	return ""
}

// esDeclarationKeyword returns the keyword a variable was declared with,
// `const` (the modifier `final`), `let` or `var`, or "" for a declaration
// that is not a variable (a `function`).
func esDeclarationKeyword(mods []ir.Modifier) string {
	for _, m := range mods {
		switch m {
		case ir.ModifierFinal:
			return "const"
		case ir.ModifierLet:
			return "let"
		case ir.ModifierVar:
			return "var"
		}
	}
	return ""
}

// esSpecifier returns one specifier of an import or export list:
// `A`, `B as C`, `type T`.
func esSpecifier(sym ir.ImportedSymbol) string {
	s := sym.Name
	if sym.Alias != "" {
		s += " as " + sym.Alias
	}
	if sym.IsType {
		s = "type " + s
	}
	return s
}

// writeInitializer ends the line of a top-level JavaScript or TypeScript
// variable: ` = <initializer as written>` when the field keeps its
// Implementation (--implementation=1), else ` = <DefaultValue>`, else
// nothing.
func writeInitializer(w io.Writer, field *ir.DistilledField) {
	switch {
	case field.Implementation != "":
		fmt.Fprint(w, " =", renderBody(field.Implementation, ""))
	case field.DefaultValue != "":
		fmt.Fprintf(w, " = %s\n", field.DefaultValue)
	default:
		fmt.Fprintln(w)
	}
}
