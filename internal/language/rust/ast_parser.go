package rust

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	sitter "github.com/smacker/go-tree-sitter"
)

// ASTParser handles Rust source code parsing using tree-sitter
type ASTParser struct {
	parser *sitter.Parser
	source []byte
}

// NewASTParser creates a new tree-sitter based parser for Rust
func NewASTParser() *ASTParser {
	// TODO: Add tree-sitter-rust grammar when available
	// For now, return a parser that will return an error
	return &ASTParser{
		parser: nil,
	}
}

// ProcessSource processes Rust source code and returns the IR
func (p *ASTParser) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	p.source = source

	if p.parser == nil {
		return nil, fmt.Errorf("tree-sitter-rust grammar not available yet")
	}

	tree, err := p.parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse source: %w", err)
	}
	defer tree.Close()

	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPoint().Row) + 1,
			},
		},
		Path:     filename,
		Language: "rust",
		Version:  "2021",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Process the root node
	p.processNode(tree.RootNode(), file, nil)

	return file, nil
}

// processNode recursively processes AST nodes
func (p *ASTParser) processNode(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	if node == nil {
		return
	}

	switch node.Type() {
	case "use_declaration":
		p.parseUse(node, file, parent)
	case "mod_item":
		p.parseMod(node, file, parent)
	case "struct_item":
		p.parseStruct(node, file, parent)
	case "enum_item":
		p.parseEnum(node, file, parent)
	case "trait_item":
		p.parseTrait(node, file, parent)
	case "impl_item":
		p.parseImpl(node, file, parent)
	case "function_item":
		p.parseFunction(node, file, parent)
	case "const_item":
		p.parseConst(node, file, parent)
	case "static_item":
		p.parseStatic(node, file, parent)
	case "type_alias":
		p.parseTypeAlias(node, file, parent)
	case "line_comment":
		p.parseLineComment(node, file, parent)
	case "block_comment":
		p.parseBlockComment(node, file, parent)
	default:
		// Recurse into children for unhandled node types
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, file, parent)
		}
	}
}

// parseUse parses use declarations
func (p *ASTParser) parseUse(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		ImportType: "use",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Find the use_tree node
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "use_tree" {
			p.parseUseTree(child, imp)
			break
		}
	}

	p.addChild(file, parent, imp)
}

// parseUseTree parses the use tree structure
func (p *ASTParser) parseUseTree(node *sitter.Node, imp *ir.DistilledImport) {
	switch node.Type() {
	case "use_tree":
		// Handle different use patterns
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			switch child.Type() {
			case "identifier", "scoped_identifier":
				if imp.Module == "" {
					imp.Module = p.nodeText(child)
				}
			case "use_list":
				// Handle use std::{io, fmt};
				p.parseUseList(child, imp)
			case "use_as_clause":
				// Handle use foo as bar;
				p.parseUseAs(child, imp)
			case "use_wildcard":
				// Handle use foo::*;
				imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: "*"})
			}
		}
	}
}

// parseUseList parses use lists like {io, fmt}
func (p *ASTParser) parseUseList(node *sitter.Node, imp *ir.DistilledImport) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "use_tree" {
			// For each item in the list
			itemText := p.nodeText(child)
			if itemText != "" && itemText != "," && itemText != "{" && itemText != "}" {
				imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: itemText})
			}
		}
	}
}

// parseUseAs parses use ... as ... clauses
func (p *ASTParser) parseUseAs(node *sitter.Node, imp *ir.DistilledImport) {
	// The alias is the identifier after "as"
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			alias := p.nodeText(child)
			if len(imp.Symbols) == 0 {
				// Create a symbol with the module name and alias
				imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
					Name:  imp.Module,
					Alias: alias,
				})
			}
		}
	}
}

// parseMod parses module declarations
func (p *ASTParser) parseMod(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	mod := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{}, // Modules are represented as classes
		Children:   []ir.DistilledNode{},
	}

	// Get module name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			mod.Name = p.nodeText(child)
		} else if child.Type() == "declaration_list" {
			// Process module body
			p.processModuleBody(child, file, mod)
		}
	}

	p.addChild(file, parent, mod)
}

// processModuleBody processes the contents of a module
func (p *ASTParser) processModuleBody(node *sitter.Node, file *ir.DistilledFile, parent *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		p.processNode(child, file, parent)
	}
}

