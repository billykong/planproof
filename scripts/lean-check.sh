#!/usr/bin/env bash
# Checks on the Lean project beyond "it builds" (PRD L-02, L-03):
#  1. no escape hatches anywhere in lean/Planproof: sorry, admit, a new
#     axiom, native_decide, implemented_by, extern, unsafe. (implemented_by
#     or extern would let the executable run different code from the code
#     the theorems are about.)
#  2. lean/Planproof/Spec is at most 600 non-blank, non-comment lines.
#  3. each theorem in lean/theorems.txt depends only on Lean's standard
#     axioms (propext, Classical.choice, Quot.sound).
# Run after `lake build`.
set -euo pipefail
cd "$(dirname "$0")/../lean"
fail=0

if grep -rnwE 'sorry|admit|axiom|native_decide|implemented_by|extern|unsafe' Planproof Planproof.lean Main.lean \
    | grep -vE '^\S+:[0-9]+:\s*(--|/-)'; then
  echo "lean-check: escape hatch above (sorry, admit, axiom, native_decide, implemented_by, extern or unsafe)"; fail=1
fi

if [ -d Planproof/Spec ]; then
  lines=$(cat Planproof/Spec/*.lean | awk '
    /^[[:space:]]*\/-/ { if ($0 !~ /-\/[[:space:]]*$/) incomment=1; next }
    incomment { if ($0 ~ /-\//) incomment=0; next }
    /^[[:space:]]*--/ || /^[[:space:]]*$/ { next }
    { n++ } END { print n+0 }')
  echo "lean-check: Spec is $lines non-blank, non-comment lines (budget 600)"
  [ "$lines" -le 600 ] || { echo "lean-check: Spec over budget"; fail=1; }
fi

theorems=$(grep -v '^#' theorems.txt 2>/dev/null | grep . || true)
if [ -n "$theorems" ]; then
  check=$(mktemp --suffix=.lean)
  { echo "import Planproof"; echo "$theorems" | sed 's/^/#print axioms /'; } > "$check"
  out=$(lake env lean "$check" 2>&1); rm -f "$check"
  echo "$out"
  if echo "$out" | grep -qE 'error|sorryAx'; then echo "lean-check: axiom check failed"; fail=1; fi
  extra=$(echo "$out" | grep -oE '\[[^]]*\]' | tr -d '[]' | tr ',' '\n' | sed 's/ //g' | grep . | grep -vxE 'propext|Classical.choice|Quot.sound' || true)
  if [ -n "$extra" ]; then echo "lean-check: non-standard axioms: $extra"; fail=1; fi
fi

exit $fail
