#!/usr/bin/env bash
# Fail if total Go coverage is below threshold.
# Usage: ./scripts/coverage-check.sh [threshold]
# Env: COVERAGE_THRESHOLD (default 55), COVERAGE_FILE (default coverage.out)

set -euo pipefail

THRESHOLD="${1:-${COVERAGE_THRESHOLD:-55}}"
COVERAGE_FILE="${COVERAGE_FILE:-coverage.out}"

if [ ! -f "$COVERAGE_FILE" ]; then
  echo "coverage: generating $COVERAGE_FILE"
  go test -coverprofile="$COVERAGE_FILE" ./... >/dev/null
fi

total=$(go tool cover -func="$COVERAGE_FILE" | awk '/^total:/ {gsub("%","",$3); print $3}')

if [ -z "$total" ]; then
  echo "coverage: could not parse total from $COVERAGE_FILE" >&2
  exit 2
fi

awk -v t="$total" -v th="$THRESHOLD" 'BEGIN { exit !(t+0 >= th+0) }' \
  && status=0 || status=1

if [ "$status" -ne 0 ]; then
  echo "coverage: ${total}% < threshold ${THRESHOLD}%" >&2
  exit 1
fi

echo "coverage: ${total}% >= threshold ${THRESHOLD}%"
