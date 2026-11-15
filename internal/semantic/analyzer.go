package semantic

import (
	"context"
	"embed"
	"fmt"
	"io"
	"strings"
	"time"

	sitter "github.com/smacker/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

//go:embed queries/*.scm
var queryFiles embed.FS

// Analyzer performs semantic analysis on source code
type Analyzer struct {
	language    *sitter.Language
	parser      *sitter.Parser
	projectRoot string
	queries     map[string]*sitter.Query
}

// NewAnalyzer creates a new semantic analyzer for Python
func NewAnalyzer(projectRoot string) (*Analyzer, error) {
	analyzer := &Analyzer{
		language:    sitter.NewLanguage(tree_sitter_python.Language()),
		parser:      sitter.NewParser(),
		projectRoot: projectRoot,
		queries:     make(map[string]*sitter.Query),
	}

	analyzer.parser.SetLanguage(analyzer.language)

	// Load tree-sitter queries
	if err := analyzer.loadQueries(); err != nil {
		return nil, fmt.Errorf("failed to load queries: %w", err)
	}

	return analyzer, nil
}

// loadQueries loads all tree-sitter query files
func (a *Analyzer) loadQueries() error {
	queryNames := []string{"python_declarations", "python_imports", "python_calls"}

	for _, queryName := range queryNames {
		queryPath := fmt.Sprintf("queries/%s.scm", queryName)
		queryBytes, err := queryFiles.ReadFile(queryPath)
		if err != nil {
			return fmt.Errorf("failed to read query file %s: %w", queryPath, err)
		}

		query, err := sitter.NewQuery(queryBytes, a.language)
		if err != nil {
			return fmt.Errorf("failed to compile query %s: %w", queryName, err)
		}

		a.queries[queryName] = query
	}

	return nil
}

// AnalyzeFile performs Pass 1 analysis on a single file
func (a *Analyzer) AnalyzeFile(ctx context.Context, reader io.Reader, filePath string) (*FileAnalysis, error) {
	startTime := time.Now()

	// Read file content
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file content: %w", err)
	}

	// Parse with tree-sitter
	tree, err := a.parser.ParseCtx(ctx, nil, content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse file: %w", err)
	}
	defer tree.Close()

	// Create symbol table
	symbolTable := NewSymbolTable(filePath, "python")

	// Create file analysis
	analysis := &FileAnalysis{
		FilePath:     filePath,
		Language:     "python",
		SymbolTable:  symbolTable,
		Dependencies: make([]DependencyInfo, 0),
		CallSites:    make([]CallSite, 0),
		AST:          tree,
		Content:      content,
		AnalysisTime: time.Since(startTime),
	}

	// Extract declarations
	if err := a.extractDeclarations(tree, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract declarations: %w", err)
	}

	// Extract imports
	if err := a.extractImports(tree, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract imports: %w", err)
	}

	// Extract calls (for future Pass 2)
	if err := a.extractCalls(tree, analysis); err != nil {
		return nil, fmt.Errorf("failed to extract calls: %w", err)
	}

	analysis.AnalysisTime = time.Since(startTime)
	return analysis, nil
}

// extractDeclarations extracts symbol declarations using tree-sitter queries
func (a *Analyzer) extractDeclarations(tree *sitter.Tree, analysis *FileAnalysis) error {
	query := a.queries["python_declarations"]
	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(query, tree.RootNode())

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			if err := a.processDeclarationCapture(captureName, node, analysis); err != nil {
				return fmt.Errorf("failed to process declaration capture %s: %w", captureName, err)
			}
		}
	}

	return nil
}

// processDeclarationCapture processes a single declaration capture
func (a *Analyzer) processDeclarationCapture(captureName string, node *sitter.Node, analysis *FileAnalysis) error {
	content := analysis.Content

	switch {
	case strings.HasPrefix(captureName, "function."):
		return a.processFunctionDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "class."):
		return a.processClassDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "method."):
		return a.processMethodDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "decorated_method."):
		return a.processMethodDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "variable."):
		return a.processVariableDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "constant."):
		return a.processConstantDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "instantiation."):
		return a.processInstantiationDeclaration(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "property."):
		return a.processPropertyDeclaration(captureName, node, content, analysis)
	}

	return nil
}

