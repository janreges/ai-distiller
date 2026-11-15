package cpp

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	sitter "github.com/smacker/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

// TreeSitterProcessor uses tree-sitter for C++ parsing
type TreeSitterProcessor struct {
	parser *sitter.Parser
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(tree_sitter_cpp.Language()))

	return &TreeSitterProcessor{
		parser: parser,
	}
}

// ProcessSource processes C++ source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// Parse the source code
	tree, err := p.parser.ParseCtx(ctx, nil, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse C++ code: %w", err)
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
		Language: "cpp",
		Version:  "1.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Process the tree
	p.processNode(tree.RootNode(), source, file, nil)
	members := memberVisibilities{}
	p.indexMembers(tree.RootNode(), source, nil, ir.VisibilityPublic, members)
	inheritOutOfClassVisibility(file.Children, members)

	return file, nil
}

// memberVisibilities maps a member key to the visibility of every in-class
// declaration or definition of that member. The key is the chain of
// enclosing class names and the member name joined by "::" ("Foo::bar",
// "Outer::Inner::bar"); namespaces and template arguments are left out.
type memberVisibilities map[string][]ir.Visibility

// indexMembers records in idx the visibility of every member function declared
// or defined in a class, struct or union body at or below node. scope holds the
// names of the enclosing classes. ceiling is the most open visibility a member
// can have: the access of the enclosing nested class within its outer class,
// so a public member of a private nested class is recorded as private.
// Function bodies are not searched: a local class has no out-of-class members.
func (p *TreeSitterProcessor) indexMembers(node *sitter.Node, source []byte, scope []string, ceiling ir.Visibility, idx memberVisibilities) {
	switch node.Type() {
	case "class_specifier", "struct_specifier", "union_specifier":
		name, body := node.ChildByFieldName("name"), node.ChildByFieldName("body")
		if name == nil || body == nil {
			return
		}
		path := append([]string{}, scope...)
		for _, seg := range splitScope(p.nodeText(name, source)) {
			path = append(path, stripTemplateArgs(seg))
		}
		access := ir.VisibilityPublic
		if node.Type() == "class_specifier" {
			access = ir.VisibilityPrivate
		}
		for i := 0; i < int(body.NamedChildCount()); i++ {
			member := body.NamedChild(i)
			if member.Type() == "access_specifier" {
				access = accessVisibility(p.nodeText(member, source))
				continue
			}
			p.indexMember(member, source, path, leastOpen(ceiling, access), idx)
		}
	case "compound_statement":
		return
	default:
		for i := 0; i < int(node.NamedChildCount()); i++ {
			p.indexMembers(node.NamedChild(i), source, scope, ceiling, idx)
		}
	}
}

// indexMember records one member of the class body whose class chain is path;
// vis is the member's visibility. A nested class is indexed with vis as its
// ceiling.
func (p *TreeSitterProcessor) indexMember(member *sitter.Node, source []byte, path []string, vis ir.Visibility, idx memberVisibilities) {
	switch member.Type() {
	case "class_specifier", "struct_specifier", "union_specifier":
		p.indexMembers(member, source, path, vis, idx)
	case "template_declaration":
		for i := 0; i < int(member.NamedChildCount()); i++ {
			p.indexMember(member.NamedChild(i), source, path, vis, idx)
		}
	case "field_declaration", "declaration", "function_definition":
		// `class In {...};` inside a class is a field_declaration whose type
		// is the nested class.
		if typ := member.ChildByFieldName("type"); typ != nil {
			p.indexMembers(typ, source, path, vis, idx)
		}
		if name := p.memberFunctionName(member, source); name != "" {
			key := strings.Join(append(append([]string{}, path...), name), "::")
			idx[key] = append(idx[key], vis)
		}
	}
}

// memberFunctionName returns the name of the function that decl declares or
// defines, looking through pointer and reference declarators, or "" when decl
// declares no function.
func (p *TreeSitterProcessor) memberFunctionName(decl *sitter.Node, source []byte) string {
	d := decl.ChildByFieldName("declarator")
	for d != nil {
		if d.Type() == "function_declarator" {
			if name := d.ChildByFieldName("declarator"); name != nil {
				return p.nodeText(name, source)
			}
			return ""
		}
		// reference_declarator has no "declarator" field: the wrapped
		// declarator is its last named child.
		next := d.ChildByFieldName("declarator")
		if next == nil && d.NamedChildCount() > 0 {
			next = d.NamedChild(int(d.NamedChildCount()) - 1)
		}
		d = next
	}
	return ""
}

// inheritOutOfClassVisibility gives every out-of-class member definition
// (`int X::m() {...}`) among nodes, including those inside namespaces, the
// visibility of the in-class declaration of m in class X. The definition's
// qualified name is matched from its longest suffix down to "Class::member",
// so namespace qualifiers (`ns::X::m`) and template arguments (`X<T>::m`) are
// ignored and nested classes (`Outer::Inner::m`) match. Without a matching
// declaration the definition stays public. With overloads, any member of that
// name decides: the definition gets the most open visibility found.
func inheritOutOfClassVisibility(nodes []ir.DistilledNode, members memberVisibilities) {
	for _, node := range nodes {
		switch n := node.(type) {
		case *ir.DistilledPackage:
			inheritOutOfClassVisibility(n.Children, members)
		case *ir.DistilledFunction:
			if vis, found := lookupMember(n.Name, members); found {
				n.Visibility = vis
			}
		}
	}
}

