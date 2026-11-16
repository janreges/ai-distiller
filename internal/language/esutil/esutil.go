// Package esutil holds what the JavaScript and TypeScript processors read the
// same way from their tree-sitter trees: export statements that declare
// nothing (re-exports, export lists, `export default <expression>`) and the
// signature text of an expression with its function bodies left out. The two
// grammars share these node types.
package esutil

import (
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// MaxValueLen is the longest expression printed for `export default <expr>`
// and `export = <expr>` when the expression holds no function.
const MaxValueLen = 80

// IsFunction reports whether node is a function expression: an arrow
// function, a function expression or a generator function.
func IsFunction(node *sitter.Node) bool {
	if node == nil {
		return false
	}
	switch node.Kind() {
	case "arrow_function", "function_expression", "function", "generator_function":
		return true
	}
	return false
}

// unwrap returns the expression inside `(x)`, `x as T`, `x satisfies T`
// and `x!`.
func unwrap(node *sitter.Node) *sitter.Node {
	for node != nil {
		switch node.Kind() {
		case "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression":
			node = node.NamedChild(0)
		default:
			return node
		}
	}
	return node
}

// ContainsJSX reports whether the tree under node holds JSX: an element, a
// self-closing element or a fragment.
func ContainsJSX(node *sitter.Node) bool {
	cursor := node.Walk()
	defer cursor.Close()
	for {
		if strings.HasPrefix(cursor.Node().Kind(), "jsx_") {
			return true
		}
		if cursor.GotoFirstChild() || cursor.GotoNextSibling() {
			continue
		}
		for !cursor.GotoNextSibling() {
			if !cursor.GotoParent() {
				return false
			}
		}
	}
}

// DeclarationModifier returns the modifier for the keyword of a
// `lexical_declaration` or `variable_declaration`: `final` for `const`,
// `let` for `let`, `var` for `var`, or "" for any other keyword.
func DeclarationModifier(node *sitter.Node) ir.Modifier {
	if node == nil {
		return ""
	}
	if node.Kind() == "variable_declaration" {
		return ir.ModifierVar
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		switch node.Child(i).Kind() {
		case "const":
			return ir.ModifierFinal
		case "let":
			return ir.ModifierLet
		}
	}
	return ""
}

// TakesFunction reports whether node is a call or `new` that passes a
// function as an argument, directly or in a call it is chained to:
// `forwardRef((p, r) => ...)`, `memo(function List() {...})`,
// `connect(map)(function X() {...})`, `useCallback(() => ..., [])`,
// `new Pool(() => ...)`. A call written in parentheses or with `as`,
// `satisfies` or `!` counts too.
func TakesFunction(node *sitter.Node) bool {
	node = unwrap(node)
	for node != nil && (node.Kind() == "call_expression" || node.Kind() == "new_expression") {
		if args := node.ChildByFieldName("arguments"); args != nil {
			for i := uint(0); i < args.NamedChildCount(); i++ {
				if IsFunction(unwrap(args.NamedChild(i))) {
					return true
				}
			}
		}
		if node.Kind() == "new_expression" {
			return false
		}
		node = unwrap(node.ChildByFieldName("function"))
	}
	return false
}

// bodyStart returns the byte where the body of a function-like node starts
// to be left out, and whether node is one: after the return type, else the
// parameters, of an arrow function (its `=>` goes too), a function or
// generator expression, or a method of an object literal or class
// expression.
func bodyStart(node *sitter.Node) (uint, bool) {
	switch node.Kind() {
	case "arrow_function", "function_expression", "function", "generator_function", "method_definition":
	default:
		return 0, false
	}
	if node.ChildByFieldName("body") == nil {
		return 0, false
	}
	for _, field := range []string{"return_type", "parameters", "parameter"} {
		if c := node.ChildByFieldName(field); c != nil {
			return c.EndByte(), true
		}
	}
	return 0, false
}

// Signature returns the source text of node, an expression, as written but
// with the body of every function in it left out: an arrow function keeps
// its `async`, type parameters, parameters (a lone parameter written without
// parentheses gets them: `x => x` reads `(x)`) and return type, a function
// expression its keyword, name, parameters and return type. Comments are
// left out, and whitespace that holds a line break becomes one space (none
// after `(`, `[` or `<`, before `)`, `]`, `>`, `,` or `;`, and a trailing
// comma before a closing bracket goes), so the result is one line. Strings
// and template literals are kept verbatim. In the signature of a call that
// takes a function (TakesFunction), an object literal passed to a call, at
// any depth, reads `{...}` when its own signature is longer than
// MaxValueLen.
//
//	forwardRef<B, P>(({ a = 1, ...rest }, ref) => { ... })
//	  -> forwardRef<B, P>(({ a = 1, ...rest }, ref))
//	core.$constructor("ZodType", (inst, def) => { ... }, { check(...chks) { ... }, ... })
//	  -> core.$constructor("ZodType", (inst, def), {...})
func Signature(node *sitter.Node, source []byte) string {
	s := &signer{source: source, prevEnd: node.StartByte(), elideObjects: TakesFunction(node)}
	s.walk(node)
	return s.b.String()
}

type signer struct {
	source  []byte
	b       strings.Builder
	prevEnd uint // end of the last byte consumed
	broken  bool // a line break, comment or left-out body since the last token
	// elideObjects: an object literal argument longer than MaxValueLen
	// reads {...}.
	elideObjects bool
}

func (s *signer) walk(node *sitter.Node) {
	switch node.Kind() {
	case "comment":
		s.skip(node.StartByte(), node.EndByte())
		return
	case "string", "template_string", "regex":
		s.token(node.StartByte(), node.EndByte())
		return
	}
	if node.ChildCount() == 0 {
		s.token(node.StartByte(), node.EndByte())
		return
	}
	cut, isFn := bodyStart(node)
	// `x => ...` without parentheses: its parameter list reads `(x)`, so the
	// function stays recognisable once its body is gone.
	var bare *sitter.Node
	if isFn && node.Kind() == "arrow_function" {
		bare = node.ChildByFieldName("parameter")
	}
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		if isFn && child.StartByte() >= cut {
			continue
		}
		if bare != nil && child.StartByte() == bare.StartByte() && child.EndByte() == bare.EndByte() {
			s.emit("(", child.StartByte(), child.StartByte())
			s.walk(child)
			s.emit(")", child.EndByte(), child.EndByte())
			continue
		}
		// An object literal passed to a call is left out as {...} when its
		// signature is longer than MaxValueLen: a long options object or
		// the method table of a constructor call. The other arguments, the
		// functions too, stay.
		if s.elideObjects && node.Kind() == "arguments" && child.Kind() == "object" && len(Signature(child, s.source)) > MaxValueLen {
			s.emit("{...}", child.StartByte(), child.EndByte())
			continue
		}
		s.walk(child)
	}
	if isFn {
		s.skip(cut, node.EndByte())
	}
}

