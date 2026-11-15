package language

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func detect(t *testing.T, src string) string {
	lang, err := NewDetector().DetectFromReader(strings.NewReader(src))
	require.NoError(t, err, src)
	return lang
}

func TestDetector_ConformanceFixtures(t *testing.T) {
	files := map[string]string{"python": "python/sample.py", "typescript": "typescript/sample.ts", "javascript": "javascript/sample.js",
		"go": "go/sample.go", "rust": "rust/sample.rs", "swift": "swift/sample.swift", "ruby": "ruby/sample.rb", "java": "java/ConfWidget.java",
		"csharp": "csharp/ConfWidget.cs", "kotlin": "kotlin/ConfWidget.kt", "cpp": "cpp/sample.cpp", "php": "php/sample.php"}
	for want, rel := range files {
		src, err := os.ReadFile(filepath.Join("..", "conformance", "testdata", rel))
		require.NoError(t, err)
		got := detect(t, string(src))
		assert.Equal(t, want, got, rel)
		_, registered := GetProcessor(got)
		assert.True(t, registered, "detector returned %q, which has no processor", got)
	}
}

func TestDetector_GenericsAreNotTypeScript(t *testing.T) {
	assert.Equal(t, "cpp", detect(t, "#include <vector>\ntemplate<typename T> class Box { public: T get(); };\n"))
	assert.Equal(t, "rust", detect(t, "use std::collections::HashMap;\npub fn f<T>(x: Vec<T>) -> Option<T> { None }\n"))
	assert.Equal(t, "java", detect(t, "import java.util.List;\npublic class A {\n    public List<String> f() { return null; }\n}\n"))
	assert.Equal(t, "csharp", detect(t, "using System;\nnamespace N { public class A { public void F() {} } }\n"))
}

// Python with a decorator and annotated class attributes used to score as
// Ruby: "@dataclass" counted as an instance variable and "def" as a Ruby def.
func TestDetector_PythonDataclass(t *testing.T) {
	src := "from dataclasses import dataclass\n\n\n@dataclass\nclass ConfRecord:\n    conf_id: int\n    conf_name: str = \"x\"\n\n" +
		"    def conf_label(self) -> str:\n        return self.conf_name\n"
	assert.Equal(t, "python", detect(t, src))
}

// "enum Name {" is TypeScript evidence, but it must not outweigh the
// language's own markers.
func TestDetector_EnumsAreNotTypeScript(t *testing.T) {
	cases := map[string]string{
		"rust": "use std::fmt;\n\npub enum ConfColor { ConfRed, ConfGreen }\n\npub fn conf_name(c: &ConfColor) -> &'static str {\n    \"x\"\n}\n",
		"java": "package com.example.conf;\n\npublic class ConfVisibility {\n    public int confPublicField;\n\n" +
			"    private enum ConfColor { CONF_RED, CONF_GREEN }\n}\n",
		"csharp": "using System.IO;\n\nnamespace Conf;\n\npublic enum ConfColor { ConfRed, ConfGreen }\n\n" +
			"public class ConfWidget {\n    public ConfColor Color { get; set; }\n}\n",
		"kotlin": "package com.example.conf\n\nenum class ConfColor { CONF_RED, CONF_GREEN }\n\n" +
			"class ConfWidget {\n    val confColor: ConfColor = ConfColor.CONF_RED\n\n    fun confName(): String {\n        return confColor.name\n    }\n}\n",
	}
	for want, src := range cases {
		assert.Equal(t, want, detect(t, src), src)
	}
}