// lookupMember returns the most open visibility recorded for the qualified
// function name, or false when name is unqualified or matches no member.
func lookupMember(name string, members memberVisibilities) (ir.Visibility, bool) {
	segs := splitScope(name)
	if len(segs) < 2 {
		return "", false
	}
	for i := range segs[:len(segs)-1] {
		segs[i] = stripTemplateArgs(segs[i])
	}
	for start := 0; start <= len(segs)-2; start++ {
		vis, found := members[strings.Join(segs[start:], "::")]
		if !found {
			continue
		}
		best := vis[0]
		for _, v := range vis[1:] {
			if visibilityRank(v) > visibilityRank(best) {
				best = v
			}
		}
		return best, true
	}
	return "", false
}

// splitScope splits a qualified name on the "::" separators that are outside
// template argument lists: "a::B<c::D>::f" gives "a", "B<c::D>", "f".
func splitScope(name string) []string {
	var segs []string
	depth, start := 0, 0
	for i := 0; i < len(name); i++ {
		switch name[i] {
		case '<':
			depth++
		case '>':
			if depth > 0 {
				depth--
			}
		case ':':
			if depth == 0 && i+1 < len(name) && name[i+1] == ':' {
				segs = append(segs, strings.TrimSpace(name[start:i]))
				start = i + 2
				i++
			}
		}
	}
	return append(segs, strings.TrimSpace(name[start:]))
}

// stripTemplateArgs returns a class name without its template argument list.
func stripTemplateArgs(seg string) string {
	if i := strings.IndexByte(seg, '<'); i > 0 {
		return strings.TrimSpace(seg[:i])
	}
	return seg
}

// accessVisibility maps the text of an access_specifier to a visibility.
func accessVisibility(spec string) ir.Visibility {
	switch strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(spec), ":")) {
	case "public":
		return ir.VisibilityPublic
	case "protected":
		return ir.VisibilityProtected
	}
	return ir.VisibilityPrivate
}

// visibilityRank orders visibilities from private (0) to public (3).
func visibilityRank(v ir.Visibility) int {
	switch v {
	case ir.VisibilityPublic:
		return 3
	case ir.VisibilityProtected:
		return 2
	case ir.VisibilityInternal:
		return 1
	}
	return 0
}

// leastOpen returns the less open of a and b.
func leastOpen(a, b ir.Visibility) ir.Visibility {
	if visibilityRank(a) < visibilityRank(b) {
		return a
	}
	return b
}

// processNode recursively processes tree-sitter nodes
func (p *TreeSitterProcessor) processNode(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	nodeType := node.Type()

	switch nodeType {
	case "translation_unit":
		// Process all children of the translation unit
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, source, file, parent)
		}
	case "preproc_include":
		p.processInclude(node, source, file)
	case "using_declaration":
		p.processUsingDeclaration(node, source, file)
	case "namespace_definition":
		p.processNamespace(node, source, file, parent)
	case "class_specifier":
		p.processClass(node, source, file, parent)
	case "struct_specifier":
		p.processStruct(node, source, file, parent)
	case "union_specifier":
		p.processUnion(node, source, file, parent)
	case "enum_specifier":
		p.processEnum(node, source, file, parent)
	case "function_definition":
		p.processFunctionDefinition(node, source, file, parent)
	case "declaration":
		p.processDeclaration(node, source, file, parent)
	case "template_declaration":
		p.processTemplateDeclaration(node, source, file, parent)
	case "friend_declaration":
		p.processFriendDeclaration(node, source, file, parent)
	case "comment":
		p.processComment(node, source, file, parent)
	case "concept_definition":
		// C++20 concepts - parse as special comment for now
		p.processConceptAsComment(node, source, file, parent)
	case "requires_clause", "requires_expression":
		// C++20 requires clauses - skip for now
		// TODO: Add proper support for C++20 concepts
	default:
		// Recursively process children
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			p.processNode(child, source, file, parent)
		}
	}
}

// processInclude handles #include directives
func (p *TreeSitterProcessor) processInclude(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "system_lib_string" || child.Type() == "string_literal" {
			path := p.nodeText(child, source)
			// Remove quotes or angle brackets
			path = strings.Trim(path, "\"<>")

			imp := &ir.DistilledImport{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(node),
				},
				Module:     path,
				ImportType: "include",
			}
			file.Children = append(file.Children, imp)
			return
		}
	}
}