// processFunctionDeclaration processes function declarations
func (a *Analyzer) processFunctionDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "function.definition" {
		return nil
	}

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	funcName := nodeText(nameNode, content)
	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract function signature
	signature := a.extractFunctionSignature(node, content)

	// Extract parameters
	parameters := a.extractParameters(node, content)

	// Extract decorators
	decorators := a.extractDecorators(node, content)

	// Determine visibility
	visibility := "public"
	if strings.HasPrefix(funcName, "_") {
		if strings.HasPrefix(funcName, "__") && strings.HasSuffix(funcName, "__") {
			visibility = "magic"
		} else {
			visibility = "private"
		}
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, funcName, ""),
		Name:       funcName,
		Kind:       SymbolKindFunction,
		Location:   location,
		Scope:      "",
		Signature:  signature,
		Visibility: visibility,
		IsExported: !strings.HasPrefix(funcName, "_"),
		Language:   "python",
		Metadata: SymbolMeta{
			Parameters: parameters,
			Decorators: decorators,
			LineCount:  int(node.EndPoint().Row - node.StartPoint().Row + 1),
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processClassDeclaration processes class declarations
func (a *Analyzer) processClassDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "class.definition" {
		return nil
	}

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	className := nodeText(nameNode, content)
	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract base classes
	var extends []string
	if superclassesNode := node.ChildByFieldName("superclasses"); superclassesNode != nil {
		extends = a.extractSuperclasses(superclassesNode, content)
	}

	// Extract decorators
	decorators := a.extractDecorators(node, content)

	// Determine visibility
	visibility := "public"
	if strings.HasPrefix(className, "_") {
		visibility = "private"
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, className, ""),
		Name:       className,
		Kind:       SymbolKindClass,
		Location:   location,
		Scope:      "",
		Visibility: visibility,
		IsExported: !strings.HasPrefix(className, "_"),
		Language:   "python",
		Metadata: SymbolMeta{
			Extends:    extends,
			Decorators: decorators,
			LineCount:  int(node.EndPoint().Row - node.StartPoint().Row + 1),
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processMethodDeclaration processes method declarations (functions inside classes)
func (a *Analyzer) processMethodDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "method.definition" && captureName != "decorated_method.definition" {
		return nil
	}

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	methodName := nodeText(nameNode, content)

	// Find the containing class
	className := a.findContainingClass(node, content)
	if className == "" {
		return nil // Skip if we can't determine the class
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract method signature
	signature := a.extractFunctionSignature(node, content)

	// Extract parameters
	parameters := a.extractParameters(node, content)

	// Extract decorators
	decorators := a.extractDecorators(node, content)

	// Determine method type and visibility
	visibility := "public"
	isStatic := false
	isAbstract := false

	if strings.HasPrefix(methodName, "_") {
		if strings.HasPrefix(methodName, "__") && strings.HasSuffix(methodName, "__") {
			visibility = "magic"
		} else {
			visibility = "private"
		}
	}

	for _, decorator := range decorators {
		switch decorator {
		case "staticmethod":
			isStatic = true
		case "abstractmethod":
			isAbstract = true
		}
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, methodName, className),
		Name:       methodName,
		Kind:       SymbolKindMethod,
		Location:   location,
		Scope:      className,
		Signature:  signature,
		Visibility: visibility,
		IsExported: !strings.HasPrefix(methodName, "_"),
		IsStatic:   isStatic,
		IsAbstract: isAbstract,
		Language:   "python",
		Metadata: SymbolMeta{
			Parameters: parameters,
			Decorators: decorators,
			LineCount:  int(node.EndPoint().Row - node.StartPoint().Row + 1),
		},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processVariableDeclaration processes variable declarations
func (a *Analyzer) processVariableDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "variable.assignment" {
		return nil
	}

	// Find the variable name
	varName := ""
	if nameNode := node.ChildByFieldName("left"); nameNode != nil {
		varName = nodeText(nameNode, content)
	}

	if varName == "" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Determine visibility
	visibility := "public"
	if strings.HasPrefix(varName, "_") {
		visibility = "private"
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, varName, ""),
		Name:       varName,
		Kind:       SymbolKindVariable,
		Location:   location,
		Scope:      "",
		Visibility: visibility,
		IsExported: !strings.HasPrefix(varName, "_"),
		Language:   "python",
		Metadata:   SymbolMeta{},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processConstantDeclaration processes constant declarations (ALL_CAPS variables)
func (a *Analyzer) processConstantDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "constant.assignment" {
		return nil
	}

	// Find the constant name
	constName := ""
	if nameNode := node.ChildByFieldName("left"); nameNode != nil {
		constName = nodeText(nameNode, content)
	}

	if constName == "" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, constName, ""),
		Name:       constName,
		Kind:       SymbolKindConstant,
		Location:   location,
		Scope:      "",
		Visibility: "public",
		IsExported: true,
		Language:   "python",
		Metadata:   SymbolMeta{},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processPropertyDeclaration processes property declarations
func (a *Analyzer) processPropertyDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "property.definition" {
		return nil
	}

	nameNode := node.ChildByFieldName("name")
	if nameNode == nil {
		return nil
	}

	propName := nodeText(nameNode, content)

	// Find the containing class
	className := a.findContainingClass(node, content)

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	symbol := &Symbol{
		ID:         GenerateSymbolID(analysis.FilePath, propName, className),
		Name:       propName,
		Kind:       SymbolKindProperty,
		Location:   location,
		Scope:      className,
		Visibility: "public",
		IsExported: true,
		Language:   "python",
		Metadata:   SymbolMeta{Decorators: []string{"property"}},
	}

	analysis.SymbolTable.AddSymbol(symbol)
	return nil
}

// processInstantiationDeclaration processes object instantiation assignments
func (a *Analyzer) processInstantiationDeclaration(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "instantiation.assignment" {
		return nil
	}

	// Extract variable name (left side of assignment)
	leftNode := node.ChildByFieldName("left")
	if leftNode == nil {
		return nil
	}
	varName := nodeText(leftNode, content)

	// Extract call expression (right side of assignment)
	rightNode := node.ChildByFieldName("right")
	if rightNode == nil {
		return nil
	}

	// Extract the function being called (should be class constructor)
	funcNode := rightNode.ChildByFieldName("function")
	if funcNode == nil {
		return nil
	}
	className := nodeText(funcNode, content)

	// Track this variable -> type mapping
	if analysis.SymbolTable.TypeTracker != nil {
		analysis.SymbolTable.TypeTracker.Variables[varName] = className
		analysis.SymbolTable.TypeTracker.Types[className] = analysis.FilePath
	}

	return nil
}

// Helper functions

// nodeText extracts text content from a tree-sitter node
func nodeText(node *sitter.Node, content []byte) string {
	return string(content[node.StartByte():node.EndByte()])
}

// extractFunctionSignature extracts the full function signature
func (a *Analyzer) extractFunctionSignature(node *sitter.Node, content []byte) string {
	// Find the parameters node
	if paramsNode := node.ChildByFieldName("parameters"); paramsNode != nil {
		nameNode := node.ChildByFieldName("name")
		if nameNode != nil {
			funcName := nodeText(nameNode, content)
			params := nodeText(paramsNode, content)
			return fmt.Sprintf("%s%s", funcName, params)
		}
	}
	return ""
}

// extractParameters extracts parameter information
func (a *Analyzer) extractParameters(node *sitter.Node, content []byte) []ParameterInfo {
	var parameters []ParameterInfo

	paramsNode := node.ChildByFieldName("parameters")
	if paramsNode == nil {
		return parameters
	}

	// Iterate through parameter children
	for i := 0; i < int(paramsNode.ChildCount()); i++ {
		child := paramsNode.Child(i)
		if child.Type() == "identifier" {
			paramName := nodeText(child, content)
			param := ParameterInfo{
				Name:       paramName,
				IsOptional: false,
				IsVariadic: false,
			}
			parameters = append(parameters, param)
		}
	}

	return parameters
}

// extractDecorators extracts decorator information
func (a *Analyzer) extractDecorators(node *sitter.Node, content []byte) []string {
	var decorators []string

	// Look for parent decorated_definition
	parent := node.Parent()
	for parent != nil {
		if parent.Type() == "decorated_definition" {
			// Find decorator_list
			for i := 0; i < int(parent.ChildCount()); i++ {
				child := parent.Child(i)
				if child.Type() == "decorator_list" {
					decorators = a.extractDecoratorsFromList(child, content)
					break
				}
			}
			break
		}
		parent = parent.Parent()
	}

	return decorators
}

// extractDecoratorsFromList extracts decorators from a decorator list
func (a *Analyzer) extractDecoratorsFromList(node *sitter.Node, content []byte) []string {
	var decorators []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "decorator" {
			decoratorText := strings.TrimPrefix(nodeText(child, content), "@")
			decorators = append(decorators, decoratorText)
		}
	}

	return decorators
}

