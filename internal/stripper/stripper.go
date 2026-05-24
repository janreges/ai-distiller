package stripper

import (
	"path/filepath"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
)

// Options configures what to strip from the IR
type Options struct {
	RemovePrivate         bool  // Removes both private and protected (legacy)
	RemovePrivateOnly     bool  // Removes only private members
	RemoveProtectedOnly   bool  // Removes only protected members
	RemoveInternalOnly    bool  // Removes only internal/package-private members
	RemoveImplementations bool
	RemoveComments        bool
	RemoveImports         bool
	RemoveDocstrings      bool  // Remove documentation comments specifically
	RemoveAnnotations     bool  // Remove decorators/annotations
	RemoveFields          bool  // Remove class fields/properties
	RemoveMethods         bool  // Remove methods/functions

	// ExpandSymbols holds glob patterns; functions/methods whose name matches
	// keep their implementation even when RemoveImplementations is set.
	ExpandSymbols []string
}

// shouldExpand reports whether a symbol name matches any --expand glob pattern.
func (o Options) shouldExpand(name string) bool {
	for _, pattern := range o.ExpandSymbols {
		if pattern == "" {
			continue
		}
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

// HasAnyOption returns true if any stripping option is enabled
func (o Options) HasAnyOption() bool {
	return o.RemovePrivate || o.RemovePrivateOnly || o.RemoveProtectedOnly || o.RemoveInternalOnly ||
		o.RemoveImplementations || o.RemoveComments || o.RemoveImports || 
		o.RemoveDocstrings || o.RemoveAnnotations || o.RemoveFields || o.RemoveMethods
}

// Stripper removes specified elements from the IR based on options
type Stripper struct {
	options Options

	// inExpandedScope is true while traversing the children of a type
	// (class/struct) whose name matched an --expand pattern, so all of its
	// methods keep their implementation. Saved/restored around each scope to
	// support nesting. Safe as instance state: a Stripper is created per file
	// and the traversal is synchronous.
	inExpandedScope bool
}

// New creates a new stripper with the given options
func New(options Options) *Stripper {
	return &Stripper{
		options: options,
	}
}

// Visit implements the Visitor interface
func (s *Stripper) Visit(node ir.DistilledNode) ir.DistilledNode {
	if node == nil {
		return nil
	}

	// Process different node types
	switch n := node.(type) {
	case *ir.DistilledFile:
		return s.visitFile(n)

	case *ir.DistilledPackage:
		return s.visitPackage(n)

	case *ir.DistilledComment:
		// Always preserve API docblocks (containing @property, @method, etc.)
		if n.Extensions != nil && n.Extensions.PHP != nil && n.Extensions.PHP.IsAPIDocblock {
			return n
		}
		
		// Check if it's a docstring/docblock
		isDocstring := n.Format == "docblock" || n.Format == "doc"
		
		// Handle docstrings vs regular comments separately
		if isDocstring {
			// For docstrings, only remove if RemoveDocstrings is true
			if s.options.RemoveDocstrings {
				return nil
			}
		} else {
			// For regular comments, only remove if RemoveComments is true
			if s.options.RemoveComments {
				return nil
			}
		}
		
		return n
		
	case *ir.DistilledImport:
		if s.options.RemoveImports {
			return nil
		}
		return n
		
	case *ir.DistilledFunction:
		return s.visitFunction(n)
		
	case *ir.DistilledClass:
		return s.visitClass(n)
		
	case *ir.DistilledInterface:
		return s.visitInterface(n)
		
	case *ir.DistilledStruct:
		return s.visitStruct(n)
		
	case *ir.DistilledEnum:
		return s.visitEnum(n)
		
	case *ir.DistilledField:
		return s.visitField(n)
		
	case *ir.DistilledTypeAlias:
		return s.visitTypeAlias(n)
		
	case *ir.DistilledRawContent:
		// Raw content is always preserved
		return n
		
	default:
		// For other nodes, visit children
		return s.visitChildren(node)
	}
}

func (s *Stripper) visitFile(n *ir.DistilledFile) *ir.DistilledFile {
	result := &ir.DistilledFile{
		BaseNode: n.BaseNode,
		Path:     n.Path,
		Language: n.Language,
		Version:  n.Version,
		Metadata: n.Metadata,
		Errors:   n.Errors,
	}
	
	// Visit children
	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}
	
	// Post-process to remove orphaned docstrings
	result.Children = s.removeOrphanedDocstrings(result.Children)

	return result
}

// visitPackage strips the children of a package/namespace node. Without this,
// languages that nest their declarations inside a DistilledPackage (e.g. C#
// namespaces) fell through to visitChildren, which returned the subtree
// unchanged — so visibility and implementation filtering never applied to
// anything inside the namespace.
func (s *Stripper) visitPackage(n *ir.DistilledPackage) ir.DistilledNode {
	result := &ir.DistilledPackage{
		BaseNode: n.BaseNode,
		Name:     n.Name,
	}

	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}

	result.Children = s.removeOrphanedDocstrings(result.Children)

	return result
}

