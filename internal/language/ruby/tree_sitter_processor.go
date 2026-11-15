package ruby

import (
	"context"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
)

// TreeSitterProcessor uses tree-sitter for Ruby parsing
type TreeSitterProcessor struct {
	// visibility holds the current `private`/`protected`/`public` section of
	// each class or module body being processed; the top is the active one.
	visibility []ir.Visibility
}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	return &TreeSitterProcessor{}
}

// ProcessSource processes Ruby source code using tree-sitter
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// The parser lives as long as this call: the tree and every node taken
	// from it are used only here.
	parser, err := tsparse.NewParser(sitter.NewLanguage(tree_sitter_ruby.Language()))
	if err != nil {
		return nil, fmt.Errorf("failed to create Ruby parser: %w", err)
	}
	defer parser.Close()

	// Parse the source code
	tree, err := tsparse.Parse(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Ruby code: %w", err)
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
		Language: "ruby",
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
	nodeType := node.Kind()

	switch nodeType {
	case "program":
		p.processStatements(childNodes(node), source, file, parent)
	case "class":
		p.processClass(node, source, file, parent)
	case "singleton_class":
		p.processSingletonClass(node, source, file, parent)
	case "module":
		p.processModule(node, source, file, parent)
	case "method", "singleton_method":
		p.processMethod(node, source, file, parent)
	case "assignment":
		p.processAssignment(node, source, file, parent)
	case "call":
		p.processCall(node, source, file, parent)
	case "alias":
		p.processAlias(node, source, file, parent)
	default:
		// Recursively process children
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(uint(i))
			p.processNode(child, source, file, parent)
		}
	}
}

// processClass handles class definitions
func (p *TreeSitterProcessor) processClass(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic, // Classes are public by default in Ruby
		Children:   []ir.DistilledNode{},
	}

	// Extract class name and superclass
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "constant", "scope_resolution":
			class.Name = string(source[child.StartByte():child.EndByte()])
		case "superclass":
			// Find the constant within superclass
			for j := 0; j < int(child.ChildCount()); j++ {
				superChild := child.Child(uint(j))
				if superChild.Kind() == "constant" || superChild.Kind() == "scope_resolution" {
					superclassName := string(source[superChild.StartByte():superChild.EndByte()])
					class.Extends = append(class.Extends, ir.TypeRef{Name: superclassName})
					break
				}
			}
		}
	}
	p.processClassBody(node, source, file, class)

	p.addToParent(file, parent, class)
}

// processSingletonClass handles class << self blocks
func (p *TreeSitterProcessor) processSingletonClass(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Create a special class to represent singleton methods
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:       "<<self>>",
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierStatic}, // Mark as static to indicate singleton
		Children:   []ir.DistilledNode{},
	}

	// Process singleton class body
	p.processClassBody(node, source, file, class)

	p.addToParent(file, parent, class)
}

// processModule handles module definitions
func (p *TreeSitterProcessor) processModule(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Modules are represented as interfaces in the IR
	module := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Visibility: ir.VisibilityPublic,
		Children:   []ir.DistilledNode{},
	}

	// Extract module name
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "constant", "scope_resolution":
			module.Name = string(source[child.StartByte():child.EndByte()])
		}
	}
	p.processModuleBody(node, source, file, module)

	p.addToParent(file, parent, module)
}

// processMethod handles method definitions
func (p *TreeSitterProcessor) processMethod(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	// Determine visibility (default is public for methods)
	fn.Visibility = p.getCurrentVisibility(parent)

	// Check if it's a singleton method (class method)
	if node.Kind() == "singleton_method" {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierStatic)
		// `def self.x` stays public inside a private section; only
		// `private_class_method :x` hides it (processPrivateClassMethod).
		fn.Visibility = ir.VisibilityPublic
	}

	// Extract method components
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier", "constant", "operator":
			fn.Name = string(source[child.StartByte():child.EndByte()])
		case "method_parameters", "parameters":
			p.extractParameters(child, source, fn)
		case "body_statement", "statements":
			// Extract method body if needed
			fn.Implementation = string(source[child.StartByte():child.EndByte()])
		}
	}

	// Handle special method names
	if fn.Name == "initialize" {
		fn.Name = "initialize" // Constructor
	}

	p.addToParent(file, parent, fn)
}

