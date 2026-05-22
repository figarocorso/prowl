# `main` branch protection

The `main` branch on `figarocorso/prowl` is protected by GitHub branch
protection rules so that history stays clean, every change is reviewed
by CI, and unsigned/unverified commits cannot land.

## Rules in force

- **Required status checks** — the `test + lint + coverage` job (from
  [`.github/workflows/ci.yml`](./workflows/ci.yml)) must pass before a
  PR can be merged. Branches must be up to date with `main` before
  merging (`strict: true`).
- **Linear history** — only squash- or rebase-merges are accepted. No
  merge commits.
- **Signed commits** — every commit on `main` must be GPG/SSH signed
  and show "Verified" on GitHub.
- **Stale review dismissal** — when new commits are pushed to a PR,
  any previously approving reviews are dismissed automatically.
- **Pull request review required** — pull requests require **0**
  approving reviews. This is a single-maintainer concession: the rule
  block is kept enabled so that `dismiss_stale_reviews` has somewhere
  to attach, but no human approval is forced. Bump
  `required_approving_review_count` to `1` once a second maintainer is
  on board.
- **Force pushes / deletions** — blocked.
- **Administrators** — *not* included. The repo owner
  (`figarocorso`) retains admin bypass so solo-maintainer work isn't
  deadlocked when CI flakes or a fast follow is needed.
- **Conversation resolution** — required before merge.

## Re-applying the policy

If the rules drift (or were wiped from the UI), re-run the idempotent
script — it's the source of truth:

```sh
./scripts/apply-branch-protection.sh
```

The script does a `PUT` on
`/repos/figarocorso/prowl/branches/main/protection` with the exact
payload above. It needs a `gh` session with `repo` scope on
`figarocorso/prowl` (the owner account). Verify after with:

```sh
gh api repos/figarocorso/prowl/branches/main/protection
```

## Why these rules

- **Signed commits + linear history** keep `git log` auditable and
  bisectable: every commit is attributable, every parent is single.
- **Required CI** prevents red `main`. The coverage gate (55% floor)
  rides along inside the same job.
- **Dismiss stale reviews** means a "looks good" from yesterday
  doesn't silently rubber-stamp today's force-added commit.
- **Admin bypass on** is a deliberate trade-off: a one-person repo
  with required reviewers and no bypass is a self-DoS.
