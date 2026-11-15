package java

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/janreges/ai-distiller/internal/formatter"
	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/processor"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJavaConstructs(t *testing.T) {
	tests := []struct {
		name         string
		construct    string
		options      processor.ProcessOptions
		expectedFile string
	}{
		// Basic construct tests
		{
			name:      "Basic_Default",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        false,
				IncludeImplementation: false,
				IncludeComments:       false,
				IncludeDocstrings:     true,
				IncludeImports:        true,
				IncludeFields:         true,  // Fixed: default should include fields
				IncludeMethods:        true,  // Fixed: default should include methods
			},
			expectedFile: "default.txt",
		},
		{
			name:      "Basic_WithImplementation",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        false,
				IncludeImplementation: true,
				IncludeComments:       false,
				IncludeDocstrings:     true,
				IncludeImports:        true,
				IncludeFields:         true,  // Fixed: default should include fields
				IncludeMethods:        true,  // Fixed: default should include methods
			},
			expectedFile: "implementation=1.txt",
		},
		{
			name:      "Basic_WithPrivate",
			construct: "01_basic",
			options: processor.ProcessOptions{
				IncludePrivate:        true,
				IncludeImplementation: false,
				IncludeComments:       false,
				IncludeDocstrings:     true,
				IncludeImports:        true,
				IncludeFields:         true,  // Fixed: default should include fields
				IncludeMethods:        true,  // Fixed: default should include methods
			},
			expectedFile: "private=1,protected=1,internal=1,implementation=0.txt",
		},
	}

	p := NewProcessor()
	textFormatter := formatter.NewLanguageAwareTextFormatter(formatter.Options{})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Read source file
			sourcePath := filepath.Join("../../../testdata/java", tt.construct, "source.java")
			sourceFile, err := os.Open(sourcePath)
			if err != nil {
				t.Fatalf("Failed to open source file: %v", err)
			}
			defer sourceFile.Close()

			// Process the file
			result, err := p.ProcessWithOptions(context.Background(), sourceFile, "source.java", tt.options)
			if err != nil {
				t.Fatalf("Processing failed: %v", err)
			}

			// Format the result
			var output strings.Builder
			if err := textFormatter.Format(&output, result); err != nil {
				t.Fatalf("Formatting failed: %v", err)
			}

			// Read expected output
			expectedPath := filepath.Join("../../../testdata/java", tt.construct, "expected", tt.expectedFile)
			expectedBytes, err := os.ReadFile(expectedPath)
			if err != nil {
				t.Fatalf("Failed to read expected file: %v", err)
			}

			expected := strings.TrimSpace(string(expectedBytes))
			actual := strings.TrimSpace(output.String())

			if expected != actual {
				t.Errorf("Output mismatch for %s:\nExpected:\n%s\n\nActual:\n%s", tt.name, expected, actual)
			}
		})
	}
}