// processUsingDeclaration handles using declarations
func (p *TreeSitterProcessor) processUsingDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	text := p.nodeText(node, source)
	// Extract the namespace or symbol being used
	parts := strings.Fields(text)
	if len(parts) >= 2 && parts[0] == "using" {
		module := strings.TrimSuffix(strings.Join(parts[1:], " "), ";")

		imp := &ir.DistilledImport{
			BaseNode: ir.BaseNode{
				Location: p.nodeLocation(node),
			},
			Module:     module,
			ImportType: "using",
		}
		file.Children = append(file.Children, imp)
	}
}

// processNamespace handles namespace definitions
func (p *TreeSitterProcessor) processNamespace(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Use DistilledPackage for namespaces (following C# pattern)
	namespace := &ir.DistilledPackage{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:     "anonymous",
		Children: []ir.DistilledNode{},
	}

	// Find namespace name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" || child.Type() == "nested_namespace_specifier" || child.Type() == "namespace_identifier" {
			namespace.Name = p.nodeText(child, source)
			// Don't break, keep looking for the actual name
		}
	}

	// Process namespace body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "declaration_list" {
			for j := 0; j < int(child.ChildCount()); j++ {
				p.processNode(child.Child(j), source, file, namespace)
			}
		}
	}

	p.addChild(file, parent, namespace)
}

// processClass handles class declarations
func (p *TreeSitterProcessor) processClass(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic, // Default for C++ classes
		Children:   []ir.DistilledNode{},
	}

	// Find class name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			class.Name = p.nodeText(child, source)
			break
		}
	}

	// Process base classes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "base_class_clause" {
			class.Extends = p.extractBaseClasses(child, source)
		}
	}

	// Process class body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_declaration_list" {
			p.processClassBody(child, source, file, class)
		}
	}

	p.addChild(file, parent, class)
}

// processStruct handles struct declarations (similar to class but default public)
func (p *TreeSitterProcessor) processStruct(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// In C++, structs are essentially classes with default public access
	structNode := &ir.DistilledStruct{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic, // Structs are public by default
		Children:   []ir.DistilledNode{},
	}

	// Find struct name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			structNode.Name = p.nodeText(child, source)
			break
		}
	}

	// Process struct body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_declaration_list" {
			p.processStructBody(child, source, file, structNode)
		}
	}

	p.addChild(file, parent, structNode)
}

// processUnion handles union declarations
func (p *TreeSitterProcessor) processUnion(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Use DistilledStruct for unions as they're similar
	union := &ir.DistilledStruct{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:       "union",
		Visibility: ir.VisibilityPublic,
		Children:   []ir.DistilledNode{},
	}

	// Find union name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			union.Name = p.nodeText(child, source)
			break
		}
	}

	// Process union body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "field_declaration_list" {
			p.processStructBody(child, source, file, union)
		}
	}

	p.addChild(file, parent, union)
}

// processEnum handles enum declarations
func (p *TreeSitterProcessor) processEnum(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	enum := &ir.DistilledEnum{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Children:   []ir.DistilledNode{},
	}

	// Find enum name and values
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" {
			enum.Name = p.nodeText(child, source)
		} else if child.Type() == "enumerator_list" {
			// Extract enum values as children
			for j := 0; j < int(child.ChildCount()); j++ {
				enumChild := child.Child(j)
				if enumChild.Type() == "enumerator" {
					for k := 0; k < int(enumChild.ChildCount()); k++ {
						if enumChild.Child(k).Type() == "identifier" {
							// Create a field for each enum value
							field := &ir.DistilledField{
								BaseNode: ir.BaseNode{
									Location: p.nodeLocation(enumChild.Child(k)),
								},
								Name:       p.nodeText(enumChild.Child(k), source),
								Visibility: ir.VisibilityPublic,
							}
							enum.Children = append(enum.Children, field)
							break
						}
					}
				}
			}
		}
	}

	p.addChild(file, parent, enum)
}

// processClassBody processes the body of a class
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, source []byte, file *ir.DistilledFile, class *ir.DistilledClass) {
	currentVisibility := ir.VisibilityPrivate // Default for class

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)

		switch child.Type() {
		case "access_specifier":
			// Update current access level
			for j := 0; j < int(child.ChildCount()); j++ {
				accessChild := child.Child(j)
				switch accessChild.Type() {
				case "public":
					currentVisibility = ir.VisibilityPublic
				case "private":
					currentVisibility = ir.VisibilityPrivate
				case "protected":
					currentVisibility = ir.VisibilityProtected
				}
			}
		case "function_definition":
			p.processMethodDefinition(child, source, file, class, currentVisibility)
		case "declaration", "field_declaration":
			p.processClassDeclaration(child, source, file, class, currentVisibility)
		case "template_declaration":
			p.processTemplateDeclaration(child, source, file, class)
		case "friend_declaration":
			p.processFriendDeclaration(child, source, file, class)
		default:
			// Process other nodes recursively
			p.processNode(child, source, file, class)
		}
	}
}

