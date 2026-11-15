package rust

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/tsparse"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_rust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
)

// TreeSitterProcessor distils Rust source with the tree-sitter-rust grammar.
type TreeSitterProcessor struct{}

// NewTreeSitterProcessor creates a new tree-sitter based processor
func NewTreeSitterProcessor() *TreeSitterProcessor {
	return &TreeSitterProcessor{}
}

// ProcessSource parses source and returns its IR. Items inside function
// bodies, macro_rules! bodies and the values of const and static items are
// not distilled; the items inside an item-level macro call with braces
// (s! { ... }, cfg_if! { ... }) are (walker.expand).
func (p *TreeSitterProcessor) ProcessSource(ctx context.Context, source []byte, filename string) (*ir.DistilledFile, error) {
	// The parser lives as long as this call: the tree and every node taken
	// from it are used only here.
	parser, err := tsparse.NewParser(rustLanguage())
	if err != nil {
		return nil, fmt.Errorf("failed to create Rust parser: %w", err)
	}
	defer parser.Close()

	tree, err := tsparse.Parse(ctx, parser, source)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Rust code: %w", err)
	}
	defer tree.Close()

	root := tree.RootNode()
	file := &ir.DistilledFile{
		BaseNode: ir.BaseNode{
			Location: ir.Location{
				StartLine: 1,
				EndLine:   int(root.EndPosition().Row) + 1,
			},
		},
		Path:     filename,
		Language: "rust",
		Version:  "2021",
		Children: []ir.DistilledNode{},
		Errors:   []ir.DistilledError{},
	}

	cursor := root.Walk()
	defer cursor.Close()
	var walkErr error
	w := &walker{ctx: ctx, source: source, file: file, cursor: cursor, err: &walkErr}
	w.items(root, nil, inFile)
	if walkErr != nil {
		return nil, walkErr
	}
	return file, nil
}

// rustLanguage returns the tree-sitter-rust grammar.
func rustLanguage() *sitter.Language {
	return sitter.NewLanguage(tree_sitter_rust.Language())
}

// maxMacroDepth is the deepest nesting of macro bodies whose items are
// distilled; a body counts one level, and so does each {} block expanded
// inside one (expandBlocksIn).
const maxMacroDepth = 8

// blockOwners are the keywords whose item owns the first {} block after them
// in a macro body: a function body, or the members of an impl, struct, enum,
// union or trait, or the items of a mod (expand).
var blockOwners = map[string]bool{
	"fn": true, "impl": true, "struct": true, "enum": true, "union": true, "trait": true, "mod": true,
}

// container is the kind of item list being walked; it decides which items a
// list holds and the default visibility of its members.
type container int

const (
	inFile      container = iota // the source file
	inMod                        // the body of an inline mod
	inImpl                       // the body of an inherent impl
	inTraitImpl                  // the body of impl Trait for Type
	inTrait                      // the body of a trait
)

// walker builds the IR of one parsed file, or of one macro body parsed on its
// own (expand). Its cursor belongs to the tree being walked and is only used
// to list children.
type walker struct {
	ctx    context.Context
	source []byte
	file   *ir.DistilledFile
	cursor *sitter.TreeCursor
	// rowOffset and colOffset place source in the file: rowOffset is added
	// to every row and colOffset to the columns of the first row. Both are 0
	// for the file itself.
	rowOffset, colOffset uint
	// depth is the number of macro bodies around source (0 for the file).
	depth int
	// macro is set when source is a macro body parsed on its own.
	macro *macroBody
	// err is the first failure to parse a macro body, shared by the walkers
	// of one file.
	err *error
}

// macroBody links the walker of a macro body to the token tree it was cut
// from.
type macroBody struct {
	// outer is the walker of the tree that holds the token tree.
	outer *walker
	// blocks are the {} token trees at the top level of the token tree, as
	// nodes of the outer tree.
	blocks []sitter.Node
	// start is the byte offset of the body in outer.source.
	start uint
}

// children returns the children of node. One cursor pass is linear, while
// Child(i) walks from the first child on each call.
func (w *walker) children(node *sitter.Node) []sitter.Node {
	return node.Children(w.cursor)
}

