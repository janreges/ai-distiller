package kotlin

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"
)

// TreeSitterProcessor uses tree-sitter for Kotlin parsing
type TreeSitterProcessor struct {
	parser *sitter.Parser
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	parser := sitter.NewParser()
	parser.SetLanguage(kotlin.GetLanguage())

	return &TreeSitterProcessor{
		parser: parser,
	}
}

// ProcessSource processes Kotlin source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// Parse the source code
	tree, err := p.parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Kotlin code: %w", err)
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
		Language: "kotlin",
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
	case "source_file":
		// Root node - process all children
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, source, file, parent)
		}
	case "package_header":
		p.processPackageHeader(node, source, file)
	case "import_list", "import_header":
		p.processImport(node, source, file)
	case "class_declaration":
		p.processClassDeclaration(node, source, file, parent)
	case "interface_declaration":
		p.processInterfaceDeclaration(node, source, file, parent)
	case "enum_class":
		p.processEnumDeclaration(node, source, file, parent)
	case "object_declaration":
		p.processObjectDeclaration(node, source, file, parent)
	case "companion_object":
		p.processCompanionObject(node, source, file, parent)
	case "function_declaration":
		p.processFunctionDeclaration(node, source, file, parent)
	case "property_declaration":
		p.processPropertyDeclaration(node, source, file, parent)
	case "secondary_constructor":
		p.processSecondaryConstructor(node, source, file, parent)
	case "type_alias":
		p.processTypeAlias(node, source, file, parent)
	case "primary_constructor":
		// Primary constructors are handled as part of class declaration
		// since they're integral to the class definition
	default:
		// Recursively process children
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, source, file, parent)
		}
	}
}

// processPackageHeader handles package declarations
func (p *TreeSitterProcessor) processPackageHeader(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	pkg := &ir.DistilledPackage{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "package_identifier" {
			pkg.Name = string(source[child.StartByte():child.EndByte()])
			break
		}
	}

	if pkg.Name != "" {
		file.Children = append(file.Children, pkg)
	}
}

// processImport handles import statements
func (p *TreeSitterProcessor) processImport(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	if node.Type() == "import_list" {
		// Process all import statements in the list
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "import_header" {
				p.processImport(child, source, file)
			}
		}
		return
	}

	// Process individual import header
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		ImportType: "import",
		Symbols:    []ir.ImportedSymbol{},
	}

	var importPath string
	var hasAlias bool
	var aliasName string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier", "package_identifier":
			importPath = string(source[child.StartByte():child.EndByte()])
		case "import_alias":
			hasAlias = true
			// Extract alias name
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				if grandchild.Type() == "type_identifier" || grandchild.Type() == "simple_identifier" {
					aliasName = string(source[grandchild.StartByte():grandchild.EndByte()])
				}
			}
		case "wildcard_import":
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{Name: "*"})
		}
	}

	imp.Module = importPath

	if !hasAlias && len(imp.Symbols) == 0 {
		// Import a specific class/function
		parts := strings.Split(importPath, ".")
		if len(parts) > 0 {
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name: parts[len(parts)-1],
			})
		}
	} else if hasAlias {
		// Import with alias
		parts := strings.Split(importPath, ".")
		if len(parts) > 0 {
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  parts[len(parts)-1],
				Alias: aliasName,
			})
		}
	}

	file.Children = append(file.Children, imp)
}

// processClassDeclaration handles class declarations (including data classes and sealed classes)
func (p *TreeSitterProcessor) processClassDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Check if this is actually an enum class
	isEnum := false
	isInterface := false
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "enum" {
			isEnum = true
			break
		} else if child.Type() == "interface" {
			isInterface = true
			break
		}
	}

	if isEnum {
		p.processEnumDeclaration(node, source, file, parent)
		return
	}

	if isInterface {
		p.processInterfaceDeclaration(node, source, file, parent)
		return
	}

	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, name, type parameters, extends, implements
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractModifiers(child, source, class)
		case "type_identifier", "simple_identifier":
			class.Name = string(source[child.StartByte():child.EndByte()])
		case "type_parameters":
			p.extractTypeParameters(child, source, class)
		case "primary_constructor":
			p.processPrimaryConstructor(child, source, class)
		case "delegation_specifier":
			p.extractDelegationSpecifier(child, source, class)
		case "class_body":
			p.processClassBody(child, source, file, class)
		}
	}

	// Set default visibility if not specified
	if class.Visibility == "" {
		class.Visibility = ir.VisibilityPublic // Classes are public by default in Kotlin
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

	// Extract modifiers, name, type parameters, extends
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractInterfaceModifiers(child, source, iface)
		case "type_identifier", "simple_identifier":
			iface.Name = string(source[child.StartByte():child.EndByte()])
		case "type_parameters":
			p.extractInterfaceTypeParameters(child, source, iface)
		case "delegation_specifier":
			p.extractInterfaceDelegationSpecifier(child, source, iface)
		case "class_body":
			p.processInterfaceBody(child, source, file, iface)
		}
	}

	// Set default visibility if not specified
	if iface.Visibility == "" {
		iface.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, iface)
}

