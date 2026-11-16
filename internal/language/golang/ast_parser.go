package golang

import (
	"bytes"
	"context"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
)

// ASTParser uses Go's native AST parser for processing Go source code
type ASTParser struct {
	source   []byte
	filename string
	fset     *token.FileSet
	// comments are the file's comment groups; claimed are the comments a
	// struct or interface body took as its children (memberNodes), which
	// processComments does not add at file level.
	comments []*ast.CommentGroup
	claimed  map[*ast.Comment]bool
	// verbatim are the spans of source that print as written (a value or
	// type the parser copies from the source): a comment inside one prints
	// with that text (innerComments).
	verbatim []span
}

// span is a stretch of source, from start to end.
type span struct{ start, end token.Pos }

// sourceText returns the source from start to end, and records it as
// printed verbatim.
func (p *ASTParser) sourceText(start, end token.Pos) (string, bool) {
	from := p.fset.Position(start).Offset
	to := p.fset.Position(end).Offset
	if from < 0 || to > len(p.source) || from >= to {
		return "", false
	}
	p.verbatim = append(p.verbatim, span{start, end})
	return string(p.source[from:to]), true
}

// signatureAsWritten returns the source from start to end, a signature,
// without trailing blanks, and its lines after the first without the
// indentation of the line it starts on, so that they keep only their
// indentation relative to it, as the text indents the signature's first
// line by its own nesting.
func (p *ASTParser) signatureAsWritten(start, end token.Pos) string {
	text, ok := p.sourceText(start, end)
	if !ok {
		return ""
	}
	from := p.fset.Position(start).Offset
	indent := string(p.source[bytes.LastIndexByte(p.source[:from], '\n')+1 : from])
	if strings.TrimLeft(indent, " \t") != "" {
		indent = "" // something else is written before start on its line
	}
	lines := strings.Split(strings.TrimRight(text, " \t"), "\n")
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.TrimPrefix(lines[i], indent)
	}
	return strings.Join(lines, "\n")
}

// innerComments returns, as line comments, the comment groups that start
// between start and end, inside a declaration, that no node took yet and
// that no verbatim text holds: those inside a part the output rebuilds (a
// signature, a type) or elides (a value printed as {...}). They are
// claimed. The caller puts them right before the declaration's node, inside
// its span, so that the stripper drops them with it and with --comments=0,
// and the text formatter prints them with it.
func (p *ASTParser) innerComments(start, end token.Pos) []ir.DistilledNode {
	var nodes []ir.DistilledNode
	for _, group := range p.comments {
		if group.Pos() <= start || group.Pos() >= end || p.claimed[group.List[0]] || p.heldVerbatim(group.Pos()) {
			continue
		}
		nodes = p.appendCommentGroup(nodes, group)
		p.claim(group)
	}
	return nodes
}

// heldVerbatim reports whether pos lies inside a span printed verbatim.
func (p *ASTParser) heldVerbatim(pos token.Pos) bool {
	for _, s := range p.verbatim {
		if s.start < pos && pos < s.end {
			return true
		}
	}
	return false
}

// NewASTParser creates a new AST parser
func NewASTParser() *ASTParser {
	return &ASTParser{
		fset: token.NewFileSet(),
	}
}

// ProcessSource processes Go source code using go/parser
func (p *ASTParser) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("golang:ast")
	defer dbg.Timing(debug.LevelDetailed, "AST parsing")()

	p.source = source
	p.filename = filename

	dbg.Logf(debug.LevelDetailed, "Parsing %d bytes with go/parser", len(source))

	// Parse the source code
	file, err := parser.ParseFile(p.fset, filename, source, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	p.comments = file.Comments
	p.claimed = make(map[*ast.Comment]bool)
	p.verbatim = nil

	// Dump raw AST at trace level
	debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
		astDump := p.dumpAST(file)
		d.Dump(debug.LevelTrace, "Raw Go AST", astDump)
	})

	// Create distilled file
	distilledFile := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(file.Pos(), file.End()),
		},
		Path:     filename,
		Language: "go",
		Version:  "1.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Extract build constraints from comments
	buildConstraints := p.extractBuildConstraints(file.Comments)
	if len(buildConstraints) > 0 {
		// Add build constraints as a comment at the top, where the first
		// constraint line is (file.Pos() is the package clause)
		at := file.Pos()
		if c := firstBuildConstraint(file.Comments); c != nil {
			at = c.Pos()
		}
		constraintComment := &ir.DistilledComment{
			BaseNode: ir.BaseNode{
				Location: p.getLocation(at, at),
			},
			Text: fmt.Sprintf("@build_constraint(%s)", strings.Join(buildConstraints, ", ")),
		}
		distilledFile.Children = append(distilledFile.Children, constraintComment)
	}

	// Process package declaration with doc comments
	if file.Doc != nil {
		// Add package documentation comments
		for _, comment := range file.Doc.List {
			docComment := &ir.DistilledComment{
				BaseNode: ir.BaseNode{
					Location: p.getLocation(comment.Pos(), comment.End()),
				},
				Text: packageDocText(comment),
			}
			distilledFile.Children = append(distilledFile.Children, docComment)
		}
	}

	// Process package declaration
	if file.Name != nil {
		pkg := &ir.DistilledPackage{
			BaseNode: ir.BaseNode{
				Location: p.getLocation(file.Name.Pos(), file.Name.End()),
			},
			Name: file.Name.Name,
		}
		distilledFile.Children = append(distilledFile.Children, pkg)
	}

	// Single-pass processing to preserve declaration order
	typeMap := make(map[string]ir.DistilledNode) // Map from type name to the distilled node

	for _, decl := range file.Decls {
		if d, ok := decl.(*ast.GenDecl); ok && d.Tok == token.IMPORT {
			// Imports, with their comments
			distilledFile.Children = append(distilledFile.Children, p.importNodes(d)...)
			continue
		}
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Extract function doc comments
			var docComments []ir.DistilledNode
			if d.Doc != nil {
				for _, comment := range d.Doc.List {
					text := commentText(comment)
					if text != "" {
						docComment := &ir.DistilledComment{
							BaseNode: ir.BaseNode{
								Location: p.getLocation(comment.Pos(), comment.End()),
							},
							Text:   text,
							Format: ir.CommentDoc,
						}
						docComments = append(docComments, docComment)
					}
				}
			}
			fn := p.processFunction(d)
			if fn != nil {
				// Add all functions (methods and regular functions) as top-level in original order
				// The formatter will handle method display correctly
				for _, doc := range docComments {
					distilledFile.Children = append(distilledFile.Children, doc)
				}
				// The comments inside the signature or between it and the
				// body, which the text prints as written while they are
				// kept
				end := d.Type.End()
				if d.Body != nil {
					end = d.Body.Lbrace
				}
				inner := p.innerComments(d.Pos(), end)
				if len(inner) > 0 {
					fn.SourceSignature = p.signatureAsWritten(d.Pos(), end)
				}
				distilledFile.Children = append(distilledFile.Children, inner...)
				distilledFile.Children = append(distilledFile.Children, fn)
			}
		case *ast.GenDecl:
			nodes := p.processGenDecl(d)
			for _, node := range nodes {
				// Add all nodes directly in original order
				if class, ok := node.(*ir.DistilledClass); ok {
					typeMap[class.Name] = class
					distilledFile.Children = append(distilledFile.Children, node)
				} else if intf, ok := node.(*ir.DistilledInterface); ok {
					typeMap[intf.Name] = intf
					distilledFile.Children = append(distilledFile.Children, node)
				} else {
					// Constants, variables, type aliases - add directly
					distilledFile.Children = append(distilledFile.Children, node)
				}
			}
		}
	}

	// Third pass: analyze interface satisfaction and add "implements" relationships
	p.analyzeInterfaceSatisfaction(typeMap)

	// Fourth pass: process remaining comments
	p.claimDeclComments(file)
	p.processComments(file, distilledFile)

	// Log summary at detailed level
	dbg.Logf(debug.LevelDetailed, "Processed Go file: %d children, %d types in typeMap",
		len(distilledFile.Children), len(typeMap))

	// Dump final structure at trace level
	debug.Lazy(ctx, debug.LevelTrace, func(d debug.Debugger) {
		d.Dump(debug.LevelTrace, "Final Go IR structure", distilledFile)
	})

	return distilledFile, nil
}

