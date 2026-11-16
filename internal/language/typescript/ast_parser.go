package typescript

import (
	"context"
	"fmt"
	"strings"
	"unsafe"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/janreges/ai-distiller/internal/language/esutil"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	typescript "tree-sitter-typescript"
)

// ASTParser provides tree-sitter based TypeScript parsing
type ASTParser struct {
	source   []byte
	filename string
	isTSX    bool
}

// NewASTParser creates a new tree-sitter TypeScript parser
func NewASTParser() *ASTParser {
	return &ASTParser{}
}

// ProcessSource processes TypeScript source using tree-sitter
func (p *ASTParser) ProcessSource(ctx context.Context, source []byte, filename string, isTSX bool) (*ir.DistilledFile, error) {
	p.source = source
	p.filename = filename
	p.isTSX = isTSX

	// Set the appropriate language based on file type
	var lang unsafe.Pointer
	if isTSX {
		lang = typescript.LanguageTSX()
	} else {
		lang = typescript.Language()
	}

	if lang == nil {
		return nil, fmt.Errorf("tree-sitter-typescript is not available (CGO disabled)")
	}

	// The parser lives as long as this call: the tree and every node taken
	// from it are used only here.
	parser, err := tsparse.NewParser(sitter.NewLanguage(lang))
	if err != nil {
		return nil, fmt.Errorf("failed to create TypeScript parser: %w", err)
	}
	defer parser.Close()

	// Parse the source code
	tree, err := tsparse.Parse(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TypeScript: %w", err)
	}
	defer tree.Close()

	rootNode := tree.RootNode()

	// Create the distilled file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(rootNode.EndPosition().Row) + 1,
			},
		},
		Path:     filename,
		Language: "typescript",
		Version:  "5.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Parse the program
	if rootNode.Kind() == "program" {
		p.parseProgram(rootNode, file)
	}

	// FIXME: The current implementation is flawed and produces incorrect results.
	// Disable until it can be made more robust.
	// p.analyzeInterfaceSatisfaction(file)

	return file, nil
}

// parseProgram parses the top-level program node
func (p *ASTParser) parseProgram(node *sitter.Node, file *ir.DistilledFile) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}
		file.Children = appendStatement(file.Children, child, p.parseStatement(child, false))
	}
	publishExportedNames(file.Children)
}

// appendStatement appends to list the nodes parsed from statement. A comment
// is appended as it is; for any other statement commentutil.AppendStatement
// drops the doc comments of a statement that yields no declaration (an
// expression such as a call or `exports.x = ...`).
func appendStatement(list []ir.DistilledNode, statement *sitter.Node, parsed []ir.DistilledNode) []ir.DistilledNode {
	if statement.Kind() == "comment" || !statement.IsNamed() {
		return append(list, parsed...)
	}
	return commentutil.AppendStatement(list, parsed)
}

// parseStatement parses one declaration-level node. It is shared by the
// program root, export statements and namespace bodies.
func (p *ASTParser) parseStatement(node *sitter.Node, isExported bool) []ir.DistilledNode {
	var nodes []ir.DistilledNode
	switch node.Kind() {
	case "comment":
		nodes = append(nodes, p.parseComment(node))
	case "import_statement":
		if imp := p.parseImport(node); imp != nil {
			nodes = append(nodes, imp)
		}
	case "export_statement":
		nodes = append(nodes, p.parseExport(node)...)
	case "class_declaration", "abstract_class_declaration":
		if class := p.parseClass(node, isExported); class != nil {
			nodes = append(nodes, class)
		}
	case "interface_declaration":
		if intf := p.parseInterface(node, isExported); intf != nil {
			nodes = append(nodes, intf)
		}
	case "type_alias_declaration":
		if alias := p.parseTypeAlias(node, isExported); alias != nil {
			nodes = append(nodes, alias)
		}
	case "enum_declaration":
		if enum := p.parseEnum(node, isExported); enum != nil {
			nodes = append(nodes, enum)
		}
	case "function_declaration", "generator_function_declaration", "function_signature":
		if fn := p.parseFunction(node, isExported); fn != nil {
			nodes = append(nodes, fn)
		}
	case "lexical_declaration", "variable_declaration":
		nodes = append(nodes, p.parseVariableDeclaration(node, isExported)...)
	case "expression_statement":
		// `namespace A { ... }` is wrapped in an expression statement.
		if first := node.NamedChild(0); first != nil && first.Kind() == "internal_module" {
			if ns := p.parseNamespace(first); ns != nil {
				nodes = append(nodes, ns)
			}
		}
	case "internal_module", "module":
		if ns := p.parseNamespace(node); ns != nil {
			nodes = append(nodes, ns)
		}
	case "ambient_declaration":
		// `declare ...` wraps the real declaration.
		for i := 0; i < int(node.NamedChildCount()); i++ {
			nodes = append(nodes, p.parseStatement(node.NamedChild(uint(i)), isExported)...)
		}
	}
	return nodes
}

// parseComment turns a comment token into a comment node. A doc comment at
// file level that carries a file tag (@file, @module, @packageDocumentation,
// ...) documents the file, not the declaration after it.
func (p *ASTParser) parseComment(node *sitter.Node) *ir.DistilledComment {
	text, format := commentutil.StripJSStyle(p.nodeText(node))
	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{Location: p.nodeToLocation(node)},
		Text:     text,
		Format:   format,
	}
	if parent := node.Parent(); format == ir.CommentDoc && parent != nil && parent.Kind() == "program" {
		comment.Inner = commentutil.DocumentsFile(text)
	}
	return comment
}

