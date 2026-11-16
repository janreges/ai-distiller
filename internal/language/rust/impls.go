package rust

import (
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/stripper"
	sitter "github.com/tree-sitter/go-tree-sitter"
)

// implTarget is what an impl block implements: the paths of its self type and
// of its trait as written, without generic arguments, lifetimes and & ([a,
// Foo] for &'a a::Foo<T>). A path is nil when the block has no trait, or when
// its self type has no name of its own (a tuple, a slice, dyn Trait), is a
// generic parameter of the block (impl<T> Trait for T) or is not a plain path
// (<T as Tr>::Out, ::a::Foo, a macro variable).
type implTarget struct {
	self, trait []string
}

// implTargetOf returns the target of impl_item node.
func (w *walker) implTargetOf(node *sitter.Node) implTarget {
	var target implTarget
	if trait := node.ChildByFieldName("trait"); trait != nil {
		target.trait = w.typePath(trait, false)
	}
	if typ := node.ChildByFieldName("type"); typ != nil {
		target.self = w.typePath(typ, true)
	}
	if params := node.ChildByFieldName("type_parameters"); params != nil && len(target.self) == 1 {
		for _, param := range w.children(params) {
			if name := param.ChildByFieldName("name"); name != nil && w.text(name) == target.self[0] {
				target.self = nil
			}
		}
	}
	return target
}

// typePath returns the path of a type node: [Foo] for Foo and Foo<'a, T>,
// [a, b, Foo] for a::b::Foo, and with references true the same for &'a Foo and
// &mut Foo, whose visibility is that of Foo. Any other type has none (nil).
func (w *walker) typePath(node *sitter.Node, references bool) []string {
	switch node.Kind() {
	case "type_identifier":
		return []string{w.text(node)}
	case "generic_type":
		if base := node.ChildByFieldName("type"); base != nil {
			return w.typePath(base, false)
		}
	case "scoped_type_identifier":
		name, path := node.ChildByFieldName("name"), node.ChildByFieldName("path")
		if name == nil || path == nil {
			return nil
		}
		if prefix := w.pathSegments(path); prefix != nil {
			return append(prefix, w.text(name))
		}
	case "identifier", "scoped_identifier":
		return w.pathSegments(node)
	case "reference_type":
		if inner := node.ChildByFieldName("type"); inner != nil && references {
			return w.typePath(inner, true)
		}
	}
	return nil
}

// pathSegments returns the segments of a plain path node, made of self,
// super, crate and names (a::b gives [a, b]), or nil for any other path:
// generic arguments, <T as Trait>, a macro variable, a path that starts with
// ::.
func (w *walker) pathSegments(node *sitter.Node) []string {
	switch node.Kind() {
	case "identifier", "self", "super", "crate":
		return []string{w.text(node)}
	case "scoped_identifier":
		name, path := node.ChildByFieldName("name"), node.ChildByFieldName("path")
		if name == nil || path == nil {
			return nil
		}
		if prefix := w.pathSegments(path); prefix != nil {
			return append(prefix, w.text(name))
		}
	}
	return nil
}

// useItem is a use declaration as hideImplsOfRemovedTypes reads it: its
// visibility and the names it binds.
type useItem struct {
	visibility ir.Visibility
	bindings   []useBinding
}

// useBinding is one name that a use declaration binds: the path it names and
// the name it is bound to, the last segment or the alias after "as". A glob
// (a::*) binds what the module at path exports and has no name.
type useBinding struct {
	path []string
	name string
	glob bool
}

// useBindings appends to into the bindings of node, a use tree under prefix
// (the path of the use lists around it).
func (w *walker) useBindings(node *sitter.Node, prefix []string, into []useBinding) []useBinding {
	join := func(path []string) []string {
		return append(append([]string{}, prefix...), path...)
	}
	switch node.Kind() {
	case "identifier", "self", "super", "crate", "scoped_identifier":
		path := w.pathSegments(node)
		if path == nil {
			return into
		}
		full := join(path)
		if len(full) > 1 && full[len(full)-1] == "self" {
			// a::{self} binds the module a.
			full = full[:len(full)-1]
		}
		return append(into, useBinding{path: full, name: full[len(full)-1]})
	case "use_as_clause":
		path, alias := node.ChildByFieldName("path"), node.ChildByFieldName("alias")
		if path == nil || alias == nil {
			return into
		}
		if segments := w.pathSegments(path); segments != nil {
			return append(into, useBinding{path: join(segments), name: w.text(alias)})
		}
	case "use_wildcard":
		kids := w.children(node)
		for i := range kids {
			if child := &kids[i]; child.IsNamed() {
				if segments := w.pathSegments(child); segments != nil {
					return append(into, useBinding{path: join(segments), glob: true})
				}
				return into
			}
		}
		// A bare * in a list: a::{*}.
		if len(prefix) > 0 {
			return append(into, useBinding{path: join(nil), glob: true})
		}
	case "scoped_use_list":
		inner := prefix
		if path := node.ChildByFieldName("path"); path != nil {
			segments := w.pathSegments(path)
			if segments == nil {
				return into
			}
			inner = join(segments)
		}
		if list := node.ChildByFieldName("list"); list != nil {
			return w.useBindings(list, inner, into)
		}
	case "use_list":
		kids := w.children(node)
		for i := range kids {
			if child := &kids[i]; child.IsNamed() {
				into = w.useBindings(child, prefix, into)
			}
		}
	}
	return into
}