// processStructBody processes the body of a struct
func (p *TreeSitterProcessor) processStructBody(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct) {
	currentVisibility := ir.VisibilityPublic // Default for struct

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)

		switch child.Type() {
		case "access_specifier":
			// Update current access level
			for j := 0; j < int(child.ChildCount()); j++ {
				accessChild := child.Child(j)
				switch accessChild.Type() {
				case "public":
					currentVisibility = ir.VisibilityPublic
				case "private":
					currentVisibility = ir.VisibilityPrivate
				case "protected":
					currentVisibility = ir.VisibilityProtected
				}
			}
		case "function_definition":
			p.processStructMethodDefinition(child, source, file, structNode, currentVisibility)
		case "declaration", "field_declaration":
			p.processStructFieldDeclaration(child, source, file, structNode, currentVisibility)
		case "template_declaration":
			p.processTemplateDeclaration(child, source, file, structNode)
		case "friend_declaration":
			p.processFriendDeclaration(child, source, file, structNode)
		default:
			// Process other nodes recursively
			p.processNode(child, source, file, structNode)
		}
	}
}

// processMethodDefinition handles method definitions inside a class
func (p *TreeSitterProcessor) processMethodDefinition(node *sitter.Node, source []byte, file *ir.DistilledFile, class *ir.DistilledClass, visibility ir.Visibility) {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: visibility,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract method details - build return type first, then function declarator
	var returnTypeStr string
	var foundDeclarator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Type() == "reference_declarator" {
					returnTypeStr += "&"
				} else {
					returnTypeStr += "*"
				}
			}
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "type_identifier", "primitive_type", "auto":
			if !foundDeclarator {
				returnTypeStr += p.nodeText(child, source)
			}
		case "type_qualifier":
			qualifier := p.nodeText(child, source)
			if qualifier == "const" && !foundDeclarator {
				// This is a const qualifier for the return type
				returnTypeStr = "const " + returnTypeStr
			}
		case "virtual":
			method.Modifiers = append(method.Modifiers, ir.ModifierVirtual)
		case "override":
			method.Modifiers = append(method.Modifiers, ir.ModifierOverride)
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
			}
		case "compound_statement":
			// Don't include implementation - it will be stripped if needed
			implementation := p.nodeText(child, source)
			if implementation != "" {
				method.Implementation = implementation
			}
		case "default_method_clause":
			method.Implementation = "= default"
		case "delete_method_clause":
			method.Implementation = "= delete"
		}
	}

	// Set return type if we found one
	if returnTypeStr != "" {
		method.Returns = &ir.TypeRef{
			Name: strings.TrimSpace(returnTypeStr),
		}
	}

	// In C++, constructors and destructors should keep their original names
	// No need to rename them

	// Check if this is a constructor or destructor and fix return type
	if method.Name == class.Name || (strings.HasPrefix(method.Name, "~") && strings.TrimPrefix(method.Name, "~") == class.Name) {
		// Constructor or destructor, no return type
		method.Returns = nil
	}

	class.Children = append(class.Children, method)
}

// processStructMethodDefinition handles method definitions inside a struct
func (p *TreeSitterProcessor) processStructMethodDefinition(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct, visibility ir.Visibility) {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: visibility,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract method details (similar to class methods)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declarator", "reference_declarator", "pointer_declarator":
			p.extractFunctionDeclarator(child, source, method)
		case "type_identifier", "primitive_type", "auto":
			if method.Returns == nil {
				method.Returns = &ir.TypeRef{
					Name: p.nodeText(child, source),
				}
			}
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
			}
		case "type_qualifier":
			qualifier := p.nodeText(child, source)
			if qualifier == "const" {
				method.Modifiers = append(method.Modifiers, ir.ModifierConst)
			}
		case "compound_statement":
			// Don't include implementation - it will be stripped if needed
			implementation := p.nodeText(child, source)
			if implementation != "" {
				method.Implementation = implementation
			}
		}
	}

	structNode.Children = append(structNode.Children, method)
}

// processFunctionDefinition handles standalone function definitions
func (p *TreeSitterProcessor) processFunctionDefinition(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	function := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic, // Standalone functions are public
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract function details - build return type first, then function declarator
	var returnTypeStr string
	var foundDeclarator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, function)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Type() == "reference_declarator" {
					returnTypeStr += "&"
				} else {
					returnTypeStr += "*"
				}
			}
			p.extractFunctionDeclarator(child, source, function)
			foundDeclarator = true
		case "type_identifier", "primitive_type", "auto":
			if !foundDeclarator {
				if returnTypeStr != "" && !strings.HasSuffix(returnTypeStr, " ") {
					returnTypeStr += " "
				}
				returnTypeStr += p.nodeText(child, source)
			}
		case "type_qualifier":
			qualifier := p.nodeText(child, source)
			if qualifier == "const" && !foundDeclarator {
				// This is a const qualifier for the return type
				returnTypeStr = "const " + returnTypeStr
			}
			if qualifier == "constexpr" {
				// Add constexpr as a const modifier
				function.Modifiers = append(function.Modifiers, ir.ModifierConst)
			}
		case "storage_class_specifier":
			spec := p.nodeText(child, source)
			if spec == "static" {
				function.Modifiers = append(function.Modifiers, ir.ModifierStatic)
			} else if spec == "inline" {
				function.Modifiers = append(function.Modifiers, ir.ModifierInline)
			}
		case "compound_statement":
			// Don't include implementation - it will be stripped if needed
			implementation := p.nodeText(child, source)
			if implementation != "" {
				function.Implementation = implementation
			}
		case "noexcept":
			// Could add as extension attribute if needed
		}
	}

	// Set return type if we found one
	if returnTypeStr != "" {
		function.Returns = &ir.TypeRef{
			Name: strings.TrimSpace(returnTypeStr),
		}
	}

	p.addChild(file, parent, function)
}

