package php

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
)

// TreeSitterProcessor processes PHP using tree-sitter
type TreeSitterProcessor struct {
	parser   *sitter.Parser
	source   []byte
	filename string

	// State for context-aware parsing
	currentNamespace     string
	useAliases           map[string]string
	currentClass         string
	currentClassDocblock string
	pendingAttributes    []string

	// PHPDoc information by line number
	docblocks map[int]*PHPDocInfo
}

// PHPDocInfo stores parsed PHPDoc information
type PHPDocInfo struct {
	propertyType string
	returnType   string
	paramTypes   map[string]string
	// variadicParams names the parameters written `...$x` in @param
	variadicParams map[string]bool
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() (*TreeSitterProcessor, error) {
	parser, err := tsparse.NewParser(sitter.NewLanguage(tree_sitter_php.LanguagePHP()))
	if err != nil {
		return nil, err
	}

	return &TreeSitterProcessor{
		parser:     parser,
		useAliases: make(map[string]string),
		docblocks:  make(map[int]*PHPDocInfo),
	}, nil
}

// ProcessSource processes PHP source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	p.source = source
	p.filename = filename

	// Reset state
	p.currentNamespace = ""
	p.useAliases = make(map[string]string)
	p.currentClass = ""
	p.docblocks = make(map[int]*PHPDocInfo)
	p.pendingAttributes = nil

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
		Language: "php",
		Version:  "8",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// First pass: collect all PHPDoc comments
	p.collectPHPDocComments(tree.RootNode())

	// Debug: print all top-level nodes
	// fmt.Printf("DEBUG: Root node has %d children\n", tree.RootNode().ChildCount())
	// for i := 0; i < int(tree.RootNode().ChildCount()); i++ {
	// 	child := tree.RootNode().Child(i)
	// 	fmt.Printf("DEBUG: Child %d: type=%s, named=%v\n", i, child.Kind(), child.IsNamed())
	// 	if child.Kind() == "comment" {
	// 		fmt.Printf("  Comment text: %s\n", p.getNodeText(child))
	// 	}
	// }

	// Second pass: process nodes with PHPDoc info available
	p.processNode(tree.RootNode(), file, nil)

	return file, nil
}

// collectPHPDocComments collects all PHPDoc comments in first pass
func (p *TreeSitterProcessor) collectPHPDocComments(node *sitter.Node) {
	if node == nil {
		return
	}

	nodeType := node.Kind()

	// Check if this is a PHPDoc comment
	if nodeType == "comment" {
		text := p.getNodeText(node)
		// fmt.Printf("DEBUG collectPHPDocComments: found comment: %s\n", text)
		if strings.HasPrefix(text, "/**") {
			// Parse PHPDoc
			info := p.parsePHPDoc(text)
			if info != nil {
				// Store by the line of the NEXT non-comment node
				nextLine := p.findNextCodeLine(node)
				if nextLine > 0 {
					p.docblocks[nextLine] = info
				}
			}
		}
	}

	// Process children
	for i := 0; i < int(node.ChildCount()); i++ {
		p.collectPHPDocComments(node.Child(uint(i)))
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
			// Skip other comments and PHP tags
			if nodeType != "comment" && nodeType != "php_tag" && nodeType != "text_interpolation" {
				return int(child.StartPosition().Row) + 1
			}
		}
	}

	return 0
}

// parsePHPDoc parses PHPDoc comment and extracts type information
func (p *TreeSitterProcessor) parsePHPDoc(text string) *PHPDocInfo {
	info := &PHPDocInfo{
		paramTypes:     make(map[string]string),
		variadicParams: make(map[string]bool),
	}

	// Clean up docblock
	text = strings.TrimPrefix(text, "/**")
	text = strings.TrimSuffix(text, "*/")

	lines := strings.Split(text, "\n")
	hasTypeInfo := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)

		// Extract @var type for properties
		if strings.HasPrefix(line, "@var ") {
			typeStr := strings.TrimSpace(strings.TrimPrefix(line, "@var"))
			// For @var, take everything as the type (no parameter name)
			if typeStr != "" {
				info.propertyType = p.normalizeArrayType(typeStr)
				hasTypeInfo = true
			}
		}

		// Extract @return type for functions
		if strings.HasPrefix(line, "@return ") {
			typeStr := strings.TrimSpace(strings.TrimPrefix(line, "@return"))
			// For @return, take everything as the type (description is usually on next line)
			if typeStr != "" {
				info.returnType = p.normalizeArrayType(typeStr)
				hasTypeInfo = true
			}
		}

		// Extract @param types for function parameters
		if strings.HasPrefix(line, "@param ") {
			paramStr := strings.TrimSpace(strings.TrimPrefix(line, "@param"))
			// For @param, we need to find where the parameter name starts
			// Complex types can have spaces, so we look for $paramName pattern
			dollarPos := strings.Index(paramStr, "$")
			if dollarPos > 0 {
				// Extract type (everything before the $). The `...` and `&`
				// of `@param int &...$x` belong to the parameter, not to its
				// type; the `&` is the code's to declare, the `...` is kept.
				typeName := strings.TrimSpace(paramStr[:dollarPos])
				variadic := strings.HasSuffix(typeName, "...")
				typeName = strings.TrimSpace(strings.TrimSuffix(typeName, "..."))
				typeName = strings.TrimSpace(strings.TrimSuffix(typeName, "&"))
				// Extract parameter name
				rest := paramStr[dollarPos+1:]
				spacePos := strings.IndexFunc(rest, unicode.IsSpace)
				paramName := rest
				if spacePos > 0 {
					paramName = rest[:spacePos]
				}
				if typeName != "" && paramName != "" {
					info.paramTypes[paramName] = p.normalizeArrayType(typeName)
					info.variadicParams[paramName] = variadic
					hasTypeInfo = true
				}
			}
		}
	}

	if hasTypeInfo {
		return info
	}
	return nil
}

// normalizeArrayType normalizes array type notation from PHPDoc
// Converts array<User> to User[], keeps User[] as is
// For associative arrays like array<string, User>, keeps the full notation
func (p *TreeSitterProcessor) normalizeArrayType(typeName string) string {
	// Handle generic array notation: array<Type> or list<Type>
	if (strings.HasPrefix(typeName, "array<") || strings.HasPrefix(typeName, "list<")) && strings.HasSuffix(typeName, ">") {
		start := strings.Index(typeName, "<")
		end := strings.LastIndex(typeName, ">")
		if start != -1 && end > start {
			innerType := typeName[start+1 : end]

			// Check if it's associative array (has comma)
			if strings.Contains(innerType, ",") {
				// For associative arrays, keep the full notation
				// This preserves array<string, User> and array<string, User[]>
				return typeName
			}

			// Simple array, convert to []
			return innerType + "[]"
		}
	}

	// Handle collection types like Collection<User>
	if strings.Contains(typeName, "<") && strings.HasSuffix(typeName, ">") {
		// For other generic types, keep the original notation
		// This includes Collection<User>, ArrayObject<User>, etc.
		return typeName
	}

	return typeName
}