// analyzeInterfaceSatisfaction determines which structs implement which interfaces
func (p *ASTParser) analyzeInterfaceSatisfaction(typeMap map[string]ir.DistilledNode) {
	// Collect all interfaces and their method sets
	interfaces := make(map[string]*ir.DistilledInterface)
	for _, node := range typeMap {
		if intf, ok := node.(*ir.DistilledInterface); ok {
			interfaces[intf.Name] = intf
		}
	}

	// Check each struct/class against each interface
	for _, node := range typeMap {
		if class, ok := node.(*ir.DistilledClass); ok {
			// Build method set for this class
			classMethods := p.buildMethodSet(class)

			// Check against each interface
			for intfName, intf := range interfaces {
				if p.satisfiesInterface(classMethods, intf) {
					// Add implements relationship
					class.Implements = append(class.Implements, ir.TypeRef{Name: intfName})
				}
			}
		}
	}
}

// buildMethodSet creates a map of method signatures for a class
func (p *ASTParser) buildMethodSet(class *ir.DistilledClass) map[string]*ir.DistilledFunction {
	methods := make(map[string]*ir.DistilledFunction)
	for _, child := range class.Children {
		if fn, ok := child.(*ir.DistilledFunction); ok {
			signature := p.getMethodSignature(fn)
			methods[signature] = fn
		}
	}
	return methods
}

// satisfiesInterface checks if a class method set satisfies an interface
func (p *ASTParser) satisfiesInterface(classMethods map[string]*ir.DistilledFunction, intf *ir.DistilledInterface) bool {
	// Check that all interface methods are implemented
	for _, child := range intf.Children {
		if intfMethod, ok := child.(*ir.DistilledFunction); ok {
			signature := p.getMethodSignature(intfMethod)
			if _, exists := classMethods[signature]; !exists {
				return false
			}
		}
	}
	return true
}

// getMethodSignature creates a string representation of a method signature
func (p *ASTParser) getMethodSignature(fn *ir.DistilledFunction) string {
	var sig strings.Builder
	sig.WriteString(fn.Name)
	sig.WriteString("(")

	// Add parameter types
	for i, param := range fn.Parameters {
		if i > 0 {
			sig.WriteString(", ")
		}
		sig.WriteString(param.Type.Name)
	}
	sig.WriteString(")")

	// Add return type
	if fn.Returns != nil {
		sig.WriteString(" -> ")
		sig.WriteString(fn.Returns.Name)
	}

	return sig.String()
}

// processComments adds all comments to the distilled file in the correct positions
func (p *ASTParser) processComments(file *ast.File, distilledFile *ir.DistilledFile) {
	// This function is now called after all other processing is done,
	// so we just need to handle inline and trailing comments that weren't
	// already processed as doc comments

	// The comments of struct and interface bodies are children of their type
	processedComments := make(map[*ast.Comment]bool, len(p.claimed))
	for c := range p.claimed {
		processedComments[c] = true
	}

	// Mark package doc comments as processed (already added)
	if file.Doc != nil {
		for _, c := range file.Doc.List {
			processedComments[c] = true
		}
	}

	// Mark declaration doc comments as processed
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc != nil {
				for _, c := range d.Doc.List {
					processedComments[c] = true
				}
			}
		case *ast.GenDecl:
			if d.Doc != nil {
				for _, c := range d.Doc.List {
					processedComments[c] = true
				}
			}
			// Also check specs within GenDecl: processGenDecl added their
			// docs and the comments at the end of their lines
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					if s.Doc != nil {
						for _, c := range s.Doc.List {
							processedComments[c] = true
						}
					}
					if s.Comment != nil {
						for _, c := range s.Comment.List {
							processedComments[c] = true
						}
					}
				case *ast.TypeSpec:
					if s.Doc != nil {
						for _, c := range s.Doc.List {
							processedComments[c] = true
						}
					}
					if s.Comment != nil {
						for _, c := range s.Comment.List {
							processedComments[c] = true
						}
					}
				}
			}
		}
	}

	// Now add inline and line comments that haven't been processed
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if !processedComments[c] {
				// This is an inline or line comment
				text := commentText(c)
				if text != "" {
					distilledComment := &ir.DistilledComment{
						BaseNode: ir.BaseNode{
							Location: p.getLocation(c.Pos(), c.End()),
						},
						Text:   text,
						Format: ir.CommentLine,
					}
					// Insert comment at the appropriate position based on line number
					p.insertCommentAtPosition(distilledFile, distilledComment)
				}
			}
		}
	}
}

