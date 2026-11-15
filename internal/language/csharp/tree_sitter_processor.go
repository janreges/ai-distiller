package csharp

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	sitter "github.com/smacker/go-tree-sitter"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
)

// TreeSitterProcessor uses tree-sitter for C# parsing
type TreeSitterProcessor struct {
	parser *sitter.Parser
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(tree_sitter_c_sharp.Language()))

	return &TreeSitterProcessor{
		parser: parser,
	}
}

// ProcessSource processes C# source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// Parse the source code
	tree, err := p.parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse C# code: %w", err)
	}
	defer tree.Close()

	// Create distilled file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPoint().Row) + 1,
			},
		},
		Path:     filename,
		Language: "csharp",
		Version:  "1.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Process the tree
	p.processNode(tree.RootNode(), source, file, nil)

	return file, nil
}

// processNode recursively processes tree-sitter nodes
func (p *TreeSitterProcessor) processNode(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	nodeType := node.Type()

	switch nodeType {
	case "compilation_unit":
		p.processStatements(node, source, file, parent)
	case "namespace_declaration", "file_scoped_namespace_declaration":
		p.processNamespaceDeclaration(node, source, file, parent)
	case "using_directive", "global_using_directive":
		p.processUsingDirective(node, source, file)
	case "class_declaration":
		p.processClassDeclaration(node, source, file, parent)
	case "interface_declaration":
		p.processInterfaceDeclaration(node, source, file, parent)
	case "enum_declaration":
		p.processEnumDeclaration(node, source, file, parent)
	case "struct_declaration":
		p.processStructDeclaration(node, source, file, parent)
	case "record_declaration", "record_struct_declaration":
		p.processRecordDeclaration(node, source, file, parent)
	case "delegate_declaration":
		p.processDelegateDeclaration(node, source, file, parent)
	case "method_declaration":
		p.processMethodDeclaration(node, source, file, parent)
	case "operator_declaration":
		p.processOperatorDeclaration(node, source, file, parent)
	case "constructor_declaration":
		p.processConstructorDeclaration(node, source, file, parent)
	case "property_declaration":
		p.processPropertyDeclaration(node, source, file, parent)
	case "field_declaration":
		p.processFieldDeclaration(node, source, file, parent)
	case "event_declaration", "event_field_declaration":
		p.processEventDeclaration(node, source, file, parent)
	case "nullable_directive", "pragma_directive", "region_directive", "endregion_directive", "define_directive", "undef_directive", "if_directive", "elif_directive", "else_directive", "endif_directive", "line_directive", "error_directive", "warning_directive", "preprocessor_call":
		// Handle preprocessor directives as raw content
		rawContent := &ir.DistilledRawContent{
			BaseNode: ir.BaseNode{
				Location: p.nodeLocation(node),
			},
			Content: string(source[node.StartByte():node.EndByte()]) + "\n",
		}
		p.addToParent(file, parent, rawContent)
	default:
		// Check if this could be a preprocessor directive
		if strings.HasPrefix(node.Type(), "preproc_") || node.Type() == "ERROR" {
			// Sometimes directives are parsed as ERROR nodes
			nodeText := string(source[node.StartByte():node.EndByte()])
			if strings.HasPrefix(nodeText, "#") {
				rawContent := &ir.DistilledRawContent{
					BaseNode: ir.BaseNode{
						Location: p.nodeLocation(node),
					},
					Content: nodeText + "\n",
				}
				// A directive in a type body stays in that body. Added to the
				// file, it would land between the type's doc comment and the
				// type (which is added after its body), and the doc would no
				// longer document the type.
				p.addToParent(file, parent, rawContent)
				return
			}
		}

		// Recursively process children
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, source, file, parent)
		}
	}
}

// processNamespaceDeclaration handles namespace declarations
func (p *TreeSitterProcessor) processNamespaceDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	ns := &ir.DistilledPackage{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract namespace name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier", "qualified_name":
			ns.Name = string(source[child.StartByte():child.EndByte()])
		case "declaration_list":
			// Process namespace body
			p.processNamespaceBody(child, source, file, ns)
		}
	}

	// For file-scoped namespaces, continue processing at the same level
	if node.Type() == "file_scoped_namespace_declaration" {
		// Add namespace to file
		file.Children = append(file.Children, ns)
		// Continue processing the rest of the file in this namespace context
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "namespace" && child.Type() != "identifier" && child.Type() != "qualified_name" && child.Type() != ";" {
				p.processNode(child, source, file, ns)
			}
		}
	} else {
		p.addToParent(file, parent, ns)
	}
}

// processUsingDirective handles using directives
func (p *TreeSitterProcessor) processUsingDirective(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		ImportType: "using",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Check if it's a global using
	if node.Type() == "global_using_directive" {
		imp.ImportType = "global using"
	}

	// Extract the namespace or type being imported
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier", "qualified_name":
			imp.Module = string(source[child.StartByte():child.EndByte()])
		case "name_equals":
			// Handle using alias (e.g., using Console = System.Console)
			for j := 0; j < int(child.ChildCount()); j++ {
				aliasChild := child.Child(j)
				if aliasChild.Type() == "identifier" {
					alias := string(source[aliasChild.StartByte():aliasChild.EndByte()])
					imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Alias: alias})
					break
				}
			}
		}
	}

	// If no alias, use the last part of the module name
	if len(imp.Symbols) == 0 && imp.Module != "" {
		parts := strings.Split(imp.Module, ".")
		imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: parts[len(parts)-1]})
	}

	file.Children = append(file.Children, imp)
}

