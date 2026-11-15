package semantic

import (
	"context"
	"fmt"
	"io"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	typescript "tree-sitter-typescript"
)

// TypeScriptAnalyzer performs semantic analysis on TypeScript/JavaScript code using tree-sitter
type TypeScriptAnalyzer struct {
	parser   *sitter.Parser
	strategy *TypeScriptStrategy
	queries  *TypeScriptQueries
}

// TypeScriptQueries holds compiled tree-sitter queries for TypeScript
type TypeScriptQueries struct {
	declarations *sitter.Query
	calls        *sitter.Query
	imports      *sitter.Query
}

// NewTypeScriptAnalyzer creates a new TypeScript semantic analyzer
func NewTypeScriptAnalyzer() (*TypeScriptAnalyzer, error) {
	parser := sitter.NewParser()
	parser.SetLanguage(sitter.NewLanguage(typescript.Language()))

	queries, err := compileTypeScriptQueries()
	if err != nil {
		return nil, fmt.Errorf("failed to compile TypeScript queries: %w", err)
	}

	return &TypeScriptAnalyzer{
		parser:   parser,
		strategy: NewTypeScriptStrategy(),
		queries:  queries,
	}, nil
}

// compileTypeScriptQueries compiles all tree-sitter queries for TypeScript
func compileTypeScriptQueries() (*TypeScriptQueries, error) {
	// TypeScript declarations query
	declarationsQuery := `
;; Function declarations
(function_declaration
  name: (identifier) @function.name) @function.definition

;; Arrow function expressions assigned to variables
(variable_declarator
  name: (identifier) @arrow_function.name
  value: (arrow_function)) @arrow_function.definition

;; Class declarations
(class_declaration
  name: (type_identifier) @class.name) @class.definition

;; Interface declarations
(interface_declaration
  name: (type_identifier) @interface.name) @interface.definition

;; Type alias declarations
(type_alias_declaration
  name: (type_identifier) @type.name) @type.definition

;; Generic type alias declarations  
(type_alias_declaration
  name: (type_identifier) @generic_type.name
  type_parameters: (type_parameters)) @generic_type.definition

;; Enum declarations
(enum_declaration
  name: (identifier) @enum.name) @enum.definition

;; Variable declarations
(variable_declarator
  name: (identifier) @variable.name) @variable.declaration

;; Method definitions in classes
(method_definition
  name: (property_identifier) @method.name) @method.definition

;; Property definitions in classes
(public_field_definition
  name: (property_identifier) @property.name) @property.definition

;; Namespace declarations (object with function properties)
(variable_declarator
  name: (identifier) @namespace.name
  value: (object)) @namespace.definition

;; Const/let/var declarations with object values
(lexical_declaration
  (variable_declarator
    name: (identifier) @object.name
    value: (object))) @object.definition

;; TypeScript namespace declarations
(internal_module
  name: (identifier) @namespace.name) @namespace.definition
`

	// TypeScript function calls query
	callsQuery := `
;; Function calls: func(args)
(call_expression
  function: (identifier) @call.function) @call.simple

;; Method calls: obj.method(args)
(call_expression
  function: (member_expression
    property: (property_identifier) @call.method)) @call.method

;; Constructor calls: new Class(args)
(new_expression
  constructor: (identifier) @constructor.class) @constructor.call
`

	// TypeScript imports query
	importsQuery := `
;; ES6 imports: import { symbol } from "module"
(import_statement) @import.statement

;; Require statements: const module = require("module")
(variable_declarator
  value: (call_expression
    function: (identifier) @require.function)) @require.statement

;; Export statements
(export_statement) @export.statement
`

	declarations, err := sitter.NewQuery([]byte(declarationsQuery), sitter.NewLanguage(typescript.Language()))
	if err != nil {
		return nil, fmt.Errorf("failed to compile declarations query: %w", err)
	}

	calls, err := sitter.NewQuery([]byte(callsQuery), sitter.NewLanguage(typescript.Language()))
	if err != nil {
		return nil, fmt.Errorf("failed to compile calls query: %w", err)
	}

	imports, err := sitter.NewQuery([]byte(importsQuery), sitter.NewLanguage(typescript.Language()))
	if err != nil {
		return nil, fmt.Errorf("failed to compile imports query: %w", err)
	}

	return &TypeScriptQueries{
		declarations: declarations,
		calls:        calls,
		imports:      imports,
	}, nil
}

