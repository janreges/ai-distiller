package ir

// Package-level nodes

// DistilledPackage represents a package/module declaration
type DistilledPackage struct {
	BaseNode
	Name     string          `json:"name"`
	Children []DistilledNode `json:"children,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledPackage) GetNodeKind() NodeKind {
	return KindPackage
}

// GetChildren implements DistilledNode
func (n *DistilledPackage) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode
func (n *DistilledPackage) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// Import nodes

// DistilledImport represents an import statement
type DistilledImport struct {
	BaseNode
	ImportType string           `json:"import_type"` // "import", "from", "require", etc.
	Module     string           `json:"module"`
	Symbols    []ImportedSymbol `json:"symbols,omitempty"`
	IsType     bool             `json:"is_type,omitempty"`
}

// ImportedSymbol represents an imported symbol
type ImportedSymbol struct {
	Name  string `json:"name"`
	Alias string `json:"alias,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledImport) GetNodeKind() NodeKind {
	return KindImport
}

// GetChildren implements DistilledNode
func (n *DistilledImport) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledImport) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// Type nodes

// DistilledClass represents a class/interface declaration
type DistilledClass struct {
	BaseNode
	Name        string           `json:"name"`
	Visibility  Visibility       `json:"visibility"`
	Modifiers   []Modifier       `json:"modifiers,omitempty"`
	Decorators  []string         `json:"decorators,omitempty"`
	TypeParams  []TypeParam      `json:"type_params,omitempty"`
	Extends     []TypeRef        `json:"extends,omitempty"`
	Implements  []TypeRef        `json:"implements,omitempty"`
	Mixins      []TypeRef        `json:"mixins,omitempty"`
	Children    []DistilledNode  `json:"children,omitempty"`
	Deprecated  *DeprecationInfo `json:"deprecated,omitempty"`
	Description string           `json:"description,omitempty"`
	APIDocblock string           `json:"api_docblock,omitempty"` // PHP: Docblock with @property/@method tags
}

// GetNodeKind implements DistilledNode
func (n *DistilledClass) GetNodeKind() NodeKind {
	return KindClass
}

// GetChildren implements DistilledNode
func (n *DistilledClass) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode
func (n *DistilledClass) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledInterface represents an interface declaration
type DistilledInterface struct {
	BaseNode
	Name       string          `json:"name"`
	Visibility Visibility      `json:"visibility"`
	Modifiers  []Modifier      `json:"modifiers,omitempty"`
	TypeParams []TypeParam     `json:"type_params,omitempty"`
	Extends    []TypeRef       `json:"extends,omitempty"`
	Permits    []TypeRef       `json:"permits,omitempty"`
	Children   []DistilledNode `json:"children,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledInterface) GetNodeKind() NodeKind {
	return KindInterface
}

// GetChildren implements DistilledNode
func (n *DistilledInterface) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode
func (n *DistilledInterface) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledStruct represents a struct declaration
type DistilledStruct struct {
	BaseNode
	Name       string          `json:"name"`
	Visibility Visibility      `json:"visibility"`
	TypeParams []TypeParam     `json:"type_params,omitempty"`
	Children   []DistilledNode `json:"children,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledStruct) GetNodeKind() NodeKind {
	return KindStruct
}

// GetChildren implements DistilledNode
func (n *DistilledStruct) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode
func (n *DistilledStruct) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledEnum represents an enum declaration
type DistilledEnum struct {
	BaseNode
	Name       string          `json:"name"`
	Visibility Visibility      `json:"visibility"`
	Type       *TypeRef        `json:"type,omitempty"`
	Children   []DistilledNode `json:"children,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledEnum) GetNodeKind() NodeKind {
	return KindEnum
}

// GetChildren implements DistilledNode
func (n *DistilledEnum) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode
func (n *DistilledEnum) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledTypeAlias represents a type alias
type DistilledTypeAlias struct {
	BaseNode
	Name       string      `json:"name"`
	Visibility Visibility  `json:"visibility"`
	Modifiers  []Modifier  `json:"modifiers,omitempty"`
	TypeParams []TypeParam `json:"type_params,omitempty"`
	Type       TypeRef     `json:"type"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledTypeAlias) GetNodeKind() NodeKind {
	return KindTypeAlias
}

