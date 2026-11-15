//go:build cgo
// +build cgo

// Command ast-dump prints the tree-sitter AST of a source file using the same
// grammars that aid's language processors use.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/kotlin"
	tree_sitter_c_sharp "github.com/tree-sitter/tree-sitter-c-sharp/bindings/go"
	tree_sitter_cpp "github.com/tree-sitter/tree-sitter-cpp/bindings/go"
	tree_sitter_java "github.com/tree-sitter/tree-sitter-java/bindings/go"
	tree_sitter_javascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tree_sitter_php "github.com/tree-sitter/tree-sitter-php/bindings/go"
	tree_sitter_python "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tree_sitter_ruby "github.com/tree-sitter/tree-sitter-ruby/bindings/go"
	typescript "tree-sitter-typescript"
)

const maxTextBytes = 60

func language(lang string) (*sitter.Language, error) {
	switch lang {
	case "python":
		return sitter.NewLanguage(tree_sitter_python.Language()), nil
	case "kotlin":
		return kotlin.GetLanguage(), nil
	case "typescript":
		return sitter.NewLanguage(typescript.Language()), nil
	case "tsx":
		return sitter.NewLanguage(typescript.LanguageTSX()), nil
	case "javascript":
		return sitter.NewLanguage(tree_sitter_javascript.Language()), nil
	case "ruby":
		return sitter.NewLanguage(tree_sitter_ruby.Language()), nil
	case "java":
		return sitter.NewLanguage(tree_sitter_java.Language()), nil
	case "csharp":
		return sitter.NewLanguage(tree_sitter_c_sharp.Language()), nil
	case "cpp":
		return sitter.NewLanguage(tree_sitter_cpp.Language()), nil
	case "php":
		return sitter.NewLanguage(tree_sitter_php.LanguagePHP()), nil
	case "go", "rust", "swift":
		return nil, fmt.Errorf("%s is not parsed with tree-sitter in aid (go uses go/parser; rust and swift use line parsers)", lang)
	}
	return nil, fmt.Errorf("unsupported language %q (supported: python, typescript, tsx, javascript, ruby, java, csharp, kotlin, cpp, php)", lang)
}

// dump parses src and renders its AST, one node per line. Anonymous nodes are
// included only when all is true.
func dump(lang string, src []byte, all bool) (string, error) {
	l, err := language(lang)
	if err != nil {
		return "", err
	}
	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(l)
	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return "", fmt.Errorf("parse %s: %w", lang, err)
	}
	defer tree.Close()

	var sb strings.Builder
	writeNode(&sb, tree.RootNode(), "", 0, src, all)
	return sb.String(), nil
}

func writeNode(sb *strings.Builder, n *sitter.Node, field string, depth int, src []byte, all bool) {
	sb.WriteString(strings.Repeat("  ", depth))
	if field != "" {
		sb.WriteString(field + ": ")
	}
	s, e := n.StartPoint(), n.EndPoint()
	fmt.Fprintf(sb, "%s [%d:%d-%d:%d]", n.Type(), s.Row, s.Column, e.Row, e.Column)
	if n.NamedChildCount() == 0 {
		sb.WriteString(" \"" + nodeText(n, src) + "\"")
	}
	sb.WriteString("\n")
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if !all && !c.IsNamed() {
			continue
		}
		writeNode(sb, c, n.FieldNameForChild(i), depth+1, src, all)
	}
}

func nodeText(n *sitter.Node, src []byte) string {
	start, end := n.StartByte(), n.EndByte()
	if start > end || int(end) > len(src) {
		return ""
	}
	b := src[start:end]
	suffix := ""
	if len(b) > maxTextBytes {
		b, suffix = b[:maxTextBytes], "..."
	}
	return strings.ReplaceAll(string(b), "\n", `\n`) + suffix
}

func main() {
	lang := flag.String("lang", "", "language to parse (python, typescript, tsx, javascript, ruby, java, csharp, kotlin, cpp, php)")
	all := flag.Bool("all", false, "also print anonymous nodes")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ast-dump --lang <lang> [--all] [file]  (reads stdin when file is missing or -)")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *lang == "" || flag.NArg() > 1 {
		flag.Usage()
		os.Exit(2)
	}

	// An unknown or unsupported language is a usage error (exit 2); only parse
	// failures exit 1.
	if _, err := language(*lang); err != nil {
		fmt.Fprintln(os.Stderr, "ast-dump:", err)
		os.Exit(2)
	}

	var (
		src []byte
		err error
	)
	if flag.NArg() == 0 || flag.Arg(0) == "-" {
		src, err = io.ReadAll(os.Stdin)
	} else {
		src, err = os.ReadFile(flag.Arg(0))
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "ast-dump:", err)
		os.Exit(2)
	}

	out, err := dump(*lang, src, *all)
	if err != nil {
		fmt.Fprintln(os.Stderr, "ast-dump:", err)
		os.Exit(1)
	}
	fmt.Print(out)
}