// extractFunctionDeclarator extracts function name and parameters
func (p *TreeSitterProcessor) extractFunctionDeclarator(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	// Handle functions returning references or pointers
	if node.Type() == "reference_declarator" || node.Type() == "pointer_declarator" {
		if function.Returns != nil {
			if node.Type() == "reference_declarator" {
				function.Returns.Name += "&"
			} else {
				function.Returns.Name += "*"
			}
		}
		// Recurse on the wrapped declarator, which is typically the last child
		if node.ChildCount() > 0 {
			p.extractFunctionDeclarator(node.Child(int(node.ChildCount())-1), source, function)
		}
		return
	}

	// Handle plain function_declarator and others
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "identifier", "field_identifier", "destructor_name", "operator_name", "type_identifier", "qualified_identifier":
			if function.Name == "" { // Avoid overwriting name from parenthesized declarators
				function.Name = p.nodeText(child, source)
			}
		case "parameter_list":
			p.extractParameters(child, source, function)
		case "type_qualifier":
			if p.nodeText(child, source) == "const" {
				function.Modifiers = append(function.Modifiers, ir.ModifierConst)
			}
		case "reference_declarator", "pointer_declarator", "parenthesized_declarator":
			// Recurse for function pointers or complex declarators
			p.extractFunctionDeclarator(child, source, function)
		}
	}
}

// extractParameters extracts function parameters
func (p *TreeSitterProcessor) extractParameters(node *sitter.Node, source []byte, function *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "parameter_declaration" || child.Type() == "optional_parameter_declaration" {
			param := ir.Parameter{
				Name: "",
				Type: ir.TypeRef{},
			}

			// Extract parameter type and name
			var typeStr string
			var foundType bool
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(j)
				switch paramChild.Type() {
				case "type_qualifier":
					if p.nodeText(paramChild, source) == "const" && !foundType {
						typeStr = "const " + typeStr
					}
				case "type_identifier", "primitive_type":
					typeStr += p.nodeText(paramChild, source)
					foundType = true
				case "identifier":
					param.Name = p.nodeText(paramChild, source)
				case "reference_declarator", "pointer_declarator":
					// Handle references and pointers - append declarator symbol to type
					declaratorPrefix := p.extractDeclaratorPrefix(paramChild, source)
					typeStr += declaratorPrefix
					// Extract name from within the declarator
					for k := 0; k < int(paramChild.ChildCount()); k++ {
						if paramChild.Child(k).Type() == "identifier" {
							param.Name = p.nodeText(paramChild.Child(k), source)
						}
					}
				case "optional_parameter_declaration":
					param.DefaultValue = "..." // Indicate it has a default
				}
			}

			if typeStr != "" {
				param.Type.Name = typeStr
			}

			function.Parameters = append(function.Parameters, param)
		}
	}
}

// processDeclaration handles general declarations
func (p *TreeSitterProcessor) processDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Build return type while looking for function declarator
	var returnTypeStr string
	var foundDeclarator bool
	var functionDeclaratorIdx int

	// First pass: find function declarator and build return type
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "function_declarator" || child.Type() == "reference_declarator" || child.Type() == "pointer_declarator" {
			foundDeclarator = true
			functionDeclaratorIdx = i
			break
		}
	}

	if foundDeclarator {
		// This is a function declaration
		function := &ir.DistilledFunction{
			BaseNode: ir.BaseNode{
				Location: p.nodeLocation(node),
			},
			Visibility: ir.VisibilityPublic, // Top-level functions are public
			Parameters: []ir.Parameter{},
			Modifiers:  []ir.Modifier{},
		}

		// Extract return type from nodes before the declarator
		for j := 0; j < functionDeclaratorIdx; j++ {
			typeChild := node.Child(j)
			switch typeChild.Type() {
			case "type_identifier", "primitive_type", "auto":
				if returnTypeStr != "" && !strings.HasSuffix(returnTypeStr, " ") {
					returnTypeStr += " "
				}
				returnTypeStr += p.nodeText(typeChild, source)
			case "type_qualifier":
				qualifier := p.nodeText(typeChild, source)
				if qualifier == "const" {
					returnTypeStr = "const " + returnTypeStr
				}
			case "storage_class_specifier":
				spec := p.nodeText(typeChild, source)
				if spec == "static" {
					function.Modifiers = append(function.Modifiers, ir.ModifierStatic)
				} else if spec == "inline" {
					function.Modifiers = append(function.Modifiers, ir.ModifierInline)
				}
			}
		}

		// Set return type if we found one
		if returnTypeStr != "" {
			function.Returns = &ir.TypeRef{
				Name: strings.TrimSpace(returnTypeStr),
			}
		}

		// Extract function details
		declarator := node.Child(functionDeclaratorIdx)
		p.extractFunctionDeclarator(declarator, source, function)
		p.addChild(file, parent, function)
		return
	}

	// Otherwise, it might be a variable declaration
	p.processVariableDeclaration(node, source, file, parent)
}

