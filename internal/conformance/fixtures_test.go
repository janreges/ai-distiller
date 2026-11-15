package conformance

// check is one capability assertion run against a single aid invocation.
type check struct {
	Cap         string   // capability id, unique within a language
	Flags       []string // extra CLI flags
	ViaStdin    bool     // pipe the fixture to stdin; pass no path and no --lang
	Contains    []string // each must occur in stdout
	NotContains []string // none may occur in stdout
	Once        []string // each must occur exactly once in stdout
}

// fixture is one source file and the checks run against it.
type fixture struct {
	Lang   string // processor name, e.g. "typescript"
	File   string // relative to internal/conformance/testdata
	Checks []check
}

// markers are the language-specific names the standard checks look for.
type markers struct {
	Public    []string // names that must be in default output; Public[0] is the type name
	Private   []string // names that must not be in default output
	ExpandSym string   // name of the function whose body holds CONF_BODY_A
}

// standard returns the ten checks every fixture must pass.
func standard(m markers) []check {
	return []check{
		{Cap: "public-decls", Contains: m.Public},
		{Cap: "private-hidden", NotContains: append(append([]string{}, m.Private...), "CONF_PRIVATE_DOC")},
		{Cap: "body-hidden-by-default", NotContains: []string{"CONF_BODY_A", "CONF_BODY_B"}},
		{Cap: "body-with-implementation", Flags: []string{"--implementation=1"}, Contains: []string{"CONF_BODY_A", "CONF_BODY_B"}},
		{Cap: "expand-by-name", Flags: []string{"--expand=" + m.ExpandSym}, Contains: []string{"CONF_BODY_A"}, NotContains: []string{"CONF_BODY_B"}},
		{Cap: "comment-hidden-by-default", NotContains: []string{"CONF_LINE_COMMENT"}},
		{Cap: "comment-with-comments", Flags: []string{"--comments=1"}, Contains: []string{"CONF_LINE_COMMENT"}},
		{Cap: "doc-by-default", Contains: []string{"CONF_DOC_COMMENT"}},
		{Cap: "doc-off", Flags: []string{"--docstrings=0"}, NotContains: []string{"CONF_DOC_COMMENT"}},
		{Cap: "public-off", Flags: []string{"--public=0", "--private=1", "--protected=1", "--internal=1"},
			Contains: []string{m.Private[0]}, NotContains: []string{m.ExpandSym}},
		{Cap: "stdin-detect", ViaStdin: true, Contains: []string{m.Public[0]}},
	}
}

