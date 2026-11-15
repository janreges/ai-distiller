// Not on the live path: the line parser is used. Kept for the tree-sitter migration (see docs/superpowers/plans/2026-11-15-aid-hardening-00-roadmap.md).

package swift

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	sitter "github.com/smacker/go-tree-sitter"
	swift "tree-sitter-swift"
)

// TreeSitterProcessor processes Swift using tree-sitter
type TreeSitterProcessor struct {
	parser   *sitter.Parser
	source   []byte
	filename string

	// State for context-aware parsing
	currentModule string
	importedTypes map[string]string

	// Track current context for nested structures
	currentClass    *ir.DistilledClass
	currentProtocol *ir.DistilledInterface
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() (*TreeSitterProcessor, error) {
	lang := swift.Language()
	if lang == nil {
		return nil, fmt.Errorf("tree-sitter-swift is not available (CGO disabled)")
	}

	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(lang))

	return &TreeSitterProcessor{
		parser:        parser,
		importedTypes: make(map[string]string),
	}, nil
}

// ProcessSource processes Swift source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	p.source = source
	p.filename = filename

	// Reset state
	p.currentModule = ""
	p.importedTypes = make(map[string]string)
	p.currentClass = nil
	p.currentProtocol = nil

	// Defer panic recovery to catch segfaults
	defer func() {
		if r := recover(); r != nil {
			// Tree-sitter crashed, return error to trigger line-based parser
			panic(fmt.Errorf("tree-sitter panic: %v", r))
		}
	}()

	// Parse with tree-sitter
	tree, err := p.parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Close()

	// Create IR file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPoint().Row) + 1,
			},
		},
		Path:     filename,
		Language: "swift",
		Version:  "5.x",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Process the AST
	p.processNode(tree.RootNode(), file, nil)

	// Check if we got meaningful results
	// If we only have imports or less than 2 nodes, it's likely tree-sitter failed
	nonImportCount := 0
	for _, child := range file.Children {
		if _, isImport := child.(*ir.DistilledImport); !isImport {
			nonImportCount++
		}
	}

	// If source has significant content but we only found imports, tree-sitter likely failed
	if nonImportCount == 0 && len(source) > 100 && strings.Contains(string(source), "func") {
		return nil, fmt.Errorf("tree-sitter produced insufficient results")
	}

	return file, nil
}

// processNode recursively processes tree-sitter nodes
func (p *TreeSitterProcessor) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if node == nil {
		return
	}

	nodeType := node.Type()

	// Skip non-named nodes except comments
	if !node.IsNamed() && nodeType != "comment" && nodeType != "multiline_comment" {
		// Process children for anonymous nodes
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(i), file, parent)
		}
		return
	}

	switch nodeType {
	case "source_file":
		// Process all top-level declarations
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(i), file, parent)
		}

	case "import_declaration":
		p.processImport(node, file, parent)

	case "class_declaration":
		// Swift tree-sitter uses class_declaration for enum/struct/class
		// We need to check the source text to determine the actual type
		text := strings.TrimSpace(p.getNodeText(node))

		// Remove visibility modifiers to check the actual type
		// Common patterns: "public enum", "private struct", "final class", etc.
		words := strings.Fields(text)
		for _, word := range words {
			if word == "enum" {
				p.processEnum(node, file, parent)
				return
			} else if word == "struct" {
				p.processStruct(node, file, parent)
				return
			} else if word == "class" {
				p.processClass(node, file, parent)
				return
			}
		}

	case "struct_declaration":
		p.processStruct(node, file, parent)

	case "enum_declaration":
		p.processEnum(node, file, parent)

	case "protocol_declaration":
		p.processProtocol(node, file, parent)

	case "extension_declaration":
		p.processExtension(node, file, parent)

	case "function_declaration":
		p.processFunction(node, file, parent)

	case "init_declaration":
		p.processInit(node, file, parent)

	case "deinit_declaration":
		p.processDeinit(node, file, parent)

	case "variable_declaration":
		// Handle top-level let/var declarations
		p.processProperty(node, file, parent)

	case "property_declaration":
		p.processProperty(node, file, parent)

	case "constant_declaration":
		// Handle let declarations
		p.processProperty(node, file, parent)

	case "actor_declaration":
		p.processActor(node, file, parent)

	case "comment", "multiline_comment":
		p.processComment(node, file, parent)

	default:
		// Process children for unhandled node types
		for i := 0; i < int(node.ChildCount()); i++ {
			p.processNode(node.Child(i), file, parent)
		}
	}
}

