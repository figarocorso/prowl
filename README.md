# 🦉 prowl

[![CI](https://github.com/figarocorso/prowl/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/figarocorso/prowl/actions/workflows/ci.yml)
[![Latest release](https://img.shields.io/github/v/release/figarocorso/prowl?sort=semver)](https://github.com/figarocorso/prowl/releases/latest)
[![Go version](https://img.shields.io/github/go-mod/go-version/figarocorso/prowl)](./go.mod)
[![License](https://img.shields.io/github/license/figarocorso/prowl)](./LICENSE)

> **Keep watch over your pull requests.**

A single-binary CLI + TUI for keeping tabs on the GitHub Pull Requests you care
about. Track a list of GitHub PR URLs, then run a single command to see their
current state (open / draft / merged / blocked), assignees, and GitHub
merge-queue position — all in a Bubble Tea interactive table, with `Enter` to
open the PR in your browser.

![prowl demo: prowl list followed by the interactive TUI navigating tracked PRs](docs/demo/demo.gif)

> Re-record with `vhs docs/demo/demo.tape` — see [`docs/demo/`](./docs/demo/).

## Features

- Interactive Bubble Tea TUI with arrow nav, open / copy / delete / refresh.
- One-shot non-interactive listing with `prowl list` (use `--json` for agents).
- GitHub merge-queue awareness (position + ETA).
- Lightweight archival: move merged / closed PRs out of the active list.
- Single static binary, no runtime dependencies beyond `gh` (used for auth).
- XDG-compliant data files, compatible with the legacy `prowl.sh`.

## Install

All install paths leave you with a `prowl` binary that must live on your
`PATH`. Verify with:

```sh
prowl check
```

If the command is not found after installing, the directory holding the
binary is not on your shell `PATH` — see the notes under each method.

### Homebrew (macOS / Linux)

Copy-paste this and you're done — `gh` is pulled in as a formula dependency:

```sh
brew install figarocorso/tap/prowl
gh auth login          # follow the prompts (skip if already authenticated)
prowl check
```

The tap lives at [`figarocorso/homebrew-tap`](https://github.com/figarocorso/homebrew-tap)
and is updated automatically on every release.

### `go install` (Linux quick start)

Three copy-paste blocks: install Go, install `gh` + helpers, then install
prowl. If you already have a recent Go toolchain on your `PATH`, skip to
Step 3.

#### Step 1 — install Go

Pick **one** of the two options. Option B is recommended because some apt
repos ship a Go version older than `go install` accepts.

**Option A — distro package (Ubuntu 24.04+ / Debian trixie+):**

```sh
sudo apt update && sudo apt install -y golang-go
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
```

**Option B — official tarball (any distro, latest stable Go):**

```sh
GO_VER=1.23.4
curl -fsSL https://go.dev/dl/go${GO_VER}.linux-amd64.tar.gz | sudo tar -C /usr/local -xz
echo 'export PATH="/usr/local/go/bin:$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
```

Full Go install docs: <https://go.dev/doc/install>.

#### Step 2 — install `gh` and clipboard / browser helpers

```sh
sudo apt update && sudo apt install -y gh xdg-utils xclip
gh auth login          # follow the prompts
```

`xdg-utils` lets prowl open PRs in your browser; `xclip` enables the copy-URL
shortcut. Both are optional but recommended.

#### Step 3 — install prowl

```sh
go install github.com/figarocorso/prowl@latest
prowl check
```

`go install` drops the binary in `$(go env GOBIN)` (if set) or
`$(go env GOPATH)/bin` — typically `~/go/bin`, which Step 1 already added to
your `PATH`.

### Pre-built binaries

Grab the archive for your platform from
[the releases page](https://github.com/figarocorso/prowl/releases), extract
it, and move the `prowl` binary somewhere already on your `PATH`:

```sh
tar -xzf prowl_<version>_<os>_<arch>.tar.gz
sudo mv prowl /usr/local/bin/prowl
```

### From source

```sh
git clone https://github.com/figarocorso/prowl.git
cd prowl
go build -o prowl .
sudo mv prowl /usr/local/bin/prowl     # or copy under any $PATH dir you own
```


## Requirements

- [`gh`][gh] (GitHub CLI), authenticated for repos you want to inspect
  (`prowl` shells out to `gh` for auth state but uses `cli/go-gh` directly for
  API calls).

Optional:

- `open` (macOS) / `xdg-open` (Linux) — open PRs in browser on `Enter`.
- `pbcopy` / `wl-copy` / `xclip` / `xsel` — copy URL to clipboard.

[gh]: https://cli.github.com/

## Usage

```sh
prowl                                       # interactive TUI
prowl watch [--interval 30s]                # TUI + periodic auto-refresh
prowl list                                  # plain table on stdout
prowl list --json                           # JSON array (agent-friendly)
prowl list --open                           # only currently-open PRs
prowl list --source reviewed                # show archived list
prowl add https://github.com/owner/repo/pull/123
prowl remove https://github.com/owner/repo/pull/123
prowl archive                               # move merged/closed to reviewed
prowl get <url> [--json]                    # single-PR detail
prowl check                                 # environment / auth / data dir
prowl version
```

### Watch mode

`prowl watch` opens the same TUI as `prowl` but re-fetches the active PR list
on a timer, so a long-running window stays current without you pressing `r`.

```sh
prowl watch                 # default: refresh every 30s
prowl watch --interval 1m   # custom cadence (minimum 5s to spare GitHub)
```

Press `q` or `Ctrl-C` to exit. The default 30s balances staying fresh against
the GitHub API rate limit; intervals below 5s are rejected.

Output styling: human terminals get colored, emoji-prefixed output. Pipes,
`NO_COLOR=1`, and `--plain` (alias `--no-color`) force ASCII-only output that
is safe for AI agents, scripts, and CI logs. `--json` is unaffected.

### Data files

`prowl` tracks two plain-text files, one URL per line:

| File                  | Purpose                                                  |
| --------------------- | -------------------------------------------------------- |
| `prs-active.txt`      | PRs you're watching                                      |
| `prs-reviewed.txt`    | Archive of PRs once they've merged or been closed        |

By default they live in `${XDG_DATA_HOME:-$HOME/.local/share}/prowl/`. Legacy
`prs-unmerged.txt` / `prs-merged.txt` / `prs-closed.txt` files are migrated
automatically on first run.

### Choosing a different data directory

Priority (highest to lowest):

1. `--data-dir <path>` flag
2. `PROWL_DATA_DIR` env var
3. `${XDG_DATA_HOME}/prowl`
4. `~/.local/share/prowl`

Individual file paths can also be overridden via `PROWL_ACTIVE` and
`PROWL_REVIEWED`.

### Profiles

Use `--profile <name>` (or `PROWL_PROFILE=<name>`) to keep PRs from different
contexts in separate subdirectories of the data dir (e.g. `work` vs
`personal`). When unset, prowl uses the base data dir unchanged so existing
setups keep working.

```sh
prowl --profile work add https://github.com/acme/api/pull/1234
PROWL_PROFILE=personal prowl list
```

### Optional config file

prowl looks for an optional YAML config at
`${XDG_CONFIG_HOME:-~/.config}/prowl/config.yml`:

```yaml
refresh_interval: 30s
columns: [PR, Assignee, Status, Details, URL]
```

### Raycast integration (macOS)

This repo ships two [Raycast Script Commands][raycast-scripts] in
[`raycast/`](./raycast/) that launch prowl in an iTerm tab.

By default they prefer the Go binary `prowl`; if it isn't on `PATH` they fall
back to `prowl.sh` (legacy). Override with `PROWL_BIN=/abs/path`.

[raycast-scripts]: https://manual.raycast.com/script-commands

### Authentication

prowl uses `cli/go-gh` and inherits whatever credentials `gh` has set up.
Authenticate once with:

```sh
gh auth login
```

Public PRs work without authentication; private PRs require it.

## AI / agent integration (MCP server)

prowl ships a stdio [Model Context Protocol][mcp] server so AI agents can
query your tracked PRs directly. Start it with:

```sh
prowl mcp
```

Available tools (read-only by default):

| Tool          | Purpose                                                                |
| ------------- | ---------------------------------------------------------------------- |
| `list_prs`    | List tracked PRs. Filter by `source` / `status` / `assignee`.          |
| `get_pr`      | Full detail for a single PR (state, assignees, queue position).        |
| `get_pr_diff` | Unified diff for a PR (or `summary=true` for `{files,+,-}` digest).    |

Mutating tools (`add_pr`, `remove_pr`) are gated behind
`--allow-mutations` or `PROWL_MCP_ALLOW_MUTATIONS=true`.

### Claude Code / Claude Desktop config

Add to your `mcpServers` section:

```json
{
  "mcpServers": {
    "prowl": {
      "command": "prowl",
      "args": ["mcp"]
    }
  }
}
```

To enable mutations, swap the args for `["mcp", "--allow-mutations"]`.

[mcp]: https://modelcontextprotocol.io

## Legacy bash script

The original `prowl.sh` (Bash + `gh` + `jq`) lives in [`legacy/`](./legacy/)
for users not ready to switch. It reads the same data directory as the Go
binary, so the two can coexist.

## License

Apache License 2.0 — see [`LICENSE`](./LICENSE).

## Contributing

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for development setup,
coverage policy, git hooks, and the release process.