// processEnumDeclaration handles enum class declarations
func (p *TreeSitterProcessor) processEnumDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	enum := &ir.DistilledEnum{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Children: []ir.DistilledNode{},
	}

	// Extract modifiers, name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractEnumModifiers(child, source, enum)
		case "type_identifier", "simple_identifier":
			enum.Name = string(source[child.StartByte():child.EndByte()])
		case "primary_constructor":
			// Enum classes can have constructors
			p.processEnumPrimaryConstructor(child, source, enum)
		case "enum_class_body", "class_body":
			p.processEnumBody(child, source, file, enum)
		}
	}

	// Set default visibility if not specified
	if enum.Visibility == "" {
		enum.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, enum)
}

// processObjectDeclaration handles object declarations (singletons)
func (p *TreeSitterProcessor) processObjectDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Object declarations are like classes but singletons
	obj := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Modifiers: []ir.Modifier{ir.ModifierStatic}, // Objects are essentially static singletons
		Children:  []ir.DistilledNode{},
	}

	// Extract modifiers, name, extends, implements
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractModifiers(child, source, obj)
		case "type_identifier", "simple_identifier":
			obj.Name = string(source[child.StartByte():child.EndByte()])
		case "delegation_specifier":
			p.extractDelegationSpecifier(child, source, obj)
		case "class_body":
			p.processClassBody(child, source, file, obj)
		}
	}

	// Set default visibility if not specified
	if obj.Visibility == "" {
		obj.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, obj)
}

// processCompanionObject handles companion object declarations
func (p *TreeSitterProcessor) processCompanionObject(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Companion objects are nested classes with special semantics
	companion := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:      "Companion", // Default name if not specified
		Modifiers: []ir.Modifier{ir.ModifierStatic},
		Children:  []ir.DistilledNode{},
	}

	// Extract modifiers, optional name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractModifiers(child, source, companion)
		case "type_identifier", "simple_identifier":
			companion.Name = string(source[child.StartByte():child.EndByte()])
		case "class_body":
			p.processClassBody(child, source, file, companion)
		}
	}

	p.addToParent(file, parent, companion)
}

// processFunctionDeclaration handles function declarations (including extension functions)
func (p *TreeSitterProcessor) processFunctionDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	function := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	var receiverType string

	// Extract modifiers, receiver type, name, parameters, return type, body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractFunctionModifiers(child, source, function)
		case "type_parameters":
			p.extractTypeParams(child, source, function)
		case "function_value_parameters":
			p.extractParameters(child, source, function)
		case "simple_identifier":
			if function.Name == "" {
				function.Name = string(source[child.StartByte():child.EndByte()])
			}
		case "user_type", "nullable_type":
			// This could be receiver type for extension functions or return type
			typeStr := string(source[child.StartByte():child.EndByte()])
			if function.Name == "" {
				// This is a receiver type (extension function)
				receiverType = typeStr
			} else if function.Returns == nil {
				// This is the return type
				function.Returns = &ir.TypeRef{Name: typeStr}
			}
		case "function_body":
			p.extractFunctionBody(child, source, function)
		}
	}

	// If it's an extension function, prepend receiver type to name
	if receiverType != "" {
		function.Name = receiverType + "." + function.Name
		function.Decorators = append(function.Decorators, "@Extension")
	}

	// Set default visibility if not specified
	if function.Visibility == "" {
		function.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, function)
}