// items adds the items of node (the source file, a declaration_list or an
// ERROR node among items) to parent, or to the file when parent is nil.
// Comments are added where they are, except a comment that starts on the
// line where the previous sibling that is not a comment ends (an item, an
// attribute, a brace): it remarks on that line and yields no IR. Outer
// attributes are skipped and leave the docs above them to the item after
// them. The doc comments right before a statement that yields no IR (a macro
// call, macro_rules!, an extern block) are dropped, so they do not document
// the next item (appendStatement). The items inside a macro call with braces
// follow its place (expand).
//
// In a macro body, an ERROR node yields no IR; at the top level of the body,
// it and an expression statement with an error stand for the {} blocks of the
// body they hold (expand). An item of a macro body whose name the parser read
// across an error yields no IR either (misnamed).
func (w *walker) items(node *sitter.Node, parent *ir.DistilledClass, in container) {
	list := &w.file.Children
	if parent != nil {
		list = &parent.Children
	}
	top := w.macro != nil && node.Kind() == "source_file"
	// prev is the row where the previous sibling that is not a comment
	// ends. A macro body starts on the row of its opening brace.
	prev := -1
	if top {
		prev = 0
	}
	kids := w.children(node)
	for i := range kids {
		child := &kids[i]
		if isComment(child) {
			if int(child.StartPosition().Row) != prev {
				*list = append(*list, w.comment(child))
			}
			continue
		}
		prev = lastRow(child)
		switch {
		case !child.IsNamed() || child.Kind() == "attribute_item":
		case top && (child.Kind() == "ERROR" || child.Kind() == "expression_statement" && child.HasError()):
			*list = appendStatement(*list, nil)
			w.expandBlocksIn(child, parent, in)
		case w.macro != nil && (child.Kind() == "ERROR" || w.misnamed(child)):
			*list = appendStatement(*list, nil)
		case child.Kind() == "ERROR":
			// Recovery: what the parser could not place is read as items of
			// this list, so the items around a broken region survive.
			w.items(child, parent, in)
		case child.Kind() == "macro_invocation":
			*list = appendStatement(*list, nil)
			if body := w.braceBody(child); body != nil {
				w.expand(body, parent, in, w.depth+1)
			}
		default:
			*list = appendStatement(*list, w.item(child, in))
		}
	}
}

// braceBody returns the argument of a macro call when it is written in
// braces (name! { ... }), or nil.
func (w *walker) braceBody(call *sitter.Node) *sitter.Node {
	for _, child := range w.children(call) {
		if child.Kind() == "token_tree" {
			if w.isBraceTree(&child) {
				return &child
			}
			return nil
		}
	}
	return nil
}

// isBraceTree reports whether node is a token tree in braces ({ ... }) rather
// than in parentheses or brackets.
func (w *walker) isBraceTree(node *sitter.Node) bool {
	start := node.StartByte()
	return node.Kind() == "token_tree" && start < uint(len(w.source)) && w.source[start] == '{'
}

// expand adds the items inside body, the {} token tree of an item-level macro
// call or a {} block inside one, to parent as items of a list of kind in,
// with their true positions in the file. Macros take any tokens, so this is a
// best effort that never fails on what it cannot read. The text between the
// braces is parsed on its own as Rust source, and the items the parser
// recognises at its top level are walked like any others, also when the parse
// has errors elsewhere (pub safe fn, libc's pub {const} fn). Each top-level
// ERROR node, and each top-level expression statement with an error (the
// if #[cfg(unix)] { ... } else { ... } of cfg_if!), stands for the {} blocks
// of body that lie entirely inside it: they are expanded the same way, in
// place (expandBlocksIn). A {} block outside them, such as the body of a
// recognised function, is never read as items, nor is the first {} block
// after fn, impl, struct, enum, union, trait or mod inside them (the body or
// the members of an item the parser could not read); other ERROR nodes yield
// no IR.
//
// A body that does not start like items (startsLikeItems), and a body whose
// parse needs more than its budget (parseBudget), yield no IR. depth is the
// nesting level of body; deeper than maxMacroDepth, nothing is added. A
// failure to parse is recorded in w.err and ends the expansion.
func (w *walker) expand(body *sitter.Node, parent *ir.DistilledClass, in container, depth int) {
	if depth > maxMacroDepth || *w.err != nil {
		return
	}
	kids := w.children(body)
	if len(kids) < 2 || kids[0].Kind() != "{" || kids[len(kids)-1].Kind() != "}" {
		return
	}
	if !w.startsLikeItems(kids[1 : len(kids)-1]) {
		return
	}
	open := kids[0].EndPosition()
	macro := &macroBody{outer: w, start: kids[0].EndByte()}
	owned := false
	for i := 1; i < len(kids)-1; i++ {
		switch kid := &kids[i]; {
		case blockOwners[kid.Kind()]:
			owned = true
		case kid.Kind() == ";":
			owned = false
		case w.isBraceTree(kid):
			// The first block after fn is the body of a function, and after
			// impl, struct, enum, union, trait or mod the members of that
			// item. It is never read as items of this list, also when the
			// parser cannot read the item around it.
			if !owned {
				macro.blocks = append(macro.blocks, *kid)
			}
			owned = false
		}
	}
	sub := &walker{
		ctx:       w.ctx,
		source:    w.source[kids[0].EndByte():kids[len(kids)-1].StartByte()],
		file:      w.file,
		rowOffset: w.rowOffset + open.Row,
		colOffset: w.column(open),
		depth:     depth,
		macro:     macro,
		err:       w.err,
	}

	// A fresh parser and tree for this body; no node of them outlives this
	// call.
	parser, err := tsparse.NewParser(rustLanguage())
	if err != nil {
		*w.err = fmt.Errorf("failed to create Rust parser: %w", err)
		return
	}
	defer parser.Close()
	tree, err := tsparse.ParseBudget(w.ctx, parser, sub.source, parseBudget(len(sub.source)))
	if errors.Is(err, tsparse.ErrBudget) {
		// The parse needs more than the budget of the body (parseBudget): the
		// body yields no IR.
		return
	}
	if err != nil {
		*w.err = fmt.Errorf("failed to parse a Rust macro body: %w", err)
		return
	}
	defer tree.Close()

	root := tree.RootNode()
	if root.Kind() == "ERROR" {
		// Nothing in the body parses: the root is one top-level ERROR node,
		// and only the {} blocks inside it are read.
		sub.expandBlocksIn(root, parent, in)
		return
	}
	cursor := root.Walk()
	defer cursor.Close()
	sub.cursor = cursor
	sub.items(root, parent, in)
}

