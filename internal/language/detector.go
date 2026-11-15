package language

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
)

// LanguageDetector detects programming language from code content
type LanguageDetector struct {
	maxBytes int
}

// NewDetector creates a new language detector
func NewDetector() *LanguageDetector {
	return &LanguageDetector{
		maxBytes: 64 * 1024, // 64KB for detection
	}
}

// DetectFromReader detects language from an io.Reader
func (d *LanguageDetector) DetectFromReader(reader io.Reader) (string, error) {
	// Read up to maxBytes for detection
	buf := make([]byte, d.maxBytes)
	n, err := reader.Read(buf)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Create a buffer that can be re-read
	content := buf[:n]

	// Try to detect language
	detected := d.detectLanguage(content)
	if detected == "" {
		return "", fmt.Errorf("could not detect language")
	}

	return detected, nil
}

// detectLanguage analyzes content and returns detected language
func (d *LanguageDetector) detectLanguage(content []byte) string {
	text := string(content)

	// 1. Check shebang first
	if lang := d.checkShebang(text); lang != "" {
		return lang
	}

	// 2. Check for high-confidence markers
	scores := make(map[string]int)

	// PHP - highest priority due to unique <?php tag
	if matched, _ := regexp.MatchString(`<\?php`, text); matched {
		return "php"
	}

	// Rust - unique syntax patterns
	if d.hasRustPatterns(text) {
		scores["rust"] += 100
	}

	// Go - package and := are very distinctive
	if d.hasGoPatterns(text) {
		scores["go"] += 100
	}

	// C++ - preprocessor directives
	if d.hasCppPatterns(text) {
		scores["cpp"] += 100
	}

	// Swift - import UIKit/Foundation
	if d.hasSwiftPatterns(text) {
		scores["swift"] += 100
	}

	// C# - using System, namespace
	if d.hasCSharpPatterns(text) {
		scores["csharp"] += 100
	}

	// Kotlin - fun and val keywords
	if d.hasKotlinPatterns(text) {
		scores["kotlin"] += 90
	}

	// Java - must check after Kotlin
	if d.hasJavaPatterns(text) {
		scores["java"] += 80
	}

	// Ruby - def/end blocks
	if d.hasRubyPatterns(text) {
		scores["ruby"] += 90
	}

	// Python - def with colons
	if d.hasPythonPatterns(text) {
		scores["python"] += 85
	}

	// TypeScript - outranks JavaScript only, so a language with its own
	// markers wins over the keywords TypeScript shares with it.
	if d.hasTypeScriptPatterns(text) {
		scores["typescript"] += 75
	}

	// JavaScript - fallback for JS family
	if d.hasJavaScriptPatterns(text) {
		scores["javascript"] += 70
	}

	// Find language with highest score; ties go to the alphabetically first
	// name so that detection does not depend on map iteration order.
	langs := make([]string, 0, len(scores))
	for lang := range scores {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	maxScore := 0
	detectedLang := ""
	for _, lang := range langs {
		if scores[lang] > maxScore {
			maxScore = scores[lang]
			detectedLang = lang
		}
	}

	// Require minimum score threshold
	if maxScore >= 70 {
		return detectedLang
	}

	return ""
}

// checkShebang checks for shebang line
func (d *LanguageDetector) checkShebang(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}

	shebang := lines[0]
	if !strings.HasPrefix(shebang, "#!") {
		return ""
	}

	switch {
	case strings.Contains(shebang, "python"):
		return "python"
	case strings.Contains(shebang, "ruby"):
		return "ruby"
	case strings.Contains(shebang, "node"):
		return "javascript"
	case strings.Contains(shebang, "php"):
		return "php"
	case strings.Contains(shebang, "swift"):
		return "swift"
	}

	return ""
}

// Language-specific pattern detection functions