// claimDeclComments claims the comments inside a declaration that no node
// took, so that processComments does not add them at file level, where they
// would read as comments on the next declaration and stay when theirs is
// removed: those in a function's body (the body keeps them verbatim and
// prints them with --implementation=1) and those in text that prints
// verbatim (a function literal value). The comments inside a signature, a
// type or a value printed as {...} are nodes already (innerComments). The
// doc above a declaration and the comment at the end of a spec's line lie
// outside these spans.
func (p *ASTParser) claimDeclComments(file *ast.File) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			p.claimWithin(d.Pos(), d.End())
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				p.claimWithin(spec.Pos(), spec.End())
			}
		}
	}
}

// claimWithin claims the comment groups that start between start and end.
func (p *ASTParser) claimWithin(start, end token.Pos) {
	for _, group := range p.comments {
		if start < group.Pos() && group.Pos() < end {
			for _, c := range group.List {
				p.claimed[c] = true
			}
		}
	}
}

// insertCommentAtPosition inserts a comment at the correct position based on
// its location. A one-line comment that starts on the last line of the node
// before it, after that node (the comment at the end of a one-line
// function's line, or of the package clause), goes right before that node,
// inside its line span, like the comment at the end of a spec's line: the
// text prints it at the end of the node's line, and the stripper drops it
// with the node.
func (p *ASTParser) insertCommentAtPosition(file *ir.DistilledFile, comment *ir.DistilledComment) {
	// Find the appropriate position to insert the comment based on line numbers
	commentLine := comment.Location.StartLine

	// Find the right position to insert
	insertIndex := len(file.Children)
	for i, child := range file.Children {
		childLine := child.GetLocation().StartLine
		if childLine > commentLine {
			insertIndex = i
			break
		}
	}
	if insertIndex > 0 && comment.Location.EndLine == commentLine {
		before := file.Children[insertIndex-1]
		at := before.GetLocation()
		if before.GetNodeKind() != ir.KindComment && at.EndLine == commentLine &&
			comment.Location.StartColumn >= at.EndColumn {
			insertIndex--
		}
	}

	// Insert at the appropriate position
	if insertIndex == len(file.Children) {
		file.Children = append(file.Children, comment)
	} else {
		// Insert in the middle
		file.Children = append(file.Children[:insertIndex+1], file.Children[insertIndex:]...)
		file.Children[insertIndex] = comment
	}
}

// firstBuildConstraint returns the first "//go:build" or "// +build" line
// comment, or nil.
func firstBuildConstraint(comments []*ast.CommentGroup) *ast.Comment {
	for _, group := range comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:build ") || strings.HasPrefix(c.Text, "// +build ") {
				return c
			}
		}
	}
	return nil
}

// extractBuildConstraints extracts build constraints from comments
func (p *ASTParser) extractBuildConstraints(comments []*ast.CommentGroup) []string {
	var constraints []string

	for _, group := range comments {
		for _, comment := range group.List {
			text := comment.Text

			// Check for //go:build directive
			if strings.HasPrefix(text, "//go:build ") {
				constraint := strings.TrimSpace(text[10:]) // Remove "//go:build "
				constraints = append(constraints, constraint)
			}

			// Check for legacy // +build directive
			if strings.HasPrefix(text, "// +build ") {
				constraint := strings.TrimSpace(text[9:]) // Remove "// +build "
				constraints = append(constraints, constraint)
			}
		}
	}

	return constraints
}

// importNodes returns the imports of an import declaration with their
// comments, all of them line comments: the declaration's doc (the group that
// ends on the line directly above "import"), marked as such (ImportDoc),
// then per import the comment groups before it, and those inside it and the
// comment at the end of its line, which go right before it, inside its span
// (as in memberNodes), so that the text prints them at the end of the
// import's line. A comment group after the last import of a block ("import
// (" ... ")") is marked ImportBlockEnd: the text prints it before ")", where
// it reads as a comment on the imports, not on the next declaration. All of
// them are claimed. Any other comment before the declaration, such as a file
// header set off by a blank line, is a free comment (processComments).
func (p *ASTParser) importNodes(decl *ast.GenDecl) []ir.DistilledNode {
	nodes := p.appendCommentGroup(nil, decl.Doc)
	if len(nodes) > 0 {
		nodes[0].(*ir.DistilledComment).Extensions = &ir.NodeExtensions{Go: &ir.GoExtensions{ImportDoc: true}}
	}
	p.claim(decl.Doc)
	var inBlock []*ast.CommentGroup // the groups between "(" and ")", in order
	if decl.Lparen.IsValid() {
		for _, group := range p.comments {
			if decl.Lparen < group.Pos() && group.Pos() < decl.Rparen {
				inBlock = append(inBlock, group)
				p.claim(group)
			}
		}
	}
	for _, s := range decl.Specs {
		spec := s.(*ast.ImportSpec)
		for len(inBlock) > 0 && inBlock[0].Pos() < spec.Pos() {
			nodes = p.appendCommentGroup(nodes, inBlock[0])
			inBlock = inBlock[1:]
		}
		// The comments inside the spec (between its name and its path)
		for len(inBlock) > 0 && inBlock[0].Pos() < spec.End() {
			nodes = p.appendCommentGroup(nodes, inBlock[0])
			inBlock = inBlock[1:]
		}
		nodes = append(nodes, p.innerComments(spec.Pos(), spec.End())...)
		if spec.Comment != nil {
			for len(inBlock) > 0 && inBlock[0].Pos() <= spec.Comment.Pos() {
				inBlock = inBlock[1:]
			}
			nodes = p.appendCommentGroup(nodes, spec.Comment)
			p.claim(spec.Comment)
		}
		nodes = append(nodes, p.processImport(spec))
	}
	for _, group := range inBlock {
		at := len(nodes)
		nodes = p.appendCommentGroup(nodes, group)
		for _, n := range nodes[at:] {
			n.(*ir.DistilledComment).Extensions = &ir.NodeExtensions{Go: &ir.GoExtensions{ImportBlockEnd: true}}
		}
	}
	return nodes
}