// processClassDeclaration handles declarations inside a class
func (p *TreeSitterProcessor) processClassDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, class *ir.DistilledClass, visibility ir.Visibility) {
	// Check if this is a method declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "function_declarator" {
			p.processMethodDeclaration(node, source, file, class, visibility)
			return
		}
	}

	// Otherwise, it's a field declaration
	p.processFieldDeclaration(node, source, file, class, visibility)
}

// processStructDeclaration handles declarations inside a struct
func (p *TreeSitterProcessor) processStructDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct, visibility ir.Visibility) {
	// Check if this is a method declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "function_declarator" {
			p.processStructMethodDeclaration(node, source, file, structNode, visibility)
			return
		}
	}

	// Otherwise, it's a field declaration
	p.processStructFieldDeclaration(node, source, file, structNode, visibility)
}

// processMethodDeclaration handles method declarations (without body)
func (p *TreeSitterProcessor) processMethodDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, class *ir.DistilledClass, visibility ir.Visibility) {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: visibility,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract method details - build return type first, then function declarator
	var returnTypeStr string
	var foundDeclarator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Type() == "reference_declarator" {
					returnTypeStr += "&"
				} else {
					returnTypeStr += "*"
				}
			}
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "type_identifier", "primitive_type", "auto":
			if !foundDeclarator {
				if returnTypeStr != "" && !strings.HasSuffix(returnTypeStr, " ") {
					returnTypeStr += " "
				}
				returnTypeStr += p.nodeText(child, source)
			}
		case "type_qualifier":
			qualifier := p.nodeText(child, source)
			if qualifier == "const" && !foundDeclarator {
				// This is a const qualifier for the return type
				returnTypeStr = "const " + returnTypeStr
			}
		case "virtual":
			method.Modifiers = append(method.Modifiers, ir.ModifierVirtual)
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
			}
		case "abstract_function_declarator":
			// Pure virtual function
			method.Modifiers = append(method.Modifiers, ir.ModifierVirtual, ir.ModifierAbstract)
		}
	}

	// Set return type if we found one
	if returnTypeStr != "" {
		method.Returns = &ir.TypeRef{
			Name: strings.TrimSpace(returnTypeStr),
		}
	}

	// Check for pure virtual (= 0)
	text := p.nodeText(node, source)
	if strings.Contains(text, "= 0") {
		method.Modifiers = append(method.Modifiers, ir.ModifierAbstract)
		if !p.hasModifier(method.Modifiers, ir.ModifierVirtual) {
			method.Modifiers = append(method.Modifiers, ir.ModifierVirtual)
		}
	}

	// In C++, constructors and destructors should keep their original names
	// No need to rename them

	// Check if this is a constructor or destructor and fix return type
	if method.Name == class.Name || (strings.HasPrefix(method.Name, "~") && strings.TrimPrefix(method.Name, "~") == class.Name) {
		// Constructor or destructor, no return type
		method.Returns = nil
	}

	class.Children = append(class.Children, method)
}

// processStructMethodDeclaration handles method declarations in structs
func (p *TreeSitterProcessor) processStructMethodDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct, visibility ir.Visibility) {
	method := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: visibility,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Extract method details - build return type first, then function declarator
	var returnTypeStr string
	var foundDeclarator bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Type() == "reference_declarator" {
					returnTypeStr += "&"
				} else {
					returnTypeStr += "*"
				}
			}
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "type_identifier", "primitive_type", "auto":
			if !foundDeclarator {
				if returnTypeStr != "" && !strings.HasSuffix(returnTypeStr, " ") {
					returnTypeStr += " "
				}
				returnTypeStr += p.nodeText(child, source)
			}
		case "type_qualifier":
			qualifier := p.nodeText(child, source)
			if qualifier == "const" && !foundDeclarator {
				// This is a const qualifier for the return type
				returnTypeStr = "const " + returnTypeStr
			}
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				method.Modifiers = append(method.Modifiers, ir.ModifierStatic)
			}
		}
	}

	// Set return type if we found one
	if returnTypeStr != "" {
		method.Returns = &ir.TypeRef{
			Name: strings.TrimSpace(returnTypeStr),
		}
	}

	structNode.Children = append(structNode.Children, method)
}

