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
		check{Cap: "arrow-expression-body", Flags: []string{"--implementation=1"}, Contains: []string{"export const confArrow = (n: number) => n"}},
		check{Cap: "type-guard-return", Contains: []string{"confIsNum(x: unknown): x is number"}},
		check{Cap: "enum-computed-init", Contains: []string{"ConfShift = 1 << 2", "ConfNeg = -1"}},
		check{Cap: "import-forms", Contains: []string{
			"import ConfDefault, { confNamed as ConfAlias } from './conf-mod'",
			"import type { ConfOnlyType } from './conf-types'",
			"import * as confNs from './conf-ns'"}},
		check{Cap: "import-filter-alias", NotContains: []string{"ConfGoneAlias"}},
		check{Cap: "import-equals-alias", Contains: []string{"import ConfAliasUsed = confNs.ConfInner"},
			NotContains: []string{"ConfAliasGone"}},
		check{Cap: "import-equals-alias-unfiltered", Flags: []string{"--filter-imports=false"},
			Contains: []string{"import ConfAliasGone = confNs.ConfUnused"}},
		check{Cap: "export-as-namespace", Contains: []string{"export as namespace ConfLib"}},
		check{Cap: "export-import-require", Contains: []string{"export import confReq = require('conf-req')"}},
		check{Cap: "import-names-with-dollar", Contains: []string{
			"import type { $ConfDollar } from './conf-dollar'", "import { confObs$ } from './conf-obs'"}},
		check{Cap: "import-used-above-later-import", Contains: []string{"import { ConfAbove } from './conf-above'",
			"    import ConfLateAlias = confNs.ConfLateInner"}},
		check{Cap: "import-alias-uses-namespace", Contains: []string{"import * as confGeo from './conf-geo'",
			"import ConfPoly = confGeo.ConfPoly"}},
		check{Cap: "import-alias-kept-unfiltered", Flags: []string{"--filter-imports=false"},
			Contains: []string{"import { confGone as ConfGoneAlias } from './conf-gone'"}},
		check{Cap: "reexport-forms", Contains: []string{
			"export * from './conf-all'",
			"export * as confAllNs from './conf-all-ns'",
			"export { confPicked, confOrig as ConfRenamed } from './conf-picked'",
			"export type { ConfReType } from './conf-retype'"}},
		check{Cap: "reexport-kept-without-imports", Flags: []string{"--imports=0"}, Contains: []string{"export * from './conf-all'"}},
		check{Cap: "export-list-makes-public", Contains: []string{"const confListed = 2", "export { confListed }", "export default confListed",
			"enum ConfListedEnum {", "export { ConfListedEnum }"},
			NotContains: []string{"export enum ConfListedEnum"}},
		check{Cap: "parameters-as-written", Contains: []string{
			"export function confParams({ confA, confB = 1 }: ConfOpts, [confX]: [number], confOpt?: string, confDef: number = 0, ...confRest: string[]): void",
			"public readonly confRo: number",
			"public constructor(confRo: number, confHid: string)"}},
		check{Cap: "multi-line-type-on-one-line", Contains: []string{"export function confMultiType(confM: { confK: string; }): void"}},
		check{Cap: "wrapper-const-signature", Contains: []string{
			"export const confWrapped = confWrap<string>((confP: string))",
			"export const confGeneric = async <T>(confV: T): Promise<T>",
			"export const confTypedArrow: ConfHandler = (confReq)"},
			NotContains: []string{"CONF_WRAPPED_BODY"}},
		check{Cap: "wrapper-const-implementation", Flags: []string{"--implementation=1"}, Contains: []string{
			"export const confWrapped = confWrap<string>((confP: string) => {",
			`return "CONF_WRAPPED_BODY";`,
			"export const confGeneric = async <T>(confV: T): Promise<T> => confV",
			"export const confTypedArrow: ConfHandler = (confReq) => confReq"}},
		check{Cap: "accessors-generators-type-defaults", Contains: []string{
			"export class ConfAccessors<T extends object = {}> {",
			"public get confSize(): number",
			"public set confSize(v: number)",
			"public async *confIter(): AsyncGenerator<T>",
			"export function* confGen(): Generator<number>"}},
		check{Cap: "expand-accessor-generator-by-name", Flags: []string{"--expand=confSize,confIter,confGen"}, Contains: []string{
			"public get confSize(): number { return 0; }",
			"public set confSize(v: number) {}",
			"public async *confIter(): AsyncGenerator<T> {}",
			"export function* confGen(): Generator<number> {}"}},
		check{Cap: "declaration-keyword", Contains: []string{
			"export let confLetArrow = (confL: number)",
			"export var confVarValue = 3"},
			NotContains: []string{"function confLetArrow"}},
		check{Cap: "member-markers-as-written", Contains: []string{
			"    property readonly confRoId: string",
			"    property confOptEmail?: string",
			"    property [confKey: string]: unknown",
			"    method confOptRun?(confA: number): void",
			"    public [confK: string]: any",
			"    public confOptName?: string"}},
		check{Cap: "heritage-as-written", Contains: []string{
			"export interface ConfBased extends ConfGenericBase<string>, confNs.ConfQualified<number> {",
			"export class ConfDerived extends confNs.ConfBaseClass<{ id: number }> implements ConfBased {"}},
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
		check{Cap: "arrow-expression-body", Flags: []string{"--implementation=1"}, Contains: []string{"=> a + 1"}},
		check{Cap: "import-default-and-named", Flags: []string{"--filter-imports=false"},
			Contains: []string{"import ConfReact, { confHook as confUseHook } from 'conf-react'"}},
		check{Cap: "import-alias-used", Contains: []string{"import ConfReact, { confHook as confUseHook } from 'conf-react'"}},
		check{Cap: "wrapper-const-signature", Contains: []string{
			"export const confWrapped = confWrap((confP))",
			"export const confAsyncArrow = async (confU)"},
			NotContains: []string{"CONF_WRAPPED_BODY"}},
		check{Cap: "wrapper-const-implementation", Flags: []string{"--implementation=1"}, Contains: []string{
			"export const confWrapped = confWrap((confP) => {",
			`return "CONF_WRAPPED_BODY";`}},
		check{Cap: "export-keyword", Contains: []string{
			"export class ConfWidget:",
			"export function confWithHook(hook = confUseHook)",
			"export async function* confGen(n)",
			"export const confExported = 3"}},
		check{Cap: "reexport-forms", Contains: []string{
			"export * from './conf-all'",
			"export { confPicked as ConfRenamed } from './conf-picked'",
			"export { confArrow }",
			"export default ConfWidget"}},
		check{Cap: "declaration-keyword", Contains: []string{
			"export let confLetArrow = (confL)",
			"export var confVarValue = 3"},
			NotContains: []string{"function confLetArrow", "let confVarValue"}},
		check{Cap: "function-expression-constant", Contains: []string{
			"export const confFnExpr = function (confQ)",
			"export const confGenExpr = function* ()"},
			NotContains: []string{"confFnExpr = (confQ)"}},
		check{Cap: "generator-method-star", Contains: []string{
			"    *confGenMethod()",
			"    async *confAsyncGenMethod()"}},
		check{Cap: "expand-generator-by-name", Flags: []string{"--expand=confGenMethod"}, Contains: []string{
			"    *confGenMethod() {}"}},
		check{Cap: "jsx-keeps-react", Contains: []string{"import React from 'react'", "export function ConfJsxView()"}},
		check{Cap: "jsdoc-braced-type", Contains: []string{
			"export const confTypedMap: Map<string, Set<number>> = new Map()"}})},
	{Lang: "go", File: "go/sample.go", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "ConfField", "ConfPublic", "ConfOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "ConfPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"\ta := \"CONF_BODY_A\"\n\treturn a\n}\n"}},
		check{Cap: "tab-indent", Flags: []string{"--implementation=1", "--internal=1"},
			Contains:    []string{"type ConfWidget struct {\n\tConfField int\n}\n", "\n\t\"strings\"\n"},
			NotContains: []string{"    "}},
		check{Cap: "body-nested-statement", Flags: []string{"--implementation=1"}, Contains: []string{"CONF_BODY_D"}},
		check{Cap: "interface-embeds",
			Contains:    []string{"type ConfReadCloser interface {\n\tio.Reader\n\tio.Closer\n\tConfExtra() int\n}\n"},
			NotContains: []string{"io.Reader | io.Closer"}},
		check{Cap: "type-set-union", Contains: []string{"type ConfNumber interface {\n\t~int | ~int64\n}\n"}},
		check{Cap: "defined-type-vs-alias", Contains: []string{"type ConfID int\n", "type ConfName = string\n",
			"type ConfPair = struct {\n\tConfLeft int\n}\n", "type ConfRouter struct {\n"},
			NotContains: []string{"type ConfID = int"}},
		check{Cap: "func-types", Contains: []string{
			"type ConfHandler func(w io.Writer, n int) error\n",
			"\tConfStack []func(io.Writer) io.Writer\n",
			"func ConfMap(items []int, f func(int) (string, error), opts ...func(*ConfWidget)) []string\n"}},
		check{Cap: "member-comments-in-place", Flags: []string{"--comments=1"}, Contains: []string{
			"type ConfRecord struct {\n\t// CONF_FIELD_DOC\n\tConfNote string // CONF_FIELD_TRAILING\n}\n",
			"\tio.Closer\n\t// CONF_METHOD_DOC\n\tConfExtra() int // CONF_METHOD_TRAILING\n}\n"},
			NotContains: []string{"CONF_HIDDEN_FIELD_NOTE"}},
		check{Cap: "member-comments-hidden-by-default", NotContains: []string{"CONF_FIELD_DOC", "CONF_METHOD_TRAILING"}},
		// An embedded element prints where it is written, with its comments.
		check{Cap: "embed-comments-in-place", Flags: []string{"--comments=1"}, Contains: []string{
			"type ConfLate interface {\n\tConfFirst()\n\t// CONF_EMBED_DOC\n\tio.Writer // CONF_EMBED_TRAILING\n}\n"}},
		check{Cap: "hidden-field-comment-shown", Flags: []string{"--comments=1", "--internal=1"},
			Contains: []string{"\tconfSecret int // CONF_HIDDEN_FIELD_NOTE\n}\n"}},
		// A comment in a function body is part of the body: it prints with
		// the body, once, and goes with a hidden function.
		check{Cap: "body-comments-in-body", Flags: []string{"--comments=1"},
			NotContains: []string{"CONF_BODY_NOTE", "CONF_HIDDEN_BODY_NOTE"}},
		check{Cap: "body-comment-printed-once", Flags: []string{"--comments=1", "--implementation=1"},
			Once: []string{"\t// CONF_BODY_NOTE\n"}, NotContains: []string{"CONF_HIDDEN_BODY_NOTE"}},
		check{Cap: "spec-comments-on-their-line", Flags: []string{"--comments=1"},
			Contains:    []string{"const ConfLimit = 10 // CONF_CONST_TRAILING\n", "type ConfKind int // CONF_TYPE_TRAILING\n"},
			NotContains: []string{"CONF_HIDDEN_CONST_NOTE"}},
		check{Cap: "spec-comments-hidden-by-default", NotContains: []string{"CONF_CONST_TRAILING", "CONF_TYPE_TRAILING"}},
		// One blank line between declarations, a method's included, above
		// the doc; none between a doc and its declaration.
		check{Cap: "doc-on-its-declaration", Contains: []string{
			")\n\n// CONF_DOC_COMMENT\ntype ConfWidget struct {\n",
			"}\n\nfunc (w *ConfWidget) ConfPublic() string\n\nfunc (w *ConfWidget) ConfOther() string\n"}},
		check{Cap: "free-comment-set-off", Flags: []string{"--comments=1"},
			Contains: []string{")\n\n// CONF_LINE_COMMENT\n\n// CONF_DOC_COMMENT\ntype ConfWidget struct {\n"}},
		// strings is used only in confPrivate's body: the import filter
		// drops it in the public output, and its comments with it.
		check{Cap: "import-comments-in-block", Flags: []string{"--comments=1", "--implementation=1", "--internal=1"},
			Contains: []string{"import (\n\t\"io\" // CONF_IMPORT_TRAILING\n\t// CONF_IMPORT_DOC\n" +
				"\t\"strings\" // CONF_DROPPED_IMPORT_NOTE\n\t// CONF_IMPORT_END\n)\n"}},
		check{Cap: "dropped-import-comments-go", Flags: []string{"--comments=1"},
			Contains:    []string{"import (\n\t\"io\" // CONF_IMPORT_TRAILING\n\t// CONF_IMPORT_END\n)\n"},
			NotContains: []string{"CONF_IMPORT_DOC", "CONF_DROPPED_IMPORT_NOTE"}},
		// A comment inside a declaration prints with it: a signature that
		// holds one prints as written, one in a value printed as {...}
		// under the declaration; it goes with a hidden declaration.
		check{Cap: "signature-comments-as-written", Flags: []string{"--comments=1"},
			Contains: []string{"// ConfOpen has a commented parameter.\nfunc ConfOpen(\n\tpath string, // CONF_PARAM_NOTE\n) error\n"}},
		check{Cap: "value-comments-under-declaration", Flags: []string{"--comments=1"},
			Contains:    []string{"// ConfTable is a commented table.\nvar ConfTable = map[string]int{...}\n\t// CONF_VALUE_NOTE\n"},
			NotContains: []string{"CONF_HIDDEN_VALUE_NOTE"}},
		check{Cap: "declaration-comments-hidden-by-default",
			Contains:    []string{"func ConfOpen(path string) error\n", "var ConfTable = map[string]int{...}\n"},
			NotContains: []string{"CONF_PARAM_NOTE", "CONF_VALUE_NOTE"}},
		// The comment at the end of a one-line function's line prints
		// there, and goes with a hidden function.
		check{Cap: "func-line-comment-on-its-line", Flags: []string{"--comments=1"},
			Contains:    []string{"\nfunc ConfQuick() int // CONF_FUNC_TRAILING\n"},
			NotContains: []string{"CONF_HIDDEN_FUNC_NOTE"}},
		check{Cap: "func-line-comment-after-body", Flags: []string{"--comments=1", "--implementation=1"},
			Contains: []string{"\nfunc ConfQuick() int { return 1 } // CONF_FUNC_TRAILING\n"}},
		// Only confPrivate, confSecret, confHiddenMax and confHiddenQuick
		// print, and none names io or strings: the import filter drops
		// both, and the block with them.
		check{Cap: "no-empty-import-block", Flags: []string{"--public=0", "--internal=1"},
			Contains: []string{"package conf\n\n// CONF_PRIVATE_DOC\n"}, NotContains: []string{"import"}},
		// A free comment before the imports (not their declaration's doc)
		// keeps its blank line and stays when the block goes.
		check{Cap: "free-comment-before-imports", Flags: []string{"--comments=1"},
			Contains: []string{"package conf\n\n// CONF_FILE_NOTE\n\nimport (\n"}},
		check{Cap: "free-comment-outlives-import-block", Flags: []string{"--comments=1", "--public=0", "--internal=1"},
			Contains: []string{"package conf\n\n// CONF_FILE_NOTE\n\n// CONF_LINE_COMMENT\n"}, NotContains: []string{"import"}})},
	{Lang: "rust", File: "rust/sample.rs", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "conf_field", "conf_public", "conf_other"},
		Private:   []string{"conf_private"},
		ExpandSym: "conf_public",
	}), check{Cap: "body-signature-once", Flags: []string{"--implementation=1"}, Once: []string{"fn conf_public"}},
		check{Cap: "body-indent", Flags: []string{"--implementation=1"},
			Contains: []string{"        let a = \"CONF_BODY_A\";\n        a\n    }\n"}},
		check{Cap: "one-line-body", Flags: []string{"--implementation=1"}, Contains: []string{"conf_quick(&self) -> i32 { 1 }"}},
		check{Cap: "const-case", Contains: []string{"conf_max"}, NotContains: []string{"CONF_MAX"}},
		check{Cap: "trait-impl-body", Flags: []string{"--expand=conf_area"}, Contains: []string{"CONF_BODY_C"}},
		check{Cap: "one-line-enum", Contains: []string{"ConfRed", "ConfGreen"}},
		check{Cap: "private-type-impls-hidden", NotContains: []string{"ConfHidden", "conf_hidden_method", "CONF_HIDDEN_IMPL_DOC", "ConfCrate"}},
		check{Cap: "private-type-impls-with-private", Flags: []string{"--private=1"},
			Contains:    []string{"/// CONF_HIDDEN_IMPL_DOC\nimpl<'a> ConfHidden<'a> {", "impl ConfShape for ConfHidden<'_> {"},
			NotContains: []string{"ConfCrate"}},
		check{Cap: "crate-type-impls-with-internal", Flags: []string{"--internal=1"},
			Contains: []string{"impl ConfCrate {\n    pub fn conf_crate_method(&self)\n}"}, NotContains: []string{"ConfHidden"}},
		// The private ConfImported of conf_tests is another type than the
		// imported one, and the pub use makes conf_engine's ConfEngine public.
		check{Cap: "same-name-in-private-module",
			Contains: []string{"impl ConfImported {\n    pub fn conf_imported_method(&self)\n}"}},
		check{Cap: "reexported-type-impls",
			Contains: []string{"impl ConfEngine {\n    pub fn conf_engine_start(&self)\n}"}},
		check{Cap: "body-starts-after-brace",
			Contains: []string{"impl ConfWidget {\n    pub fn conf_public(&self)", "pub trait ConfShape {\n    fn conf_area(&self) -> i32;"}})},
	{Lang: "swift", File: "swift/sample.swift", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        let a = \"CONF_BODY_A\"\n        return a\n    }\n"}},
		check{Cap: "struct-and-default", Contains: []string{"public struct ConfPoint", "confX: Int = 0"}, NotContains: []string{"= = 0"}})},
	{Lang: "ruby", File: "ruby/sample.rb", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "conf_public", "conf_other"},
		Private:   []string{"conf_private"},
		ExpandSym: "conf_public",
	}), check{Cap: "body-indent", Flags: []string{"--implementation=1"},
		Contains: []string{"        a = \"CONF_BODY_A\"\n        a\n"}},
		check{Cap: "require-kept", Flags: []string{"--filter-imports=false"}, Contains: []string{"json"}},
		check{Cap: "attr-reader", Contains: []string{"conf_name"}},
		check{Cap: "splat-param", Contains: []string{"conf_splat(*conf_args)"}},
		check{Cap: "include-kept", Contains: []string{"Comparable"}})},
	{Lang: "java", File: "java/ConfWidget.java", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "confField", "confPublic", "confOther"},
		Private:   []string{"confPrivate"},
		ExpandSym: "confPublic",
	}), check{Cap: "throws-kept", Contains: []string{"confThrows() throws java.io.IOException"}},
		check{Cap: "body-indent", Flags: []string{"--implementation=1"},
			Contains: []string{"        String a = \"CONF_BODY_A\";\n        return a;\n"}},
		check{Cap: "public-off-keeps-private-enum-constants", Flags: []string{"--public=0", "--private=1"},
			Contains: []string{"package com.example.conf", "CONF_RED", "CONF_GREEN"}, NotContains: []string{"confField"}})},
	{Lang: "csharp", File: "csharp/ConfWidget.cs", Checks: append(standard(markers{
		Public:    []string{"ConfWidget", "ConfField", "ConfPublic", "ConfOther"},
		Private:   []string{"ConfPrivate"},
		ExpandSym: "ConfPublic",
	}), check{Cap: "compound-hidden-by-default", NotContains: []string{"ConfProtInt", "ConfPrivProt"}},
		check{Cap: "compound-with-protected", Flags: []string{"--protected=1"},
			Contains: []string{"protected internal string ConfProtInt", "private protected string ConfPrivProt"}},
		check{Cap: "compound-with-internal", Flags: []string{"--internal=1"},
			Contains: []string{"protected internal string ConfProtInt"}, NotContains: []string{"ConfPrivProt"}},
		check{Cap: "compound-with-private", Flags: []string{"--private=1"},
			Contains: []string{"private protected string ConfPrivProt", "ConfPrivate"}, NotContains: []string{"ConfProtInt"}},
		check{Cap: "enum-members", Contains: []string{"ConfRed,", "ConfGreen = 2"}, NotContains: []string{"static const"}})},
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
		Contains: []string{"        $a = \"CONF_BODY_A\";\n        return $a;\n"}},
		// A use statement prints as written, and the import filter keeps it
		// while its name (the alias, else the last segment) is in the output.
		check{Cap: "use-as-written", Contains: []string{
			"use Conf\\Base\\ConfBase as ConfAliasBase;\n",
			"use Conf\\Contracts\\{ConfShown as ConfAliasShown, ConfUnusedMember};\n",
			"class ConfWidget extends ConfAliasBase implements ConfAliasShown {\n    use ConfTrait;\n"}},
		// The keyword of a statement with several clauses applies to each.
		check{Cap: "use-function-const", Flags: []string{"--implementation=1"}, Contains: []string{
			"use function Conf\\Helpers\\conf_helper;\nuse function Conf\\Helpers\\conf_other as confOtherFn;\n",
			"use const Conf\\Helpers\\CONF_LIMIT;\nuse const Conf\\Helpers\\CONF_MAX;\n"}},
		// Parameters and return types print as written.
		check{Cap: "signature-as-written", Contains: []string{
			"    public static confMake(int ...$confIds): static\n",
			"    public &confRef(array &$confArr): array\n",
			"    public confNever(): never\n",
			"conf_dnf((ConfA&ConfB)|null $confDnf): (ConfA&ConfB)|null\n"}},
		// An enum's members print inside it, with or without a constant, and
		// its header keeps attributes and implements.
		check{Cap: "enum-members", Contains: []string{
			"#[ConfMarker]\nenum ConfStatus: string implements ConfAliasShown {\n    use ConfTrait;\n    case ConfActive = 'active';\n" +
				"    public const CONF_DEFAULT = self::ConfActive;\n    public confLabel(): string\n}\n",
			"enum ConfPure {\n    case ConfOne;\n    public confPure(): int\n}\n"}},
		// A class named like a cast type before `::` in an argument list
		// parses (tree-sitter-php v0.25.1 made it an ERROR).
		check{Cap: "cast-named-class", Contains: []string{
			"use Conf\\Types\\Integer;\n",
			"#[ConfCast(Integer::class)]\nfinal class ConfDto {\n    public confCast(): int\n}\n"}},
		// A constant keeps its visibility, in a class and in an enum, and its
		// type.
		check{Cap: "const-visibility", NotContains: []string{"CONF_SECRET", "CONF_ENUM_SECRET"},
			Contains: []string{"    public const int CONF_TYPED = 2;\n"}},
		check{Cap: "const-visibility-private", Flags: []string{"--private=1"}, Contains: []string{
			"    private const CONF_SECRET = 1;\n", "    private const CONF_ENUM_SECRET = 'x';\n"}})},
}