// parseNamespace parses `namespace X {}` and `declare module "x" {}` into a
// DistilledPackage whose children are the declarations of the body. In an
// ambient namespace or module (one inside a `declare`, such as `declare
// namespace X {}`, `declare module "x" {}` and the namespaces nested in them)
// every member is exported without the `export` keyword, so it is public.
func (p *ASTParser) parseNamespace(node *sitter.Node) *ir.DistilledPackage {
	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}
	pkg := &ir.DistilledPackage{
		BaseNode: ir.BaseNode{Location: p.nodeToLocation(node)},
		Name:     p.nodeText(nameNode),
	}
	ambient := insideAmbientDeclaration(node)
	if body := node.ChildByFieldName("body"); body != nil {
		for i := 0; i < int(body.NamedChildCount()); i++ {
			statement := body.NamedChild(uint(i))
			members := p.parseStatement(statement, false)
			if ambient {
				for _, member := range members {
					makePublic(member)
				}
			}
			pkg.Children = appendStatement(pkg.Children, statement, members)
		}
	}
	publishExportedNames(pkg.Children)
	return pkg
}

// insideAmbientDeclaration reports whether node lies within a `declare`
// (ambient_declaration) node.
func insideAmbientDeclaration(node *sitter.Node) bool {
	for n := node.Parent(); n != nil; n = n.Parent() {
		if n.Kind() == "ambient_declaration" {
			return true
		}
	}
	return false
}

// makePublic gives a declaration public visibility. It adds no `export`
// modifier, so the output does not show a keyword the source lacks.
func makePublic(node ir.DistilledNode) {
	switch n := node.(type) {
	case *ir.DistilledFunction:
		n.Visibility = ir.VisibilityPublic
	case *ir.DistilledField:
		n.Visibility = ir.VisibilityPublic
	case *ir.DistilledClass:
		n.Visibility = ir.VisibilityPublic
	case *ir.DistilledInterface:
		n.Visibility = ir.VisibilityPublic
	case *ir.DistilledTypeAlias:
		n.Visibility = ir.VisibilityPublic
	case *ir.DistilledEnum:
		n.Visibility = ir.VisibilityPublic
	}
}

// parseImport parses an import statement. The symbols follow the convention
// of ir.ImportedSymbol: a default import is the symbol "default" with the
// local name as its alias, a namespace import the symbol "*" with its alias.
// `import x = require("m")` is the import type "require" with the symbol "*"
// aliased x.
func (p *ASTParser) parseImport(node *sitter.Node) *ir.DistilledImport {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		ImportType: "import",
		Symbols:    []ir.ImportedSymbol{},
	}

	imp.Module = p.moduleName(node.ChildByFieldName("source"))

	// Check for type-only import
	if p.findChild(node, "type") != nil {
		imp.IsType = true
	}

	// Parse import clause
	if importClause := p.findChild(node, "import_clause"); importClause != nil {
		p.parseImportClause(importClause, imp)
	} else if require := p.findChild(node, "import_require_clause"); require != nil {
		imp.ImportType = "require"
		imp.Module = p.moduleName(require.ChildByFieldName("source"))
		if name := p.findChild(require, "identifier"); name != nil {
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: "*", Alias: p.nodeText(name)})
		}
	}

	return imp
}

// moduleName returns the text of a module string without its quotes.
func (p *ASTParser) moduleName(source *sitter.Node) string {
	text := p.nodeText(source)
	if len(text) >= 2 {
		return text[1 : len(text)-1]
	}
	return ""
}

// parseImportClause parses the import clause
func (p *ASTParser) parseImportClause(node *sitter.Node, imp *ir.DistilledImport) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "identifier":
			// Default import: the module's "default" under a local name
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  "default",
				Alias: p.nodeText(child),
			})
		case "namespace_import":
			// import * as name
			asClause := p.findChild(child, "identifier")
			if asClause != nil {
				imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
					Name:  "*",
					Alias: p.nodeText(asClause),
				})
			}
		case "named_imports":
			imp.Symbols = append(imp.Symbols, p.specifiers(child, "import_specifier")...)
		}
	}
}

// specifiers returns the specifiers of a named_imports or export_clause node
// (kind is import_specifier or export_specifier): name, alias, and whether
// the specifier is written `type A`.
func (p *ASTParser) specifiers(node *sitter.Node, kind string) []ir.ImportedSymbol {
	var symbols []ir.ImportedSymbol
	for i := 0; i < int(node.NamedChildCount()); i++ {
		spec := node.NamedChild(uint(i))
		if spec == nil || spec.Kind() != kind {
			continue
		}
		name := p.nodeText(spec.ChildByFieldName("name"))
		if name == "" {
			continue
		}
		symbols = append(symbols, ir.ImportedSymbol{
			Name:   name,
			Alias:  p.nodeText(spec.ChildByFieldName("alias")),
			IsType: p.findChild(spec, "type") != nil,
		})
	}
	return symbols
}

// parseExport parses an export statement. An exported declaration carries the
// modifier export, and default too after `export default`; `export default
// class {}` is a class without a name. Every other export (a re-export, an
// export list, `export default <expression>`, `export = x`) is a
// DistilledExport (esutil.Export).
func (p *ASTParser) parseExport(node *sitter.Node) []ir.DistilledNode {
	isDefault := p.findChild(node, "default") != nil
	if decl := node.ChildByFieldName("declaration"); decl != nil {
		nodes := p.parseStatement(decl, true)
		if isDefault {
			for _, n := range nodes {
				addModifier(n, ir.ModifierDefault)
			}
		}
		return nodes
	}
	if value := node.ChildByFieldName("value"); value != nil && value.Kind() == "class" {
		class := p.parseClass(value, true)
		class.Modifiers = append(class.Modifiers, ir.ModifierDefault)
		return []ir.DistilledNode{class}
	}
	if exp := esutil.Export(node, p.source); exp != nil {
		return []ir.DistilledNode{exp}
	}
	return nil
}

// addModifier adds mod to a declaration that has modifiers.
func addModifier(node ir.DistilledNode, mod ir.Modifier) {
	switch n := node.(type) {
	case *ir.DistilledFunction:
		n.Modifiers = append(n.Modifiers, mod)
	case *ir.DistilledField:
		n.Modifiers = append(n.Modifiers, mod)
	case *ir.DistilledClass:
		n.Modifiers = append(n.Modifiers, mod)
	case *ir.DistilledInterface:
		n.Modifiers = append(n.Modifiers, mod)
	case *ir.DistilledTypeAlias:
		n.Modifiers = append(n.Modifiers, mod)
	}
}