var fixtures = []fixture{
	{Lang: "python", File: "python/sample.py", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "conf_field", "conf_public", "conf_other"},
		Private:   []string{"_conf_private"},
		ExpandSym: "conf_public",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        a = \"CONF_BODY_A\"\n        return a\n"}},
		check{Cap: "dataclass-fields", Contains: []string{"conf_id: int", `conf_name: str = "x"`}, NotContains: []string{"conf_id = int"}},
		check{Cap: "fn-doc-by-default", Contains: []string{"CONF_FN_DOC"}},
		check{Cap: "fn-doc-once", Flags: []string{"--implementation=1"}, Once: []string{"CONF_FN_DOC"}},
		check{Cap: "fn-doc-off", Flags: []string{"--implementation=1", "--docstrings=0"}, NotContains: []string{"CONF_FN_DOC"}})},
	{Lang: "typescript", File: "typescript/sample.ts", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "export-type-alias", Contains: []string{"export type ConfId = string"}},
		check{Cap: "decl-enum", Contains: []string{"export enum ConfColor {", "ConfRed", `ConfGreen = "green"`}},
		check{Cap: "decl-ambient-implicit", Contains: []string{"confAmbientImplicit(): void", "namespace ConfDeclared {", "confDeclaredFn(): void"}},
		check{Cap: "decl-namespace", Contains: []string{"namespace ConfSpace {", "confInSpace(): number"}},
		check{Cap: "decl-ambient-module", Contains: []string{`declare module "conf-ambient" {`, "confAmbient(): void"}},
		check{Cap: "fn-return-type", Contains: []string{"confTopLevel(n: number): number"}},
		check{Cap: "const-type", Contains: []string{"export const confConst: number = 1"}},
		check{Cap: "hash-private-hidden", NotContains: []string{"confSecret"}},
		check{Cap: "hash-method-hidden", NotContains: []string{"confHashMethod"}},
		check{Cap: "hash-private-named", Flags: []string{"--private=1"}, Contains: []string{"#confSecret"}},
		check{Cap: "arrow-expression-body", Flags: []string{"--implementation=1"}, Contains: []string{"confArrow(n: number) => n"}},
		check{Cap: "type-guard-return", Contains: []string{"confIsNum(x: unknown): x is number"}},
		check{Cap: "enum-computed-init", Contains: []string{"ConfShift = 1 << 2", "ConfNeg = -1"}},
		check{Cap: "body-indent", Flags: []string{"--implementation=1"},
			Contains: []string{"        const a = \"CONF_BODY_A\";\n        return a;\n    }\n"}})},
	{Lang: "javascript", File: "javascript/sample.js", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confPublic", "confOther"},
		Private:   []string{"#confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        const a = \"CONF_BODY_A\";\n        return a;\n    }\n"}},
		check{Cap: "underscore-method-hidden", NotContains: []string{"_confUnderscore"}},
		check{Cap: "hash-method-hidden", NotContains: []string{"confHashMethod"}},
		check{Cap: "arrow-expression-body", Flags: []string{"--implementation=1"}, Contains: []string{"=> a + 1"}})},
	{Lang: "go", File: "go/sample.go", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "ConfField", "ConfPublic", "ConfOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "ConfPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"    a := \"CONF_BODY_A\"\n    return a\n}\n"}},
		check{Cap: "body-nested-statement", Flags: []string{"--implementation=1"}, Contains: []string{"CONF_BODY_D"}})},
	{Lang: "rust", File: "rust/sample.rs", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "conf_field", "conf_public", "conf_other"},
		Private:   []string{"conf_private"},
		ExpandSym: "conf_public",
	}), check{Cap: "body-signature-once", Flags: []string{"--implementation=1"}, Once: []string{"fn conf_public"}},
		check{Cap: "body-indent", Flags: []string{"--implementation=1"},
			Contains: []string{"        let a = \"CONF_BODY_A\";\n        a\n    }\n"}},
		check{Cap: "one-line-body", Flags: []string{"--implementation=1"}, Contains: []string{"conf_quick(&self) -> i32 { 1 }"}},
		check{Cap: "const-case", Contains: []string{"conf_max"}, NotContains: []string{"CONF_MAX"}},
		check{Cap: "trait-impl-body", Flags: []string{"--expand=conf_area"}, Contains: []string{"CONF_BODY_C"}})},
	// An enum has its own file: the stdin detector reads "enum Name" as
	// TypeScript, so in sample.rs it would break rust/stdin-detect.
	{Lang: "rust", File: "rust/one_line_enum.rs", Checks: []check{
		{Cap: "one-line-enum", Contains: []string{"ConfRed", "ConfGreen"}},
	}},
	{Lang: "swift", File: "swift/sample.swift", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        let a = \"CONF_BODY_A\"\n        return a\n    }\n"}})},
	{Lang: "ruby", File: "ruby/sample.rb", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "conf_public", "conf_other"},
		Private:   []string{"conf_private"},
		ExpandSym: "conf_public",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        a = \"CONF_BODY_A\"\n        a\n"}},
		check{Cap: "require-kept", Flags: []string{"--filter-imports=false"}, Contains: []string{"json"}},
		check{Cap: "attr-reader", Contains: []string{"conf_name"}},
		check{Cap: "include-kept", Contains: []string{"Comparable"}})},
	{Lang: "java", File: "java/ConfWidget.java", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "throws-kept", Contains: []string{"confThrows() throws java.io.IOException"}},
		check{Cap: "body-indent", Flags: []string{"--implementation=1"},
			Contains: []string{"        String a = \"CONF_BODY_A\";\n        return a;\n"}})},
	{Lang: "java", File: "java/ConfVisibility.java", Checks: []check{
		{Cap: "public-off-keeps-private-enum-constants", Flags: []string{"--public=0", "--private=1"},
			Contains: []string{"package com.example.conf", "CONF_RED", "CONF_GREEN"}, NotContains: []string{"confPublicField"}}}},
	{Lang: "csharp", File: "csharp/ConfWidget.cs", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "ConfField", "ConfPublic", "ConfOther"},
		Private:   []string{"ConfPrivate"},
		ExpandSym: "ConfPublic",
	}), check{Cap: "compound-hidden-by-default", NotContains: []string{"ConfProtInt"}},
		check{Cap: "compound-with-protected", Flags: []string{"--protected=1"}, Contains: []string{"protected internal string ConfProtInt"}})},
	{Lang: "kotlin", File: "kotlin/ConfWidget.kt", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        val a = \"CONF_BODY_A\"\n        return a\n    }\n"}})},
	{Lang: "cpp", File: "cpp/sample.cpp", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        std::string a = \"CONF_BODY_A\";\n        return a;\n    }\n"}},
		check{Cap: "out-of-class-def", Contains: []string{"ConfWidget::confOut()"}, NotContains: []string{"std::string ();"}},
		check{Cap: "out-of-class-private-hidden", Contains: []string{"ConfInner::confInnerOpen()"}, NotContains: []string{"confHiddenOut", "confNsSecret"}},
		check{Cap: "out-of-class-expand", Flags: []string{"--expand=confOut"}, Contains: []string{"CONF_BODY_C"}, NotContains: []string{"CONF_BODY_A"}})},
	{Lang: "php", File: "php/sample.php", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        $a = \"CONF_BODY_A\";\n        return $a;\n"}})},
}