// itemStarts are the first tokens of a macro body that holds items: the
// keywords an item can start with, an attribute (#) and the if of cfg_if!.
// extern, macro_rules, union and default are identifiers in a token tree.
var itemStarts = map[string]bool{
	"pub": true, "fn": true, "struct": true, "enum": true, "union": true, "trait": true,
	"impl": true, "mod": true, "use": true, "const": true, "static": true, "type": true,
	"extern": true, "unsafe": true, "async": true, "crate": true, "macro_rules": true,
	"default": true, "#": true, "if": true,
}

// startsLikeItems reports whether tokens, the tokens of a macro body, start
// like items: with one of itemStarts or a macro call (a name or path followed
// by !), after any comments, docs included; a body of comments only does too.
// Any other body (match arms, tables, a language of the macro's own) is not
// parsed as items: tree-sitter's error recovery takes time quadratic in the
// length of text that is not items.
func (w *walker) startsLikeItems(tokens []sitter.Node) bool {
	i := 0
	for i < len(tokens) && isComment(&tokens[i]) {
		i++
	}
	if i == len(tokens) {
		// Comments only: they are kept, and cost nothing to parse.
		return true
	}
	first := tokens[i].Kind()
	if first == "identifier" {
		first = w.text(&tokens[i])
	}
	if itemStarts[first] {
		return true
	}
	// A macro call: a path such as cfg_if or cfg_if::cfg_if, then !.
	for start := i; i < len(tokens); i++ {
		switch tokens[i].Kind() {
		case "identifier", "self", "super", "::":
		case "!":
			return i > start && tokens[i-1].Kind() != "::"
		default:
			return false
		}
	}
	return false
}

// parseBudget is the number of progress checks (tsparse.ParseBudget) the
// parse of a macro body of n bytes may take: budgetChecksPerKiB for each KiB
// plus budgetChecksBase, and never more than budgetChecksMax. The parser
// checks its progress at an almost constant rate per byte, also while it
// recovers from errors, but recovery over a long run of text that is not
// items takes time quadratic in its length (a 720 KiB table took 15,400
// checks and 18 s), so only the cap bounds that time.
func parseBudget(n int) int {
	return min(n*budgetChecksPerKiB/1024+budgetChecksBase, budgetChecksMax)
}

// The parse budget of a macro body, measured on the 2,585 macro bodies of
// 2,165 Rust files of 21 crates: a body took at most 22.1 checks per KiB
// (the median of the bodies over 1 KiB is 5.5) and 412 checks in all (62 KiB
// of nix). The budget is four times both; the cap is about 75 KiB of the
// densest code, and a table that long parses in about 0.25 s.
const (
	budgetChecksPerKiB = 89
	budgetChecksBase   = 100
	budgetChecksMax    = 1648
)