// processPropertyDeclaration handles property declarations
func (p *TreeSitterProcessor) processPropertyDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Properties in Kotlin can have getters/setters
	property := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Modifiers: []ir.Modifier{},
	}

	var isVal bool
	var hasGetter, hasSetter bool

	// Extract modifiers, val/var, name, type, initializer
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractPropertyModifiers(child, source, property)
		case "val", "var":
			isVal = child.Type() == "val"
		case "variable_declaration":
			p.extractVariableDeclaration(child, source, property)
		case "getter":
			hasGetter = true
		case "setter":
			hasSetter = true
		case "property_delegate":
			// Handle delegated properties
			property.Decorators = append(property.Decorators, "@Delegated")
		}
	}

	// Val properties are immutable (final)
	if isVal {
		property.Modifiers = append(property.Modifiers, ir.ModifierFinal)
	}

	// If property has custom getter/setter, note it
	if hasGetter || hasSetter {
		accessors := []string{}
		if hasGetter {
			accessors = append(accessors, "get")
		}
		if hasSetter {
			accessors = append(accessors, "set")
		}
		property.Decorators = append(property.Decorators, "@Accessors("+strings.Join(accessors, ",")+")")
	}

	// Set default visibility if not specified
	if property.Visibility == "" {
		property.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, property)
}

// processSecondaryConstructor handles secondary constructor declarations
func (p *TreeSitterProcessor) processSecondaryConstructor(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	constructor := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:       "constructor",
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract modifiers, parameters, body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractFunctionModifiers(child, source, constructor)
		case "function_value_parameters":
			p.extractParameters(child, source, constructor)
		case "constructor_delegation_call":
			// Note if this constructor delegates to another
			constructor.Decorators = append(constructor.Decorators, "@Delegates")
		case "block":
			constructor.Implementation = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Set default visibility if not specified
	if constructor.Visibility == "" {
		constructor.Visibility = ir.VisibilityPublic
	}

	p.addToParent(file, parent, constructor)
}

// processPrimaryConstructor handles primary constructor as part of class declaration
func (p *TreeSitterProcessor) processPrimaryConstructor(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	// Create a constructor function
	constructor := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:       "constructor",
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
		Decorators: []string{"@Primary"},
	}

	// Extract visibility and parameters
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			p.extractFunctionModifiers(child, source, constructor)
		case "class_parameter":
			param := p.extractClassParameter(child, source)
			constructor.Parameters = append(constructor.Parameters, *param)

			// If parameter is val/var, it's also a property
			// For data classes, ALL parameters are properties
			isDataClass := false
			for _, mod := range class.Modifiers {
				if mod == ir.ModifierData {
					isDataClass = true
					break
				}
			}

			if isDataClass || p.isPropertyParameter(child, source) {
				p.createPropertyFromParameter(child, source, class, param)
			}
		}
	}

	// Set default visibility if not specified
	if constructor.Visibility == "" {
		constructor.Visibility = ir.VisibilityPublic
	}

	// Add constructor to class
	class.Children = append(class.Children, constructor)
}

// processTypeAlias handles type alias declarations
func (p *TreeSitterProcessor) processTypeAlias(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	alias := &ir.DistilledTypeAlias{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic, // Default visibility
	}

	// Extract modifiers, name, and type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "modifiers":
			// Extract visibility
			for j := 0; j < int(child.ChildCount()); j++ {
				mod := child.Child(j)
				text := string(source[mod.StartByte():mod.EndByte()])
				switch text {
				case "public":
					alias.Visibility = ir.VisibilityPublic
				case "private":
					alias.Visibility = ir.VisibilityPrivate
				case "protected":
					alias.Visibility = ir.VisibilityProtected
				case "internal":
					alias.Visibility = ir.VisibilityInternal
				}
			}
		case "type_identifier":
			alias.Name = string(source[child.StartByte():child.EndByte()])
		case "type", "user_type", "function_type", "nullable_type":
			// The actual type being aliased
			alias.Type = ir.TypeRef{
				Name: string(source[child.StartByte():child.EndByte()]),
			}
		}
	}

	p.addToParent(file, parent, alias)
}

// Helper methods