// Helper functions for processing specific node types

func (p *TreeSitterProcessor) processImport(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	imp := &ir.DistilledImport{
		BaseNode:   p.nodeLocation(node),
		ImportType: "import",
		Module:     p.getImportPath(node),
		Symbols:    []ir.ImportedSymbol{},
	}

	p.addNode(file, parent, imp)
}

func (p *TreeSitterProcessor) processClass(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Modifiers:  p.getModifiers(node),
		Extends:    p.getSuperclasses(node),
		Implements: p.getProtocols(node),
		Children:   []ir.DistilledNode{},
		Decorators: p.getAttributes(node),
	}

	// Set current class context
	prevClass := p.currentClass
	p.currentClass = class

	// Process class body
	// Process all children of the class declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "class_body" {
			// Process class body contents
			for j := 0; j < int(child.ChildCount()); j++ {
				p.processNode(child.Child(j), file, class)
			}
		}
	}

	// Restore previous context
	p.currentClass = prevClass

	p.addNode(file, parent, class)
}

func (p *TreeSitterProcessor) processFunction(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Modifiers:  p.getFunctionModifiers(node),
		Parameters: p.getParameters(node),
		Returns:    p.getReturnType(node),
		Decorators: p.getAttributes(node),
		TypeParams: p.getGenericParameters(node),
	}

	// Get implementation if present
	if body := p.findChildByType(node, "function_body"); body != nil {
		fn.Implementation = p.getNodeText(body)
	}

	p.addNode(file, parent, fn)
}

func (p *TreeSitterProcessor) processInit(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Name:       "init",
		Visibility: p.getVisibility(node),
		Modifiers:  p.getFunctionModifiers(node),
		Parameters: p.getParameters(node),
		Decorators: p.getAttributes(node),
	}

	// Get implementation if present
	if body := p.findChildByType(node, "function_body"); body != nil {
		fn.Implementation = p.getNodeText(body)
	}

	p.addNode(file, parent, fn)
}

func (p *TreeSitterProcessor) processDeinit(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode:   p.nodeLocation(node),
		Name:       "deinit",
		Visibility: p.getVisibility(node),
		Modifiers:  p.getFunctionModifiers(node),
		Decorators: p.getAttributes(node),
	}

	// Get implementation if present
	if body := p.findChildByType(node, "function_body"); body != nil {
		fn.Implementation = p.getNodeText(body)
	}

	p.addNode(file, parent, fn)
}

// Helper methods

func (p *TreeSitterProcessor) nodeLocation(node *sitter.Node) ir.BaseNode {
	return ir.BaseNode{
		Location: ir.Location{
			StartLine: int(node.StartPoint().Row) + 1,
			EndLine:   int(node.EndPoint().Row) + 1,
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

func (p *TreeSitterProcessor) findChildByType(node *sitter.Node, nodeType string) *sitter.Node {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == nodeType {
			return child
		}
	}
	return nil
}

func (p *TreeSitterProcessor) getNodeName(node *sitter.Node) string {
	// For class/struct/enum declarations, the name is in type_identifier
	if nameNode := p.findChildByType(node, "type_identifier"); nameNode != nil {
		return p.getNodeText(nameNode)
	}
	// For functions and other declarations, it's in simple_identifier
	if nameNode := p.findChildByType(node, "simple_identifier"); nameNode != nil {
		return p.getNodeText(nameNode)
	}
	if nameNode := p.findChildByType(node, "identifier"); nameNode != nil {
		return p.getNodeText(nameNode)
	}
	return ""
}

func (p *TreeSitterProcessor) getVisibility(node *sitter.Node) ir.Visibility {
	// Look for modifiers child node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "modifiers" {
			// Look for visibility_modifier within modifiers
			for j := 0; j < int(child.ChildCount()); j++ {
				modChild := child.Child(j)
				if modChild.Type() == "visibility_modifier" {
					// Get the actual visibility keyword
					if modChild.ChildCount() > 0 {
						visKeyword := modChild.Child(0)
						switch visKeyword.Type() {
						case "private":
							return ir.VisibilityPrivate
						case "fileprivate":
							return ir.VisibilityFilePrivate
						case "internal":
							return ir.VisibilityInternal
						case "public":
							return ir.VisibilityPublic
						case "open":
							return ir.VisibilityOpen
						}
					}
				}
			}
		}
	}
	// Default visibility in Swift is internal
	return ir.VisibilityInternal
}

