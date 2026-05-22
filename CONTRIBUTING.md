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

- **pre-commit** — `gofmt`, `go vet`, `golangci-lint`
- **pre-push** — `go test -race` + coverage threshold check

Install once:

```sh
brew install lefthook   # or: go install github.com/evilmartians/lefthook@latest
task install-hooks      # runs `lefthook install`
```

`golangci-lint` must also be on your `PATH` for the pre-commit hook:

```sh
go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest
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
