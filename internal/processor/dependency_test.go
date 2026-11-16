package processor

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