// processClassDeclaration handles class declarations
func (p *TreeSitterProcessor) processClassDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, attributes, name, type parameters, base types
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractClassModifiers(child, source, class)
		case "attribute_list":
			p.extractAttributes(child, source, &class.Decorators)
		case "identifier":
			class.Name = string(source[child.StartByte():child.EndByte()])
		case "type_parameter_list":
			p.extractTypeParameters(child, source, class)
		case "type_parameter_constraints_clauses": // Handle the plural wrapper node
			for j := 0; j < int(child.ChildCount()); j++ {
				constraintClause := child.Child(j)
				if constraintClause.Type() == "type_parameter_constraints_clause" {
					p.extractTypeParameterConstraints(constraintClause, source, class.TypeParams)
				}
			}
		case "type_parameter_constraints_clause":
			// Extract where constraints
			p.extractTypeParameterConstraints(child, source, class.TypeParams)
		case "base_list":
			p.extractBaseList(child, source, class)
		case "declaration_list":
			p.processClassBody(child, source, file, class)
		}
	}

	// Set default visibility if not specified
	if class.Visibility == "" {
		class.Visibility = ir.VisibilityInternal // C# default for types
	}

	p.addToParent(file, parent, class)
}

// processInterfaceDeclaration handles interface declarations
func (p *TreeSitterProcessor) processInterfaceDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	iface := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, attributes, name, type parameters, base interfaces
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractInterfaceModifiers(child, source, iface)
		case "attribute_list":
			// TODO: Add decorator support to interfaces in IR
		case "identifier":
			iface.Name = string(source[child.StartByte():child.EndByte()])
		case "type_parameter_list":
			p.extractInterfaceTypeParameters(child, source, iface)
		case "type_parameter_constraints_clauses": // Handle the plural wrapper node
			for j := 0; j < int(child.ChildCount()); j++ {
				constraintClause := child.Child(j)
				if constraintClause.Type() == "type_parameter_constraints_clause" {
					p.extractTypeParameterConstraints(constraintClause, source, iface.TypeParams)
				}
			}
		case "type_parameter_constraints_clause":
			// Extract where constraints
			p.extractTypeParameterConstraints(child, source, iface.TypeParams)
		case "base_list":
			p.extractInterfaceBaseList(child, source, iface)
		case "declaration_list":
			p.processInterfaceBody(child, source, file, iface)
		}
	}

	// Set default visibility if not specified
	if iface.Visibility == "" {
		iface.Visibility = ir.VisibilityInternal // C# default for types
	}

	p.addToParent(file, parent, iface)
}

// processStructDeclaration handles struct declarations
func (p *TreeSitterProcessor) processStructDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	strct := &ir.DistilledStruct{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, attributes, name, type parameters, interfaces
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractStructModifiers(child, source, strct)
		case "attribute_list":
			// TODO: Add decorator support to structs in IR
		case "identifier":
			strct.Name = string(source[child.StartByte():child.EndByte()])
		case "type_parameter_list":
			p.extractStructTypeParameters(child, source, strct)
		case "type_parameter_constraints_clauses": // Handle the plural wrapper node
			for j := 0; j < int(child.ChildCount()); j++ {
				constraintClause := child.Child(j)
				if constraintClause.Type() == "type_parameter_constraints_clause" {
					p.extractTypeParameterConstraints(constraintClause, source, strct.TypeParams)
				}
			}
		case "type_parameter_constraints_clause":
			// Extract where constraints
			p.extractTypeParameterConstraints(child, source, strct.TypeParams)
		case "base_list":
			// Structs can only implement interfaces
			p.extractStructInterfaces(child, source, strct)
		case "declaration_list":
			p.processStructBody(child, source, file, strct)
		}
	}

	// Set default visibility if not specified
	if strct.Visibility == "" {
		strct.Visibility = ir.VisibilityInternal // C# default for types
	}

	p.addToParent(file, parent, strct)
}

// processEnumDeclaration handles enum declarations
func (p *TreeSitterProcessor) processEnumDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	enum := &ir.DistilledEnum{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, attributes, name, base type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractEnumModifiers(child, source, enum)
		case "attribute_list":
			// TODO: Add decorator support to enums in IR
		case "identifier":
			enum.Name = string(source[child.StartByte():child.EndByte()])
		case "enum_base_clause":
			// Extract underlying type (e.g., : byte)
			for j := 0; j < int(child.ChildCount()); j++ {
				baseChild := child.Child(j)
				if baseChild.Type() == "predefined_type" || baseChild.Type() == "identifier" {
					enum.Type = &ir.TypeRef{Name: string(source[baseChild.StartByte():baseChild.EndByte()])}
					break
				}
			}
		case "enum_member_declaration_list":
			p.processEnumBody(child, source, file, enum)
		}
	}

	// Set default visibility if not specified
	if enum.Visibility == "" {
		enum.Visibility = ir.VisibilityInternal // C# default for types
	}

	p.addToParent(file, parent, enum)
}

// processRecordDeclaration handles record declarations (C# 9+)
func (p *TreeSitterProcessor) processRecordDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Records are classes or structs with special compiler-generated members
	// We represent both as classes with different modifiers
	isStruct := node.Type() == "record_struct_declaration"

	record := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Modifiers: []ir.Modifier{ir.ModifierData}, // Mark as record
		Children:  []ir.DistilledNode{},
	}

	// If it's a record struct, add struct modifier
	if isStruct {
		record.Modifiers = append(record.Modifiers, ir.ModifierStruct)   // Mark as struct
		record.Modifiers = append(record.Modifiers, ir.ModifierReadonly) // Record structs are readonly by default
	}

	// Extract modifiers, attributes, name, parameters, base types
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractClassModifiers(child, source, record)
		case "attribute_list":
			// Extract attributes
			p.extractAttributes(child, source, &record.Decorators)
		case "identifier":
			record.Name = string(source[child.StartByte():child.EndByte()])
		case "parameter_list":
			// Record parameters become properties
			p.extractRecordParameters(child, source, record)
		case "base_list":
			p.extractBaseList(child, source, record)
		case "declaration_list":
			p.processClassBody(child, source, file, record)
		}
	}

	// Set default visibility if not specified
	if record.Visibility == "" {
		record.Visibility = ir.VisibilityInternal
	}

	p.addToParent(file, parent, record)
}

