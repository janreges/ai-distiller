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
