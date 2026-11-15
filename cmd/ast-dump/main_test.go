//go:build cgo
// +build cgo

package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDump_PythonFunction(t *testing.T) {
	out, err := dump("python", []byte("def f():\n    return 1\n"), false)
	require.NoError(t, err)
	assert.Contains(t, out, "function_definition [0:0-1:12]")
	assert.Contains(t, out, `name: identifier [0:4-0:5] "f"`)
}

func TestDump_AllIncludesAnonymousNodes(t *testing.T) {
	named, _ := dump("python", []byte("x = 1\n"), false)
	all, _ := dump("python", []byte("x = 1\n"), true)
	assert.NotContains(t, named, `"="`)
	assert.Contains(t, all, `"="`)
}

func TestDump_EveryTreeSitterLanguageParses(t *testing.T) {
	for _, lang := range []string{"python", "typescript", "tsx", "javascript", "ruby", "java", "csharp", "kotlin", "cpp", "php"} {
		out, err := dump(lang, []byte("x"), false)
		require.NoError(t, err, lang)
		assert.NotEmpty(t, out, lang)
	}
}

func TestDump_UnsupportedLanguage(t *testing.T) {
	for _, lang := range []string{"go", "rust", "swift"} {
		_, err := dump(lang, []byte("x"), false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not parsed with tree-sitter")
	}
}
