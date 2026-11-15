package python

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

// NativeTreeSitterProcessor uses native Go tree-sitter bindings
type NativeTreeSitterProcessor struct {
	parser   *sitter.Parser
	source   []byte
	filename string
}

// NewNativeTreeSitterProcessor creates a new native tree-sitter processor
func NewNativeTreeSitterProcessor() (*NativeTreeSitterProcessor, error) {
	parser, err := tsparse.NewParser(sitter.NewLanguage(tree_sitter_python.Language()))
	if err != nil {
		return nil, err
	}

	return &NativeTreeSitterProcessor{
		parser: parser,
	}, nil
}

// ProcessSource processes Python source code using tree-sitter
func (p *NativeTreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("python:tree-sitter")
	defer dbg.Timing(debug.LevelDetailed, "tree-sitter parsing")()

	p.source = source
	p.filename = filename

	dbg.Logf(debug.LevelDetailed, "Parsing %d bytes with tree-sitter", len(source))

	// Parse with tree-sitter
	tree, err := tsparse.Parse(ctx, p.parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Close()

	// Dump raw AST at trace level
	debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
		d.Dump(debug.LevelTrace, "Raw tree-sitter AST", p.dumpTree(tree.RootNode(), 0))
	})

	// Create IR file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPosition().Row) + 1,
			},
		},
		Path:     filename,
		Language: "python",
		Version:  "3",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Process root node
	p.processNode(tree.RootNode(), file, nil)

	dbg.Logf(debug.LevelDetailed, "Processed %d top-level nodes", len(file.Children))

	// Analyze protocol satisfaction
	p.analyzeProtocolSatisfaction(file)

	// Dump final IR structure at trace level
	debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
		d.Dump(debug.LevelTrace, "Final Python IR structure", file)
	})

	return file, nil
}

// processNode recursively processes tree-sitter nodes
func (p *NativeTreeSitterProcessor) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if node == nil {
		return
	}

	nodeType := node.Kind()

	// Only process named nodes unless it's a special case
	if !node.IsNamed() && nodeType != "comment" {
		// Process children for anonymous nodes
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
		return
	}

	switch nodeType {
	case "module":
		// Process all children at module level; the module docstring
		// documents the module itself
		doc := docstringIndex(node)
		for i := 0; i < int(node.ChildCount()); i++ {
			if i == doc {
				p.addNode(file, parent, p.docstringComment(node.Child(uint(i)).NamedChild(0), true))
				continue
			}
			p.processNode(node.Child(uint(i)), file, parent)
		}

	case "import_statement":
		p.processImport(node, file, parent)

	case "import_from_statement":
		p.processFromImport(node, file, parent)

	case "class_definition":
		p.processClass(node, file, parent)

	case "function_definition":
		p.processFunction(node, file, parent, false)

	case "async_function_definition":
		p.processFunction(node, file, parent, true)

	case "decorated_definition":
		p.processDecoratedDefinition(node, file, parent)

	case "assignment":
		p.processAssignment(node, file, parent)

	case "expression_statement":
		// Check for bare strings and assignments
		if node.ChildCount() > 0 {
			child := node.Child(0)
			if child.Kind() == "string" {
				// Only module-level strings, not those inside function bodies
				if parent == nil { // parent == nil means it's at module level
					p.processBareString(child, file, parent)
				}
			} else if child.Kind() == "assignment" {
				p.processAssignment(child, file, parent)
			}
		}

	case "comment":
		p.processComment(node, file, parent)

	default:
		// Process children for other node types
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(uint(i)), file, parent)
		}
	}
}

// processImport processes import statements
func (p *NativeTreeSitterProcessor) processImport(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	imp := &ir.DistilledImport{
		BaseNode:   p.nodeLocation(node),
		ImportType: "import",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Find module name and alias
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "dotted_name", "identifier":
			if imp.Module == "" {
				imp.Module = p.getNodeText(child)
			}
		case "aliased_import":
			// Has alias
			if child.ChildCount() >= 3 {
				nameNode := child.Child(0)
				aliasNode := child.Child(2) // Skip "as" keyword
				if nameNode != nil && aliasNode != nil {
					imp.Module = p.getNodeText(nameNode)
					imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
						Name:  imp.Module,
						Alias: p.getNodeText(aliasNode),
					})
				}
			}
		}
	}

	p.addNode(file, parent, imp)
}