// expandBlocksIn expands the {} blocks of the macro body w walks that lie
// entirely inside node, a top-level node of the body's parse, one level
// deeper.
func (w *walker) expandBlocksIn(node *sitter.Node, parent *ir.DistilledClass, in container) {
	start, end := w.macro.start+node.StartByte(), w.macro.start+node.EndByte()
	for i := range w.macro.blocks {
		block := &w.macro.blocks[i]
		if block.StartByte() >= start && block.EndByte() <= end {
			w.macro.outer.expand(block, parent, in, w.depth+1)
		}
	}
}

// misnamed reports whether the parser read the name of item, a node of a
// macro body, across an error: an ERROR or missing node lies between the
// keyword before the name (fn, struct, const, for) and the name, or inside
// the name. A macro can give an item a name that is not Rust (bitflags'
// pub struct Mode: libc::mode_t, paste's pub fn [<get_ $name>]), and the
// parser then takes a later token for the name. An error before the keyword
// (Rust 2024 pub safe fn, libc's pub {const} fn) or after the name (syn's
// pub struct ExprArray #full { ... }) leaves the name right. The name of an
// impl is its type.
func (w *walker) misnamed(item *sitter.Node) bool {
	field := "name"
	if item.Kind() == "impl_item" {
		field = "type"
	}
	name := item.ChildByFieldName(field)
	if name == nil {
		return false
	}
	if name.IsMissing() || name.HasError() {
		return true
	}
	broken := false
	for _, child := range w.children(item) {
		switch {
		case child.Equals(*name):
			return broken
		case child.IsError() || child.IsMissing():
			broken = true
		case isKeyword(&child):
			// What comes before the keyword does not touch the name.
			broken = false
		}
	}
	return broken
}

// isKeyword reports whether node is a keyword token (fn, struct, for).
func isKeyword(node *sitter.Node) bool {
	kind := node.Kind()
	return !node.IsNamed() && kind != "" && kind[0] >= 'a' && kind[0] <= 'z'
}

// appendStatement appends the IR of one statement, or nil for a statement
// that yields none, to list. A nil statement takes the outer doc comments
// right before it along, as commentutil.AppendStatement does, so they do not
// appear to document the next item. Inner docs (//!, /*!) document the
// enclosing file or module and stay: a crate doc is often followed by
// #![...] attributes, which yield no IR.
func appendStatement(list []ir.DistilledNode, node ir.DistilledNode) []ir.DistilledNode {
	if node != nil {
		return append(list, node)
	}
	start := len(list)
	for start > 0 {
		if _, ok := list[start-1].(*ir.DistilledComment); !ok {
			break
		}
		start--
	}
	kept := list[:start]
	for _, waiting := range list[start:] {
		if c := waiting.(*ir.DistilledComment); c.Inner || !c.IsDoc() {
			kept = append(kept, c)
		}
	}
	return kept
}

// item returns the IR of one item in a list of kind in, or nil for an item
// that yields none in this list (a macro definition, a macro call statement
// such as m!(...);, a union, an extern block or crate, an inner attribute, a
// trait const).
func (w *walker) item(node *sitter.Node, in container) ir.DistilledNode {
	switch node.Kind() {
	case "function_item", "function_signature_item":
		return w.function(node, in)
	case "struct_item":
		return w.structItem(node)
	case "enum_item":
		return w.enumItem(node)
	case "trait_item":
		return w.traitItem(node)
	case "impl_item":
		return w.implItem(node)
	case "mod_item":
		return w.modItem(node)
	case "use_declaration":
		return w.useDeclaration(node)
	case "const_item":
		if in == inTrait {
			return nil
		}
		return w.valueItem(node, ir.ModifierFinal)
	case "static_item":
		return w.valueItem(node, ir.ModifierStatic)
	case "type_item":
		if in == inTrait {
			return nil
		}
		return w.typeItem(node)
	case "associated_type":
		return w.associatedType(node)
	}
	return nil
}

// function returns a free function, a method or a trait method. A function
// without a body (a required trait method) is abstract.
func (w *walker) function(node *sitter.Node, in container) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	fn := &ir.DistilledFunction{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: w.visibility(node),
		Modifiers:  w.functionModifiers(node),
		Parameters: w.parameters(node.ChildByFieldName("parameters")),
	}
	if in == inTrait || in == inTraitImpl && w.visibilityModifier(node) == nil {
		// Trait members and the members of a trait impl take no visibility
		// keyword; they are as visible as the trait.
		fn.Visibility = ir.VisibilityPublic
	}

	returns := ""
	if ret := node.ChildByFieldName("return_type"); ret != nil {
		returns = w.flat(ret)
	}
	if where := w.whereClause(node); where != "" {
		returns = strings.TrimSpace(returns + " " + where)
	}
	if returns != "" {
		fn.Returns = &ir.TypeRef{Name: returns}
	}

	if body := node.ChildByFieldName("body"); body != nil {
		fn.Implementation = w.text(body)
	} else {
		fn.Modifiers = append(fn.Modifiers, ir.ModifierAbstract)
	}
	return fn
}

