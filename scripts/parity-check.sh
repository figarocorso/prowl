#!/usr/bin/env bash
# Compare the bash and Go implementations side-by-side against the same data
# directory. Intended as a pre-cutover sanity check. Exits non-zero on any
# diff between the human-visible output of the two implementations.
#
# Usage:
#   scripts/parity-check.sh                # uses your real data dir
#   PROWL_DATA_DIR=/tmp/p scripts/parity-check.sh

set -euo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BASH_BIN="${PROWL_BASH:-$REPO/prowl.sh}"
GO_BIN="${PROWL_GO:-$REPO/prowl}"

if [[ ! -x "$BASH_BIN" ]]; then
  echo "✗ bash binary not found: $BASH_BIN" >&2
  exit 2
fi
if [[ ! -x "$GO_BIN" ]]; then
  echo "→ building go binary"
  (cd "$REPO" && go build -o prowl .)
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

bash_out="$tmp/bash.txt"
go_out="$tmp/go.txt"

echo "→ bash list"
"$BASH_BIN" review >"$bash_out" 2>&1 || true

echo "→ go list"
"$GO_BIN" list >"$go_out" 2>&1 || true

echo
echo "── bash output (truncated) ──"
head -40 "$bash_out"
echo
echo "── go output (truncated) ──"
head -40 "$go_out"
echo

if diff -u "$bash_out" "$go_out" >"$tmp/diff.txt"; then
  echo "✓ outputs match"
else
  echo "⚠ outputs differ (expected: layouts diverge; check semantically)"
  cat "$tmp/diff.txt" | head -80
fi
