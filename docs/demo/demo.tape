# Demo tape for the README screencast.
#
# Render with:  vhs docs/demo/demo.tape
#
# Requires the `prowl` binary on PATH and `gh auth status` healthy, since
# the demo hits live GitHub PRs in this repo.

Output docs/demo/demo.gif

Set Shell "bash"
Set FontSize 16
Set Width 1200
Set Height 720
Set Theme "Dracula"
Set TypingSpeed 40ms
Set PlaybackSpeed 1.0

# Seed a throwaway data dir so the demo never touches the user's real list.
Hide
Type "export PROWL_DATA_DIR=$(mktemp -d)"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/34 >/dev/null"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/33 >/dev/null"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/32 >/dev/null"
Enter
Type "clear"
Enter
Show

# Lead with the wow: interactive TUI on a pre-loaded list.
Type "prowl"
Sleep 500ms
Enter
Sleep 3s
Down
Sleep 800ms
Down
Sleep 800ms
Up
Sleep 1s

# Add an open PR live via the slash-command palette.
Type "/"
Sleep 600ms
Type "add https://github.com/figarocorso/prowl/pull/36"
Sleep 800ms
Enter
Sleep 4s
Down
Sleep 700ms
Down
Sleep 700ms
Down
Sleep 1200ms

# Quit — prowl offers to archive the merged ones. Confirm.
Type "q"
Sleep 2500ms
Type "y"
Sleep 1500ms

# Re-enter prowl: merged are gone, only the open PR remains.
Type "prowl"
Sleep 400ms
Enter
Sleep 3500ms
Type "q"
Sleep 800ms

# Same list, now ready for AI agents.
Type "# Ready for AI"
Enter
Sleep 500ms
Type "prowl list --json"
Sleep 400ms
Enter
Sleep 4s