// processFromImport processes from...import statements
func (p *NativeTreeSitterProcessor) processFromImport(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Simple approach - just get the text and parse it ourselves
	nodeText := p.getNodeText(node)

	imp := &ir.DistilledImport{
		BaseNode:   p.nodeLocation(node),
		ImportType: "from",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Parse "from MODULE import SYMBOL[, SYMBOL2] [as ALIAS]"
	if strings.HasPrefix(nodeText, "from ") && strings.Contains(nodeText, " import ") {
		parts := strings.Split(nodeText, " import ")
		if len(parts) == 2 {
			// Extract module name
			modulePart := strings.TrimPrefix(parts[0], "from ")
			imp.Module = strings.TrimSpace(modulePart)

			// Extract symbols
			symbolsPart := strings.TrimSpace(parts[1])
			if symbolsPart == "*" {
				// Star import - leave symbols empty
			} else {
				// Parse individual symbols
				symbols := strings.Split(symbolsPart, ",")
				for _, sym := range symbols {
					sym = strings.TrimSpace(sym)
					if sym == "" {
						continue
					}

					// Handle "symbol as alias"
					if strings.Contains(sym, " as ") {
						aliasParts := strings.Split(sym, " as ")
						if len(aliasParts) == 2 {
							imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
								Name:  strings.TrimSpace(aliasParts[0]),
								Alias: strings.TrimSpace(aliasParts[1]),
							})
						}
					} else {
						imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
							Name: sym,
						})
					}
				}
			}
		}
	}

	p.addNode(file, parent, imp)
}

// processClass processes class definitions and returns the class it added
func (p *NativeTreeSitterProcessor) processClass(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) *ir.DistilledClass {
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Modifiers:  []ir.Modifier{},
		Extends:    []ir.TypeRef{},
		Children:   []ir.DistilledNode{},
		Decorators: []string{},
	}

	// Walk through children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			if class.Name == "" {
				class.Name = p.getNodeText(child)
			}

		case "argument_list":
			// Base classes
			for j := 0; j < int(child.ChildCount()); j++ {
				arg := child.Child(uint(j))
				if arg.IsNamed() && (arg.Kind() == "identifier" || arg.Kind() == "attribute") {
					class.Extends = append(class.Extends, ir.TypeRef{
						Name: p.getNodeText(arg),
					})
				}
			}

		case "block":
			// Class body
			p.processClassBody(child, file, class)
		}
	}

	// Set visibility
	if p.isPrivateName(class.Name) {
		class.Visibility = ir.VisibilityPrivate
	} else {
		class.Visibility = ir.VisibilityPublic
	}

	p.addNode(file, parent, class)
	return class
}

// processClassBody processes the body of a class
func (p *NativeTreeSitterProcessor) processClassBody(node *sitter.Node, file *ir.DistilledFile, class *ir.DistilledClass) {
	doc := docstringIndex(node)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if i == doc {
			// The class docstring documents the class itself
			p.addNode(file, class, p.docstringComment(child.NamedChild(0), true))
			continue
		}
		switch child.Kind() {
		case "function_definition", "async_function_definition":
			p.processFunction(child, file, class, child.Kind() == "async_function_definition")

		case "decorated_definition":
			p.processDecoratedDefinition(child, file, class)

		case "class_definition":
			p.processClass(child, file, class)

		case "expression_statement":
			// Check for bare strings or assignments
			if child.ChildCount() > 0 {
				expr := child.Child(0)
				if expr.Kind() == "string" {
					p.processBareString(expr, file, class)
				} else if expr.Kind() == "assignment" {
					p.processAssignment(expr, file, class)
				}
			}

		case "assignment":
			p.processAssignment(child, file, class)
		}
	}
}

