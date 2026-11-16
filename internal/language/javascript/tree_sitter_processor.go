package javascript

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/janreges/ai-distiller/internal/language/esutil"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
)

// TreeSitterProcessor processes JavaScript using tree-sitter
type TreeSitterProcessor struct {
	parser   *sitter.Parser
	source   []byte
	filename string

	// State for context-aware parsing
	currentClass string
	insideClass  bool

	// JSDoc information by line number
	jsdocComments map[int]*JSDocInfo

	// hasJSX is set when the first pass meets a JSX node
	hasJSX bool
}

// JSDocInfo stores parsed JSDoc information
type JSDocInfo struct {
	description string
	paramTypes  map[string]string
	returnType  string
	typedefType string
	isPrivate   bool
	isProtected bool
	isPublic    bool
	isStatic    bool
	isAsync     bool
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() (*TreeSitterProcessor, error) {
	parser, err := tsparse.NewParser(sitter.NewLanguage(tree_sitter_javascript.Language()))
	if err != nil {
		return nil, err
	}

	return &TreeSitterProcessor{
		parser:        parser,
		jsdocComments: make(map[int]*JSDocInfo),
	}, nil
}

// ProcessSource processes JavaScript source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	p.source = source
	p.filename = filename

	// Reset state
	p.currentClass = ""
	p.insideClass = false
	p.jsdocComments = make(map[int]*JSDocInfo)
	p.hasJSX = false

	// Parse with tree-sitter
	tree, err := tsparse.Parse(ctx, p.parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Close()

	// Create IR file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPosition().Row) + 1,
			},
		},
		Path:     filename,
		Language: "javascript",
		Version:  "ES2022",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// First pass: collect all JSDoc comments, and note JSX
	p.collectJSDocComments(tree.RootNode())
	if p.hasJSX {
		file.Extensions = &ir.NodeExtensions{JavaScript: &ir.JavaScriptExtensions{HasJSX: true}}
	}

	// Second pass: process nodes with JSDoc info available
	p.processNode(tree.RootNode(), file, nil)

	return file, nil
}

// collectJSDocComments collects all JSDoc comments in first pass
func (p *TreeSitterProcessor) collectJSDocComments(node *sitter.Node) {
	if node == nil {
		return
	}

	nodeType := node.Kind()
	if strings.HasPrefix(nodeType, "jsx_") {
		p.hasJSX = true
	}

	// Check if this is a JSDoc comment
	if nodeType == "comment" {
		text := p.getNodeText(node)
		if strings.HasPrefix(text, "/**") && !strings.HasPrefix(text, "/***") {
			// Parse JSDoc
			info := p.parseJSDoc(text)
			if info != nil {
				// Store by the line of the NEXT non-comment node
				nextLine := p.findNextCodeLine(node)
				if nextLine > 0 {
					p.jsdocComments[nextLine] = info
				}
			}
		}
	}

	// Process children
	for i := 0; i < int(node.ChildCount()); i++ {
		p.collectJSDocComments(node.Child(uint(i)))
	}
}

// findNextCodeLine finds the line number of the next code element after a comment
func (p *TreeSitterProcessor) findNextCodeLine(commentNode *sitter.Node) int {
	parent := commentNode.Parent()
	if parent == nil {
		return 0
	}

	foundComment := false
	for i := 0; i < int(parent.ChildCount()); i++ {
		child := parent.Child(uint(i))
		if child.Equals(*commentNode) {
			foundComment = true
			continue
		}

		if foundComment && child.IsNamed() {
			nodeType := child.Kind()
			// Skip other comments
			if nodeType != "comment" {
				return int(child.StartPosition().Row) + 1
			}
		}
	}

	return 0
}