// processFieldDeclaration handles field declarations
func (p *TreeSitterProcessor) processFieldDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, class *ir.DistilledClass, visibility ir.Visibility) {
	var typeStr string
	modifiers := []ir.Modifier{}

	// Extract type and modifiers
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier", "primitive_type":
			typeStr = p.nodeText(child, source)
		case "storage_class_specifier":
			spec := p.nodeText(child, source)
			if spec == "static" {
				modifiers = append(modifiers, ir.ModifierStatic)
			} else if spec == "mutable" {
				modifiers = append(modifiers, ir.ModifierMutable)
			}
		case "type_qualifier":
			if p.nodeText(child, source) == "const" {
				modifiers = append(modifiers, ir.ModifierConst)
			}
		case "field_identifier", "identifier":
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(child),
				},
				Name:       p.nodeText(child, source),
				Visibility: visibility,
				Modifiers:  modifiers,
			}

			if typeStr != "" {
				field.Type = &ir.TypeRef{
					Name: typeStr,
				}
			}

			class.Children = append(class.Children, field)
		case "init_declarator":
			// Handle initialized fields
			for j := 0; j < int(child.ChildCount()); j++ {
				declChild := child.Child(j)
				if declChild.Type() == "identifier" {
					field := &ir.DistilledField{
						BaseNode: ir.BaseNode{
							Location: p.nodeLocation(declChild),
						},
						Name:       p.nodeText(declChild, source),
						Visibility: visibility,
						Modifiers:  modifiers,
					}

					if typeStr != "" {
						field.Type = &ir.TypeRef{
							Name: typeStr,
						}
					}

					class.Children = append(class.Children, field)
					break
				}
			}
		}
	}
}

// processStructFieldDeclaration handles field declarations in structs
func (p *TreeSitterProcessor) processStructFieldDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct, visibility ir.Visibility) {
	var typeStr string
	modifiers := []ir.Modifier{}

	// Extract type and modifiers (similar to class fields)
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier", "primitive_type":
			typeStr = p.nodeText(child, source)
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				modifiers = append(modifiers, ir.ModifierStatic)
			}
		case "type_qualifier":
			if p.nodeText(child, source) == "const" {
				modifiers = append(modifiers, ir.ModifierConst)
			}
		case "field_identifier", "identifier":
			field := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(child),
				},
				Name:       p.nodeText(child, source),
				Visibility: visibility,
				Modifiers:  modifiers,
			}

			if typeStr != "" {
				field.Type = &ir.TypeRef{
					Name: typeStr,
				}
			}

			structNode.Children = append(structNode.Children, field)
		}
	}
}

// processVariableDeclaration handles variable declarations
func (p *TreeSitterProcessor) processVariableDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	var typeStr string
	modifiers := []ir.Modifier{}

	// Extract type and modifiers
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "type_identifier", "primitive_type":
			typeStr = p.nodeText(child, source)
		case "storage_class_specifier":
			if p.nodeText(child, source) == "static" {
				modifiers = append(modifiers, ir.ModifierStatic)
			}
		case "type_qualifier":
			if p.nodeText(child, source) == "const" {
				modifiers = append(modifiers, ir.ModifierConst)
			}
		case "identifier":
			// Global variables are represented as fields
			variable := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(child),
				},
				Name:       p.nodeText(child, source),
				Visibility: ir.VisibilityPublic, // Top-level variables are public
				Modifiers:  modifiers,
			}

			if typeStr != "" {
				variable.Type = &ir.TypeRef{
					Name: typeStr,
				}
			}

			p.addChild(file, parent, variable)
		case "init_declarator":
			// Handle initialized variables
			for j := 0; j < int(child.ChildCount()); j++ {
				declChild := child.Child(j)
				if declChild.Type() == "identifier" {
					variable := &ir.DistilledField{
						BaseNode: ir.BaseNode{
							Location: p.nodeLocation(declChild),
						},
						Name:       p.nodeText(declChild, source),
						Visibility: ir.VisibilityPublic,
						Modifiers:  modifiers,
					}

					if typeStr != "" {
						variable.Type = &ir.TypeRef{
							Name: typeStr,
						}
					}

					p.addChild(file, parent, variable)
					break
				}
			}
		}
	}
}

// processTemplateDeclaration handles template declarations
func (p *TreeSitterProcessor) processTemplateDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Extract template parameters
	var templateParams []ir.TypeParam
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "template_parameter_list" {
			templateParams = p.extractTemplateParameters(child, source)
			break
		}
	}

	// Process the templated declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "class_specifier":
			// Create a temporary parent to capture the class
			var capturedClass *ir.DistilledClass
			tempParent := &captureNode{
				onAdd: func(child ir.DistilledNode) {
					if class, ok := child.(*ir.DistilledClass); ok {
						capturedClass = class
					}
				},
			}
			p.processClass(child, source, file, tempParent)

			// Add template parameters to the class
			if capturedClass != nil {
				capturedClass.TypeParams = templateParams
				p.addChild(file, parent, capturedClass)
			}
		case "function_definition":
			// Create a temporary parent to capture the function
			var capturedFunc *ir.DistilledFunction
			tempParent := &captureNode{
				onAdd: func(child ir.DistilledNode) {
					if fn, ok := child.(*ir.DistilledFunction); ok {
						capturedFunc = fn
					}
				},
			}
			p.processFunctionDefinition(child, source, file, tempParent)

			// Add template parameters to the function
			if capturedFunc != nil {
				capturedFunc.TypeParams = templateParams
				p.addChild(file, parent, capturedFunc)
			}
		case "declaration":
			p.processDeclaration(child, source, file, parent)
		}
	}
}