// processFunction processes function definitions and returns the function it
// added. A docstring becomes a doc comment placed right before the function
// and is not part of its implementation.
func (p *NativeTreeSitterProcessor) processFunction(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, isAsync bool) *ir.DistilledFunction {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
		Decorators: []string{},
	}
	var doc *ir.DistilledComment

	if isAsync {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}

	// Walk through children
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			if fn.Name == "" {
				fn.Name = p.getNodeText(child)
			}

		case "parameters":
			p.processParameters(child, fn)

		case "type":
			// Return type annotation
			fn.Returns = &ir.TypeRef{
				Name: p.getNodeText(child),
			}

		case "block":
			// Function body; the implementation starts after the docstring
			startByte := child.StartByte()
			endByte := child.EndByte()
			if i := docstringIndex(child); i >= 0 {
				doc = p.docstringComment(child.Child(uint(i)).NamedChild(0), false)
				startByte = endByte
				fn.DocOnlyBody = true
				for j := i + 1; j < int(child.ChildCount()); j++ {
					if next := child.Child(uint(j)); next.IsNamed() {
						startByte = next.StartByte()
						fn.DocOnlyBody = false
						break
					}
				}
			}
			if int(startByte) <= len(p.source) && int(endByte) <= len(p.source) {
				fn.Implementation = string(p.source[startByte:endByte])
			}

			// Also process the contents of the block recursively
			// This allows us to extract field assignments like self.field = value
			// We pass a special context that includes both the function and its parent (class)
			for j := 0; j < int(child.ChildCount()); j++ {
				blockChild := child.Child(uint(j))
				p.processNodeWithClassContext(blockChild, file, fn, parent)
			}
		}
	}

	// Set visibility
	if p.isPrivateName(fn.Name) {
		fn.Visibility = ir.VisibilityPrivate
	} else {
		fn.Visibility = ir.VisibilityPublic
	}

	// The fields found in the body are already in place; the docstring goes
	// directly before the function it documents
	if doc != nil {
		p.addNode(file, parent, doc)
	}
	p.addNode(file, parent, fn)
	return fn
}

// processDecoratedDefinition processes decorated functions/classes
func (p *NativeTreeSitterProcessor) processDecoratedDefinition(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	decorators := []string{}
	var definition *sitter.Node

	// Collect decorators and find the definition
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "decorator" {
			// Extract decorator name (skip @ symbol)
			for j := 0; j < int(child.ChildCount()); j++ {
				decChild := child.Child(uint(j))
				if decChild.Kind() == "identifier" || decChild.Kind() == "attribute" || decChild.Kind() == "call" {
					decorators = append(decorators, p.getNodeText(decChild))
				}
			}
		} else if child.Kind() == "function_definition" || child.Kind() == "async_function_definition" || child.Kind() == "class_definition" {
			definition = child
		}
	}

	if definition == nil {
		return
	}

	// Process the definition and give it the decorators. The node it adds is
	// not necessarily the first new child: fields assigned in a method body
	// and the docstring come before the method. The definition starts at its
	// first decorator.
	start := p.nodeLocation(node).Location
	switch definition.Kind() {
	case "function_definition", "async_function_definition":
		fn := p.processFunction(definition, file, parent, definition.Kind() == "async_function_definition")
		fn.Decorators = decorators
		startAt(&fn.Location, start)

	case "class_definition":
		class := p.processClass(definition, file, parent)
		class.Decorators = decorators
		startAt(&class.Location, start)
	}
}

// startAt moves the start of loc to the start of from.
func startAt(loc *ir.Location, from ir.Location) {
	loc.StartLine, loc.StartColumn, loc.StartByte = from.StartLine, from.StartColumn, from.StartByte
}

// processParameters processes function parameters
func (p *NativeTreeSitterProcessor) processParameters(node *sitter.Node, fn *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))

		// Skip punctuation
		if !child.IsNamed() {
			continue
		}

		param := &ir.Parameter{}

		switch child.Kind() {
		case "identifier":
			param.Name = p.getNodeText(child)

		case "typed_parameter":
			// parameter: type
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" && param.Name == "" {
					param.Name = p.getNodeText(subChild)
				} else if subChild.Kind() == "type" {
					param.Type = ir.TypeRef{
						Name: p.getNodeText(subChild),
					}
				}
			}

		case "default_parameter":
			// parameter = default
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" && param.Name == "" {
					param.Name = p.getNodeText(subChild)
				} else if subChild.IsNamed() && j > 0 { // Skip identifier and =
					param.DefaultValue = p.getNodeText(subChild)
				}
			}

		case "typed_default_parameter":
			// parameter: type = default
			foundType := false
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" && param.Name == "" {
					param.Name = p.getNodeText(subChild)
				} else if subChild.Kind() == "type" && !foundType {
					param.Type = ir.TypeRef{
						Name: p.getNodeText(subChild),
					}
					foundType = true
				} else if subChild.IsNamed() && foundType {
					param.DefaultValue = p.getNodeText(subChild)
				}
			}

		case "list_splat_parameter":
			// *args
			if child.ChildCount() > 0 {
				nameNode := child.Child(0)
				if nameNode != nil && nameNode.Kind() == "identifier" {
					param.Name = "*" + p.getNodeText(nameNode)
				}
			}

		case "dictionary_splat_parameter":
			// **kwargs
			if child.ChildCount() > 0 {
				nameNode := child.Child(0)
				if nameNode != nil && nameNode.Kind() == "identifier" {
					param.Name = "**" + p.getNodeText(nameNode)
				}
			}
		}

		if param.Name != "" {
			fn.Parameters = append(fn.Parameters, *param)
		}
	}
}