// publishExportedNames makes public the declarations in nodes that an export
// list or `export default <name>` in the same nodes exports (`const q = 1;
// export { q };`): they belong to the module's API although their own
// statement has no `export`, so it is not added to them.
func publishExportedNames(nodes []ir.DistilledNode) {
	exported := map[string]bool{}
	for _, n := range nodes {
		if exp, ok := n.(*ir.DistilledExport); ok {
			for _, name := range esutil.LocalNames(exp) {
				exported[name] = true
			}
		}
	}
	if len(exported) == 0 {
		return
	}
	for _, n := range nodes {
		var name string
		switch d := n.(type) {
		case *ir.DistilledFunction:
			name = d.Name
		case *ir.DistilledField:
			name = d.Name
		case *ir.DistilledClass:
			name = d.Name
		case *ir.DistilledInterface:
			name = d.Name
		case *ir.DistilledTypeAlias:
			name = d.Name
		case *ir.DistilledEnum:
			name = d.Name
		default:
			continue
		}
		if exported[name] {
			makePublic(n)
		}
	}
}

// parseClass parses class declarations
func (p *ASTParser) parseClass(node *sitter.Node, isExported bool) *ir.DistilledClass {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}

	// Parse class name
	nameNode := p.findChild(node, "type_identifier")
	if nameNode != nil {
		class.Name = p.nodeText(nameNode)
	}

	// Parse modifiers
	if p.hasModifier(node, "abstract") {
		class.Modifiers = append(class.Modifiers, ir.ModifierAbstract)
	}
	if isExported {
		class.Modifiers = append(class.Modifiers, ir.ModifierExport)
	}

	// Parse type parameters
	typeParams := p.findChild(node, "type_parameters")
	if typeParams != nil {
		class.TypeParams = p.parseTypeParameters(typeParams)
	}

	// Parse the extends and implements clauses
	if heritage := p.findChild(node, "class_heritage"); heritage != nil {
		for i := 0; i < int(heritage.NamedChildCount()); i++ {
			switch clause := heritage.NamedChild(uint(i)); clause.Kind() {
			case "extends_clause":
				class.Extends = append(class.Extends, p.heritageTypes(clause)...)
			case "implements_clause":
				class.Implements = append(class.Implements, p.heritageTypes(clause)...)
			}
		}
	}

	// Parse class body
	classBody := p.findChild(node, "class_body")
	if classBody != nil {
		p.parseClassBody(classBody, class)
	}

	return class
}

// parseClassBody parses class body members. They are emitted fields first,
// then the constructor, then methods; the comments before a member move with
// it, so a doc comment stays directly before the member it documents.
func (p *ASTParser) parseClassBody(node *sitter.Node, class *ir.DistilledClass) {
	// First pass: collect all fields including parameter properties
	var fields []ir.DistilledNode
	var methods []ir.DistilledNode
	var constructorGroup []ir.DistilledNode
	var pendingDecorators []string
	var pendingComments []ir.DistilledNode

	// withComments returns member preceded by the comments waiting for it.
	withComments := func(member ir.DistilledNode) []ir.DistilledNode {
		group := append(pendingComments, member)
		pendingComments = nil
		return group
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "comment":
			pendingComments = append(pendingComments, p.parseComment(child))
		case "decorated_definition":
			// Collect decorators and locate the real definition
			var localDecs []string
			var def *sitter.Node
			for j := 0; j < int(child.ChildCount()); j++ {
				sub := child.Child(uint(j))
				if sub == nil {
					continue
				}
				if sub.Kind() == "decorator" {
					if d := p.parseDecorator(sub); d != "" {
						localDecs = append(localDecs, d)
					}
				} else {
					def = sub // first non-decorator child = wrapped definition
				}
			}
			if def != nil {
				switch def.Kind() {
				case "method_definition":
					if m := p.parseMethod(def); m != nil {
						m.Decorators = append(m.Decorators, localDecs...)
						if m.Name == "constructor" {
							constructorGroup = withComments(m)
							for k := 0; k < int(def.ChildCount()); k++ {
								if fc := def.Child(uint(k)); fc != nil && fc.Kind() == "formal_parameters" {
									for _, f := range p.parseParameterProperties(fc) {
										fields = append(fields, f)
									}
									break
								}
							}
						} else {
							methods = append(methods, withComments(m)...)
						}
					}
				case "public_field_definition", "private_field_definition", "protected_field_definition":
					if f := p.parseField(def); f != nil {
						f.Decorators = append(f.Decorators, localDecs...)
						fields = append(fields, withComments(f)...)
					}
				}
			}
		case "decorator":
			// Collect decorators for the next method/field
			if decoratorText := p.parseDecorator(child); decoratorText != "" {
				pendingDecorators = append(pendingDecorators, decoratorText)
			}
		case "method_definition":
			if method := p.parseMethod(child); method != nil {
				// Attach any pending decorators
				if len(pendingDecorators) > 0 {
					method.Decorators = append(method.Decorators, pendingDecorators...)
					pendingDecorators = nil // Reset for next item
				}

				if method.Name == "constructor" {
					constructorGroup = withComments(method)
					// Find parameter properties from constructor
					for j := 0; j < int(child.ChildCount()); j++ {
						methodChild := child.Child(uint(j))
						if methodChild != nil && methodChild.Kind() == "formal_parameters" {
							paramProps := p.parseParameterProperties(methodChild)
							// Add parameter properties to fields
							for _, field := range paramProps {
								fields = append(fields, field)
							}
							break
						}
					}
				} else {
					methods = append(methods, withComments(method)...)
				}
			}
		case "public_field_definition", "private_field_definition", "protected_field_definition":
			if field := p.parseField(child); field != nil {
				// Attach any pending decorators
				if len(pendingDecorators) > 0 {
					field.Decorators = append(field.Decorators, pendingDecorators...)
					pendingDecorators = nil // Reset for next item
				}
				fields = append(fields, withComments(field)...)
			}
		case "method_signature", "abstract_method_signature":
			if method := p.parseMethodSignature(child); method != nil {
				// Attach any pending decorators
				if len(pendingDecorators) > 0 {
					method.Decorators = append(method.Decorators, pendingDecorators...)
					pendingDecorators = nil // Reset for next item
				}

				// Mark as abstract if it's an abstract_method_signature
				// (and parseMethodSignature didn't already add it)
				if child.Kind() == "abstract_method_signature" {
					hasAbstract := false
					for _, mod := range method.Modifiers {
						if mod == ir.ModifierAbstract {
							hasAbstract = true
							break
						}
					}
					if !hasAbstract {
						method.Modifiers = append(method.Modifiers, ir.ModifierAbstract)
					}
				}
				methods = append(methods, withComments(method)...)
			}
		default:
			// A member that is not emitted (an index signature, a static
			// block): the docs before it documented it, not the next member.
			if child.IsNamed() {
				pendingComments = commentutil.WithoutDocs(pendingComments)
			}
		}
	}

	// Add members in the correct order: fields first, then constructor, then
	// methods; comments after the last member close the body.
	class.Children = append(class.Children, fields...)
	class.Children = append(class.Children, constructorGroup...)
	class.Children = append(class.Children, methods...)
	class.Children = append(class.Children, pendingComments...)
}