// extractModifiers extracts modifiers for classes
func (p *TreeSitterProcessor) extractModifiers(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(source[child.StartByte():child.EndByte()])

		switch text {
		case "public":
			class.Visibility = ir.VisibilityPublic
		case "protected":
			class.Visibility = ir.VisibilityProtected
		case "private":
			class.Visibility = ir.VisibilityPrivate
		case "internal":
			class.Visibility = ir.VisibilityInternal
		case "abstract":
			class.Modifiers = append(class.Modifiers, ir.ModifierAbstract)
		case "final":
			class.Modifiers = append(class.Modifiers, ir.ModifierFinal)
		case "open":
			// Open is the default for non-final classes
		case "sealed":
			class.Modifiers = append(class.Modifiers, ir.ModifierSealed)
		case "data":
			class.Modifiers = append(class.Modifiers, ir.ModifierData)
		case "annotation":
			class.Modifiers = append(class.Modifiers, ir.ModifierAnnotation)
		case "inner":
			// Inner classes - add as decorator
			class.Decorators = append(class.Decorators, "@inner")
		case "value":
			// Value classes (inline classes)
			class.Decorators = append(class.Decorators, "@value")
		case "inline":
			class.Decorators = append(class.Decorators, "@inline")
		}

		// Handle annotations
		if child.Type() == "annotation" || child.Type() == "user_type" {
			p.extractAnnotation(child, source, &class.Decorators)
		}
	}
}

// extractInterfaceModifiers extracts modifiers for interfaces
func (p *TreeSitterProcessor) extractInterfaceModifiers(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(source[child.StartByte():child.EndByte()])

		switch text {
		case "public":
			iface.Visibility = ir.VisibilityPublic
		case "protected":
			iface.Visibility = ir.VisibilityProtected
		case "private":
			iface.Visibility = ir.VisibilityPrivate
		case "internal":
			iface.Visibility = ir.VisibilityInternal
		case "sealed":
			// TODO: Add sealed modifier support to interfaces in IR if needed
		case "fun":
			// Functional interfaces
			// TODO: Add functional interface support to IR if needed
		}

		// Handle annotations
		if child.Type() == "annotation" || child.Type() == "user_type" {
			// TODO: Add decorator support to interfaces in IR
		}
	}
}

// extractEnumModifiers extracts modifiers for enums
func (p *TreeSitterProcessor) extractEnumModifiers(node *sitter.Node, source []byte, enum *ir.DistilledEnum) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(source[child.StartByte():child.EndByte()])

		switch text {
		case "public":
			enum.Visibility = ir.VisibilityPublic
		case "protected":
			enum.Visibility = ir.VisibilityProtected
		case "private":
			enum.Visibility = ir.VisibilityPrivate
		case "internal":
			enum.Visibility = ir.VisibilityInternal
		}

		// Handle annotations
		if child.Type() == "annotation" || child.Type() == "user_type" {
			// TODO: Add decorator support to enums in IR if needed
		}
	}
}

// extractFunctionModifiers extracts modifiers for functions
func (p *TreeSitterProcessor) extractFunctionModifiers(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(source[child.StartByte():child.EndByte()])

		switch text {
		case "public":
			function.Visibility = ir.VisibilityPublic
		case "protected":
			function.Visibility = ir.VisibilityProtected
		case "private":
			function.Visibility = ir.VisibilityPrivate
		case "internal":
			function.Visibility = ir.VisibilityInternal
		case "abstract":
			function.Modifiers = append(function.Modifiers, ir.ModifierAbstract)
		case "final":
			function.Modifiers = append(function.Modifiers, ir.ModifierFinal)
		case "override":
			function.Modifiers = append(function.Modifiers, ir.ModifierOverride)
		case "open":
			// Open is the default for non-final functions
		case "inline":
			function.Modifiers = append(function.Modifiers, ir.ModifierInline)
		case "suspend":
			function.Modifiers = append(function.Modifiers, ir.ModifierAsync)
		case "operator":
			function.Decorators = append(function.Decorators, "@operator")
		case "infix":
			function.Decorators = append(function.Decorators, "@infix")
		case "tailrec":
			function.Decorators = append(function.Decorators, "@tailrec")
		}

		// Handle annotations
		if child.Type() == "annotation" || child.Type() == "user_type" {
			p.extractAnnotation(child, source, &function.Decorators)
		}
	}
}