// processNode recursively processes tree-sitter nodes
func (p *TreeSitterProcessor) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if node == nil {
		return
	}

	nodeType := node.Kind()
	// if nodeType == "comment" || strings.Contains(p.getNodeText(node), "Constants") {
	// 	fmt.Printf("DEBUG processNode: type=%s, text=%s\n", nodeType, p.getNodeText(node))
	// }

	// Process comments even if they're not named
	if nodeType == "comment" {
		text := p.getNodeText(node)
		if strings.HasPrefix(text, "/**") && node.Parent() != nil {
			// This is a docblock, check if next sibling is a class
			parent := node.Parent()
			var nodeIndex int = -1
			for i := 0; i < int(parent.ChildCount()); i++ {
				if parent.Child(uint(i)).Equals(*node) {
					nodeIndex = i
					break
				}
			}

			if nodeIndex >= 0 && nodeIndex+1 < int(parent.ChildCount()) {
				nextNode := parent.Child(uint(nodeIndex + 1))
				// Skip whitespace
				for j := nodeIndex + 1; j < int(parent.ChildCount()); j++ {
					nextNode = parent.Child(uint(j))
					if nextNode.IsNamed() || nextNode.Kind() == "comment" {
						break
					}
				}

				if nextNode.Kind() == "class_declaration" {
					// Store docblock for the class
					p.currentClassDocblock = text
				}
			}
		}
		// Don't create comment nodes for class docblocks that will be attached to classes
		if p.currentClassDocblock == text {
			return
		}
	}

	// Skip other non-named nodes
	if !node.IsNamed() && nodeType != "php_tag" {
		// Process children for anonymous nodes
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
		return
	}

	switch nodeType {
	case "program":
		// Process all children at top level
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(uint(i))
			// Check for PHP tag containing the actual content
			if child.Kind() == "text" || child.Kind() == "php_tag" {
				// PHP content is nested inside php_tag
				p.processNode(child, file, parent)
			} else {
				p.processNode(child, file, parent)
			}
		}

	case "php_tag", "text":
		// Process PHP content inside the tag
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(uint(i))
			// fmt.Printf("php_tag child %d: %s\n", i, child.Kind())
			// Check if this is a docblock followed by a class
			if child.Kind() == "comment" {
				text := p.getNodeText(child)
				if strings.HasPrefix(text, "/**") {
					// Look ahead to find the next non-comment node
					for j := i + 1; j < int(node.ChildCount()); j++ {
						nextChild := node.Child(uint(j))
						if nextChild.Kind() == "class_declaration" {
							// This is a class docblock, store it temporarily
							p.currentClassDocblock = text
							break
						} else if nextChild.IsNamed() && nextChild.Kind() != "comment" {
							// Found a different named node, stop looking
							break
						}
					}
				}
			}
			p.processNode(child, file, parent)
		}

	case "attribute_list":
		// This node precedes the actual declaration (class, method, etc.)
		// We process it and store the results to be attached to the next node.
		p.processAttributes(node, &p.pendingAttributes)

	case "namespace_definition":
		p.processNamespace(node, file, parent)

	case "namespace_use_declaration":
		// fmt.Printf("DEBUG: Found namespace_use_declaration\n")
		p.processUseDeclaration(node, file, parent)

	case "class_declaration":
		p.processClass(node, file, parent)

	case "interface_declaration":
		p.processInterface(node, file, parent)

	case "trait_declaration":
		p.processTrait(node, file, parent)

	case "enum_declaration":
		p.processEnum(node, file, parent)

	case "function_definition":
		p.processFunction(node, file, parent, false)

	case "method_declaration":
		p.processMethod(node, file, parent)

	case "property_declaration":
		p.processProperty(node, file, parent)

	case "const_declaration":
		p.processConstant(node, file, parent)

	case "comment":
		// fmt.Printf("DEBUG: Found comment: %s\n", p.getNodeText(node))
		p.processComment(node, file, parent)

	case "use_declaration":
		// Check if we're inside a class (trait usage)
		if class, ok := parent.(*ir.DistilledClass); ok {
			p.processTraitUse(node, file, class)
		}

	default:
		// Process children for other node types
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
	}
}

// processNamespace processes namespace declarations
func (p *TreeSitterProcessor) processNamespace(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Get namespace name
	var namespaceName string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "namespace_name" {
			namespaceName = p.getNodeText(child)
			break
		}
	}

	// Update current namespace
	p.currentNamespace = namespaceName

	// Add namespace as an import node so it's visible in the output
	if namespaceName != "" {
		imp := &ir.DistilledImport{
			BaseNode:   p.nodeLocation(node),
			ImportType: "namespace",
			Module:     namespaceName,
			Symbols:    []ir.ImportedSymbol{},
		}
		p.addNode(file, parent, imp)
	}

	// Process namespace body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "compound_statement" || child.Kind() == "declaration_list" {
			// Process declarations within namespace
			for j := 0; j < int(child.ChildCount()); j++ {
				p.processNode(child.Child(uint(j)), file, parent)
			}
		}
	}
}

// processUseDeclaration turns a namespace use statement into imports that keep
// what the source wrote. ImportType is `use`, `use function` or `use const`.
//
//   - Each clause of `use A\B, C\D as E;` is one import whose Module is the
//     name as written and whose single symbol carries that name and the alias.
//   - A group `use A\{B, C as D};` is one import whose Module is the prefix
//     with its trailing backslash (`A\`) and whose symbols are the members; a
//     member with its own kind (`use A\{function f}`) keeps the keyword in
//     its symbol name.
func (p *TreeSitterProcessor) processUseDeclaration(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// `use function A\{...}` puts the kind on the statement; `use function
	// A\b;` puts it on the clause.
	kind := p.useKind(node)

	if group := node.ChildByFieldName("body"); group != nil && group.Kind() == "namespace_use_group" {
		// The prefix is the text between the keyword(s) and the brace,
		// leading backslash included.
		start := node.Child(0).EndByte()
		if t := node.ChildByFieldName("type"); t != nil {
			start = t.EndByte()
		}
		prefix := strings.TrimSpace(string(p.source[start:group.StartByte()]))

		imp := &ir.DistilledImport{
			BaseNode:   p.nodeLocation(node),
			ImportType: useImportType(kind),
			Module:     prefix,
		}
		for i := uint(0); i < group.NamedChildCount(); i++ {
			clause := group.NamedChild(i)
			if clause.Kind() != "namespace_use_clause" {
				continue
			}
			name, alias := p.useClauseName(clause)
			if name == "" {
				continue
			}
			p.recordUseAlias(strings.TrimSuffix(prefix, "\\")+"\\"+name, alias)
			if memberKind := p.useKind(clause); memberKind != "" {
				name = memberKind + " " + name
			}
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: name, Alias: alias})
		}
		if len(imp.Symbols) > 0 {
			p.addNode(file, parent, imp)
		}
		return
	}

	for i := uint(0); i < node.NamedChildCount(); i++ {
		clause := node.NamedChild(i)
		if clause.Kind() != "namespace_use_clause" {
			continue
		}
		name, alias := p.useClauseName(clause)
		if name == "" {
			continue
		}
		p.recordUseAlias(name, alias)
		clauseKind := kind
		if k := p.useKind(clause); k != "" {
			clauseKind = k
		}
		p.addNode(file, parent, &ir.DistilledImport{
			BaseNode:   p.nodeLocation(clause),
			ImportType: useImportType(clauseKind),
			Module:     name,
			Symbols:    []ir.ImportedSymbol{{Name: name, Alias: alias}},
		})
	}
}