// parseMethod parses method definitions
func (p *ASTParser) parseMethod(node *sitter.Node) *ir.DistilledFunction {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
	}

	// Parse accessibility modifiers and decorators
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "decorator":
			// Process decorators that are children of the method_definition
			if decoratorText := p.parseDecorator(child); decoratorText != "" {
				method.Decorators = append(method.Decorators, decoratorText)
			}
		case "accessibility_modifier":
			switch p.nodeText(child) {
			case "private":
				method.Visibility = ir.VisibilityPrivate
			case "protected":
				method.Visibility = ir.VisibilityProtected
			case "public":
				method.Visibility = ir.VisibilityPublic
			}
		case "static":
			method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
		case "async":
			method.Modifiers = append(method.Modifiers, ir.ModifierAsync)
		case "abstract":
			method.Modifiers = append(method.Modifiers, ir.ModifierAbstract)
		case "readonly":
			method.Modifiers = append(method.Modifiers, ir.ModifierReadonly)
		case "property_name":
			if nameChild := child.Child(0); nameChild != nil {
				if nameChild.Kind() == "identifier" || nameChild.Kind() == "property_identifier" {
					method.Name = p.nodeText(nameChild)
				}
			}
		case "property_identifier":
			method.Name = p.nodeText(child)
		case "private_property_identifier":
			// #name method or accessor: keep the name and mark it private
			method.Name = p.nodeText(child)
			method.Visibility = ir.VisibilityPrivate
		case "type_parameters":
			method.TypeParams = p.parseTypeParameters(child)
		case "formal_parameters":
			method.Parameters = p.parseParameters(child)
		case "type_annotation", "type_predicate_annotation", "asserts_annotation":
			method.Returns = p.typeFromAnnotation(child)
		case "statement_block":
			method.Implementation = p.nodeText(child)
		}
	}
	method.Name = p.accessorName(node, method.Name)

	return method
}

// parseMethodSignature parses method signatures (in interfaces)
func (p *ASTParser) parseMethodSignature(node *sitter.Node) *ir.DistilledFunction {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Parse method signature components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "accessibility_modifier":
			switch p.nodeText(child) {
			case "private":
				method.Visibility = ir.VisibilityPrivate
			case "protected":
				method.Visibility = ir.VisibilityProtected
			case "public":
				method.Visibility = ir.VisibilityPublic
			}
		case "abstract":
			method.Modifiers = append(method.Modifiers, ir.ModifierAbstract)
		case "property_name":
			if nameChild := child.Child(0); nameChild != nil {
				if nameChild.Kind() == "identifier" || nameChild.Kind() == "property_identifier" {
					method.Name = p.nodeText(nameChild)
				}
			}
		case "property_identifier":
			method.Name = p.nodeText(child)
		case "type_parameters":
			method.TypeParams = p.parseTypeParameters(child)
		case "formal_parameters":
			method.Parameters = p.parseParameters(child)
		case "type_annotation", "type_predicate_annotation", "asserts_annotation":
			method.Returns = p.typeFromAnnotation(child)
		}
	}
	method.Name = p.accessorName(node, method.Name)

	return method
}

// parseField parses field definitions
func (p *ASTParser) parseField(node *sitter.Node) *ir.DistilledField {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{},
	}

	// Debug
	// fmt.Printf("Parsing field, node type: %s, children: %d\n", node.Kind(), node.ChildCount())
	// for i := 0; i < int(node.ChildCount()); i++ {
	// 	child := node.Child(i)
	// 	if child != nil {
	// 		fmt.Printf("  Child %d: type=%s, text=%q\n", i, child.Kind(), p.nodeText(child))
	// 	}
	// }

	// Parse field components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "accessibility_modifier":
			switch p.nodeText(child) {
			case "private":
				field.Visibility = ir.VisibilityPrivate
			case "protected":
				field.Visibility = ir.VisibilityProtected
			case "public":
				field.Visibility = ir.VisibilityPublic
			}
		case "static":
			field.Modifiers = append(field.Modifiers, ir.ModifierStatic)
		case "readonly":
			field.Modifiers = append(field.Modifiers, ir.ModifierReadonly)
		case "property_name":
			if nameChild := child.Child(0); nameChild != nil {
				switch nameChild.Kind() {
				case "identifier", "property_identifier":
					field.Name = p.nodeText(nameChild)
				case "private_property_identifier":
					field.Name = p.nodeText(nameChild)
					field.Visibility = ir.VisibilityPrivate
				}
			}
		case "property_identifier":
			field.Name = p.nodeText(child)
		case "private_property_identifier":
			field.Name = p.nodeText(child)
			field.Visibility = ir.VisibilityPrivate
		case "type_annotation":
			field.Type = p.typeFromAnnotation(child)
		case "identifier": // For simple field declarations
			field.Name = p.nodeText(child)
		}
	}

	// Look for field initializer (e.g., = null)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child != nil && child.Kind() == "=" {
			// Next child should be the initializer value
			if i+1 < int(node.ChildCount()) {
				valueNode := node.Child(uint(i + 1))
				if valueNode != nil {
					field.DefaultValue = p.nodeText(valueNode)
				}
			}
			break
		}
	}

	return field
}

