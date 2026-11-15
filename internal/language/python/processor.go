package python

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/janreges/ai-distiller/internal/debug"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/parser"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/janreges/ai-distiller/internal/stripper"
)

// Processor implements the LanguageProcessor interface for Python
type Processor struct {
	processor.BaseProcessor
	wasmRuntime   *parser.WASMRuntime
	module        *parser.WASMModule
	useTreeSitter bool
}

// NewProcessor creates a new Python language processor
func NewProcessor() *Processor {
	return &Processor{
		BaseProcessor: processor.NewBaseProcessor(
			"python",
			"1.0.0",
			[]string{".py", ".pyw", ".pyi"},
		),
		useTreeSitter: true, // Default to tree-sitter parser
	}
}

// EnableTreeSitter enables tree-sitter based parsing
func (p *Processor) EnableTreeSitter() {
	p.useTreeSitter = true
}

// InitializeWASM sets up the WASM runtime and loads the Python parser
func (p *Processor) InitializeWASM(ctx context.Context, wasmBytes []byte) error {
	// Create WASM runtime if not exists
	if p.wasmRuntime == nil {
		runtime, err := parser.NewWASMRuntime(ctx)
		if err != nil {
			return fmt.Errorf("failed to create WASM runtime: %w", err)
		}
		p.wasmRuntime = runtime
	}

	// Load Python parser module
	if err := p.wasmRuntime.LoadModule("tree-sitter-python", wasmBytes); err != nil {
		return fmt.Errorf("failed to load Python parser: %w", err)
	}

	// Get the module
	module, err := p.wasmRuntime.GetModule("tree-sitter-python")
	if err != nil {
		return fmt.Errorf("failed to get Python module: %w", err)
	}

	p.module = module
	return nil
}

// Process parses Python source code and returns the IR representation
func (p *Processor) Process(ctx context.Context, reader io.Reader, filename string) (*ir.DistilledFile, error) {
	// Default options
	opts := processor.DefaultProcessOptions()
	return p.ProcessWithOptions(ctx, reader, filename, opts)
}

// ProcessFile processes a file by path
func (p *Processor) ProcessFile(filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	// Read the actual file
	source, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	ctx := context.Background()

	// Try tree-sitter first if enabled
	if p.useTreeSitter {
		processor, err := NewNativeTreeSitterProcessor()
		if err == nil {
			defer processor.parser.Close()
			file, err := processor.ProcessSource(ctx, source, filename)
			if err == nil {
				// Apply stripper if any options are set
				stripperOpts := opts.ToStripperOptions()

				// Only strip if there's something to strip
				if stripperOpts.HasAnyOption() {

					s := stripper.New(stripperOpts)
					stripped := file.Accept(s)
					if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
						return strippedFile, nil
					}
				}

				return file, nil
			}
			// Fall back to line-based parser on error
		}
	}

	// Use line-based parser as fallback
	return p.parseActualFile(ctx, source, filename, opts)
}

// ProcessWithOptions parses with specific options
func (p *Processor) ProcessWithOptions(ctx context.Context, reader io.Reader, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	dbg := debug.FromContext(ctx).WithSubsystem("python")
	defer dbg.Timing(debug.LevelDetailed, "ProcessWithOptions")()

	// Read source code
	source, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read source: %w", err)
	}

	dbg.Logf(debug.LevelDetailed, "Processing %s (%d bytes)", filename, len(source))

	// Try tree-sitter first if enabled
	if p.useTreeSitter {
		dbg.Logf(debug.LevelDetailed, "Using tree-sitter parser")
		processor, err := NewNativeTreeSitterProcessor()
		if err == nil {
			defer processor.parser.Close()
			file, err := processor.ProcessSource(ctx, source, filename)
			if err == nil {
				dbg.Logf(debug.LevelDetailed, "Tree-sitter parsing successful")
				// Apply stripper if any options are set
				stripperOpts := opts.ToStripperOptions()

				// Only strip if there's something to strip
				if stripperOpts.HasAnyOption() {

					s := stripper.New(stripperOpts)
					stripped := file.Accept(s)
					if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
						return strippedFile, nil
					}
				}

				return file, nil
			}
			// Fall back to line-based parser on error
		}
	}

	// If no WASM module loaded, use line-based parser
	if p.module == nil {
		return p.parseActualFile(ctx, source, filename, opts)
	}

	// Create parser
	tsParser, err := parser.NewParser(p.module)
	if err != nil {
		return nil, fmt.Errorf("failed to create parser: %w", err)
	}
	defer tsParser.Delete()

	// Parse the source
	tree, err := tsParser.Parse(ctx, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse: %w", err)
	}
	defer tree.Delete()

	// Convert to IR
	converter := parser.NewConverter("python", source)
	file, err := converter.ConvertTree(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to IR: %w", err)
	}

	// Apply stripper if any options are set
	stripperOpts := opts.ToStripperOptions()
	if stripperOpts.HasAnyOption() {
		s := stripper.New(stripperOpts)
		stripped := file.Accept(s)
		if strippedFile, ok := stripped.(*ir.DistilledFile); ok {
			file = strippedFile
		}
	}

	file.Path = filename
	return file, nil
}