// useKind returns `function` or `const` when node (a use statement or one
// of its clauses) has that kind keyword, else "".
func (p *TreeSitterProcessor) useKind(node *sitter.Node) string {
	if t := node.ChildByFieldName("type"); t != nil {
		return strings.ToLower(p.getNodeText(t))
	}
	return ""
}

// useImportType is the ImportType of a use statement of the given kind.
func useImportType(kind string) string {
	if kind == "" {
		return "use"
	}
	return "use " + kind
}

// useClauseName returns the imported name of a namespace_use_clause as
// written and its alias. The alias is a `name` too, so it is told apart by
// its field.
func (p *TreeSitterProcessor) useClauseName(clause *sitter.Node) (name, alias string) {
	if a := clause.ChildByFieldName("alias"); a != nil {
		alias = p.getNodeText(a)
	}
	for i := uint(0); i < clause.ChildCount(); i++ {
		switch clause.Child(i).Kind() {
		case "qualified_name", "namespace_name", "name":
			if clause.FieldNameForChild(uint32(i)) != "alias" {
				return p.getNodeText(clause.Child(i)), alias
			}
		}
	}
	return "", alias
}

// recordUseAlias remembers which full name the short name (the alias, or the
// last segment) stands for.
func (p *TreeSitterProcessor) recordUseAlias(fullName, alias string) {
	if alias == "" {
		parts := strings.Split(fullName, "\\")
		alias = parts[len(parts)-1]
	}
	p.useAliases[alias] = fullName
}

// processClass processes class declarations
func (p *TreeSitterProcessor) processClass(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Modifiers:  []ir.Modifier{},
		Extends:    []ir.TypeRef{},
		Implements: []ir.TypeRef{},
		Children:   []ir.DistilledNode{},
		Decorators: []string{},
	}

	// Try multiple ways to find the docblock
	docblockFound := false
	// classDoc is the raw docblock the program level held back for this class.
	classDoc := ""

	// Method 1: Check for stored docblock comment from program level
	if p.currentClassDocblock != "" {
		classDoc = p.currentClassDocblock
		// Parse PSR-19 tags from docblock
		p.parseClassDocblock(p.currentClassDocblock, class)
		p.currentClassDocblock = "" // Clear after use
		docblockFound = true
	}

	// Method 2: Check parent's children for preceding comment
	if !docblockFound && node.Parent() != nil {
		parent := node.Parent()
		for i := 0; i < int(parent.ChildCount()); i++ {
			child := parent.Child(uint(i))
			if child.Equals(*node) && i > 0 {
				// Check previous sibling
				prev := parent.Child(uint(i - 1))
				if prev.Kind() == "comment" {
					text := p.getNodeText(prev)
					if strings.HasPrefix(text, "/**") {
						p.parseClassDocblock(text, class)
						docblockFound = true
						break
					}
				}
			}
		}
	}

	// Method 3: Original getPrecedingDocComment
	if !docblockFound {
		if docComment := p.getPrecedingDocComment(node); docComment != "" {
			p.parseClassDocblock(docComment, class)
		}
	}

	// Track current class
	prevClass := p.currentClass

	// First pass: get class name for better debugging
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "name" {
			class.Name = p.getNodeText(child)
			p.currentClass = class.Name
			break
		}
	}

	// Process class parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		// Debug
		// fmt.Printf("Class %s child %d: %s = %s\n", class.Name, i, child.Kind(), p.getNodeText(child))
		switch child.Kind() {
		case "attribute_list":
			// Process attributes directly as children of the class
			var classAttributes []string
			p.processAttributes(child, &classAttributes)
			class.Decorators = append(class.Decorators, classAttributes...)

		case "visibility_modifier":
			// Handle visibility
			visibility := p.getNodeText(child)
			if visibility == "private" {
				class.Visibility = ir.VisibilityPrivate
			} else if visibility == "protected" {
				class.Visibility = ir.VisibilityProtected
			} else {
				class.Visibility = ir.VisibilityPublic
			}

		case "abstract_modifier":
			class.Modifiers = append(class.Modifiers, ir.ModifierAbstract)

		case "final_modifier":
			class.Modifiers = append(class.Modifiers, ir.ModifierFinal)

		case "readonly_modifier":
			class.Modifiers = append(class.Modifiers, ir.ModifierReadonly)

		case "name":
			// Already set in first pass

		case "base_clause":
			// Extends clause
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(uint(j)).Kind() == "qualified_name" || child.Child(uint(j)).Kind() == "name" {
					// Don't resolve the full name, just use the short name or alias
					// The imports are already tracked separately
					class.Extends = append(class.Extends, ir.TypeRef{
						Name: p.getNodeText(child.Child(uint(j))),
					})
				}
			}

		case "class_interface_clause":
			p.processImplements(child, class)

		case "declaration_list":
			// Class body
			p.processClassBody(child, file, class)
		}
	}

	// If no explicit visibility, default to public
	if class.Visibility == "" {
		class.Visibility = ir.VisibilityPublic
	}

	// The docblock was held back from the comment path so that its tags could
	// feed the class. A class without API tags prints it as a doc comment ahead
	// of the declaration, like every other language.
	if classDoc != "" && class.APIDocblock == "" {
		p.addNode(file, parent, docblockComment(classDoc, p.nodeLocation(node)))
	}
	p.addNode(file, parent, class)

	// Restore previous class context
	p.currentClass = prevClass
}

// processImplements adds the interfaces of an `implements` clause, as
// written (short name or alias; the imports are tracked separately).
func (p *TreeSitterProcessor) processImplements(clause *sitter.Node, class *ir.DistilledClass) {
	for i := uint(0); i < clause.ChildCount(); i++ {
		child := clause.Child(i)
		if child.Kind() == "qualified_name" || child.Kind() == "name" {
			class.Implements = append(class.Implements, ir.TypeRef{Name: p.getNodeText(child)})
		}
	}
}