func (p *TreeSitterProcessor) getModifiers(node *sitter.Node) []ir.Modifier {
	var modifiers []ir.Modifier

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := p.getNodeText(child)

		switch text {
		case "final":
			modifiers = append(modifiers, ir.ModifierFinal)
		case "static":
			modifiers = append(modifiers, ir.ModifierStatic)
		case "class":
			if child.Type() == "ownership_modifier" {
				modifiers = append(modifiers, ir.ModifierStatic)
			}
		}
	}

	return modifiers
}

// Stub implementations for remaining helper methods
func (p *TreeSitterProcessor) getImportPath(node *sitter.Node) string {
	// Look for identifier after import keyword
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "simple_identifier" {
			return p.getNodeText(child)
		}
	}
	return ""
}

func (p *TreeSitterProcessor) getSuperclasses(node *sitter.Node) []ir.TypeRef {
	var superclasses []ir.TypeRef

	// Look for type_inheritance_clause
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_inheritance_clause" {
			// Process each inherited type
			for j := 0; j < int(child.ChildCount()); j++ {
				inheritChild := child.Child(j)
				if inheritChild.Type() == "user_type" {
					superclasses = append(superclasses, ir.TypeRef{
						Name: p.getNodeText(inheritChild),
					})
				}
			}
		}
	}

	return superclasses
}

func (p *TreeSitterProcessor) getProtocols(node *sitter.Node) []ir.TypeRef {
	// In Swift, both superclasses and protocols are in the type_inheritance_clause
	// For now, we'll treat them the same as getSuperclasses
	// A more sophisticated implementation would distinguish between them
	return p.getSuperclasses(node)
}

func (p *TreeSitterProcessor) getFunctionModifiers(node *sitter.Node) []ir.Modifier {
	modifiers := []ir.Modifier{}

	// Look for modifiers child node
	if modifiersNode := p.findChildByType(node, "modifiers"); modifiersNode != nil {
		for i := 0; i < int(modifiersNode.ChildCount()); i++ {
			child := modifiersNode.Child(i)
			switch child.Type() {
			case "async_modifier":
				modifiers = append(modifiers, ir.ModifierAsync)
			case "throws_keyword":
				modifiers = append(modifiers, ir.ModifierThrows)
			case "rethrows_keyword":
				modifiers = append(modifiers, ir.ModifierRethrows)
			case "mutation_modifier":
				text := p.getNodeText(child)
				if text == "mutating" {
					modifiers = append(modifiers, ir.ModifierMutating)
				} else if text == "nonmutating" {
					modifiers = append(modifiers, ir.ModifierNonMutating)
				}
			case "static_modifier":
				modifiers = append(modifiers, ir.ModifierStatic)
			case "class_modifier":
				modifiers = append(modifiers, ir.ModifierClass)
			case "final_modifier":
				modifiers = append(modifiers, ir.ModifierFinal)
			}
		}
	}

	// Also check for async/throws as direct children of function node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "async_keyword":
			modifiers = append(modifiers, ir.ModifierAsync)
		case "throws_keyword":
			modifiers = append(modifiers, ir.ModifierThrows)
		case "rethrows_keyword":
			modifiers = append(modifiers, ir.ModifierRethrows)
		}
	}

	return modifiers
}