// parseInterface parses interface declarations
func (p *ASTParser) parseInterface(node *sitter.Node, isExported bool) *ir.DistilledInterface {
	intf := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}

	if isExported {
		intf.Modifiers = append(intf.Modifiers, ir.ModifierExport)
	}

	// Debug: print interface AST structure
	// fmt.Printf("Interface AST for node at line %d:\n", node.StartPosition().Row+1)
	// for i := 0; i < int(node.ChildCount()); i++ {
	// 	child := node.Child(i)
	// 	if child != nil {
	// 		fmt.Printf("  Child %d: type=%s, text=%q\n", i, child.Kind(), p.nodeText(child))
	// 	}
	// }

	// Parse interface name
	nameNode := p.findChild(node, "type_identifier")
	if nameNode != nil {
		intf.Name = p.nodeText(nameNode)
	}

	// Parse type parameters
	typeParams := p.findChild(node, "type_parameters")
	if typeParams != nil {
		intf.TypeParams = p.parseTypeParameters(typeParams)
	}

	// Parse extends clause
	if extendsClause := p.findChild(node, "extends_type_clause"); extendsClause != nil {
		intf.Extends = p.heritageTypes(extendsClause)
	}

	// Parse interface body
	interfaceBody := p.findChild(node, "interface_body")
	if interfaceBody != nil {
		p.parseInterfaceBody(interfaceBody, intf)
	}

	return intf
}

// heritageTypes returns the bases an extends, implements or interface
// extends clause names, each as written: `B<string>`, `ns.Base<T>`,
// `React.Component<{ x: number }>`, `mixin(A, B)`. A class's extends clause
// gives the base expression and its type arguments as two children, which
// are joined.
func (p *ASTParser) heritageTypes(clause *sitter.Node) []ir.TypeRef {
	var refs []ir.TypeRef
	for i := 0; i < int(clause.NamedChildCount()); i++ {
		child := clause.NamedChild(uint(i))
		switch {
		case child.Kind() == "comment":
		case child.Kind() == "type_arguments" && len(refs) > 0:
			refs[len(refs)-1].Name += esutil.Signature(child, p.source)
		default:
			refs = append(refs, ir.TypeRef{Name: esutil.Signature(child, p.source)})
		}
	}
	return refs
}

// parseInterfaceBody parses interface body members
func (p *ASTParser) parseInterfaceBody(node *sitter.Node, intf *ir.DistilledInterface) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "property_signature":
			if field := p.parsePropertySignature(child); field != nil {
				intf.Children = append(intf.Children, field)
			}
		case "method_signature":
			if method := p.parseMethodSignature(child); method != nil {
				intf.Children = append(intf.Children, method)
			}
		}
	}
}

// parsePropertySignature parses property signatures
func (p *ASTParser) parsePropertySignature(node *sitter.Node) *ir.DistilledField {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{},
	}

	// Parse property signature components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "readonly":
			field.Modifiers = append(field.Modifiers, ir.ModifierReadonly)
		case "property_name":
			if nameChild := child.Child(0); nameChild != nil {
				if nameChild.Kind() == "identifier" || nameChild.Kind() == "property_identifier" {
					field.Name = p.nodeText(nameChild)
				}
			}
		case "property_identifier":
			field.Name = p.nodeText(child)
		case "type_annotation":
			// Extract the actual type from the type annotation (skip the colon)
			for j := 0; j < int(child.ChildCount()); j++ {
				typeChild := child.Child(uint(j))
				if typeChild != nil && typeChild.Kind() != ":" {
					field.Type = &ir.TypeRef{Name: p.nodeText(typeChild)}
					break
				}
			}
		}
	}

	return field
}

// parseTypeAlias parses type alias declarations
func (p *ASTParser) parseTypeAlias(node *sitter.Node, isExported bool) *ir.DistilledTypeAlias {
	alias := &ir.DistilledTypeAlias{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Modifiers:  []ir.Modifier{},
	}

	if isExported {
		alias.Modifiers = append(alias.Modifiers, ir.ModifierExport)
	}

	// Parse type alias name
	nameNode := p.findChild(node, "type_identifier")
	if nameNode != nil {
		alias.Name = p.nodeText(nameNode)
	}

	// Parse type parameters
	typeParams := p.findChild(node, "type_parameters")
	if typeParams != nil {
		alias.TypeParams = p.parseTypeParameters(typeParams)
	}

	// Parse the aliased type by looking for any type-like node after the =
	// The type could be a union_type, intersection_type, or other type node
	foundEquals := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		// Skip until we find the equals sign
		if !foundEquals && p.nodeText(child) == "=" {
			foundEquals = true
			continue
		}

		// After equals, the next non-punctuation node is our type
		if foundEquals && child.Kind() != "=" {
			alias.Type = ir.TypeRef{Name: p.nodeText(child)}
			break
		}
	}

	return alias
}

// parseEnum parses enum declarations
func (p *ASTParser) parseEnum(node *sitter.Node, isExported bool) *ir.DistilledEnum {
	enum := &ir.DistilledEnum{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Children:   []ir.DistilledNode{},
	}

	// Parse enum name
	nameNode := p.findChild(node, "identifier")
	if nameNode != nil {
		enum.Name = p.nodeText(nameNode)
	}

	// Parse enum body
	enumBody := p.findChild(node, "enum_body")
	if enumBody != nil {
		p.parseEnumBody(enumBody, enum)
	}

	return enum
}