// extractSuperclasses extracts base class information
func (a *Analyzer) extractSuperclasses(node *sitter.Node, content []byte) []string {
	var superclasses []string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == "identifier" {
			superclasses = append(superclasses, nodeText(child, content))
		}
	}

	return superclasses
}

// findContainingClass finds the name of the class containing a node
func (a *Analyzer) findContainingClass(node *sitter.Node, content []byte) string {
	current := node.Parent()
	for current != nil {
		if current.Type() == "class_definition" {
			if nameNode := current.ChildByFieldName("name"); nameNode != nil {
				return nodeText(nameNode, content)
			}
		}
		current = current.Parent()
	}
	return ""
}

// extractImports extracts import statements using tree-sitter queries
func (a *Analyzer) extractImports(tree *sitter.Tree, analysis *FileAnalysis) error {
	query := a.queries["python_imports"]
	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(query, tree.RootNode())

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			if err := a.processImportCapture(captureName, node, analysis); err != nil {
				return fmt.Errorf("failed to process import capture %s: %w", captureName, err)
			}
		}
	}

	return nil
}

// processImportCapture processes import-related captures
func (a *Analyzer) processImportCapture(captureName string, node *sitter.Node, analysis *FileAnalysis) error {
	content := analysis.Content

	switch captureName {
	case "import.statement":
		return a.processImportStatement(node, content, analysis)
	case "from_import.statement":
		return a.processFromImportStatement(node, content, analysis)
	}

	return nil
}