func (p *TreeSitterProcessor) getGenericParameters(node *sitter.Node) []ir.TypeParam {
	params := []ir.TypeParam{}

	// Look for type_parameters
	if typeParams := p.findChildByType(node, "type_parameters"); typeParams != nil {
		for i := 0; i < int(typeParams.ChildCount()); i++ {
			child := typeParams.Child(i)
			if child.Type() == "type_parameter" {
				param := ir.TypeParam{
					Name: p.getNodeName(child),
				}

				// TODO: Add constraint parsing
				params = append(params, param)
			}
		}
	}

	return params
}

func (p *TreeSitterProcessor) getAttributes(node *sitter.Node) []string {
	// TODO: Implement attribute extraction
	return nil
}

func (p *TreeSitterProcessor) getFunctionName(node *sitter.Node) string {
	return p.getNodeName(node)
}

func (p *TreeSitterProcessor) getParameters(node *sitter.Node) []ir.Parameter {
	params := []ir.Parameter{}

	// Find parameter clause
	paramClause := p.findChildByType(node, "parameter_clause")
	if paramClause == nil {
		return params
	}

	// Process each parameter
	for i := 0; i < int(paramClause.ChildCount()); i++ {
		child := paramClause.Child(i)
		if child.Type() == "parameter" {
			param := ir.Parameter{
				Name: p.getParameterName(child),
			}

			// Get parameter type
			if typeAnnotation := p.findChildByType(child, "type_annotation"); typeAnnotation != nil {
				param.Type = p.parseType(typeAnnotation)
			}

			// Get default value
			if defaultValue := p.findChildByType(child, "parameter_default_value"); defaultValue != nil {
				param.DefaultValue = p.getNodeText(defaultValue)
			}

			params = append(params, param)
		}
	}

	return params
}

func (p *TreeSitterProcessor) getParameterName(node *sitter.Node) string {
	// Swift parameters can have external and internal names
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "simple_identifier" {
			return p.getNodeText(child)
		}
	}
	return ""
}

func (p *TreeSitterProcessor) parseType(node *sitter.Node) ir.TypeRef {
	// Skip the type_annotation node and get the actual type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "user_type" || child.Type() == "some_type" ||
			child.Type() == "any_type" || child.Type() == "opaque_type" ||
			child.Type() == "tuple_type" || child.Type() == "array_type" ||
			child.Type() == "dictionary_type" || child.Type() == "optional_type" {
			return ir.TypeRef{
				Name: p.getNodeText(child),
			}
		}
	}

	return ir.TypeRef{
		Name: p.getNodeText(node),
	}
}

func (p *TreeSitterProcessor) getReturnType(node *sitter.Node) *ir.TypeRef {
	// Look for -> followed by a type
	foundArrow := false

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)

		if child.Type() == "->" {
			foundArrow = true
			continue
		}

		if foundArrow && (child.Type() == "user_type" || child.Type() == "optional_type" ||
			child.Type() == "array_type" || child.Type() == "tuple_type") {
			return &ir.TypeRef{
				Name: p.getNodeText(child),
			}
		}
	}

	return nil
}

func (p *TreeSitterProcessor) processStruct(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Structs are similar to classes but value types
	struct_ := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Modifiers:  append(p.getModifiers(node), ir.ModifierStruct),
		Extends:    p.getSuperclasses(node),
		Implements: p.getProtocols(node),
		Children:   []ir.DistilledNode{},
		Decorators: p.getAttributes(node),
	}

	// Set current class context
	prevClass := p.currentClass
	p.currentClass = struct_

	// Process all children of the struct declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.IsNamed() {
			p.processNode(child, file, struct_)
		}
	}

	// Restore previous context
	p.currentClass = prevClass

	p.addNode(file, parent, struct_)
}

