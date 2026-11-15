package ir

// WalkFunc is called for each node during traversal
// Return false to stop traversal
type WalkFunc func(node DistilledNode) bool

// Walk traverses the IR tree calling fn for each node
func Walk(node DistilledNode, fn WalkFunc) {
	if node == nil || !fn(node) {
		return
	}

	children := node.GetChildren()
	for _, child := range children {
		Walk(child, fn)
	}
}
