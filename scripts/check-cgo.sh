#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
# Matches CGO_ENABLED=0, CGO_ENABLED: '0', CGO_ENABLED := 0, ENV CGO_ENABLED 0 (but not =1 or =10).
pattern='CGO_ENABLED([[:space:]]*:?=|:|[[:space:]]+)[[:space:]]*["'"'"']?0["'"'"']?([^0-9A-Za-z_]|$)'
rc=0
git grep -nE "$pattern" -- Makefile Dockerfile scripts mcp-npm/scripts .github/workflows ':!scripts/check-cgo.sh' || rc=$?
case $rc in
  0) echo "cgo must stay enabled: aid does not compile without it" >&2; exit 1 ;;
  1) echo "ok" ;;
  *) echo "check-cgo: git grep failed (exit $rc)" >&2; exit 2 ;;
esac
