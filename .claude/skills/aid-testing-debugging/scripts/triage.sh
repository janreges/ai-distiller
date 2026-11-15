#!/bin/sh
# Shows which layer loses or changes a construct: builds aid from this checkout
# into a temp directory and prints three views of one input.
#
#   1. default text output
#   2. text output with everything included
#   3. --format jsonl with everything included: one line per IR node
#
# With every include flag on, the stripper has nothing to remove and does not
# run, so view 3 is the parser's IR and view 2 is that IR through the text
# formatter.
#
# Usage: sh triage.sh <lang> <file>     aid picks the processor from the file
#                                       extension; a note says if it is not <lang>
#        sh triage.sh <lang> -          reads the input from stdin, runs aid --lang <lang>
# <lang> is a language name as in testdata/ (python, typescript, cpp, ...).
set -eu

if [ $# -ne 2 ]; then
  echo "usage: sh $0 <lang> <file|->" >&2
  exit 2
fi
lang=$1
file=$2
if [ "$file" != - ] && [ ! -f "$file" ]; then
  echo "triage.sh: $file is not a file" >&2
  exit 2
fi

root=$(cd "$(dirname "$0")/../../../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

(cd "$root" && CGO_ENABLED=1 CGO_CFLAGS="-O2 -w" go build -o "$tmp/aid" ./cmd/aid)
if [ "$file" = - ]; then
  cat > "$tmp/input"
fi

all="--private=1 --protected=1 --internal=1 --implementation=1 --comments=1"

# run FLAGS...: one aid run on the input, stdout only.
run() {
  if [ "$file" = - ]; then
    "$tmp/aid" --lang "$lang" --summary-type=off "$@" < "$tmp/input"
  else
    "$tmp/aid" "$file" --stdout --summary-type=off "$@" < /dev/null
  fi
}

# show TITLE FLAGS...: prints one labelled section; a failing run does not stop the script.
show() {
  title=$1
  shift
  printf '=== %s ===\n' "$title"
  code=0
  run "$@" > "$tmp/out" || code=$?
  if [ -s "$tmp/out" ]; then cat "$tmp/out"; else echo "(no output)"; fi
  if [ "$code" -ne 0 ]; then echo "(aid exited with $code)"; fi
  echo
}

if [ "$file" != - ]; then
  detected=$(run --format jsonl 2>/dev/null | sed -n '1s/.*"language":"\([^"]*\)".*/\1/p') || true
  if [ "$detected" != "$lang" ]; then
    echo "note: aid processes $file as '${detected:-nothing}', not '$lang'" >&2
  fi
fi

show "1. default text"
# shellcheck disable=SC2086 # $all is a list of flags
show "2. text, everything included: $all" $all
# shellcheck disable=SC2086
show "3. IR as jsonl, everything included (the stripper does not run)" --format jsonl $all
