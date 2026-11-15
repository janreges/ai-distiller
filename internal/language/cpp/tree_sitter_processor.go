package cpp

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
)

// TreeSitterProcessor uses tree-sitter for C++ parsing
type TreeSitterProcessor struct{}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	return &TreeSitterProcessor{}
}

// ProcessSource processes C++ source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// The parser lives as long as this call: the tree and every node taken
	// from it are used only here.
	parser, err := tsparse.NewParser(sitter.NewLanguage(tree_sitter_cpp.Language()))
	if err != nil {
		return nil, fmt.Errorf("failed to create C++ parser: %w", err)
	}
	defer parser.Close()

	// Parse the source code
	tree, err := tsparse.Parse(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse C++ code: %w", err)
	}
	defer tree.Close()

	// Create distilled file
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(tree.RootNode().EndPosition().Row) + 1,
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
	switch node.Kind() {
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
		if node.Kind() == "class_specifier" {
			access = ir.VisibilityPrivate
		}
		for i := 0; i < int(body.NamedChildCount()); i++ {
			member := body.NamedChild(uint(i))
			if member.Kind() == "access_specifier" {
				access = accessVisibility(p.nodeText(member, source))
				continue
			}
			p.indexMember(member, source, path, leastOpen(ceiling, access), idx)
		}
	case "compound_statement":
		return
	default:
		for i := 0; i < int(node.NamedChildCount()); i++ {
			p.indexMembers(node.NamedChild(uint(i)), source, scope, ceiling, idx)
		}
	}
}