// parseJSDoc parses JSDoc comment and extracts type information
func (p *TreeSitterProcessor) parseJSDoc(text string) *JSDocInfo {
	info := &JSDocInfo{
		paramTypes: make(map[string]string),
	}

	// Clean up docblock
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimSuffix(text, "*/")

	lines := strings.Split(text, "\n")
	hasTypeInfo := false

	// A tag runs on over the next lines while its braced type is open, so
	// `@type {Array<{a: number,` + `b: string}>}` is read whole. A line that
	// starts with `@` starts the next tag even then: a `{` that never closes
	// leaves its own tag untyped, not the tags after it.
	var tags []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		if n := len(tags); n > 0 && jsdocTypeOpen(tags[n-1]) && !strings.HasPrefix(line, "@") {
			tags[n-1] += " " + line
		} else if strings.HasPrefix(line, "@") {
			tags = append(tags, line)
		} else if info.description == "" {
			// First non-tag line is description
			info.description = line
		}
	}

	for _, tag := range tags {
		word, after := jsdocTag(tag)
		typ, rest, typed := jsdocType(after)
		switch word {
		case "@param":
			// @param {type} name - description
			if name := jsdocParamName(rest); typed && name != "" {
				info.paramTypes[name] = typ
				hasTypeInfo = true
			}
		case "@returns", "@return":
			// @returns {type} description
			if typed {
				info.returnType = typ
				hasTypeInfo = true
			}
		case "@type":
			// @type {type}
			if typed {
				info.typedefType = typ
				hasTypeInfo = true
			}
		case "@private":
			info.isPrivate = true
			hasTypeInfo = true
		case "@protected":
			info.isProtected = true
			hasTypeInfo = true
		case "@public":
			info.isPublic = true
			hasTypeInfo = true
		case "@static":
			info.isStatic = true
			hasTypeInfo = true
		case "@async":
			info.isAsync = true
			hasTypeInfo = true
		}
	}

	if hasTypeInfo || info.description != "" {
		return info
	}
	return nil
}

// jsdocType splits the text after a JSDoc tag into its braced type, read
// whole up to the brace that closes it (`{Object<string, {a: number}>}`
// gives `Object<string, {a: number}>`), and the text after it. ok is false
// when the text does not start with a closed `{...}`.
func jsdocType(s string) (typ, rest string, ok bool) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "{") {
		return "", s, false
	}
	depth := 0
	for i, r := range s {
		switch r {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(s[1:i]), strings.TrimSpace(s[i+1:]), true
			}
		}
	}
	return "", s, false
}

// jsdocTypeOpen reports whether a tag's braced type is not closed yet, so
// the tag goes on on the next line.
func jsdocTypeOpen(tag string) bool {
	_, after := jsdocTag(tag)
	_, _, ok := jsdocType(after)
	return strings.HasPrefix(after, "{") && !ok
}

// jsdocTag splits a tag line into its tag word (`@param`) and the text
// after it.
func jsdocTag(tag string) (word, after string) {
	fields := strings.Fields(tag)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimSpace(strings.TrimPrefix(tag, fields[0]))
}

// jsdocParamName returns the parameter a `@param {T}` tag names: `name`
// from `name`, `...name`, `[name]` and `[name=default]`.
func jsdocParamName(rest string) string {
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	name := strings.TrimPrefix(fields[0], "...")
	if strings.HasPrefix(name, "[") {
		name = strings.TrimPrefix(name, "[")
		if i := strings.IndexAny(name, "=]"); i >= 0 {
			name = name[:i]
		}
	}
	return name
}

// processNode recursively processes tree-sitter nodes
func (p *TreeSitterProcessor) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if node == nil {
		return
	}

	nodeType := node.Kind()

	// Skip non-named nodes except specific ones
	if !node.IsNamed() && nodeType != "comment" {
		// Process children for anonymous nodes
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
		return
	}

	switch nodeType {
	case "program":
		// Process all children at top level. The doc comments of a statement
		// that yields no declaration (a re-export, `module.exports = ...`, a
		// call) are dropped, so they do not document the next declaration.
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(uint(i))
			before := len(file.Children)
			p.processNode(child, file, parent)
			if child.IsNamed() && child.Kind() != "comment" {
				added := append([]ir.DistilledNode(nil), file.Children[before:]...)
				file.Children = commentutil.AppendStatement(file.Children[:before], added)
			}
		}

	case "import_statement":
		p.processImport(node, file, parent)

	case "export_statement":
		p.processExport(node, file, parent)

	case "class_declaration":
		p.processClass(node, file, parent)

	case "function_declaration":
		p.processFunction(node, file, parent, false)

	case "generator_function_declaration":
		p.processFunction(node, file, parent, false)

	case "variable_declaration":
		p.processVariableDeclaration(node, file, parent)

	case "lexical_declaration":
		p.processLexicalDeclaration(node, file, parent)

	case "comment":
		p.processComment(node, file, parent)

	case "expression_statement":
		// Check for module.exports = ...
		p.processExpressionStatement(node, file, parent)

	default:
		// Process children for other node types
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
	}
}

