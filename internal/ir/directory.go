package ir

import "encoding/json"

// DistilledDirectory represents a directory containing distilled files
type DistilledDirectory struct {
	BaseNode
	Path     string          `json:"path"`
	Children []DistilledNode `json:"children"`
}

// GetNodeKind implements DistilledNode for DistilledDirectory
func (n *DistilledDirectory) GetNodeKind() NodeKind {
	return KindDirectory
}

// GetChildren implements DistilledNode for DistilledDirectory
func (n *DistilledDirectory) GetChildren() []DistilledNode {
	return n.Children
}

// Accept implements DistilledNode for DistilledDirectory
func (n *DistilledDirectory) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

// MarshalJSON implements json.Marshaler for DistilledDirectory
func (n *DistilledDirectory) MarshalJSON() ([]byte, error) {
	type Alias DistilledDirectory
	return json.Marshal(&struct {
		Kind string `json:"kind"`
		*Alias
	}{
		Kind:  string(n.GetNodeKind()),
		Alias: (*Alias)(n),
	})
}