// extractTemplateParameters extracts template parameter names
func (p *TreeSitterProcessor) extractTemplateParameters(node *sitter.Node, source []byte) []ir.TypeParam {
	var params []ir.TypeParam
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_parameter_declaration" || child.Type() == "parameter_declaration" {
			param := ir.TypeParam{}
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(j).Type() == "type_identifier" {
					param.Name = p.nodeText(child.Child(j), source)
					params = append(params, param)
					break
				}
			}
		}
	}
	return params
}

// processFriendDeclaration handles friend declarations
func (p *TreeSitterProcessor) processFriendDeclaration(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// For now, we'll add a comment indicating a friend declaration
	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   "friend " + strings.TrimSpace(p.nodeText(node, source)),
		Format: ir.CommentLine,
	}
	p.addChild(file, parent, comment)
}

// processConceptAsComment handles C++20 concepts as special comments
func (p *TreeSitterProcessor) processConceptAsComment(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Extract the full concept definition
	conceptText := p.nodeText(node, source)

	// Create a special comment to preserve the concept
	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   "C++20 Concept: " + conceptText,
		Format: ir.CommentDoc,
	}
	p.addChild(file, parent, comment)
}

// processComment handles comments
func (p *TreeSitterProcessor) processComment(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	text := p.nodeText(node, source)
	format := ir.CommentLine
	if strings.HasPrefix(text, "/*") {
		format = ir.CommentBlock
		if strings.HasPrefix(text, "/**") || strings.Contains(text, "*/") && strings.Contains(text, "@") {
			format = ir.CommentDoc
		}
	} else if strings.HasPrefix(text, "///") {
		format = ir.CommentDoc
	}

	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   text,
		Format: format,
	}
	p.addChild(file, parent, comment)
}

// extractBaseClasses extracts base class names from inheritance clause
func (p *TreeSitterProcessor) extractBaseClasses(node *sitter.Node, source []byte) []ir.TypeRef {
	var bases []ir.TypeRef

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "type_identifier" || child.Type() == "qualified_identifier" {
			bases = append(bases, ir.TypeRef{
				Name: p.nodeText(child, source),
			})
		} else if child.Type() == "base_class_clause" {
			// Recursively extract from base_class_clause
			bases = append(bases, p.extractBaseClasses(child, source)...)
		}
	}

	return bases
}

// extractDeclaratorPrefix extracts pointer/reference prefix
func (p *TreeSitterProcessor) extractDeclaratorPrefix(node *sitter.Node, source []byte) string {
	switch node.Type() {
	case "pointer_declarator":
		return "*"
	case "reference_declarator":
		return "&"
	default:
		return ""
	}
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

// Helper methods

// nodeText extracts the text content of a node
func (p *TreeSitterProcessor) nodeText(node *sitter.Node, source []byte) string {
	return string(source[node.StartByte():node.EndByte()])
}

// nodeLocation creates a Location from a node
func (p *TreeSitterProcessor) nodeLocation(node *sitter.Node) ir.Location {
	return ir.Location{
		StartLine:   int(node.StartPoint().Row) + 1,
		EndLine:     int(node.EndPoint().Row) + 1,
		StartColumn: int(node.StartPoint().Column) + 1,
		EndColumn:   int(node.EndPoint().Column) + 1,
	}
}

// addChild adds a child node to the appropriate parent
func (p *TreeSitterProcessor) addChild(file *ir.DistilledFile, parent ir.DistilledNode, child ir.DistilledNode) {
	if parent == nil {
		file.Children = append(file.Children, child)
	} else {
		switch p := parent.(type) {
		case *ir.DistilledClass:
			p.Children = append(p.Children, child)
		case *ir.DistilledStruct:
			p.Children = append(p.Children, child)
		case *ir.DistilledPackage:
			p.Children = append(p.Children, child)
		case *ir.DistilledEnum:
			p.Children = append(p.Children, child)
		case *captureNode:
			p.onAdd(child)
		}
	}
}

// captureNode is a helper type for capturing nodes during processing
type captureNode struct {
	onAdd func(ir.DistilledNode)
}

// Implement ir.DistilledNode interface
func (c *captureNode) GetLocation() ir.Location             { return ir.Location{} }
func (c *captureNode) GetChildren() []ir.DistilledNode      { return nil }
func (c *captureNode) Accept(v ir.Visitor) ir.DistilledNode { return c }
func (c *captureNode) GetNodeKind() ir.NodeKind             { return ir.NodeKind("capture") }
func (c *captureNode) GetSymbolID() *ir.SymbolID            { return nil }