// processMock creates a mock IR for testing without WASM
func (p *Processor) processMock(ctx context.Context, source []byte, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	// Create a mock file structure
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine:   1,
				StartColumn: 1,
				EndLine:     countLines(source),
				EndColumn:   1,
			},
		},
		Path:     filename,
		Language: "python",
		Version:  "2.0.0",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Parse Python-specific constructs (simplified mock)
	// In real implementation, this would use tree-sitter
	nodes := p.mockParsePython(source, opts)
	file.Children = nodes

	return file, nil
}

// mockParsePython creates mock nodes for testing
func (p *Processor) mockParsePython(source []byte, opts processor.ProcessOptions) []ir.DistilledNode {
	nodes := []ir.DistilledNode{}

	// Mock: Add a module-level comment
	if opts.IncludeComments {
		nodes = append(nodes, createMockComment("Python source file"))
	}

	// Mock: Add an import
	if opts.IncludeImports {
		nodes = append(nodes, createMockImport("typing", []string{"List", "Dict", "Optional"}))
	}

	// Mock: Add a class
	classNode := createMockClass("ExampleClass", "public")

	// Mock: Add a method to the class
	methodNode := createMockFunction("example_method", "public", []string{"self", "arg1: str", "arg2: int"}, "str")
	classNode.Children = append(classNode.Children, methodNode)

	nodes = append(nodes, classNode)

	// Mock: Add a function
	if opts.IncludePrivate || !isPrivate("process_data") {
		funcNode := createMockFunction("process_data", "public", []string{"data: List[Dict]"}, "Dict")
		nodes = append(nodes, funcNode)
	}

	return nodes
}

// extractDocstringFromImplementation extracts docstring from function implementation
func extractDocstringFromImplementation(implementation string) string {
	if implementation == "" {
		return ""
	}

	lines := strings.Split(implementation, "\n")
	if len(lines) == 0 {
		return ""
	}

	// Find the first non-empty line after any opening braces
	startLine := -1
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && trimmed != "{" {
			startLine = i
			break
		}
	}

	if startLine == -1 {
		return ""
	}

	// Check if first meaningful line is a docstring
	firstLine := strings.TrimSpace(lines[startLine])
	if !strings.HasPrefix(firstLine, `"""`) && !strings.HasPrefix(firstLine, `'''`) {
		return ""
	}

	// Find the end of the docstring
	quote := `"""`
	if strings.HasPrefix(firstLine, `'''`) {
		quote = `'''`
	}

	// Handle single-line docstring
	if strings.Count(firstLine, quote) >= 2 {
		return strings.TrimSpace(firstLine)
	}

	// Handle multi-line docstring
	var docstringLines []string
	docstringLines = append(docstringLines, lines[startLine])

	for i := startLine + 1; i < len(lines); i++ {
		docstringLines = append(docstringLines, lines[i])
		if strings.Contains(lines[i], quote) {
			break
		}
	}

	return strings.Join(docstringLines, "\n")
}

// Helper functions

func countLines(source []byte) int {
	if len(source) == 0 {
		return 0
	}
	lines := 1
	for _, b := range source {
		if b == '\n' {
			lines++
		}
	}
	return lines
}