// parseStruct parses struct declarations
func (p *ASTParser) parseStruct(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	strct := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierStruct},
		Children:   []ir.DistilledNode{},
	}

	var genericParams string
	var whereClause string

	// Get struct name and process fields
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			strct.Name = p.nodeText(child)
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "where_clause":
			whereClause = p.nodeText(child)
		case "field_declaration_list":
			p.parseStructFields(child, strct)
		case "ordered_field_declaration_list":
			p.parseTupleStructFields(child, strct)
		}
	}

	// Include generic parameters and where clause in name
	if genericParams != "" {
		strct.Name += genericParams
	}
	if whereClause != "" {
		strct.Name += " " + whereClause
	}

	p.addChild(file, parent, strct)
}

// parseStructFields parses struct field declarations
func (p *ASTParser) parseStructFields(node *sitter.Node, parent *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_declaration" {
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: ir.Location{
						StartLine: int(child.StartPoint().Row) + 1,
						EndLine:   int(child.EndPoint().Row) + 1,
					},
				},
				Visibility: p.parseVisibility(child),
			}

			// Get field name and type
			for j := 0; j < int(child.ChildCount()); j++ {
				fieldChild := child.Child(j)
				switch fieldChild.Type() {
				case "field_identifier":
					field.Name = p.nodeText(fieldChild)
				case "type_identifier", "primitive_type", "reference_type", "pointer_type", "array_type", "tuple_type", "generic_type":
					field.Type = &ir.TypeRef{Name: p.nodeText(fieldChild)}
				}
			}

			parent.Children = append(parent.Children, field)
		}
	}
}

// parseTupleStructFields parses tuple struct fields
func (p *ASTParser) parseTupleStructFields(node *sitter.Node, parent *ir.DistilledClass) {
	fieldIndex := 0
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		// Skip parentheses and commas
		if child.Type() != "(" && child.Type() != ")" && child.Type() != "," {
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: ir.Location{
						StartLine: int(child.StartPoint().Row) + 1,
						EndLine:   int(child.EndPoint().Row) + 1,
					},
				},
				Name:       fmt.Sprintf("%d", fieldIndex),
				Visibility: ir.VisibilityPrivate, // Tuple fields are accessed by index
				Type:       &ir.TypeRef{Name: p.nodeText(child)},
			}
			parent.Children = append(parent.Children, field)
			fieldIndex++
		}
	}
}

// parseEnum parses enum declarations
func (p *ASTParser) parseEnum(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	enum := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierEnum},
		Children:   []ir.DistilledNode{},
	}

	// Get enum name and variants
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			enum.Name = p.nodeText(child)
		case "enum_variant_list":
			p.parseEnumVariants(child, enum)
		}
	}

	p.addChild(file, parent, enum)
}

// parseEnumVariants parses enum variant declarations
func (p *ASTParser) parseEnumVariants(node *sitter.Node, parent *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "enum_variant" {
			variant := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: ir.Location{
						StartLine: int(child.StartPoint().Row) + 1,
						EndLine:   int(child.EndPoint().Row) + 1,
					},
				},
				Visibility: ir.VisibilityPublic, // Enum variants are always public
			}

			// Get variant name and associated data
			for j := 0; j < int(child.ChildCount()); j++ {
				varChild := child.Child(j)
				switch varChild.Type() {
				case "identifier":
					variant.Name = p.nodeText(varChild)
				case "field_declaration_list":
					// Struct-like variant
					variant.Type = &ir.TypeRef{Name: "struct"}
				case "ordered_field_declaration_list":
					// Tuple-like variant
					tupleTypes := p.parseTupleTypes(varChild)
					variant.Type = &ir.TypeRef{Name: fmt.Sprintf("(%s)", tupleTypes)}
				}
			}

			parent.Children = append(parent.Children, variant)
		}
	}
}

// parseTupleTypes extracts tuple types as a comma-separated string
func (p *ASTParser) parseTupleTypes(node *sitter.Node) string {
	var types []string
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "(" && child.Type() != ")" && child.Type() != "," {
			types = append(types, p.nodeText(child))
		}
	}
	return strings.Join(types, ", ")
}

// parseTrait parses trait declarations
func (p *ASTParser) parseTrait(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	trait := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierAbstract}, // Use abstract modifier for traits
		Children:   []ir.DistilledNode{},
	}

	var genericParams string
	var traitBounds string

	// Get trait name and body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			trait.Name = p.nodeText(child)
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "trait_bounds":
			traitBounds = p.nodeText(child)
		case "declaration_list":
			// Process trait body
			p.processTraitBody(child, file, trait)
		}
	}

	// Include generic parameters in the name
	if genericParams != "" {
		trait.Name += genericParams
	}
	if traitBounds != "" {
		trait.Name += ": " + traitBounds
	}

	p.addChild(file, parent, trait)
}