// processImport processes import statements. A default import is the symbol
// "default" with the local name as its alias, a namespace import the symbol
// "*" with its alias (ir.ImportedSymbol).
func (p *TreeSitterProcessor) processImport(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	imp := &ir.DistilledImport{
		BaseNode:   p.nodeLocation(node),
		ImportType: "import",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Process import parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "import_clause":
			p.processImportClause(child, imp)
		case "string":
			// Module path
			imp.Module = strings.Trim(p.getNodeText(child), "\"'`")
		}
	}

	p.addNode(file, parent, imp)
}

// processImportClause processes the import clause (what's being imported)
func (p *TreeSitterProcessor) processImportClause(node *sitter.Node, imp *ir.DistilledImport) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			// Default import: the module's "default" under a local name
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  "default",
				Alias: p.getNodeText(child),
			})
		case "named_imports":
			// { a, b as c }
			p.processNamedImports(child, imp)
		case "namespace_import":
			// * as name
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(uint(j))
				if grandchild.Kind() == "identifier" {
					imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
						Name:  "*",
						Alias: p.getNodeText(grandchild),
					})
				}
			}
		}
	}
}

// processNamedImports processes named imports { a, b as c }
func (p *TreeSitterProcessor) processNamedImports(node *sitter.Node, imp *ir.DistilledImport) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "import_specifier" {
			var name, alias string

			// Use field names for more reliable parsing
			nameNode := child.ChildByFieldName("name")
			if nameNode != nil {
				name = p.getNodeText(nameNode)
			}

			aliasNode := child.ChildByFieldName("alias")
			if aliasNode != nil {
				alias = p.getNodeText(aliasNode)
			}

			// Fallback to positional if fields not available
			if name == "" {
				for j := 0; j < int(child.ChildCount()); j++ {
					grandchild := child.Child(uint(j))
					if grandchild.Kind() == "identifier" {
						if name == "" {
							name = p.getNodeText(grandchild)
						} else {
							alias = p.getNodeText(grandchild)
						}
					}
				}
			}

			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  name,
				Alias: alias,
			})
		}
	}
}

// processExport processes export statements: an exported declaration is
// parsed as the declaration and gets the modifier export (and default after
// `export default`; `export default class {}` is a class without a name); a
// re-export, an export list or `export default <expression>` becomes a
// DistilledExport (esutil.Export).
func (p *TreeSitterProcessor) processExport(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if exp := esutil.Export(node, p.source); exp != nil {
		p.addNode(file, parent, exp)
		return
	}
	mods := []ir.Modifier{ir.ModifierExport}
	before := len(file.Children)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "default":
			mods = append(mods, ir.ModifierDefault)
		case "class_declaration", "class":
			p.processClass(child, file, parent)
		case "function_declaration", "generator_function_declaration":
			p.processFunction(child, file, parent, false)
		case "lexical_declaration":
			p.processLexicalDeclaration(child, file, parent)
		case "variable_declaration":
			p.processVariableDeclaration(child, file, parent)
		}
	}
	if parent != nil {
		return
	}
	for _, n := range file.Children[before:] {
		switch d := n.(type) {
		case *ir.DistilledClass:
			d.Modifiers = append(d.Modifiers, mods...)
		case *ir.DistilledFunction:
			d.Modifiers = append(d.Modifiers, mods...)
		case *ir.DistilledField:
			d.Modifiers = append(d.Modifiers, mods...)
		}
	}
}

// processClass processes class declarations
func (p *TreeSitterProcessor) processClass(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Classes are public by default in JS
		Modifiers:  []ir.Modifier{},
		Extends:    []ir.TypeRef{},
		Implements: []ir.TypeRef{},
		Children:   []ir.DistilledNode{},
		Decorators: []string{},
	}

	// Track current class
	prevClass := p.currentClass
	prevInsideClass := p.insideClass
	p.insideClass = true

	// Check for JSDoc
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists {
		if jsdoc.isPrivate {
			class.Visibility = ir.VisibilityPrivate
		} else if jsdoc.isProtected {
			class.Visibility = ir.VisibilityProtected
		}
	}

	// Process class parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			class.Name = p.getNodeText(child)
			p.currentClass = class.Name

		case "class_heritage":
			// extends clause
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(uint(j))
				if grandchild.Kind() == "identifier" || grandchild.Kind() == "member_expression" {
					class.Extends = append(class.Extends, ir.TypeRef{
						Name: p.getNodeText(grandchild),
					})
				}
			}

		case "class_body":
			// Process class members
			p.processClassBody(child, file, class)
		}
	}

	p.addNode(file, parent, class)

	// Restore previous class context
	p.currentClass = prevClass
	p.insideClass = prevInsideClass
}