func isPrivate(name string) bool {
	// In Python, names starting with _ are considered private
	// BUT dunder methods (like __init__, __repr__) are public API
	if len(name) == 0 {
		return false
	}

	// Dunder methods are public
	if strings.HasPrefix(name, "__") && strings.HasSuffix(name, "__") {
		return false
	}

	// Single underscore prefix means private
	return name[0] == '_'
}

// Mock node creators for testing

func createMockComment(text string) *ir.DistilledComment {
	return &ir.DistilledComment{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: 1, EndLine: 1},
		},
		Text:   text,
		Format: ir.CommentLine,
	}
}

func createMockImport(module string, names []string) *ir.DistilledImport {
	symbols := make([]ir.ImportedSymbol, len(names))
	for i, name := range names {
		symbols[i] = ir.ImportedSymbol{
			Name:  name,
			Alias: "",
		}
	}

	return &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: 2, EndLine: 2},
		},
		ImportType: "from",
		Module:     module,
		Symbols:    symbols,
	}
}

func createMockClass(name string, visibility string) *ir.DistilledClass {
	return &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: 4, EndLine: 10},
		},
		Name:       name,
		Visibility: ir.Visibility(visibility),
		Decorators: []string{},
		TypeParams: []ir.TypeParam{},
		Extends:    []ir.TypeRef{},
		Implements: []ir.TypeRef{},
		Children:   []ir.DistilledNode{},
	}
}

func createMockFunction(name string, visibility string, params []string, returnType string) *ir.DistilledFunction {
	parameters := make([]ir.Parameter, len(params))
	for i, param := range params {
		parameters[i] = ir.Parameter{
			Name: param,
			Type: ir.TypeRef{Name: "Any"},
		}
	}

	return &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: 5, EndLine: 8},
		},
		Name:       name,
		Visibility: ir.Visibility(visibility),
		Modifiers:  []ir.Modifier{},
		Parameters: parameters,
		Returns: &ir.TypeRef{
			Name: returnType,
		},
		Decorators:     []string{},
		Implementation: "# Implementation details omitted",
	}
}

// parseActualFile parses actual Python source code
func (p *Processor) parseActualFile(ctx context.Context, source []byte, filename string, opts processor.ProcessOptions) (*ir.DistilledFile, error) {
	lineCount := countLines(source)
	if lineCount == 0 {
		lineCount = 1 // Ensure at least 1 line
	}

	// Create the root file node
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   lineCount,
			},
		},
		Path:     filename,
		Language: "python",
		Version:  "3.x",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	// Create error collector
	errorCollector := NewErrorCollector()

	// Simple line-based parser with error recovery
	// This is a simplified parser that extracts basic Python constructs
	lines := strings.Split(string(source), "\n")

	for i := 0; i < len(lines); i++ {
		// Check for indentation errors
		if indentErr := detectIndentationError(lines, i); indentErr != nil {
			if indentErr.Severity == "error" {
				errorCollector.AddError(indentErr.Line, indentErr.Column, indentErr.Message, indentErr.Kind)
			} else {
				errorCollector.AddWarning(indentErr.Line, indentErr.Column, indentErr.Message, indentErr.Kind)
			}
		}

		line := strings.TrimSpace(lines[i])

		// Skip empty lines
		if line == "" {
			continue
		}

		// Parse imports
		if strings.HasPrefix(line, "import ") || strings.HasPrefix(line, "from ") {
			if opts.IncludeImports {
				imp, err := p.parseImportWithRecovery(line, i+1, errorCollector)
				if err != nil {
					errorCollector.AddError(i+1, 1, err.Error(), ErrorKindSyntax)
				}
				if imp != nil {
					file.Children = append(file.Children, imp)
				}
			}
		}

		// Parse class definitions
		if strings.HasPrefix(line, "class ") {
			class, endLine, err := p.parseClassWithRecovery(lines, i, opts, errorCollector)
			if err != nil {
				errorCollector.AddError(i+1, 1, err.Error(), ErrorKindSyntax)
				// Try to recover
				i = tryRecoverFromError(lines, i, ErrorKindIncomplete) - 1
			} else if class != nil {
				file.Children = append(file.Children, class)
				i = endLine - 1 // Skip to end of class
			}
		}

		// Parse function definitions (top-level)
		if strings.HasPrefix(line, "def ") && !strings.HasPrefix(lines[i], "    ") {
			fn, endLine, err := p.parseFunctionWithRecovery(lines, i, opts, errorCollector)
			if err != nil {
				errorCollector.AddError(i+1, 1, err.Error(), ErrorKindSyntax)
				// Try to recover
				i = tryRecoverFromError(lines, i, ErrorKindIncomplete) - 1
			} else if fn != nil {
				file.Children = append(file.Children, fn)
				i = endLine - 1 // Skip to end of function
			}
		}

		// Parse async function definitions
		if strings.HasPrefix(line, "async def ") && !strings.HasPrefix(lines[i], "    ") {
			fn, endLine, err := p.parseFunctionWithRecovery(lines, i, opts, errorCollector)
			if err != nil {
				errorCollector.AddError(i+1, 1, err.Error(), ErrorKindSyntax)
				// Try to recover
				i = tryRecoverFromError(lines, i, ErrorKindIncomplete) - 1
			} else if fn != nil {
				if fnNode, ok := fn.(*ir.DistilledFunction); ok {
					fnNode.Modifiers = append(fnNode.Modifiers, ir.ModifierAsync)
				}
				file.Children = append(file.Children, fn)
				i = endLine - 1
			}
		}
	}

	// Add collected errors to the file
	file.Errors = errorCollector.ToDistilledErrors()

	return file, nil
}