// processImportStatement processes simple import statements
func (a *Analyzer) processImportStatement(node *sitter.Node, content []byte, analysis *FileAnalysis) error {

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract module name
	moduleNode := node.ChildByFieldName("name")
	if moduleNode == nil {
		return nil
	}

	var moduleName, alias string
	if moduleNode.Type() == "aliased_import" {
		// import module as alias
		if nameNode := moduleNode.ChildByFieldName("name"); nameNode != nil {
			moduleName = nodeText(nameNode, content)
		}
		if aliasNode := moduleNode.ChildByFieldName("alias"); aliasNode != nil {
			alias = nodeText(aliasNode, content)
		}
	} else {
		// simple import module
		moduleName = nodeText(moduleNode, content)
	}

	dependency := DependencyInfo{
		SourceFile:      analysis.FilePath,
		TargetModule:    moduleName,
		ImportedSymbols: []string{}, // Empty for whole module import
		ImportAlias:     alias,
		ImportType:      "import",
		IsRelative:      false,
		Location:        location,
	}

	analysis.Dependencies = append(analysis.Dependencies, dependency)
	analysis.SymbolTable.Dependencies = append(analysis.SymbolTable.Dependencies, moduleName)

	return nil
}

// processFromImportStatement processes from-import statements
func (a *Analyzer) processFromImportStatement(node *sitter.Node, content []byte, analysis *FileAnalysis) error {

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract module and symbol information
	var moduleName, symbolName, alias string

	moduleNode := node.ChildByFieldName("module_name")
	if moduleNode != nil {
		moduleName = nodeText(moduleNode, content)
	}

	// Find imported symbols in the import list
	nameNode := node.ChildByFieldName("name")
	if nameNode != nil {
		for i := 0; i < int(nameNode.ChildCount()); i++ {
			child := nameNode.Child(i)
			if child.Type() == "identifier" {
				symbolName = nodeText(child, content)
			} else if child.Type() == "aliased_import" {
				if symNode := child.ChildByFieldName("name"); symNode != nil {
					symbolName = nodeText(symNode, content)
				}
				if aliasNode := child.ChildByFieldName("alias"); aliasNode != nil {
					alias = nodeText(aliasNode, content)
				}
			}
		}
	}

	dependency := DependencyInfo{
		SourceFile:      analysis.FilePath,
		TargetModule:    moduleName,
		ImportedSymbols: []string{symbolName},
		ImportAlias:     alias,
		ImportType:      "from_import",
		IsRelative:      strings.HasPrefix(moduleName, "."),
		Location:        location,
	}

	analysis.Dependencies = append(analysis.Dependencies, dependency)
	if moduleName != "" {
		analysis.SymbolTable.Dependencies = append(analysis.SymbolTable.Dependencies, moduleName)
	}

	return nil
}

