#!/bin/bash
#
# Copyright 2026 Miguel Julian
# Licensed under the Apache License, Version 2.0.
# See LICENSE in the project root for the full text.

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title prowl
# @raycast.mode silent

# Optional parameters:
# @raycast.icon 🦉
# @raycast.packageName prowl
# @raycast.description Launch prowl interactive TUI in iTerm
# @raycast.author figarocorso

# Resolve which prowl to launch. Override with PROWL_BIN=/abs/path.
# Default: prefer the Go binary `prowl`; fall back to the legacy `prowl.sh`.
if [[ -n "${PROWL_BIN:-}" ]]; then
  PROWL_CMD="$PROWL_BIN"
elif command -v prowl >/dev/null 2>&1; then
  PROWL_CMD="prowl"
else
  PROWL_CMD="prowl.sh"
fi

osascript <<EOF
tell application "iTerm"
  activate
  if (count of windows) = 0 then
    create window with default profile
    tell current session of current window to write text "$PROWL_CMD"
  else
    tell current window
      create tab with default profile
      tell current session to write text "$PROWL_CMD"
    end tell
  end if
end tell
EOF
