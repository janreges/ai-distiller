package commentutil_test

import (
	"testing"

	"github.com/janreges/ai-distiller/internal/ir"
	"github.com/janreges/ai-distiller/internal/language/commentutil"
	"github.com/stretchr/testify/assert"
)

func TestStripCStyleDelimiters(t *testing.T) {
	cases := []struct{ raw, text, format string }{
		{"// hello", "hello", ir.CommentLine},
		{"//no space", "no space", ir.CommentLine},
		{"// a */ b", "a */ b", ir.CommentLine},
		{"/* block */", "block", ir.CommentBlock},
		{"/** Doc. */", "Doc.", ir.CommentDoc},
		{"/**\n * First.\n * Second.\n */", "First.\nSecond.", ir.CommentDoc},
		{"/// Triple", "Triple", ir.CommentDoc},
		{"/**/", "", ir.CommentBlock},
	}
	for _, c := range cases {
		text, format := commentutil.StripCStyle(c.raw)
		assert.Equal(t, c.text, text, c.raw)
		assert.Equal(t, c.format, format, c.raw)
	}
}

// Shapes found in real sources: gutters without a space or missing on a
// line, indentation kept under the gutter, CRLF line ends, blocks without
// gutters, banner comments and separator lines.
func TestStripCStyleShapes(t *testing.T) {
	cases := []struct{ raw, text, format string }{
		{"/**\n * Text.\n* No space before the gutter.\n *Glued.\n */", "Text.\nNo space before the gutter.\nGlued.", ir.CommentDoc},
		{"/**\n * Example:\n *\n *     run(1)\n */", "Example:\n\n    run(1)", ir.CommentDoc},
		{"/**\r\n * One.\r\n * Two.\r\n */", "One.\nTwo.", ir.CommentDoc},
		{"/*\n    first\n      nested\n*/", "first\n  nested", ir.CommentBlock},
		{"/*** banner ***/", "banner", ir.CommentBlock},
		{"/** Doc. **/", "Doc.", ir.CommentDoc},
		{"//// four", "four", ir.CommentLine},
		{"//////////", "", ir.CommentLine},
		{"/** */", "", ir.CommentDoc},
		{"/***/", "", ir.CommentBlock},
		{"/* *bold* */", "*bold*", ir.CommentBlock},
	}
	for _, c := range cases {
		text, format := commentutil.StripCStyle(c.raw)
		assert.Equal(t, c.text, text, c.raw)
		assert.Equal(t, c.format, format, c.raw)
	}
}

func TestStripJSStyle(t *testing.T) {
	text, format := commentutil.StripJSStyle(`/// <reference types="node" />`)
	assert.Equal(t, `<reference types="node" />`, text)
	assert.Equal(t, ir.CommentLine, format)
	_, format = commentutil.StripJSStyle("/** Doc. */")
	assert.Equal(t, ir.CommentDoc, format)
	_, format = commentutil.StripJSStyle("/* block */")
	assert.Equal(t, ir.CommentBlock, format)
}

func TestStripDoxygen(t *testing.T) {
	cases := []struct {
		raw, text, format string
		trailing          bool
	}{
		{"// line", "line", ir.CommentLine, false},
		{"/* block */", "block", ir.CommentBlock, false},
		{"/** Doc. */", "Doc.", ir.CommentDoc, false},
		{"/// Doc.", "Doc.", ir.CommentDoc, false},
		{"//! Qt doc.", "Qt doc.", ir.CommentDoc, false},
		{"/*! Qt doc. */", "Qt doc.", ir.CommentDoc, false},
		{"/*!\n * First.\n * Second.\n */", "First.\nSecond.", ir.CommentDoc, false},
		{"///< X coordinate", "X coordinate", ir.CommentDoc, true},
		{"//!< X coordinate", "X coordinate", ir.CommentDoc, true},
		{"/**< X coordinate */", "X coordinate", ir.CommentDoc, true},
		{"/*!< X coordinate */", "X coordinate", ir.CommentDoc, true},
		{"//// banner", "banner", ir.CommentLine, false},
		{"/*** banner ***/", "banner", ir.CommentBlock, false},
	}
	for _, c := range cases {
		text, format, trailing := commentutil.StripDoxygen(c.raw)
		assert.Equal(t, c.text, text, c.raw)
		assert.Equal(t, c.format, format, c.raw)
		assert.Equal(t, c.trailing, trailing, c.raw)
	}
}

func TestDocumentsFile(t *testing.T) {
	assert.True(t, commentutil.DocumentsFile("@file app.js\n@description The app."))
	assert.True(t, commentutil.DocumentsFile("Utilities.\n@module utils"))
	assert.True(t, commentutil.DocumentsFile("@fileoverview Helpers."))
	assert.True(t, commentutil.DocumentsFile("@packageDocumentation"))
	assert.False(t, commentutil.DocumentsFile("Creates a user.\n@param {string} name"))
	assert.False(t, commentutil.DocumentsFile("Mentions @file in the middle of a line."))
}

func TestAppendStatement(t *testing.T) {
	doc := &ir.DistilledComment{Text: "doc", Format: ir.CommentDoc}
	line := &ir.DistilledComment{Text: "line", Format: ir.CommentLine}
	marker := &ir.DistilledComment{Text: "module.exports = x", Format: ir.CommentExport}
	innerDoc := &ir.DistilledComment{Text: "inner", Format: ir.CommentDoc}
	fn := &ir.DistilledFunction{Name: "f"}
	earlier := &ir.DistilledComment{Text: "earlier", Format: ir.CommentDoc}
	prefix := []ir.DistilledNode{earlier, fn}

	// A statement with a declaration keeps the docs before it.
	assert.Equal(t, []ir.DistilledNode{earlier, fn, doc, line, fn},
		commentutil.AppendStatement([]ir.DistilledNode{earlier, fn, doc, line}, []ir.DistilledNode{fn}))
	// Nothing parsed: the docs before it go, the line comment and earlier nodes stay.
	assert.Equal(t, []ir.DistilledNode{earlier, fn, line},
		commentutil.AppendStatement(append(append([]ir.DistilledNode{}, prefix...), doc, line), nil))
	// Only comments parsed (an export marker, a doc inside it): their docs go too.
	assert.Equal(t, []ir.DistilledNode{earlier, fn, line, marker},
		commentutil.AppendStatement(append(append([]ir.DistilledNode{}, prefix...), doc, line), []ir.DistilledNode{innerDoc, marker}))
}