// processAssignment handles constant and class variable assignments
func (p *TreeSitterProcessor) processAssignment(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Look for constant assignments
	var name string
	var isConstant bool
	var isClassVar bool
	var isInstanceVar bool

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "constant":
			name = string(source[child.StartByte():child.EndByte()])
			isConstant = true
		case "class_variable":
			name = string(source[child.StartByte():child.EndByte()])
			isClassVar = true
		case "instance_variable":
			name = string(source[child.StartByte():child.EndByte()])
			isInstanceVar = true
		}
	}

	if name != "" && (isConstant || isClassVar || isInstanceVar) {
		field := &ir.DistilledField{
			BaseNode: ir.BaseNode{
				Location: p.nodeLocation(node),
			},
			Name:       name,
			Visibility: ir.VisibilityPublic,
			Modifiers:  []ir.Modifier{},
		}

		if isConstant {
			field.Modifiers = append(field.Modifiers, ir.ModifierFinal)
		}
		if isClassVar {
			field.Modifiers = append(field.Modifiers, ir.ModifierStatic)
		}

		p.addToParent(file, parent, field)
	}
}

// processCall handles method calls that might be DSL methods
func (p *TreeSitterProcessor) processCall(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Look for common metaprogramming patterns
	var methodName string
	var arguments []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "identifier" {
			methodName = string(source[child.StartByte():child.EndByte()])
		} else if child.Kind() == "argument_list" {
			// Extract arguments
			for j := 0; j < int(child.ChildCount()); j++ {
				arg := child.Child(uint(j))
				switch arg.Kind() {
				case "symbol", "simple_symbol", "string", "constant", "scope_resolution":
					arguments = append(arguments, string(source[arg.StartByte():arg.EndByte()]))
				}
			}
		}
	}

	// Keyword calls have no receiver: `YAML.load(x)` and `Foo.include Bar`
	// are ordinary method calls, not declarations. `self.include X` and
	// `self.attr_reader :x` in a class body are the keyword calls written out.
	if receiver := node.ChildByFieldName("receiver"); receiver != nil && receiver.Kind() != "self" {
		return
	}

	// Handle common metaprogramming methods
	switch methodName {
	case "require", "require_relative", "load":
		p.processRequire(methodName, node, source, file)
	case "attr_reader", "attr_writer", "attr_accessor":
		p.processAttrMethods(methodName, arguments, node, file, parent)
	case "include":
		// extend/prepend are skipped on purpose: Implements cannot carry the
		// keyword, and printing them as `include` would change the meaning.
		p.processModuleInclusion(methodName, arguments, node, file, parent)
	case "private_class_method":
		p.processPrivateClassMethod(node, file, parent, source)
	case "private", "protected", "public":
		p.processVisibilityCall(ir.Visibility(methodName), node, file, parent, source)
	case "alias_method":
		if len(arguments) >= 2 {
			p.processAliasMethod(arguments[0], arguments[1], node, file, parent)
		}
	}
}

// processAttrMethods handles attr_reader, attr_writer, attr_accessor
func (p *TreeSitterProcessor) processAttrMethods(methodName string, arguments []string, node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	for _, arg := range arguments {
		// Remove quotes and colons
		attrName := strings.Trim(arg, "\":' ")

		// Create getter
		if methodName == "attr_reader" || methodName == "attr_accessor" {
			getter := &ir.DistilledFunction{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(node),
				},
				Name:       attrName,
				Visibility: p.getCurrentVisibility(parent),
				Returns:    &ir.TypeRef{Name: "Object"},
			}
			p.addToParent(file, parent, getter)
		}

		// Create setter
		if methodName == "attr_writer" || methodName == "attr_accessor" {
			setter := &ir.DistilledFunction{
				BaseNode: ir.BaseNode{
					Location: p.nodeLocation(node),
				},
				Name:       attrName + "=",
				Visibility: p.getCurrentVisibility(parent),
				Parameters: []ir.Parameter{
					{Name: "value", Type: ir.TypeRef{Name: "Object"}},
				},
			}
			p.addToParent(file, parent, setter)
		}

		// Also create the instance variable
		field := &ir.DistilledField{
			BaseNode: ir.BaseNode{
				Location: p.nodeLocation(node),
			},
			Name:       "@" + attrName,
			Visibility: ir.VisibilityPrivate,
			Type:       &ir.TypeRef{Name: "Object"},
		}
		p.addToParent(file, parent, field)
	}
}

