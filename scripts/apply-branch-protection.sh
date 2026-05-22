#!/usr/bin/env bash
# Apply (or re-apply) branch protection rules to figarocorso/prowl `main`.
# Idempotent: GitHub's PUT replaces the protection config wholesale.
#
# Requires: gh CLI authenticated with `repo` scope on figarocorso/prowl.
# Policy source of truth: .github/branch-protection.md
#
# Usage:
#   ./scripts/apply-branch-protection.sh
#
# Override the target repo/branch via env if needed:
#   REPO=figarocorso/prowl BRANCH=main ./scripts/apply-branch-protection.sh

set -euo pipefail

REPO="${REPO:-figarocorso/prowl}"
BRANCH="${BRANCH:-main}"
REQUIRED_CHECK="${REQUIRED_CHECK:-test + lint + coverage}"

echo "Applying branch protection to ${REPO}@${BRANCH}..."

payload=$(cat <<JSON
{
  "required_status_checks": {
    "strict": true,
    "checks": [
      { "context": "${REQUIRED_CHECK}" }
    ]
  },
  "enforce_admins": null,
  "required_pull_request_reviews": {
    "dismiss_stale_reviews": true,
    "require_code_owner_reviews": false,
    "required_approving_review_count": 0,
    "require_last_push_approval": false
  },
  "restrictions": null,
  "required_linear_history": true,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_conversation_resolution": true,
  "required_signatures": true,
  "lock_branch": false,
  "allow_fork_syncing": false
}
JSON
)

# PUT replaces the whole config. required_signatures travels in the same
# payload now (no need for the legacy /required_signatures sub-endpoint).
echo "${payload}" | gh api \
  --method PUT \
  -H "Accept: application/vnd.github+json" \
  "/repos/${REPO}/branches/${BRANCH}/protection" \
  --input - > /dev/null

echo "Done. Current protection:"
gh api "/repos/${REPO}/branches/${BRANCH}/protection" \
  --jq '{
    required_status_checks: .required_status_checks,
    required_linear_history: .required_linear_history.enabled,
    required_signatures: .required_signatures.enabled,
    enforce_admins: .enforce_admins.enabled,
    dismiss_stale_reviews: .required_pull_request_reviews.dismiss_stale_reviews,
    required_approving_review_count: .required_pull_request_reviews.required_approving_review_count,
    allow_force_pushes: .allow_force_pushes.enabled,
    allow_deletions: .allow_deletions.enabled,
    required_conversation_resolution: .required_conversation_resolution.enabled
  }'