// GetChildren implements DistilledNode
func (n *DistilledTypeAlias) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledTypeAlias) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// Member nodes

// DistilledFunction represents a function/method declaration
type DistilledFunction struct {
	BaseNode
	Name           string           `json:"name"`
	Visibility     Visibility       `json:"visibility"`
	Modifiers      []Modifier       `json:"modifiers,omitempty"`
	Decorators     []string         `json:"decorators,omitempty"`
	TypeParams     []TypeParam      `json:"type_params,omitempty"`
	Parameters     []Parameter      `json:"parameters"`
	Returns        *TypeRef         `json:"returns,omitempty"`
	Throws         []TypeRef        `json:"throws,omitempty"`
	Implementation string           `json:"implementation,omitempty"`
	ThrowsInfo     []ThrowsInfo     `json:"throws_info,omitempty"`
	Deprecated     *DeprecationInfo `json:"deprecated,omitempty"`
	Description    string           `json:"description,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledFunction) GetNodeKind() NodeKind {
	return KindFunction
}

// GetChildren implements DistilledNode
func (n *DistilledFunction) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledFunction) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledField represents a field/property declaration
type DistilledField struct {
	BaseNode
	Name         string     `json:"name"`
	Visibility   Visibility `json:"visibility"`
	Modifiers    []Modifier `json:"modifiers,omitempty"`
	Type         *TypeRef   `json:"type,omitempty"`
	DefaultValue string     `json:"default_value,omitempty"`
	Decorators   []string   `json:"decorators,omitempty"`
	// Property-specific fields (mainly for C#)
	IsProperty       bool        `json:"is_property,omitempty"`
	HasGetter        bool        `json:"has_getter,omitempty"`
	HasSetter        bool        `json:"has_setter,omitempty"`
	GetterVisibility *Visibility `json:"getter_visibility,omitempty"`
	SetterVisibility *Visibility `json:"setter_visibility,omitempty"`
	// PSR-19 support fields
	Description string           `json:"description,omitempty"`
	Deprecated  *DeprecationInfo `json:"deprecated,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledField) GetNodeKind() NodeKind {
	return KindField
}

// GetChildren implements DistilledNode
func (n *DistilledField) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledField) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// Documentation nodes

// DistilledComment represents a comment
type DistilledComment struct {
	BaseNode
	Text   string `json:"text"`
	Format string `json:"format"` // CommentLine, CommentBlock, CommentDoc (comment.go)
	// Inner marks a doc comment that documents its enclosing container (the
	// file, module or type it sits in, like Rust //! and /*!), not the
	// declaration after it.
	Inner bool `json:"inner,omitempty"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledComment) GetNodeKind() NodeKind {
	return KindComment
}

// GetChildren implements DistilledNode
func (n *DistilledComment) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledComment) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// DistilledRawContent represents raw text content without parsing
type DistilledRawContent struct {
	BaseNode
	Content string `json:"content"`
}

// GetNodeKind implements DistilledNode
func (n *DistilledRawContent) GetNodeKind() NodeKind {
	return KindRawContent
}

// GetChildren implements DistilledNode
func (n *DistilledRawContent) GetChildren() []DistilledNode {
	return nil
}

// Accept implements DistilledNode
func (n *DistilledRawContent) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// Type system helpers

// TypeRef represents a type reference
type TypeRef struct {
	Name       string    `json:"name"`
	Package    string    `json:"package,omitempty"`
	TypeArgs   []TypeRef `json:"type_args,omitempty"`
	IsNullable bool      `json:"is_nullable,omitempty"`
	IsArray    bool      `json:"is_array,omitempty"`
	ArrayDims  int       `json:"array_dims,omitempty"`
}

// TypeParam represents a type parameter
type TypeParam struct {
	Name        string    `json:"name"`
	Constraints []TypeRef `json:"constraints,omitempty"`
	Default     *TypeRef  `json:"default,omitempty"`
}

// Parameter represents a function parameter
type Parameter struct {
	Name         string   `json:"name"`
	Type         TypeRef  `json:"type"`
	DefaultValue string   `json:"default_value,omitempty"`
	IsVariadic   bool     `json:"is_variadic,omitempty"`
	IsOptional   bool     `json:"is_optional,omitempty"`
	Decorators   []string `json:"decorators,omitempty"`
}