// docblockComment turns the raw text of a /** */ comment into a doc comment
// node with the comment markers and leading asterisks removed.
func docblockComment(raw string, loc ir.BaseNode) *ir.DistilledComment {
	text := strings.TrimSuffix(strings.TrimPrefix(raw, "/**"), "*/")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
	}
	return &ir.DistilledComment{
		BaseNode: loc,
		Text:     strings.TrimSpace(strings.Join(lines, "\n")),
		Format:   ir.CommentDoc,
	}
}

// processClassBody processes the body of a class
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, file *ir.DistilledFile, class *ir.DistilledClass) {
	// Debug: dump AST structure for class body
	// fmt.Fprintf(os.Stderr, "DEBUG: AST for class body (%s):\n", node.Kind())
	// for i := 0; i < int(node.ChildCount()); i++ {
	// 	child := node.Child(i)
	// 	fmt.Fprintf(os.Stderr, "  Child %d: type=%s, named=%v, text=%q\n", i, child.Kind(), child.IsNamed(), p.getNodeText(child))
	// }

	for i := 0; i < int(node.ChildCount()); i++ {
		p.processNode(node.Child(uint(i)), file, class)
	}
}

// processMethod processes method declarations
func (p *TreeSitterProcessor) processMethod(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
		Decorators: []string{},
	}

	var hasVisibility bool

	// Process method parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		// Debug: print all child types
		//fmt.Printf("Class child %d: %s = %s\n", i, child.Kind(), p.getNodeText(child))
		switch child.Kind() {
		case "attribute_list":
			// Process attributes directly as children of the method
			var methodAttributes []string
			p.processAttributes(child, &methodAttributes)
			fn.Decorators = append(fn.Decorators, methodAttributes...)

		case "visibility_modifier":
			visibility := p.getNodeText(child)
			if visibility == "private" {
				fn.Visibility = ir.VisibilityPrivate
			} else if visibility == "protected" {
				fn.Visibility = ir.VisibilityProtected
			} else {
				fn.Visibility = ir.VisibilityPublic
			}
			hasVisibility = true

		case "static_modifier":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierStatic)

		case "abstract_modifier":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierAbstract)

		case "final_modifier":
			fn.Modifiers = append(fn.Modifiers, ir.ModifierFinal)

		case "name":
			fn.Name = p.getNodeText(child)

		case "formal_parameters":
			p.processParameters(child, fn)
			// For constructor, handle property promotion
			if fn.Name == "__construct" && parent != nil {
				if class, ok := parent.(*ir.DistilledClass); ok {
					p.processPromotedProperties(child, class)
				}
			}

		case "reference_modifier":
			setReturnsReference(fn)

		case "compound_statement":
			// Method body
			startByte := child.StartByte()
			endByte := child.EndByte()
			if int(startByte) < len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}
		}
	}

	// Default visibility for methods is public
	if !hasVisibility {
		fn.Visibility = ir.VisibilityPublic
	}
	p.readReturnType(node, fn)

	// Check for PHPDoc type information
	nodeLine := int(node.StartPosition().Row) + 1
	if docInfo, exists := p.docblocks[nodeLine]; exists {
		// Apply return type
		if docInfo.returnType != "" {
			// Always prefer docblock type if it's more specific
			if fn.Returns == nil || fn.Returns.Name == "" ||
				fn.Returns.Name == "array" ||
				fn.Returns.Name == "mixed" ||
				fn.Returns.Name == "object" ||
				fn.Returns.Name == "callable" ||
				fn.Returns.Name == "string" ||
				fn.Returns.Name == "int" ||
				fn.Returns.Name == "bool" {
				fn.Returns = &ir.TypeRef{Name: docInfo.returnType}
			}
		}

		// Apply param types
		for i := range fn.Parameters {
			// A parameter the docblock writes `...$x` takes extra arguments
			// (func_get_args) even when the code declares it plain.
			if docInfo.variadicParams[fn.Parameters[i].Name] {
				fn.Parameters[i].IsVariadic = true
			}
			if docType, exists := docInfo.paramTypes[fn.Parameters[i].Name]; exists {
				// Always prefer docblock type if it's more specific
				// This includes class-string<T>, callable():object, int-mask-of<>, etc.
				if fn.Parameters[i].Type.Name == "" ||
					fn.Parameters[i].Type.Name == "array" ||
					fn.Parameters[i].Type.Name == "mixed" ||
					fn.Parameters[i].Type.Name == "object" ||
					fn.Parameters[i].Type.Name == "callable" ||
					fn.Parameters[i].Type.Name == "string" ||
					fn.Parameters[i].Type.Name == "int" {
					fn.Parameters[i].Type = ir.TypeRef{Name: docType}
				}
			}
		}
	}

	p.addNode(file, parent, fn)
}

// processProperty processes property declarations
func (p *TreeSitterProcessor) processProperty(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	var visibility ir.Visibility = ir.VisibilityPublic
	var modifiers []ir.Modifier
	var propertyType *ir.TypeRef
	var attributes []string

	// The type of any kind, `(A&B)|null` included
	if t := node.ChildByFieldName("type"); t != nil {
		propertyType = &ir.TypeRef{Name: p.getNodeText(t)}
	}

	// Process property modifiers
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "visibility_modifier":
			vis := p.getNodeText(child)
			if vis == "private" {
				visibility = ir.VisibilityPrivate
			} else if vis == "protected" {
				visibility = ir.VisibilityProtected
			}

		case "static_modifier":
			modifiers = append(modifiers, ir.ModifierStatic)

		case "readonly_modifier":
			modifiers = append(modifiers, ir.ModifierReadonly)

		case "attribute_list":
			p.processAttributes(child, &attributes)

		case "property_element":
			// Individual property with possible initializer
			p.processPropertyElement(child, file, parent, visibility, modifiers, propertyType, attributes)
		}
	}
}

// processPropertyElement processes individual property elements
func (p *TreeSitterProcessor) processPropertyElement(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode,
	visibility ir.Visibility, modifiers []ir.Modifier, propertyType *ir.TypeRef, attributes []string) {

	field := &ir.DistilledField{
		BaseNode:   p.nodeLocation(node),
		Visibility: visibility,
		Modifiers:  modifiers,
		Type:       propertyType,
		Decorators: attributes,
	}

	// Get property name and default value
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "variable_name":
			field.Name = strings.TrimPrefix(p.getNodeText(child), "$")

		case "property_initializer":
			// Get default value
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(uint(j)).Kind() != "=" {
					field.DefaultValue = p.getNodeText(child.Child(uint(j)))
					break
				}
			}
		}
	}

	// Check for PHPDoc type information
	nodeLine := int(node.StartPosition().Row) + 1
	if docInfo, exists := p.docblocks[nodeLine]; exists && docInfo.propertyType != "" {
		if field.Type == nil || field.Type.Name == "" {
			field.Type = &ir.TypeRef{Name: docInfo.propertyType}
		} else if field.Type.Name == "array" {
			// PHPDoc has more specific array type
			field.Type = &ir.TypeRef{Name: docInfo.propertyType}
		}
	}

	p.addNode(file, parent, field)
}