// claim claims the comments of group, so that processComments does not add
// them at file level. A nil group claims nothing.
func (p *ASTParser) claim(group *ast.CommentGroup) {
	if group == nil {
		return
	}
	for _, c := range group.List {
		p.claimed[c] = true
	}
}

// processImport processes an import specification
func (p *ASTParser) processImport(spec *ast.ImportSpec) *ir.DistilledImport {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(spec.Pos(), spec.End()),
		},
		ImportType: "import",
		Symbols:    []ir.ImportedSymbol{},
	}

	// Get the import path without quotes
	path := strings.Trim(spec.Path.Value, `"`)
	imp.Module = path

	// Handle aliased imports
	if spec.Name != nil {
		alias := spec.Name.Name
		if alias == "." {
			// Dot import
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  ".",
				Alias: "",
			})
		} else if alias == "_" {
			// Blank import
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  "_",
				Alias: "",
			})
		} else {
			// Named import
			imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
				Name:  path,
				Alias: alias,
			})
		}
	} else {
		// Standard import: use the last part of the path as the name
		parts := strings.Split(path, "/")
		pkgName := parts[len(parts)-1]
		// Handle versioned paths like "gopkg.in/yaml.v2"
		if strings.HasPrefix(pkgName, "v") && len(pkgName) <= 3 && len(parts) > 1 {
			pkgName = parts[len(parts)-2]
		}
		imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
			Name: pkgName,
		})
	}

	return imp
}

// processGenDecl processes a general declaration (type, const, var)
func (p *ASTParser) processGenDecl(decl *ast.GenDecl) []ir.DistilledNode {
	var nodes []ir.DistilledNode

	// Process declaration-level doc comments
	if decl.Doc != nil {
		for _, comment := range decl.Doc.List {
			text := commentText(comment)
			if text != "" {
				docComment := &ir.DistilledComment{
					BaseNode: ir.BaseNode{
						Location: p.getLocation(comment.Pos(), comment.End()),
					},
					Text:   text,
					Format: ir.CommentDoc,
				}
				nodes = append(nodes, docComment)
			}
		}
	}

	for _, spec := range decl.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			// Add type doc comment if present
			if s.Doc != nil {
				for _, comment := range s.Doc.List {
					text := commentText(comment)
					if text != "" {
						docComment := &ir.DistilledComment{
							BaseNode: ir.BaseNode{
								Location: p.getLocation(comment.Pos(), comment.End()),
							},
							Text:   text,
							Format: ir.CommentDoc,
						}
						nodes = append(nodes, docComment)
					}
				}
			}
			node := p.processTypeSpec(s)
			if node != nil {
				// The comments inside the spec that no member took (in a
				// rebuilt type), then the comment at the end of the spec's
				// line go right before it, inside its span, as in
				// memberNodes
				nodes = append(nodes, p.innerComments(s.Pos(), s.End())...)
				nodes = p.appendCommentGroup(nodes, s.Comment)
				nodes = append(nodes, node)
			}
		case *ast.ValueSpec:
			// Add value doc comment if present
			if s.Doc != nil {
				for _, comment := range s.Doc.List {
					text := commentText(comment)
					if text != "" {
						docComment := &ir.DistilledComment{
							BaseNode: ir.BaseNode{
								Location: p.getLocation(comment.Pos(), comment.End()),
							},
							Text:   text,
							Format: ir.CommentDoc,
						}
						nodes = append(nodes, docComment)
					}
				}
			}
			// Process const/var declarations; each spans from its name to
			// the end of the spec. The comments inside the spec that no
			// verbatim value holds (in a value printed as {...}, in its
			// type) go right before their const or var, inside its span:
			// those in a value before the name it is assigned to, the
			// others, then the comment at the end of the spec's line,
			// before the last one
			for i, name := range s.Names {
				field := &ir.DistilledField{
					BaseNode: ir.BaseNode{
						Location: p.getLocation(name.Pos(), s.End()),
					},
					Name:       name.Name,
					Visibility: p.getVisibility(name.Name),
					Modifiers:  []ir.Modifier{},
				}

				// Determine if const or var
				if decl.Tok == token.CONST {
					field.Modifiers = append(field.Modifiers, ir.ModifierFinal)
				}

				// Get type if specified
				if s.Type != nil {
					field.Type = &ir.TypeRef{Name: p.typeToString(s.Type)}
				}

				// Get value if specified
				if s.Values != nil && i < len(s.Values) {
					field.DefaultValue = p.exprToString(s.Values[i])
				}

				if i < len(s.Names)-1 {
					if i < len(s.Values) {
						nodes = append(nodes, p.innerComments(s.Values[i].Pos(), s.Values[i].End())...)
					}
				} else {
					nodes = append(nodes, p.innerComments(s.Pos(), s.End())...)
					nodes = p.appendCommentGroup(nodes, s.Comment)
				}
				nodes = append(nodes, field)
			}
		}
	}

	return nodes
}