// skip consumes source[start:end] without writing it.
func (s *signer) skip(start, end uint) {
	if start < s.prevEnd {
		start = s.prevEnd
	}
	if end > start {
		s.gap(start)
		s.broken = true
		s.prevEnd = end
	}
}

// gap records the whitespace between the last consumed byte and start.
func (s *signer) gap(start uint) {
	if start > s.prevEnd && strings.Contains(string(s.source[s.prevEnd:start]), "\n") {
		s.broken = true
	}
}

// token writes source[start:end] after the separator it needs.
func (s *signer) token(start, end uint) {
	s.emit(string(s.source[start:end]), start, end)
}

// emit writes text, which stands for source[start:end], after the separator
// it needs.
func (s *signer) emit(text string, start, end uint) {
	if start < s.prevEnd {
		return
	}
	s.gap(start)
	if s.b.Len() > 0 {
		if s.broken {
			s.writeBreak(text)
		} else {
			s.b.WriteString(string(s.source[s.prevEnd:start]))
		}
	}
	s.b.WriteString(text)
	s.prevEnd = end
	s.broken = false
}

// writeBreak writes the separator for a line break or a left-out part
// before the token next.
func (s *signer) writeBreak(next string) {
	out := s.b.String()
	closing := strings.HasPrefix(next, ")") || strings.HasPrefix(next, "]") || strings.HasPrefix(next, "}") || next == ">"
	if closing && strings.HasSuffix(out, ",") {
		out = strings.TrimSuffix(out, ",")
		s.b.Reset()
		s.b.WriteString(out)
	}
	switch {
	case strings.HasSuffix(out, "(") || strings.HasSuffix(out, "[") || strings.HasSuffix(out, "<"):
	case strings.HasPrefix(next, ")") || strings.HasPrefix(next, "]") || next == ">":
	case strings.HasPrefix(next, ",") || strings.HasPrefix(next, ";"):
	default:
		s.b.WriteString(" ")
	}
}