// processFunction processes function declarations
func (p *TreeSitterProcessor) processFunction(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, isAsync bool) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic, // Functions are always public
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
		Decorators: p.pendingAttributes, // Use pending attributes
	}
	p.pendingAttributes = nil // Clear after use

	if isAsync {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}

	// Process function parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "attribute_list":
			p.processAttributes(child, &fn.Decorators)

		case "name":
			fn.Name = p.getNodeText(child)

		case "formal_parameters":
			p.processParameters(child, fn)

		case "reference_modifier":
			setReturnsReference(fn)

		case "compound_statement":
			// Function body
			startByte := child.StartByte()
			endByte := child.EndByte()
			if int(startByte) < len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}
		}
	}
	p.readReturnType(node, fn)

	// Check for PHPDoc type information
	nodeLine := int(node.StartPosition().Row) + 1
	if docInfo, exists := p.docblocks[nodeLine]; exists {
		// Apply return type
		if docInfo.returnType != "" {
			// Always prefer docblock type if it's more specific
			if fn.Returns == nil || fn.Returns.Name == "" ||
				fn.Returns.Name == "array" ||
				fn.Returns.Name == "mixed" ||
				fn.Returns.Name == "object" ||
				fn.Returns.Name == "callable" ||
				fn.Returns.Name == "string" ||
				fn.Returns.Name == "int" ||
				fn.Returns.Name == "bool" {
				fn.Returns = &ir.TypeRef{Name: docInfo.returnType}
			}
		}

		// Apply param types
		for i := range fn.Parameters {
			// A parameter the docblock writes `...$x` takes extra arguments
			// (func_get_args) even when the code declares it plain.
			if docInfo.variadicParams[fn.Parameters[i].Name] {
				fn.Parameters[i].IsVariadic = true
			}
			if docType, exists := docInfo.paramTypes[fn.Parameters[i].Name]; exists {
				// Always prefer docblock type if it's more specific
				// This includes class-string<T>, callable():object, int-mask-of<>, etc.
				if fn.Parameters[i].Type.Name == "" ||
					fn.Parameters[i].Type.Name == "array" ||
					fn.Parameters[i].Type.Name == "mixed" ||
					fn.Parameters[i].Type.Name == "object" ||
					fn.Parameters[i].Type.Name == "callable" ||
					fn.Parameters[i].Type.Name == "string" ||
					fn.Parameters[i].Type.Name == "int" {
					fn.Parameters[i].Type = ir.TypeRef{Name: docType}
				}
			}
		}
	}

	p.addNode(file, parent, fn)
}

// readReturnType sets the declared return type of a function or method as
// written, whatever its kind (`never` is a bottom_type, `(A&B)|null` a
// disjunctive_normal_form_type).
func (p *TreeSitterProcessor) readReturnType(node *sitter.Node, fn *ir.DistilledFunction) {
	if rt := node.ChildByFieldName("return_type"); rt != nil {
		fn.Returns = &ir.TypeRef{Name: p.getNodeText(rt)}
	}
}

// setReturnsReference marks a function declared `function &name()`.
func setReturnsReference(fn *ir.DistilledFunction) {
	if fn.Extensions == nil {
		fn.Extensions = &ir.NodeExtensions{}
	}
	if fn.Extensions.PHP == nil {
		fn.Extensions.PHP = &ir.PHPExtensions{}
	}
	fn.Extensions.PHP.ReturnsReference = true
}

// processParameters processes function/method parameters
func (p *TreeSitterProcessor) processParameters(node *sitter.Node, fn *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "simple_parameter" || child.Kind() == "variadic_parameter" ||
			child.Kind() == "property_promotion_parameter" {
			param := p.processParameter(child)
			if param.Name != "" {
				fn.Parameters = append(fn.Parameters, param)
			}
		}
	}
}

// processParameter reads one parameter as written: its type of any kind
// (`(A&B)|null` included), `&` for a reference, `...` for a variadic and the
// default value.
func (p *TreeSitterProcessor) processParameter(node *sitter.Node) ir.Parameter {
	param := ir.Parameter{IsVariadic: node.Kind() == "variadic_parameter"}
	if t := node.ChildByFieldName("type"); t != nil {
		param.Type = ir.TypeRef{Name: p.getNodeText(t)}
	}
	if node.ChildByFieldName("reference_modifier") != nil {
		param.IsReference = true
	}
	param.Name, param.IsReference = p.parameterName(node, param.IsReference)
	if d := node.ChildByFieldName("default_value"); d != nil {
		param.DefaultValue = p.getNodeText(d)
	}
	return param
}

// parameterName returns the name of a parameter without its `$`. A promoted
// parameter written `&$x` has a by_ref node as its name, which also makes
// it a reference.
func (p *TreeSitterProcessor) parameterName(node *sitter.Node, isReference bool) (string, bool) {
	name := node.ChildByFieldName("name")
	if name == nil {
		return "", isReference
	}
	if name.Kind() == "by_ref" {
		isReference = true
		for i := uint(0); i < name.NamedChildCount(); i++ {
			if v := name.NamedChild(i); v.Kind() == "variable_name" {
				name = v
				break
			}
		}
	}
	return strings.TrimPrefix(p.getNodeText(name), "$"), isReference
}

// processPromotedProperties processes constructor promoted properties
func (p *TreeSitterProcessor) processPromotedProperties(parametersNode *sitter.Node, class *ir.DistilledClass) {
	for i := 0; i < int(parametersNode.ChildCount()); i++ {
		child := parametersNode.Child(uint(i))
		if child.Kind() == "property_promotion_parameter" {
			// This is a promoted property
			field := &ir.DistilledField{
				BaseNode:   p.nodeLocation(child),
				Visibility: ir.VisibilityPublic, // Default
				Modifiers:  []ir.Modifier{},
			}

			// Process promotion parameter parts
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(uint(j))
				switch paramChild.Kind() {
				case "visibility_modifier":
					visibility := p.getNodeText(paramChild)
					if visibility == "private" {
						field.Visibility = ir.VisibilityPrivate
					} else if visibility == "protected" {
						field.Visibility = ir.VisibilityProtected
					} else {
						field.Visibility = ir.VisibilityPublic
					}

				case "readonly_modifier":
					field.Modifiers = append(field.Modifiers, ir.ModifierReadonly)
				}
			}
			if t := child.ChildByFieldName("type"); t != nil {
				field.Type = &ir.TypeRef{Name: p.getNodeText(t)}
			}
			field.Name, _ = p.parameterName(child, false)

			// Add the field to the class, ahead of the comments that precede the
			// constructor: they document the constructor, not its fields.
			at := len(class.Children)
			for at > 0 {
				if _, ok := class.Children[at-1].(*ir.DistilledComment); !ok {
					break
				}
				at--
			}
			class.Children = slices.Insert(class.Children, at, ir.DistilledNode(field))
		}
	}
}