// processDelegateDeclaration handles delegate declarations
func (p *TreeSitterProcessor) processDelegateDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Delegates are type-safe function pointers
	// We'll represent them as a special type alias
	delegate := &ir.DistilledTypeAlias{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
	}

	// Extract modifiers, return type, name, parameters
	var returnType string
	var parameters []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			if vis, ok := declaredVisibility(child, source); ok {
				delegate.Visibility = vis
			}
		case "void_keyword":
			returnType = "void"
		case "predefined_type", "identifier", "generic_name", "qualified_name":
			if returnType == "" && delegate.Name == "" {
				returnType = string(source[child.StartByte():child.EndByte()])
			} else if delegate.Name == "" {
				delegate.Name = string(source[child.StartByte():child.EndByte()])
			}
		case "parameter_list":
			// Extract parameter types
			p.extractDelegateParameters(child, source, &parameters)
		}
	}

	// Build delegate type representation
	paramStr := strings.Join(parameters, ", ")
	delegate.Type = ir.TypeRef{
		Name: fmt.Sprintf("delegate %s(%s)", returnType, paramStr),
	}

	// Set default visibility if not specified
	if delegate.Visibility == "" {
		delegate.Visibility = ir.VisibilityInternal
	}

	p.addToParent(file, parent, delegate)
}

// processMethodDeclaration handles method declarations
func (p *TreeSitterProcessor) processMethodDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract modifiers, attributes, return type, name, parameters, body
	// We need to be careful about the order - return type comes before method name
	hasSeenType := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractMethodModifiers(child, source, method)
		case "attribute_list":
			p.extractAttributes(child, source, &method.Decorators)
		case "void_keyword":
			method.Returns = &ir.TypeRef{Name: "void"}
			hasSeenType = true
		case "predefined_type", "array_type", "generic_name", "qualified_name", "tuple_type", "nullable_type":
			if method.Returns == nil {
				method.Returns = p.extractType(child, source)
				hasSeenType = true
			}
		case "identifier":
			// In C#, return type comes before method name
			// So first identifier is return type (unless we already have one)
			// Second identifier is method name
			if !hasSeenType && method.Returns == nil {
				// This is the return type
				method.Returns = p.extractType(child, source)
				hasSeenType = true
			} else if method.Name == "" {
				// This is the method name
				method.Name = string(source[child.StartByte():child.EndByte()])
			}
		case "type_parameter_list":
			// Handle generic methods
			p.extractMethodTypeParameters(child, source, method)
		case "type_parameter_constraints_clauses": // Handle the plural wrapper node
			for j := 0; j < int(child.ChildCount()); j++ {
				constraintClause := child.Child(j)
				if constraintClause.Type() == "type_parameter_constraints_clause" {
					p.extractTypeParameterConstraints(constraintClause, source, method.TypeParams)
				}
			}
		case "type_parameter_constraints_clause":
			// Extract where constraints
			p.extractTypeParameterConstraints(child, source, method.TypeParams)
		case "parameter_list":
			p.extractParameters(child, source, method)
		case "block", "arrow_expression_clause":
			method.Implementation = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Check if this is an extension method
	if len(method.Parameters) > 0 && method.Modifiers != nil {
		for _, mod := range method.Modifiers {
			if mod == ir.ModifierStatic {
				// Check first parameter for "this" modifier
				// TODO: Properly detect extension methods
				break
			}
		}
	}

	// Set default visibility if not specified
	if method.Visibility == "" {
		if parent != nil {
			switch parent.(type) {
			case *ir.DistilledInterface:
				method.Visibility = ir.VisibilityPublic // Interface members are public by default
			default:
				method.Visibility = ir.VisibilityPrivate // Class/struct members are private by default
			}
		} else {
			method.Visibility = ir.VisibilityPrivate
		}
	}

	p.addToParent(file, parent, method)
}

// processConstructorDeclaration handles constructor declarations
func (p *TreeSitterProcessor) processConstructorDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	constructor := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract modifiers, attributes, name, parameters, initializer, body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractMethodModifiers(child, source, constructor)
		case "attribute_list":
			p.extractAttributes(child, source, &constructor.Decorators)
		case "identifier":
			constructor.Name = string(source[child.StartByte():child.EndByte()])
		case "parameter_list":
			p.extractParameters(child, source, constructor)
		case "constructor_initializer":
			// TODO: Handle base() and this() calls
		case "block", "arrow_expression_clause":
			constructor.Implementation = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Set default visibility if not specified
	if constructor.Visibility == "" {
		constructor.Visibility = ir.VisibilityPrivate // Default for constructors
	}

	p.addToParent(file, parent, constructor)
}

