#!/usr/bin/env bash
# kit-parity.sh — how far the UI kit copied from Solco has drifted.
#
# Solco Git started from a copy of Solco's UI kit and Git/Files presentation
# (2026-11-07 decision: "UI kit copied first"). Both repositories keep changing,
# so this lists every file name the two internal/nativeui directories share and
# says whether the two copies are still identical, differ (with the number of
# changed lines), or exist only here / only in Solco.
#
# Local only: it needs the sibling checkout (../solco), which CI does not have.
#
#   scripts/kit-parity.sh             # table of differing files, then the summary
#   scripts/kit-parity.sh --all       # also list the identical files
#   scripts/kit-parity.sh --diff X.go # the full diff of one shared file (Solco → here)
#   SOLCO_DIR=~/src/solco scripts/kit-parity.sh   # when the checkout is elsewhere
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
solco="${SOLCO_DIR:-$here/../solco}"
mine="$here/internal/nativeui"
theirs="$solco/internal/nativeui"

if [ ! -d "$theirs" ]; then
  echo "kit-parity: no Solco checkout at $solco (set SOLCO_DIR)" >&2
  exit 2
fi

case "${1:-}" in
  --diff)
    file="${2:?usage: kit-parity.sh --diff <file.go>}"
    [ -f "$theirs/$file" ] || { echo "kit-parity: $file is not in Solco's nativeui" >&2; exit 2; }
    [ -f "$mine/$file" ] || { echo "kit-parity: $file is not in Solco Git's nativeui" >&2; exit 2; }
    diff -u "$theirs/$file" "$mine/$file" || true
    exit 0
    ;;
  ""|--all) ;;
  *)
    echo "usage: kit-parity.sh [--all | --diff <file.go>]" >&2
    exit 2
    ;;
esac
show_same=0
[ "${1:-}" = "--all" ] && show_same=1

same=0
differ=0
only_here=0
printf '%-34s %s\n' "FILE" "STATE"
for path in "$mine"/*.go; do
  name="$(basename "$path")"
  case "$name" in *_test.go) continue ;; esac
  if [ ! -f "$theirs/$name" ]; then
    only_here=$((only_here + 1))
    continue
  fi
  if cmp -s "$path" "$theirs/$name"; then
    same=$((same + 1))
    [ "$show_same" = 1 ] && printf '%-34s identical\n' "$name"
    continue
  fi
  differ=$((differ + 1))
  changed="$(diff "$theirs/$name" "$path" | rg -c '^[<>]' || true)"
  printf '%-34s differs (%s changed lines)\n' "$name" "${changed:-0}"
done

echo
echo "shared and identical: $same"
echo "shared and differing: $differ"
echo "only in Solco Git:    $only_here  (the window, tabs, welcome, Quick Launch and the Files/Git extras)"
