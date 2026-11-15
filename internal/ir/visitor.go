package ir

// Visitor defines the visitor pattern interface
type Visitor interface {
	// Visit processes a node and returns the (potentially modified) node
	// Returning nil removes the node from the tree
	Visit(node DistilledNode) DistilledNode
}

// BaseVisitor provides a default implementation that visits children
type BaseVisitor struct{}

// Visit implements Visitor with default behavior (no transformation)
func (v *BaseVisitor) Visit(node DistilledNode) DistilledNode {
	return node
}

// Walker traverses an IR tree using a visitor
type Walker struct {
	visitor Visitor
}

// NewWalker creates a new tree walker
func NewWalker(visitor Visitor) *Walker {
	return &Walker{visitor: visitor}
}

// Walk traverses the tree starting from the given node
func (w *Walker) Walk(node DistilledNode) DistilledNode {
	if node == nil {
		return nil
	}

	// Visit the node
	result := w.visitor.Visit(node)
	if result == nil {
		return nil
	}

	// Visit children if the node has any
	if children := node.GetChildren(); len(children) > 0 {
		newChildren := make([]DistilledNode, 0, len(children))
		for _, child := range children {
			if newChild := w.Walk(child); newChild != nil {
				newChildren = append(newChildren, newChild)
			}
		}

		// Update children in the result node
		// This requires type switching since we need to maintain immutability
		result = w.updateChildren(result, newChildren)
	}

	return result
}

// updateChildren creates a new node with updated children
func (w *Walker) updateChildren(node DistilledNode, children []DistilledNode) DistilledNode {
	switch n := node.(type) {
	case *DistilledFile:
		// Create a new file node with updated children
		newFile := *n
		newFile.Children = children
		return &newFile

	case *DistilledPackage:
		// Create a new package node with updated children
		newPackage := *n
		newPackage.Children = children
		return &newPackage

	case *DistilledClass:
		// Create a new class node with updated children
		newClass := *n
		newClass.Children = children
		return &newClass

	case *DistilledInterface:
		// Create a new interface node with updated children
		newInterface := *n
		newInterface.Children = children
		return &newInterface

	case *DistilledStruct:
		// Create a new struct node with updated children
		newStruct := *n
		newStruct.Children = children
		return &newStruct

	case *DistilledEnum:
		// Create a new enum node with updated children
		newEnum := *n
		newEnum.Children = children
		return &newEnum

	default:
		// For nodes without children or unknown types, return as-is
		return node
	}
}

// TransformFunc is a function that transforms a node
type TransformFunc func(node DistilledNode) DistilledNode

// FuncVisitor wraps a function as a Visitor
type FuncVisitor struct {
	fn TransformFunc
}

// NewFuncVisitor creates a visitor from a transform function
func NewFuncVisitor(fn TransformFunc) Visitor {
	return &FuncVisitor{fn: fn}
}

// Visit implements Visitor
func (v *FuncVisitor) Visit(node DistilledNode) DistilledNode {
	return v.fn(node)
}

// ChainVisitor chains multiple visitors together
type ChainVisitor struct {
	visitors []Visitor
}

// NewChainVisitor creates a visitor that applies multiple visitors in sequence
func NewChainVisitor(visitors ...Visitor) Visitor {
	return &ChainVisitor{visitors: visitors}
}

// Visit implements Visitor by applying all visitors in sequence
func (v *ChainVisitor) Visit(node DistilledNode) DistilledNode {
	result := node
	for _, visitor := range v.visitors {
		if result == nil {
			break
		}
		result = visitor.Visit(result)
	}
	return result
}