// TestJavaParserFeatures tests specific Java language features
func TestJavaParserFeatures(t *testing.T) {
	t.Run("Generics", func(t *testing.T) {
		source := `
package test;

public interface Container<T> {
    void add(T item);
    T get(int index);
}

public class StringContainer implements Container<String> {
    public void add(String item) { }
    public String get(int index) { return null; }
}
`
		proc := NewProcessor()
		ctx := context.Background()
		result, err := proc.Process(ctx, strings.NewReader(source), "test.java")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check that we have an interface with type parameters
		var foundInterface bool
		for _, child := range result.Children {
			if iface, ok := child.(*ir.DistilledInterface); ok {
				assert.Equal(t, "Container", iface.Name)
				// TODO: Check type parameters when parser supports them
				foundInterface = true
			}
		}
		assert.True(t, foundInterface, "Interface not found")
	})

	t.Run("Annotations", func(t *testing.T) {
		source := `
package test;

public class Test {
    @Override
    public String toString() {
        return "test";
    }
    
    @Deprecated
    @SuppressWarnings("unchecked")
    public void oldMethod() { }
}
`
		proc := NewProcessor()
		ctx := context.Background()
		result, err := proc.Process(ctx, strings.NewReader(source), "test.java")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Find the class
		var foundClass *ir.DistilledClass
		for _, child := range result.Children {
			if class, ok := child.(*ir.DistilledClass); ok {
				foundClass = class
				break
			}
		}
		require.NotNil(t, foundClass)

		// Check methods have decorators
		var toStringMethod, oldMethod *ir.DistilledFunction
		for _, child := range foundClass.Children {
			if fn, ok := child.(*ir.DistilledFunction); ok {
				if fn.Name == "toString" {
					toStringMethod = fn
				} else if fn.Name == "oldMethod" {
					oldMethod = fn
				}
			}
		}

		require.NotNil(t, toStringMethod)
		assert.Contains(t, toStringMethod.Decorators, "@Override")

		require.NotNil(t, oldMethod)
		assert.Contains(t, oldMethod.Decorators, "@Deprecated")
		assert.Contains(t, oldMethod.Decorators, "@SuppressWarnings(\"unchecked\")")
	})

	t.Run("Sealed Classes", func(t *testing.T) {
		source := `
package test;

public sealed class Shape permits Circle, Square { }
final class Circle extends Shape { }
final class Square extends Shape { }
`
		proc := NewProcessor()
		ctx := context.Background()
		result, err := proc.Process(ctx, strings.NewReader(source), "test.java")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Check we found all three classes
		classCount := 0
		for _, child := range result.Children {
			if _, ok := child.(*ir.DistilledClass); ok {
				classCount++
			}
		}
		assert.Equal(t, 3, classCount, "Expected 3 classes")
	})

	t.Run("Records", func(t *testing.T) {
		source := `
package test;

public record Point(int x, int y) {
    public Point {
        if (x < 0 || y < 0) {
            throw new IllegalArgumentException();
        }
    }
}
`
		proc := NewProcessor()
		ctx := context.Background()
		result, err := proc.Process(ctx, strings.NewReader(source), "test.java")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Records are represented as classes with IsRecord flag in Java extensions
		var foundRecord *ir.DistilledClass
		for _, child := range result.Children {
			if class, ok := child.(*ir.DistilledClass); ok {
				if class.Extensions != nil &&
					class.Extensions.Java != nil &&
					class.Extensions.Java.IsRecord {
					foundRecord = class
					break
				}
			}
		}
		assert.NotNil(t, foundRecord, "Record not found")
		assert.Equal(t, "Point", foundRecord.Name)
		assert.True(t, foundRecord.Extensions.Java.IsRecord, "IsRecord flag should be true")
	})
}

func parseJava(t *testing.T, src string) *ir.DistilledFile {
	t.Helper()
	file, err := NewProcessor().Process(context.Background(), strings.NewReader(src), "Test.java")
	require.NoError(t, err)
	require.NotNil(t, file)
	return file
}

// commentsOf returns the format and text of every comment among nodes.
func commentsOf(nodes []ir.DistilledNode) [][2]string {
	var got [][2]string
	for _, c := range nodes {
		if cm, ok := c.(*ir.DistilledComment); ok {
			got = append(got, [2]string{cm.Format, cm.Text})
		}
	}
	return got
}

func TestJava_Comments(t *testing.T) {
	file := parseJava(t, "// line c\n\n/** Doc c */\npublic class A {\n    public int f() { return 1; }\n}\n")
	assert.Equal(t, [][2]string{{ir.CommentLine, "line c"}, {ir.CommentDoc, "Doc c"}}, commentsOf(file.Children))
}