// processClassBody processes the body of a class
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, file *ir.DistilledFile, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "comment":
			p.processComment(child, file, class)

		case "method_definition":
			p.processMethod(child, file, class)

		case "field_definition":
			p.processField(child, file, class)

		case "static_block":
			// Static initialization block
			comment := &ir.DistilledComment{
				BaseNode: p.nodeLocation(child),
				Text:     "static initialization block",
				Format:   ir.CommentBlock,
			}
			p.addNode(file, class, comment)
		}
	}
}

// processMethod processes method definitions
func (p *TreeSitterProcessor) processMethod(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Methods are public by default
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
		Decorators: []string{},
	}

	var isGetter, isSetter, isStatic, isAsync, isGenerator bool
	var propertyName string

	// Check for JSDoc
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists {
		if jsdoc.isPrivate {
			fn.Visibility = ir.VisibilityPrivate
		} else if jsdoc.isProtected {
			fn.Visibility = ir.VisibilityProtected
		}
		if jsdoc.returnType != "" {
			fn.Returns = &ir.TypeRef{Name: jsdoc.returnType}
		}
		// We'll apply param types when processing parameters
	}

	// Process method parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		text := p.getNodeText(child)

		switch child.Kind() {
		case "get":
			isGetter = true
		case "set":
			isSetter = true
		case "static":
			isStatic = true
		case "async":
			isAsync = true
		case "*":
			isGenerator = true

		case "property_identifier":
			propertyName = text
			// Underscore convention: private unless JSDoc says otherwise
			if isUnderscorePrivate(text) && fn.Visibility == ir.VisibilityPublic && !p.jsdocMarksPublic(nodeLine) {
				fn.Visibility = ir.VisibilityPrivate
			}

		case "private_property_identifier":
			// Private field/method with # prefix
			propertyName = text
			fn.Visibility = ir.VisibilityPrivate

		case "computed_property_name":
			// [expression]
			propertyName = fmt.Sprintf("[%s]", p.getNodeText(child))

		case "formal_parameters":
			p.processParameters(child, fn)

		case "statement_block":
			// Method body
			startByte := child.StartByte()
			endByte := child.EndByte()
			if int(startByte) < len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}
		}
	}

	// Set method name and modifiers
	if isGetter {
		fn.Name = fmt.Sprintf("get %s", propertyName)
	} else if isSetter {
		fn.Name = fmt.Sprintf("set %s", propertyName)
	} else {
		fn.Name = propertyName
	}

	if isStatic {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierStatic)
	}
	if isAsync {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}
	if isGenerator {
		fn.Name = "*" + fn.Name
	}

	// Special case for constructor
	if fn.Name == "constructor" {
		fn.Name = "constructor"
		// No return type for constructors
		fn.Returns = nil
	}

	p.addNode(file, parent, fn)
}

// isUnderscorePrivate reports whether name follows the leading-underscore
// "private by convention" rule.
func isUnderscorePrivate(name string) bool {
	return strings.HasPrefix(name, "_")
}

// jsdocMarksPublic reports whether the JSDoc block attached to line carries @public.
func (p *TreeSitterProcessor) jsdocMarksPublic(line int) bool {
	jsdoc, ok := p.jsdocComments[line]
	return ok && jsdoc.isPublic
}

// processField processes field definitions
func (p *TreeSitterProcessor) processField(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Fields are public by default
		Modifiers:  []ir.Modifier{},
	}

	var isStatic bool

	// Check for JSDoc
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists {
		if jsdoc.isPrivate {
			field.Visibility = ir.VisibilityPrivate
		} else if jsdoc.isProtected {
			field.Visibility = ir.VisibilityProtected
		}
		if jsdoc.typedefType != "" {
			field.Type = &ir.TypeRef{Name: jsdoc.typedefType}
		}
	}

	// Process field parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "static":
			isStatic = true

		case "property_identifier":
			field.Name = p.getNodeText(child)
			// Underscore convention: private unless JSDoc says otherwise
			if isUnderscorePrivate(field.Name) && field.Visibility == ir.VisibilityPublic && !p.jsdocMarksPublic(nodeLine) {
				field.Visibility = ir.VisibilityPrivate
			}

		case "private_property_identifier":
			// Private field with # prefix
			field.Name = p.getNodeText(child)
			field.Visibility = ir.VisibilityPrivate

		default:
			// Initializer expression
			if child.Kind() != "=" && field.DefaultValue == "" {
				field.DefaultValue = p.getNodeText(child)
			}
		}
	}

	if isStatic {
		field.Modifiers = append(field.Modifiers, ir.ModifierStatic)
	}

	p.addNode(file, parent, field)
}