func (p *TreeSitterProcessor) processEnum(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	enum := &ir.DistilledEnum{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Children:   []ir.DistilledNode{},
	}

	// Check for raw value type (inheritance)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_inheritance_clause" || child.Type() == "inheritance_specifier" {
			// Extract the raw value type
			if child.Type() == "inheritance_specifier" {
				// Direct raw value type
				enum.Type = &ir.TypeRef{
					Name: p.getNodeText(child),
				}
				break
			} else {
				// Process inheritance clause - take the first type as raw value type
				for j := 0; j < int(child.ChildCount()); j++ {
					inheritChild := child.Child(j)
					if inheritChild.Type() == "user_type" || inheritChild.Type() == "inheritance_specifier" {
						enum.Type = &ir.TypeRef{
							Name: p.getNodeText(inheritChild),
						}
						break
					}
				}
			}
		}
	}

	// Process enum body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "enum_class_body" {
			// Process enum cases
			for j := 0; j < int(child.ChildCount()); j++ {
				bodyChild := child.Child(j)
				if bodyChild.Type() == "enum_entry" {
					// Add enum case as a field
					enumCase := &ir.DistilledField{
						BaseNode:   p.nodeLocation(bodyChild),
						Visibility: ir.VisibilityPublic, // Enum cases are always public
					}

					// Process enum entry children
					hasEquals := false
					var associatedTypes []string

					for k := 0; k < int(bodyChild.ChildCount()); k++ {
						caseChild := bodyChild.Child(k)

						switch caseChild.Type() {
						case "simple_identifier":
							if enumCase.Name == "" {
								enumCase.Name = p.getNodeText(caseChild)
							}
						case "=":
							hasEquals = true
						case "line_string_literal", "integer_literal", "real_literal":
							if hasEquals {
								// Raw value
								enumCase.DefaultValue = p.getNodeText(caseChild)
							}
						case "tuple_type":
							// Associated values in tuple form
							tupleText := p.getNodeText(caseChild)
							// Extract types from tuple
							associatedTypes = append(associatedTypes, tupleText)
						}
					}

					// If we have associated types, add them to the enum case name
					if len(associatedTypes) > 0 && enumCase.Name != "" {
						enumCase.Name = enumCase.Name + associatedTypes[0]
					}

					if enumCase.Name != "" {
						enum.Children = append(enum.Children, enumCase)
					}
				}
			}
		}
	}

	p.addNode(file, parent, enum)
}

func (p *TreeSitterProcessor) processProtocol(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	protocol := &ir.DistilledInterface{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Extends:    p.getProtocols(node), // Protocols can inherit from other protocols
		Children:   []ir.DistilledNode{},
	}

	// Set current protocol context
	prevProtocol := p.currentProtocol
	p.currentProtocol = protocol

	// Process protocol body
	if body := p.findChildByType(node, "protocol_body"); body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			p.processNode(body.Child(i), file, protocol)
		}
	}

	// Restore previous context
	p.currentProtocol = prevProtocol

	p.addNode(file, parent, protocol)
}

func (p *TreeSitterProcessor) processExtension(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Extensions in Swift add functionality to existing types
	// We'll treat them as a special kind of class for now
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Name:       "extension " + p.getExtendedTypeName(node),
		Visibility: ir.VisibilityPublic,
		Implements: p.getProtocols(node),
		Children:   []ir.DistilledNode{},
	}

	// Process extension body
	if body := p.findChildByType(node, "extension_body"); body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			p.processNode(body.Child(i), file, class)
		}
	}

	p.addNode(file, parent, class)
}

func (p *TreeSitterProcessor) processActor(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Actors are similar to classes but with concurrency guarantees
	class := &ir.DistilledClass{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getNodeName(node),
		Visibility: p.getVisibility(node),
		Modifiers:  append(p.getModifiers(node), ir.ModifierActor),
		Implements: p.getProtocols(node),
		Children:   []ir.DistilledNode{},
		Decorators: p.getAttributes(node),
	}

	// Process actor body
	if body := p.findChildByType(node, "actor_body"); body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			p.processNode(body.Child(i), file, class)
		}
	}

	p.addNode(file, parent, class)
}

