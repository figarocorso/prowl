# 🦉 prowl

> **Keep watch over your pull requests.**

A single-binary CLI + TUI for keeping tabs on the GitHub Pull Requests you care
about. Track a list of PR URLs, then run a single command to see their current
state (open / draft / merged / blocked), assignees, and GitHub merge-queue
position — all in a Bubble Tea interactive table, with `Enter` to open the PR
in your browser.

```
🦉 prowl  📊 2 open · 1 merged · 0 closed

  PR     │ Assignee   │ Status        │ Queue                │ Pos │ ETA │ URL
 ────────┼────────────┼───────────────┼──────────────────────┼─────┼─────┼──────────
  #1234  │ alice      │ open          │ queued (mergeable)   │ 2   │ ~7m │ …pull/1234
  #1235  │ bob,carol  │ open/blocked  │ -                    │ -   │ -   │ …pull/1235
  #1198  │ dave       │ merged        │ -                    │ -   │ -   │ …pull/1198

  ↑↓ nav · ⏎ open · c copy · d delete · r refresh · q quit
```

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

```sh
brew install figarocorso/tap/prowl
```

The tap lives at [`figarocorso/homebrew-tap`](https://github.com/figarocorso/homebrew-tap)
and is updated automatically on every release.

### `go install`

```sh
go install github.com/figarocorso/prowl@latest
```

This drops the binary in `$(go env GOBIN)` (if set) or `$(go env GOPATH)/bin`
(typically `~/go/bin`). Add it to your shell `PATH` once:

```sh
# zsh
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc

# bash
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.bashrc && source ~/.bashrc
```

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

### Optional config file

prowl looks for an optional YAML config at
`${XDG_CONFIG_HOME:-~/.config}/prowl/config.yml`:

```yaml
refresh_interval: 30s
columns: [PR, Assignee, Status, Queue, URL]
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

Issues and pull requests welcome. Keep changes focused, small, and
self-contained.