// indexMember records one member of the class body whose class chain is path;
// vis is the member's visibility. A nested class is indexed with vis as its
// ceiling.
func (p *TreeSitterProcessor) indexMember(member *sitter.Node, source []byte, path []string, vis ir.Visibility, idx memberVisibilities) {
	switch member.Kind() {
	case "class_specifier", "struct_specifier", "union_specifier":
		p.indexMembers(member, source, path, vis, idx)
	case "template_declaration":
		for i := 0; i < int(member.NamedChildCount()); i++ {
			p.indexMember(member.NamedChild(uint(i)), source, path, vis, idx)
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
		if d.Kind() == "function_declarator" {
			if name := d.ChildByFieldName("declarator"); name != nil {
				return p.nodeText(name, source)
			}
			return ""
		}
		// reference_declarator has no "declarator" field: the wrapped
		// declarator is its last named child.
		next := d.ChildByFieldName("declarator")
		if next == nil && d.NamedChildCount() > 0 {
			next = d.NamedChild(uint(int(d.NamedChildCount()) - 1))
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
	nodeType := node.Kind()

	switch nodeType {
	case "translation_unit", "preproc_if", "preproc_ifdef", "preproc_else", "preproc_elif", "preproc_elifdef",
		"linkage_specification", "declaration_list":
		// The file, an #if/#ifdef branch (include guards) and an extern "C"
		// block hold declarations of the enclosing scope.
		p.processStatements(node, p.childList(file, parent), func(child *sitter.Node) {
			p.processNode(child, source, file, parent)
		})
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
		// A friend is not a member of the class and adds no declaration.
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
			child := node.Child(uint(i))
			p.processNode(child, source, file, parent)
		}
	}
}

// processInclude handles #include directives
func (p *TreeSitterProcessor) processInclude(node *sitter.Node, source []byte, file *ir.DistilledFile) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "system_lib_string" || child.Kind() == "string_literal" {
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
		child := node.Child(uint(i))
		if child.Kind() == "identifier" || child.Kind() == "nested_namespace_specifier" || child.Kind() == "namespace_identifier" {
			namespace.Name = p.nodeText(child, source)
			// Don't break, keep looking for the actual name
		}
	}

	// Process namespace body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "declaration_list" {
			p.processNode(child, source, file, namespace)
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
		child := node.Child(uint(i))
		if child.Kind() == "type_identifier" {
			class.Name = p.nodeText(child, source)
			break
		}
	}

	// Process base classes
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "base_class_clause" {
			class.Extends = p.extractBaseClasses(child, source)
		}
	}

	// Process class body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "field_declaration_list" {
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
		child := node.Child(uint(i))
		if child.Kind() == "type_identifier" {
			structNode.Name = p.nodeText(child, source)
			break
		}
	}

	// Process struct body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "field_declaration_list" {
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
		child := node.Child(uint(i))
		if child.Kind() == "type_identifier" {
			union.Name = p.nodeText(child, source)
			break
		}
	}

	// Process union body
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "field_declaration_list" {
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
		child := node.Child(uint(i))
		if child.Kind() == "type_identifier" {
			enum.Name = p.nodeText(child, source)
		} else if child.Kind() == "enumerator_list" {
			// Extract enum values as children
			for j := 0; j < int(child.ChildCount()); j++ {
				enumChild := child.Child(uint(j))
				if enumChild.Kind() == "enumerator" {
					for k := 0; k < int(enumChild.ChildCount()); k++ {
						if enumChild.Child(uint(k)).Kind() == "identifier" {
							// Create a field for each enum value
							field := &ir.DistilledField{
								BaseNode: ir.BaseNode{
									Location: p.nodeLocation(enumChild.Child(uint(k))),
								},
								Name:       p.nodeText(enumChild.Child(uint(k)), source),
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

	p.processStatements(node, &class.Children, func(child *sitter.Node) {
		switch child.Kind() {
		case "access_specifier":
			// Update current access level
			for j := 0; j < int(child.ChildCount()); j++ {
				accessChild := child.Child(uint(j))
				switch accessChild.Kind() {
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
			// A friend is not a member and adds no declaration.
		default:
			// Process other nodes recursively
			p.processNode(child, source, file, class)
		}
	})
}

// processStructBody processes the body of a struct
func (p *TreeSitterProcessor) processStructBody(node *sitter.Node, source []byte, file *ir.DistilledFile, structNode *ir.DistilledStruct) {
	currentVisibility := ir.VisibilityPublic // Default for struct

	p.processStatements(node, &structNode.Children, func(child *sitter.Node) {
		switch child.Kind() {
		case "access_specifier":
			// Update current access level
			for j := 0; j < int(child.ChildCount()); j++ {
				accessChild := child.Child(uint(j))
				switch accessChild.Kind() {
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
			// A friend is not a member and adds no declaration.
		default:
			// Process other nodes recursively
			p.processNode(child, source, file, structNode)
		}
	})
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
		child := node.Child(uint(i))
		switch child.Kind() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Kind() == "reference_declarator" {
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
		child := node.Child(uint(i))
		switch child.Kind() {
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
		child := node.Child(uint(i))
		switch child.Kind() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, function)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Kind() == "reference_declarator" {
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
	if node.Kind() == "reference_declarator" || node.Kind() == "pointer_declarator" {
		if function.Returns != nil {
			if node.Kind() == "reference_declarator" {
				function.Returns.Name += "&"
			} else {
				function.Returns.Name += "*"
			}
		}
		// Recurse on the wrapped declarator, which is typically the last child
		if node.ChildCount() > 0 {
			p.extractFunctionDeclarator(node.Child(uint(int(node.ChildCount())-1)), source, function)
		}
		return
	}

	// Handle plain function_declarator and others
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
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
		child := node.Child(uint(i))
		if child.Kind() == "parameter_declaration" || child.Kind() == "optional_parameter_declaration" {
			param := ir.Parameter{
				Name: "",
				Type: ir.TypeRef{},
			}

			// Extract parameter type and name
			var typeStr string
			var foundType bool
			for j := 0; j < int(child.ChildCount()); j++ {
				paramChild := child.Child(uint(j))
				switch paramChild.Kind() {
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
						if paramChild.Child(uint(k)).Kind() == "identifier" {
							param.Name = p.nodeText(paramChild.Child(uint(k)), source)
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
		child := node.Child(uint(i))
		if child.Kind() == "function_declarator" || child.Kind() == "reference_declarator" || child.Kind() == "pointer_declarator" {
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
			typeChild := node.Child(uint(j))
			switch typeChild.Kind() {
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
		declarator := node.Child(uint(functionDeclaratorIdx))
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
		child := node.Child(uint(i))
		if child.Kind() == "function_declarator" {
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
		child := node.Child(uint(i))
		if child.Kind() == "function_declarator" {
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
		child := node.Child(uint(i))
		switch child.Kind() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Kind() == "reference_declarator" {
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
		child := node.Child(uint(i))
		switch child.Kind() {
		case "function_declarator":
			p.extractFunctionDeclarator(child, source, method)
			foundDeclarator = true
		case "reference_declarator", "pointer_declarator":
			// For return types with references/pointers, we need to process them differently
			if !foundDeclarator {
				if child.Kind() == "reference_declarator" {
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
		child := node.Child(uint(i))
		switch child.Kind() {
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
				declChild := child.Child(uint(j))
				if declChild.Kind() == "identifier" {
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
		child := node.Child(uint(i))
		switch child.Kind() {
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
		child := node.Child(uint(i))
		switch child.Kind() {
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
				declChild := child.Child(uint(j))
				if declChild.Kind() == "identifier" {
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
		child := node.Child(uint(i))
		if child.Kind() == "template_parameter_list" {
			templateParams = p.extractTemplateParameters(child, source)
			break
		}
	}

	// Process the templated declaration
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
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
		child := node.Child(uint(i))
		if child.Kind() == "type_parameter_declaration" || child.Kind() == "parameter_declaration" {
			param := ir.TypeParam{}
			for j := 0; j < int(child.ChildCount()); j++ {
				if child.Child(uint(j)).Kind() == "type_identifier" {
					param.Name = p.nodeText(child.Child(uint(j)), source)
					params = append(params, param)
					break
				}
			}
		}
	}
	return params
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

// processComment adds a comment. Doxygen reads "/** */", "/*! */", "///" and
// "//!" as the doc of the next declaration (commentutil.StripDoxygen); a run of
// "///" or "//!" lines is one doc. A trailing member doc ("int x; ///< x") is
// placed in front of the declaration it documents, so it stays and goes with
// it; when the parser leaves that member out, the doc is left out too, and
// with no statement before it, it is kept as a plain comment. A file-level
// doc with @file or \file documents the file (Inner).
func (p *TreeSitterProcessor) processComment(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	raw := p.nodeText(node, source)
	text, format, trailing := commentutil.StripDoxygen(raw)
	comment := &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Text:   text,
		Format: format,
	}
	list := p.childList(file, parent)

	if trailing {
		stmt := prevStatement(node)
		if list != nil && stmt != nil {
			if i := lastDeclaration(*list); i >= 0 && declaredBy((*list)[i], stmt) {
				*list = append((*list)[:i], append([]ir.DistilledNode{comment}, (*list)[i:]...)...)
			}
			// Otherwise the member it documents is left out: it goes too.
			return
		}
		comment.Format = ir.CommentBlock
		if strings.HasPrefix(strings.TrimSpace(raw), "//") {
			comment.Format = ir.CommentLine
		}
	}

	if list != nil {
		if doc := commentutil.JoinLineDoc(*list, comment, node, source, isLineDoc); doc != nil {
			doc.Inner = doc.Inner || parent == nil && documentsFile(doc.Text)
			return
		}
	}

	if parent == nil && format == ir.CommentDoc && documentsFile(text) {
		comment.Inner = true
	}
	p.addChild(file, parent, comment)
}

// isLineDoc reports whether a raw comment is a "///" or "//!" doc line that
// documents the next declaration.
func isLineDoc(raw string) bool {
	_, format, trailing := commentutil.StripDoxygen(raw)
	return format == ir.CommentDoc && !trailing && strings.HasPrefix(raw, "//")
}

// documentsFile reports whether a doc's text has a Doxygen @file or \file
// command, which makes it the doc of the file.
func documentsFile(text string) bool {
	for _, line := range strings.Split(text, "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && (fields[0] == "@file" || fields[0] == "\\file") {
			return true
		}
	}
	return false
}

// prevStatement returns the sibling before node that is not a comment: the
// statement a trailing member doc documents, or nil.
func prevStatement(node *sitter.Node) *sitter.Node {
	stmt := node.PrevNamedSibling()
	for stmt != nil && stmt.Kind() == "comment" {
		stmt = stmt.PrevNamedSibling()
	}
	return stmt
}

// declaredBy reports whether decl starts within the lines of stmt.
func declaredBy(decl ir.DistilledNode, stmt *sitter.Node) bool {
	line := decl.GetLocation().StartLine
	return line >= int(stmt.StartPosition().Row)+1 && line <= int(stmt.EndPosition().Row)+1
}

// lastDeclaration returns the index of the last node in nodes that is not a
// comment, or -1.
func lastDeclaration(nodes []ir.DistilledNode) int {
	for i := len(nodes) - 1; i >= 0; i-- {
		if _, ok := nodes[i].(*ir.DistilledComment); !ok {
			return i
		}
	}
	return -1
}

// processStatements runs process on each child of node, a block whose
// statements add their nodes to list: the file, a namespace, class or struct
// body, an #if/#ifdef branch or an extern "C" block. The doc comments right
// before a statement that adds no declaration (a concept, a #define, a
// static_assert, a using declaration in a namespace, a friend) are dropped,
// so they do not document the next declaration (commentutil.AppendStatement).
// Only the comments inside the block are its own to drop: a doc before an
// include guard is not dropped by the guard's #define. A file doc (Inner)
// documents the file, not the next statement, so no statement drops it, nor
// the comments before it. Comments, access
// specifiers ("public:") and #pragma lines are not statements of their own,
// and the name or condition of an #if/#ifdef and the "C" of extern "C" are
// not statements at all.
func (p *TreeSitterProcessor) processStatements(node *sitter.Node, list *[]ir.DistilledNode, process func(*sitter.Node)) {
	floor := 0
	if list != nil {
		floor = len(*list)
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch {
		case isBlockHeader(node, child):
			continue
		case list == nil, !child.IsNamed(), child.Kind() == "comment",
			child.Kind() == "access_specifier", child.Kind() == "preproc_call":
			process(child)
			continue
		}
		before := len(*list)
		process(child)
		start := before
		for start > floor && isWaitingComment((*list)[start-1]) {
			start--
		}
		waiting := append([]ir.DistilledNode(nil), (*list)[start:before]...)
		added := append([]ir.DistilledNode(nil), (*list)[before:]...)
		*list = append((*list)[:start], commentutil.AppendStatement(waiting, added)...)
	}
}

// isBlockHeader reports whether child is the name or condition of an
// #if/#ifdef/#elif (node) or the linkage string of extern "C".
func isBlockHeader(node, child *sitter.Node) bool {
	for _, field := range []string{"name", "condition", "value"} {
		if h := node.ChildByFieldName(field); h != nil && h.StartByte() == child.StartByte() && h.EndByte() == child.EndByte() {
			return true
		}
	}
	return false
}

// isWaitingComment reports whether node is a comment that waits for the next
// declaration: any comment but a file doc.
func isWaitingComment(node ir.DistilledNode) bool {
	c, ok := node.(*ir.DistilledComment)
	return ok && !c.Inner
}

// extractBaseClasses extracts base class names from inheritance clause
func (p *TreeSitterProcessor) extractBaseClasses(node *sitter.Node, source []byte) []ir.TypeRef {
	var bases []ir.TypeRef

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "type_identifier" || child.Kind() == "qualified_identifier" {
			bases = append(bases, ir.TypeRef{
				Name: p.nodeText(child, source),
			})
		} else if child.Kind() == "base_class_clause" {
			// Recursively extract from base_class_clause
			bases = append(bases, p.extractBaseClasses(child, source)...)
		}
	}

	return bases
}

// extractDeclaratorPrefix extracts pointer/reference prefix
func (p *TreeSitterProcessor) extractDeclaratorPrefix(node *sitter.Node, source []byte) string {
	switch node.Kind() {
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
		StartLine:   int(node.StartPosition().Row) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		StartColumn: int(node.StartPosition().Column) + 1,
		EndColumn:   int(node.EndPosition().Column) + 1,
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

// childList returns the children list that addChild appends to for parent,
// or nil for a capture node.
func (p *TreeSitterProcessor) childList(file *ir.DistilledFile, parent ir.DistilledNode) *[]ir.DistilledNode {
	switch n := parent.(type) {
	case nil:
		return &file.Children
	case *ir.DistilledClass:
		return &n.Children
	case *ir.DistilledStruct:
		return &n.Children
	case *ir.DistilledPackage:
		return &n.Children
	case *ir.DistilledEnum:
		return &n.Children
	}
	return nil
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