func TestDetector_TypeScriptEvidence(t *testing.T) {
	for _, src := range []string{
		"interface ConfShape {\n    area(): number;\n}\n",
		"interface ConfProps {\n    name: string;\n    age?: number;\n}\n",
		"type ConfId = string | number;\n",
		"export enum ConfColor { Red, Green }\n",
		"function confAdd(a: number, b: number) {\n    return a + b;\n}\n",
		"const confAdd = (a: number, b: number) => a + b;\n",
		"class ConfWidget {\n    confPublic(n: number) {\n        return n;\n    }\n}\n",
		// A keyword shared with Java/C# counts next to JavaScript syntax.
		"import { ConfShape } from \"./shape\";\n\nclass ConfWidget implements ConfShape {\n}\n",
		"class ConfWidget {\n    readonly confField = 1;\n}\n",
		"const conf = { a: 1 } as const;\n",
		"declare module \"conf-ambient\" {\n}\n",
	} {
		assert.Equal(t, "typescript", detect(t, src), src)
	}
}

func TestDetector_JavaScriptIsNotTypeScript(t *testing.T) {
	for _, src := range []string{
		// JSX: a bare <Identifier> is not a type argument.
		"import React from \"react\";\n\nexport function ConfApp() {\n    return <Widget />;\n}\nconst x = () => <Widget>text</Widget>;\n",
		// Destructuring with a rename looks like "name: type" inside a parameter list.
		"const confPick = ({ a: first, b: second }) => first + second;\nexport default confPick;\n",
		// An arrow function returning void is not an annotation.
		"const confRun = async () => {\n    await confWork();\n};\nexport { confRun };\n",
	} {
		assert.Equal(t, "javascript", detect(t, src), src)
	}
}

// Now that TypeScript no longer wins outright, the other languages' patterns
// must not claim TypeScript that shares some of their syntax.
func TestDetector_TypeScriptSharingOtherLanguagesSyntax(t *testing.T) {
	for _, src := range []string{
		// Java: extends and implements
		"export class ConfWidget extends ConfBase implements ConfShape {\n    confArea(n: number) {\n        return n;\n    }\n}\n",
		// Swift: let/var name: Type and optional chaining
		"let confWidget: ConfWidget = confMake();\nvar confCount: number = 0;\nconst confSize = confWidget?.size;\n" +
			"export function confArea(w: ConfWidget) {\n    return w?.size;\n}\n",
		// C#: namespace and an index expression
		"namespace ConfSpace {\n    export const confItems: string[] = [];\n    export function confFirst(i: number) {\n        return confItems[i];\n    }\n}\n",
		// Python: an import line and a colon after the class name
		"import React from \"react\";\n\nexport class ConfView extends React.Component<{ title: string }> {\n" +
			"    confRender(depth: number) {\n        return this.props.title;\n    }\n}\n",
	} {
		assert.Equal(t, "typescript", detect(t, src), src)
	}
}

func TestDetector_JavaScriptIsNotKotlin(t *testing.T) {
	src := "var confReady = !!window.conf;\n\nexport function confInit() {\n    return confReady?.value;\n}\n"
	assert.Equal(t, "javascript", detect(t, src))
}

// detectOrNone returns the detected language, or "" when detection fails
// (aid then asks for --lang).
func detectOrNone(src string) string {
	lang, err := NewDetector().DetectFromReader(strings.NewReader(src))
	if err != nil {
		return ""
	}
	return lang
}