// assignmentParts reads an assignment through the grammar's field names
// (left, type, right), so `x = 1`, `x: int`, `x: int = 1` and `self.x: int = 1`
// all resolve to the right name, type and value. leftNode is returned so
// callers can detect `self.x` (attribute) targets.
func (p *NativeTreeSitterProcessor) assignmentParts(node *sitter.Node) (name string, typeRef *ir.TypeRef, value string, leftNode *sitter.Node) {
	leftNode = node.ChildByFieldName("left")
	if leftNode != nil {
		switch leftNode.Kind() {
		case "identifier":
			name = p.getNodeText(leftNode)
		case "attribute":
			// self.field_name: use the attribute (field) name, not the object
			if attr := leftNode.ChildByFieldName("attribute"); attr != nil {
				name = p.getNodeText(attr)
			}
		}
	}
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		typeRef = &ir.TypeRef{Name: p.getNodeText(typeNode)}
	}
	if rightNode := node.ChildByFieldName("right"); rightNode != nil {
		value = p.getNodeText(rightNode)
	}
	return name, typeRef, value, leftNode
}

// processAssignment processes variable assignments
func (p *NativeTreeSitterProcessor) processAssignment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	name, typeRef, value, leftNode := p.assignmentParts(node)

	if name != "" {
		field := &ir.DistilledField{
			BaseNode:     p.nodeLocation(node),
			Name:         name,
			Type:         typeRef,
			DefaultValue: value,
		}

		// Set visibility
		if p.isPrivateName(name) {
			field.Visibility = ir.VisibilityPrivate
		} else {
			field.Visibility = ir.VisibilityPublic
		}

		// Special handling for self.field assignments in Python:
		// If this is an attribute assignment (self.field = value) and we're inside a function
		// that's inside a class, associate the field with the class instead of the function
		targetParent := parent
		if leftNode != nil && leftNode.Kind() == "attribute" {
			// Check if we're in a function that's inside a class
			if function, ok := parent.(*ir.DistilledFunction); ok {
				// Look through the file to find which class contains this function
				if class := p.findClassContainingFunction(file, function); class != nil {
					targetParent = class
				}
			}
		}

		p.addNode(file, targetParent, field)
	}
}

// processNodeWithClassContext processes a node with knowledge of both function and class context
func (p *NativeTreeSitterProcessor) processNodeWithClassContext(node *sitter.Node, file *ir.DistilledFile, function *ir.DistilledFunction, classParent ir.DistilledNode) {
	switch node.Kind() {
	case "assignment":
		p.processAssignmentWithClassContext(node, file, function, classParent)

	case "expression_statement":
		// Check for assignments within expression statements
		if node.ChildCount() > 0 {
			child := node.Child(0)
			if child.Kind() == "assignment" {
				p.processAssignmentWithClassContext(child, file, function, classParent)
			}
		}

	default:
		// For other node types, recurse into children with the same context
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNodeWithClassContext(node.Child(uint(i)), file, function, classParent)
		}
	}
}

// processAssignmentWithClassContext processes assignments with class context available
func (p *NativeTreeSitterProcessor) processAssignmentWithClassContext(node *sitter.Node, file *ir.DistilledFile, function *ir.DistilledFunction, classParent ir.DistilledNode) {
	name, typeRef, value, leftNode := p.assignmentParts(node)

	if name != "" {
		field := &ir.DistilledField{
			BaseNode:     p.nodeLocation(node),
			Name:         name,
			Type:         typeRef,
			DefaultValue: value,
		}

		// Set visibility
		if p.isPrivateName(name) {
			field.Visibility = ir.VisibilityPrivate
		} else {
			field.Visibility = ir.VisibilityPublic
		}

		// Determine target parent: if this is a self.field assignment and we have a class context, use the class
		var targetParent ir.DistilledNode = function // default to function
		if leftNode != nil && leftNode.Kind() == "attribute" && classParent != nil {
			if class, ok := classParent.(*ir.DistilledClass); ok {
				targetParent = class
			}
		}

		p.addNode(file, targetParent, field)
	}
}