// extractPropertyModifiers extracts modifiers for properties
func (p *TreeSitterProcessor) extractPropertyModifiers(node *sitter.Node, source []byte, property *ir.DistilledField) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(source[child.StartByte():child.EndByte()])

		switch text {
		case "public":
			property.Visibility = ir.VisibilityPublic
		case "protected":
			property.Visibility = ir.VisibilityProtected
		case "private":
			property.Visibility = ir.VisibilityPrivate
		case "internal":
			property.Visibility = ir.VisibilityInternal
		case "override":
			property.Modifiers = append(property.Modifiers, ir.ModifierOverride)
		case "lateinit":
			// Lateinit modifier - add as decorator
			property.Decorators = append(property.Decorators, "@lateinit")
		case "const":
			property.Modifiers = append(property.Modifiers, ir.ModifierConst)
		}

		// Handle annotations
		if child.Type() == "annotation" || child.Type() == "user_type" {
			p.extractAnnotation(child, source, &property.Decorators)
		}
	}
}

// extractAnnotation extracts annotation information as decorator
func (p *TreeSitterProcessor) extractAnnotation(node *sitter.Node, source []byte, decorators *[]string) {
	annText := string(source[node.StartByte():node.EndByte()])
	if !strings.HasPrefix(annText, "@") {
		annText = "@" + annText
	}
	*decorators = append(*decorators, annText)
}

// extractTypeParameters extracts generic type parameters for classes
func (p *TreeSitterProcessor) extractTypeParameters(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	// TODO: Extract and store type parameters
	// For now, we'll add them as a decorator
	typeParams := string(source[node.StartByte():node.EndByte()])
	class.Decorators = append(class.Decorators, "@TypeParams"+typeParams)
}

// extractTypeParams extracts generic type parameters for functions
func (p *TreeSitterProcessor) extractTypeParams(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter" {
			param := ir.TypeParam{
				Name: "",
			}

			// Extract parameter name and constraints
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				if grandchild.Type() == "type_identifier" || grandchild.Type() == "simple_identifier" {
					param.Name = string(source[grandchild.StartByte():grandchild.EndByte()])
				} else if grandchild.Type() == "type_constraint" {
					// Extract constraint
					for k := 0; k < int(grandchild.ChildCount()); k++ {
						constraintChild := grandchild.Child(k)
						if constraintChild.Type() == "user_type" || constraintChild.Type() == "nullable_type" {
							typeRef := p.extractType(constraintChild, source)
							if typeRef != nil {
								param.Constraints = append(param.Constraints, *typeRef)
							}
						}
					}
				}
			}

			if param.Name != "" {
				function.TypeParams = append(function.TypeParams, param)
			}
		}
	}
}

// extractInterfaceTypeParameters extracts generic type parameters for interfaces
func (p *TreeSitterProcessor) extractInterfaceTypeParameters(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	// TODO: Extract and store type parameters
}

// extractDelegationSpecifier extracts superclass and interfaces for classes
func (p *TreeSitterProcessor) extractDelegationSpecifier(node *sitter.Node, source []byte, class *ir.DistilledClass) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "delegation_specifier" {
			// Each delegation_specifier is either a superclass or interface
			typeRef := p.extractDelegatedType(child, source)
			if typeRef != nil {
				// In Kotlin, the first one could be a class, rest are interfaces
				// But we can't distinguish easily, so we'll add all to implements
				class.Implements = append(class.Implements, *typeRef)
			}
		}
	}
}

// extractInterfaceDelegationSpecifier extracts extended interfaces
func (p *TreeSitterProcessor) extractInterfaceDelegationSpecifier(node *sitter.Node, source []byte, iface *ir.DistilledInterface) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "delegation_specifier" {
			typeRef := p.extractDelegatedType(child, source)
			if typeRef != nil {
				iface.Extends = append(iface.Extends, *typeRef)
			}
		}
	}
}

// extractDelegatedType extracts a delegated type from delegation_specifier
func (p *TreeSitterProcessor) extractDelegatedType(node *sitter.Node, source []byte) *ir.TypeRef {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "user_type", "nullable_type":
			return p.extractType(child, source)
		case "constructor_invocation":
			// Extract type from constructor invocation
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				if grandchild.Type() == "user_type" {
					return p.extractType(grandchild, source)
				}
			}
		}
	}
	return nil
}