// parseImportWithRecovery parses import statements with error recovery
func (p *Processor) parseImportWithRecovery(line string, lineNum int, errorCollector *ErrorCollector) (*ir.DistilledImport, error) {
	imp, err := p.parseImportInternal(line, lineNum)
	return imp, err
}

// parseImport parses import statements (legacy interface)
func (p *Processor) parseImport(line string, lineNum int) *ir.DistilledImport {
	imp, _ := p.parseImportInternal(line, lineNum)
	return imp
}

// parseImportInternal parses import statements
func (p *Processor) parseImportInternal(line string, lineNum int) (*ir.DistilledImport, error) {
	imp := &ir.DistilledImport{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: lineNum, EndLine: lineNum},
		},
	}

	if strings.HasPrefix(line, "from ") {
		// Parse "from X import Y, Z"
		parts := strings.Split(line[5:], " import ")
		if len(parts) == 2 {
			imp.ImportType = "from"
			imp.Module = strings.TrimSpace(parts[0])

			// Parse imported symbols
			symbols := strings.Split(parts[1], ",")
			for _, sym := range symbols {
				sym = strings.TrimSpace(sym)
				if sym == "" {
					continue
				}

				// Handle "X as Y"
				if strings.Contains(sym, " as ") {
					aliasParts := strings.Split(sym, " as ")
					imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
						Name:  strings.TrimSpace(aliasParts[0]),
						Alias: strings.TrimSpace(aliasParts[1]),
					})
				} else {
					imp.Symbols = append(imp.Symbols, ir.ImportedSymbol{
						Name: sym,
					})
				}
			}
		}
	} else if strings.HasPrefix(line, "import ") {
		// Parse "import X" or "import X as Y"
		module := strings.TrimSpace(line[7:])
		imp.ImportType = "import"

		if strings.Contains(module, " as ") {
			parts := strings.Split(module, " as ")
			imp.Module = strings.TrimSpace(parts[0])
			imp.Symbols = []ir.ImportedSymbol{{
				Name:  strings.TrimSpace(parts[0]),
				Alias: strings.TrimSpace(parts[1]),
			}}
		} else {
			imp.Module = module
		}
	}

	// Validate the import
	if imp.Module == "" && len(imp.Symbols) == 0 {
		return nil, fmt.Errorf("invalid import statement: %s", line)
	}

	return imp, nil
}

// parseClassWithRecovery parses class definitions with error recovery
func (p *Processor) parseClassWithRecovery(lines []string, startIdx int, opts processor.ProcessOptions, errorCollector *ErrorCollector) (ir.DistilledNode, int, error) {
	node, endLine := p.parseClass(lines, startIdx, opts)
	if node == nil {
		return nil, startIdx + 1, fmt.Errorf("failed to parse class definition")
	}

	// Check for common errors
	if class, ok := node.(*ir.DistilledClass); ok {
		if err := validatePythonName(class.Name); err != nil {
			// Don't return the node if it has invalid name
			return nil, endLine, fmt.Errorf("invalid class name '%s': %w", class.Name, err)
		}
	}

	return node, endLine, nil
}