// processPropertyDeclaration handles property declarations
func (p *TreeSitterProcessor) processPropertyDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Properties are represented as fields with accessor information
	property := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Modifiers:  []ir.Modifier{},
		IsProperty: true, // Mark this as a property, not a field
	}

	// Track accessor information
	hasGetter := false
	hasSetter := false
	hasInit := false

	// Extract modifiers, attributes, type, name, accessors
	hasSeenType := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractFieldModifiers(child, source, property)
		case "attribute_list":
			p.extractAttributes(child, source, &property.Decorators)
		case "predefined_type", "array_type", "generic_name", "qualified_name", "nullable_type":
			if property.Type == nil {
				property.Type = p.extractType(child, source)
				hasSeenType = true
			}
		case "identifier":
			// Property type comes before property name
			if !hasSeenType && property.Type == nil {
				// This is the property type
				property.Type = p.extractType(child, source)
				hasSeenType = true
			} else if property.Name == "" {
				// This is the property name
				property.Name = string(source[child.StartByte():child.EndByte()])
			}
		case "accessor_list":
			// Parse get/set/init accessors
			for j := 0; j < int(child.ChildCount()); j++ {
				accessor := child.Child(j)
				if accessor.Type() == "accessor_declaration" {
					for k := 0; k < int(accessor.ChildCount()); k++ {
						accChild := accessor.Child(k)
						text := string(source[accChild.StartByte():accChild.EndByte()])
						switch text {
						case "get":
							hasGetter = true
						case "set":
							hasSetter = true
						case "init":
							hasInit = true
						}
					}
				}
			}
		case "arrow_expression_clause":
			// Expression-bodied property (implies getter only)
			hasGetter = true
			property.DefaultValue = string(source[child.StartByte():child.EndByte()])
		case "equals_value_clause":
			// Property initializer
			for j := 0; j < int(child.ChildCount()); j++ {
				valueChild := child.Child(j)
				if valueChild.Type() != "=" {
					property.DefaultValue = string(source[valueChild.StartByte():valueChild.EndByte()])
					break
				}
			}
		}
	}

	// Set property accessor information
	property.HasGetter = hasGetter
	property.HasSetter = hasSetter || hasInit

	// Add readonly modifier for init-only or getter-only properties
	if hasGetter && !hasSetter && !hasInit {
		property.Modifiers = append(property.Modifiers, ir.ModifierReadonly)
	}

	// Set default visibility if not specified
	if property.Visibility == "" {
		property.Visibility = ir.VisibilityPrivate // Default for properties
	}

	p.addToParent(file, parent, property)
}

// processFieldDeclaration handles field declarations
func (p *TreeSitterProcessor) processFieldDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// A field declaration can declare multiple fields
	var fieldType *ir.TypeRef
	var modifiers []ir.Modifier
	var visibility ir.Visibility
	var decorators []string

	// Extract modifiers and type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			mod, _ := p.extractFieldModifier(child, source)
			if mod != "" {
				modifiers = append(modifiers, mod)
			}
			if vis, ok := declaredVisibility(child, source); ok {
				visibility = vis
			}
		case "attribute_list":
			p.extractAttributes(child, source, &decorators)
		case "predefined_type", "array_type", "generic_name", "qualified_name", "identifier":
			if fieldType == nil {
				fieldType = p.extractType(child, source)
			}
		case "variable_declaration":
			// Extract type from variable_declaration if we haven't found it yet
			if fieldType == nil {
				for j := 0; j < int(child.ChildCount()); j++ {
					varChild := child.Child(j)
					switch varChild.Type() {
					case "predefined_type", "array_type", "generic_name", "qualified_name", "identifier":
						fieldType = p.extractType(varChild, source)
						break
					}
				}
			}

			// Process variable declarators
			for j := 0; j < int(child.ChildCount()); j++ {
				varChild := child.Child(j)
				if varChild.Type() == "variable_declarator" {
					field := &ir.DistilledField{
						BaseNode: ir.BaseNode{
							Location: p.nodeLocation(varChild),
						},
						Type:       fieldType,
						Modifiers:  modifiers,
						Visibility: visibility,
						Decorators: decorators,
					}

					// Extract field name and value
					hasSeenEquals := false
					for k := 0; k < int(varChild.ChildCount()); k++ {
						declChild := varChild.Child(k)
						switch declChild.Type() {
						case "identifier":
							if !hasSeenEquals {
								field.Name = string(source[declChild.StartByte():declChild.EndByte()])
							}
						case "=":
							hasSeenEquals = true
						case "equals_value_clause":
							// Extract initializer
							for l := 0; l < int(declChild.ChildCount()); l++ {
								valueChild := declChild.Child(l)
								if valueChild.Type() != "=" {
									field.DefaultValue = string(source[valueChild.StartByte():valueChild.EndByte()])
									break
								}
							}
						default:
							// If we've seen equals, this is the value
							if hasSeenEquals && field.DefaultValue == "" {
								field.DefaultValue = string(source[declChild.StartByte():declChild.EndByte()])
							}
						}
					}

					// Set default visibility if not specified
					if field.Visibility == "" {
						field.Visibility = ir.VisibilityPrivate
					}

					p.addToParent(file, parent, field)
				}
			}
		}
	}
}

// processEventDeclaration handles event declarations
func (p *TreeSitterProcessor) processEventDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Events are special fields with restricted access
	// We'll represent them as fields with a special modifier
	event := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Modifiers: []ir.Modifier{}, // TODO: Add event modifier to IR
	}

	// Extract modifiers, attributes, type, name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractFieldModifiers(child, source, event)
		case "attribute_list":
			p.extractAttributes(child, source, &event.Decorators)
		case "predefined_type", "generic_name", "qualified_name", "identifier":
			if event.Type == nil && event.Name == "" {
				event.Type = p.extractType(child, source)
			}
		case "variable_declaration":
			// Extract event name(s)
			for j := 0; j < int(child.ChildCount()); j++ {
				varChild := child.Child(j)
				if varChild.Type() == "variable_declarator" {
					for k := 0; k < int(varChild.ChildCount()); k++ {
						declChild := varChild.Child(k)
						if declChild.Type() == "identifier" {
							event.Name = string(source[declChild.StartByte():declChild.EndByte()])
							break
						}
					}
				}
			}
		case "accessor_list":
			// Custom event accessors (add/remove)
			// TODO: Handle custom accessors
		}
	}

	// Set default visibility if not specified
	if event.Visibility == "" {
		event.Visibility = ir.VisibilityPrivate
	}

	p.addToParent(file, parent, event)
}