func (p *TreeSitterProcessor) processProperty(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode:   p.nodeLocation(node),
		Name:       p.getPropertyName(node),
		Visibility: p.getVisibility(node),
		Modifiers:  p.getModifiers(node),
		Type:       p.getPropertyType(node),
	}

	// Check for default value
	if init := p.findChildByType(node, "property_initializer"); init != nil {
		field.DefaultValue = p.getNodeText(init)
	}

	// Check for computed property accessors
	hasAccessors := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "computed_property":
			hasAccessors = true
			field.IsProperty = true
			// Look for get/set blocks within computed_property
			for j := 0; j < int(child.ChildCount()); j++ {
				accessor := child.Child(j)
				if accessor.Type() == "computed_getter" || p.getNodeText(accessor) == "get" {
					field.HasGetter = true
				} else if accessor.Type() == "computed_setter" || p.getNodeText(accessor) == "set" {
					field.HasSetter = true
				}
			}
		case "willSet", "didSet":
			// Property observers indicate this is a stored property with observers
			hasAccessors = true
		}
	}

	// If we found accessors, mark it as a property
	if hasAccessors {
		field.IsProperty = true
		// If only observers, it's a stored property that can be read and written
		if !field.HasGetter && !field.HasSetter {
			field.HasGetter = true
			field.HasSetter = true
		}
	}

	p.addNode(file, parent, field)
}

func (p *TreeSitterProcessor) processComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	comment := &ir.DistilledComment{
		BaseNode: p.nodeLocation(node),
		Text:     p.getNodeText(node),
		Format:   p.getCommentFormat(node),
	}

	p.addNode(file, parent, comment)
}

func (p *TreeSitterProcessor) getExtendedTypeName(node *sitter.Node) string {
	// The type being extended is usually a `user_type` child of the extension_declaration node
	if typeNode := p.findChildByType(node, "user_type"); typeNode != nil {
		return p.getNodeText(typeNode)
	}
	// Also check for simple_identifier (for basic types)
	if idNode := p.findChildByType(node, "simple_identifier"); idNode != nil {
		return p.getNodeText(idNode)
	}
	return "Unknown" // Keep as fallback
}

func (p *TreeSitterProcessor) getPropertyName(node *sitter.Node) string {
	// Look for pattern -> simple_identifier
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "pattern" {
			// Look for simple_identifier within pattern
			for j := 0; j < int(child.ChildCount()); j++ {
				patternChild := child.Child(j)
				if patternChild.Type() == "simple_identifier" {
					return p.getNodeText(patternChild)
				}
			}
		}
	}
	return ""
}

func (p *TreeSitterProcessor) getPropertyType(node *sitter.Node) *ir.TypeRef {
	// Look for type_annotation -> type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_annotation" {
			// Look for the type within type_annotation
			for j := 0; j < int(child.ChildCount()); j++ {
				typeChild := child.Child(j)
				if typeChild.IsNamed() && typeChild.Type() != ":" {
					return &ir.TypeRef{
						Name: p.getNodeText(typeChild),
					}
				}
			}
		}
	}
	return nil
}

func (p *TreeSitterProcessor) getCommentFormat(node *sitter.Node) string {
	text := p.getNodeText(node)
	if strings.HasPrefix(text, "///") || strings.HasPrefix(text, "/**") {
		return ir.CommentDoc
	}
	if strings.Contains(text, "\n") {
		return ir.CommentBlock
	}
	return ir.CommentLine
}

func (p *TreeSitterProcessor) addNode(file *ir.DistilledFile, parent ir.DistilledNode, node ir.DistilledNode) {
	if parent == nil {
		file.Children = append(file.Children, node)
		return
	}

	switch p := parent.(type) {
	case *ir.DistilledClass:
		p.Children = append(p.Children, node)
	case *ir.DistilledInterface:
		p.Children = append(p.Children, node)
	case *ir.DistilledEnum:
		p.Children = append(p.Children, node)
	default:
		file.Children = append(file.Children, node)
	}
}

// Close releases resources
func (p *TreeSitterProcessor) Close() {
	if p.parser != nil {
		p.parser.Close()
	}
}