// scope is the file or an inline module, as hideImplsOfRemovedTypes sees it.
type scope struct {
	parent *scope
	// removedBy is the visibility for which the stripper removes the module
	// with its items, its own or that of a module around it; "" when it stays
	// (always for the file). ownRemoved is set when its own visibility does.
	removedBy  ir.Visibility
	ownRemoved bool
	modules    map[string]*scope
	// types are the structs, enums, unions and traits declared here, by name.
	types map[string][]*typeDecl
	// imported are the names that the use declarations here bind.
	imported map[string]bool
	uses     []scopeUse
	impls    []*ir.DistilledClass
}

// typeDecl is one declaration of a struct, enum, union or trait.
type typeDecl struct {
	// removedBy and ownRemoved are as for a scope.
	removedBy  ir.Visibility
	ownRemoved bool
	// reexported is set when a pub use that stays makes it reachable.
	reexported bool
}

// kept reports whether the declaration shows: the stripper keeps it, or it
// is removed only with a module around it while a re-export that stays
// reaches it, as rustdoc then documents it.
func (d *typeDecl) kept() bool {
	return d.removedBy == "" || d.reexported && !d.ownRemoved
}

// scopeUse is a use declaration of a scope. exports is set when it re-exports
// what it binds (pub, pub(crate), ... that the stripper keeps), kept when it
// also stays with every module around it.
type scopeUse struct {
	bindings      []useBinding
	exports, kept bool
}

func newScope(parent *scope, removedBy ir.Visibility, ownRemoved bool) *scope {
	return &scope{
		parent: parent, removedBy: removedBy, ownRemoved: ownRemoved,
		modules: map[string]*scope{}, types: map[string][]*typeDecl{}, imported: map[string]bool{},
	}
}

// module returns the module that path names from sc: self is sc, super the
// module around it, crate is root (nil when crate paths are not followed) and
// a name a module declared in the current one. nil when path names no inline
// module of the file.
func (sc *scope) module(path []string, root *scope) *scope {
	current := sc
	for i, segment := range path {
		switch {
		case segment == "self" && i == 0:
		case segment == "super" && (i == 0 || path[i-1] == "super"):
			current = current.parent
		case segment == "crate" && i == 0:
			current = root
		default:
			current = current.modules[segment]
		}
		if current == nil {
			return nil
		}
	}
	return current
}

// declarations returns the declarations of the type or trait that path names
// in an impl block of sc: [Foo] one declared in sc itself, a longer path one
// declared in the inline module it names (crate:: is not followed, as the
// file may not be the crate root). None when the name comes in through a use
// declaration or is not declared there: its visibility is not known here.
func (sc *scope) declarations(path []string) []*typeDecl {
	if len(path) == 0 {
		return nil
	}
	target := sc.module(path[:len(path)-1], nil)
	if target == nil {
		return nil
	}
	name := path[len(path)-1]
	if target.imported[name] {
		return nil
	}
	return target.types[name]
}

// removedBy returns the visibility that removes every declaration in decls,
// or "" when one of them shows or there is none.
func removedBy(decls []*typeDecl) ir.Visibility {
	for _, decl := range decls {
		if decl.kept() {
			return ""
		}
	}
	if len(decls) == 0 {
		return ""
	}
	return decls[0].removedBy
}

