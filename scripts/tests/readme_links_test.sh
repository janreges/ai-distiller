#!/bin/sh
set -eu
cd "$(dirname "$0")/../.."
# The loop below runs in a pipeline subshell, so it reports broken links through
# a temp file outside the repository; a run that is cut short leaves nothing
# behind that a later run would read.
bad=$(mktemp); trap 'rm -f "$bad"' EXIT
for f in README.md CONTRIBUTING.md; do
  [ -f "$f" ] || { echo "$f is missing"; echo 1 > "$bad"; continue; }
  grep -oE '\]\([^)#:]+(#[^)]*)?\)' "$f" | sed -E 's/^\]\(//; s/(#.*)?\)$//' | sort -u | while read -r target; do
    [ -e "$target" ] || { echo "$f: broken link to $target"; echo 1 > "$bad"; }
  done
done
[ ! -s "$bad" ] || exit 1
exit 0
