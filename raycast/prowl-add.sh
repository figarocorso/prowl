#!/bin/bash
#
# Copyright 2026 Miguel Julian
# Licensed under the Apache License, Version 2.0.
# See LICENSE in the project root for the full text.

# Required parameters:
# @raycast.schemaVersion 1
# @raycast.title prowl-add
# @raycast.mode silent

# Optional parameters:
# @raycast.icon ➕
# @raycast.argument1 { "type": "text", "placeholder": "PR URL" }
# @raycast.packageName prowl
# @raycast.description Add a PR URL to prowl tracking list
# @raycast.author figarocorso

# Override with: export PROWL_BIN=/path/to/prowl.sh
PROWL_CMD="${PROWL_BIN:-prowl.sh}"
PR_URL="$1"

PR_URL=$(echo "$PR_URL" | tr -d '[:space:]')

if [ -z "$PR_URL" ]; then
    echo "Please provide a PR URL"
    exit 1
fi

osascript <<EOF
tell application "iTerm"
  activate
  if (count of windows) = 0 then
    create window with default profile
    tell current session of current window to write text "$PROWL_CMD add $PR_URL"
  else
    tell current window
      create tab with default profile
      tell current session to write text "$PROWL_CMD add $PR_URL"
    end tell
  end if
end tell
EOF