// processModuleInclusion records the modules of an `include` call as the
// Implements of the enclosing class. `extend` and `prepend` are not routed
// here: Implements cannot carry the keyword.
func (p *TreeSitterProcessor) processModuleInclusion(methodName string, arguments []string, node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	for _, arg := range arguments {
		ref := ir.TypeRef{Name: strings.Trim(arg, "\"' ")}
		switch n := parent.(type) {
		case *ir.DistilledClass:
			n.Implements = append(n.Implements, ref)
		case *ir.DistilledInterface:
			// A module is an interface in the IR, which has no Implements.
			n.Extends = append(n.Extends, ref)
		}
	}
}

// processVisibilityCall handles `private :a, :b` (visibility of methods seen
// so far, section unchanged) and `private def x ... end` (that method only,
// section unchanged). A bare `private` is handled by the body loops.
func (p *TreeSitterProcessor) processVisibilityCall(vis ir.Visibility, node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, source []byte) {
	args := node.ChildByFieldName("arguments")
	if args == nil {
		return
	}
	var names []string
	for i := 0; i < int(args.ChildCount()); i++ {
		arg := args.Child(uint(i))
		switch arg.Kind() {
		case "simple_symbol", "string":
			names = append(names, strings.Trim(string(source[arg.StartByte():arg.EndByte()]), "\":' "))
		case "call":
			// `private attr_reader :x`: the accessors it declares get the visibility
			var before int
			if children := childrenOf(parent); children != nil {
				before = len(*children)
			}
			p.processCall(arg, source, file, parent)
			if children := childrenOf(parent); children != nil {
				for _, c := range (*children)[before:] {
					if fn, ok := c.(*ir.DistilledFunction); ok {
						fn.Visibility = vis
					}
				}
			}
		case "method", "singleton_method":
			p.processMethod(arg, source, file, parent)
			if children := childrenOf(parent); children != nil && len(*children) > 0 {
				if fn, ok := (*children)[len(*children)-1].(*ir.DistilledFunction); ok && arg.Kind() == "method" {
					fn.Visibility = vis
				}
			}
		}
	}
	if len(names) == 0 {
		return
	}
	if children := childrenOf(parent); children != nil {
		for _, c := range *children {
			if fn, ok := c.(*ir.DistilledFunction); ok && !isSingletonMethod(fn) {
				for _, name := range names {
					if fn.Name == name {
						fn.Visibility = vis
					}
				}
			}
		}
	}
}

// processPrivateClassMethod handles `private_class_method :a, :b`: the
// singleton methods (`def self.a`) of that name seen so far become private.
func (p *TreeSitterProcessor) processPrivateClassMethod(node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode, source []byte) {
	args := node.ChildByFieldName("arguments")
	children := childrenOf(parent)
	if args == nil || children == nil {
		return
	}
	for i := 0; i < int(args.ChildCount()); i++ {
		arg := args.Child(uint(i))
		if arg.Kind() != "simple_symbol" && arg.Kind() != "string" {
			continue
		}
		name := strings.Trim(string(source[arg.StartByte():arg.EndByte()]), "\":' ")
		for _, c := range *children {
			if fn, ok := c.(*ir.DistilledFunction); ok && isSingletonMethod(fn) && fn.Name == name {
				fn.Visibility = ir.VisibilityPrivate
			}
		}
	}
}

func isSingletonMethod(fn *ir.DistilledFunction) bool {
	for _, m := range fn.Modifiers {
		if m == ir.ModifierStatic {
			return true
		}
	}
	return false
}

func childrenOf(parent ir.DistilledNode) *[]ir.DistilledNode {
	switch n := parent.(type) {
	case *ir.DistilledClass:
		return &n.Children
	case *ir.DistilledInterface:
		return &n.Children
	}
	return nil
}

// processBody walks the body of a class, module or singleton class (node)
// with a fresh `public` section.
func (p *TreeSitterProcessor) processBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.visibility = append(p.visibility, ir.VisibilityPublic)
	defer func() { p.visibility = p.visibility[:len(p.visibility)-1] }()
	p.processStatements(bodyStatements(node), source, file, parent)
}

