#!/bin/sh
# Prints one source file in every output format of a path run: builds aid from
# this checkout into a temp directory and runs
#
#   aid <file> --stdout --summary-type=off --format <fmt> [aid flags...]
#
# for text, md, jsonl, json-structured and xml, each under a "=== <fmt> ===" heading.
# Extra arguments after the file are passed to every run, for example
# --private=1 --implementation=1. Nothing is written: --stdout keeps aid from
# creating .aid/ in the project.
#
# Usage: sh format-matrix.sh <file> [aid flags...]
set -eu

if [ $# -lt 1 ]; then
  echo "usage: sh $0 <file> [aid flags...]" >&2
  exit 2
fi
file=$1
shift
if [ ! -f "$file" ]; then
  echo "format-matrix.sh: $file is not a file" >&2
  exit 2
fi

root=$(cd "$(dirname "$0")/../../../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

(cd "$root" && CGO_ENABLED=1 CGO_CFLAGS=-w go build -o "$tmp/aid" ./cmd/aid)

for fmt in text md jsonl json-structured xml; do
  printf '=== %s ===\n' "$fmt"
  code=0
  # The path argument wins over stdin; </dev/null is the project's rule for
  # scripted runs on a path.
  "$tmp/aid" "$file" --stdout --summary-type=off --format "$fmt" "$@" < /dev/null > "$tmp/out" || code=$?
  if [ -s "$tmp/out" ]; then cat "$tmp/out"; else echo "(no output)"; fi
  if [ "$code" -ne 0 ]; then echo "(aid exited with $code)"; fi
  echo
done