func (s *Stripper) visitFunction(n *ir.DistilledFunction) ir.DistilledNode {
	// Check if methods should be removed entirely
	if s.options.RemoveMethods {
		return nil
	}
	
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Create copy
	result := &ir.DistilledFunction{
		BaseNode:   n.BaseNode,
		Name:       n.Name,
		Visibility: n.Visibility,
		Modifiers:  n.Modifiers,
		Decorators: n.Decorators,
		TypeParams: n.TypeParams,
		Parameters: n.Parameters,
		Returns:    n.Returns,
		Implementation: n.Implementation,
	}
	
	// Strip implementation if requested, unless this symbol is explicitly
	// expanded via --expand (by its own name, or by being inside a type whose
	// name matched).
	if s.options.RemoveImplementations && !s.options.shouldExpand(n.Name) && !s.inExpandedScope {
		result.Implementation = ""
	}
	
	// Strip decorators/annotations if requested
	if s.options.RemoveAnnotations {
		result.Decorators = nil
	}
	
	// Functions don't have children in our IR
	
	return result
}

func (s *Stripper) visitClass(n *ir.DistilledClass) ir.DistilledNode {
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Create copy
	result := &ir.DistilledClass{
		BaseNode:    n.BaseNode,
		Name:        n.Name,
		Visibility:  n.Visibility,
		Modifiers:   n.Modifiers,
		Decorators:  n.Decorators,
		TypeParams:  n.TypeParams,
		Extends:     n.Extends,
		Implements:  n.Implements,
		Mixins:      n.Mixins,
		Deprecated:  n.Deprecated,
		Description: n.Description,
		APIDocblock: n.APIDocblock,
	}

	// Visit children. If this class name matched --expand, keep all of its
	// methods' implementations.
	prevExpandedScope := s.inExpandedScope
	if s.options.shouldExpand(n.Name) {
		s.inExpandedScope = true
	}
	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}
	s.inExpandedScope = prevExpandedScope

	// Post-process to remove orphaned docstrings
	result.Children = s.removeOrphanedDocstrings(result.Children)
	
	// Strip decorators/annotations if requested
	if s.options.RemoveAnnotations {
		result.Decorators = nil
	}
	
	return result
}

func (s *Stripper) visitInterface(n *ir.DistilledInterface) ir.DistilledNode {
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Create copy
	result := &ir.DistilledInterface{
		BaseNode:   n.BaseNode,
		Name:       n.Name,
		Visibility: n.Visibility,
		Modifiers:  n.Modifiers,
		TypeParams: n.TypeParams,
		Extends:    n.Extends,
		Permits:    n.Permits,
	}
	
	// Visit children
	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}
	
	return result
}

func (s *Stripper) visitStruct(n *ir.DistilledStruct) ir.DistilledNode {
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Create copy
	result := &ir.DistilledStruct{
		BaseNode:   n.BaseNode,
		Name:       n.Name,
		Visibility: n.Visibility,
		TypeParams: n.TypeParams,
	}

	// Visit children. If this struct name matched --expand, keep all of its
	// methods' implementations.
	prevExpandedScope := s.inExpandedScope
	if s.options.shouldExpand(n.Name) {
		s.inExpandedScope = true
	}
	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}
	s.inExpandedScope = prevExpandedScope

	return result
}

func (s *Stripper) visitEnum(n *ir.DistilledEnum) ir.DistilledNode {
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Create copy
	result := &ir.DistilledEnum{
		BaseNode:   n.BaseNode,
		Name:       n.Name,
		Visibility: n.Visibility,
	}
	
	// Visit children
	for _, child := range n.Children {
		if visited := child.Accept(s); visited != nil {
			result.Children = append(result.Children, visited)
		}
	}
	
	return result
}

func (s *Stripper) visitField(n *ir.DistilledField) ir.DistilledNode {
	// Check if fields should be removed entirely
	if s.options.RemoveFields {
		return nil
	}
	
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Determine if we should strip the default value
	defaultValue := n.DefaultValue
	
	// For enum cases, always show values regardless of implementation flag
	// Enum values are part of the API definition, not implementation
	
	// Determine decorators
	decorators := n.Decorators
	if s.options.RemoveAnnotations {
		decorators = nil
	}
	
	// Return copy
	return &ir.DistilledField{
		BaseNode:     n.BaseNode,
		Name:         n.Name,
		Visibility:   n.Visibility,
		Modifiers:    n.Modifiers,
		Type:         n.Type,
		DefaultValue: defaultValue,
		Decorators:   decorators,
		// Property-specific fields
		IsProperty:       n.IsProperty,
		HasGetter:        n.HasGetter,
		HasSetter:        n.HasSetter,
		GetterVisibility: n.GetterVisibility,
		SetterVisibility: n.SetterVisibility,
		Description:      n.Description,
		Deprecated:       n.Deprecated,
	}
}