// processInterface processes interface declarations
func (p *TreeSitterProcessor) processInterface(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Use proper DistilledInterface type
	intf := &ir.DistilledInterface{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic,
		Extends:    []ir.TypeRef{},
		Children:   []ir.DistilledNode{},
	}

	// Process interface parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "name":
			intf.Name = p.getNodeText(child)

		case "interface_extends_clause", "base_clause":
			// Interfaces can extend multiple interfaces
			// Debug: print child types
			//fmt.Printf("DEBUG: interface_extends_clause has %d children\n", child.ChildCount())
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(uint(j))
				//fmt.Printf("DEBUG: child[%d] type=%s text=%s\n", j, grandchild.Kind(), p.getNodeText(grandchild))
				if grandchild.Kind() == "qualified_name" || grandchild.Kind() == "name" {
					// Don't resolve the full name, just use the short name or alias
					// The imports are already tracked separately
					intf.Extends = append(intf.Extends, ir.TypeRef{
						Name: p.getNodeText(grandchild),
					})
				}
			}

		case "declaration_list":
			// Interface body - process methods
			for j := 0; j < int(child.ChildCount()); j++ {
				methodNode := child.Child(uint(j))
				switch methodNode.Kind() {
				case "method_declaration":
					p.processMethod(methodNode, file, intf)
				case "comment":
					if p.commentPrints(child, j, "method_declaration") {
						p.processComment(methodNode, file, intf)
					}
				}
			}
		}
	}

	p.addNode(file, parent, intf)
}

// processTrait processes trait declarations
func (p *TreeSitterProcessor) processTrait(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Process trait as a class with PHP extension flag
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
		Decorators: []string{},
	}

	// Mark as trait in PHP extensions
	if class.Extensions == nil {
		class.Extensions = &ir.NodeExtensions{}
	}
	if class.Extensions.PHP == nil {
		class.Extensions.PHP = &ir.PHPExtensions{}
	}
	class.Extensions.PHP.IsTrait = true

	// Process trait parts
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "name":
			class.Name = p.getNodeText(child)

		case "declaration_list":
			// Trait body
			p.processClassBody(child, file, class)
		}
	}

	p.addNode(file, parent, class)
}

// processEnum processes enum declarations (PHP 8.1+)
func (p *TreeSitterProcessor) processEnum(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// For now, treat enums as classes with special marker
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierFinal}, // Enums are implicitly final
		Children:   []ir.DistilledNode{},
		Decorators: []string{},
	}

	// Mark this as an enum
	if class.Extensions == nil {
		class.Extensions = &ir.NodeExtensions{}
	}
	if class.Extensions.PHP == nil {
		class.Extensions.PHP = &ir.PHPExtensions{}
	}
	class.Extensions.PHP.IsEnum = true

	// Process enum parts: the header like a class's, then the members
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "attribute_list":
			p.processAttributes(child, &class.Decorators)

		case "name":
			class.Name = p.getNodeText(child)

		case "primitive_type":
			// Backing type (int or string)
			if i > 0 && node.Child(uint(i-1)).Kind() == ":" {
				// This is the backing type
				class.Extensions.PHP.EnumBackingType = p.getNodeText(child)
			}

		case "class_interface_clause":
			p.processImplements(child, class)

		case "enum_declaration_list":
			p.processEnumBody(child, file, class)
		}
	}

	p.addNode(file, parent, class)
}

// processEnumBody adds the members of an enum to it, as a class body does:
// cases, constants, methods and trait uses.
func (p *TreeSitterProcessor) processEnumBody(node *sitter.Node, file *ir.DistilledFile, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "enum_case":
			// Process enum case as a constant
			p.processEnumCase(child, file, class)
		case "const_declaration":
			p.processConstant(child, file, class)
		case "method_declaration":
			p.processMethod(child, file, class)
		case "use_declaration":
			p.processTraitUse(child, file, class)
		case "comment":
			// The formatter prints trait uses above the members, so a
			// docblock of one would land above an unrelated member.
			if p.commentPrints(node, i, "enum_case", "const_declaration", "method_declaration") {
				p.processComment(child, file, class)
			}
		}
	}
}

// commentPrints reports whether the comment at index i of body is kept as a
// node. A doc comment documents the next declaration, so it is kept only when
// that declaration is of a kind the formatter prints in place (kinds);
// otherwise it would end up above an unrelated member. Other comments keep
// their place.
func (p *TreeSitterProcessor) commentPrints(body *sitter.Node, i int, kinds ...string) bool {
	if !strings.HasPrefix(p.getNodeText(body.Child(uint(i))), "/**") {
		return true
	}
	for j := i + 1; j < int(body.ChildCount()); j++ {
		next := body.Child(uint(j))
		if next.IsNamed() && next.Kind() != "comment" {
			return slices.Contains(kinds, next.Kind())
		}
	}
	return false
}

// processEnumCase processes individual enum cases
func (p *TreeSitterProcessor) processEnumCase(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode:   p.nodeLocation(node),
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierFinal},
	}

	// Mark this as an enum case
	if field.Extensions == nil {
		field.Extensions = &ir.NodeExtensions{}
	}
	if field.Extensions.PHP == nil {
		field.Extensions.PHP = &ir.PHPExtensions{}
	}
	field.Extensions.PHP.IsEnumCase = true

	// Debug: print node structure
	// fmt.Printf("DEBUG processEnumCase: node has %d children\n", node.ChildCount())
	// for i := 0; i < int(node.ChildCount()); i++ {
	// 	child := node.Child(i)
	// 	fmt.Printf("  Child %d: type=%s, text=%s\n", i, child.Kind(), p.getNodeText(child))
	// }

	// Look for name and value directly as siblings
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "name":
			field.Name = p.getNodeText(child)
		case "=":
			// Next sibling should be the value
			if i+1 < int(node.ChildCount()) {
				nextChild := node.Child(uint(i + 1))
				field.DefaultValue = p.getNodeText(nextChild)
			}
		}
	}

	p.addNode(file, parent, field)
}