// Helper methods

// visibilityFromModifiers derives the visibility from all modifier tokens of a
// declaration. It reports false when no access modifier is present.
func visibilityFromModifiers(tokens []string) (ir.Visibility, bool) {
	has := map[string]bool{}
	for _, t := range tokens {
		has[t] = true
	}
	switch {
	case has["protected"] && has["internal"]:
		return ir.VisibilityProtectedInternal, true
	case has["private"] && has["protected"]:
		return ir.VisibilityPrivateProtected, true
	case has["public"]:
		return ir.VisibilityPublic, true
	case has["private"]:
		return ir.VisibilityPrivate, true
	case has["protected"]:
		return ir.VisibilityProtected, true
	case has["internal"]:
		return ir.VisibilityInternal, true
	}
	return "", false
}

// declaredVisibility collects the modifier tokens of the declaration that owns
// the given modifier node and derives its visibility.
func declaredVisibility(modifier *sitter.Node, source []byte) (ir.Visibility, bool) {
	decl := modifier.Parent()
	if decl == nil {
		return "", false
	}
	var tokens []string
	for i := 0; i < int(decl.ChildCount()); i++ {
		if c := decl.Child(i); c.Type() == "modifier" {
			tokens = append(tokens, string(source[c.StartByte():c.EndByte()]))
		}
	}
	return visibilityFromModifiers(tokens)
}

// extractClassModifiers extracts modifiers for classes
func (p *TreeSitterProcessor) extractClassModifiers(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	if vis, ok := declaredVisibility(node, source); ok {
		class.Visibility = vis
	}
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	case "abstract":
		class.Modifiers = append(class.Modifiers, ir.ModifierAbstract)
	case "sealed":
		class.Modifiers = append(class.Modifiers, ir.ModifierSealed)
	case "static":
		class.Modifiers = append(class.Modifiers, ir.ModifierStatic)
	case "partial":
		class.Modifiers = append(class.Modifiers, ir.ModifierPartial)
	}
}

// extractInterfaceModifiers extracts modifiers for interfaces
func (p *TreeSitterProcessor) extractInterfaceModifiers(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	if vis, ok := declaredVisibility(node, source); ok {
		iface.Visibility = vis
	}
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	}
}

// extractStructModifiers extracts modifiers for structs
func (p *TreeSitterProcessor) extractStructModifiers(node *sitter.Node, source []byte, strct *ir.DistilledStruct) {
	if vis, ok := declaredVisibility(node, source); ok {
		strct.Visibility = vis
	}
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	case "readonly":
		// TODO: Add readonly struct support
	case "ref":
		// TODO: Add ref struct support
	}
}

// extractEnumModifiers extracts modifiers for enums
func (p *TreeSitterProcessor) extractEnumModifiers(node *sitter.Node, source []byte, enum *ir.DistilledEnum) {
	if vis, ok := declaredVisibility(node, source); ok {
		enum.Visibility = vis
	}
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	}
}

// extractMethodModifiers extracts modifiers for methods
func (p *TreeSitterProcessor) extractMethodModifiers(node *sitter.Node, source []byte, method *ir.DistilledFunction) {
	if vis, ok := declaredVisibility(node, source); ok {
		method.Visibility = vis
	}
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	case "abstract":
		method.Modifiers = append(method.Modifiers, ir.ModifierAbstract)
	case "virtual":
		method.Modifiers = append(method.Modifiers, ir.ModifierVirtual)
	case "override":
		method.Modifiers = append(method.Modifiers, ir.ModifierOverride)
	case "sealed":
		method.Modifiers = append(method.Modifiers, ir.ModifierSealed)
	case "static":
		method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
	case "async":
		method.Modifiers = append(method.Modifiers, ir.ModifierAsync)
	case "extern":
		method.Modifiers = append(method.Modifiers, ir.ModifierExtern)
	case "partial":
		method.Modifiers = append(method.Modifiers, ir.ModifierPartial)
	case "new":
		// TODO: Handle new modifier (hides inherited member)
	}
}

// extractFieldModifiers extracts modifiers for fields
func (p *TreeSitterProcessor) extractFieldModifiers(node *sitter.Node, source []byte, field *ir.DistilledField) {
	mod, _ := p.extractFieldModifier(node, source)
	if mod != "" {
		field.Modifiers = append(field.Modifiers, mod)
	}
	if vis, ok := declaredVisibility(node, source); ok {
		field.Visibility = vis
	}
}

// extractFieldModifier extracts a single field modifier
func (p *TreeSitterProcessor) extractFieldModifier(node *sitter.Node, source []byte) (ir.Modifier, ir.Visibility) {
	text := string(source[node.StartByte():node.EndByte()])

	switch text {
	case "static":
		return ir.ModifierStatic, ""
	case "readonly":
		return ir.ModifierReadonly, ""
	case "const":
		return ir.ModifierConst, ""
	case "volatile":
		return ir.ModifierVolatile, ""
	case "new":
		// TODO: Handle new modifier
		return "", ""
	default:
		return "", ""
	}
}

// extractAttributes extracts attributes (decorators in IR terms)
func (p *TreeSitterProcessor) extractAttributes(node *sitter.Node, source []byte, decorators *[]string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "attribute" {
			// Extract the full attribute text
			attrText := string(source[child.StartByte():child.EndByte()])
			*decorators = append(*decorators, "["+attrText+"]")
		}
	}
}

// extractTypeParameters extracts generic type parameters
func (p *TreeSitterProcessor) extractTypeParameters(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter" {
			typeParam := ir.TypeParam{
				Name:        "",
				Constraints: []ir.TypeRef{},
			}

			// Extract parameter name and constraints
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				if paramChild.Type() == "identifier" {
					typeParam.Name = string(source[paramChild.StartByte():paramChild.EndByte()])
				}
			}

			class.TypeParams = append(class.TypeParams, typeParam)
		}
	}
}