func (d *LanguageDetector) hasRustPatterns(text string) bool {
	rustPatterns := []string{
		`\bfn\s+\w+\s*(<[^>]*>)?\s*\(`,                           // fn function_name( or fn name<T>(
		`\bpub(\([^)]*\))?\s+(fn|struct|enum|trait|mod|const)\b`, // pub item
		`[(,]\s*\w+\s*:\s*&|(:|->)\s*&?(mut\s+)?(i8|i16|i32|i64|i128|isize|u8|u16|u32|u64|u128|usize|f32|f64)\b`, // (s: &T), -> f64
		`\blet\s+mut\s+`,        // let mut
		`\bimpl\s+\w+\s+for\s+`, // impl Trait for Type
		`\btrait\s+\w+\s*\{`,    // trait Name {
		`\buse\s+std::`,         // use std::
		`::<\w+>`,               // ::<Type> turbofish
	}

	score := 0
	for _, pattern := range rustPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasGoPatterns(text string) bool {
	goPatterns := []string{
		`^package\s+\w+`,                       // package main
		`^import\s+\(`,                         // import (
		`\bfunc\s+(\(\w+\s+\*?\w+\)\s+)?\w+\(`, // func or method
		`:=`,                                   // := short assignment
		`\bdefer\s+`,                           // defer keyword
		`\bchan\s+`,                            // chan keyword
		`\bgoroutine\s+`,                       // go keyword for goroutines
	}

	score := 0
	for _, pattern := range goPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasCppPatterns(text string) bool {
	cppPatterns := []string{
		`#include\s*<\w+>`,           // #include <iostream>
		`#pragma\s+once`,             // #pragma once
		`\bstd::\w+`,                 // std::cout
		`\btemplate\s*<`,             // template<
		`\bclass\s+\w+\s*:\s*public`, // class inheritance
		`\bnamespace\s+\w+\s*\{`,     // namespace
	}

	score := 0
	for _, pattern := range cppPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasSwiftPatterns(text string) bool {
	swiftPatterns := []string{
		`\bimport\s+(UIKit|Foundation|SwiftUI)`, // iOS imports
		`\b(var|let)\s+\w+\s*:\s*\w+`,           // var/let name: Type (TypeScript has both, so one pattern)
		`\bfunc\s+\w+\([^)]*\)\s*->`,            // func name() -> Type
		`\bprotocol\s+\w+`,                      // protocol Name
		`\benum\s+\w+[^{]*\{\s*case\s+\w+`,      // enum Name { case a, b
		`!\s*$`,                                 // forced unwrap at end of line
	}

	score := 0
	for _, pattern := range swiftPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasCSharpPatterns(text string) bool {
	csharpPatterns := []string{
		`\busing\s+System`,                // using System;
		`\bnamespace\s+\w+`,               // namespace Name
		`(?m)^\s*\[\w+(\(.*\))?\]\s*$`,    // [Attribute] on its own line, not an index a[i]
		`\bpublic\s+class\s+\w+`,          // public class Name
		`\bget;\s*set;`,                   // get; set; properties
		`\basync\s+Task`,                  // async Task
		`\bpublic\s+static\s+void\s+Main`, // Main method
	}

	score := 0
	for _, pattern := range csharpPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasKotlinPatterns(text string) bool {
	kotlinPatterns := []string{
		`\bpackage\s+com\.`,   // package com.example
		`\bimport\s+kotlin\.`, // import kotlin.
		`\bfun\s+\w+\(`,       // fun functionName(
		`\bfun\s+(<[^>]*>\s*)?[\w.]+\s*\((\s*\w+\s*:\s*\w|[^)]*\)\s*:\s*\w)`, // fun with a typed parameter or return type
		`\bval\s+\w+`,    // val name
		`\?\.`,           // safe call operator
		`[\w)\]]!!`,      // not-null assertion x!! (JavaScript's !!x is a prefix)
		`\bdata\s+class`, // data class
	}

	score := 0
	for _, pattern := range kotlinPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasJavaPatterns(text string) bool {
	javaPatterns := []string{
		`(?m)^\s*package\s+[\w.]+\s*;`,            // package a.b; (Kotlin and Go have no semicolon)
		`\bimport\s+java\.`,                       // import java.
		`\bpublic\s+static\s+void\s+main`,         // main method
		`\bSystem\.out\.println`,                  // System.out.println
		`\bpublic\s+(class|interface|enum)\s+\w+`, // public class / interface / enum
		`\b(extends|implements)\s+\w+`,            // one pattern: TypeScript classes use both
		`@Override`,                               // @Override annotation
	}

	score := 0
	for _, pattern := range javaPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasRubyPatterns(text string) bool {
	rubyPatterns := []string{
		`\brequire(_relative)?\s+['"]`,                        // require 'gem'
		`(?m)^\s*def\s+(self\.)?\w+[?!=]?\s*(\([^)]*\))?\s*$`, // def name (a Python def ends with ":")
		`(?m)^\s*end\s*$`,                                     // end on its own line
		`(?m)^\s*attr_(reader|writer|accessor)\s+:`,           // attr_reader :name
		`\bputs\s+`,                            // puts
		`\|\|=`,                                // ||= operator
		`@\w+\s*=[^=]`,                         // @ivar = value (not a decorator or annotation)
		`(?m)^\s*class\s+\w+\s*<\s*[\w:]+\s*$`, // class Name < Base (not a generic class Name<T>)
	}

	score := 0
	for _, pattern := range rubyPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

func (d *LanguageDetector) hasPythonPatterns(text string) bool {
	pythonPatterns := []string{
		`\bdef\s+\w+\s*\([^)]*\)\s*(->\s*[^:\n]+)?:`, // def f(): or def f() -> T:
		`\bimport\s+\w+`,                                // import module
		`\bfrom\s+\w+\s+import`,                         // from module import
		`(?m)^\s*class\s+\w+(\([^)]*\))?\s*:\s*(#.*)?$`, // class Name: or class Name(Base):
		`\bself\b`, // self keyword
		`__init__`, // __init__ method
		`""".*"""`, // docstring
		`\bif\s+__name__\s*==\s*['"]__main__['"]`, // if __name__ == "__main__"
	}

	score := 0
	for _, pattern := range pythonPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}

// tsTypedParams matches a parameter list in which a parameter carries a type
// annotation: "(n: number)", "(a, b?: T)", "(private x: T)", "(...xs: T[])".
// The annotated name has to open the list or follow a comma, and the list may
// hold no braces or nested parentheses, so a JavaScript destructuring rename
// "({ a: b })" or a ternary "(a ? b : c)" is not taken for one.
const tsTypedParams = `\((?:[^(){};]*,)?\s*(?:(?:public|private|protected|readonly)\s+)*(?:\.\.\.)?\w+\??\s*:\s*[A-Za-z_][^(){};]*\)`

// tsMemberStart is the start of a class or interface member line up to its
// name, which is captured.
const tsMemberStart = `(?m)^[ \t]*(?:(?:public|private|protected|static|async|readonly|abstract|override|declare|get|set)\s+)*(\w+)\??[ \t]*(?:<[^>\n]*>)?[ \t]*`

// tsOnlyPatterns are TypeScript syntax that no other detected language has:
// type annotations in places JavaScript would accept the code without them,
// and TypeScript-only keywords. One of them is enough.
var tsOnlyPatterns = []string{
	`\bfunction\b[^(\n]*` + tsTypedParams,                                                                 // function f(n: number)
	`\bfunction\b[^(\n]*\([^(){};]*\)[ \t]*:[ \t]*[A-Za-z_{\[]`,                                           // function f(): T
	tsTypedParams + `\s*(:[^=;{}()]+)?=>`,                                                                 // (n: number) =>
	`\([^(){};]*\)[ \t]*:[ \t]*[A-Za-z_][^=;{}()\n]*=>`,                                                   // (n): T =>
	`\(\s*\{[^(){}]*\}\s*:\s*[A-Za-z_{]`,                                                                  // ({ name }: Props)
	tsMemberStart + `\([^(){};]*\)[ \t]*:[ \t]*[A-Za-z_\[][^;{}()\n]*(;|\s\{)`,                            // area(): number; / greet(): string {
	`\binterface\s+\w+(\s*<[^>]*>)?(\s+extends\s+[^{]+)?\s*\{\s*(readonly\s+)?\w+\??\s*:\s*[A-Za-z_{\[(]`, // interface I { name: T
	`\btype\s+\w+(\s*<[^>]*>)?\s*=\s*(\{|[^;\n]*\|)`,                                                      // type T = { or type T = A | B
	`\bexport\s+(default\s+)?(declare\s+)?(abstract\s+class|interface|type|enum|namespace|module|const\s+enum)\b`,
	`\bexport\s+(declare\s+)?(const|let|var)\s+\w+\s*:\s*[A-Za-z_{\[(]`, // export const a: number
	`\bdeclare\s+(module|namespace|global|const|let|var|function|class|enum|type|interface|abstract)\b`,
	`\bimport\s+type\s`,          // import type { T }
	`\breadonly\s+\w+\??\s*[:=]`, // readonly x: T (C# puts a type first)
	`\bas\s+const\b`,             // as const
}

// tsSharedPatterns are TypeScript keywords that Java, C#, Kotlin, Swift, Rust
// or Go also have. They count only next to JavaScript syntax (jsContextPatterns).
var tsSharedPatterns = []string{
	`\binterface\s+\w+(\s*<[^>]*>)?(\s+extends\s+[^{]+)?\s*\{`, // interface Name {
	`\benum\s+\w+\s*\{`,             // enum Name {
	`\btype\s+\w+(\s*<[^>]*>)?\s*=`, // type Name =
	`\bimplements\s+\w+`,            // implements Interface
}

// jsContextPatterns are JavaScript syntax that the languages sharing
// TypeScript's keywords do not have.
var jsContextPatterns = []string{
	`\bimport\s+[^;\n]*\bfrom\s+['"]`, // import x from "y"
	`(?m)^\s*export\s`,                // export ...
	`\brequire\(\s*['"]`,              // require("x")
	`\bfunction\s*\*?\s*\w*\s*\(`,     // function f(
	`\bconsole\.\w+\(`,                // console.log(
}

// tsClassMember matches a class member declaration with typed parameters and
// a body, e.g. "confPublic(n: number) {" or "constructor(private x: T) {".
const tsClassMember = tsMemberStart + tsTypedParams + `[ \t]*\{`

// phpTypedParam matches a PHP parameter written "type $name" ("(int $x)",
// "?string $a,", "string ...$rest)"); TypeScript writes "$name: type".
const phpTypedParam = `[(,]\s*\??[A-Za-z_\\][\w\\|]*\s+&?(\.\.\.)?\$\w+\s*[,)=]`

func (d *LanguageDetector) hasTypeScriptPatterns(text string) bool {
	// PHP without "<?php" has return types like TypeScript's.
	if matched, _ := regexp.MatchString(phpTypedParam, text); matched {
		return false
	}

	// A bare <Identifier> is no evidence (C++ includes, Java and C# generics,
	// XML doc comments, JSX), and neither is ": void" (C++ "public: void f();").
	if matchesAny(text, tsOnlyPatterns) {
		return true
	}

	// Typed parameters of a class member. Swift's "init(x: Int) {" is not one.
	if matched, _ := regexp.MatchString(`\bclass\b`, text); matched {
		for _, m := range regexp.MustCompile(tsClassMember).FindAllStringSubmatch(text, -1) {
			if m[1] != "init" {
				return true
			}
		}
	}

	return matchesAny(text, tsSharedPatterns) && matchesAny(text, jsContextPatterns)
}

// jsxElement matches a JSX closing tag (</div>, </Foo.Bar>, </>) or a
// self-closing element (<Foo />, <br/>); TypeScript generics and type
// assertions produce neither.
var jsxElement = regexp.MustCompile(`</([A-Za-z][\w.:-]*)?\s*>|<[A-Za-z][\w.:-]*(\s[^<>]*)?/>`)

// HasJSX reports whether source contains JSX elements. The TypeScript
// processor needs its TSX grammar for such source.
func HasJSX(source []byte) bool {
	return jsxElement.Match(source)
}

// matchesAny reports whether any of patterns matches text.
func matchesAny(text string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			return true
		}
	}
	return false
}

func (d *LanguageDetector) hasJavaScriptPatterns(text string) bool {
	jsPatterns := []string{
		`\bconst\s+\w+\s*=`,        // const variable
		`\blet\s+\w+\s*=`,          // let variable
		`\bfunction\s+\w+\(`,       // function declaration
		`\brequire\(['"]`,          // require('module')
		`\bconsole\.log`,           // console.log
		`=>`,                       // arrow function
		`\basync\s+function`,       // async function
		`\bawait\s+`,               // await keyword
		`\bexport\s+(default\s+)?`, // export
	}

	score := 0
	for _, pattern := range jsPatterns {
		if matched, _ := regexp.MatchString(pattern, text); matched {
			score++
		}
	}

	return score >= 2
}
