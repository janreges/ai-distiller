package tsparse

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	sitter "github.com/tree-sitter/go-tree-sitter"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
)

func newPythonParser(t *testing.T) *sitter.Parser {
	t.Helper()
	parser, err := NewParser(sitter.NewLanguage(tree_sitter_python.Language()))
	require.NoError(t, err)
	t.Cleanup(parser.Close)
	return parser
}

func TestParse_Background(t *testing.T) {
	parser := newPythonParser(t)

	tree, err := Parse(context.Background(), parser, []byte("def f(x):\n    return x\n"))
	require.NoError(t, err)
	defer tree.Close()

	root := tree.RootNode()
	require.Equal(t, "module", root.Kind())
	require.Equal(t, "function_definition", root.Child(0).Kind())
}

func TestParse_ContextDoneBeforeParse(t *testing.T) {
	parser := newPythonParser(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tree, err := Parse(ctx, parser, []byte("x = 1\n"))
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tree)
}

// A context cancelled while tree-sitter parses stops the parse with the
// context's error instead of crashing (Parser.ParseCtx of v0.25.0 dereferences
// a nil cancellation flag here), and the parser parses the next text afresh.
func TestParse_ContextCancelledDuringParse(t *testing.T) {
	parser := newPythonParser(t)
	// About 10 MB; parsing it takes far longer than the timeout.
	big := []byte(strings.Repeat("def f(x):\n    return [y for y in range(x) if y]\n", 200000))

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	start := time.Now()
	tree, err := Parse(ctx, parser, big)
	if tree != nil {
		tree.Close()
		t.Fatalf("the parse finished in %v before the context was cancelled; enlarge the input", time.Since(start))
	}
	require.True(t, errors.Is(err, context.DeadlineExceeded), "err = %v", err)

	tree, err = Parse(context.Background(), parser, []byte("class A:\n    pass\n"))
	require.NoError(t, err)
	defer tree.Close()
	require.Equal(t, "(module (class_definition name: (identifier) body: (block (pass_statement))))", tree.RootNode().ToSexp())
}

func TestParse_CancellableContextNotCancelled(t *testing.T) {
	parser := newPythonParser(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := []byte("import os\n\nclass A:\n    def m(self):\n        return os.sep\n")

	tree, err := Parse(ctx, parser, src)
	require.NoError(t, err)
	defer tree.Close()

	plain, err := Parse(context.Background(), parser, src)
	require.NoError(t, err)
	defer plain.Close()
	require.Equal(t, plain.RootNode().ToSexp(), tree.RootNode().ToSexp())
	require.Equal(t, uint(len(src)), tree.RootNode().EndByte())
}

// A parse that needs more progress checks than its budget stops with
// ErrBudget, and the parser parses the next text afresh.
func TestParseBudget_StopsAndLeavesTheParserReusable(t *testing.T) {
	parser := newPythonParser(t)
	big := []byte(strings.Repeat("def f(x):\n    return [y for y in range(x) if y]\n", 2000))

	tree, err := ParseBudget(context.Background(), parser, big, 10)
	if tree != nil {
		tree.Close()
		t.Fatal("the parse finished within 10 progress checks; enlarge the input")
	}
	require.ErrorIs(t, err, ErrBudget)

	tree, err = ParseBudget(context.Background(), parser, []byte("class A:\n    pass\n"), 10)
	require.NoError(t, err)
	defer tree.Close()
	require.Equal(t, "(module (class_definition name: (identifier) body: (block (pass_statement))))", tree.RootNode().ToSexp())
}

// A parse within its budget gives the tree Parse gives.
func TestParseBudget_WithinBudget(t *testing.T) {
	parser := newPythonParser(t)
	src := []byte(strings.Repeat("def f(x):\n    return [y for y in range(x) if y]\n", 200))

	tree, err := ParseBudget(context.Background(), parser, src, 1_000_000)
	require.NoError(t, err)
	defer tree.Close()

	plain, err := Parse(context.Background(), parser, src)
	require.NoError(t, err)
	defer plain.Close()
	require.Equal(t, plain.RootNode().ToSexp(), tree.RootNode().ToSexp())
}

// A context that is done stops a budgeted parse with its error, not ErrBudget.
func TestParseBudget_ContextDone(t *testing.T) {
	parser := newPythonParser(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tree, err := ParseBudget(ctx, parser, []byte("x = 1\n"), 1_000_000)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, tree)
}