// processWildcardImport processes wildcard imports (from module import *)
func (a *Analyzer) processWildcardImport(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "wildcard_import.statement" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	moduleNode := node.ChildByFieldName("module_name")
	if moduleNode == nil {
		return nil
	}

	moduleName := nodeText(moduleNode, content)

	dependency := DependencyInfo{
		SourceFile:      analysis.FilePath,
		TargetModule:    moduleName,
		ImportedSymbols: []string{"*"}, // Wildcard import
		ImportType:      "wildcard_import",
		IsRelative:      strings.HasPrefix(moduleName, "."),
		Location:        location,
	}

	analysis.Dependencies = append(analysis.Dependencies, dependency)
	analysis.SymbolTable.Dependencies = append(analysis.SymbolTable.Dependencies, moduleName)

	return nil
}

// processRelativeImport processes relative imports
func (a *Analyzer) processRelativeImport(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "relative_import.statement" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract relative module information
	moduleNode := node.ChildByFieldName("module_name")
	var moduleName string
	if moduleNode != nil {
		if relativeNode := moduleNode.Child(0); relativeNode != nil && relativeNode.Type() == "relative_import" {
			moduleName = nodeText(moduleNode, content)
		}
	}

	dependency := DependencyInfo{
		SourceFile:   analysis.FilePath,
		TargetModule: moduleName,
		ImportType:   "relative_import",
		IsRelative:   true,
		Location:     location,
	}

	analysis.Dependencies = append(analysis.Dependencies, dependency)
	if moduleName != "" {
		analysis.SymbolTable.Dependencies = append(analysis.SymbolTable.Dependencies, moduleName)
	}

	return nil
}

// extractCalls extracts function/method calls using tree-sitter queries
func (a *Analyzer) extractCalls(tree *sitter.Tree, analysis *FileAnalysis) error {
	query := a.queries["python_calls"]
	qc := sitter.NewQueryCursor()
	defer qc.Close()

	qc.Exec(query, tree.RootNode())

	for {
		match, ok := qc.NextMatch()
		if !ok {
			break
		}

		for _, capture := range match.Captures {
			captureName := query.CaptureNameForId(capture.Index)
			node := capture.Node

			if err := a.processCallCapture(captureName, node, analysis); err != nil {
				return fmt.Errorf("failed to process call capture %s: %w", captureName, err)
			}
		}
	}

	return nil
}

// processCallCapture processes function call captures
func (a *Analyzer) processCallCapture(captureName string, node *sitter.Node, analysis *FileAnalysis) error {
	content := analysis.Content

	switch {
	case strings.HasPrefix(captureName, "call."):
		return a.processFunctionCall(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "constructor."):
		return a.processConstructorCall(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "super."):
		return a.processSuperCall(captureName, node, content, analysis)
	case strings.HasPrefix(captureName, "builtin."):
		return a.processBuiltinCall(captureName, node, content, analysis)
	}

	return nil
}