// processConstant processes constant declarations
func (p *TreeSitterProcessor) processConstant(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Process each constant in the declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "const_element" {
			field := &ir.DistilledField{
				BaseNode:   p.nodeLocation(child),
				Visibility: ir.VisibilityPublic,
				Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierFinal},
			}

			// Get constant name and value
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(uint(j))
				switch grandchild.Kind() {
				case "name":
					field.Name = p.getNodeText(grandchild)
				default:
					if grandchild.Kind() != "=" {
						field.DefaultValue = p.getNodeText(grandchild)
					}
				}
			}

			p.addNode(file, parent, field)
		}
	}
}

// processTraitUse processes trait usage in a class
func (p *TreeSitterProcessor) processTraitUse(node *sitter.Node, file *ir.DistilledFile, class *ir.DistilledClass) {
	// Add traits to the class's Mixins field
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "qualified_name" || child.Kind() == "name" {
			traitName := p.getNodeText(child)
			class.Mixins = append(class.Mixins, ir.TypeRef{
				Name: traitName,
			})
		}
	}
}

// processComment processes comments
func (p *TreeSitterProcessor) processComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	text := p.getNodeText(node)

	// Determine comment type
	format := ir.CommentLine
	if strings.HasPrefix(text, "/**") {
		format = "docblock"
		// Clean up docblock
		text = strings.TrimPrefix(text, "/**")
		text = strings.TrimSuffix(text, "*/")
		// Remove leading asterisks from each line
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		}
		text = strings.Join(lines, "\n")

		// Extract type information from PHPDoc if this is attached to a node
		if parent != nil {
			p.extractPHPDocTypes(text, parent)
		}
	} else if strings.HasPrefix(text, "/*") {
		format = ir.CommentBlock
		text = strings.TrimPrefix(text, "/*")
		text = strings.TrimSuffix(text, "*/")
	} else if strings.HasPrefix(text, "//") {
		text = strings.TrimPrefix(text, "//")
	} else if strings.HasPrefix(text, "#") {
		text = strings.TrimPrefix(text, "#")
	}

	comment := &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     strings.TrimSpace(text),
		Format:   format,
	}

	p.addNode(file, parent, comment)
}

// extractPHPDocTypes extracts type information from PHPDoc comments
func (p *TreeSitterProcessor) extractPHPDocTypes(docText string, node ir.DistilledNode) {
	// This method is not used anymore as PHPDoc parsing is handled
	// in the collectPHPDocComments and parsePHPDoc methods
}

// resolveTypeName resolves a type name using current namespace and use statements
func (p *TreeSitterProcessor) resolveTypeName(name string) string {
	// If already fully qualified, return as is
	if strings.HasPrefix(name, "\\") {
		return name
	}

	// Check if it's an aliased type
	parts := strings.SplitN(name, "\\", 2)
	if len(parts) > 0 {
		if fullName, ok := p.useAliases[parts[0]]; ok {
			if len(parts) > 1 {
				return fullName + "\\" + parts[1]
			}
			return fullName
		}
	}

	// If in a namespace, prepend it
	if p.currentNamespace != "" {
		return p.currentNamespace + "\\" + name
	}

	return name
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
		case *ir.DistilledInterface:
			p.Children = append(p.Children, node)
		case *ir.DistilledFunction:
			// Functions don't typically have children in PHP
		}
	} else {
		file.Children = append(file.Children, node)
	}
}

// processAttributes processes PHP 8 attributes
func (p *TreeSitterProcessor) processAttributes(node *sitter.Node, attributes *[]string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "attribute_group" {
			// Process each attribute in the group
			for j := 0; j < int(child.ChildCount()); j++ {
				attr := child.Child(uint(j))
				if attr.Kind() == "attribute" {
					attributeText := p.getNodeText(attr)
					*attributes = append(*attributes, attributeText)
				}
			}
		}
	}
}

// Close frees the parser. Trees parsed by it are closed by ProcessSource; a
// second Close does nothing.
func (p *TreeSitterProcessor) Close() {
	if p.parser != nil {
		p.parser.Close()
		p.parser = nil
	}
}

// getPrecedingDocComment gets the docblock comment immediately preceding a node
func (p *TreeSitterProcessor) getPrecedingDocComment(node *sitter.Node) string {
	if node == nil || node.Parent() == nil {
		return ""
	}

	// Find the index of current node in parent
	parent := node.Parent()
	var nodeIndex int = -1
	for i := 0; i < int(parent.ChildCount()); i++ {
		if parent.Child(uint(i)).Equals(*node) {
			nodeIndex = i
			break
		}
	}

	if nodeIndex <= 0 {
		return ""
	}

	// Check the previous sibling
	for i := nodeIndex - 1; i >= 0; i-- {
		prevNode := parent.Child(uint(i))
		if prevNode == nil {
			continue
		}

		nodeType := prevNode.Kind()

		// Skip whitespace nodes
		if !prevNode.IsNamed() && strings.TrimSpace(p.getNodeText(prevNode)) == "" {
			continue
		}

		// Found a comment
		if nodeType == "comment" {
			text := p.getNodeText(prevNode)
			if strings.HasPrefix(text, "/**") {
				return text
			}
		}

		// Found something else, stop looking
		break
	}

	return ""
}

// parseClassDocblock parses PSR-19 tags from class docblock
func (p *TreeSitterProcessor) parseClassDocblock(docComment string, class *ir.DistilledClass) {
	lines := strings.Split(docComment, "\n")

	// Track if we're in description
	inDescription := false
	description := []string{}
	hasAPITags := false

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip comment markers
		if line == "/**" || line == "*/" || line == "*" {
			continue
		}

		// Remove leading asterisk
		line = strings.TrimPrefix(line, "* ")
		line = strings.TrimPrefix(line, "*")

		// Check for tags
		if strings.HasPrefix(line, "@") {
			inDescription = false
			p.parseDocblockTag(line, class)

			// Check if this is an API-defining tag
			tag := strings.Split(line, " ")[0]
			if tag == "@property" || tag == "@property-read" || tag == "@property-write" ||
				tag == "@method" || tag == "@deprecated" || tag == "@api" || tag == "@internal" ||
				tag == "@throws" || tag == "@psalm-" || tag == "@phpstan-" {
				hasAPITags = true
			}
		} else if line != "" {
			// This is part of description
			if !inDescription && len(description) == 0 {
				inDescription = true
			}
			if inDescription {
				description = append(description, line)
			}
		}
	}

	// Set description if found
	if len(description) > 0 {
		class.Description = strings.Join(description, " ")
	}

	// Store the docblock text in the class if it contains API-defining tags
	if hasAPITags {
		// Store the docblock directly in the class's Description field
		// This way it will be available for formatters to display
		if class.Description == "" && len(description) == 0 {
			// No description was parsed, use the full docblock
			class.APIDocblock = docComment
		} else {
			// We have a description, store the docblock separately
			class.APIDocblock = docComment
		}
	}
}