// findClassContainingFunction searches for the class that contains the given function
func (p *NativeTreeSitterProcessor) findClassContainingFunction(file *ir.DistilledFile, function *ir.DistilledFunction) *ir.DistilledClass {
	// Search through all children of the file
	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok {
			// Check if this class contains the function
			for _, classChild := range class.Children {
				if classChild == function {
					return class
				}
			}
		}
	}
	return nil
}

// docstringIndex returns the index among body's children of body's docstring:
// its first statement, when that is a bare string. Comments are not
// statements. It returns -1 when body has no docstring. body is a module or
// the block of a class or function.
func docstringIndex(body *sitter.Node) int {
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(uint(i))
		if !child.IsNamed() || child.Kind() == "comment" {
			continue
		}
		if child.Kind() == "expression_statement" && child.NamedChildCount() == 1 && child.NamedChild(0).Kind() == "string" {
			return i
		}
		return -1
	}
	return -1
}

// docstringComment returns the doc comment for a docstring (a string node).
// inner marks the docstring of a module or class, which documents its
// container rather than the declaration after it.
func (p *NativeTreeSitterProcessor) docstringComment(node *sitter.Node, inner bool) *ir.DistilledComment {
	return &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     cleanDocstring(p.stringContent(node)),
		Format:   ir.CommentDoc,
		Inner:    inner,
	}
}

// processBareString keeps a bare string that is not a docstring (one after
// the first statement of a module or class) as a block comment, so it
// follows --comments like the other comments.
func (p *NativeTreeSitterProcessor) processBareString(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.addNode(file, parent, &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     cleanDocstring(p.stringContent(node)),
		Format:   ir.CommentBlock,
	})
}

// stringContent returns the text of a string node between its quotes; a
// prefix such as r or u belongs to the opening quotes.
func (p *NativeTreeSitterProcessor) stringContent(node *sitter.Node) string {
	start, end := node.StartByte(), node.EndByte()
	for i := 0; i < int(node.ChildCount()); i++ {
		switch child := node.Child(uint(i)); child.Kind() {
		case "string_start":
			start = child.EndByte()
		case "string_end":
			end = child.StartByte()
		}
	}
	if start > end || int(end) > len(p.source) {
		return ""
	}
	return string(p.source[start:end])
}

// cleanDocstring trims a docstring the way Python's inspect.cleandoc does:
// the first line loses its leading whitespace, the lines after it the
// indentation they share, and blank lines at either end are dropped. Trailing
// whitespace is removed from every line.
func cleanDocstring(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	margin := -1
	for _, line := range lines[1:] {
		if content := strings.TrimLeft(line, " \t"); content != "" {
			if indent := len(line) - len(content); margin < 0 || indent < margin {
				margin = indent
			}
		}
	}
	for i, line := range lines {
		switch {
		case i == 0 || strings.TrimSpace(line) == "":
			line = strings.TrimLeft(line, " \t")
		case margin > 0:
			line = line[margin:]
		}
		lines[i] = strings.TrimRight(line, " \t")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n")
}

// processComment processes regular comments
func (p *NativeTreeSitterProcessor) processComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	text := p.getNodeText(node)

	// Remove # and trim
	text = strings.TrimPrefix(text, "#")
	text = strings.TrimSpace(text)

	comment := &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     text,
		Format:   ir.CommentLine,
	}

	p.addNode(file, parent, comment)
}

// Helper methods

// isPrivateName checks if a name should be considered private in Python
func (p *NativeTreeSitterProcessor) isPrivateName(name string) bool {
	// In Python, names starting with _ are considered private
	// BUT dunder methods (like __init__, __repr__) are public API
	if len(name) == 0 {
		return false
	}

	// Dunder methods are public
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return false
	}

	// Single underscore prefix means private
	return name[0] == '_'
}

func (p *NativeTreeSitterProcessor) nodeLocation(node *sitter.Node) ir.BaseNode {
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

func (p *NativeTreeSitterProcessor) getNodeText(node *sitter.Node) string {
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

func (p *NativeTreeSitterProcessor) addNode(file *ir.DistilledFile, parent ir.DistilledNode, node ir.DistilledNode) {
	if parent != nil {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, node)
		case *ir.DistilledFunction:
			// Functions don't typically have children in Python
		}
	} else {
		file.Children = append(file.Children, node)
	}
}

