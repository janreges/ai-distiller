// Package tsparse creates tree-sitter parsers and runs a parse that honours a
// context, on the official runtime github.com/tree-sitter/go-tree-sitter.
//
// The runtime has no finalizers: the caller closes every Parser and Tree it
// gets from here, and a Node must not be used after its Tree is closed (a Node
// does not keep its Tree alive).
package tsparse

import (
	"context"
	"errors"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

// ErrNoTree is returned when tree-sitter returns no tree although the context
// was not cancelled.
var ErrNoTree = errors.New("tree-sitter returned no tree")

// NewParser returns a parser for lang. The caller closes it.
//
// It fails when the grammar's ABI version is outside the range the runtime
// accepts (13 to 15 in v0.25.0).
func NewParser(lang *sitter.Language) (*sitter.Parser, error) {
	parser := sitter.NewParser()
	if err := parser.SetLanguage(lang); err != nil {
		parser.Close()
		return nil, err
	}
	return parser, nil
}

// Parse parses src with parser and returns the tree; the caller closes it.
//
// A context that is already done returns its error without parsing. A
// context that can be cancelled is checked while the parse runs (every 100
// parser operations), and its error is returned when it stops the parse; the
// parser is then reset, so it can parse another text.
//
// It does not call Parser.ParseCtx: in v0.25.0 that method stores into the
// parser's cancellation flag, which is nil unless the caller sets one, so a
// context cancelled during a parse crashes the process. A context that can
// never be cancelled (context.Background and contexts derived from it with
// values only, which is what the CLI passes) takes the plain Parse path,
// because ParseWithOptions registers its options with go-pointer and never
// releases them (an allocation of about a hundred bytes per call).
func Parse(ctx context.Context, parser *sitter.Parser, src []byte) (*sitter.Tree, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var tree *sitter.Tree
	if ctx.Done() == nil {
		tree = parser.Parse(src, nil)
	} else {
		read := func(offset int, _ sitter.Point) []byte {
			if offset < len(src) {
				return src[offset:]
			}
			return []byte{}
		}
		tree = parser.ParseWithOptions(read, nil, &sitter.ParseOptions{
			ProgressCallback: func(sitter.ParseState) bool { return ctx.Err() != nil },
		})
	}

	if tree == nil {
		// A stopped parse would otherwise resume on the parser's next call,
		// whatever text that call passes.
		parser.Reset()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrNoTree
	}
	return tree, nil
}
