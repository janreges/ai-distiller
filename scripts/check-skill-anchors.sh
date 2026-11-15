#!/bin/sh
# Checks that every path and symbol the skills (and CLAUDE.md) name is still true.
# A path counts only when git tracks it (a directory: when git tracks a file
# under it) and it exists, so a local run judges what a clean checkout in CI sees.
# Usage: sh scripts/check-skill-anchors.sh [skills-dir]   (default .claude/skills)
# Env:   CLAUDE_MD_MAX_LINES (default 150) limits CLAUDE.md when the default dir is used.
#        SKILL_ANCHORS_FIXTURE_DIR names one repo-relative directory whose files
#        count as tracked; only scripts/tests/check_skill_anchors_test.sh sets it,
#        for its untracked Go fixtures.
set -u
cd "$(dirname "$0")/.."
root=$(pwd)

default_dir=0
if [ $# -ge 1 ]; then dir=$1; else dir=.claude/skills; default_dir=1; fi
max_md=${CLAUDE_MD_MAX_LINES:-150}
fixture_dir=${SKILL_ANCHORS_FIXTURE_DIR:-}
fixture_dir=${fixture_dir%/}
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
errs="$tmp/errs"; : > "$errs"
if ! git -c core.quotepath=off ls-files > "$tmp/tracked"; then
  echo "check-skill-anchors: git ls-files failed; run it in a git checkout" >&2; exit 2
fi

problem() { printf '%s: %s\n' "$1" "$2" >> "$errs"; }

# tracked PATH -> 0 when git tracks PATH, or a file under it when PATH is a directory.
tracked() {
  tr_p=${1%/}
  if [ -n "$fixture_dir" ]; then
    case $tr_p in "$fixture_dir"|"$fixture_dir"/*) return 0;; esac
  fi
  awk -v p="$tr_p" '$0 == p || index($0, p "/") == 1 { f = 1; exit } END { exit !f }' "$tmp/tracked"
}

# check_go FILE SYMBOL -> 0 when FILE declares SYMBOL (or Type.Method).
# POSIX sh has no `local`, so this function uses names no other function uses.
check_go() {
  go_file=$1; go_sym=$2
  case $go_sym in
    *.*)
      go_t=${go_sym%%.*}; go_m=${go_sym#*.}
      grep -Eq "^func \([A-Za-z0-9_]* ?\*?$go_t(\[[^]]*\])?\) $go_m([^A-Za-z0-9_]|\$)" "$go_file"
      return $?;;
  esac
  grep -Eq "^func $go_sym([^A-Za-z0-9_]|\$)" "$go_file" && return 0
  grep -Eq "^func \([^)]*\) $go_sym([^A-Za-z0-9_]|\$)" "$go_file" && return 0
  grep -Eq "^type $go_sym([^A-Za-z0-9_]|\$)" "$go_file" && return 0
  grep -Eq "^[[:space:]]+type $go_sym([^A-Za-z0-9_]|\$)" "$go_file" && return 0
  go_names="([A-Za-z0-9_]+[[:space:]]*,[[:space:]]*)*$go_sym([^A-Za-z0-9_]|\$)"
  grep -Eq "^(var|const)[[:space:]]+$go_names" "$go_file" && return 0
  # inside a var/const/type ( ... ) block, at the indentation of its first entry
  awk -v re="^[ \t]+$go_names" '
    /^(var|const|type)[ \t]*\(/ { blk = 1; ind = ""; next }
    blk && /^\)/ { blk = 0; next }
    blk && /^[ \t]*($|\/\/)/ { next }
    blk {
      match($0, /^[ \t]*/); cur = substr($0, 1, RLENGTH)
      if (ind == "") ind = cur
      if (cur == ind && $0 ~ re) found = 1
    }
    END { exit !found }' "$go_file" && return 0
  return 1
}

# check_span FILE SPAN
check_span() {
  file=$1; span=$2
  w=${span%%[[:space:]]*}
  case $w in *'<'*|*'>'*|*'*'*) return 0;; esac
  if printf '%s\n' "$w" | grep -Eq '\.[a-z]+:[0-9]+'; then
    problem "$file" "\`$w\`: use path#Symbol instead of a line number"; return 0
  fi
  case $w in
    cmd/*|internal/*|scripts/*|mcp-npm/*|testdata/*|docs/*|.github/*|.claude/*) ;;
    *) return 0;;
  esac
  path=${w%%#*}
  sym=
  case $w in *'#'*) sym=${w#*#};; esac
  if [ ! -e "$path" ]; then problem "$file" "path does not exist: $path"; return 0; fi
  if ! tracked "$path"; then problem "$file" "path is not tracked by git: $path"; return 0; fi
  [ -n "$sym" ] || return 0
  [ -d "$path" ] && return 0
  case $path in
    *.md) return 0;;
    *.go) check_go "$path" "$sym" || problem "$file" "$path has no declaration of $sym";;
    *) w2=${sym##*.}
       grep -qw -- "$w2" "$path" || problem "$file" "$path does not contain $sym";;
  esac
}

# check_md FILE: scan inline backticks outside fenced blocks.
check_md() {
  f=$1
  awk '
    /^[[:space:]]*(```|~~~)/ { fence = !fence; next }
    fence { next }
    {
      line = $0
      while (match(line, /`[^`]+`/)) {
        print substr(line, RSTART + 1, RLENGTH - 2)
        line = substr(line, RSTART + RLENGTH)
      }
    }' "$f" > "$tmp/spans"
  while IFS= read -r span; do
    check_span "$f" "$span"
  done < "$tmp/spans"
}

check_skill_md() {
  f=$1
  skill=$(basename "$(dirname "$f")")
  if [ "$(sed -n 1p "$f")" != "---" ]; then problem "$f" "frontmatter must begin with ---"; return; fi
  fm=$(awk 'NR==1{next} /^---[[:space:]]*$/{exit} {print}' "$f")
  name=$(printf '%s\n' "$fm" | sed -n 's/^name:[[:space:]]*//p' | head -n 1 | sed 's/[[:space:]]*$//')
  [ "$name" = "$skill" ] || problem "$f" "frontmatter name '$name' must equal directory name '$skill'"
  desc=$(printf '%s\n' "$fm" | awk '
    /^description:/ { sub(/^description:[[:space:]]*/, ""); if ($0 ~ /^[>|][-+]?[[:space:]]*$/) $0 = ""; d = $0; on = 1; next }
    on && /^[[:space:]]+/ { sub(/^[[:space:]]+/, ""); d = d (d == "" ? "" : " ") $0; next }
    { on = 0 }
    END { printf "%s", d }')
  if [ -z "$desc" ]; then problem "$f" "frontmatter has no description"
  elif [ "$(printf '%s' "$desc" | wc -c)" -gt 1024 ]; then problem "$f" "description is longer than 1024 characters"; fi
  n=$(wc -l < "$f")
  [ "$n" -le 300 ] || problem "$f" "SKILL.md has $n lines, limit is 300"
}

find "$dir" -type f -name '*.md' 2>/dev/null | sort > "$tmp/files"
while IFS= read -r f; do
  check_md "$f"
  [ "$(basename "$f")" = "SKILL.md" ] && check_skill_md "$f"
done < "$tmp/files"

if [ "$default_dir" = 1 ] && [ -f CLAUDE.md ]; then
  check_md CLAUDE.md
  n=$(wc -l < CLAUDE.md)
  [ "$n" -le "$max_md" ] || problem CLAUDE.md "has $n lines, limit is $max_md"
fi

if [ -s "$errs" ]; then cat "$errs"; exit 1; fi
exit 0
