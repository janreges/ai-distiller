#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
fx=scripts/tests/.anchor-fixture-$$
tmp=$(mktemp -d); trap 'rm -rf "$tmp" "$fx"' EXIT
mk() { mkdir -p "$tmp/$1/demo"; printf '%s\n' "---" "name: demo" "description: Demo skill. Use when testing." "---" "$2" > "$tmp/$1/demo/SKILL.md"; }
expect() { # name, expected exit, expected output fragment
  out=$(CLAUDE_MD_MAX_LINES=100000 sh scripts/check-skill-anchors.sh "$tmp/$1" 2>&1) && code=0 || code=$?
  [ "$code" = "$2" ] || { echo "FAIL $1: exit $code, want $2"; echo "$out"; exit 1; }
  [ -z "$3" ] || echo "$out" | grep -q "$3" || { echo "FAIL $1: output lacks '$3'"; echo "$out"; exit 1; }
}
mk good 'See `internal/stripper/stripper.go#Stripper` and `cmd/aid/main.go`. Placeholder `testdata/<lang>/x` is fine.'
expect good 0 ""
mk missing-path 'See `internal/nope/gone.go`.'
expect missing-path 1 "internal/nope/gone.go"
mk missing-symbol 'See `internal/stripper/stripper.go#NoSuchThing`.'
expect missing-symbol 1 "NoSuchThing"
mk line-anchor 'See `internal/stripper/stripper.go:42`.'
expect line-anchor 1 "use path#Symbol"
mkdir -p "$tmp/bad-name/demo"; printf '%s\n' "---" "name: other" "description: x" "---" > "$tmp/bad-name/demo/SKILL.md"
expect bad-name 1 "name"
# non-Go anchors: whole word in the file; markdown fragments only check the path
mk word-ok 'See `scripts/check-cgo.sh#cgo` and `docs/user/COMMAND-LINE-OPTIONS.md#anything`.'
expect word-ok 0 ""
mk word-missing 'See `scripts/check-cgo.sh#NoSuchWordAnywhere`.'
expect word-missing 1 "NoSuchWordAnywhere"
# Type.Method and directories
mk method 'See `internal/stripper/stripper.go#Stripper.Visit` and `internal/stripper/`.'
expect method 0 ""
# command spans: first word is checked when it has a known prefix
mk cmd-ok 'Run `scripts/check-skill-anchors.sh .claude/skills` or `go test ./...`.'
expect cmd-ok 0 ""
mk cmd-missing 'Run `scripts/no-such-script.sh arg`.'
expect cmd-missing 1 "scripts/no-such-script.sh"
# fenced blocks are not scanned
mk fenced 'Prose only.'
printf '%s\n' '```' '`internal/nope/gone.go`' '```' >> "$tmp/fenced/demo/SKILL.md"
expect fenced 0 ""
# a valid Go anchor must not hide later problems or the frontmatter check
mkdir -p "$tmp/clobber/demo"
printf '%s\n' "---" "name: WRONG" "description: x" "---" 'See `internal/stripper/stripper.go#Stripper` then `internal/nope/gone.go`.' > "$tmp/clobber/demo/SKILL.md"
expect clobber 1 "clobber/demo/SKILL.md: path does not exist: internal/nope/gone.go"
expect clobber 1 "SKILL.md: frontmatter name 'WRONG'"
# links to other skills (.claude/...) are checked like code paths
mk claude-ok 'See `.claude/skills/README.md`.'
expect claude-ok 0 ""
mk claude-missing 'See `.claude/skills/no-such-skill/SKILL.md`.'
expect claude-missing 1 "path does not exist: .claude/skills/no-such-skill/SKILL.md"
# Go symbols must be real declarations, not strings, struct fields or comments
mkdir -p "$fx"
printf '%s\n' 'package x' 'var s = "Phantom"' 'type T struct {' '	Field string' '}' 'var (' '	Real = 1' '	A, Multi = 1, 2' ')' 'var a, Listed = 1, 2' 'type (' '	Grp struct{}' '	Grp2 int' ')' 'var (' '	V = map[string]int{' '		Key: 1,' '	}' ')' '/*' '	Prose words' '*/' > "$fx/x.go"
# the fixture exists on disk but git does not track it: a clean checkout lacks it
mk untracked-file "See \`$fx/x.go#Real\`."
expect untracked-file 1 "path is not tracked by git: $fx/x.go"
mk untracked-dir "See \`$fx/\`."
expect untracked-dir 1 "path is not tracked by git: $fx/"
# the cases below use the fixture, so it counts as tracked for them
export SKILL_ANCHORS_FIXTURE_DIR="$fx"
mk go-real "See \`$fx/x.go#Real\`, \`$fx/x.go#Multi\`, \`$fx/x.go#Listed\`, \`$fx/x.go#T\`."
expect go-real 0 ""
mk go-group "See \`$fx/x.go#Grp\`, \`$fx/x.go#Grp2\`, \`$fx/x.go#V\`."
expect go-group 0 ""
mk go-nested "See \`$fx/x.go#Key\`."
expect go-nested 1 "Key"
mk go-string "See \`$fx/x.go#Phantom\`."
expect go-string 1 "Phantom"
mk go-field "See \`$fx/x.go#Field\`."
expect go-field 1 "Field"
mk go-prose "See \`$fx/x.go#Prose\`."
expect go-prose 1 "Prose"
# size limits
mkdir -p "$tmp/long-desc/demo"
{ printf '%s' "---
name: demo
description: "; i=0; while [ $i -lt 1100 ]; do printf x; i=$((i+1)); done; printf '\n---\n'; } > "$tmp/long-desc/demo/SKILL.md"
expect long-desc 1 "description is longer than 1024"
mkdir -p "$tmp/long-file/demo"
{ printf '%s\n' "---" "name: demo" "description: ok" "---"; i=0; while [ $i -lt 310 ]; do echo line; i=$((i+1)); done; } > "$tmp/long-file/demo/SKILL.md"
expect long-file 1 "limit is 300"
echo ok