// labels names every node: "doc:<text>", "line:<text>" and "block:<text>" for
// comments, the name for the rest.
func labels(nodes []ir.DistilledNode) []string {
	var out []string
	for _, n := range nodes {
		switch n := n.(type) {
		case *ir.DistilledComment:
			out = append(out, n.Format+":"+n.Text)
		case *ir.DistilledFunction:
			out = append(out, n.Name)
		case *ir.DistilledField:
			out = append(out, n.Name)
		case *ir.DistilledClass:
			out = append(out, n.Name)
		case *ir.DistilledPackage:
			out = append(out, "package "+n.Name)
		}
	}
	return out
}

func TestJava_ClassBodyComments(t *testing.T) {
	file := parseJava(t, "package p;\n// after package\npublic class A {\n    // in body\n    /* block\n       two lines */\n    /** m doc */\n    public int f() { return 1; }\n    /*** banner ***/\n    public int x;\n}\n")
	assert.Equal(t, []string{"package p", "line:after package", "A"}, labels(file.Children))
	assert.Equal(t, []string{"line:in body", "block:block\ntwo lines", "doc:m doc", "f", "block:banner", "x"},
		labels(file.Children[2].(*ir.DistilledClass).Children))
}

// Java 23 Markdown doc comments: a run of "///" lines is one doc.
func TestJava_TripleSlashDocLines(t *testing.T) {
	file := parseJava(t, "/// First.\n/// Second.\npublic class A {}\n")
	assert.Equal(t, []string{"doc:First.\nSecond.", "A"}, labels(file.Children))
}

// Comments inside initializer blocks are implementation.
func TestJava_CommentsInsideInitializersStayOut(t *testing.T) {
	file := parseJava(t, "public class A {\n    static {\n        // in static\n    }\n    {\n        // in instance initializer\n    }\n    public int x;\n}\n")
	assert.Equal(t, []string{"x"}, labels(file.Children[0].(*ir.DistilledClass).Children))
}

// A doc before a member that adds no declaration (a static or instance
// initializer block, a record's compact constructor, an interface constant)
// is dropped with it: it must not document the next member.
func TestJava_DocOfInitializerDoesNotMoveToNextMember(t *testing.T) {
	file := parseJava(t, "public class A {\n    /** static init doc */\n    static { }\n    public void bar() {}\n    /** instance init doc */\n    { }\n    public void baz() {}\n}\n")
	assert.Equal(t, []string{"bar", "baz"}, labels(file.Children[0].(*ir.DistilledClass).Children))

	file = parseJava(t, "public record R(int x) {\n    /** compact doc */\n    public R { }\n    public int twice() { return x * 2; }\n}\n")
	assert.Equal(t, []string{"twice"}, labels(file.Children[0].(*ir.DistilledClass).Children))

	file = parseJava(t, "public interface I {\n    /** constant doc */\n    int X = 1;\n    void m();\n}\n")
	assert.Equal(t, []string{"m"}, labels(file.Children[0].(*ir.DistilledInterface).Children))
}

// A plain comment above the Javadoc of a removed member goes with the doc and
// the member (spec H1): the Javadoc carries its line numbers, so the stripper
// sees the comments as one run directly above the member.
func TestJava_CommentAboveJavadocOfRemovedMemberLeavesWithIt(t *testing.T) {
	src := "public class A {\n    // about secret\n    /**\n     * Secret doc.\n     */\n    private int secret;\n\n    // Public API.\n\n    public void open() {}\n}\n"
	opts := processor.DefaultProcessOptions()
	opts.IncludePrivate = false
	opts.IncludeComments = true
	file, err := NewProcessor().ProcessWithOptions(context.Background(), strings.NewReader(src), "Test.java", opts)
	require.NoError(t, err)
	assert.Equal(t, []string{"line:Public API.", "open"}, labels(file.Children[0].(*ir.DistilledClass).Children))

	doc := parseJava(t, src).Children[0].(*ir.DistilledClass).Children[1].(*ir.DistilledComment)
	assert.Equal(t, [2]int{3, 5}, [2]int{doc.Location.StartLine, doc.Location.EndLine}, "the Javadoc keeps its lines")
}