// processFunction processes function declarations
func (p *TreeSitterProcessor) processFunction(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, isAsync bool) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Functions are public by default
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
		Decorators: []string{},
	}

	var isGenerator bool

	// Check for JSDoc
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists {
		if jsdoc.isPrivate {
			fn.Visibility = ir.VisibilityPrivate
		} else if jsdoc.isProtected {
			fn.Visibility = ir.VisibilityProtected
		}
		if jsdoc.returnType != "" {
			fn.Returns = &ir.TypeRef{Name: jsdoc.returnType}
		}
		if jsdoc.isAsync {
			isAsync = true
		}
		// We'll apply param types when processing parameters
	}

	// Process function parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "async":
			isAsync = true

		case "*":
			isGenerator = true

		case "identifier":
			fn.Name = p.getNodeText(child)
			// Check for underscore prefix convention
			if strings.HasPrefix(fn.Name, "_") {
				fn.Visibility = ir.VisibilityPrivate
			}

		case "formal_parameters":
			p.processParameters(child, fn)

		case "statement_block":
			// Function body
			startByte := child.StartByte()
			endByte := child.EndByte()
			if int(startByte) < len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}
		}
	}

	if isAsync {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}

	if isGenerator {
		fn.Name = "*" + fn.Name
	}

	p.addNode(file, parent, fn)
}

// processParameters processes function/method parameters
func (p *TreeSitterProcessor) processParameters(node *sitter.Node, fn *ir.DistilledFunction) {
	// Get JSDoc for this function
	var jsdocParamTypes map[string]string
	fnLine := int(node.Parent().StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[fnLine]; exists && jsdoc.paramTypes != nil {
		jsdocParamTypes = jsdoc.paramTypes
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			param := ir.Parameter{
				Name: p.getNodeText(child),
			}
			// Check if we have JSDoc type for this param
			if jsdocParamTypes != nil {
				if jsdocType, exists := jsdocParamTypes[param.Name]; exists {
					param.Type = ir.TypeRef{Name: jsdocType}
				}
			}
			fn.Parameters = append(fn.Parameters, param)

		case "rest_pattern":
			// ...args
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(uint(j)).Kind() == "identifier" {
					param := ir.Parameter{
						Name: "..." + p.getNodeText(child.Child(uint(j))),
					}
					if jsdocParamTypes != nil {
						if jsdocType, exists := jsdocParamTypes[p.getNodeText(child.Child(uint(j)))]; exists {
							param.Type = ir.TypeRef{Name: jsdocType}
						}
					}
					fn.Parameters = append(fn.Parameters, param)
				}
			}

		case "assignment_pattern":
			// param = defaultValue; the parameter is the left side as written
			// (a name or a destructuring pattern), the default the right side,
			// which may itself be a name.
			paramName := p.getNodeText(child.ChildByFieldName("left"))
			defaultValue := p.getNodeText(child.ChildByFieldName("right"))
			param := ir.Parameter{
				Name:         paramName,
				DefaultValue: defaultValue,
			}
			if jsdocParamTypes != nil {
				if jsdocType, exists := jsdocParamTypes[paramName]; exists {
					param.Type = ir.TypeRef{Name: jsdocType}
				}
			}
			fn.Parameters = append(fn.Parameters, param)

		case "object_pattern", "array_pattern":
			// Destructuring parameters
			param := ir.Parameter{
				Name: p.getNodeText(child),
			}
			fn.Parameters = append(fn.Parameters, param)
		}
	}
}

// processVariableDeclaration processes var declarations
func (p *TreeSitterProcessor) processVariableDeclaration(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Process each variable declarator
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "variable_declarator" {
			p.processVariableDeclarator(child, file, parent, "var")
		}
	}
}

// processLexicalDeclaration processes let/const declarations
func (p *TreeSitterProcessor) processLexicalDeclaration(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	var kind string

	// Determine if let or const
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "let" || child.Kind() == "const" {
			kind = child.Kind()
			break
		}
	}

	// Process each variable declarator
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "variable_declarator" {
			p.processVariableDeclarator(child, file, parent, kind)
		}
	}
}