// processFunctionCall processes function calls
func (a *Analyzer) processFunctionCall(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if !strings.HasSuffix(captureName, ".simple") && !strings.HasSuffix(captureName, ".method") {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract call information
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil {
		return nil
	}

	var calleeName string
	if functionNode.Type() == "identifier" {
		// Simple function call
		calleeName = nodeText(functionNode, content)
	} else if functionNode.Type() == "attribute" {
		// Method call - extract the method name
		if attrNode := functionNode.ChildByFieldName("attribute"); attrNode != nil {
			calleeName = nodeText(attrNode, content)
		}
	}

	if calleeName == "" {
		return nil
	}

	// Find caller context
	callerID := a.findCallerContext(node, content, analysis.FilePath)

	callSite := CallSite{
		CallerID:   callerID,
		CalleeID:   "", // Will be resolved in Pass 2
		CalleeName: calleeName,
		Location:   location,
		IsResolved: false,
	}

	analysis.CallSites = append(analysis.CallSites, callSite)
	return nil
}

// processConstructorCall processes constructor calls (class instantiation)
func (a *Analyzer) processConstructorCall(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "constructor.call" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	// Extract class name
	functionNode := node.ChildByFieldName("function")
	if functionNode == nil || functionNode.Type() != "identifier" {
		return nil
	}

	className := nodeText(functionNode, content)
	callerID := a.findCallerContext(node, content, analysis.FilePath)

	callSite := CallSite{
		CallerID:   callerID,
		CalleeID:   "",        // Will be resolved in Pass 2
		CalleeName: className, // Constructor call to class
		Location:   location,
		IsResolved: false,
	}

	analysis.CallSites = append(analysis.CallSites, callSite)
	return nil
}

// processSuperCall processes super() calls
func (a *Analyzer) processSuperCall(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "super.call" {
		return nil
	}

	location := FileLocation{
		FilePath:  analysis.FilePath,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
		StartCol:  int(node.StartPoint().Column),
		EndCol:    int(node.EndPoint().Column),
	}

	callerID := a.findCallerContext(node, content, analysis.FilePath)

	callSite := CallSite{
		CallerID:   callerID,
		CalleeID:   "", // Will be resolved in Pass 2
		CalleeName: "super",
		Location:   location,
		IsResolved: false,
	}

	analysis.CallSites = append(analysis.CallSites, callSite)
	return nil
}

// processBuiltinCall processes built-in function calls
func (a *Analyzer) processBuiltinCall(captureName string, node *sitter.Node, content []byte, analysis *FileAnalysis) error {
	if captureName != "builtin.call" {
		return nil
	}

	// We don't need to track built-in function calls for semantic analysis
	// but we could add them for completeness
	return nil
}

// findCallerContext finds the function or method that contains the given call node
func (a *Analyzer) findCallerContext(node *sitter.Node, content []byte, filePath string) SymbolID {
	current := node.Parent()
	for current != nil {
		if current.Type() == "function_definition" {
			if nameNode := current.ChildByFieldName("name"); nameNode != nil {
				funcName := nodeText(nameNode, content)

				// Check if this function is inside a class
				className := a.findContainingClass(current, content)
				return GenerateSymbolID(filePath, funcName, className)
			}
		}
		current = current.Parent()
	}

	// If not in a function, it's module-level
	return GenerateSymbolID(filePath, "<module>", "")
}

// FileAnalysis contains the result of analyzing a single file
type FileAnalysis struct {
	FilePath     string           `json:"file_path"`
	Language     string           `json:"language"`
	SymbolTable  *SymbolTable     `json:"symbol_table"`
	Dependencies []DependencyInfo `json:"dependencies"`
	CallSites    []CallSite       `json:"call_sites"`
	AST          *sitter.Tree     `json:"-"` // Not serialized
	Content      []byte           `json:"-"` // Not serialized
	AnalysisTime time.Duration    `json:"analysis_time"`
}
