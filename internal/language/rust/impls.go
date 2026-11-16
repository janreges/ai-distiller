package rust

import (
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/stripper"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// implTarget is what an impl block implements: the bare names (no path, no
// generic arguments, no lifetimes) of its self type and of its trait. A name
// is empty when the block has no trait, or when its self type has no name of
// its own (a tuple, a slice, dyn Trait) or is a generic parameter of the
// block (impl<T> Trait for T).
type implTarget struct {
	self, trait string
}

// implTargetOf returns the target of impl_item node.
func (w *walker) implTargetOf(node *sitter.Node) implTarget {
	var target implTarget
	if trait := node.ChildByFieldName("trait"); trait != nil {
		target.trait = w.typeName(trait, false)
	}
	if typ := node.ChildByFieldName("type"); typ != nil {
		target.self = w.typeName(typ, true)
	}
	if params := node.ChildByFieldName("type_parameters"); params != nil && target.self != "" {
		for _, param := range w.children(params) {
			if name := param.ChildByFieldName("name"); name != nil && w.text(name) == target.self {
				target.self = ""
			}
		}
	}
	return target
}

// typeName returns the bare name of a type node: Foo for Foo, Foo<'a, T> and
// a::b::Foo, and with references true also for &'a Foo and &mut Foo, whose
// visibility is that of Foo. Any other type has none ("").
func (w *walker) typeName(node *sitter.Node, references bool) string {
	switch node.Kind() {
	case "type_identifier":
		return w.text(node)
	case "generic_type":
		if base := node.ChildByFieldName("type"); base != nil {
			return w.typeName(base, false)
		}
	case "scoped_type_identifier":
		if name := node.ChildByFieldName("name"); name != nil {
			return w.text(name)
		}
	case "reference_type":
		if inner := node.ChildByFieldName("type"); inner != nil && references {
			return w.typeName(inner, true)
		}
	}
	return ""
}

// hideImplsOfRemovedTypes prepares file for s: every impl block whose self
// type or trait is declared in file (a struct, enum, union or trait, matched
// by its bare name) and removed by s for its visibility, its own or that of a
// module around it, takes that visibility, so that s removes the block with
// the type, as rustdoc does. A name declared more than once removes the
// block only when s removes every declaration of it. A self type or trait not
// declared in file leaves the block alone: its visibility is unknown. impls
// are the impl blocks of file with their targets.
func hideImplsOfRemovedTypes(file *ir.DistilledFile, impls map[*ir.DistilledClass]implTarget, s *stripper.Stripper) {
	if len(impls) == 0 {
		return
	}
	// removedBy maps a declared name to the visibility that removes it, or to
	// "" when a declaration of it stays.
	removedBy := map[string]ir.Visibility{}
	var walk func(nodes []ir.DistilledNode, outer ir.Visibility)
	walk = func(nodes []ir.DistilledNode, outer ir.Visibility) {
		for _, node := range nodes {
			class, ok := node.(*ir.DistilledClass)
			if !ok {
				continue
			}
			if _, isImpl := impls[class]; isImpl {
				continue
			}
			by := outer
			if by == "" && s.RemovesVisibility(class.Visibility) {
				by = class.Visibility
			}
			switch {
			case declaresType(class):
				name := class.Name
				if i := strings.IndexByte(name, '<'); i >= 0 {
					name = name[:i]
				}
				if prev, seen := removedBy[name]; !seen || prev != "" && by == "" {
					removedBy[name] = by
				}
			case len(class.Modifiers) == 0:
				// A module: its items are removed with it.
				walk(class.Children, by)
			}
		}
	}
	walk(file.Children, "")
	for class, target := range impls {
		if by := removedBy[target.self]; target.self != "" && by != "" {
			class.Visibility = by
		} else if by := removedBy[target.trait]; target.trait != "" && by != "" {
			class.Visibility = by
		}
	}
}

// declaresType reports whether class is a struct, union, enum or trait.
func declaresType(class *ir.DistilledClass) bool {
	for _, mod := range class.Modifiers {
		switch mod {
		case ir.ModifierStruct, ir.ModifierUnion, ir.ModifierEnum, ir.ModifierAbstract:
			return true
		}
	}
	return false
}