// Export returns the export statement node as a DistilledExport, or nil when
// it exports a declaration (`export class X`, `export default function f`),
// which the processor parses itself, or a class expression. Handled:
//
//	export * from "m"               symbol "*"
//	export * as ns from "m"         symbol "*" aliased ns
//	export { a, b as c } from "m"   symbols a and b (alias c); IsType for `export type {...}`
//	export { a }                    no module: names of this file
//	export default <expression>     IsDefault, Value: the expression's Signature
//	export = <expression>           Value (TypeScript)
//	export as namespace Lib         Namespace (TypeScript)
//
// The statement is left out (nil) when the value of `export default` or
// `export =` holds no function and its signature is longer than
// MaxValueLen: a long object literal or configuration call.
func Export(node *sitter.Node, source []byte) *ir.DistilledExport {
	if node.ChildByFieldName("declaration") != nil {
		return nil
	}
	exp := &ir.DistilledExport{BaseNode: ir.BaseNode{Location: location(node)}}
	if src := node.ChildByFieldName("source"); src != nil {
		exp.Module = strings.Trim(text(src, source), "\"'`")
	}
	if value := node.ChildByFieldName("value"); value != nil {
		if value.Kind() == "class" {
			return nil
		}
		exp.IsDefault = true
		exp.Value = Signature(value, source)
		if !IsFunction(value) && !TakesFunction(value) && len(exp.Value) > MaxValueLen {
			return nil
		}
		return exp
	}
	afterEquals, asNamespace := false, false
	for i := uint(0); i < node.ChildCount(); i++ {
		child := node.Child(i)
		if child == nil {
			continue
		}
		switch child.Kind() {
		case "type":
			exp.IsType = true
		case "*":
			exp.Symbols = append(exp.Symbols, ir.ImportedSymbol{Name: "*"})
		case "namespace_export":
			alias := ""
			for j := uint(0); j < child.NamedChildCount(); j++ {
				alias = text(child.NamedChild(j), source)
			}
			exp.Symbols = append(exp.Symbols, ir.ImportedSymbol{Name: "*", Alias: alias})
		case "export_clause":
			for j := uint(0); j < child.NamedChildCount(); j++ {
				spec := child.NamedChild(j)
				if spec == nil || spec.Kind() != "export_specifier" {
					continue
				}
				name := text(spec.ChildByFieldName("name"), source)
				if name == "" {
					continue
				}
				isType := false
				for k := uint(0); k < spec.ChildCount(); k++ {
					if c := spec.Child(k); c != nil && c.Kind() == "type" {
						isType = true
					}
				}
				exp.Symbols = append(exp.Symbols, ir.ImportedSymbol{
					Name:   name,
					Alias:  text(spec.ChildByFieldName("alias"), source),
					IsType: isType,
				})
			}
		case "=":
			afterEquals = true
		case "namespace":
			asNamespace = true
		default:
			switch {
			case asNamespace && child.Kind() == "identifier" && exp.Namespace == "":
				exp.Namespace = text(child, source)
			case afterEquals && child.IsNamed() && exp.Value == "":
				exp.Value = Signature(child, source)
				if !IsFunction(child) && !TakesFunction(child) && len(exp.Value) > MaxValueLen {
					return nil
				}
			}
		}
	}
	if len(exp.Symbols) == 0 && exp.Value == "" && exp.Namespace == "" {
		return nil
	}
	return exp
}

// LocalNames returns the names of this file that exp exports: the names of
// an export list without a module, and the value of `export default` or
// `export =` when it is a plain identifier.
func LocalNames(exp *ir.DistilledExport) []string {
	var names []string
	if exp.Module == "" {
		for _, sym := range exp.Symbols {
			if sym.Name != "*" {
				names = append(names, sym.Name)
			}
		}
	}
	if isIdentifier(exp.Value) {
		names = append(names, exp.Value)
	}
	return names
}

func isIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		ok := r == '_' || r == '$' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r > 127 || (i > 0 && r >= '0' && r <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func text(node *sitter.Node, source []byte) string {
	if node == nil || node.EndByte() > uint(len(source)) {
		return ""
	}
	return string(source[node.StartByte():node.EndByte()])
}

func location(node *sitter.Node) ir.Location {
	return ir.Location{
		StartLine:   int(node.StartPosition().Row) + 1,
		StartColumn: int(node.StartPosition().Column) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		EndColumn:   int(node.EndPosition().Column) + 1,
		StartByte:   int(node.StartByte()),
		EndByte:     int(node.EndByte()),
	}
}