// processTypeSpec processes a type specification
func (p *ASTParser) processTypeSpec(spec *ast.TypeSpec) ir.DistilledNode {
	// Extract type parameters if present
	var typeParams string
	if spec.TypeParams != nil {
		var params []string
		for _, field := range spec.TypeParams.List {
			for _, name := range field.Names {
				paramStr := name.Name
				if field.Type != nil {
					paramStr += " " + p.typeToString(field.Type)
				}
				params = append(params, paramStr)
			}
		}
		if len(params) > 0 {
			typeParams = "[" + strings.Join(params, ", ") + "]"
		}
	}

	// A defined type (type Named int) or, with "=", an alias (type Alias =
	// int), which carries the modifier type_alias, as does an alias of a
	// struct or interface literal (type Point = struct{ X int }).
	switch t := spec.Type.(type) {
	case *ast.StructType:
		class := p.processStructWithParams(spec.Name.Name, typeParams, t)
		if spec.Assign.IsValid() {
			class.Modifiers = append(class.Modifiers, ir.ModifierTypeAlias)
		}
		return class
	case *ast.InterfaceType:
		intf := p.processInterfaceWithParams(spec.Name.Name, typeParams, t)
		if spec.Assign.IsValid() {
			intf.Modifiers = append(intf.Modifiers, ir.ModifierTypeAlias)
		}
		return intf
	default:
		alias := &ir.DistilledTypeAlias{
			BaseNode: ir.BaseNode{
				Location: p.getLocation(spec.Pos(), spec.End()),
			},
			Name:       spec.Name.Name + typeParams,
			Visibility: p.getVisibility(spec.Name.Name),
			Type:       ir.TypeRef{Name: p.typeToString(spec.Type)},
		}
		if spec.Assign.IsValid() {
			alias.Modifiers = []ir.Modifier{ir.ModifierTypeAlias}
		}
		return alias
	}
}

// processStructWithParams processes a struct type with optional type parameters
func (p *ASTParser) processStructWithParams(name, typeParams string, structType *ast.StructType) *ir.DistilledClass {
	return p.processStruct(name+typeParams, structType)
}

// processInterfaceWithParams processes an interface type with optional type parameters
func (p *ASTParser) processInterfaceWithParams(name, typeParams string, interfaceType *ast.InterfaceType) *ir.DistilledInterface {
	return p.processInterface(name+typeParams, interfaceType)
}

// processStruct processes a struct type. Its children are its fields and
// the comments of its body, in source order (memberNodes).
func (p *ASTParser) processStruct(name string, structType *ast.StructType) *ir.DistilledClass {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(structType.Pos(), structType.End()),
		},
		Name:       name,
		Visibility: p.getVisibility(name),
		Modifiers:  []ir.Modifier{},
	}

	class.Children = p.memberNodes(structType.Fields, func(field *ast.Field) []ir.DistilledNode {
		fieldType := p.typeToString(field.Type)
		if len(field.Names) == 0 {
			// Embedded field, named after its type
			return []ir.DistilledNode{&ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.getLocation(field.Pos(), field.End()),
				},
				Name:       fieldType,
				Type:       &ir.TypeRef{Name: fieldType},
				Visibility: ir.VisibilityPublic,
				Modifiers:  []ir.Modifier{ir.ModifierEmbedded},
			}}
		}
		// Named fields; each spans from its name to the end of the field
		var nodes []ir.DistilledNode
		for _, name := range field.Names {
			// TODO: Process tags when annotation support is added
			nodes = append(nodes, &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.getLocation(name.Pos(), field.End()),
				},
				Name:       name.Name,
				Type:       &ir.TypeRef{Name: fieldType},
				Visibility: p.getVisibility(name.Name),
			})
		}
		return nodes
	})

	return class
}

// processInterface processes an interface type. Each embedded element (an
// embedded interface such as io.Reader, or a type set such as ~int | ~string)
// is one entry of Extends, as written, with its location in ExtendsAt; the
// children are the methods and the comments of the body, in source order
// (memberNodes), an embedded element's doc and line-end comment included.
func (p *ASTParser) processInterface(name string, interfaceType *ast.InterfaceType) *ir.DistilledInterface {
	intf := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(interfaceType.Pos(), interfaceType.End()),
		},
		Name:       name,
		Visibility: p.getVisibility(name),
	}

	intf.Children = p.memberNodes(interfaceType.Methods, func(method *ast.Field) []ir.DistilledNode {
		if len(method.Names) == 0 {
			// Embedded element: one entry, a union stays whole
			intf.Extends = append(intf.Extends, ir.TypeRef{Name: p.typeToString(method.Type)})
			intf.ExtendsAt = append(intf.ExtendsAt, p.getLocation(method.Pos(), method.End()))
			return nil
		}
		var nodes []ir.DistilledNode
		for _, name := range method.Names {
			if funcType, ok := method.Type.(*ast.FuncType); ok {
				fn := p.processFuncType(name.Name, funcType)
				fn.Location = p.getLocation(name.Pos(), method.End())
				fn.Visibility = ir.VisibilityPublic
				nodes = append(nodes, fn)
			}
		}
		return nodes
	})

	return intf
}

// memberNodes returns the children of a struct or interface body: for each
// field (a struct field, an interface method or embedded element) in source
// order, the comment group above it (its doc), the nodes build returns for
// it, and the comment group at the end of its line. That trailing comment is
// put right before the field's last node, inside that node's line span, so
// that the stripper drops it with the node; the text formatter prints it at
// the end of the node's line. The other comment groups of the body stay
// where they are. A comment inside a field (in a nested struct type, which
// prints as struct{...}, or in an interface method's parameters) goes
// before the field's last node too, before the one at the end of its line
// (innerComments); an interface method with one gets its SourceSignature.
// All of them are claimed, so that processComments does not add them at
// file level. Every comment is a line comment: it follows --comments.
func (p *ASTParser) memberNodes(list *ast.FieldList, build func(*ast.Field) []ir.DistilledNode) []ir.DistilledNode {
	nodes := []ir.DistilledNode{}
	if list == nil {
		return nodes
	}
	attached := make(map[*ast.CommentGroup]bool)
	for _, field := range list.List {
		attached[field.Doc] = true
		attached[field.Comment] = true
	}
	var free []*ast.CommentGroup // groups attached to no field, in order
	for _, group := range p.comments {
		if group.Pos() <= list.Opening || group.Pos() >= list.Closing || insideField(list, group.Pos()) {
			continue
		}
		p.claim(group)
		if !attached[group] {
			free = append(free, group)
		}
	}

	next := 0
	freeBefore := func(pos token.Pos) {
		for next < len(free) && free[next].Pos() < pos {
			nodes = p.appendCommentGroup(nodes, free[next])
			next++
		}
	}
	for _, field := range list.List {
		start := field.Pos()
		if field.Doc != nil {
			start = field.Doc.Pos()
		}
		freeBefore(start)
		nodes = p.appendCommentGroup(nodes, field.Doc)
		members := build(field)
		inner := p.innerComments(field.Pos(), field.End())
		if len(members) == 0 {
			nodes = append(nodes, inner...)
			nodes = p.appendCommentGroup(nodes, field.Comment)
			continue
		}
		last := len(members) - 1
		if fn, ok := members[last].(*ir.DistilledFunction); ok && len(inner) > 0 {
			// An interface method prints as written while they are kept
			fn.SourceSignature = p.signatureAsWritten(field.Pos(), field.End())
		}
		nodes = append(nodes, members[:last]...)
		nodes = append(nodes, inner...)
		nodes = p.appendCommentGroup(nodes, field.Comment)
		nodes = append(nodes, members[last])
	}
	freeBefore(list.Closing)
	return nodes
}

