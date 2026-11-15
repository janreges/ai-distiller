package ir

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Mock node for testing
type mockNode struct {
	BaseNode
	Name     string
	Children []DistilledNode
	Kind     NodeKind
}

func (n *mockNode) GetNodeKind() NodeKind {
	return n.Kind
}

func (n *mockNode) GetChildren() []DistilledNode {
	return n.Children
}

func (n *mockNode) Accept(visitor Visitor) DistilledNode {
	return visitor.Visit(n)
}

func TestBaseVisitor(t *testing.T) {
	visitor := &BaseVisitor{}
	node := &mockNode{Name: "test", Kind: "mock"}

	result := visitor.Visit(node)
	assert.Equal(t, node, result)
}

func TestWalker(t *testing.T) {
	t.Run("WalkSimpleNode", func(t *testing.T) {
		visitor := &BaseVisitor{}
		walker := NewWalker(visitor)

		node := &mockNode{Name: "root", Kind: "mock"}
		result := walker.Walk(node)

		assert.NotNil(t, result)
		assert.Equal(t, node, result)
	})

	t.Run("WalkWithChildren", func(t *testing.T) {
		// Create a tree structure
		child1 := &mockNode{Name: "child1", Kind: "mock"}
		child2 := &mockNode{Name: "child2", Kind: "mock"}
		root := &mockNode{
			Name:     "root",
			Kind:     "mock",
			Children: []DistilledNode{child1, child2},
		}

		visited := []string{}
		visitor := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			if m, ok := node.(*mockNode); ok {
				visited = append(visited, m.Name)
			}
			return node
		})

		walker := NewWalker(visitor)
		result := walker.Walk(root)

		assert.NotNil(t, result)
		assert.Equal(t, []string{"root", "child1", "child2"}, visited)
	})

	t.Run("WalkWithNodeRemoval", func(t *testing.T) {
		child1 := &mockNode{Name: "keep", Kind: "mock"}
		child2 := &mockNode{Name: "remove", Kind: "mock"}
		root := &DistilledFile{
			BaseNode: BaseNode{},
			Path:     "test.go",
			Children: []DistilledNode{child1, child2},
		}

		// Visitor that removes nodes with name "remove"
		visitor := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			if m, ok := node.(*mockNode); ok && m.Name == "remove" {
				return nil
			}
			return node
		})

		walker := NewWalker(visitor)
		result := walker.Walk(root)

		assert.NotNil(t, result)
		file, ok := result.(*DistilledFile)
		assert.True(t, ok)
		assert.Len(t, file.Children, 1)
		assert.Equal(t, "keep", file.Children[0].(*mockNode).Name)
	})

	t.Run("WalkNilNode", func(t *testing.T) {
		walker := NewWalker(&BaseVisitor{})
		result := walker.Walk(nil)
		assert.Nil(t, result)
	})
}

func TestFuncVisitor(t *testing.T) {
	called := false
	visitor := NewFuncVisitor(func(node DistilledNode) DistilledNode {
		called = true
		return node
	})

	node := &mockNode{Name: "test", Kind: "mock"}
	result := visitor.Visit(node)

	assert.True(t, called)
	assert.Equal(t, node, result)
}

func TestChainVisitor(t *testing.T) {
	t.Run("ChainMultipleVisitors", func(t *testing.T) {
		transformations := []string{}

		visitor1 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			transformations = append(transformations, "visitor1")
			return node
		})

		visitor2 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			transformations = append(transformations, "visitor2")
			return node
		})

		visitor3 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			transformations = append(transformations, "visitor3")
			return node
		})

		chain := NewChainVisitor(visitor1, visitor2, visitor3)
		node := &mockNode{Name: "test", Kind: "mock"}
		result := chain.Visit(node)

		assert.NotNil(t, result)
		assert.Equal(t, []string{"visitor1", "visitor2", "visitor3"}, transformations)
	})

	t.Run("ChainStopsOnNil", func(t *testing.T) {
		calls := []string{}

		visitor1 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			calls = append(calls, "visitor1")
			return node
		})

		visitor2 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			calls = append(calls, "visitor2")
			return nil // This stops the chain
		})

		visitor3 := NewFuncVisitor(func(node DistilledNode) DistilledNode {
			calls = append(calls, "visitor3")
			return node
		})

		chain := NewChainVisitor(visitor1, visitor2, visitor3)
		node := &mockNode{Name: "test", Kind: "mock"}
		result := chain.Visit(node)

		assert.Nil(t, result)
		assert.Equal(t, []string{"visitor1", "visitor2"}, calls)
		assert.NotContains(t, calls, "visitor3")
	})
}

func TestUpdateChildren(t *testing.T) {
	t.Run("UpdateDistilledFileChildren", func(t *testing.T) {
		originalChildren := []DistilledNode{
			&mockNode{Name: "child1", Kind: "mock"},
			&mockNode{Name: "child2", Kind: "mock"},
		}

		file := &DistilledFile{
			Path:     "test.go",
			Children: originalChildren,
		}

		newChildren := []DistilledNode{
			&mockNode{Name: "newChild", Kind: "mock"},
		}

		walker := &Walker{visitor: &BaseVisitor{}}
		result := walker.updateChildren(file, newChildren)

		updatedFile, ok := result.(*DistilledFile)
		assert.True(t, ok)
		assert.Len(t, updatedFile.Children, 1)
		assert.Equal(t, "newChild", updatedFile.Children[0].(*mockNode).Name)

		// Ensure original is unchanged (immutability)
		assert.Len(t, file.Children, 2)
	})

	t.Run("UpdateUnknownNodeType", func(t *testing.T) {
		node := &mockNode{Name: "test", Kind: "mock"}
		walker := &Walker{visitor: &BaseVisitor{}}

		result := walker.updateChildren(node, []DistilledNode{})
		assert.Equal(t, node, result) // Should return unchanged
	})
}
