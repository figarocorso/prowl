# 🦉 prowl

> **Keep watch over your pull requests.**

A tiny, dependency-light Bash CLI for keeping tabs on the GitHub Pull Requests
you care about. Track a list of PR URLs, then run a single command to see their
current state (open/draft/merged/blocked), assignees, and GitHub merge-queue
position — all in a coloured terminal table, with an optional `fzf` interactive
mode that opens each PR in your browser on `<Enter>`.

```
┌───────┬──────────────┬───────────────┬──────────────────────┬─────┬─────┬────────────────────────────────────────────┐
│ PR    │ Assignee     │ Status        │ Queue                │ Pos │ ETA │ URL                                        │
├───────┼──────────────┼───────────────┼──────────────────────┼─────┼─────┼────────────────────────────────────────────┤
│ #1234 │ alice        │ open          │ queued (mergeable)   │ 2   │ ~7m │ https://github.com/acme/api/pull/1234      │
│ #1235 │ bob,carol    │ open/blocked  │ -                    │ -   │ -   │ https://github.com/acme/api/pull/1235      │
│ #1198 │ dave         │ merged        │ -                    │ -   │ -   │ https://github.com/acme/api/pull/1198      │
└───────┴──────────────┴───────────────┴──────────────────────┴─────┴─────┴────────────────────────────────────────────┘
```

## Features

- One-shot review of every PR you track, with a colour-coded table.
- GitHub merge-queue awareness (position + ETA).
- Interactive `fzf` mode: arrows to navigate, `Enter` to open, `r` to refresh,
  `c` to copy the URL.
- Lightweight archival: move merged / closed-not-merged PRs out of the active
  list automatically.
- Pure-Bash, follows the [XDG Base Directory Specification][xdg] for data files.
- Self-checks its environment with `check-dependencies`.

[xdg]: https://specifications.freedesktop.org/basedir-spec/basedir-spec-latest.html

## Requirements

**Required**

- `bash` 3.2+ (bash 4+ recommended)
- [`gh`][gh] (GitHub CLI), authenticated for repos you want to inspect
- `jq`
- `awk`

**Optional (the script degrades gracefully)**

- [`gum`][gum]  — coloured boxes, spinners, prompts
- [`fzf`][fzf]  — interactive table
- `open` (macOS) or `xdg-open` (Linux) — open PRs in browser
- `pbcopy` / `wl-copy` / `xclip` / `xsel` — copy URL to clipboard

Run `prowl.sh check-dependencies` to verify your environment.

[gh]: https://cli.github.com/
[gum]: https://github.com/charmbracelet/gum
[fzf]: https://github.com/junegunn/fzf

## Install

Drop the script anywhere on your `PATH` and make it executable:

```sh
curl -L https://raw.githubusercontent.com/<you>/prowl/main/prowl.sh \
  -o ~/.local/bin/prowl.sh
chmod +x ~/.local/bin/prowl.sh
```

Or clone the repo and symlink:

```sh
git clone https://github.com/<you>/prowl.git
ln -s "$PWD/prowl/prowl.sh" ~/.local/bin/prowl.sh
```

Then verify:

```sh
prowl.sh check-dependencies
```

## Usage

```sh
prowl.sh                                  # = prowl.sh review
prowl.sh review                           # show table for every tracked PR
prowl.sh add                              # prompt for one or more URLs
prowl.sh add https://github.com/o/r/pull/1
prowl.sh clean-merged                     # archive merged PRs
prowl.sh clean-closed                     # archive closed-not-merged PRs
prowl.sh check-dependencies               # environment check
prowl.sh --help
```

### Data files

`prowl.sh` tracks three plain-text files, one URL per line:

| File                | Purpose                                |
| ------------------- | -------------------------------------- |
| `prs-unmerged.txt`  | Active list — PRs you are watching     |
| `prs-merged.txt`    | Archive of PRs that have been merged   |
| `prs-closed.txt`    | Archive of PRs closed without merging  |

By default they live in `${XDG_DATA_HOME:-$HOME/.local/share}/prowl/`,
following the XDG Base Directory Specification. The directory and files are
created automatically on first use.

### Choosing a different data directory

Priority (highest to lowest):

1. `--data-dir <path>` flag
2. `PROWL_DATA_DIR` environment variable
3. `${XDG_DATA_HOME}/prowl`
4. `~/.local/share/prowl`

```sh
prowl.sh --data-dir ~/work/prs review
PROWL_DATA_DIR=~/work/prs prowl.sh review
```

Individual file paths can also be overridden via `PROWL_UNMERGED`,
`PROWL_MERGED`, `PROWL_CLOSED`.

### Interactive table (`fzf`)

When `fzf` is installed, `prowl.sh review` opens an interactive table:

| Key             | Action                                  |
| --------------- | --------------------------------------- |
| `↑` / `↓`       | Navigate                                |
| `Enter`         | Open the highlighted PR in your browser |
| `c`             | Copy the URL to your clipboard          |
| `r` / `Ctrl-R`  | Re-fetch every PR from GitHub           |
| `Esc` / `q`     | Quit                                    |

### Raycast integration (macOS)

This repo ships two [Raycast Script Commands][raycast-scripts] in
[`raycast/`](./raycast/) that launch `prowl.sh` in an iTerm tab:

| Script                  | What it does                                              |
| ----------------------- | --------------------------------------------------------- |
| `raycast/prowl.sh`  | Open a new iTerm tab and run `prowl.sh`               |
| `raycast/prowl-add.sh`     | Open a new iTerm tab and run `prowl.sh add <URL>`     |

**Setup:**

1. In Raycast → Extensions → Script Commands → add the `raycast/` directory as
   a script command root.
2. Make sure `prowl.sh` is on your `PATH`, **or** export an absolute path
   for Raycast to use:

   ```sh
   # In ~/.zshenv or ~/.bash_profile (Raycast inherits these on login):
   export PROWL_BIN=~/.local/bin/prowl.sh
   ```

3. iTerm must be installed (the scripts use AppleScript to drive it). To use a
   different terminal, edit the `osascript` block in each file.

[raycast-scripts]: https://manual.raycast.com/script-commands

### Authentication

`prowl.sh` shells out to `gh api graphql` and inherits whatever credentials
`gh` is using. Authenticate once with:

```sh
gh auth login
```

Public PRs work without authentication; private PRs require it.

## License

This project is licensed under the
[Apache License 2.0](https://www.apache.org/licenses/LICENSE-2.0).
See [`LICENSE`](./LICENSE) for the full text.

## Contributing

Issues and pull requests are welcome. Keep changes focused, small, and
self-contained; the goal is to remain a single-file Bash script that runs
anywhere with minimal dependencies.
