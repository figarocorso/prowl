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
Type "prowl add https://github.com/figarocorso/prowl/pull/29 >/dev/null"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/30 >/dev/null"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/28 >/dev/null"
Enter
Type "prowl add https://github.com/figarocorso/prowl/pull/27 >/dev/null"
Enter
Type "clear"
Enter
Show

# One-shot listing.
Type "prowl list"
Sleep 600ms
Enter
Sleep 5s

Type "clear"
Enter
Sleep 300ms

# Interactive TUI: open, navigate, quit.
Type "prowl"
Sleep 500ms
Enter
Sleep 3s
Down
Sleep 700ms
Down
Sleep 700ms
Down
Sleep 1500ms
Up
Sleep 1500ms
Type "q"
Sleep 800ms