// processTraitBody processes the contents of a trait
func (p *ASTParser) processTraitBody(node *sitter.Node, file *ir.DistilledFile, parent *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		// Debug: print all child types
		// fmt.Printf("Trait body child: %s\n", child.Type())
		switch child.Type() {
		case "function_signature_item":
			p.parseFunctionSignature(child, file, parent)
		case "function_item":
			// Trait methods with default implementations
			p.parseFunction(child, file, parent)
		case "associated_type":
			p.parseAssociatedType(child, parent)
		case "{", "}":
			// Skip braces
		default:
			// Recurse for any other node types
			for j := 0; j < int(child.ChildCount()); j++ {
				p.processTraitBody(child.Child(j), file, parent)
			}
		}
	}
}

// parseFunctionSignature parses trait method signatures
func (p *ASTParser) parseFunctionSignature(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: ir.VisibilityPublic, // Trait methods are public
		Modifiers:  []ir.Modifier{ir.ModifierAbstract},
		Parameters: []ir.Parameter{},
	}

	var genericParams string
	var whereClause string
	var returnType string
	var returnStart int = -1

	// Parse function signature
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			if fn.Name == "" {
				fn.Name = p.nodeText(child)
			}
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "parameters":
			fn.Parameters = p.parseParameters(child)
		case "->":
			returnStart = i
		case "where_clause":
			whereClause = p.nodeText(child)
		default:
			// Capture return type after ->
			if returnStart >= 0 && i > returnStart && child.Type() != "where_clause" {
				if returnType == "" {
					returnType = p.nodeText(child)
				}
			}
		}
	}

	// Include generic parameters in function name
	if genericParams != "" {
		fn.Name += genericParams
	}

	// Build complete return type with where clause
	if returnType != "" {
		if whereClause != "" {
			returnType += " " + whereClause
		}
		fn.Returns = &ir.TypeRef{Name: returnType}
	}

	p.addChild(file, parent, fn)
}

// parseAssociatedType parses associated type declarations in traits
func (p *ASTParser) parseAssociatedType(node *sitter.Node, parent *ir.DistilledClass) {
	assocType := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierAbstract},
	}

	var typeName string
	var genericParams string
	var bounds string

	// Parse associated type components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier":
			if typeName == "" {
				typeName = p.nodeText(child)
			}
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "trait_bounds":
			// Associated type bounds (e.g., : Clone)
			bounds = p.nodeText(child)
		}
	}

	// Build the complete name with generics and bounds
	assocType.Name = "type " + typeName
	if genericParams != "" {
		assocType.Name = typeName + genericParams
	}

	if bounds != "" {
		assocType.Type = &ir.TypeRef{Name: bounds}
	} else {
		assocType.Type = &ir.TypeRef{Name: ""}
	}

	parent.Children = append(parent.Children, assocType)
}

// parseImpl parses impl blocks
func (p *ASTParser) parseImpl(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	impl := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: ir.VisibilityPublic,
		Children:   []ir.DistilledNode{},
	}

	var implType string
	var traitName string
	var genericParams string
	var whereClause string

	// Parse impl block
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "type_identifier", "generic_type", "reference_type", "primitive_type":
			if implType == "" {
				implType = p.nodeText(child)
			} else if traitName == "" && i > 0 && node.Child(i-1).Type() == "for" {
				// This is the type after "for"
				implType = p.nodeText(child)
			} else {
				// This might be the trait name
				traitName = implType
				implType = p.nodeText(child)
			}
		case "scoped_type_identifier":
			// Handle trait paths like std::fmt::Display
			if i > 0 && node.Child(i-1).Type() != "for" {
				traitName = p.nodeText(child)
			} else {
				implType = p.nodeText(child)
			}
		case "where_clause":
			whereClause = p.nodeText(child)
		case "declaration_list":
			// Process impl body
			p.processImplBody(child, file, impl)
		}
	}

	// Set impl name with generic parameters
	if traitName != "" {
		if genericParams != "" {
			impl.Name = fmt.Sprintf("impl%s %s for %s", genericParams, traitName, implType)
		} else {
			impl.Name = fmt.Sprintf("impl %s for %s", traitName, implType)
		}
		// Add trait to implements list
		impl.Implements = []ir.TypeRef{{Name: traitName}}
	} else {
		if genericParams != "" {
			impl.Name = fmt.Sprintf("impl%s %s", genericParams, implType)
		} else {
			impl.Name = fmt.Sprintf("impl %s", implType)
		}
	}

	// Add where clause if present
	if whereClause != "" {
		impl.Name += " " + whereClause
	}

	p.addChild(file, parent, impl)
}

