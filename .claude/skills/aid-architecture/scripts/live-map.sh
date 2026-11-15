#!/bin/sh
# Prints the API of one directory of this checkout as aid sees it: builds aid
# from the working tree into a temp directory and runs
#
#   aid <dir> --stdout --summary-type=off --exclude '*_test.go' --internal=1 --private=1
#
# The map is produced by the code it describes, so it cannot go stale.
# --internal=1 shows Go's unexported names; test files are left out; bodies are
# not printed (--implementation stays 0). Nothing is written: --stdout keeps aid
# from creating .aid/ in the project.
#
# Usage: sh live-map.sh <dir>     e.g. sh live-map.sh internal/stripper
set -eu

if [ $# -ne 1 ]; then
  echo "usage: sh $0 <dir>" >&2
  exit 2
fi
dir=$1
if [ ! -d "$dir" ]; then
  echo "live-map.sh: $dir is not a directory" >&2
  exit 2
fi

root=$(cd "$(dirname "$0")/../../../.." && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

(cd "$root" && CGO_ENABLED=1 CGO_CFLAGS="-O2 -w" go build -o "$tmp/aid" ./cmd/aid)

# The path argument wins over stdin; </dev/null is the project's rule for
# scripted runs on a path.
"$tmp/aid" "$dir" --stdout --summary-type=off --exclude '*_test.go' --internal=1 --private=1 < /dev/null