// functionModifiers returns async for an async fn and final for a const fn;
// unsafe, extern and default are not represented.
func (w *walker) functionModifiers(node *sitter.Node) []ir.Modifier {
	var async, constant bool
	for _, child := range w.children(node) {
		if child.Kind() != "function_modifiers" {
			continue
		}
		for _, keyword := range w.children(&child) {
			switch keyword.Kind() {
			case "async":
				async = true
			case "const":
				constant = true
			}
		}
	}
	var modifiers []ir.Modifier
	if async {
		modifiers = append(modifiers, ir.ModifierAsync)
	}
	if constant {
		modifiers = append(modifiers, ir.ModifierFinal)
	}
	return modifiers
}

// parameters returns the parameters of a parameters node. A self parameter
// keeps its whole text (&mut self) as its name and has no type; the name of
// any other parameter is its pattern, with mut when it is declared mut.
func (w *walker) parameters(node *sitter.Node) []ir.Parameter {
	params := []ir.Parameter{}
	if node == nil {
		return params
	}
	for _, child := range w.children(node) {
		switch child.Kind() {
		case "self_parameter", "variadic_parameter":
			params = append(params, ir.Parameter{Name: w.flat(&child)})
		case "parameter":
			pattern, typ := child.ChildByFieldName("pattern"), child.ChildByFieldName("type")
			if pattern == nil {
				continue
			}
			name, _ := w.flatRange(&child, child.StartByte(), pattern.EndByte())
			param := ir.Parameter{Name: name}
			if typ != nil {
				param.Type = ir.TypeRef{Name: w.flat(typ)}
			}
			params = append(params, param)
		}
	}
	return params
}

// structItem returns a struct with its named fields. Tuple fields and the
// comments of the body are not distilled.
func (w *walker) structItem(node *sitter.Node) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	class := &ir.DistilledClass{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierStruct},
		Children:   []ir.DistilledNode{},
	}
	body := node.ChildByFieldName("body")
	if body == nil || body.Kind() != "field_declaration_list" {
		return class
	}
	for _, child := range w.children(body) {
		if child.Kind() != "field_declaration" {
			continue
		}
		fieldName := child.ChildByFieldName("name")
		if fieldName == nil {
			continue
		}
		field := &ir.DistilledField{
			BaseNode:   ir.BaseNode{Location: w.location(&child)},
			Name:       w.text(fieldName),
			Visibility: w.visibility(&child),
		}
		if typ := child.ChildByFieldName("type"); typ != nil {
			field.Type = &ir.TypeRef{Name: w.flat(typ)}
		}
		class.Children = append(class.Children, field)
	}
	return class
}

// enumItem returns an enum whose variants are public fields: the payload as
// written ("(T, U)" or "{ x: T }", runs of white space collapsed, comments
// removed) is the type, and the discriminant is the default value.
func (w *walker) enumItem(node *sitter.Node) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	class := &ir.DistilledClass{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierEnum},
		Children:   []ir.DistilledNode{},
	}
	body := node.ChildByFieldName("body")
	if body == nil {
		return class
	}
	for _, child := range w.children(body) {
		if child.Kind() != "enum_variant" {
			continue
		}
		variantName := child.ChildByFieldName("name")
		if variantName == nil {
			continue
		}
		variant := &ir.DistilledField{
			BaseNode:   ir.BaseNode{Location: w.location(&child)},
			Name:       w.text(variantName),
			Visibility: ir.VisibilityPublic,
		}
		if payload := child.ChildByFieldName("body"); payload != nil {
			variant.Type = &ir.TypeRef{Name: collapse(w.code(payload))}
		}
		if value := child.ChildByFieldName("value"); value != nil {
			variant.DefaultValue = collapse(w.code(value))
		}
		class.Children = append(class.Children, variant)
	}
	return class
}

// traitItem returns a trait: an abstract class whose children are its
// methods, associated types and comments.
func (w *walker) traitItem(node *sitter.Node) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	class := &ir.DistilledClass{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierAbstract},
		Children:   []ir.DistilledNode{},
	}
	if body := node.ChildByFieldName("body"); body != nil {
		w.items(body, class, inTrait)
	}
	return class
}

