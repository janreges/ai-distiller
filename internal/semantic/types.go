package semantic

import (
	"encoding/json"
	"time"
)

// SymbolID uniquely identifies any symbol in the project
type SymbolID string

// SymbolKind represents the type of symbol
type SymbolKind string

const (
	SymbolKindFunction  SymbolKind = "function"
	SymbolKindMethod    SymbolKind = "method"
	SymbolKindClass     SymbolKind = "class"
	SymbolKindInterface SymbolKind = "interface"
	SymbolKindVariable  SymbolKind = "variable"
	SymbolKindParameter SymbolKind = "parameter"
	SymbolKindField     SymbolKind = "field"
	SymbolKindProperty  SymbolKind = "property"
	SymbolKindEnum      SymbolKind = "enum"
	SymbolKindEnumValue SymbolKind = "enum_value"
	SymbolKindModule    SymbolKind = "module"
	SymbolKindNamespace SymbolKind = "namespace"
	SymbolKindConstant  SymbolKind = "constant"
	SymbolKindType      SymbolKind = "type"
)

// FileLocation represents a location within a source file
type FileLocation struct {
	FilePath  string `json:"file_path"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	StartCol  int    `json:"start_col"`
	EndCol    int    `json:"end_col"`
}

// Symbol represents a single declared symbol (function, class, variable, etc.)
type Symbol struct {
	ID         SymbolID     `json:"id"`
	Name       string       `json:"name"`
	Kind       SymbolKind   `json:"kind"`
	Location   FileLocation `json:"location"`
	Scope      string       `json:"scope"`       // Parent scope (e.g., class name for methods)
	Signature  string       `json:"signature"`   // Function signature, type information
	Visibility string       `json:"visibility"`  // public, private, protected
	IsExported bool         `json:"is_exported"` // For modules/packages
	IsStatic   bool         `json:"is_static"`   // For methods and fields
	IsAbstract bool         `json:"is_abstract"` // For methods and classes
	Language   string       `json:"language"`    // Source language
	Metadata   SymbolMeta   `json:"metadata"`    // Additional language-specific data
}

// CallInfo represents one direct call the symbol performs
type CallInfo struct {
	FunctionName string `json:"function_name"`
	// In future: FilePath, ReceiverType, etc.
}

// SymbolMeta contains additional metadata for symbols
type SymbolMeta struct {
	ReturnType   string            `json:"return_type,omitempty"`
	Parameters   []ParameterInfo   `json:"parameters,omitempty"`
	Decorators   []string          `json:"decorators,omitempty"`    // Python decorators, Java annotations
	Extends      []string          `json:"extends,omitempty"`       // Base classes/interfaces
	Implements   []string          `json:"implements,omitempty"`    // Implemented interfaces
	DocString    string            `json:"doc_string,omitempty"`    // Documentation string
	LineCount    int               `json:"line_count,omitempty"`    // Size metric
	Complexity   int               `json:"complexity,omitempty"`    // Cyclomatic complexity
	CustomFields map[string]string `json:"custom_fields,omitempty"` // Language-specific fields
	Calls        []CallInfo        `json:"calls,omitempty"`         // Functions/methods called by this symbol
}

// ParameterInfo represents function/method parameter information
type ParameterInfo struct {
	Name         string `json:"name"`
	Type         string `json:"type,omitempty"`
	DefaultValue string `json:"default_value,omitempty"`
	IsOptional   bool   `json:"is_optional"`
	IsVariadic   bool   `json:"is_variadic"` // *args, **kwargs, ...rest
}

// TypeTracker tracks variable types and object instantiations
type TypeTracker struct {
	Variables map[string]string `json:"variables"` // variable name -> type name
	Types     map[string]string `json:"types"`     // type name -> scope/module
}

// SymbolTable contains symbols declared within a specific scope (e.g., a file)
type SymbolTable struct {
	FilePath     string             `json:"file_path"`
	Language     string             `json:"language"`
	Symbols      map[string]*Symbol `json:"symbols"`       // Maps symbol name to definition
	NestedScopes map[string]*Symbol `json:"nested_scopes"` // Maps scope name to its symbol
	Dependencies []string           `json:"dependencies"`  // List of imported file paths
	Exports      []string           `json:"exports"`       // List of exported symbol names
	TypeTracker  *TypeTracker       `json:"type_tracker"`  // NEW: Track variable types
	Timestamp    time.Time          `json:"timestamp"`     // When this table was created
}

// NewSymbolTable creates a new symbol table for a file
func NewSymbolTable(filePath, language string) *SymbolTable {
	return &SymbolTable{
		FilePath:     filePath,
		Language:     language,
		Symbols:      make(map[string]*Symbol),
		NestedScopes: make(map[string]*Symbol),
		Dependencies: make([]string, 0),
		Exports:      make([]string, 0),
		TypeTracker: &TypeTracker{
			Variables: make(map[string]string),
			Types:     make(map[string]string),
		},
		Timestamp: time.Now(),
	}
}

// AddSymbol adds a symbol to the table
func (st *SymbolTable) AddSymbol(symbol *Symbol) {
	st.Symbols[symbol.Name] = symbol

	// Track nested scopes (classes, modules, etc.)
	if symbol.Kind == SymbolKindClass || symbol.Kind == SymbolKindModule || symbol.Kind == SymbolKindNamespace {
		st.NestedScopes[symbol.Name] = symbol
	}
}

// GetSymbol retrieves a symbol by name
func (st *SymbolTable) GetSymbol(name string) (*Symbol, bool) {
	symbol, exists := st.Symbols[name]
	return symbol, exists
}

// GetSymbolsOfKind returns all symbols of a specific kind
func (st *SymbolTable) GetSymbolsOfKind(kind SymbolKind) []*Symbol {
	var symbols []*Symbol
	for _, symbol := range st.Symbols {
		if symbol.Kind == kind {
			symbols = append(symbols, symbol)
		}
	}
	return symbols
}

// DependencyInfo represents an import/dependency relationship
type DependencyInfo struct {
	SourceFile      string       `json:"source_file"`
	TargetModule    string       `json:"target_module"`
	ImportedSymbols []string     `json:"imported_symbols"` // empty if importing entire module
	ImportAlias     string       `json:"import_alias,omitempty"`
	ImportType      string       `json:"import_type"` // "import", "from_import", "require", etc.
	IsRelative      bool         `json:"is_relative"`
	Location        FileLocation `json:"location"`
}

// CallSite represents a function/method call location
type CallSite struct {
	CallerID   SymbolID     `json:"caller_id"`
	CalleeID   SymbolID     `json:"callee_id"`
	CalleeName string       `json:"callee_name"` // For unresolved calls
	Location   FileLocation `json:"location"`
	Arguments  []string     `json:"arguments,omitempty"` // Argument expressions
	IsResolved bool         `json:"is_resolved"`
}

// SemanticGraph contains all semantic information for a project
type SemanticGraph struct {
	FileSymbolTables map[string]*SymbolTable `json:"file_symbol_tables"` // map[filePath]SymbolTable
	CallSites        []CallSite              `json:"call_sites"`
	Dependencies     []DependencyInfo        `json:"dependencies"`
	CallGraph        map[SymbolID][]SymbolID `json:"call_graph"`       // Adjacency list: caller -> [callees]
	DependencyGraph  map[string][]string     `json:"dependency_graph"` // map[filePath][]dependencyPath
	ProjectRoot      string                  `json:"project_root"`
	Timestamp        time.Time               `json:"timestamp"`
	Statistics       SemanticStats           `json:"statistics"`
}

// SemanticStats provides statistics about the semantic analysis
type SemanticStats struct {
	TotalFiles        int                `json:"total_files"`
	TotalSymbols      int                `json:"total_symbols"`
	SymbolsByKind     map[SymbolKind]int `json:"symbols_by_kind"`
	TotalCallSites    int                `json:"total_call_sites"`
	ResolvedCalls     int                `json:"resolved_calls"`
	UnresolvedCalls   int                `json:"unresolved_calls"`
	TotalDependencies int                `json:"total_dependencies"`
	AnalysisTime      time.Duration      `json:"analysis_time"`
}

// NewSemanticGraph creates a new semantic graph
func NewSemanticGraph(projectRoot string) *SemanticGraph {
	return &SemanticGraph{
		FileSymbolTables: make(map[string]*SymbolTable),
		CallSites:        make([]CallSite, 0),
		Dependencies:     make([]DependencyInfo, 0),
		CallGraph:        make(map[SymbolID][]SymbolID),
		DependencyGraph:  make(map[string][]string),
		ProjectRoot:      projectRoot,
		Timestamp:        time.Now(),
		Statistics: SemanticStats{
			SymbolsByKind: make(map[SymbolKind]int),
		},
	}
}

// AddSymbolTable adds a symbol table for a file
func (sg *SemanticGraph) AddSymbolTable(table *SymbolTable) {
	sg.FileSymbolTables[table.FilePath] = table
	sg.updateStatistics()
}

// GetSymbolTable retrieves a symbol table for a file
func (sg *SemanticGraph) GetSymbolTable(filePath string) (*SymbolTable, bool) {
	table, exists := sg.FileSymbolTables[filePath]
	return table, exists
}

// AddCallSite adds a call site to the graph
func (sg *SemanticGraph) AddCallSite(callSite CallSite) {
	sg.CallSites = append(sg.CallSites, callSite)

	// Update call graph if call is resolved
	if callSite.IsResolved {
		if callees, exists := sg.CallGraph[callSite.CallerID]; exists {
			sg.CallGraph[callSite.CallerID] = append(callees, callSite.CalleeID)
		} else {
			sg.CallGraph[callSite.CallerID] = []SymbolID{callSite.CalleeID}
		}
	}
}

// AddDependency adds a dependency relationship
func (sg *SemanticGraph) AddDependency(dep DependencyInfo) {
	sg.Dependencies = append(sg.Dependencies, dep)

	// Update dependency graph
	if deps, exists := sg.DependencyGraph[dep.SourceFile]; exists {
		sg.DependencyGraph[dep.SourceFile] = append(deps, dep.TargetModule)
	} else {
		sg.DependencyGraph[dep.SourceFile] = []string{dep.TargetModule}
	}
}

// updateStatistics recalculates statistics
func (sg *SemanticGraph) updateStatistics() {
	stats := &sg.Statistics
	stats.TotalFiles = len(sg.FileSymbolTables)
	stats.TotalSymbols = 0
	stats.SymbolsByKind = make(map[SymbolKind]int)

	for _, table := range sg.FileSymbolTables {
		stats.TotalSymbols += len(table.Symbols)
		for _, symbol := range table.Symbols {
			stats.SymbolsByKind[symbol.Kind]++
		}
	}

	stats.TotalCallSites = len(sg.CallSites)
	stats.ResolvedCalls = 0
	stats.UnresolvedCalls = 0

	for _, callSite := range sg.CallSites {
		if callSite.IsResolved {
			stats.ResolvedCalls++
		} else {
			stats.UnresolvedCalls++
		}
	}

	stats.TotalDependencies = len(sg.Dependencies)
}

// FindSymbol searches for a symbol across all symbol tables
func (sg *SemanticGraph) FindSymbol(name string) []*Symbol {
	var found []*Symbol
	for _, table := range sg.FileSymbolTables {
		if symbol, exists := table.GetSymbol(name); exists {
			found = append(found, symbol)
		}
	}
	return found
}

// GetCallersOf returns all symbols that call the given symbol
func (sg *SemanticGraph) GetCallersOf(symbolID SymbolID) []SymbolID {
	var callers []SymbolID
	for caller, callees := range sg.CallGraph {
		for _, callee := range callees {
			if callee == symbolID {
				callers = append(callers, caller)
				break
			}
		}
	}
	return callers
}

// GetCalleesOf returns all symbols called by the given symbol
func (sg *SemanticGraph) GetCalleesOf(symbolID SymbolID) []SymbolID {
	if callees, exists := sg.CallGraph[symbolID]; exists {
		return callees
	}
	return []SymbolID{}
}

// ToJSON converts the semantic graph to JSON for serialization
func (sg *SemanticGraph) ToJSON() ([]byte, error) {
	return json.MarshalIndent(sg, "", "  ")
}

// FromJSON loads a semantic graph from JSON
func FromJSON(data []byte) (*SemanticGraph, error) {
	var sg SemanticGraph
	err := json.Unmarshal(data, &sg)
	return &sg, err
}

// GenerateSymbolID creates a unique identifier for a symbol
func GenerateSymbolID(filePath, symbolName, scope string) SymbolID {
	if scope != "" {
		return SymbolID(filePath + "::" + scope + "::" + symbolName)
	}
	return SymbolID(filePath + "::" + symbolName)
}