// extractType extracts type information
func (p *TreeSitterProcessor) extractType(node *sitter.Node, source []byte) *ir.TypeRef {
	nodeType := node.Type()

	switch nodeType {
	case "type_identifier", "simple_identifier":
		return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
	case "user_type":
		// Handle qualified type names and generics
		var typeName string
		var generics []ir.TypeRef
		var isNullable bool

		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			switch child.Type() {
			case "type_identifier", "simple_identifier":
				if typeName != "" {
					typeName += "."
				}
				typeName += string(source[child.StartByte():child.EndByte()])
			case "type_arguments":
				// Extract generic arguments
				for j := 0; j < int(child.ChildCount()); j++ {
					argChild := child.Child(j)
					if argChild.Type() == "type_argument" {
						for k := 0; k < int(argChild.ChildCount()); k++ {
							typeChild := argChild.Child(k)
							if typeChild.Type() != "," {
								argType := p.extractType(typeChild, source)
								if argType != nil {
									generics = append(generics, *argType)
								}
							}
						}
					}
				}
			}
		}

		return &ir.TypeRef{
			Name:       typeName,
			TypeArgs:   generics,
			IsNullable: isNullable,
		}
	case "nullable_type":
		// Extract the underlying type and mark as nullable
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() != "?" {
				typeRef := p.extractType(child, source)
				if typeRef != nil {
					typeRef.IsNullable = true
					return typeRef
				}
			}
		}
	case "function_type":
		// Lambda/function types
		return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
	}

	// Default: return the raw text
	return &ir.TypeRef{Name: string(source[node.StartByte():node.EndByte()])}
}

// extractParameters extracts function parameters
func (p *TreeSitterProcessor) extractParameters(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter" {
			param := p.extractParameter(child, source)
			function.Parameters = append(function.Parameters, *param)
		}
	}
}

// extractParameter extracts a single parameter
func (p *TreeSitterProcessor) extractParameter(node *sitter.Node, source []byte) *ir.Parameter {
	param := &ir.Parameter{}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "simple_identifier":
			param.Name = string(source[child.StartByte():child.EndByte()])
		case "user_type", "nullable_type", "function_type":
			typeRef := p.extractType(child, source)
			if typeRef != nil {
				param.Type = *typeRef
			}
		case "parameter_modifiers":
			// Handle parameter modifiers like vararg
			for j := 0; j < int(child.ChildCount()); j++ {
				modChild := child.Child(j)
				if string(source[modChild.StartByte():modChild.EndByte()]) == "vararg" {
					param.IsVariadic = true
				}
			}
		case "parameter_with_optional_type":
			// Handle parameters with default values
			param.IsOptional = true
		}
	}

	return param
}

// extractClassParameter extracts parameters from primary constructor
func (p *TreeSitterProcessor) extractClassParameter(node *sitter.Node, source []byte) *ir.Parameter {
	param := &ir.Parameter{}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "simple_identifier":
			param.Name = string(source[child.StartByte():child.EndByte()])
		case "user_type", "nullable_type":
			typeRef := p.extractType(child, source)
			if typeRef != nil {
				param.Type = *typeRef
			}
		case "modifiers":
			// Check for vararg
			for j := 0; j < int(child.ChildCount()); j++ {
				modChild := child.Child(j)
				if string(source[modChild.StartByte():modChild.EndByte()]) == "vararg" {
					param.IsVariadic = true
				}
			}
		}
	}

	return param
}

// isPropertyParameter checks if a class parameter is also a property (val/var)
func (p *TreeSitterProcessor) isPropertyParameter(node *sitter.Node, source []byte) bool {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "val" || child.Type() == "var" {
			return true
		}
		// Check binding_pattern_kind for val/var
		if child.Type() == "binding_pattern_kind" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				if grandchild.Type() == "val" || grandchild.Type() == "var" {
					return true
				}
			}
		}
	}
	return false
}