// implItem returns an impl block: a public class named after its header
// ("impl<T> Trait for Type<T>", white space collapsed) whose children are the
// items of the block.
func (w *walker) implItem(node *sitter.Node) ir.DistilledNode {
	typ := node.ChildByFieldName("type")
	if typ == nil {
		return nil
	}
	name := "impl"
	if params := node.ChildByFieldName("type_parameters"); params != nil {
		name += w.generics(params)
	}
	name += " "
	in := inImpl
	if trait := node.ChildByFieldName("trait"); trait != nil {
		in = inTraitImpl
		for _, child := range w.children(node) {
			if child.Kind() == "!" {
				name += "!"
			}
		}
		name += w.flat(trait) + " for "
	}
	class := &ir.DistilledClass{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       collapse(name + w.flat(typ)),
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}
	if body := node.ChildByFieldName("body"); body != nil {
		w.items(body, class, in)
	}
	return class
}

// modItem returns a module: a class without modifiers whose children are the
// items of its body; "mod name;" has none.
func (w *walker) modItem(node *sitter.Node) ir.DistilledNode {
	name := node.ChildByFieldName("name")
	if name == nil {
		return nil
	}
	class := &ir.DistilledClass{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       w.text(name),
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{},
		Children:   []ir.DistilledNode{},
	}
	if body := node.ChildByFieldName("body"); body != nil {
		w.items(body, class, inMod)
	}
	return class
}

// useDeclaration returns a use declaration. "use a::{b, c as d}" gives the
// module a with the symbols b and c (alias d); any other tree is the module.
func (w *walker) useDeclaration(node *sitter.Node) ir.DistilledNode {
	arg := node.ChildByFieldName("argument")
	if arg == nil {
		return nil
	}
	imp := &ir.DistilledImport{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		ImportType: "use",
	}
	switch arg.Kind() {
	case "scoped_use_list":
		if path := arg.ChildByFieldName("path"); path != nil {
			imp.Module = normalize(w.code(path))
		}
		if list := arg.ChildByFieldName("list"); list != nil {
			imp.Symbols = w.useSymbols(list)
		}
	case "use_list":
		imp.Symbols = w.useSymbols(arg)
	default:
		imp.Module = normalize(w.code(arg))
	}
	return imp
}

// useSymbols returns the items of a use_list, each as written with its text
// normalised; "X as Y" is the symbol X with the alias Y.
func (w *walker) useSymbols(list *sitter.Node) []ir.ImportedSymbol {
	var symbols []ir.ImportedSymbol
	for _, child := range w.children(list) {
		if !child.IsNamed() || isComment(&child) {
			continue
		}
		if child.Kind() == "use_as_clause" {
			path, alias := child.ChildByFieldName("path"), child.ChildByFieldName("alias")
			if path != nil && alias != nil {
				symbols = append(symbols, ir.ImportedSymbol{Name: normalize(w.code(path)), Alias: w.text(alias)})
				continue
			}
		}
		symbols = append(symbols, ir.ImportedSymbol{Name: normalize(w.code(&child))})
	}
	return symbols
}

// valueItem returns a const (modifier final) or static (modifier static) as
// a field. Its value is kept when it fits on one line; a longer one is "...".
func (w *walker) valueItem(node *sitter.Node, kind ir.Modifier) ir.DistilledNode {
	name := node.ChildByFieldName("name")
	if name == nil {
		return nil
	}
	field := &ir.DistilledField{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       w.text(name),
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{kind},
		Type:       &ir.TypeRef{},
	}
	if typ := node.ChildByFieldName("type"); typ != nil {
		field.Type.Name = w.flat(typ)
	}
	if value := node.ChildByFieldName("value"); value != nil {
		if value.StartPosition().Row == value.EndPosition().Row {
			field.DefaultValue = w.text(value)
		} else {
			field.DefaultValue = "..."
		}
	}
	return field
}

// typeItem returns a type alias as a field with the modifier type_alias; its
// generics stay in the name, and a where clause follows the type.
func (w *walker) typeItem(node *sitter.Node) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	typ := ""
	if t := node.ChildByFieldName("type"); t != nil {
		typ = w.flat(t)
	}
	if where := w.whereClause(node); where != "" {
		typ = strings.TrimSpace(typ + " " + where)
	}
	return &ir.DistilledField{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: w.visibility(node),
		Modifiers:  []ir.Modifier{ir.ModifierTypeAlias},
		Type:       &ir.TypeRef{Name: typ},
	}
}

