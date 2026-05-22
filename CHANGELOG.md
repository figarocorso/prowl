# Changelog

All notable changes to prowl are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.1] - 2026-05-22

### Added
- Homebrew tap publishing via GoReleaser. Install with
  `brew install figarocorso/tap/prowl`.

## [0.1.0] - 2026-05-22

First stable Go release. The legacy `prowl.sh` Bash script moves to
[`legacy/`](./legacy/) and continues to work side-by-side against the
same data directory.

### Added
- Cobra-based CLI: `add`, `list`, `get`, `remove`, `archive`, `check`.
- Bubble Tea TUI with table view, search, refresh, copy, open-in-browser,
  and inline confirmation prompts for both delete (`d`) and the archive
  prompt on quit when closed/merged PRs are still tracked.
- Stdio MCP server (`prowl mcp`) exposing read-only tools for AI agents.
- One-way migration from the legacy `prs-unmerged/merged/closed.txt`
  layout to plain `active.txt` / `reviewed.txt` files under the XDG
  data directory.
- GoReleaser release pipeline producing tarballs/zips for
  linux/darwin/windows × amd64/arm64 on every `v*` tag push.
- `scripts/parity-check.sh` to diff Go vs legacy bash output during the
  cutover window.

[0.1.1]: https://github.com/figarocorso/prowl/releases/tag/v0.1.1
[0.1.0]: https://github.com/figarocorso/prowl/releases/tag/v0.1.0
