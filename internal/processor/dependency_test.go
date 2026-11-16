package processor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandRustUseTree(t *testing.T) {
	cases := map[string][]string{
		"crate::a::B":                       {"crate::a::B"},
		"crate::a::{self, B}":               {"crate::a", "crate::a::B"},
		"crate::{a::B, c::{D, E as F}}":     {"crate::a::B", "crate::c::D", "crate::c::E"},
		"super::*":                          {"super::*"},
		"::std::fmt":                        {"std::fmt"},
		"crate::x::{\n    y::Z,\n    w,\n}": {"crate::x::y::Z", "crate::x::w"},
		"siteone_crawler::engine::initiator::Initiator as Init": {"siteone_crawler::engine::initiator::Initiator"},
	}
	for tree, want := range cases {
		assert.Equal(t, want, expandRustUseTree(tree), tree)
	}
}

func TestPhpImportedClasses(t *testing.T) {
	cases := map[string][]string{
		`App\Models\User`:            {`App\Models\User`},
		`\App\Models\User as Member`: {`App\Models\User`},
		`App\A, App\B as C`:          {`App\A`, `App\B`},
		"App\\Contracts\\{\n    Arrayable,\n    Jsonable as Json,\n}": {`App\Contracts\Arrayable`, `App\Contracts\Jsonable`},
		`App\{Models\User, function helper, const LIMIT}`:             {`App\Models\User`},
		`function App\Support\helper`:                                 nil,
		`const App\LIMIT`:                                             nil,
	}
	for body, want := range cases {
		assert.Equal(t, want, phpImportedClasses(body, map[string]string{}), body)
	}

	aliases := map[string]string{}
	phpImportedClasses(`App\Contracts\{Arrayable, Jsonable as Json}`, aliases)
	assert.Equal(t, []string{`App\Contracts\Jsonable`}, phpCandidates("Json", `App\Models`, aliases))
	assert.Equal(t, []string{`App\Contracts\Arrayable\Nested`}, phpCandidates(`Arrayable\Nested`, `App\Models`, aliases))
	assert.Equal(t, []string{`App\Models\Model`, "Model"}, phpCandidates("Model", `App\Models`, aliases))
	assert.Equal(t, []string{`Other\Model`}, phpCandidates(`\Other\Model`, `App\Models`, aliases))
}

// Go imports come from go/parser: every formatting of the import
// declarations, and never a string literal after them.
func TestGoImportPaths(t *testing.T) {
	cases := map[string][]string{
		"package main\n\nimport (\"fmt\"; \"m/a\")\n\nvar names = []string{\n\t\"m/b\",\n}\n":                                                   {"fmt", "m/a"},
		"package main\nimport \"m/a\"\nimport x \"m/b\"\n":                                                                                      {"m/a", "m/b"},
		"package main\n\nimport (\n\t// a comment\n\t. \"m/dot\"\n\t_ \"m/blank\"\n\talias \"m/alias\" // trailing\n\t/* block */ \"m/c\"\n)\n": {"m/dot", "m/blank", "m/alias", "m/c"},
		"package main\n\nimport `m/raw`\n":                                                                                                      {"m/raw"},
		"package main\n\nimport \"m/a\"\n\nfunc f() { _ = \"import (\\\"m/x\\\")\" }\n":                                                         {"m/a"},
		"package main\n\nfunc main() {}\n":                                                                                                      nil,
		"// import \"m/comment\"\npackage main\n":                                                                                               nil,
		"package main\n\nimport (\n\t\"m/a\"\n\t\"m/b\n)\n":                                                                                     {"m/a"},
	}
	for src, want := range cases {
		assert.Equal(t, want, goImportPaths([]byte(src)), src)
	}
}

// JS and TS specifiers come from the tree-sitter tree: declarations,
// import() and require() with a string literal, in source order; nothing
// from comments, strings, template literals or regular expressions.
func TestJSImportSpecifiers(t *testing.T) {
	ts := "import type { A } from './a';\n" +
		"import B, { C } from \"./b\";\n" +
		"import './side';\n" +
		"import fs = require('./legacy');\n" +
		"export { D } from './d';\n" +
		"export * as E from './e';\n" +
		"const lazy = () => import('./lazy');\n" +
		"const tpl = import(`./tpl/${name}`);\n" +
		"const dyn = require(name);\n" +
		"const s = \"import x from './str'\"; // import y from './comment'\n" +
		"/* require('./block') */ const r = /from '.\\/regex'/;\n" +
		"const t = `export * from './template'`;\n"
	got, err := jsImportSpecifiers(context.Background(), []byte(ts), "main.ts")
	require.NoError(t, err)
	assert.Equal(t, []string{"./a", "./b", "./side", "./legacy", "./d", "./e", "./lazy"}, got)

	jsx := "import React from 'react';\nconst Button = require('./button');\n" +
		"export default function App() { return <div title=\"import('./attr')\">{require('./inline')}</div>; }\n"
	got, err = jsImportSpecifiers(context.Background(), []byte(jsx), "app.jsx")
	require.NoError(t, err)
	assert.Equal(t, []string{"react", "./button", "./inline"}, got)
}

func TestStripJSONC(t *testing.T) {
	in := "{\n  // a comment\n  \"a\": \"http://x/*y*/\", /* block */\n  \"b\": [1, 2,],\n  \"c\": {\"d\": 1, // last\n  },\n}\n"
	assert.JSONEq(t, `{"a": "http://x/*y*/", "b": [1, 2], "c": {"d": 1}}`, string(stripJSONC([]byte(in))))
}

func TestTSConfigMatchPrefersExactThenLongestPrefix(t *testing.T) {
	cfg := &tsConfig{
		pathsDir: "/p",
		paths: map[string][]string{
			"@/*":         {"src/*"},
			"@/lib/*":     {"lib/*", "vendor/lib/*"},
			"config":      {"src/config.ts"},
			"*.generated": {"gen/*"},
		},
	}
	assert.Equal(t, []string{"/p/lib/x", "/p/vendor/lib/x"}, cfg.match("@/lib/x"))
	assert.Equal(t, []string{"/p/src/app"}, cfg.match("@/app"))
	assert.Equal(t, []string{"/p/src/config.ts"}, cfg.match("config"))
	assert.Empty(t, cfg.match("react"))

	cfg.baseURL = "/base"
	assert.Equal(t, []string{"/base/react"}, cfg.match("react"))
}

func TestFollowScopePathHelpers(t *testing.T) {
	assert.True(t, pathInside("/r/a/b.go", "/r"))
	assert.True(t, pathInside("/r", "/r"))
	assert.False(t, pathInside("/rx/b.go", "/r"))
	assert.False(t, pathInside("/b.go", "/r"))
	assert.True(t, pathInside("/r/..b/c.go", "/r"))

	assert.Equal(t, "/r/go", commonDir("/r/go/shop/cmd/shop", "/r/go/kit/log/log.go"))
	assert.Equal(t, "/r/app", commonDir("/r/app", "/r/app/sub/x.py"))

	assert.Equal(t, []string{"/r", "/r/a", "/r/a/b"}, dirsBetween("/r", "/r/a/b"))
	assert.Equal(t, []string{"/r"}, dirsBetween("/r", "/r"))
}