// "interface Name {" and "enum Name {" are TypeScript keywords that Java, C#,
// Kotlin, Swift and Rust share. On their own they must not outrank a language
// that needs two markers. A snippet valid in two languages may stay undetected.
func TestDetector_SharedKeywordsAreNotTypeScript(t *testing.T) {
	cases := []struct {
		src    string
		accept []string
	}{
		{"interface Repo {\n    String find(int id);\n}\n", []string{"java", ""}},
		{"public class Order {\n    public enum Kind { SMALL, LARGE }\n\n    private Kind kind;\n\n" +
			"    public Kind getKind() {\n        return kind;\n    }\n}\n", []string{"java", "csharp", ""}},
		{"import java.util.*;\n\npublic interface Repo<T> {\n    T find(int id);\n    List<T> all();\n}\n", []string{"java"}},
		{"public interface IRepo {\n    void F();\n}\n", []string{"csharp", "java", ""}},
		{"public class Svc {\n    public string Name { get; set; }\n\n    public enum K { A, B }\n}\n", []string{"csharp"}},
		{"interface Repo {\n    fun find(id: Int): User?\n}\n", []string{"kotlin"}},
		{"enum Dir {\n    case up, down\n}\n\nfunc f(d: Dir) -> Int {\n    return 1\n}\n", []string{"swift"}},
		{"enum Shape {\n    Circle(f64),\n    Square(f64),\n}\n\nfn area(s: &Shape) -> f64 {\n    match s {\n" +
			"        Shape::Circle(r) => 3.14 * r * r,\n        Shape::Square(w) => w * w,\n    }\n}\n", []string{"rust"}},
		{"enum Color { Red, Green }\n", []string{""}},
		// Typed parameters that are not a TypeScript class member: a Swift
		// init, a C# named argument, a Go type alias.
		{"class Point {\n    var x: Int\n    init(x: Int) {\n        self.x = x\n    }\n}\n", []string{"swift", ""}},
		{"public class Svc {\n    public void Run() {\n        Foo(name: value);\n    }\n}\n", []string{"csharp", "java", ""}},
		{"type Handler = func(int) error\n\ntype Server struct {\n\tPort int\n}\n", []string{"go", ""}},
	}
	for _, c := range cases {
		assert.Contains(t, c.accept, detectOrNone(c.src), c.src)
	}
}

// A return type annotation must not hide a Python def.
func TestDetector_PythonReturnAnnotation(t *testing.T) {
	src := "def conf_add(x: int, y: int) -> int:\n    return x + y\n\n\nclass ConfA:\n    pass\n"
	assert.Equal(t, "python", detect(t, src))
}

// TypeScript whose only annotations are return types, field types or a typed
// destructured parameter is still TypeScript.
func TestDetector_TypeScriptWithoutParameterTypes(t *testing.T) {
	for _, src := range []string{
		"class ConfUser {\n    name: string;\n    private age: number;\n\n    greet(): string {\n        return this.name;\n    }\n}\n",
		"abstract class ConfShape {\n    abstract area(): number;\n}\n",
		"import { sep } from \"path\";\n\nexport const confA: number = 1;\n",
		"function confNow(): Date {\n    return new Date();\n}\n",
		// TSX: typed destructured props next to JSX
		"import { useState } from \"react\";\n\nexport function ConfGreeting({ name }: { name: string }) {\n" +
			"    const [count, setCount] = useState<number>(0);\n    return <p>{name} {count}</p>;\n}\n",
		"export const ConfBadge = ({ label }: ConfBadgeProps) => <span>{label}</span>;\n",
	} {
		assert.Equal(t, "typescript", detectOrNone(src), src)
	}
}

// PHP without "<?php": "function f(int $x): int {" looks like a TypeScript
// return type, but a parameter written "type $name" is PHP.
func TestDetector_PHPWithoutTagIsNotTypeScript(t *testing.T) {
	for _, src := range []string{
		"class ConfCalc {\n    public function confAdd(int $x): int {\n        return $x + 1;\n    }\n}\n",
		"function confJoin(?string $a, string ...$rest): string {\n    return $a;\n}\n",
	} {
		assert.NotEqual(t, "typescript", detectOrNone(src), src)
	}
	// TypeScript allows $ in identifiers.
	assert.Equal(t, "typescript", detectOrNone("function confBind($el: HTMLElement): void {\n}\n"))
}

func TestHasJSX(t *testing.T) {
	for _, src := range []string{
		"return <Hello name={title} />;",
		"const x = <h1>Hello {name}</h1>;",
		"return (\n  <>\n    <Item/>\n  </>\n);",
	} {
		assert.True(t, HasJSX([]byte(src)), src)
	}
	for _, src := range []string{
		"const m = new Map<string, Array<number>>();",
		"const x = <Foo>bar;",
		"function id<T>(x: T): T {\n    return x;\n}\n",
		"if (a < b && c > d) {\n}\n",
	} {
		assert.False(t, HasJSX([]byte(src)), src)
	}
}