// AnalyzeFile performs semantic analysis on a TypeScript file
func (tsa *TypeScriptAnalyzer) AnalyzeFile(ctx context.Context, reader io.Reader, filename string) (*FileAnalysis, error) {
	// Read the entire file content
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	// Parse the file
	tree, err := tsa.parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse TypeScript file: %w", err)
	}
	defer tree.Close()

	// Create file analysis
	analysis := &FileAnalysis{
		FilePath:     filename,
		Language:     "typescript",
		SymbolTable:  NewSymbolTable(filename, "typescript"),
		Dependencies: make([]DependencyInfo, 0),
		CallSites:    make([]CallSite, 0),
	}

	// Extract symbols using tree-sitter queries
	if err := tsa.extractSymbols(tree, content, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract symbols: %w", err)
	}

	// Extract call sites
	if err := tsa.extractCallSites(tree, content, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract call sites: %w", err)
	}

	// Extract dependencies
	if err := tsa.extractDependencies(tree, content, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract dependencies: %w", err)
	}

	return analysis, nil
}

// extractSymbols extracts symbol declarations using tree-sitter queries
func (tsa *TypeScriptAnalyzer) extractSymbols(tree *sitter.Tree, content []byte, analysis *FileAnalysis) error {
	rootNode := tree.RootNode()

	// Query for declarations
	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(tsa.queries.declarations, rootNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			node := capture.Node
			captureName := tsa.queries.declarations.CaptureNameForId(capture.Index)

			switch captureName {
			case "function.name", "arrow_function.name":
				tsa.extractFunction(node, content, analysis, SymbolKindFunction)
			case "class.name":
				tsa.extractClass(node, content, analysis)
			case "interface.name":
				tsa.extractInterface(node, content, analysis)
			case "type.name", "generic_type.name":
				tsa.extractType(node, content, analysis)
			case "enum.name":
				tsa.extractEnum(node, content, analysis)
			case "variable.name":
				tsa.extractVariable(node, content, analysis)
			case "method.name":
				tsa.extractMethod(node, content, analysis)
			case "property.name":
				tsa.extractProperty(node, content, analysis)
			case "namespace.name", "object.name":
				tsa.extractNamespace(node, content, analysis)
			}
		}
	}

	return nil
}