// parseClass parses class definitions
func (p *Processor) parseClass(lines []string, startIdx int, opts processor.ProcessOptions) (ir.DistilledNode, int) {
	line := strings.TrimSpace(lines[startIdx])

	// Extract class name and inheritance
	classLine := line[6:] // Remove "class "
	className := ""
	var extends []ir.TypeRef

	if idx := strings.Index(classLine, "("); idx > 0 {
		className = strings.TrimSpace(classLine[:idx])
		// Parse base classes
		if endIdx := strings.Index(classLine[idx:], ")"); endIdx > 0 {
			bases := classLine[idx+1 : idx+endIdx]
			for _, base := range strings.Split(bases, ",") {
				base = strings.TrimSpace(base)
				if base != "" && base != "object" {
					extends = append(extends, ir.TypeRef{Name: base})
				}
			}
		}
	} else if idx := strings.Index(classLine, ":"); idx > 0 {
		className = strings.TrimSpace(classLine[:idx])
	} else {
		className = strings.TrimSpace(classLine)
	}

	// Check if private
	visibility := ir.VisibilityPublic
	if strings.HasPrefix(className, "_") {
		visibility = ir.VisibilityPrivate
	}

	class := &ir.DistilledClass{
		BaseNode: ir.BaseNode{
			Location: ir.Location{StartLine: startIdx + 1},
		},
		Name:       className,
		Visibility: visibility,
		Extends:    extends,
		Children:   []ir.DistilledNode{},
	}

	// Find end of class and parse methods
	endLine := p.findBlockEnd(lines, startIdx)
	// Ensure endLine is at least startIdx + 1
	if endLine <= startIdx {
		endLine = startIdx + 1
	}
	class.BaseNode.Location.EndLine = endLine

	// Parse class body
	for i := startIdx + 1; i < endLine; i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// Skip empty lines and comments
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Parse methods (must be indented)
		if strings.HasPrefix(line, "    def ") || strings.HasPrefix(line, "    async def ") {
			if method, methodEnd := p.parseFunction(lines, i, opts); method != nil {
				class.Children = append(class.Children, method)
				i = methodEnd - 1
			}
		}

		// Parse decorators like @property, @staticmethod, etc.
		if strings.HasPrefix(line, "    @") {
			// Look for the function after decorators
			j := i + 1
			for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "@") {
				j++
			}
			if j < len(lines) && (strings.Contains(lines[j], "def ")) {
				if method, methodEnd := p.parseFunction(lines, j, opts); method != nil {
					// Add decorators
					for k := i; k < j; k++ {
						decorator := strings.TrimSpace(lines[k])[1:] // Remove @
						if fn, ok := method.(*ir.DistilledFunction); ok {
							fn.Decorators = append(fn.Decorators, decorator)
						}
					}
					class.Children = append(class.Children, method)
					i = methodEnd - 1
				}
			}
		}
	}

	return class, endLine
}

// parseFunctionWithRecovery parses function definitions with error recovery
func (p *Processor) parseFunctionWithRecovery(lines []string, startIdx int, opts processor.ProcessOptions, errorCollector *ErrorCollector) (ir.DistilledNode, int, error) {
	node, endLine := p.parseFunction(lines, startIdx, opts)
	if node == nil {
		return nil, startIdx + 1, fmt.Errorf("failed to parse function definition")
	}

	// Check for common errors
	if fn, ok := node.(*ir.DistilledFunction); ok {
		if err := validatePythonName(fn.Name); err != nil {
			// Don't return the node if it has invalid name
			return nil, endLine, fmt.Errorf("invalid function name '%s': %w", fn.Name, err)
		}

		// Check for unclosed parentheses
		line := strings.TrimSpace(lines[startIdx])
		if col, hasUnclosed := findUnclosedParenthesis(line); hasUnclosed {
			errorCollector.AddWarning(startIdx+1, col+1, "unclosed parenthesis", ErrorKindUnclosedExpr)
		}
	}

	return node, endLine, nil
}

