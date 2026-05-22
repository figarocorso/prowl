# Contributing to prowl

Issues and pull requests welcome. Keep changes focused, small, and
self-contained.

## Reporting bugs / requesting features

Open a [GitHub issue](https://github.com/figarocorso/prowl/issues) and
describe the expected vs. actual behavior, your OS, and the `prowl version`
output.

## Pull requests

- Branch off `main`.
- One logical change per PR. Avoid drive-by refactors.
- Run the local CI before pushing (see below) so you catch failures in
  seconds instead of waiting on GitHub Actions.
- Squash-merge is the default.
- `main` is protected: signed commits, linear history, passing CI, and
  no force pushes. See
  [`.github/branch-protection.md`](./.github/branch-protection.md) for
  the full policy and
  [`scripts/apply-branch-protection.sh`](./scripts/apply-branch-protection.sh)
  to (re-)apply it.

## Development

Common workflows via [Task](https://taskfile.dev):

```sh
task build           # go build -o prowl .
task test            # go test ./...
task test-race       # go test -race ./...
task coverage        # write coverage.out + print total
task coverage-html   # render coverage.html
task coverage-check  # fail if total < threshold (default 55%)
task lint            # golangci-lint
task tidy            # go mod tidy
task ci              # tidy + lint + test + coverage-check
```

Override the coverage threshold with
`COVERAGE_THRESHOLD=70 task coverage-check`.

## Coverage

CI enforces a project-wide coverage floor (currently **55%**) via
[`scripts/coverage-check.sh`](./scripts/coverage-check.sh). PRs that lower
coverage below the threshold fail the build.

On every PR, [`fgrosse/go-coverage-report`](https://github.com/fgrosse/go-coverage-report)
posts a comment showing per-package coverage delta vs. `main`. The comment
only appears when the PR changes `.go` files.

## Git hooks

This repo uses [Lefthook](https://lefthook.dev) (config: `lefthook.yml`):

- **pre-commit** — `gofmt`, `goimports`, `go vet`, `golangci-lint` (full +
  new-only), `go test -short`, `shellcheck`, `gitleaks`
- **commit-msg** — Conventional Commits via `commitlint` (with regex fallback)
- **pre-push** — `go test -race` + coverage threshold check

Install once:

```sh
brew install lefthook   # or: go install github.com/evilmartians/lefthook@latest
task install-hooks      # runs `lefthook install`
```

`golangci-lint` must be on your `PATH` for the pre-commit hook:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
```

### Optional tools

The remaining hooks soft-fail (warn, exit 0) when their tool is missing so
contributors aren't blocked locally — CI is the hard gate. Install whichever
you want to run pre-commit:

```sh
go install golang.org/x/tools/cmd/goimports@latest   # missing/extra imports
brew install shellcheck                              # shell script lint
brew install gitleaks                                # secret scanning
brew install node                                    # commitlint via npx
```

## Releasing

Releases are cut by tagging `vX.Y.Z` on `main` and pushing the tag. The
`release` workflow runs [GoReleaser](https://goreleaser.com), publishes
archives + checksums to the GitHub release, and updates the Homebrew tap at
[`figarocorso/homebrew-tap`](https://github.com/figarocorso/homebrew-tap).

Local snapshot build:

```sh
task release-snapshot
```
