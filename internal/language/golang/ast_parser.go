package golang

import (
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
		// Add build constraints as a comment at the top
		constraintComment := &ir.DistilledComment{
			BaseNode: ir.BaseNode{
				Location: p.getLocation(file.Pos(), file.Pos()),
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
				Text: strings.TrimPrefix(comment.Text, "//"),
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

	// Process imports
	for _, spec := range file.Imports {
		imp := p.processImport(spec)
		if imp != nil {
			distilledFile.Children = append(distilledFile.Children, imp)
		}
	}

	// Single-pass processing to preserve declaration order
	typeMap := make(map[string]ir.DistilledNode) // Map from type name to the distilled node

	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			// Extract function doc comments
			var docComments []ir.DistilledNode
			if d.Doc != nil {
				for _, comment := range d.Doc.List {
					text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
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

	processedComments := make(map[*ast.Comment]bool)

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
			// Also check specs within GenDecl
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
				text := strings.TrimSpace(strings.TrimPrefix(c.Text, "//"))
				if strings.HasPrefix(text, " ") {
					text = strings.TrimSpace(text)
				}
				// Also handle /* */ style comments
				if strings.HasPrefix(c.Text, "/*") && strings.HasSuffix(c.Text, "*/") {
					text = strings.TrimSpace(c.Text[2 : len(c.Text)-2])
				}
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

// insertCommentAtPosition inserts a comment at the correct position based on its location
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

	// Insert at the appropriate position
	if insertIndex == len(file.Children) {
		file.Children = append(file.Children, comment)
	} else {
		// Insert in the middle
		file.Children = append(file.Children[:insertIndex+1], file.Children[insertIndex:]...)
		file.Children[insertIndex] = comment
	}
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
			text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
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
					text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
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
				nodes = append(nodes, node)
			}
		case *ast.ValueSpec:
			// Add value doc comment if present
			if s.Doc != nil {
				for _, comment := range s.Doc.List {
					text := strings.TrimSpace(strings.TrimPrefix(comment.Text, "//"))
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
			// Process const/var declarations
			for i, name := range s.Names {

				field := &ir.DistilledField{
					BaseNode: ir.BaseNode{
						Location: p.getLocation(name.Pos(), name.End()),
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

	switch t := spec.Type.(type) {
	case *ast.StructType:
		return p.processStructWithParams(spec.Name.Name, typeParams, t)
	case *ast.InterfaceType:
		return p.processInterfaceWithParams(spec.Name.Name, typeParams, t)
	default:
		// A defined type (type Named int) or, with "=", an alias
		// (type Alias = int), which carries the modifier type_alias.
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

// processStruct processes a struct type
func (p *ASTParser) processStruct(name string, structType *ast.StructType) *ir.DistilledClass {
	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(structType.Pos(), structType.End()),
		},
		Name:       name,
		Visibility: p.getVisibility(name),
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}

	// Process fields
	for _, field := range structType.Fields.List {
		// Get field type
		fieldType := p.typeToString(field.Type)

		if len(field.Names) == 0 {
			// Embedded field
			embedField := &ir.DistilledField{
				BaseNode: ir.BaseNode{
					Location: p.getLocation(field.Pos(), field.End()),
				},
				Name:       fieldType, // Use type name as field name
				Type:       &ir.TypeRef{Name: fieldType},
				Visibility: ir.VisibilityPublic,
				Modifiers:  []ir.Modifier{ir.ModifierEmbedded}, // Mark as embedded
			}
			class.Children = append(class.Children, embedField)
		} else {
			// Named fields
			for _, name := range field.Names {
				distilledField := &ir.DistilledField{
					BaseNode: ir.BaseNode{
						Location: p.getLocation(name.Pos(), name.End()),
					},
					Name:       name.Name,
					Type:       &ir.TypeRef{Name: fieldType},
					Visibility: p.getVisibility(name.Name),
				}

				// TODO: Process tags when annotation support is added

				class.Children = append(class.Children, distilledField)
			}
		}
	}

	return class
}

// processInterface processes an interface type. Each embedded element (an
// embedded interface such as io.Reader, or a type set such as ~int | ~string)
// is one entry of Extends, as written; the methods are the children.
func (p *ASTParser) processInterface(name string, interfaceType *ast.InterfaceType) *ir.DistilledInterface {
	intf := &ir.DistilledInterface{
		BaseNode: ir.BaseNode{
			Location: p.getLocation(interfaceType.Pos(), interfaceType.End()),
		},
		Name:       name,
		Visibility: p.getVisibility(name),
		Children:   []ir.DistilledNode{},
	}

	// Process methods
	for _, method := range interfaceType.Methods.List {
		if len(method.Names) == 0 {
			// Embedded element: one entry, a union stays whole
			intf.Extends = append(intf.Extends, ir.TypeRef{Name: p.typeToString(method.Type)})
		} else {
			// Method declaration
			for _, name := range method.Names {
				if funcType, ok := method.Type.(*ast.FuncType); ok {
					fn := p.processFuncType(name.Name, funcType)
					fn.Visibility = ir.VisibilityPublic
					intf.Children = append(intf.Children, fn)
				}
			}
		}
	}

	return intf
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
		return "func" // Simplified
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
	start := p.fset.Position(expr.Pos()).Offset
	end := p.fset.Position(expr.End()).Offset
	if start >= 0 && end <= len(p.source) && start < end {
		return string(p.source[start:end])
	}
	return "unknown"
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
		start := p.fset.Position(expr.Pos()).Offset
		end := p.fset.Position(expr.End()).Offset
		if start >= 0 && end <= len(p.source) && start < end {
			return string(p.source[start:end])
		}
		return ""
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