// bodyStatements returns the statements of a class, module or singleton class
// body in source order. The grammar puts the comments between the header and
// the first statement (all comments of a body without statements) on the
// definition node itself, not on its body_statement.
func bodyStatements(node *sitter.Node) []*sitter.Node {
	var stmts []*sitter.Node
	for _, child := range childNodes(node) {
		switch child.Kind() {
		case "comment":
			stmts = append(stmts, child)
		case "body_statement", "statements":
			stmts = append(stmts, childNodes(child)...)
		}
	}
	return stmts
}

func childNodes(node *sitter.Node) []*sitter.Node {
	nodes := make([]*sitter.Node, 0, node.ChildCount())
	for i := 0; i < int(node.ChildCount()); i++ {
		nodes = append(nodes, node.Child(uint(i)))
	}
	return nodes
}

// processStatements processes the statements of the file or of a body. In a
// body, a bare `private`, `protected` or `public` (an identifier statement)
// switches the section for the definitions that follow.
//
// A run of "#" comments, each on a line of its own and on the line right
// below the one before, that ends on the line directly above a class, module
// or def is one doc comment of that definition; every other comment is a line
// comment (=begin/=end a block comment). The doc comments right before a
// statement that adds no declaration (`private_class_method def self.x`) are
// dropped, so they do not document the next declaration
// (commentutil.AppendStatement).
func (p *TreeSitterProcessor) processStatements(stmts []*sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	list := p.childList(file, parent)
	for i := 0; i < len(stmts); i++ {
		stmt := stmts[i]
		switch {
		case stmt.Kind() == "comment":
			end := commentRunEnd(stmts, i, source)
			p.processComments(stmts[i:end], end < len(stmts) && documents(stmts[i:end], stmts[end], source), source, file, parent)
			i = end - 1
			continue
		case !stmt.IsNamed():
			continue
		case len(p.visibility) > 0 && stmt.Kind() == "identifier":
			switch name := string(source[stmt.StartByte():stmt.EndByte()]); name {
			case "private", "protected", "public":
				p.visibility[len(p.visibility)-1] = ir.Visibility(name)
				continue
			}
		}
		before := len(*list)
		p.processNode(stmt, source, file, parent)
		added := append([]ir.DistilledNode(nil), (*list)[before:]...)
		*list = commentutil.AppendStatement((*list)[:before], added)
	}
}

// commentRunEnd returns the end (exclusive) of the comment run that starts at
// stmts[i]: the "#" comments on lines of their own that follow it line by
// line. Any other comment is a run of one.
func commentRunEnd(stmts []*sitter.Node, i int, source []byte) int {
	end := i + 1
	if !isOwnLineHashComment(stmts[i], source) {
		return end
	}
	for end < len(stmts) && stmts[end].Kind() == "comment" && isOwnLineHashComment(stmts[end], source) &&
		stmts[end].StartPosition().Row == stmts[end-1].EndPosition().Row+1 {
		end++
	}
	return end
}

// documents reports whether a comment run is the doc of next: a run of "#"
// lines that ends on the line directly above a class, module or def (also a
// def passed to a call, as in `private def x`).
func documents(run []*sitter.Node, next *sitter.Node, source []byte) bool {
	last := run[len(run)-1]
	if !isOwnLineHashComment(last, source) || next.StartPosition().Row != last.EndPosition().Row+1 {
		return false
	}
	switch next.Kind() {
	case "class", "module", "singleton_class", "method", "singleton_method":
		return true
	case "call":
		if args := next.ChildByFieldName("arguments"); args != nil {
			for i := 0; i < int(args.NamedChildCount()); i++ {
				if t := args.NamedChild(uint(i)).Kind(); t == "method" || t == "singleton_method" {
					return true
				}
			}
		}
	}
	return false
}

// isOwnLineHashComment reports whether a comment is a "#" comment with only
// indentation before it on its line (not a comment after code, not
// =begin/=end, not the "#!" line that starts a script).
func isOwnLineHashComment(comment *sitter.Node, source []byte) bool {
	start := int(comment.StartByte())
	if start >= len(source) || source[start] != '#' {
		return false
	}
	if start == 0 && strings.HasPrefix(string(source), "#!") {
		return false
	}
	for i := start - 1; i >= 0 && source[i] != '\n'; i-- {
		if source[i] != ' ' && source[i] != '\t' {
			return false
		}
	}
	return true
}

