#!/usr/bin/env bash
# commit-msg hook helper. Enforces Conventional Commits.
#
# Prefers @commitlint/config-conventional via npx when node is available;
# falls back to an inline regex check otherwise. Either path is a hard fail
# for malformed messages — formatting is cheap and CI shouldn't have to
# bounce a PR for a typo'd commit subject.
set -euo pipefail

msg_file="${1:-}"
if [ -z "$msg_file" ] || [ ! -f "$msg_file" ]; then
  echo "commit-msg: no message file passed" >&2
  exit 1
fi

# Strip comment lines so we lint only the real subject/body.
first_line=$(grep -v '^#' "$msg_file" | sed '/./,$!d' | head -n 1)

# Skip auto-generated messages (merges, reverts, fixups) that have their own
# established prefixes.
case "$first_line" in
  "Merge "*|"Revert "*|"fixup! "*|"squash! "*|"amend! "*)
    exit 0
    ;;
esac

if command -v npx >/dev/null 2>&1 && command -v node >/dev/null 2>&1; then
  # `npx -y` auto-installs into the npm cache on first run; subsequent runs
  # are fast. `--extends` keeps us off a repo-level commitlint config so we
  # don't have to ship one.
  if npx -y \
       -p @commitlint/cli \
       -p @commitlint/config-conventional \
       commitlint --extends @commitlint/config-conventional --edit "$msg_file"; then
    exit 0
  fi
  echo "commit-msg: commitlint rejected the message" >&2
  exit 1
fi

echo "commit-msg: node/npx not found — falling back to inline regex check"

# Conventional Commits: <type>(optional scope)!?: <subject>
# Types match the @commitlint/config-conventional default set.
pattern='^(build|chore|ci|docs|feat|fix|perf|refactor|revert|style|test)(\([^)]+\))?!?: .+'
if ! printf '%s' "$first_line" | grep -Eq "$pattern"; then
  cat >&2 <<EOF
commit-msg: subject does not match Conventional Commits.

Got:      $first_line
Expected: <type>(scope)?: <subject>
Types:    build, chore, ci, docs, feat, fix, perf, refactor, revert, style, test

Example:  feat(watch): add refresh interval flag
EOF
  exit 1
fi