// extractInterfaceTypeParameters extracts generic type parameters for interfaces
func (p *TreeSitterProcessor) extractInterfaceTypeParameters(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter" {
			typeParam := ir.TypeParam{
				Name:        "",
				Constraints: []ir.TypeRef{},
			}

			// Extract parameter name
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				if paramChild.Type() == "identifier" {
					typeParam.Name = string(source[paramChild.StartByte():paramChild.EndByte()])
				}
			}

			iface.TypeParams = append(iface.TypeParams, typeParam)
		}
	}
}

// extractStructTypeParameters extracts generic type parameters for structs
func (p *TreeSitterProcessor) extractStructTypeParameters(node *sitter.Node, source []byte, strct *ir.DistilledStruct) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter" {
			typeParam := ir.TypeParam{
				Name:        "",
				Constraints: []ir.TypeRef{},
			}

			// Extract parameter name
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				if paramChild.Type() == "identifier" {
					typeParam.Name = string(source[paramChild.StartByte():paramChild.EndByte()])
				}
			}

			strct.TypeParams = append(strct.TypeParams, typeParam)
		}
	}
}

// extractMethodTypeParameters extracts generic type parameters for methods
func (p *TreeSitterProcessor) extractMethodTypeParameters(node *sitter.Node, source []byte, method *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter" {
			typeParam := ir.TypeParam{
				Name:        "",
				Constraints: []ir.TypeRef{},
			}

			// Extract parameter name
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				if paramChild.Type() == "identifier" {
					typeParam.Name = string(source[paramChild.StartByte():paramChild.EndByte()])
				}
			}

			method.TypeParams = append(method.TypeParams, typeParam)
		}
	}
}

// extractBaseList extracts base class and interfaces
func (p *TreeSitterProcessor) extractBaseList(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == ":" {
			continue
		}

		// First base type is usually the base class (unless it's an interface)
		baseType := p.extractType(child, source)
		if len(class.Extends) == 0 {
			// Assume first is base class (C# only allows single inheritance)
			class.Extends = append(class.Extends, *baseType)
		} else {
			// Rest are interfaces
			class.Implements = append(class.Implements, *baseType)
		}

		// TODO: Properly distinguish between base class and interfaces
	}
}

// extractInterfaceBaseList extracts base interfaces
func (p *TreeSitterProcessor) extractInterfaceBaseList(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == ":" || child.Type() == "," {
			continue
		}

		baseType := p.extractType(child, source)
		iface.Extends = append(iface.Extends, *baseType)
	}
}

// extractStructInterfaces extracts implemented interfaces
func (p *TreeSitterProcessor) extractStructInterfaces(node *sitter.Node, source []byte, strct *ir.DistilledStruct) {
	// Structs can only implement interfaces, not inherit from base types
	// TODO: Add interface implementation support to structs in IR
}

// extractRecordParameters extracts record parameters as properties
func (p *TreeSitterProcessor) extractRecordParameters(node *sitter.Node, source []byte, record ir.DistilledNode) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			// Create a property for each record parameter
			property := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(child),
				},
				Visibility: ir.VisibilityPublic,                // Record properties are public by default
				Modifiers:  []ir.Modifier{ir.ModifierReadonly}, // Record properties are readonly
				IsProperty: true,                               // Record parameters are properties
				HasGetter:  true,                               // Record properties have getters
				HasSetter:  false,                              // Record properties are init-only
			}

			// Extract parameter attributes, type and name
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				switch paramChild.Type() {
				case "attribute_list":
					// Handle property attributes like [property: StringRange(3, 12)]
					// For record parameters, preserve the full attribute text including target
					attrText := string(source[paramChild.StartByte():paramChild.EndByte()])
					property.Decorators = append(property.Decorators, attrText)
				case "predefined_type", "array_type", "generic_name", "qualified_name":
					if property.Type == nil {
						property.Type = p.extractType(paramChild, source)
					}
				case "identifier":
					if property.Name == "" {
						property.Name = string(source[paramChild.StartByte():paramChild.EndByte()])
					} else if property.Type == nil {
						// This might be the type
						property.Type = p.extractType(paramChild, source)
					}
				}
			}

			// Add property to record
			switch r := record.(type) {
			case *ir.DistilledClass:
				r.Children = append(r.Children, property)
			case *ir.DistilledStruct:
				r.Children = append(r.Children, property)
			}
		}
	}
}

// extractType extracts type information
func (p *TreeSitterProcessor) extractType(node *sitter.Node, source []byte) *ir.TypeRef {
	nodeType := node.Type()

	switch nodeType {
	case "predefined_type", "identifier", "qualified_name":
		return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
	case "generic_name":
		// Handle generic types like List<string>
		typeName := ""
		var generics []ir.TypeRef

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			switch child.Type() {
			case "identifier":
				typeName = string(source[child.StartByte():child.EndByte()])
			case "type_argument_list":
				// Extract generic arguments
				for j := 0; j < int(child.ChildCount()); j++ {
					argChild := child.Child(j)
					if argChild.Type() != "<" && argChild.Type() != ">" && argChild.Type() != "," {
						argType := p.extractType(argChild, source)
						generics = append(generics, *argType)
					}
				}
			}
		}

		return &ir.TypeRef{
			Name:     typeName,
			TypeArgs: generics,
		}
	case "array_type":
		// Handle array types
		var elementType *ir.TypeRef
		var ranks []string

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "array_rank_specifier" {
				// Count dimensions
				ranks = append(ranks, string(source[child.StartByte():child.EndByte()]))
			} else if elementType == nil {
				elementType = p.extractType(child, source)
			}
		}

		if elementType != nil {
			// Append array notation to type name
			arrayType := elementType.Name
			for _, rank := range ranks {
				arrayType += rank
			}
			return &ir.TypeRef{Name: arrayType}
		}
	case "nullable_type":
		// Handle nullable types (e.g., int?, string?)
		// The nullable_type node contains the base type as first child and "?" as second
		if node.ChildCount() > 0 {
			// Get the base type (first child)
			baseType := p.extractType(node.Child(0), source)
			baseType.IsNullable = true
			return baseType
		}
		// Fallback: extract type name without the ?
		typeName := string(source[node.StartByte():node.EndByte()])
		typeName = strings.TrimSuffix(typeName, "?")
		return &ir.TypeRef{Name: typeName, IsNullable: true}
	case "tuple_type":
		// Handle tuple types like (double C, double A)
		// For now, just return the raw text representation
		return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
	}

	// Default: return the raw text
	return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
}