// insideField reports whether pos lies within one of the fields of list,
// between its first name (or type) and its end.
func insideField(list *ast.FieldList, pos token.Pos) bool {
	for _, field := range list.List {
		if field.Pos() <= pos && pos < field.End() {
			return true
		}
	}
	return false
}

// appendCommentGroup appends a comment group as one line comment whose
// location spans the group and whose text has one line per comment line
// (an empty "//" line stays as an empty line). A nil or empty group adds
// nothing.
func (p *ASTParser) appendCommentGroup(nodes []ir.DistilledNode, group *ast.CommentGroup) []ir.DistilledNode {
	if group == nil {
		return nodes
	}
	lines := make([]string, 0, len(group.List))
	for _, c := range group.List {
		lines = append(lines, commentText(c))
	}
	text := strings.Trim(strings.Join(lines, "\n"), "\n")
	if text == "" {
		return nodes
	}
	return append(nodes, &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(group.Pos(), group.End()),
		},
		Text:   text,
		Format: ir.CommentLine,
	})
}

// commentText returns the text of a comment without its markers: "//" and
// the spaces around the text of a line comment, "/*" and "*/" of a block
// comment (blockCommentText).
func commentText(c *ast.Comment) string {
	if strings.HasPrefix(c.Text, "/*") && strings.HasSuffix(c.Text, "*/") {
		return blockCommentText(c.Text[2 : len(c.Text)-2])
	}
	return strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
}

// blockCommentText returns the text between "/*" and "*/" with one line per
// source line: the first without the spaces around it, the following ones
// without the indentation they all share (the comment's indentation in the
// source, which the formatter replaces with its own) and without trailing
// spaces; indentation beyond the shared one (a code example) stays. Blank
// lines at the start and the end go.
func blockCommentText(text string) string {
	lines := strings.Split(text, "\n")
	lines[0] = strings.TrimSpace(lines[0])
	shared, found := "", false
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if !found {
			shared, found = indent, true
			continue
		}
		for !strings.HasPrefix(indent, shared) {
			shared = shared[:len(shared)-1]
		}
	}
	for i := 1; i < len(lines); i++ {
		lines[i] = strings.TrimRight(strings.TrimPrefix(lines[i], shared), " \t")
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = ""
		}
	}
	return strings.Trim(strings.Join(lines, "\n"), "\n")
}

// packageDocText returns the text of a line of the package doc without "//"
// and the one space after it, which the formatter prints again; further
// indentation (a list item, a code block) stays. A block comment loses its
// markers (commentText).
func packageDocText(c *ast.Comment) string {
	if !strings.HasPrefix(c.Text, "//") {
		return commentText(c)
	}
	text := strings.TrimPrefix(c.Text, "//")
	return strings.TrimRight(strings.TrimPrefix(text, " "), " \t")
}