// analyzeProtocolSatisfaction analyzes protocol implementations using duck typing
func (p *NativeTreeSitterProcessor) analyzeProtocolSatisfaction(file *ir.DistilledFile) {
	// Collect all protocols and classes
	protocols := make(map[string]*ir.DistilledClass)
	classes := make(map[string]*ir.DistilledClass)

	for _, child := range file.Children {
		if class, ok := child.(*ir.DistilledClass); ok {
			// Check if it's a Protocol by looking for Protocol in inheritance
			isProtocol := false
			for _, ext := range class.Extends {
				if ext.Name == "Protocol" {
					isProtocol = true
					break
				}
			}

			if isProtocol {
				protocols[class.Name] = class
			} else {
				classes[class.Name] = class
			}
		}
	}

	// For each class, check if it satisfies any protocols (duck typing)
	for _, class := range classes {
		for protocolName, protocol := range protocols {
			if p.classSatisfiesProtocol(class, protocol) {
				// Add implicit protocol satisfaction
				class.Implements = append(class.Implements, ir.TypeRef{
					Name: protocolName + " (duck typing)",
				})
			}
		}
	}
}

// classSatisfiesProtocol checks if a class satisfies a protocol via duck typing
func (p *NativeTreeSitterProcessor) classSatisfiesProtocol(class *ir.DistilledClass, protocol *ir.DistilledClass) bool {
	// Create method map for the class
	classMethods := make(map[string]*ir.DistilledFunction)

	for _, child := range class.Children {
		if method, ok := child.(*ir.DistilledFunction); ok {
			// Only consider public methods for protocol satisfaction
			if method.Visibility == ir.VisibilityPublic {
				classMethods[method.Name] = method
			}
		}
	}

	// Check if class has all protocol methods
	for _, child := range protocol.Children {
		if protocolMethod, ok := child.(*ir.DistilledFunction); ok {
			classMethod, exists := classMethods[protocolMethod.Name]
			if !exists {
				return false
			}

			// Check method signature compatibility (simplified)
			if !p.methodSignaturesCompatible(classMethod, protocolMethod) {
				return false
			}
		}
	}

	return true
}

// methodSignaturesCompatible checks if two method signatures are compatible
func (p *NativeTreeSitterProcessor) methodSignaturesCompatible(classMethod, protocolMethod *ir.DistilledFunction) bool {
	// Check parameter count (excluding 'self')
	classParams := p.getNonSelfParameters(classMethod.Parameters)
	protocolParams := p.getNonSelfParameters(protocolMethod.Parameters)

	if len(classParams) != len(protocolParams) {
		return false
	}

	// Check parameter types (if specified)
	for i, classParam := range classParams {
		protocolParam := protocolParams[i]

		// If protocol specifies type, class must match (simplified check)
		if protocolParam.Type.Name != "" && classParam.Type.Name != "" {
			if !p.typesCompatible(classParam.Type, protocolParam.Type) {
				return false
			}
		}
	}

	// Check return type compatibility
	if protocolMethod.Returns != nil && classMethod.Returns != nil {
		return p.typesCompatible(*classMethod.Returns, *protocolMethod.Returns)
	}

	return true
}

// getNonSelfParameters filters out 'self' parameter
func (p *NativeTreeSitterProcessor) getNonSelfParameters(params []ir.Parameter) []ir.Parameter {
	var result []ir.Parameter
	for _, param := range params {
		if param.Name != "self" && param.Name != "cls" {
			result = append(result, param)
		}
	}
	return result
}

// typesCompatible checks if two types are compatible (simplified)
func (p *NativeTreeSitterProcessor) typesCompatible(type1, type2 ir.TypeRef) bool {
	// Simple string comparison for now
	return strings.TrimSpace(type1.Name) == strings.TrimSpace(type2.Name)
}

// dumpTree creates a structured representation of the AST for debugging
func (p *NativeTreeSitterProcessor) dumpTree(node *sitter.Node, depth int) map[string]interface{} {
	if node == nil {
		return nil
	}

	result := map[string]interface{}{
		"type":      node.Kind(),
		"named":     node.IsNamed(),
		"startLine": node.StartPosition().Row + 1,
		"endLine":   node.EndPosition().Row + 1,
		"startCol":  node.StartPosition().Column,
		"endCol":    node.EndPosition().Column,
	}

	// Add node text for leaf nodes or small nodes
	if node.ChildCount() == 0 || (node.EndByte()-node.StartByte() < 100) {
		text := p.getNodeText(node)
		if text != "" && len(text) < 100 {
			result["text"] = text
		}
	}

	// Add children
	if node.ChildCount() > 0 {
		children := make([]map[string]interface{}, 0, node.ChildCount())
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(uint(i))
			if child != nil {
				children = append(children, p.dumpTree(child, depth+1))
			}
		}
		result["children"] = children
	}

	return result
}