// extractParameters extracts method parameters
func (p *TreeSitterProcessor) extractParameters(node *sitter.Node, source []byte, method *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			param := p.extractParameter(child, source)
			method.Parameters = append(method.Parameters, *param)
		}
	}
}

// extractParameter extracts a single parameter
func (p *TreeSitterProcessor) extractParameter(node *sitter.Node, source []byte) *ir.Parameter {
	param := &ir.Parameter{}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "attribute_list":
			// Handle parameter attributes
		case "modifier":
			// Handle parameter modifiers (ref, out, in, this, params)
			text := string(source[child.StartByte():child.EndByte()])
			switch text {
			case "ref", "out", "in":
				// TODO: Add parameter modifier support to IR
			case "this":
				// This indicates an extension method
			case "params":
				param.IsVariadic = true
			}
		case "predefined_type", "array_type", "generic_name", "qualified_name", "nullable_type", "tuple_type":
			if param.Type.Name == "" {
				param.Type = *p.extractType(child, source)
			}
		case "identifier":
			// In C#, type comes before name, so if we already have a type,
			// this identifier is the parameter name
			if param.Type.Name != "" {
				param.Name = string(source[child.StartByte():child.EndByte()])
			} else {
				// This might be a simple type name (like T in generics)
				// We'll treat it as type for now, but it might get overwritten
				// if we find a proper type node
				tempType := string(source[child.StartByte():child.EndByte()])
				if param.Name == "" {
					// First identifier - could be type or name
					param.Type = ir.TypeRef{Name: tempType}
				} else {
					// We already have something in name, swap them
					param.Type = ir.TypeRef{Name: param.Name}
					param.Name = tempType
				}
			}
		case "equals_value_clause":
			// Extract default value
			for j := 0; j < int(child.ChildCount()); j++ {
				valueChild := child.Child(j)
				if valueChild.Type() != "=" {
					param.DefaultValue = string(source[valueChild.StartByte():valueChild.EndByte()])
					param.IsOptional = true
					break
				}
			}
		}
	}

	return param
}

// extractDelegateParameters extracts delegate parameter types
func (p *TreeSitterProcessor) extractDelegateParameters(node *sitter.Node, source []byte, parameters *[]string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			// Extract parameter type
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				switch paramChild.Type() {
				case "predefined_type", "array_type", "generic_name", "qualified_name", "identifier":
					typeRef := p.extractType(paramChild, source)
					*parameters = append(*parameters, typeRef.Name)
					break
				}
			}
		}
	}
}

// processNamespaceBody processes the body of a namespace
func (p *TreeSitterProcessor) processNamespaceBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processStatements(node, source, file, parent)
}

// processClassBody processes the body of a class
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processStatements(node, source, file, parent)
}

// processInterfaceBody processes the body of an interface
func (p *TreeSitterProcessor) processInterfaceBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processStatements(node, source, file, parent)
}

// processStructBody processes the body of a struct
func (p *TreeSitterProcessor) processStructBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processStatements(node, source, file, parent)
}

// processStatements processes the children of the compilation unit, a
// namespace body or a type body. Comments are added where they are. The doc
// comments right before a statement that adds nothing to this list (an
// assembly attribute, a using directive in a namespace body, which goes to
// the file, a declaration kind the parser does not handle) are dropped, so
// they do not document the next declaration (commentutil.AppendStatement).
func (p *TreeSitterProcessor) processStatements(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	list := p.childList(file, parent)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch {
		case child.Type() == "comment":
			p.processComment(child, source, file, parent)
			continue
		case !child.IsNamed():
			continue
		}
		before := len(*list)
		p.processNode(child, source, file, parent)
		added := append([]ir.DistilledNode(nil), (*list)[before:]...)
		*list = commentutil.AppendStatement((*list)[:before], added)
	}
}

// processComment adds a comment. "///" lines are XML doc comments, and a run
// of them on consecutive lines is one doc whose text lines are joined with
// "\n"; "/** */" is a doc comment too. XML tags stay in the text as written.
func (p *TreeSitterProcessor) processComment(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	text, format := commentutil.StripCStyle(string(source[node.StartByte():node.EndByte()]))
	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   text,
		Format: format,
	}
	if commentutil.JoinLineDoc(*p.childList(file, parent), comment, node, source, commentutil.IsTripleSlashDoc) != nil {
		return
	}
	p.addToParent(file, parent, comment)
}

// processEnumBody processes the body of an enum
func (p *TreeSitterProcessor) processEnumBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "comment":
			p.processComment(child, source, file, parent)
		case "enum_member_declaration":
			p.processEnumMember(child, source, file, parent)
		}
	}
}

