#!/usr/bin/env bash
# The kernel's budgets, checked in CI (K-02, K-03, SEC-02):
#   - at most MAX_LINES non-blank, non-comment lines of non-test Go;
#   - imports only from the allowlist below: no os, net, time, math/rand,
#     crypto/rand, syscall, unsafe, and nothing outside the standard library;
#   - no goroutines, channels or select, and no printing or scanning.
# Changing the allowlist or the budget is a change to the trusted base:
# review it like kernel code.
set -euo pipefail
cd "$(dirname "$0")/.."

MAX_LINES=1800
ALLOWED="bytes cmp crypto/sha256 encoding/hex encoding/json errors fmt io maps math reflect slices sort strconv strings unicode unicode/utf8"

fail=0
files=$(ls kernel/*.go | grep -v '_test\.go$')

lines=$(cat $files | grep -cvE '^[[:space:]]*(//.*)?$' || true)
echo "kernel: $lines non-blank, non-comment lines (budget $MAX_LINES)"
if [ "$lines" -gt "$MAX_LINES" ]; then
  echo "FAIL: kernel is over its size budget"; fail=1
fi

for imp in $(go list -f '{{join .Imports " "}}' ./kernel); do
  case " $ALLOWED " in
    *" $imp "*) ;;
    *) echo "FAIL: kernel imports $imp, which is not on the allowlist"; fail=1 ;;
  esac
done

# Line comments are stripped first, so prose in comments doesn't match.
if sed 's|//.*||' $files | grep -nE '(^|[^[:alnum:]_.])go[[:space:]]+[[:alnum:]_(]|\bchan\b|(^|[^[:alnum:]_.])select[[:space:]]*\{|fmt\.(F|S)?(Print|Scan)'; then
  echo "FAIL: goroutines, channels, select, printing or scanning in the kernel"; fail=1
fi

exit $fail