// associatedType returns an associated type of a trait (type Item: Bound
// where ...;) as a public type_alias field whose type is its bounds.
func (w *walker) associatedType(node *sitter.Node) ir.DistilledNode {
	name := w.nameWithGenerics(node)
	if name == "" {
		return nil
	}
	field := &ir.DistilledField{
		BaseNode:   ir.BaseNode{Location: w.location(node)},
		Name:       name,
		Visibility: ir.VisibilityPublic,
		Modifiers:  []ir.Modifier{ir.ModifierTypeAlias},
	}
	if bounds := node.ChildByFieldName("bounds"); bounds != nil {
		typ := strings.TrimSpace(strings.TrimPrefix(w.flat(bounds), ":"))
		if where := w.whereClause(node); where != "" {
			typ += " " + where
		}
		field.Type = &ir.TypeRef{Name: typ}
	}
	return field
}

// comment returns the comment of a line_comment or block_comment node. The
// text loses its delimiters and the doc marker ("/" or "!") after them. The
// grammar's markers decide the format: doc for ///, //!, /** and /*!
// (inner for //! and /*!), line for other // comments (also ////) and block
// for other /* */ comments.
func (w *walker) comment(node *sitter.Node) *ir.DistilledComment {
	raw := w.text(node)
	format, text, marker := ir.CommentLine, strings.TrimPrefix(raw, "//"), "/"
	if node.Kind() == "block_comment" {
		format, marker = ir.CommentBlock, "*"
		text = strings.TrimSuffix(strings.TrimPrefix(raw, "/*"), "*/")
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	text = strings.TrimPrefix(strings.TrimPrefix(text, marker), "!")
	inner := node.ChildByFieldName("inner") != nil
	if inner || node.ChildByFieldName("outer") != nil {
		format = ir.CommentDoc
	}

	loc := w.location(node)
	if end := node.EndPosition(); end.Column == 0 && end.Row > node.StartPosition().Row {
		// A doc line comment ends at column 0 of the next line, after its
		// newline; its text is on the line before.
		loc.EndLine = int(end.Row + w.rowOffset)
		loc.EndColumn = loc.StartColumn + len(strings.TrimRight(raw, "\r\n"))
	}
	return &ir.DistilledComment{
		BaseNode: ir.BaseNode{Location: loc},
		Text:     strings.TrimSpace(text),
		Format:   format,
		Inner:    inner,
	}
}

// visibilityModifier returns the visibility_modifier child of node, or nil.
func (w *walker) visibilityModifier(node *sitter.Node) *sitter.Node {
	for _, child := range w.children(node) {
		if child.Kind() == "visibility_modifier" {
			return &child
		}
	}
	return nil
}

// visibility maps the visibility of node: none is private, pub(crate)
// internal, pub(super) and pub(in path) protected, any other pub (also
// pub(self)) public, and the 2015 crate private.
func (w *walker) visibility(node *sitter.Node) ir.Visibility {
	modifier := w.visibilityModifier(node)
	if modifier == nil {
		return ir.VisibilityPrivate
	}
	vis := strings.Join(strings.Fields(w.code(modifier)), "")
	switch {
	case strings.HasPrefix(vis, "pub(crate)"):
		return ir.VisibilityInternal
	case strings.HasPrefix(vis, "pub(super)"), strings.HasPrefix(vis, "pub(in"):
		return ir.VisibilityProtected
	case strings.HasPrefix(vis, "pub"):
		return ir.VisibilityPublic
	}
	return ir.VisibilityPrivate
}

// nameWithGenerics returns the name of node followed by its generic
// parameters as written ("longest<'a, T>"), or "" when it has no name.
func (w *walker) nameWithGenerics(node *sitter.Node) string {
	name := node.ChildByFieldName("name")
	if name == nil {
		return ""
	}
	text := w.text(name)
	if params := node.ChildByFieldName("type_parameters"); params != nil {
		text += w.generics(params)
	}
	return text
}

// generics returns the text of a type_parameters node. A list written over
// several lines is normalised, and its angle brackets lose the spaces and
// the trailing comma inside them: "<\n    T,\n>" becomes "<T>".
func (w *walker) generics(node *sitter.Node) string {
	text, changed := w.flatRange(node, node.StartByte(), node.EndByte())
	if changed && strings.HasPrefix(text, "<") && strings.HasSuffix(text, ">") {
		inner := strings.TrimSpace(text[1 : len(text)-1])
		text = "<" + strings.TrimSpace(strings.TrimSuffix(inner, ",")) + ">"
	}
	return text
}

// whereClause returns the where clause of node ("where T: Clone,"), with a
// clause written over several lines normalised; its trailing comma stays.
func (w *walker) whereClause(node *sitter.Node) string {
	for _, child := range w.children(node) {
		if child.Kind() == "where_clause" {
			return w.flat(&child)
		}
	}
	return ""
}

// text returns the source text of node.
func (w *walker) text(node *sitter.Node) string {
	return string(w.source[node.StartByte():node.EndByte()])
}

// flat returns the text of node for a signature: as written when it is one
// line without comments, otherwise without its comments and normalised.
func (w *walker) flat(node *sitter.Node) string {
	text, _ := w.flatRange(node, node.StartByte(), node.EndByte())
	return text
}

// flatRange is flat for the bytes start to end of node, and also reports
// whether the text was normalised.
func (w *walker) flatRange(node *sitter.Node, start, end uint) (string, bool) {
	text, cut := w.codeRange(node, start, end)
	if cut || strings.ContainsAny(text, "\r\n") {
		return normalize(text), true
	}
	return text, false
}

// code returns the source text of node without the comments inside it.
func (w *walker) code(node *sitter.Node) string {
	text, _ := w.codeRange(node, node.StartByte(), node.EndByte())
	return text
}

// codeRange returns the source bytes start to end, which lie inside node,
// with each comment inside them replaced by a space, and whether there was
// one.
func (w *walker) codeRange(node *sitter.Node, start, end uint) (string, bool) {
	text := string(w.source[start:end])
	if !strings.Contains(text, "//") && !strings.Contains(text, "/*") {
		return text, false
	}
	var b strings.Builder
	pos := start
	w.cutComments(node, start, end, &pos, &b)
	if pos == start {
		return text, false
	}
	b.Write(w.source[pos:end])
	return b.String(), true
}

// cutComments writes to b the source from *pos up to each comment inside
// node that lies between start and end, writes a space for the comment, and
// moves *pos past it.
func (w *walker) cutComments(node *sitter.Node, start, end uint, pos *uint, b *strings.Builder) {
	for _, child := range w.children(node) {
		if child.EndByte() <= start || child.StartByte() >= end {
			continue
		}
		if isComment(&child) {
			b.Write(w.source[*pos:child.StartByte()])
			b.WriteByte(' ')
			*pos = child.EndByte()
			continue
		}
		if child.ChildCount() > 0 {
			w.cutComments(&child, start, end, pos, b)
		}
	}
}

// isComment reports whether node is a Rust comment.
func isComment(node *sitter.Node) bool {
	return node.Kind() == "line_comment" || node.Kind() == "block_comment"
}

// location converts the position of node to an IR location in the file.
func (w *walker) location(node *sitter.Node) ir.Location {
	start, end := node.StartPosition(), node.EndPosition()
	return ir.Location{
		StartLine:   int(start.Row+w.rowOffset) + 1,
		StartColumn: int(w.column(start)) + 1,
		EndLine:     int(end.Row+w.rowOffset) + 1,
		EndColumn:   int(w.column(end)) + 1,
	}
}

// column returns the 0-based column in the file of point, a position in
// w.source.
func (w *walker) column(point sitter.Point) uint {
	if point.Row == 0 {
		return point.Column + w.colOffset
	}
	return point.Column
}

// lastRow returns the row of the last character of node: a node that ends
// after a newline (a doc line comment at its end) ends on the row before.
func lastRow(node *sitter.Node) int {
	end := node.EndPosition()
	if end.Column == 0 && end.Row > node.StartPosition().Row {
		return int(end.Row) - 1
	}
	return int(end.Row)
}

// collapse replaces each run of white space in s with one space and trims
// its ends.
func collapse(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// normalize returns s on one line: runs of white space collapsed to one
// space, no space after "(", "[" or "{" nor before ")", "]" or "}", and no
// comma directly before a closing bracket.
func normalize(s string) string {
	s = collapse(s)
	closes := func(i int) bool { return i < len(s) && strings.IndexByte(")]}", s[i]) >= 0 }
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ' ' && (len(out) > 0 && strings.IndexByte("([{", out[len(out)-1]) >= 0 || closes(i+1)):
			continue
		case c == ',' && (closes(i+1) || i+1 < len(s) && s[i+1] == ' ' && closes(i+2)):
			continue
		}
		out = append(out, c)
	}
	return string(out)
}