// processComments adds the comments of a run: one doc comment whose text
// lines are those of the run when isDoc is set, otherwise one comment per
// token.
func (p *TreeSitterProcessor) processComments(run []*sitter.Node, isDoc bool, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	if isDoc {
		lines := make([]string, len(run))
		for i, token := range run {
			lines[i], _ = stripComment(string(source[token.StartByte():token.EndByte()]))
		}
		location := p.nodeLocation(run[0])
		last := p.nodeLocation(run[len(run)-1])
		location.EndLine, location.EndColumn = last.EndLine, last.EndColumn
		p.addToParent(file, parent, &ir.DistilledComment{
			BaseNode: ir.BaseNode{Location: location},
			Text:     strings.Join(lines, "\n"),
			Format:   ir.CommentDoc,
		})
		return
	}
	for _, token := range run {
		text, format := stripComment(string(source[token.StartByte():token.EndByte()]))
		p.addToParent(file, parent, &ir.DistilledComment{
			BaseNode: ir.BaseNode{Location: p.nodeLocation(token)},
			Text:     text,
			Format:   format,
		})
	}
}

// stripComment removes the delimiters of a Ruby comment: "#" and the space
// after it (further indentation is kept), or the =begin and =end lines of a
// block comment.
func stripComment(raw string) (text, format string) {
	raw = strings.TrimRight(raw, " \t\r\n")
	if !strings.HasPrefix(raw, "=begin") {
		text = strings.TrimPrefix(strings.TrimPrefix(raw, "#"), " ")
		return text, ir.CommentLine
	}
	lines := strings.Split(raw, "\n")
	lines = lines[1:] // =begin and its optional label
	if n := len(lines); n > 0 && strings.HasPrefix(strings.TrimSpace(lines[n-1]), "=end") {
		lines = lines[:n-1]
	}
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t\r")
	}
	for len(lines) > 0 && lines[0] == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n"), ir.CommentBlock
}

// processAlias handles alias statements
func (p *TreeSitterProcessor) processAlias(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	var newName, oldName string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		if child.Kind() == "identifier" || child.Kind() == "symbol" {
			text := string(source[child.StartByte():child.EndByte()])
			if newName == "" {
				newName = strings.TrimPrefix(text, ":")
			} else if oldName == "" {
				oldName = strings.TrimPrefix(text, ":")
			}
		}
	}

	if newName != "" && oldName != "" {
		p.processAliasMethod(newName, oldName, node, file, parent)
	}
}

// processAliasMethod creates an alias method
func (p *TreeSitterProcessor) processAliasMethod(newName, oldName string, node *sitter.Node, file *ir.DistilledFile, parent ir.DistilledNode) {
	// Create a function that represents the alias
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		Name:           strings.TrimPrefix(newName, ":"),
		Visibility:     ir.VisibilityPublic,
		Implementation: fmt.Sprintf("alias for %s", oldName),
	}
	p.addToParent(file, parent, fn)
}

// processRequire records a require/require_relative/load call whose argument
// is a plain string literal ('json', "lib/x", %q(x)) as an import of that
// path. A call with any other argument (`File.expand_path(...)`, an
// interpolated string, a variable) is skipped: the path is only known at run
// time, and an import's Module is a path that formatters print quoted, so
// carrying the expression would present it as a path.
func (p *TreeSitterProcessor) processRequire(methodName string, node *sitter.Node, source []byte, file *ir.DistilledFile) {
	args := node.ChildByFieldName("arguments")
	if args == nil || args.NamedChildCount() == 0 {
		return
	}
	path, ok := plainStringLiteral(args.NamedChild(0), source)
	if !ok {
		return
	}
	file.Children = append(file.Children, &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: p.nodeLocation(node),
		},
		ImportType: methodName,
		Module:     path,
		Symbols:    []ir.ImportedSymbol{{Name: path}},
	})
}