// processImplBody processes the contents of an impl block
func (p *ASTParser) processImplBody(node *sitter.Node, file *ir.DistilledFile, parent *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "associated_type":
			// Handle associated type implementations
			p.parseImplAssociatedType(child, parent)
		default:
			p.processNode(child, file, parent)
		}
	}
}

// parseImplAssociatedType parses associated type implementations in impl blocks
func (p *ASTParser) parseImplAssociatedType(node *sitter.Node, parent *ir.DistilledClass) {
	assocType := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: ir.VisibilityPublic,
	}

	var typeName string
	var typeValue string
	var genericParams string
	var foundEquals bool

	// Parse associated type implementation
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier":
			if !foundEquals {
				typeName = p.nodeText(child)
			} else {
				typeValue = p.nodeText(child)
			}
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "=":
			foundEquals = true
		default:
			if foundEquals && typeValue == "" {
				typeValue = p.nodeText(child)
			}
		}
	}

	// Build the complete name
	assocType.Name = "type " + typeName
	if genericParams != "" {
		assocType.Name += genericParams
	}

	if typeValue != "" {
		assocType.Type = &ir.TypeRef{Name: typeValue}
	}

	parent.Children = append(parent.Children, assocType)
}

// parseFunction parses function declarations
func (p *ASTParser) parseFunction(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  p.parseFunctionModifiers(node),
		Parameters: []ir.Parameter{},
	}

	var genericParams string
	var whereClause string
	var returnType string
	var returnStart int = -1

	// Parse function components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			if fn.Name == "" {
				fn.Name = p.nodeText(child)
			}
		case "type_parameters":
			genericParams = p.nodeText(child)
		case "parameters":
			fn.Parameters = p.parseParameters(child)
		case "->":
			returnStart = i
		case "where_clause":
			whereClause = p.nodeText(child)
		case "block":
			// Function body
			fn.Implementation = p.nodeText(child)
		default:
			// Capture return type after ->
			if returnStart >= 0 && i > returnStart && child.Type() != "where_clause" && child.Type() != "block" {
				if returnType == "" {
					returnType = p.nodeText(child)
				}
			}
		}
	}

	// Include generic parameters in function name
	if genericParams != "" {
		fn.Name += genericParams
	}

	// Build complete return type with where clause
	if returnType != "" {
		if whereClause != "" {
			returnType += " " + whereClause
		}
		fn.Returns = &ir.TypeRef{Name: returnType}
	}

	p.addChild(file, parent, fn)
}

// parseFunctionModifiers extracts function modifiers
func (p *ASTParser) parseFunctionModifiers(node *sitter.Node) []ir.Modifier {
	var modifiers []ir.Modifier

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "async":
			modifiers = append(modifiers, ir.ModifierAsync)
		case "const":
			modifiers = append(modifiers, ir.ModifierFinal)
		case "unsafe":
			// TODO: Add unsafe modifier if needed
		}
	}

	return modifiers
}

// parseParameters parses function parameters
func (p *ASTParser) parseParameters(node *sitter.Node) []ir.Parameter {
	var params []ir.Parameter

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "parameter", "self_parameter", "variadic_parameter":
			param := p.parseParameter(child)
			if param.Name != "" {
				params = append(params, param)
			}
		}
	}

	return params
}

// parseParameter parses a single parameter
func (p *ASTParser) parseParameter(node *sitter.Node) ir.Parameter {
	param := ir.Parameter{}

	if node.Type() == "self_parameter" {
		// Handle self, &self, &mut self
		param.Name = p.nodeText(node)
		return param
	}

	// Parse regular parameters
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			param.Name = p.nodeText(child)
		case "mutable_specifier":
			// Prepend "mut" to parameter name
			if param.Name != "" {
				param.Name = "mut " + param.Name
			}
		case "type_identifier", "primitive_type", "reference_type", "pointer_type", "generic_type":
			param.Type = ir.TypeRef{Name: p.nodeText(child)}
		}
	}

	return param
}