// createPropertyFromParameter creates a property from a primary constructor parameter
func (p *TreeSitterProcessor) createPropertyFromParameter(node *sitter.Node, source []byte, class *ir.DistilledClass, param *ir.Parameter) {
	property := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:       param.Name,
		Type:       &param.Type,
		Visibility: ir.VisibilityPublic, // Default for constructor properties
		Modifiers:  []ir.Modifier{},
	}

	// Check if val or var
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "val" {
			property.Modifiers = append(property.Modifiers, ir.ModifierFinal)
		}
		// Check binding_pattern_kind for val/var
		if child.Type() == "binding_pattern_kind" {
			for j := 0; j < int(child.ChildCount()); j++ {
				grandchild := child.Child(j)
				if grandchild.Type() == "val" {
					property.Modifiers = append(property.Modifiers, ir.ModifierFinal)
				}
			}
		}
		// Extract visibility modifiers
		if child.Type() == "modifiers" {
			p.extractPropertyModifiers(child, source, property)
		}
	}

	class.Children = append(class.Children, property)
}

// extractVariableDeclaration extracts variable name and type from property
func (p *TreeSitterProcessor) extractVariableDeclaration(node *sitter.Node, source []byte, property *ir.DistilledField) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "simple_identifier":
			property.Name = string(source[child.StartByte():child.EndByte()])
		case "user_type", "nullable_type":
			property.Type = p.extractType(child, source)
		case "=":
			// Skip assignment operator
		default:
			// This could be the initializer
			if i > 0 && node.Child(i-1).Type() == "=" {
				property.DefaultValue = string(source[child.StartByte():child.EndByte()])
			}
		}
	}
}

// extractFunctionBody extracts function implementation
func (p *TreeSitterProcessor) extractFunctionBody(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	// Expression body ("= expr") or braced block; the grammar has no separate
	// block node, so a braced body is the whole function_body text.
	for i := 0; i < int(node.ChildCount())-1; i++ {
		if node.Child(i).Type() == "=" {
			expr := node.Child(i + 1)
			function.Implementation = "= " + string(source[expr.StartByte():expr.EndByte()])
			return
		}
	}
	function.Implementation = strings.TrimSpace(string(source[node.StartByte():node.EndByte()]))
}

// processClassBody processes the body of a class
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		p.processNode(child, source, file, parent)
	}
}

// processInterfaceBody processes the body of an interface
func (p *TreeSitterProcessor) processInterfaceBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		p.processNode(child, source, file, parent)
	}
}

// processEnumBody processes the body of an enum
func (p *TreeSitterProcessor) processEnumBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		nodeType := child.Type()

		switch nodeType {
		case "enum_entry":
			p.processEnumEntry(child, source, file, parent)
		case "function_declaration", "property_declaration":
			// Enums can have methods and properties
			p.processNode(child, source, file, parent)
		}
	}
}

// processEnumEntry processes enum constants
func (p *TreeSitterProcessor) processEnumEntry(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	field := &ir.DistilledField{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierStatic, ir.ModifierFinal},
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "simple_identifier":
			field.Name = string(source[child.StartByte():child.EndByte()])
		case "value_arguments":
			// Capture constructor arguments
			field.DefaultValue = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Enum constants have the enum type
	if enum, ok := parent.(*ir.DistilledEnum); ok {
		field.Type = &ir.TypeRef{Name: enum.Name}
	}

	p.addToParent(file, parent, field)
}

// processEnumPrimaryConstructor processes enum primary constructor
func (p *TreeSitterProcessor) processEnumPrimaryConstructor(node *sitter.Node, source []byte, enum *ir.DistilledEnum) {
	// Enum constructors define the structure of enum entries
	// We can note this in the enum's decorators
	params := string(source[node.StartByte():node.EndByte()])
	// Add constructor info as a comment
	enum.Children = append(enum.Children, &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   "Constructor: " + params,
		Format: "doc",
	})
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

// addToParent adds a node to its parent or to the file
func (p *TreeSitterProcessor) addToParent(file *ir.DistilledFile, parent ir.DistilledNode, child ir.DistilledNode) {
	if parent != nil {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, child)
		case *ir.DistilledInterface:
			p.Children = append(p.Children, child)
		case *ir.DistilledEnum:
			p.Children = append(p.Children, child)
		default:
			file.Children = append(file.Children, child)
		}
	} else {
		file.Children = append(file.Children, child)
	}
}