// processVariableDeclarator processes individual variable declarations
func (p *TreeSitterProcessor) processVariableDeclarator(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, kind string) {
	field := &ir.DistilledField{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Variables are public by default
		Modifiers:  []ir.Modifier{},
	}

	// The keyword as written: const is final, let and var are their own.
	switch kind {
	case "const":
		field.Modifiers = append(field.Modifiers, ir.ModifierFinal)
	case "let":
		field.Modifiers = append(field.Modifiers, ir.ModifierLet)
	case "var":
		field.Modifiers = append(field.Modifiers, ir.ModifierVar)
	}

	// Check for JSDoc
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists {
		if jsdoc.isPrivate {
			field.Visibility = ir.VisibilityPrivate
		} else if jsdoc.isProtected {
			field.Visibility = ir.VisibilityProtected
		}
		if jsdoc.typedefType != "" {
			field.Type = &ir.TypeRef{Name: jsdoc.typedefType}
		}
	}

	// The name: an identifier or a destructuring pattern. The value is read
	// from its own field, so a value that is a plain name
	// (`const encode = encodeURIComponent`) does not replace it.
	if name := node.ChildByFieldName("name"); name != nil {
		field.Name = p.getNodeText(name)
		// Check for underscore prefix convention
		if name.Kind() == "identifier" && strings.HasPrefix(field.Name, "_") {
			field.Visibility = ir.VisibilityPrivate
		}
	}

	if child := node.ChildByFieldName("value"); child != nil {
		// Check if it's a function expression, generator function, or arrow function
		if child.Kind() == "function_expression" || child.Kind() == "arrow_function" || child.Kind() == "generator_function" {
			// Store as a function instead
			fn := &ir.DistilledFunction{
				BaseNode:   p.nodeLocation(child),
				Name:       field.Name,
				Visibility: field.Visibility,
				Modifiers:  field.Modifiers,
				Parameters: []ir.Parameter{},
			}

			// Process function expression
			if child.Kind() == "arrow_function" {
				p.processArrowFunction(child, fn)
			} else {
				// Printed `const f = function (a)`, not as an arrow
				fn.Extensions = &ir.NodeExtensions{JavaScript: &ir.JavaScriptExtensions{IsFunctionExpression: true}}
				p.processFunctionExpression(child, fn)
			}

			p.addNode(file, parent, fn)
			return
		} else if esutil.TakesFunction(child) {
			// forwardRef((props, ref) => ...): the call with the
			// signatures of its function arguments; the whole
			// initializer is the implementation.
			field.DefaultValue = esutil.Signature(child, p.source)
			field.Implementation = p.getNodeText(child)
		} else if child.Kind() == "object" {
			// Simple object literal analysis
			field.DefaultValue = p.processObjectLiteral(child)
		} else {
			field.DefaultValue = p.getNodeText(child)
		}
	}

	p.addNode(file, parent, field)
}

// processArrowFunction processes arrow function expressions
func (p *TreeSitterProcessor) processArrowFunction(node *sitter.Node, fn *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "async":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)

		case "formal_parameters":
			p.processParameters(child, fn)
		}
	}

	// Single parameter without parentheses: x => ...
	if param := node.ChildByFieldName("parameter"); param != nil {
		fn.Parameters = append(fn.Parameters, ir.Parameter{
			Name: p.getNodeText(param),
		})
	}

	// The body is a statement_block (stored with its braces) or any
	// expression (stored bare; the formatter prints it as "=> expr").
	if body := node.ChildByFieldName("body"); body != nil {
		fn.Implementation = p.getNodeText(body)
	}
}

// processFunctionExpression processes function expressions
func (p *TreeSitterProcessor) processFunctionExpression(node *sitter.Node, fn *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "async":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)

		case "*":
			// Generator
			fn.Name = "*" + fn.Name

		case "identifier":
			// Named function expression
			// Keep the original variable name

		case "formal_parameters":
			p.processParameters(child, fn)

		case "statement_block":
			// Function body
			startByte := child.StartByte()
			endByte := child.EndByte()
			if int(startByte) < len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}
		}
	}
}