// processFunction processes a function declaration
func (p *ASTParser) processFunction(fn *ast.FuncDecl) *ir.DistilledFunction {
	distilledFn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(fn.Pos(), fn.End()),
		},
		Name:       fn.Name.Name,
		Visibility: p.getVisibility(fn.Name.Name),
		Modifiers:  []ir.Modifier{},
		Parameters: []ir.Parameter{},
	}

	// Check if it's a method (has receiver)
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv := fn.Recv.List[0]
		recvType := p.typeToString(recv.Type)

		// Add receiver as special parameter
		recvParam := ir.Parameter{
			Name: "receiver",
			Type: ir.TypeRef{Name: recvType},
		}
		if len(recv.Names) > 0 {
			recvParam.Name = recv.Names[0].Name
		}
		distilledFn.Parameters = append(distilledFn.Parameters, recvParam)

		// Mark as method
		distilledFn.Modifiers = append(distilledFn.Modifiers, ir.ModifierAbstract) // Using Abstract as "method" marker

		// CRITICAL FIX: For methods, visibility should be the minimum of method name visibility
		// and receiver type visibility (Go rule: a method can't be more visible than its receiver type)
		methodVisibility := p.getVisibility(fn.Name.Name)
		receiverVisibility := p.getReceiverTypeVisibility(recvType)
		distilledFn.Visibility = p.getMinimumVisibility(methodVisibility, receiverVisibility)

	}

	// Process type parameters (generics)
	if fn.Type.TypeParams != nil {
		for _, field := range fn.Type.TypeParams.List {
			for _, name := range field.Names {
				param := ir.TypeParam{
					Name: name.Name,
				}
				// Add constraint if present
				if field.Type != nil {
					param.Constraints = []ir.TypeRef{{Name: p.typeToString(field.Type)}}
				}
				distilledFn.TypeParams = append(distilledFn.TypeParams, param)
			}
		}
	}

	// Process parameters
	if fn.Type.Params != nil {
		for _, param := range fn.Type.Params.List {
			paramType := p.typeToString(param.Type)
			if len(param.Names) == 0 {
				// Unnamed parameter
				distilledFn.Parameters = append(distilledFn.Parameters, ir.Parameter{
					Type: ir.TypeRef{Name: paramType},
				})
			} else {
				// Named parameters
				for _, name := range param.Names {
					distilledFn.Parameters = append(distilledFn.Parameters, ir.Parameter{
						Name: name.Name,
						Type: ir.TypeRef{Name: paramType},
					})
				}
			}
		}
	}

	// Process return types
	if fn.Type.Results != nil && len(fn.Type.Results.List) > 0 {
		if len(fn.Type.Results.List) == 1 && len(fn.Type.Results.List[0].Names) == 0 {
			// Single unnamed return
			distilledFn.Returns = &ir.TypeRef{Name: p.typeToString(fn.Type.Results.List[0].Type)}
		} else {
			// Multiple returns or named returns
			var returns []string
			for _, result := range fn.Type.Results.List {
				resultType := p.typeToString(result.Type)
				if len(result.Names) == 0 {
					returns = append(returns, resultType)
				} else {
					for _, name := range result.Names {
						returns = append(returns, name.Name+" "+resultType)
					}
				}
			}
			distilledFn.Returns = &ir.TypeRef{Name: "(" + strings.Join(returns, ", ") + ")"}
		}
	}

	// Extract function body if exists and detect concurrency patterns
	if fn.Body != nil {
		// The body is kept verbatim, braces included ("{ ... }").
		start := p.fset.Position(fn.Body.Lbrace).Offset
		end := p.fset.Position(fn.Body.Rbrace).Offset + 1
		if start >= 0 && end <= len(p.source) && start < end {
			distilledFn.Implementation = string(p.source[start:end])
		}

		// Check for interesting constructs for modifiers
		var goroutines, defers int
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.GoStmt:
				goroutines++
			case *ast.DeferStmt:
				defers++
			}
			return true
		})

		// Add modifiers for interesting constructs
		if goroutines > 0 {
			distilledFn.Modifiers = append(distilledFn.Modifiers, ir.ModifierAsync) // Using Async for goroutines
		}
	}

	return distilledFn
}

// processFuncType processes a function type (for interface methods)
func (p *ASTParser) processFuncType(name string, funcType *ast.FuncType) *ir.DistilledFunction {
	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(funcType.Pos(), funcType.End()),
		},
		Name:       name,
		Parameters: []ir.Parameter{},
	}

	// Process parameters
	if funcType.Params != nil {
		for _, param := range funcType.Params.List {
			paramType := p.typeToString(param.Type)
			if len(param.Names) == 0 {
				fn.Parameters = append(fn.Parameters, ir.Parameter{
					Type: ir.TypeRef{Name: paramType},
				})
			} else {
				for _, name := range param.Names {
					fn.Parameters = append(fn.Parameters, ir.Parameter{
						Name: name.Name,
						Type: ir.TypeRef{Name: paramType},
					})
				}
			}
		}
	}

	// Process return types
	if funcType.Results != nil && len(funcType.Results.List) > 0 {
		if len(funcType.Results.List) == 1 && len(funcType.Results.List[0].Names) == 0 {
			fn.Returns = &ir.TypeRef{Name: p.typeToString(funcType.Results.List[0].Type)}
		} else {
			var returns []string
			for _, result := range funcType.Results.List {
				resultType := p.typeToString(result.Type)
				if len(result.Names) == 0 {
					returns = append(returns, resultType)
				} else {
					for _, name := range result.Names {
						returns = append(returns, name.Name+" "+resultType)
					}
				}
			}
			fn.Returns = &ir.TypeRef{Name: "(" + strings.Join(returns, ", ") + ")"}
		}
	}

	return fn
}

// Helper methods

// getLocation converts token positions to IR location
func (p *ASTParser) getLocation(start, end token.Pos) ir.Location {
	startPos := p.fset.Position(start)
	endPos := p.fset.Position(end)
	return ir.Location{
		StartLine:   startPos.Line,
		StartColumn: startPos.Column,
		EndLine:     endPos.Line,
		EndColumn:   endPos.Column,
	}
}

// getVisibility determines visibility based on name
func (p *ASTParser) getVisibility(name string) ir.Visibility {
	if name == "" || name[0] >= 'A' && name[0] <= 'Z' {
		return ir.VisibilityPublic
	}
	// In Go, lowercase names are package-private (internal), not truly private
	return ir.VisibilityInternal
}

// getReceiverTypeVisibility determines the visibility of a receiver type
func (p *ASTParser) getReceiverTypeVisibility(recvType string) ir.Visibility {
	// Strip pointer prefix if present (*User -> User)
	if strings.HasPrefix(recvType, "*") {
		recvType = recvType[1:]
	}

	// Extract base type name for generic types (Cache[K,V] -> Cache)
	if bracketIndex := strings.Index(recvType, "["); bracketIndex != -1 {
		recvType = recvType[:bracketIndex]
	}

	// Determine visibility based on Go naming convention
	return p.getVisibility(recvType)
}

// getMinimumVisibility returns the more restrictive visibility
func (p *ASTParser) getMinimumVisibility(vis1, vis2 ir.Visibility) ir.Visibility {
	// In Go, package-private (internal) is more restrictive than public
	// If either is internal, the result is internal
	if vis1 == ir.VisibilityInternal || vis2 == ir.VisibilityInternal {
		return ir.VisibilityInternal
	}
	return ir.VisibilityPublic
}