// extractFunction extracts function symbols
func (tsa *TypeScriptAnalyzer) extractFunction(node *sitter.Node, content []byte, analysis *FileAnalysis, kind SymbolKind) {
	name := string(content[node.StartByte():node.EndByte()])

	// Determine scope
	scope := tsa.strategy.DetermineScope(node, nil, content)

	// Generate symbol ID
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	// Extract parameters and return type from parent function node
	funcNode := node.Parent()
	params := tsa.extractParameters(funcNode, content)
	returnType := tsa.extractReturnType(funcNode, content)

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       kind,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(funcNode, content),
		Metadata: SymbolMeta{
			Parameters: params,
			ReturnType: returnType,
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractClass extracts class symbols
func (tsa *TypeScriptAnalyzer) extractClass(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	classNode := node.Parent()

	// Extract inheritance information
	var extends []string
	if heritageClause := classNode.ChildByFieldName("heritage"); heritageClause != nil {
		extends = tsa.extractTypeReferences(heritageClause, content)
	}

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindClass,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(classNode, content),
		Metadata: SymbolMeta{
			Extends: extends,
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractInterface extracts interface symbols
func (tsa *TypeScriptAnalyzer) extractInterface(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	_ = node.Parent() // interfaceNode not used yet

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindInterface,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: "public", // Interfaces are always public in TypeScript
		Metadata:   SymbolMeta{},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractType extracts type alias symbols
func (tsa *TypeScriptAnalyzer) extractType(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindType,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: "public",
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractEnum extracts enum symbols
func (tsa *TypeScriptAnalyzer) extractEnum(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindEnum,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(node.Parent(), content),
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractVariable extracts variable symbols
func (tsa *TypeScriptAnalyzer) extractVariable(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	varNode := node.Parent()
	_ = tsa.extractVariableType(varNode, content) // varType not used yet

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindVariable,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(varNode, content),
		Metadata:   SymbolMeta{},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractMethod extracts method symbols
func (tsa *TypeScriptAnalyzer) extractMethod(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	methodNode := node.Parent()
	params := tsa.extractParameters(methodNode, content)
	returnType := tsa.extractReturnType(methodNode, content)

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindMethod,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(methodNode, content),
		Metadata: SymbolMeta{
			Parameters: params,
			ReturnType: returnType,
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractProperty extracts property symbols
func (tsa *TypeScriptAnalyzer) extractProperty(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	propNode := node.Parent()
	_ = tsa.extractPropertyType(propNode, content) // propType not used yet

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindProperty,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: tsa.determineVisibility(propNode, content),
		Metadata:   SymbolMeta{},
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractNamespace extracts namespace symbols
func (tsa *TypeScriptAnalyzer) extractNamespace(node *sitter.Node, content []byte, analysis *FileAnalysis) {
	name := string(content[node.StartByte():node.EndByte()])
	scope := tsa.strategy.DetermineScope(node, nil, content)
	symbolID := GenerateSymbolID(analysis.FilePath, name, scope)

	symbol := &Symbol{
		ID:         symbolID,
		Name:       name,
		Kind:       SymbolKindNamespace,
		Scope:      scope,
		Location:   nodeToLocation(node, analysis.FilePath),
		Visibility: "public",
	}

	analysis.SymbolTable.AddSymbol(symbol)
}

// extractCallSites extracts function call sites using tree-sitter queries
func (tsa *TypeScriptAnalyzer) extractCallSites(tree *sitter.Tree, content []byte, analysis *FileAnalysis) error {
	rootNode := tree.RootNode()

	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(tsa.queries.calls, rootNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			node := capture.Node
			captureName := tsa.queries.calls.CaptureNameForId(capture.Index)

			var calleeName string

			switch captureName {
			case "call.function", "generic_call.function":
				calleeName = string(content[node.StartByte():node.EndByte()])
			case "call.method", "chained_call.method", "optional_call.method":
				calleeName = string(content[node.StartByte():node.EndByte()])
			case "constructor.class":
				calleeName = string(content[node.StartByte():node.EndByte()])
			default:
				continue
			}

			// Determine caller context
			callerScope := tsa.strategy.DetermineScope(node, tree, content)
			var callerID SymbolID
			if callerScope != "" {
				callerID = GenerateSymbolID(analysis.FilePath, "", callerScope)
			}

			callSite := CallSite{
				CalleeName: calleeName,
				CallerID:   callerID,
				Location:   nodeToLocation(node, analysis.FilePath),
				IsResolved: false,
			}

			analysis.CallSites = append(analysis.CallSites, callSite)
		}
	}

	return nil
}

// extractDependencies extracts import dependencies using tree-sitter queries
func (tsa *TypeScriptAnalyzer) extractDependencies(tree *sitter.Tree, content []byte, analysis *FileAnalysis) error {
	rootNode := tree.RootNode()

	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(tsa.queries.imports, rootNode)

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			node := capture.Node

			// Extract import path from different import types
			if importPath := tsa.extractImportPath(node, content); importPath != "" {
				dep := DependencyInfo{
					SourceFile:   analysis.FilePath,
					TargetModule: importPath,
					ImportType:   "import",
					IsRelative:   strings.HasPrefix(importPath, "."),
					Location:     nodeToLocation(node, analysis.FilePath),
				}
				analysis.Dependencies = append(analysis.Dependencies, dep)
			}
		}
	}

	return nil
}

// Helper functions for TypeScript-specific extraction

func (tsa *TypeScriptAnalyzer) extractImportPath(node *sitter.Node, content []byte) string {
	// Handle different import statement types
	switch node.Type() {
	case "import_statement":
		if sourceNode := node.ChildByFieldName("source"); sourceNode != nil {
			if sourceNode.Type() == "string" {
				// Remove quotes from string literal
				source := string(content[sourceNode.StartByte():sourceNode.EndByte()])
				return strings.Trim(source, "\"'")
			}
		}
	case "variable_declarator":
		// Handle require() calls: const config = require('./config')
		if valueNode := node.ChildByFieldName("value"); valueNode != nil && valueNode.Type() == "call_expression" {
			if funcNode := valueNode.ChildByFieldName("function"); funcNode != nil {
				funcName := string(content[funcNode.StartByte():funcNode.EndByte()])
				if funcName == "require" {
					if argsNode := valueNode.ChildByFieldName("arguments"); argsNode != nil {
						for i := 0; i < int(argsNode.ChildCount()); i++ {
							child := argsNode.Child(i)
							if child.Type() == "string" {
								source := string(content[child.StartByte():child.EndByte()])
								return strings.Trim(source, "\"'")
							}
						}
					}
				}
			}
		}
	case "call_expression":
		// Handle dynamic imports and other call expressions
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "arguments" {
				for j := 0; j < int(child.ChildCount()); j++ {
					arg := child.Child(j)
					if arg.Type() == "string" {
						source := string(content[arg.StartByte():arg.EndByte()])
						return strings.Trim(source, "\"'")
					}
				}
			}
		}
	}
	return ""
}

func (tsa *TypeScriptAnalyzer) extractParameters(node *sitter.Node, content []byte) []ParameterInfo {
	var params []ParameterInfo

	if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
		for i := 0; i < int(paramsNode.ChildCount()); i++ {
			child := paramsNode.Child(i)
			if child.Type() == "required_parameter" || child.Type() == "optional_parameter" {
				if nameNode := child.ChildByFieldName("pattern"); nameNode != nil {
					paramName := string(content[nameNode.StartByte():nameNode.EndByte()])
					paramType := ""

					if typeNode := child.ChildByFieldName("type"); typeNode != nil {
						paramType = string(content[typeNode.StartByte():typeNode.EndByte()])
					}

					params = append(params, ParameterInfo{
						Name: paramName,
						Type: paramType,
					})
				}
			}
		}
	}

	return params
}

func (tsa *TypeScriptAnalyzer) extractReturnType(node *sitter.Node, content []byte) string {
	if typeNode := node.ChildByFieldName("return_type"); typeNode != nil {
		return string(content[typeNode.StartByte():typeNode.EndByte()])
	}
	return ""
}

func (tsa *TypeScriptAnalyzer) extractTypeParameters(node *sitter.Node, content []byte) []string {
	var typeParams []string

	if typeParamsNode := node.ChildByFieldName("type_parameters"); typeParamsNode != nil {
		for i := 0; i < int(typeParamsNode.ChildCount()); i++ {
			child := typeParamsNode.Child(i)
			if child.Type() == "type_parameter" {
				if nameNode := child.ChildByFieldName("name"); nameNode != nil {
					typeParams = append(typeParams, string(content[nameNode.StartByte():nameNode.EndByte()]))
				}
			}
		}
	}

	return typeParams
}

func (tsa *TypeScriptAnalyzer) extractTypeReferences(node *sitter.Node, content []byte) []string {
	var refs []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "extends_clause" {
			for j := 0; j < int(child.ChildCount()); j++ {
				typeNode := child.Child(j)
				if typeNode.Type() == "type_identifier" || typeNode.Type() == "generic_type" {
					typeName := string(content[typeNode.StartByte():typeNode.EndByte()])
					refs = append(refs, typeName)
				}
			}
		}
	}

	return refs
}

func (tsa *TypeScriptAnalyzer) extractVariableType(node *sitter.Node, content []byte) string {
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		return string(content[typeNode.StartByte():typeNode.EndByte()])
	}
	return ""
}

func (tsa *TypeScriptAnalyzer) extractPropertyType(node *sitter.Node, content []byte) string {
	if typeNode := node.ChildByFieldName("type"); typeNode != nil {
		return string(content[typeNode.StartByte():typeNode.EndByte()])
	}
	return ""
}

func (tsa *TypeScriptAnalyzer) determineVisibility(node *sitter.Node, content []byte) string {
	// Look for access modifiers
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		text := string(content[child.StartByte():child.EndByte()])

		switch text {
		case "private":
			return "private"
		case "protected":
			return "protected"
		case "public":
			return "public"
		}
	}

	// Default visibility in TypeScript
	return "public"
}

// nodeToLocation converts a tree-sitter node to a FileLocation
func nodeToLocation(node *sitter.Node, filePath string) FileLocation {
	startPoint := node.StartPoint()
	endPoint := node.EndPoint()

	return FileLocation{
		FilePath:  filePath,
		StartLine: int(startPoint.Row) + 1,
		EndLine:   int(endPoint.Row) + 1,
		StartCol:  int(startPoint.Column) + 1,
		EndCol:    int(endPoint.Column) + 1,
	}
}