// parseConst parses const declarations
func (p *ASTParser) parseConst(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierFinal},
	}

	// Parse const components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			field.Name = p.nodeText(child)
		case "type_identifier", "primitive_type", "reference_type":
			field.Type = &ir.TypeRef{Name: p.nodeText(child)}
		case "integer_literal", "string_literal", "boolean_literal", "float_literal":
			field.DefaultValue = p.nodeText(child)
		}
	}

	p.addChild(file, parent, field)
}

// parseStatic parses static declarations
func (p *ASTParser) parseStatic(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierStatic},
	}

	// Check for mutable static
	hasMut := false
	for i := 0; i < int(node.ChildCount()); i++ {
		if node.Child(i).Type() == "mutable_specifier" {
			hasMut = true
			break
		}
	}

	// Parse static components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			field.Name = p.nodeText(child)
		case "type_identifier", "primitive_type", "reference_type":
			field.Type = &ir.TypeRef{Name: p.nodeText(child)}
		case "integer_literal", "string_literal", "boolean_literal", "float_literal":
			field.DefaultValue = p.nodeText(child)
		}
	}

	// Add mutable information to name if needed
	if hasMut {
		// Could add a mutable modifier if needed
	}

	p.addChild(file, parent, field)
}

// parseTypeAlias parses type alias declarations
func (p *ASTParser) parseTypeAlias(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Visibility: p.parseVisibility(node),
	}

	// Parse type alias components
	foundName := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier":
			if !foundName {
				field.Name = p.nodeText(child)
				foundName = true
			} else {
				// This is the aliased type
				field.Type = &ir.TypeRef{Name: p.nodeText(child)}
			}
		case "primitive_type", "reference_type", "generic_type":
			field.Type = &ir.TypeRef{Name: p.nodeText(child)}
		}
	}

	p.addChild(file, parent, field)
}

// parseLineComment parses line comments
func (p *ASTParser) parseLineComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	text := p.nodeText(node)
	text = strings.TrimPrefix(text, "//")

	format := ir.CommentLine
	inner := strings.HasPrefix(text, "!") // //! documents the enclosing item
	if strings.HasPrefix(text, "/") || inner {
		format = ir.CommentDoc
		text = strings.TrimPrefix(text, "/")
		text = strings.TrimPrefix(text, "!")
	}

	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Text:   strings.TrimSpace(text),
		Format: format,
		Inner:  inner,
	}

	p.addChild(file, parent, comment)
}

// parseBlockComment parses block comments
func (p *ASTParser) parseBlockComment(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	text := p.nodeText(node)
	text = strings.TrimPrefix(text, "/*")
	text = strings.TrimSuffix(text, "*/")

	format := ir.CommentBlock
	inner := strings.HasPrefix(text, "!") // /*! documents the enclosing item
	if strings.HasPrefix(text, "*") || inner {
		format = ir.CommentDoc
		text = strings.TrimPrefix(text, "*")
		text = strings.TrimPrefix(text, "!")
	}

	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: int(node.StartPoint().Row) + 1,
				EndLine:   int(node.EndPoint().Row) + 1,
			},
		},
		Text:   strings.TrimSpace(text),
		Format: format,
		Inner:  inner,
	}

	p.addChild(file, parent, comment)
}

// parseVisibility extracts visibility from node
func (p *ASTParser) parseVisibility(node *sitter.Node) ir.Visibility {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "visibility_modifier" {
			visText := p.nodeText(child)
			if strings.HasPrefix(visText, "pub(crate)") {
				return ir.VisibilityInternal
			}
			if strings.HasPrefix(visText, "pub(super)") || strings.HasPrefix(visText, "pub(in") {
				return ir.VisibilityInternal
			}
			if strings.HasPrefix(visText, "pub") {
				return ir.VisibilityPublic
			}
		}
	}
	// Default visibility in Rust is private
	return ir.VisibilityPrivate
}

// nodeText safely extracts text from a node
func (p *ASTParser) nodeText(node *sitter.Node) string {
	if node == nil {
		return ""
	}
	start := node.StartByte()
	end := node.EndByte()
	if start > end || end > uint32(len(p.source)) {
		return ""
	}
	return string(p.source[start:end])
}

// addChild adds a child node to the appropriate parent
func (p *ASTParser) addChild(file *ir.DistilledFile, parent ir.DistilledNode, child ir.DistilledNode) {
	if parent != nil {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, child)
		case *ir.DistilledFile:
			p.Children = append(p.Children, child)
		}
	} else {
		file.Children = append(file.Children, child)
	}
}