// typeToString converts an AST expression to a string representation
func (p *ASTParser) typeToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + p.typeToString(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			return "[]" + p.typeToString(t.Elt)
		}
		return "[" + p.exprToString(t.Len) + "]" + p.typeToString(t.Elt)
	case *ast.SliceExpr:
		return "[]" + p.typeToString(t.X)
	case *ast.MapType:
		return "map[" + p.typeToString(t.Key) + "]" + p.typeToString(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + p.typeToString(t.Value)
		case ast.RECV:
			return "<-chan " + p.typeToString(t.Value)
		default:
			return "chan " + p.typeToString(t.Value)
		}
	case *ast.FuncType:
		return "func" + p.signatureToString(t)
	case *ast.InterfaceType:
		if len(t.Methods.List) == 0 {
			return "interface{}"
		}
		return "interface{...}"
	case *ast.StructType:
		if len(t.Fields.List) == 0 {
			return "struct{}"
		}
		return "struct{...}"
	case *ast.SelectorExpr:
		return p.typeToString(t.X) + "." + t.Sel.Name
	case *ast.BinaryExpr:
		// A union of a type set: ~int | ~string
		if t.Op == token.OR {
			return p.typeToString(t.X) + " | " + p.typeToString(t.Y)
		}
	case *ast.UnaryExpr:
		// An underlying-type term of a type set: ~int
		if t.Op == token.TILDE {
			return "~" + p.typeToString(t.X)
		}
	case *ast.Ellipsis:
		return "..." + p.typeToString(t.Elt)
	case *ast.IndexExpr:
		// Generic type with single type parameter: T[K]
		return p.typeToString(t.X) + "[" + p.typeToString(t.Index) + "]"
	case *ast.IndexListExpr:
		// Generic type with multiple type parameters: T[K, V]
		var indices []string
		for _, index := range t.Indices {
			indices = append(indices, p.typeToString(index))
		}
		return p.typeToString(t.X) + "[" + strings.Join(indices, ", ") + "]"
	}
	// Fallback: get the source text
	if text, ok := p.sourceText(expr.Pos(), expr.End()); ok {
		return text
	}
	return "unknown"
}

// signatureToString returns the parameters and results of a function type as
// written, after "func": "(a int, b string) (string, error)". Grouped names
// stay grouped ("a, b int"), as in the source.
func (p *ASTParser) signatureToString(ft *ast.FuncType) string {
	sig := "(" + p.fieldListToString(ft.Params) + ")"
	if ft.Results == nil || len(ft.Results.List) == 0 {
		return sig
	}
	if len(ft.Results.List) == 1 && len(ft.Results.List[0].Names) == 0 {
		return sig + " " + p.typeToString(ft.Results.List[0].Type)
	}
	return sig + " (" + p.fieldListToString(ft.Results) + ")"
}

// fieldListToString returns the fields of a parameter or result list as
// written, separated by ", ": "a, b int, s ...string" or "int, error".
func (p *ASTParser) fieldListToString(fl *ast.FieldList) string {
	if fl == nil {
		return ""
	}
	parts := make([]string, 0, len(fl.List))
	for _, field := range fl.List {
		typ := p.typeToString(field.Type)
		if len(field.Names) == 0 {
			parts = append(parts, typ)
			continue
		}
		names := make([]string, len(field.Names))
		for i, name := range field.Names {
			names[i] = name.Name
		}
		parts = append(parts, strings.Join(names, ", ")+" "+typ)
	}
	return strings.Join(parts, ", ")
}

// exprToString converts an expression to string (for default values)
func (p *ASTParser) exprToString(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return e.Value
	case *ast.Ident:
		return e.Name
	case *ast.CompositeLit:
		return p.typeToString(e.Type) + "{...}"
	default:
		// Get source text
		text, _ := p.sourceText(expr.Pos(), expr.End())
		return text
	}
}

// dumpAST creates a structured representation of the Go AST for debugging
func (p *ASTParser) dumpAST(node ast.Node) map[string]interface{} {
	if node == nil {
		return nil
	}

	result := map[string]interface{}{
		"type": fmt.Sprintf("%T", node),
	}

	// Add position info
	if node.Pos().IsValid() {
		pos := p.fset.Position(node.Pos())
		result["startLine"] = pos.Line
		result["startCol"] = pos.Column
	}
	if node.End().IsValid() {
		pos := p.fset.Position(node.End())
		result["endLine"] = pos.Line
		result["endCol"] = pos.Column
	}

	// Add node-specific information
	switch n := node.(type) {
	case *ast.File:
		result["package"] = n.Name.Name
		result["imports"] = len(n.Imports)
		result["declarations"] = len(n.Decls)

		// Add summary of declarations
		var funcs, types, vars, consts int
		for _, decl := range n.Decls {
			switch decl.(type) {
			case *ast.FuncDecl:
				funcs++
			case *ast.GenDecl:
				gd := decl.(*ast.GenDecl)
				switch gd.Tok {
				case token.TYPE:
					types++
				case token.VAR:
					vars++
				case token.CONST:
					consts++
				}
			}
		}
		result["summary"] = map[string]int{
			"functions": funcs,
			"types":     types,
			"variables": vars,
			"constants": consts,
		}

	case *ast.FuncDecl:
		result["name"] = n.Name.Name
		if n.Recv != nil && len(n.Recv.List) > 0 {
			result["receiver"] = p.formatFieldList(n.Recv)
		}
		result["params"] = p.formatFieldList(n.Type.Params)
		result["results"] = p.formatFieldList(n.Type.Results)

	case *ast.GenDecl:
		result["token"] = n.Tok.String()
		result["specs"] = len(n.Specs)

	case *ast.TypeSpec:
		result["name"] = n.Name.Name
		result["typeKind"] = fmt.Sprintf("%T", n.Type)

	case *ast.StructType:
		result["fields"] = n.Fields.NumFields()

	case *ast.InterfaceType:
		result["methods"] = len(n.Methods.List)

	case *ast.ImportSpec:
		if n.Name != nil {
			result["alias"] = n.Name.Name
		}
		result["path"] = n.Path.Value
	}

	return result
}

// formatFieldList formats a field list for debugging output
func (p *ASTParser) formatFieldList(fields *ast.FieldList) []string {
	if fields == nil || fields.List == nil {
		return nil
	}

	var result []string
	for _, field := range fields.List {
		fieldStr := ""
		if len(field.Names) > 0 {
			names := make([]string, len(field.Names))
			for i, name := range field.Names {
				names[i] = name.Name
			}
			fieldStr = strings.Join(names, ", ") + " "
		}
		fieldStr += p.typeToString(field.Type)
		result = append(result, fieldStr)
	}
	return result
}
