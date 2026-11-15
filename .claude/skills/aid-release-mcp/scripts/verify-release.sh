#!/bin/sh
# Checks a published release from the outside. Read-only: it runs only
# `gh release view`, `gh release download --output` (into a temp directory)
# and `npm view`.
#
#   1. The GitHub Release for the tag exists.
#   2. It has the five archives the installers and postinstall.js download
#      (aid-linux-amd64, aid-linux-arm64, aid-darwin-amd64, aid-darwin-arm64 as
#      .tar.gz, aid-windows-amd64 as .zip, each named ...-v<version>.<ext>)
#      and checksums.txt.
#   3. checksums.txt has a line for each archive. When GitHub reports the
#      asset's sha256, the line must carry the same hash: postinstall.js
#      refuses to install on a missing line and on a mismatch.
#   4. npm has @janreges/ai-distiller-mcp at <version>.
#
# Prints one line per check, starting with "ok" or "FAIL".
# Exit 0 when every check passes, 1 when one fails, 2 on wrong usage or when
# gh or npm is missing.
#
# Usage: sh verify-release.sh vX.Y.Z     e.g. sh verify-release.sh v1.4.0
set -u

repo=janreges/ai-distiller
pkg=@janreges/ai-distiller-mcp

if [ $# -ne 1 ]; then
  echo "usage: sh $0 vX.Y.Z" >&2
  exit 2
fi
tag=$1
case $tag in
  v[0-9]*) ;;
  *) echo "verify-release.sh: expected a tag like v1.4.0, got '$tag'" >&2; exit 2;;
esac
version=${tag#v}
for cmd in gh npm; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "verify-release.sh: $cmd is not installed" >&2; exit 2; }
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
failed=0
ok() { echo "ok   $*"; }
fail() { echo "FAIL $*"; failed=1; }

archives="aid-linux-amd64-v$version.tar.gz aid-linux-arm64-v$version.tar.gz aid-darwin-amd64-v$version.tar.gz aid-darwin-arm64-v$version.tar.gz aid-windows-amd64-v$version.zip"

# One line per asset: name<TAB>sha256 hex (empty when GitHub reports no digest).
if gh release view "$tag" --repo "$repo" --json assets \
    --jq '.assets[] | [.name, ((.digest // "") | sub("^sha256:"; ""))] | @tsv' \
    > "$tmp/assets" 2> "$tmp/err"; then
  ok "release $tag exists in $repo"
  for a in $archives checksums.txt; do
    if cut -f1 "$tmp/assets" | grep -qxF "$a"; then
      ok "asset $a"
    else
      fail "asset $a is missing"
    fi
  done
  if gh release download "$tag" --repo "$repo" --pattern checksums.txt \
      --output "$tmp/checksums.txt" 2> "$tmp/err"; then
    for a in $archives; do
      # sha256sum writes "<hash>  <name>", or "<hash> *<name>" in binary mode.
      listed=$(awk -v n="$a" '{ f = $2; sub(/^\*/, "", f) } f == n { print tolower($1); exit }' "$tmp/checksums.txt")
      actual=$(awk -F '\t' -v n="$a" '$1 == n { print tolower($2); exit }' "$tmp/assets")
      if [ -z "$listed" ]; then
        fail "checksums.txt has no line for $a"
      elif [ -z "$actual" ]; then
        ok "checksums.txt lists $a (GitHub reports no sha256 to compare)"
      elif [ "$listed" = "$actual" ]; then
        ok "checksums.txt lists $a with the asset's sha256"
      else
        fail "checksums.txt lists $a as $listed, GitHub reports $actual"
      fi
    done
  else
    fail "checksums.txt could not be downloaded: $(head -n 1 "$tmp/err")"
  fi
elif grep -q "release not found" "$tmp/err"; then
  fail "release not found: $tag in $repo"
else
  fail "gh release view $tag: $(head -n 1 "$tmp/err")"
fi

got=$(npm view "$pkg@$version" version 2> "$tmp/err")
if [ "$got" = "$version" ]; then
  ok "npm $pkg@$version"
else
  fail "npm $pkg@$version: $(sed -n 's/^npm error //p' "$tmp/err" | grep -v '^code ' | head -n 1)"
fi

exit $failed