func (s *Stripper) visitTypeAlias(n *ir.DistilledTypeAlias) ir.DistilledNode {
	// Check if should remove by visibility
	if s.shouldRemoveByVisibility(n.Name, n.Visibility) {
		return nil
	}
	
	// Return copy
	return &ir.DistilledTypeAlias{
		BaseNode:   n.BaseNode,
		Name:       n.Name,
		Visibility: n.Visibility,
		TypeParams: n.TypeParams,
		Type:       n.Type,
	}
}

func (s *Stripper) visitChildren(node ir.DistilledNode) ir.DistilledNode {
	// For unknown node types, just return as-is
	// In a real implementation, we'd need to handle all node types
	return node
}

// shouldRemoveByVisibility checks if a node should be removed based on visibility settings
func (s *Stripper) shouldRemoveByVisibility(name string, visibility ir.Visibility) bool {
	// Check specific visibility removal options first
	if s.options.RemovePrivateOnly || s.options.RemoveProtectedOnly || s.options.RemoveInternalOnly {
		switch visibility {
		case ir.VisibilityPrivate, ir.VisibilityFilePrivate:
			// Remove if private-only flag is set
			return s.options.RemovePrivateOnly
		case ir.VisibilityInternal, ir.VisibilityPackage:
			// Remove if internal-only flag is set
			return s.options.RemoveInternalOnly
		case ir.VisibilityProtected, ir.VisibilityProtectedInternal:
			// Remove if protected-only flag is set
			return s.options.RemoveProtectedOnly
		case ir.VisibilityPrivateProtected:
			// C# private protected - remove if either private or protected is being removed
			return s.options.RemovePrivateOnly || s.options.RemoveProtectedOnly
		case ir.VisibilityPublic, ir.VisibilityOpen:
			return false
		default:
			// For implicit visibility, check language conventions
			if strings.HasPrefix(name, "_") && s.options.RemovePrivateOnly {
				return true
			}
			// Go convention: lowercase = package-private (internal)
			if len(name) > 0 && name[0] >= 'a' && name[0] <= 'z' && s.options.RemoveInternalOnly {
				// This should be handled by the language processor setting proper visibility
				// but we keep it as a fallback
				return false // Let the language processor handle this
			}
			return false
		}
	}
	
	// Legacy behavior: RemovePrivate removes both private and protected
	if s.options.RemovePrivate {
		switch visibility {
		case ir.VisibilityPrivate, ir.VisibilityProtected, ir.VisibilityInternal, 
			 ir.VisibilityFilePrivate, ir.VisibilityPackage, ir.VisibilityProtectedInternal,
			 ir.VisibilityPrivateProtected:
			return true
		case ir.VisibilityPublic, ir.VisibilityOpen:
			return false
		}
		
		// If no explicit visibility, use language conventions
		if strings.HasPrefix(name, "_") {
			return true
		}
	}
	
	// Default to not removing
	return false
}

// removeOrphanedDocstrings removes docstring comments that appear to be orphaned
// (i.e., not followed by any declaration they could be documenting)
func (s *Stripper) removeOrphanedDocstrings(children []ir.DistilledNode) []ir.DistilledNode {
	if len(children) == 0 {
		return children
	}
	
	result := make([]ir.DistilledNode, 0, len(children))
	
	for i, child := range children {
		// Check if this is a docstring comment
		if comment, ok := child.(*ir.DistilledComment); ok && comment.Format == "doc" {
			// Check if this docstring is followed by a declaration
			hasFollowingDeclaration := false
			for j := i + 1; j < len(children); j++ {
				next := children[j]
				switch next.(type) {
				case *ir.DistilledFunction, *ir.DistilledField, *ir.DistilledClass, 
					 *ir.DistilledInterface, *ir.DistilledStruct, *ir.DistilledEnum:
					hasFollowingDeclaration = true
					break
				case *ir.DistilledComment:
					// Skip other comments
					continue
				default:
					// Stop at first non-comment
					break
				}
			}
			
			// If it's the last element and it's a docstring, it's likely orphaned
			// OR if there's no following declaration, it's orphaned
			if !hasFollowingDeclaration {
				// Skip this orphaned docstring
				continue
			}
		}
		
		result = append(result, child)
	}
	
	return result
}