// parseFunction parses function definitions
func (p *Processor) parseFunction(lines []string, startIdx int, opts processor.ProcessOptions) (ir.DistilledNode, int) {
	line := strings.TrimSpace(lines[startIdx])

	// Handle async functions
	isAsync := false
	if strings.HasPrefix(line, "async def ") {
		isAsync = true
		line = strings.TrimSpace(line[6:]) // Remove "async "
	}

	// Extract function signature
	if !strings.HasPrefix(line, "def ") {
		return nil, startIdx
	}

	line = line[4:] // Remove "def "

	// Find function name and parameters
	parenIdx := strings.Index(line, "(")
	if parenIdx < 0 {
		return nil, startIdx
	}

	funcName := strings.TrimSpace(line[:parenIdx])

	// Check visibility
	visibility := ir.VisibilityPublic
	if strings.HasPrefix(funcName, "_") && funcName != "__init__" {
		visibility = ir.VisibilityPrivate
	}

	// Find end of function
	endLine := p.findBlockEnd(lines, startIdx)
	// Ensure endLine is at least startIdx + 1
	if endLine <= startIdx {
		endLine = startIdx + 1
	}

	fn := &ir.DistilledFunction{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: startIdx + 1,
				EndLine:   endLine,
			},
		},
		Name:       funcName,
		Visibility: visibility,
		Parameters: []ir.Parameter{},
		Modifiers:  []ir.Modifier{},
	}

	if isAsync {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAsync)
	}

	// Parse parameters (simplified)
	if endParenIdx := strings.Index(line[parenIdx:], ")"); endParenIdx > 0 {
		params := line[parenIdx+1 : parenIdx+endParenIdx]
		if params != "" {
			for _, param := range strings.Split(params, ",") {
				param = strings.TrimSpace(param)
				if param == "" {
					continue
				}

				// Simple parameter parsing
				paramName := param
				paramType := ""

				// Handle type annotations
				if colonIdx := strings.Index(param, ":"); colonIdx > 0 {
					paramName = strings.TrimSpace(param[:colonIdx])
					paramType = strings.TrimSpace(param[colonIdx+1:])

					// Handle default values
					if eqIdx := strings.Index(paramType, "="); eqIdx > 0 {
						paramType = strings.TrimSpace(paramType[:eqIdx])
					}
				} else if eqIdx := strings.Index(param, "="); eqIdx > 0 {
					paramName = strings.TrimSpace(param[:eqIdx])
				}

				p := ir.Parameter{Name: paramName}
				if paramType != "" {
					p.Type = ir.TypeRef{Name: paramType}
				}
				fn.Parameters = append(fn.Parameters, p)
			}
		}

		// Check for return type
		remaining := line[parenIdx+endParenIdx+1:]
		if arrowIdx := strings.Index(remaining, "->"); arrowIdx >= 0 {
			retType := strings.TrimSpace(remaining[arrowIdx+2:])
			if colonIdx := strings.Index(retType, ":"); colonIdx > 0 {
				retType = strings.TrimSpace(retType[:colonIdx])
			}
			if retType != "" {
				fn.Returns = &ir.TypeRef{Name: retType}
			}
		}
	}

	// Get implementation if requested
	if opts.IncludeImplementation && endLine > startIdx+1 {
		var implLines []string
		for i := startIdx + 1; i < endLine; i++ {
			implLines = append(implLines, lines[i])
		}
		fn.Implementation = strings.Join(implLines, "\n")
	}

	return fn, endLine
}

// findBlockEnd finds the end of a code block (class or function)
func (p *Processor) findBlockEnd(lines []string, startIdx int) int {
	if startIdx >= len(lines) {
		return startIdx
	}

	// Get the indentation level of the definition line
	defLine := lines[startIdx]
	baseIndent := len(defLine) - len(strings.TrimLeft(defLine, " \t"))

	// Find where the block ends
	for i := startIdx + 1; i < len(lines); i++ {
		line := lines[i]

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check indentation
		currentIndent := len(line) - len(strings.TrimLeft(line, " \t"))

		// If we find a line with same or less indentation (and it's not empty), block ends
		if currentIndent <= baseIndent && strings.TrimSpace(line) != "" {
			return i
		}
	}

	return len(lines)
}