// processEnumMember processes enum members
func (p *TreeSitterProcessor) processEnumMember(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierConst},
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "attribute_list":
			p.extractAttributes(child, source, &field.Decorators)
		case "identifier":
			field.Name = string(source[child.StartByte():child.EndByte()])
		case "equals_value_clause":
			// Extract enum value
			for j := 0; j < int(child.ChildCount()); j++ {
				valueChild := child.Child(j)
				if valueChild.Type() != "=" {
					field.DefaultValue = string(source[valueChild.StartByte():valueChild.EndByte()])
					break
				}
			}
		case "=":
			// The grammar puts the value directly after the "=" token
			if next := child.NextNamedSibling(); next != nil && field.DefaultValue == "" {
				field.DefaultValue = string(source[next.StartByte():next.EndByte()])
			}
		}
	}

	// Enum members have the enum type
	if enum, ok := parent.(*ir.DistilledEnum); ok {
		field.Type = &ir.TypeRef{Name: enum.Name}
	}

	p.addToParent(file, parent, field)
}

// nodeLocation converts tree-sitter node position to IR location
func (p *TreeSitterProcessor) nodeLocation(node *sitter.Node) ir.Location {
	return ir.Location{
		StartLine:   int(node.StartPoint().Row) + 1,
		StartColumn: int(node.StartPoint().Column) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		EndColumn:   int(node.EndPoint().Column) + 1,
	}
}

// childList returns the list addToParent adds the children of parent to.
func (p *TreeSitterProcessor) childList(file *ir.DistilledFile, parent ir.DistilledNode) *[]ir.DistilledNode {
	switch n := parent.(type) {
	case *ir.DistilledClass:
		return &n.Children
	case *ir.DistilledInterface:
		return &n.Children
	case *ir.DistilledStruct:
		return &n.Children
	case *ir.DistilledEnum:
		return &n.Children
	case *ir.DistilledPackage:
		return &n.Children
	}
	return &file.Children
}

// addToParent adds a node to its parent or to the file
func (p *TreeSitterProcessor) addToParent(file *ir.DistilledFile, parent ir.DistilledNode, child ir.DistilledNode) {
	if parent != nil {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, child)
		case *ir.DistilledInterface:
			p.Children = append(p.Children, child)
		case *ir.DistilledStruct:
			p.Children = append(p.Children, child)
		case *ir.DistilledEnum:
			p.Children = append(p.Children, child)
		case *ir.DistilledPackage:
			p.Children = append(p.Children, child)
		default:
			file.Children = append(file.Children, child)
		}
	} else {
		file.Children = append(file.Children, child)
	}
}

// extractTypeParameterConstraints extracts where constraints for generic type parameters
func (p *TreeSitterProcessor) extractTypeParameterConstraints(node *sitter.Node, source []byte, typeParams []ir.TypeParam) {
	// Parse: where TUser : class, IUser
	// Structure: type_parameter_constraints_clause
	//   -> identifier (TUser)
	//   -> type_constraint+ (class, IUser)

	var paramName string
	var constraints []ir.TypeRef

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier":
			// This is the type parameter name (e.g., TUser)
			paramName = string(source[child.StartByte():child.EndByte()])
		case "type_parameter_constraint":
			// Extract the constraint type
			constraint := p.extractTypeConstraint(child, source)
			if constraint != nil {
				constraints = append(constraints, *constraint)
			}
		}
	}

	// Find the matching type parameter and update its constraints
	for i := range typeParams {
		if typeParams[i].Name == paramName {
			typeParams[i].Constraints = append(typeParams[i].Constraints, constraints...)
			break
		}
	}
}

// extractTypeConstraint extracts a single type constraint
func (p *TreeSitterProcessor) extractTypeConstraint(node *sitter.Node, source []byte) *ir.TypeRef {
	// Type constraint can be: class, struct, new(), or a type name

	// If the node itself is the constraint text (e.g., "class", "IUser")
	nodeText := string(source[node.StartByte():node.EndByte()])
	if nodeText == "class" || nodeText == "struct" {
		return &ir.TypeRef{Name: nodeText}
	}

	// Otherwise check child nodes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "class_constraint":
			return &ir.TypeRef{Name: "class"}
		case "struct_constraint":
			return &ir.TypeRef{Name: "struct"}
		case "constructor_constraint":
			return &ir.TypeRef{Name: "new()"}
		case "identifier", "generic_name", "qualified_name":
			return p.extractType(child, source)
		}
	}

	// If no specific constraint type found, use the node text directly
	if nodeText != "" {
		return &ir.TypeRef{Name: nodeText}
	}

	return nil
}

// processOperatorDeclaration handles operator declarations
func (p *TreeSitterProcessor) processOperatorDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	operator := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract modifiers, return type, operator symbol, parameters
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifier":
			p.extractMethodModifiers(child, source, operator)
		case "predefined_type", "array_type", "generic_name", "qualified_name", "identifier":
			if operator.Returns == nil {
				operator.Returns = p.extractType(child, source)
			}
		case "operator":
			// Next child should be the operator symbol
			if i+1 < int(node.ChildCount()) {
				nextChild := node.Child(i + 1)
				operatorSymbol := string(source[nextChild.StartByte():nextChild.EndByte()])
				operator.Name = "operator " + operatorSymbol
				i++ // Skip the operator symbol
			}
		case "parameter_list":
			p.extractParameters(child, source, operator)
		case "block", "arrow_expression_clause":
			operator.Implementation = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Operators are always public static
	operator.Visibility = ir.VisibilityPublic
	if !p.hasModifier(operator.Modifiers, ir.ModifierStatic) {
		operator.Modifiers = append(operator.Modifiers, ir.ModifierStatic)
	}

	p.addToParent(file, parent, operator)
}

// hasModifier checks if a modifier list contains a specific modifier
func (p *TreeSitterProcessor) hasModifier(modifiers []ir.Modifier, modifier ir.Modifier) bool {
	for _, m := range modifiers {
		if m == modifier {
			return true
		}
	}
	return false
}