// hideImplsOfRemovedTypes prepares file for s: every impl block whose self
// type (else trait) is a struct, enum, union or trait declared in the file,
// that s removes for its visibility or that of a module around it, takes that
// visibility, so that s removes the block with the type, as rustdoc does. The
// type is looked up as Rust resolves the block's path: a bare name in the
// module of the block, a path through the inline modules it names. A name
// that a use declaration binds there, or that is not declared there, leaves
// the block alone: its visibility is not known here. A type that a pub use
// that stays re-exports shows, also from a private module. A name declared
// more than once in one module removes the block only when every declaration
// goes. impls are the impl blocks of file with their targets, uses its use
// declarations with what they bind.
func hideImplsOfRemovedTypes(file *ir.DistilledFile, impls map[*ir.DistilledClass]implTarget, uses map[*ir.DistilledImport]useItem, s *stripper.Stripper) {
	if len(impls) == 0 {
		return
	}
	root := newScope(nil, "", false)
	scopes := []*scope{root}
	var add func(sc *scope, nodes []ir.DistilledNode)
	add = func(sc *scope, nodes []ir.DistilledNode) {
		for _, node := range nodes {
			switch n := node.(type) {
			case *ir.DistilledImport:
				item, ok := uses[n]
				if !ok {
					continue
				}
				exports := item.visibility != ir.VisibilityPrivate && !s.RemovesVisibility(item.visibility)
				sc.uses = append(sc.uses, scopeUse{bindings: item.bindings, exports: exports, kept: exports && sc.removedBy == ""})
				for _, binding := range item.bindings {
					if !binding.glob {
						sc.imported[binding.name] = true
					}
				}
			case *ir.DistilledClass:
				if _, isImpl := impls[n]; isImpl {
					sc.impls = append(sc.impls, n)
					continue
				}
				own := s.RemovesVisibility(n.Visibility)
				by := sc.removedBy
				if by == "" && own {
					by = n.Visibility
				}
				switch {
				case declaresType(n):
					name := n.Name
					if i := strings.IndexByte(name, '<'); i >= 0 {
						name = name[:i]
					}
					sc.types[name] = append(sc.types[name], &typeDecl{removedBy: by, ownRemoved: own})
				case len(n.Modifiers) == 0:
					// A module: its items are removed with it.
					module := newScope(sc, by, own)
					if _, twice := sc.modules[n.Name]; !twice {
						sc.modules[n.Name] = module
					}
					scopes = append(scopes, module)
					add(module, n.Children)
				}
			}
		}
	}
	add(root, file.Children)

	e := exporter{root: root, seen: map[*scope]map[string]bool{}}
	for _, sc := range scopes {
		for _, use := range sc.uses {
			if use.kept {
				e.bindings(sc, use.bindings)
			}
		}
	}

	for _, sc := range scopes {
		for _, class := range sc.impls {
			target := impls[class]
			by := removedBy(sc.declarations(target.self))
			if by == "" {
				by = removedBy(sc.declarations(target.trait))
			}
			if by != "" {
				class.Visibility = by
			}
		}
	}
}

// exporter marks the declarations that pub use declarations re-export.
type exporter struct {
	root *scope
	// seen are the names (and "*" for a whole module) already marked per
	// module; use declarations can form cycles (use super::*).
	seen map[*scope]map[string]bool
}

func (e *exporter) visit(sc *scope, name string) bool {
	if e.seen[sc] == nil {
		e.seen[sc] = map[string]bool{}
	}
	if e.seen[sc][name] {
		return true
	}
	e.seen[sc][name] = true
	return false
}

// bindings marks what the bindings of a use declaration of sc re-export.
func (e *exporter) bindings(sc *scope, bindings []useBinding) {
	for _, binding := range bindings {
		if binding.glob {
			if module := sc.module(binding.path, e.root); module != nil {
				e.module(module)
			}
		} else if len(binding.path) > 0 {
			if module := sc.module(binding.path[:len(binding.path)-1], e.root); module != nil {
				e.name(module, binding.path[len(binding.path)-1])
			}
		}
	}
}

// name marks what name names in sc: its declarations, the module of that
// name, and what the re-exports of sc bound to the name reach.
func (e *exporter) name(sc *scope, name string) {
	if e.visit(sc, name) {
		return
	}
	for _, decl := range sc.types[name] {
		decl.reexported = true
	}
	if module := sc.modules[name]; module != nil {
		e.module(module)
	}
	for _, use := range sc.uses {
		if !use.exports {
			continue
		}
		for _, binding := range use.bindings {
			switch {
			case binding.glob:
				if module := sc.module(binding.path, e.root); module != nil {
					e.name(module, name)
				}
			case binding.name == name:
				e.bindings(sc, []useBinding{binding})
			}
		}
	}
}

// module marks what a re-export of the module sc reaches: its types, the
// modules in it that the stripper keeps for their own visibility, and what its
// own re-exports reach. A type removed for its own visibility stays removed
// (typeDecl.kept).
func (e *exporter) module(sc *scope) {
	if e.visit(sc, "*") {
		return
	}
	for _, decls := range sc.types {
		for _, decl := range decls {
			decl.reexported = true
		}
	}
	for _, module := range sc.modules {
		if !module.ownRemoved {
			e.module(module)
		}
	}
	for _, use := range sc.uses {
		if use.exports {
			e.bindings(sc, use.bindings)
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