// parseEnumBody parses enum body members
func (p *ASTParser) parseEnumBody(node *sitter.Node, enum *ir.DistilledEnum) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		if child.Kind() == "enum_assignment" {
			// Parse enum assignment like ADMIN = "admin"
			nameNode := p.findChild(child, "property_identifier")
			if nameNode != nil {
				field := &ir.DistilledField{
					BaseNode: ir.BaseNode{
						Location: p.nodeToLocation(child),
					},
					Name:       p.nodeText(nameNode),
					Visibility: ir.VisibilityPublic,
					Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierReadonly},
				}

				// The initialiser as written: a literal, "-1", "1 << 2", "Other.X"
				if value := child.ChildByFieldName("value"); value != nil {
					field.DefaultValue = strings.Join(strings.Fields(p.nodeText(value)), " ")
				}

				enum.Children = append(enum.Children, field)
			}
		} else if child.Kind() == "property_identifier" {
			// Simple enum member without assignment
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeToLocation(child),
				},
				Name:       p.nodeText(child),
				Visibility: ir.VisibilityPublic,
				Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierReadonly},
			}

			enum.Children = append(enum.Children, field)
		}
	}
}

// parseFunction parses function declarations
func (p *ASTParser) parseFunction(node *sitter.Node, isExported bool) *ir.DistilledFunction {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
	}

	if isExported {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierExport)
	}

	// Parse function components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "async":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
		case "identifier":
			fn.Name = p.nodeText(child)
			// Check for underscore prefix indicating private
			if strings.HasPrefix(fn.Name, "_") && !isExported {
				fn.Visibility = ir.VisibilityPrivate
			}
		case "type_parameters":
			fn.TypeParams = p.parseTypeParameters(child)
		case "formal_parameters":
			fn.Parameters = p.parseParameters(child)
		case "type_annotation", "type_predicate_annotation", "asserts_annotation":
			fn.Returns = p.typeFromAnnotation(child)
		case "statement_block":
			fn.Implementation = p.nodeText(child)
		}
	}
	fn.Name = p.accessorName(node, fn.Name)

	return fn
}

// accessorName returns name as the JavaScript processor names a method:
// `get size` and `set size` for an accessor, `*iter` for a generator (a
// generator function declaration too). The get, set and * tokens are
// anonymous children of the method or function node; a method that is
// itself named get has a property_identifier name instead.
func (p *ASTParser) accessorName(node *sitter.Node, name string) string {
	for i := uint(0); i < node.ChildCount(); i++ {
		switch node.Child(i).Kind() {
		case "get", "set":
			return node.Child(i).Kind() + " " + name
		case "*":
			return "*" + name
		}
	}
	return name
}

// typeFromAnnotation returns the type of a type_annotation node, or of a
// return-type type_predicate_annotation ("x is T") or asserts_annotation
// ("asserts x is T"): its first child that is not the ":" token, as written
// on one line (esutil.Signature: `{ a: string; }` for an object type spread
// over lines), or nil when there is none.
func (p *ASTParser) typeFromAnnotation(node *sitter.Node) *ir.TypeRef {
	if node == nil {
		return nil
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		c := node.Child(uint(i))
		if c != nil && c.Kind() != ":" {
			return &ir.TypeRef{Name: esutil.Signature(c, p.source)}
		}
	}
	return nil
}

// maxConstValueLen is the longest single-line initialiser printed for a constant.
const maxConstValueLen = 80

// parseVariableDeclaration parses variable declarations. A const or variable
// initialised with an arrow function and without a type annotation is a
// function. Any other variable is a field; when its initializer is a function
// expression, a typed arrow function or a call that takes a function
// (`forwardRef((props, ref) => ...)`), the field's DefaultValue is the
// initializer's signature (esutil.Signature) and its Implementation the
// initializer as written.
func (p *ASTParser) parseVariableDeclaration(node *sitter.Node, isExported bool) []ir.DistilledNode {
	var nodes []ir.DistilledNode

	// Find variable declarators
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil || child.Kind() != "variable_declarator" {
			continue
		}

		value := child.ChildByFieldName("value")
		arrowFunc := value
		if arrowFunc != nil && (arrowFunc.Kind() != "arrow_function" || child.ChildByFieldName("type") != nil) {
			arrowFunc = nil
		}
		if arrowFunc != nil {
			// This is a function, not a field
			fn := p.parseArrowFunction(arrowFunc, isExported)
			if fn != nil {
				// Get the function name from the variable declarator
				nameNode := p.findChild(child, "identifier")
				if nameNode != nil {
					fn.Name = p.nodeText(nameNode)
					// Check for underscore prefix indicating private
					if strings.HasPrefix(fn.Name, "_") && !isExported {
						fn.Visibility = ir.VisibilityPrivate
					}
				}
				// fmt.Printf("Parsed arrow function %s with %d params\n", fn.Name, len(fn.Parameters))

				// Check if it's const (makes it immutable)
				if p.hasToken(node, "const") {
					fn.Modifiers = append(fn.Modifiers, ir.ModifierFinal)
				}

				// Add export modifier if needed
				if isExported {
					fn.Modifiers = append(fn.Modifiers, ir.ModifierExport)
				}

				nodes = append(nodes, fn)
			}
		} else {
			// Regular variable/field
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeToLocation(child),
				},
				Visibility: p.getVisibility(isExported),
				Modifiers:  []ir.Modifier{},
			}

			// Check if it's const
			if p.hasToken(node, "const") {
				field.Modifiers = append(field.Modifiers, ir.ModifierFinal)
			}

			// Add export modifier if needed
			if isExported {
				field.Modifiers = append(field.Modifiers, ir.ModifierExport)
			}

			// Parse variable declarator
			nameNode := p.findChild(child, "identifier")
			if nameNode != nil {
				field.Name = p.nodeText(nameNode)
				// Check for underscore prefix indicating private
				if strings.HasPrefix(field.Name, "_") && !isExported {
					field.Visibility = ir.VisibilityPrivate
				}
			}

			typeAnnotation := p.findChild(child, "type_annotation")
			if typeAnnotation != nil {
				field.Type = p.typeFromAnnotation(typeAnnotation)
			}

			// A function, or a call that takes one, keeps its signature and
			// its full text; otherwise keep short single-line initialisers,
			// as large literals would bloat the output.
			switch {
			case value == nil:
			case esutil.IsFunction(value) || esutil.TakesFunction(value):
				field.DefaultValue = esutil.Signature(value, p.source)
				field.Implementation = p.nodeText(value)
			default:
				if v := p.nodeText(value); !strings.Contains(v, "\n") && len(v) <= maxConstValueLen {
					field.DefaultValue = v
				}
			}

			nodes = append(nodes, field)
		}
	}

	return nodes
}