// plainStringLiteral returns the content of node when it is a string literal
// without interpolation or escape sequences, and false otherwise.
func plainStringLiteral(node *sitter.Node, source []byte) (string, bool) {
	if node.Kind() != "string" {
		return "", false
	}
	content := ""
	for i := 0; i < int(node.NamedChildCount()); i++ {
		part := node.NamedChild(uint(i))
		if part.Kind() != "string_content" {
			return "", false
		}
		content += string(source[part.StartByte():part.EndByte()])
	}
	return content, content != ""
}

// processClassBody processes the body of a class or singleton class (node)
func (p *TreeSitterProcessor) processClassBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processBody(node, source, file, parent)
}

// processModuleBody processes the body of a module (node)
func (p *TreeSitterProcessor) processModuleBody(node *sitter.Node, source []byte, file *ir.DistilledFile, parent ir.DistilledNode) {
	p.processBody(node, source, file, parent)
}

// Helper methods

// nodeLocation converts tree-sitter node position to IR location
func (p *TreeSitterProcessor) nodeLocation(node *sitter.Node) ir.Location {
	return ir.Location{
		StartLine:   int(node.StartPosition().Row) + 1,
		StartColumn: int(node.StartPosition().Column) + 1,
		EndLine:     int(node.EndPosition().Row) + 1,
		EndColumn:   int(node.EndPosition().Column) + 1,
	}
}

// extractParameters extracts method parameters
func (p *TreeSitterProcessor) extractParameters(node *sitter.Node, source []byte, fn *ir.DistilledFunction) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(uint(i))
		switch child.Kind() {
		case "identifier":
			// Simple parameter
			param := ir.Parameter{
				Name: string(source[child.StartByte():child.EndByte()]),
				Type: ir.TypeRef{Name: "Object"}, // Ruby is dynamically typed
			}
			fn.Parameters = append(fn.Parameters, param)
		case "optional_parameter":
			// Parameter with default value
			var paramName string
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" && paramName == "" {
					paramName = string(source[subChild.StartByte():subChild.EndByte()])
				}
			}
			if paramName != "" {
				param := ir.Parameter{
					Name:         paramName,
					Type:         ir.TypeRef{Name: "Object"},
					DefaultValue: "...", // Simplified
					IsOptional:   true,
				}
				fn.Parameters = append(fn.Parameters, param)
			}
		case "keyword_parameter":
			// Keyword parameter (name:)
			var paramName string
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" {
					paramName = string(source[subChild.StartByte():subChild.EndByte()])
					break
				}
			}
			if paramName != "" {
				param := ir.Parameter{
					Name: paramName + ":",
					Type: ir.TypeRef{Name: "Object"},
				}
				fn.Parameters = append(fn.Parameters, param)
			}
		case "splat_parameter":
			// *args
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" {
					param := ir.Parameter{
						Name:       "*" + string(source[subChild.StartByte():subChild.EndByte()]),
						Type:       ir.TypeRef{Name: "Array"},
						IsVariadic: true,
					}
					fn.Parameters = append(fn.Parameters, param)
					break
				}
			}
		case "hash_splat_parameter":
			// **kwargs
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" {
					param := ir.Parameter{
						Name: "**" + string(source[subChild.StartByte():subChild.EndByte()]),
						Type: ir.TypeRef{Name: "Hash"},
					}
					fn.Parameters = append(fn.Parameters, param)
					break
				}
			}
		case "block_parameter":
			// &block
			for j := 0; j < int(child.ChildCount()); j++ {
				subChild := child.Child(uint(j))
				if subChild.Kind() == "identifier" {
					param := ir.Parameter{
						Name: "&" + string(source[subChild.StartByte():subChild.EndByte()]),
						Type: ir.TypeRef{Name: "Proc"},
					}
					fn.Parameters = append(fn.Parameters, param)
					break
				}
			}
		}
	}
}

// getCurrentVisibility determines the current visibility context
func (p *TreeSitterProcessor) getCurrentVisibility(parent ir.DistilledNode) ir.Visibility {
	if n := len(p.visibility); n > 0 {
		return p.visibility[n-1]
	}
	return ir.VisibilityPublic
}

// childList returns the list addToParent adds the children of parent to.
func (p *TreeSitterProcessor) childList(file *ir.DistilledFile, parent ir.DistilledNode) *[]ir.DistilledNode {
	if children := childrenOf(parent); children != nil {
		return children
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
		default:
			file.Children = append(file.Children, child)
		}
	} else {
		file.Children = append(file.Children, child)
	}
}