// processExpressionStatement processes expression statements, checking for module.exports
func (p *TreeSitterProcessor) processExpressionStatement(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Look for assignment expressions
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "assignment_expression" {
			// Check if it's module.exports = ...
			left := child.ChildByFieldName("left")
			right := child.ChildByFieldName("right")

			if left != nil && right != nil {
				leftText := p.getNodeText(left)
				if leftText == "module.exports" {
					// This is a module.exports assignment
					// For now, just add as a comment to indicate it's exported
					comment := &ir.DistilledComment{
						BaseNode: p.nodeLocation(node),
						Text:     fmt.Sprintf("module.exports = %s", p.getNodeText(right)),
						Format:   ir.CommentExport,
					}
					p.addNode(file, parent, comment)
				}
			}
		}
	}
}

// processComment emits a comment node where the comment occurs. A JSDoc block
// becomes a doc comment; its tags were already read by collectJSDocComments.
// A file-level doc that carries a file tag (@file, @module, ...) documents
// the file, not the declaration after it.
func (p *TreeSitterProcessor) processComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	text, format := commentutil.StripJSStyle(p.getNodeText(node))
	comment := &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     text,
		Format:   format,
	}
	if treeParent := node.Parent(); format == ir.CommentDoc && treeParent != nil && treeParent.Kind() == "program" {
		comment.Inner = commentutil.DocumentsFile(text)
	}

	p.addNode(file, parent, comment)
}

// Helper methods

func (p *TreeSitterProcessor) nodeLocation(node *sitter.Node) ir.BaseNode {
	startPoint := node.StartPosition()
	endPoint := node.EndPosition()

	return ir.BaseNode{
		Location: ir.Location{
			StartLine:   int(startPoint.Row) + 1, // tree-sitter uses 0-based lines
			StartColumn: int(startPoint.Column),
			EndLine:     int(endPoint.Row) + 1,
			EndColumn:   int(endPoint.Column),
			StartByte:   int(node.StartByte()),
			EndByte:     int(node.EndByte()),
		},
	}
}

func (p *TreeSitterProcessor) getNodeText(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if int(start) < len(p.source) && int(end) <= len(p.source) {
		return string(p.source[start:end])
	}
	return ""
}

func (p *TreeSitterProcessor) addNode(file *ir.DistilledFile, parent ir.DistilledNode, node ir.DistilledNode) {
	if parent != nil {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, node)
		case *ir.DistilledFunction:
			// Functions don't typically have children in JavaScript
		}
	} else {
		file.Children = append(file.Children, node)
	}
}

// processObjectLiteral creates a simplified representation of an object literal
func (p *TreeSitterProcessor) processObjectLiteral(node *sitter.Node) string {
	var members []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		childType := child.Kind()

		// Skip punctuation nodes
		if childType == "{" || childType == "}" || childType == "," {
			continue
		}

		if childType == "spread_element" {
			// Handle spread syntax like ...otherObject
			// The spread element has the entire "...identifier" as its text
			spreadText := p.getNodeText(child)
			if spreadText != "" {
				members = append(members, spreadText)
			}
		} else if childType == "pair" {
			// Get key
			key := ""

			keyNode := child.ChildByFieldName("key")
			valueNode := child.ChildByFieldName("value")

			if keyNode != nil {
				key = p.getNodeText(keyNode)
			}

			if valueNode != nil {
				valueType := valueNode.Kind()
				if valueType == "function_expression" || valueType == "arrow_function" {
					// It's a method - extract parameters, async prefix and return type
					params := p.extractFunctionParams(valueNode)
					returnType := p.extractReturnType(valueNode)
					asyncPrefix := p.extractAsyncPrefix(valueNode)

					methodSig := fmt.Sprintf("%s%s(%s)", asyncPrefix, key, params)
					if returnType != "" {
						methodSig += " -> " + returnType
					}
					members = append(members, methodSig)
				} else {
					// It's a property
					members = append(members, key)
				}
			}
		} else if child.Kind() == "method_definition" {
			// Handle method definitions (including getters/setters)
			methodStr := p.processMethodDefinition(child)
			if methodStr != "" {
				members = append(members, methodStr)
			}
		}
	}

	if len(members) > 0 {
		return fmt.Sprintf("{ %s }", strings.Join(members, ", "))
	}
	return "{}"
}