// parseDocblockTag parses a single docblock tag and applies it to the class
func (p *TreeSitterProcessor) parseDocblockTag(line string, class *ir.DistilledClass) {
	// Split tag and content
	parts := strings.SplitN(line, " ", 2)
	if len(parts) == 0 {
		return
	}

	tag := parts[0]
	content := ""
	if len(parts) > 1 {
		content = strings.TrimSpace(parts[1])
	}

	switch tag {
	case "@property", "@property-read", "@property-write":
		p.parsePropertyTag(tag, content, class)
	case "@method":
		p.parseMethodTag(content, class)
	case "@deprecated":
		p.parseDeprecatedTag(content, class)
	case "@api":
		// Mark class as part of public API
		// No ModifierPublic, just ensure it's public visibility
		class.Visibility = ir.VisibilityPublic
	case "@internal":
		// Mark class as internal
		class.Visibility = ir.VisibilityInternal
	}
}

// parsePropertyTag parses @property* tags and creates virtual fields
func (p *TreeSitterProcessor) parsePropertyTag(tag, content string, class *ir.DistilledClass) {
	// Parse: [Type] $propertyName [Description]
	// Example: @property string $name User's full name

	// Debug - removed, use -vvv instead

	parts := strings.Fields(content)
	if len(parts) < 1 {
		return
	}

	var typeName, propertyName, description string
	var typeIndex, nameIndex int

	// Find the property name (starts with $)
	for i, part := range parts {
		if strings.HasPrefix(part, "$") {
			propertyName = strings.TrimPrefix(part, "$")
			nameIndex = i
			break
		}
	}

	if propertyName == "" {
		return
	}

	// Type is everything before the property name
	if nameIndex > 0 {
		typeName = strings.Join(parts[0:nameIndex], " ")
		typeIndex = nameIndex
	}

	// Description is everything after the property name
	if len(parts) > typeIndex+1 {
		description = strings.Join(parts[typeIndex+1:], " ")
	}

	// Create virtual field
	field := &ir.DistilledField{
		BaseNode:    ir.BaseNode{},
		Name:        propertyName,
		Visibility:  ir.VisibilityPublic, // Magic properties are always public
		Description: description,
	}

	// Set type if found
	if typeName != "" {
		field.Type = &ir.TypeRef{Name: p.normalizeArrayType(typeName)}
	}

	// Set PHP extensions
	phpExt := &ir.PHPExtensions{
		Origin:           ir.FieldOriginDocblock,
		SourceAnnotation: strings.TrimSpace(tag + " " + content),
	}

	// Set access mode based on tag
	switch tag {
	case "@property":
		phpExt.AccessMode = ir.FieldAccessReadWrite
	case "@property-read":
		phpExt.AccessMode = ir.FieldAccessReadOnly
		field.HasGetter = true
		field.HasSetter = false
	case "@property-write":
		phpExt.AccessMode = ir.FieldAccessWriteOnly
		field.HasGetter = false
		field.HasSetter = true
	}

	// Create NodeExtensions if needed
	if field.Extensions == nil {
		field.Extensions = &ir.NodeExtensions{}
	}
	field.Extensions.PHP = phpExt

	// Add to class children
	class.Children = append(class.Children, field)
}

// parseMethodTag parses @method tags and creates virtual methods
func (p *TreeSitterProcessor) parseMethodTag(content string, class *ir.DistilledClass) {
	// Parse: [static] [Type] methodName([params]) [Description]
	// Example: @method static User|null find(int $id) Find user by ID

	// Basic regex to extract method signature
	// This is simplified - a full parser would be more complex
	methodRegex := regexp.MustCompile(`^(\s*static\s+)?(?:([^\s(]+)\s+)?(\w+)\s*\(([^)]*)\)(.*)$`)
	matches := methodRegex.FindStringSubmatch(content)

	if len(matches) < 4 {
		return
	}

	isStatic := strings.TrimSpace(matches[1]) != ""
	returnType := strings.TrimSpace(matches[2])
	methodName := matches[3]
	paramsStr := matches[4]
	description := strings.TrimSpace(matches[5])

	// Create virtual method
	method := &ir.DistilledFunction{
		BaseNode:    ir.BaseNode{},
		Name:        methodName,
		Visibility:  ir.VisibilityPublic, // Magic methods are always public
		Parameters:  []ir.Parameter{},
		Description: description,
	}

	// Add static modifier if needed
	if isStatic {
		method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
	}

	// Set return type if found
	if returnType != "" {
		method.Returns = &ir.TypeRef{Name: p.normalizeArrayType(returnType)}
	}

	// Parse parameters (simplified)
	if paramsStr != "" {
		params := strings.Split(paramsStr, ",")
		for _, param := range params {
			param = strings.TrimSpace(param)
			if param == "" {
				continue
			}

			// Simple parameter parsing - just extract type and name
			paramParts := strings.Fields(param)
			if len(paramParts) >= 2 {
				// Last part is parameter name (may start with $)
				paramName := paramParts[len(paramParts)-1]
				paramName = strings.TrimPrefix(paramName, "$")

				// Everything else is the type
				paramType := strings.Join(paramParts[:len(paramParts)-1], " ")

				method.Parameters = append(method.Parameters, ir.Parameter{
					Name: paramName,
					Type: ir.TypeRef{Name: p.normalizeArrayType(paramType)},
				})
			} else if len(paramParts) == 1 {
				// Just parameter name
				paramName := strings.TrimPrefix(paramParts[0], "$")
				method.Parameters = append(method.Parameters, ir.Parameter{
					Name: paramName,
				})
			}
		}
	}

	// Set PHP extensions
	phpExt := &ir.PHPExtensions{
		Origin:           ir.FieldOriginDocblock,
		SourceAnnotation: strings.TrimSpace("@method " + content),
	}

	// Create NodeExtensions if needed
	if method.Extensions == nil {
		method.Extensions = &ir.NodeExtensions{}
	}
	method.Extensions.PHP = phpExt

	// Add to class children
	class.Children = append(class.Children, method)
}

// parseDeprecatedTag parses @deprecated tag
func (p *TreeSitterProcessor) parseDeprecatedTag(content string, class *ir.DistilledClass) {
	// Parse: [@deprecated] [version] [description]
	parts := strings.SplitN(content, " ", 2)

	deprecated := &ir.DeprecationInfo{}

	if len(parts) > 0 && parts[0] != "" {
		// Check if first part looks like a version
		if strings.ContainsAny(parts[0], "0123456789.") {
			deprecated.Version = parts[0]
			if len(parts) > 1 {
				deprecated.Description = parts[1]
			}
		} else {
			// No version, everything is description
			deprecated.Description = content
		}
	}

	class.Deprecated = deprecated
}