// parseArrowFunction parses arrow function expressions
func (p *ASTParser) parseArrowFunction(node *sitter.Node, isExported bool) *ir.DistilledFunction {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeToLocation(node),
		},
		Visibility: p.getVisibility(isExported),
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
	}

	// Debug: print the AST structure
	// fmt.Printf("\nArrow function AST for node at line %d:\n", node.StartPosition().Row+1)
	// for i := 0; i < int(node.ChildCount()); i++ {
	// 	child := node.Child(i)
	// 	if child != nil {
	// 		fmt.Printf("  Child %d: type=%s, text=%q\n", i, child.Kind(), p.nodeText(child))
	// 	}
	// }

	// Parse arrow function components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil {
			continue
		}

		switch child.Kind() {
		case "async":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
		case "type_parameters":
			fn.TypeParams = p.parseTypeParameters(child)
		case "formal_parameters":
			fn.Parameters = p.parseParameters(child)
		case "identifier":
			// Single parameter without parentheses (e.g., x => x * 2)
			if len(fn.Parameters) == 0 && i == 0 {
				fn.Parameters = append(fn.Parameters, ir.Parameter{
					Name: p.nodeText(child),
				})
			}
		case "type_annotation", "type_predicate_annotation", "asserts_annotation":
			fn.Returns = p.typeFromAnnotation(child)
		case "statement_block":
			fn.Implementation = p.nodeText(child)
		case "=>":
			// Arrow token, skip
			continue
		default:
			// If it's not one of the above and we're past the arrow, it's likely the body
			if fn.Implementation == "" && child.Kind() != "=>" {
				// Check if we've seen the arrow already
				hasArrow := false
				for j := 0; j < i; j++ {
					if prev := node.Child(uint(j)); prev != nil && prev.Kind() == "=>" {
						hasArrow = true
						break
					}
				}
				if hasArrow {
					fn.Implementation = p.nodeText(child)
				}
			}
		}
	}

	// An expression body that is a bare identifier (x => x) is mistaken for a
	// parameter above; the body field is authoritative.
	if fn.Implementation == "" {
		if body := node.ChildByFieldName("body"); body != nil {
			fn.Implementation = p.nodeText(body)
		}
	}

	return fn
}

// parseParameterProperties returns the fields that the parameter properties
// of a constructor's formal_parameters declare: a parameter with an
// accessibility modifier or `readonly` (`private readonly s: S` is a private
// readonly field s, `readonly t: T` a public one).
func (p *ASTParser) parseParameterProperties(node *sitter.Node) []*ir.DistilledField {
	var fields []*ir.DistilledField
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "required_parameter" && child.Kind() != "optional_parameter" {
			continue
		}
		pattern := child.ChildByFieldName("pattern")
		if pattern == nil || pattern.Kind() != "identifier" {
			continue
		}
		field := &ir.DistilledField{
			BaseNode:  ir.BaseNode{Location: p.nodeToLocation(child)},
			Name:      p.nodeText(pattern),
			Type:      p.typeFromAnnotation(child.ChildByFieldName("type")),
			Modifiers: []ir.Modifier{},
		}
		isProperty := false
		for j := uint(0); j < child.ChildCount(); j++ {
			switch sub := child.Child(j); sub.Kind() {
			case "accessibility_modifier":
				isProperty = true
				switch p.nodeText(sub) {
				case "private":
					field.Visibility = ir.VisibilityPrivate
				case "protected":
					field.Visibility = ir.VisibilityProtected
				default:
					field.Visibility = ir.VisibilityPublic
				}
			case "readonly":
				isProperty = true
				field.Modifiers = append(field.Modifiers, ir.ModifierReadonly)
			}
		}
		if !isProperty {
			continue
		}
		if field.Visibility == "" {
			field.Visibility = ir.VisibilityPublic
		}
		fields = append(fields, field)
	}
	return fields
}

// parseParameters returns the parameters of a formal_parameters node as
// written. A destructuring pattern is the parameter's name, as its source
// text on one line (`{ a, b = 1 }`, `[x, y]`); `...rest` is variadic, `c?`
// optional, `this` a parameter named this; a default value keeps its text
// with function bodies left out. The accessibility and readonly modifiers of
// a parameter property are not part of the parameter:
// parseParameterProperties makes them fields.
func (p *ASTParser) parseParameters(node *sitter.Node) []ir.Parameter {
	var params []ir.Parameter
	for i := uint(0); i < node.NamedChildCount(); i++ {
		child := node.NamedChild(i)
		if child.Kind() != "required_parameter" && child.Kind() != "optional_parameter" {
			continue
		}
		param := ir.Parameter{IsOptional: child.Kind() == "optional_parameter"}
		pattern := child.ChildByFieldName("pattern")
		if pattern != nil && pattern.Kind() == "rest_pattern" {
			param.IsVariadic = true
			pattern = pattern.NamedChild(0)
		}
		if pattern != nil {
			param.Name = esutil.Signature(pattern, p.source)
		}
		if t := p.typeFromAnnotation(child.ChildByFieldName("type")); t != nil {
			param.Type = *t
		}
		if value := child.ChildByFieldName("value"); value != nil {
			param.DefaultValue = esutil.Signature(value, p.source)
		}
		params = append(params, param)
	}
	return params
}