// extractFunctionParams extracts parameter names from a function node
func (p *TreeSitterProcessor) extractFunctionParams(node *sitter.Node) string {
	var params []string

	// Find formal_parameters node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "formal_parameters" {
			// Extract each parameter
			for j := 0; j < int(child.ChildCount()); j++ {
				paramNode := child.Child(uint(j))
				switch paramNode.Kind() {
				case "identifier":
					params = append(params, p.getNodeText(paramNode))
				case "assignment_pattern":
					// Parameter with default value
					for k := 0; k < int(paramNode.ChildCount()); k++ {
						if paramNode.Child(uint(k)).Kind() == "identifier" {
							paramName := p.getNodeText(paramNode.Child(uint(k)))
							// Find default value
							var defaultVal string
							for l := k + 1; l < int(paramNode.ChildCount()); l++ {
								if paramNode.Child(uint(l)).Kind() != "=" {
									defaultVal = p.getNodeText(paramNode.Child(uint(l)))
									break
								}
							}
							if defaultVal != "" {
								params = append(params, fmt.Sprintf("%s = %s", paramName, defaultVal))
							} else {
								params = append(params, paramName)
							}
							break
						}
					}
				case "rest_pattern":
					// ...args
					for k := 0; k < int(paramNode.ChildCount()); k++ {
						if paramNode.Child(uint(k)).Kind() == "identifier" {
							params = append(params, "..."+p.getNodeText(paramNode.Child(uint(k))))
						}
					}
				case "object_pattern", "array_pattern":
					// Destructuring - just show the pattern
					params = append(params, p.getNodeText(paramNode))
				}
			}
			break
		} else if child.Kind() == "identifier" && node.Kind() == "arrow_function" {
			// Single parameter arrow function without parentheses
			params = append(params, p.getNodeText(child))
			break
		}
	}

	return strings.Join(params, ", ")
}

// processMethodDefinition processes method definitions including getters/setters
func (p *TreeSitterProcessor) processMethodDefinition(node *sitter.Node) string {
	var name, params string
	var isGetter, isSetter, isAsync, isGenerator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "get":
			isGetter = true
		case "set":
			isSetter = true
		case "async":
			isAsync = true
		case "*":
			isGenerator = true
		case "property_identifier":
			name = p.getNodeText(child)
		case "formal_parameters":
			params = p.extractParametersFromNode(child)
		}
	}

	// Build method signature
	prefix := ""
	if isAsync {
		prefix = "async "
	}
	if isGenerator {
		prefix += "*"
	}

	if isGetter {
		return fmt.Sprintf("get %s()", name)
	} else if isSetter {
		if params == "" {
			params = "value" // Default parameter name for setters
		}
		return fmt.Sprintf("set %s(%s)", name, params)
	} else if name != "" {
		// Check for return type from JSDoc
		returnType := ""
		nodeLine := int(node.StartPosition().Row) + 1
		if jsdoc, exists := p.jsdocComments[nodeLine]; exists && jsdoc.returnType != "" {
			returnType = jsdoc.returnType
		} else if isAsync {
			returnType = "Promise"
		}

		methodSig := fmt.Sprintf("%s%s(%s)", prefix, name, params)
		if returnType != "" {
			methodSig += " -> " + returnType
		}
		return methodSig
	}

	return ""
}

// extractParametersFromNode extracts parameters from a formal_parameters node
func (p *TreeSitterProcessor) extractParametersFromNode(node *sitter.Node) string {
	var params []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			params = append(params, p.getNodeText(child))
		case "assignment_pattern":
			// Parameter with default value
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(uint(j)).Kind() == "identifier" {
					params = append(params, p.getNodeText(child.Child(uint(j))))
					break
				}
			}
		}
	}

	return strings.Join(params, ", ")
}

// extractReturnType extracts return type from JSDoc or async detection
func (p *TreeSitterProcessor) extractReturnType(node *sitter.Node) string {
	// Check if it's an async function
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(uint(i)).Kind() == "async" {
			// For now, just mark async functions as returning Promise
			return "Promise"
		}
	}

	// Check JSDoc comments for return type
	nodeLine := int(node.StartPosition().Row) + 1
	if jsdoc, exists := p.jsdocComments[nodeLine]; exists && jsdoc.returnType != "" {
		return jsdoc.returnType
	}

	return ""
}

// extractAsyncPrefix checks if function is async or generator
func (p *TreeSitterProcessor) extractAsyncPrefix(node *sitter.Node) string {
	var isAsync, isGenerator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "async":
			isAsync = true
		case "*":
			isGenerator = true
		}
	}

	prefix := ""
	if isAsync {
		prefix = "async "
	}
	if isGenerator {
		prefix += "*"
	}

	return prefix
}

// Close frees the parser. Trees parsed by it are closed by ProcessSource; a
// second Close does nothing.
func (p *TreeSitterProcessor) Close() {
	if p.parser != nil {
		p.parser.Close()
		p.parser = nil
	}
}