// parseTypeParameters parses type parameters
func (p *ASTParser) parseTypeParameters(node *sitter.Node) []ir.TypeParam {
	var typeParams []ir.TypeParam

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child == nil || child.Kind() != "type_parameter" {
			continue
		}

		param := ir.TypeParam{}

		// Parse type parameter components
		nameNode := p.findChild(child, "type_identifier")
		if nameNode != nil {
			param.Name = p.nodeText(nameNode)
		}

		constraint := p.findChild(child, "constraint")
		if constraint != nil {
			// The constraint contains "extends" followed by the type
			// We need to extract the type part after "extends"
			constraintText := p.nodeText(constraint)
			if strings.HasPrefix(constraintText, "extends ") {
				constraintType := strings.TrimPrefix(constraintText, "extends ")
				param.Constraints = append(param.Constraints, ir.TypeRef{
					Name: constraintType,
				})
			}
		}

		// The default type: `= {}` in `T extends object = {}`.
		if value := child.ChildByFieldName("value"); value != nil {
			if def := value.NamedChild(0); def != nil {
				param.Default = &ir.TypeRef{Name: esutil.Signature(def, p.source)}
			}
		}

		typeParams = append(typeParams, param)
	}

	return typeParams
}

// parseDecorator parses decorator expressions
func (p *ASTParser) parseDecorator(node *sitter.Node) string {
	// TypeScript decorators are like @LogExecution or @Component({...})
	decoratorText := p.nodeText(node)

	// Remove the leading @ symbol if present
	if strings.HasPrefix(decoratorText, "@") {
		decoratorText = decoratorText[1:]
	}

	return decoratorText
}

// Helper functions

func (p *ASTParser) nodeToLocation(node *sitter.Node) ir.Location {
	return ir.Location{
		StartLine:   int(node.StartPosition().Row) + 1,
		StartColumn: int(node.StartPosition().Column) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		EndColumn:   int(node.EndPosition().Column) + 1,
		StartByte:   int(node.StartByte()),
		EndByte:     int(node.EndByte()),
	}
}

func (p *ASTParser) nodeText(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	sourceLen := uint(len(p.source))
	if start > end || end > sourceLen {
		return ""
	}
	return string(p.source[start:end])
}

func (p *ASTParser) findChild(node *sitter.Node, childType string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child != nil && child.Kind() == childType {
			return child
		}
	}
	return nil
}

func (p *ASTParser) hasModifier(node *sitter.Node, modifier string) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child != nil && child.Kind() == modifier {
			return true
		}
	}
	return false
}

func (p *ASTParser) hasToken(node *sitter.Node, token string) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child != nil && p.nodeText(child) == token {
			return true
		}
	}
	return false
}

func (p *ASTParser) getVisibility(isExported bool) ir.Visibility {
	if isExported {
		return ir.VisibilityPublic
	}
	// Non-exported items are module-private (internal)
	// They're accessible within the module but not externally
	return ir.VisibilityInternal
}

// analyzeInterfaceSatisfaction analyzes interface implementations
// This enhances explicit implements clauses with structural type analysis
func (p *ASTParser) analyzeInterfaceSatisfaction(file *ir.DistilledFile) {
	// Collect all interfaces and classes
	interfaces := make(map[string]*ir.DistilledInterface)
	classes := make(map[string]*ir.DistilledClass)

	for _, child := range file.Children {
		switch node := child.(type) {
		case *ir.DistilledInterface:
			interfaces[node.Name] = node
		case *ir.DistilledClass:
			classes[node.Name] = node
		}
	}

	// For each class, check if it structurally satisfies interfaces it doesn't explicitly implement
	for _, class := range classes {
		// Get already explicitly implemented interfaces
		explicitImpls := make(map[string]bool)
		for _, impl := range class.Implements {
			explicitImpls[impl.Name] = true
		}

		// Check each interface for structural compatibility
		for intfName, intf := range interfaces {
			if explicitImpls[intfName] {
				continue // Already explicitly implemented
			}

			if p.classImplementsInterface(class, intf) {
				// Add implicit implementation
				class.Implements = append(class.Implements, ir.TypeRef{
					Name: intfName + " (implicit)",
				})
			}
		}
	}
}

// classImplementsInterface checks if a class structurally implements an interface
func (p *ASTParser) classImplementsInterface(class *ir.DistilledClass, intf *ir.DistilledInterface) bool {
	// Create method and property maps for the class
	// Note: This function is disabled in the current implementation
	// as structural typing detection proved too complex
	_ = class
	_ = intf
	return false
	/*
			classMethods := make(map[string]*ir.DistilledFunction)
			classProps := make(map[string]*ir.DistilledField)

			for _, child := range class.Children {
				switch member := child.(type) {
				case *ir.DistilledFunction:
					classMethods[member.Name] = member
				case *ir.DistilledField:
					classProps[member.Name] = member
				}
			}

			// Check if class satisfies all interface requirements
			for _, child := range intf.Children {
				switch requirement := child.(type) {
				case *ir.DistilledFunction:
					// Check if class has matching method
					classMethod, exists := classMethods[requirement.Name]
					if !exists {
						return false
					}

					// Check method signature compatibility
					if !p.methodsCompatible(classMethod, requirement) {
						return false
					}

				case *ir.DistilledField:
					// Check if class has matching property
					classProp, exists := classProps[requirement.Name]
					if !exists {
						return false
					}

					// Check property type compatibility
					if !p.typesCompatible(classProp.Type, requirement.Type) {
						return false
					}
				}
			}

			return true
		}

		// methodsCompatible checks if two methods are compatible
		func (p *ASTParser) methodsCompatible(classMethod, intfMethod *ir.DistilledFunction) bool {
			// Check parameter count
			if len(classMethod.Parameters) != len(intfMethod.Parameters) {
				return false
			}

			// Check parameter types
			for i, classParam := range classMethod.Parameters {
				intfParam := intfMethod.Parameters[i]
				if !p.typesCompatible(&classParam.Type, &intfParam.Type) {
					return false
				}
			}

			// Check return type
			return p.typesCompatible(classMethod.Returns, intfMethod.Returns)
	*/
}

// typesCompatible checks if two types are compatible (simplified structural check)
func (p *ASTParser) typesCompatible(type1, type2 *ir.TypeRef) bool {
	if type1 == nil && type2 == nil {
		return true
	}
	if type1 == nil || type2 == nil {
		return false
	}

	// Simple string comparison for now
	// In a full implementation, this would be more sophisticated
	return strings.TrimSpace(type1.Name) == strings.TrimSpace(type2.Name)
}
