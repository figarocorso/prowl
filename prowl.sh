#!/usr/bin/env bash
#
# Copyright 2026 Miguel Julian
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" &>/dev/null && pwd)"

# Data directory resolution (XDG Base Directory spec).
# Priority: --data-dir flag > PROWL_DATA_DIR env > XDG_DATA_HOME/prowl > ~/.local/share/prowl
default_data_dir() {
  printf '%s/prowl' "${XDG_DATA_HOME:-$HOME/.local/share}"
}
DATA_DIR="${PROWL_DATA_DIR:-$(default_data_dir)}"

# Per-file overrides (legacy env vars take precedence over data dir).
resolve_files() {
  UNMERGED_FILE="${PROWL_UNMERGED:-$DATA_DIR/prs-unmerged.txt}"
  MERGED_FILE="${PROWL_MERGED:-$DATA_DIR/prs-merged.txt}"
  CLOSED_FILE="${PROWL_CLOSED:-$DATA_DIR/prs-closed.txt}"
}
resolve_files

PINK="212"
CYAN="51"
GREEN="46"
YELLOW="220"
GREY="245"
RED="196"

HAS_GUM=0
HAS_FZF=0
command -v gum >/dev/null 2>&1 && HAS_GUM=1
command -v fzf >/dev/null 2>&1 && HAS_FZF=1

# ---------------- UI helpers ----------------

banner() {
  if (( HAS_GUM )); then
    gum style --border rounded --padding "0 1" --border-foreground "$PINK" --foreground "$CYAN" -- "$@"
  else
    printf '== %s ==\n' "$*"
  fi
}

info() { (( HAS_GUM )) && gum style --foreground "$GREY"  -- "$@" || printf '%s\n' "$*"; }
ok()   { (( HAS_GUM )) && gum style --foreground "$GREEN" --bold -- "$@" || printf '%s\n' "$*"; }
warn() { (( HAS_GUM )) && gum style --foreground "$YELLOW" --bold -- "$@" || printf '%s\n' "$*" >&2; }
err()  { (( HAS_GUM )) && gum style --foreground "$RED"   --bold -- "$@" || printf '%s\n' "$*" >&2; }

is_tty() { [[ -t 0 && -t 1 ]]; }

confirm() {
  local prompt="$1" ans
  if (( HAS_GUM )) && is_tty; then
    gum confirm --prompt.foreground "$CYAN" --selected.background "$GREEN" "$prompt"
  else
    read -r -p "$prompt [y/N] " ans
    [[ "$ans" =~ ^[Yy] ]]
  fi
}

prompt_input() {
  local placeholder="$1" preset="${2:-}" ans
  if (( HAS_GUM )) && is_tty; then
    if [[ -n "$preset" ]]; then
      gum input --placeholder "$placeholder" --value "$preset"
    else
      gum input --placeholder "$placeholder"
    fi
  else
    read -r -p "$placeholder${preset:+ [$preset]}: " ans || true
    printf '%s' "${ans:-$preset}"
  fi
}

# ---------------- help ----------------

usage() {
  local body
  body=$(cat <<EOF
$(ok 'review')              Query GitHub for every tracked PR and print a table.
$(ok 'add')                 Add a PR URL to $(info "$(basename "$UNMERGED_FILE")").
                      Usage: $(basename "$0") add [url]
$(ok 'clean-merged')        Move merged PRs from $(info "$(basename "$UNMERGED_FILE")") to $(info "$(basename "$MERGED_FILE")").
$(ok 'clean-closed')        Move closed-but-not-merged PRs from $(info "$(basename "$UNMERGED_FILE")") to $(info "$(basename "$CLOSED_FILE")").
$(ok 'check-dependencies')  Verify required and optional tools are installed.

$(info 'Global flags:')
  $(ok '--data-dir <path>')   Directory holding the three list files (created if missing).

$(info 'Files (current data dir:') $(ok "$DATA_DIR")$(info '):')
  $(info "$UNMERGED_FILE")   $(info '# active list')
  $(info "$MERGED_FILE")     $(info '# archive of merged PRs')
  $(info "$CLOSED_FILE")     $(info '# archive of closed-without-merge PRs')

$(info 'Line format:') one PR URL per line.
$(info 'Env overrides:')
  PROWL_DATA_DIR        # default data directory
  PROWL_UNMERGED        # override individual file path
  PROWL_MERGED
  PROWL_CLOSED
  XDG_DATA_HOME             # honoured for default data dir
EOF
)
  banner "🦉 prowl"
  if (( HAS_GUM )); then
    gum style --padding "0 2" "$body"
  else
    printf '%s\n' "$body"
  fi
}

empty_files_help() {
  banner "📭 No PRs to review"
  if (( HAS_GUM )); then
    gum style --padding "0 2" \
      "$(info 'All list files are empty.')" \
      "" \
      "$(info 'Add one with:')" \
      "  $(ok "$(basename "$0") add")               $(info '# interactive')" \
      "  $(ok "$(basename "$0") add <pr-url>")      $(info '# direct')" \
      "" \
      "$(info 'Or edit the files manually:')" \
      "  $(info "$UNMERGED_FILE")" \
      "  $(info "$MERGED_FILE")" \
      "  $(info "$CLOSED_FILE")"
  else
    cat <<EOF
All list files are empty.

Add one with:
  $(basename "$0") add
  $(basename "$0") add <pr-url>

Or edit the files manually:
  $UNMERGED_FILE
  $MERGED_FILE
  $CLOSED_FILE
EOF
  fi
}

# ---------------- core ----------------

need() {
  command -v "$1" >/dev/null 2>&1 || { err "Missing dependency: $1"; exit 1; }
}

ensure_files() {
  # Create data dir for files that live under DATA_DIR.
  local f
  for f in "$UNMERGED_FILE" "$MERGED_FILE" "$CLOSED_FILE"; do
    local d
    d="$(dirname -- "$f")"
    [[ -d "$d" ]] || mkdir -p -- "$d"
  done
  [[ -f "$UNMERGED_FILE" ]] || : > "$UNMERGED_FILE"
  [[ -f "$MERGED_FILE" ]]   || : > "$MERGED_FILE"
  [[ -f "$CLOSED_FILE" ]]   || : > "$CLOSED_FILE"
}

# Echo URL extracted from a stored line (trim, strip legacy "<anything>|" prefix).
line_url() {
  local raw="$1"
  [[ "$raw" == *"|"* ]] && raw="${raw#*|}"
  raw="${raw#"${raw%%[![:space:]]*}"}"; raw="${raw%"${raw##*[![:space:]]}"}"
  printf '%s' "$raw"
}

# Echo TSV: number, state, mergeStateStatus, isDraft, assignees, queueState, queuePos, queueEta
fetch_pr() {
  local url="$1"
  local owner repo num json
  owner="$(printf '%s' "$url" | awk -F'/' '{print $4}')"
  repo="$(printf '%s'  "$url" | awk -F'/' '{print $5}')"
  num="$(printf '%s'   "$url" | awk -F'/' '{print $7}')"
  if [[ -z "$owner" || -z "$repo" || -z "$num" ]]; then
    printf '\t\t\t\t\t\t\t\n'; return 0
  fi
  local q='query($owner:String!,$repo:String!,$num:Int!){
    repository(owner:$owner,name:$repo){
      pullRequest(number:$num){
        number state mergeStateStatus isDraft
        assignees(first:5){ nodes{ login } }
        mergeQueueEntry{ state position estimatedTimeToMerge }
      }
    }
  }'
  if ! json="$(gh api graphql -F owner="$owner" -F repo="$repo" -F num="$num" -f query="$q" 2>/dev/null)"; then
    printf '\t\t\t\t\t\t\t\n'; return 0
  fi
  printf '%s' "$json" | jq -r '
    .data.repository.pullRequest as $p |
    if $p == null then "\t\t\t\t\t\t\t" else
      [
        ($p.number|tostring),
        $p.state,
        ($p.mergeStateStatus // ""),
        ($p.isDraft|tostring),
        ($p.assignees.nodes | map(.login) | join(",") | if . == "" then "-" else . end),
        ($p.mergeQueueEntry.state // ""),
        (($p.mergeQueueEntry.position // "") | tostring),
        (($p.mergeQueueEntry.estimatedTimeToMerge // "") | tostring)
      ] | @tsv
    end'
}

queue_label() {
  case "$1" in
    "")              echo "-" ;;
    AWAITING_CHECKS) echo "queued (awaiting checks)" ;;
    MERGEABLE)       echo "queued (mergeable)" ;;
    LOCKED)          echo "queued (locked)" ;;
    UNMERGEABLE)     echo "queued (unmergeable)" ;;
    QUEUED)          echo "queued" ;;
    *)               printf 'queued (%s)\n' "$(printf '%s' "$1" | tr '[:upper:]_' '[:lower:] ')" ;;
  esac
}

# Format an ETA expressed in seconds into "~Ns" / "~Nm" / "~Nh".
eta_format() {
  local secs="$1"
  [[ -z "$secs" ]] && { echo "-"; return; }
  if (( secs < 60 )); then
    printf '~%ds\n' "$secs"
  elif (( secs < 3600 )); then
    printf '~%dm\n' $(( (secs + 30) / 60 ))
  else
    printf '~%dh\n' $(( (secs + 1800) / 3600 ))
  fi
}

state_label() {
  local state="$1" mss="$2" draft="$3"
  case "$state" in
    MERGED) echo "merged" ;;
    CLOSED) echo "closed" ;;
    OPEN)
      if [[ "$draft" == "true" ]]; then
        echo "draft"
      else
        case "$mss" in
          CLEAN|HAS_HOOKS|UNSTABLE) echo "open" ;;
          BLOCKED|BEHIND|DIRTY|DRAFT) echo "open/blocked" ;;
          *) echo "open" ;;
        esac
      fi
      ;;
    "") echo "unknown" ;;
    *)  echo "$(printf '%s' "$state" | tr '[:upper:]' '[:lower:]')" ;;
  esac
}

count_entries() {
  local file="$1" line
  local n=0
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ "${line#"${line%%[![:space:]]*}"}" == \#* ]] && continue
    n=$((n+1))
  done < "$file"
  echo "$n"
}

# Stream URL per active entry in $1.
iter_urls() {
  local file="$1" line url
  while IFS= read -r line || [[ -n "$line" ]]; do
    [[ -z "${line//[[:space:]]/}" ]] && continue
    [[ "${line#"${line%%[![:space:]]*}"}" == \#* ]] && continue
    url="$(line_url "$line")"
    [[ -n "$url" ]] && printf '%s\n' "$url"
  done < "$file"
}

# Build rows TSV. Columns:
#   1 PR  2 Assignee  3 Status  4 Queue  5 Pos  6 ETA  7 URL  8 RawState  9 SourceTag
build_rows() {
  local source_file="$1" source_tag="$2" out="$3"
  local url info_tsv number state mss draft assignees qstate qpos qeta
  local label queue pos eta num
  while IFS= read -r url; do
    [[ -z "$url" ]] && continue
    info_tsv="$(fetch_pr "$url")"
    IFS=$'\t' read -r number state mss draft assignees qstate qpos qeta <<< "$info_tsv"
    label="$(state_label "$state" "$mss" "$draft")"
    queue="$(queue_label "$qstate")"
    pos="${qpos:--}"; [[ -z "$qpos" ]] && pos="-"
    eta="$(eta_format "$qeta")"
    if [[ -n "$number" ]]; then
      num="#$number"
    else
      num="#$(printf '%s' "$url" | awk -F'/' '{print $NF}')"
    fi
    [[ -z "$assignees" ]] && assignees="-"
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
      "$num" "$assignees" "$label" "$queue" "$pos" "$eta" "$url" "${state:-UNKNOWN}" "$source_tag" >> "$out"
  done < <(iter_urls "$source_file")
}

# Browser opener (cross-platform fallback chain).
open_url() {
  local url="$1"
  if command -v open >/dev/null 2>&1; then
    open "$url"
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$url" >/dev/null 2>&1 &
  elif [[ -n "${BROWSER:-}" ]]; then
    "$BROWSER" "$url" &
  else
    warn "No browser launcher found. URL: $url"
  fi
}

# Format rows TSV (from stdin) into padded display + URL appended, header on first line.
emit_padded() {
  awk -F'\t' '
    BEGIN {
      h[1]="PR"; h[2]="Assignee"; h[3]="Status"
      h[4]="Queue"; h[5]="Pos"; h[6]="ETA"; h[7]="URL"
      N=7
    }
    {
      for (i=1;i<=N;i++) {
        a[NR,i]=$i
        if (length($i)>w[i]) w[i]=length($i)
      }
      url[NR]=$7
    }
    END {
      for (i=1;i<=N;i++) if (length(h[i])>w[i]) w[i]=length(h[i])
      hline=""
      for (i=1;i<=N;i++) hline = hline sprintf("%-*s", w[i], h[i]) (i<N? " | " : "")
      print "\033[1;36m" hline "\033[0m\t"
      for (r=1;r<=NR;r++) {
        line=""
        for (i=1;i<=N;i++) line = line sprintf("%-*s", w[i], a[r,i]) (i<N? " | " : "")
        print line "\t" url[r]
      }
    }
  '
}

# Internal subcommand: re-fetch every tracked PR (UNMERGED + CLOSED) and emit padded rows.
cmd_emit_rows() {
  ensure_files
  local rows; rows=$(mktemp); trap "rm -f '$rows'" RETURN
  (( $(count_entries "$UNMERGED_FILE") > 0 )) && build_rows "$UNMERGED_FILE" "UNMERGED" "$rows"
  (( $(count_entries "$CLOSED_FILE")   > 0 )) && build_rows "$CLOSED_FILE"   "CLOSED"   "$rows"
  emit_padded < "$rows"
}

# Interactive table backed by fzf. Enter → open URL in browser (stays open). r → refresh.
fzf_interactive_table() {
  local file="$1" summary="$2"
  local self="${BASH_SOURCE[0]}"
  local opener
  if command -v open >/dev/null 2>&1; then
    opener="open"
  elif command -v xdg-open >/dev/null 2>&1; then
    opener="xdg-open"
  else
    opener="${BROWSER:-true}"
  fi
  local copier=""
  if command -v pbcopy >/dev/null 2>&1; then
    copier="pbcopy"
  elif command -v wl-copy >/dev/null 2>&1; then
    copier="wl-copy"
  elif command -v xclip >/dev/null 2>&1; then
    copier="xclip -selection clipboard"
  elif command -v xsel >/dev/null 2>&1; then
    copier="xsel --clipboard --input"
  fi
  local copy_hint=""
  local copy_bind=()
  if [[ -n "$copier" ]]; then
    copy_hint=" · c copy URL"
    copy_bind=(--bind "c:execute-silent(printf '%s' {2} | $copier)+bell")
  fi
  local hints=$'\033[2m↑/↓ navigate · Enter open PR'"${copy_hint}"$' · r refresh · Esc/q quit\033[0m'
  local header="${summary}"$'\n'"${hints}"
  emit_padded < "$file" | \
    fzf --ansi --reverse --info=inline \
        --height=80% --no-clear \
        --header "$header" \
        --header-lines=1 --header-first \
        --delimiter $'\t' --with-nth=1 \
        --no-multi \
        --prompt 'PR > ' \
        --pointer '▶' \
        --color "prompt:$CYAN,pointer:$PINK,fg+:$GREEN,hl:$YELLOW,hl+:$YELLOW,info:$GREY,header:$CYAN" \
        --bind "enter:execute-silent($opener {2} >/dev/null 2>&1)" \
        ${copy_bind[@]+"${copy_bind[@]}"} \
        --bind "r:reload($self _emit-rows)" \
        --bind "ctrl-r:reload($self _emit-rows)" \
        >/dev/null || true
}

interactive_table() {
  local file="$1" summary="${2:-}"
  if (( HAS_FZF )) && is_tty; then
    fzf_interactive_table "$file" "$summary"
  else
    render_table "$file"
  fi
}

# Render a boxed table. Uses first 7 columns (PR/Assignee/Status/Queue/Pos/ETA/URL).
render_table() {
  local file="$1"
  awk -F'\t' '
    BEGIN {
      headers[1]="PR"; headers[2]="Assignee"; headers[3]="Status"
      headers[4]="Queue"; headers[5]="Pos"; headers[6]="ETA"; headers[7]="URL"
      N=7
    }
    {
      for (i=1;i<=N;i++) {
        rows[NR,i]=$i
        if (length($i) > w[i]) w[i]=length($i)
      }
    }
    END {
      for (i=1;i<=N;i++) if (length(headers[i]) > w[i]) w[i]=length(headers[i])
      sep="┌"; for (i=1;i<=N;i++) sep=sep dup("─", w[i]+2) (i<N?"┬":"┐")
      mid="├"; for (i=1;i<=N;i++) mid=mid dup("─", w[i]+2) (i<N?"┼":"┤")
      bot="└"; for (i=1;i<=N;i++) bot=bot dup("─", w[i]+2) (i<N?"┴":"┘")
      print sep
      printf "│"; for (i=1;i<=N;i++) printf " %-" w[i] "s │", headers[i]; print ""
      print mid
      for (r=1; r<=NR; r++) {
        if (r>1) print mid
        printf "│"
        for (i=1;i<=N;i++) printf " %-" w[i] "s │", rows[r,i]
        print ""
      }
      print bot
    }
    function dup(c, n,    s) { s=""; while (n-- > 0) s=s c; return s }
  ' "$file"
}

# ---------------- commands ----------------

run_build_rows() {
  local file="$1" tag="$2" out="$3" title="$4"
  if (( HAS_GUM )); then
    gum spin --spinner dot --title "$title" -- \
      bash -c "$(declare -f line_url fetch_pr state_label queue_label eta_format iter_urls build_rows); \
               build_rows '$file' '$tag' '$out'"
  else
    info "$title"
    build_rows "$file" "$tag" "$out"
  fi
}

cmd_review() {
  ensure_files

  local total_unmerged total_closed
  total_unmerged=$(count_entries "$UNMERGED_FILE")
  total_closed=$(count_entries "$CLOSED_FILE")

  if (( total_unmerged + total_closed == 0 )); then
    banner "🦉 prowl" "📋 Reviewing tracked PRs"
    empty_files_help
    return 0
  fi

  local rows_file
  rows_file="$(mktemp)"
  # shellcheck disable=SC2064
  trap "rm -f '$rows_file'" RETURN

  (( total_unmerged > 0 )) && run_build_rows "$UNMERGED_FILE" "UNMERGED" "$rows_file" "Fetching $total_unmerged active PR(s)..."
  (( total_closed   > 0 )) && run_build_rows "$CLOSED_FILE"   "CLOSED"   "$rows_file" "Fetching $total_closed archived closed PR(s)..."

  if [[ ! -s "$rows_file" ]]; then
    warn "✗ No data fetched"
    return 1
  fi

  # Executive summary based on raw GitHub state (col 8).
  local n_open n_merged n_closed n_other
  n_open=$(awk   -F'\t' '$8=="OPEN"   {c++} END{print c+0}' "$rows_file")
  n_merged=$(awk -F'\t' '$8=="MERGED" {c++} END{print c+0}' "$rows_file")
  n_closed=$(awk -F'\t' '$8=="CLOSED" {c++} END{print c+0}' "$rows_file")
  n_other=$(awk  -F'\t' '$8!="OPEN" && $8!="MERGED" && $8!="CLOSED" {c++} END{print c+0}' "$rows_file")
  local summary
  summary="📊 ${n_open} open · ${n_merged} merged · ${n_closed} closed"
  (( n_other > 0 )) && summary="${summary} · ${n_other} unknown"
  banner "🦉 prowl" "$summary"

  interactive_table "$rows_file" "🦉 prowl · ${summary}"

  local merged_urls closed_urls
  merged_urls="$(awk -F'\t' '$9=="UNMERGED" && $8=="MERGED" {print $7}' "$rows_file")"
  closed_urls="$(awk -F'\t' '$9=="UNMERGED" && $8=="CLOSED" {print $7}' "$rows_file")"

  local merged_count closed_count
  merged_count=$(printf '%s' "$merged_urls" | grep -c . || true)
  closed_count=$(printf '%s' "$closed_urls" | grep -c . || true)

  if (( merged_count > 0 )); then
    info ""
    if confirm "$merged_count merged PR(s) in the active list. Move them to $(basename "$MERGED_FILE")?"; then
      move_urls "$UNMERGED_FILE" "$MERGED_FILE" "$merged_urls" "merged"
    fi
  fi

  if (( closed_count > 0 )); then
    info ""
    if confirm "$closed_count closed-not-merged PR(s) in the active list. Move them to $(basename "$CLOSED_FILE")?"; then
      move_urls "$UNMERGED_FILE" "$CLOSED_FILE" "$closed_urls" "closed"
    fi
  fi
}

# Move lines whose URL is in $3 (newline-separated) from $1 → $2.
# $4 = adjective for log message (e.g. "merged" / "closed").
move_urls() {
  local src="$1" dst="$2" urls="$3" what="$4"
  ensure_files
  local tmp; tmp="$(mktemp)"
  local kept=0 moved=0 line url
  while IFS= read -r line || [[ -n "$line" ]]; do
    if [[ -z "${line//[[:space:]]/}" || "${line#"${line%%[![:space:]]*}"}" == \#* ]]; then
      printf '%s\n' "$line" >> "$tmp"; continue
    fi
    url="$(line_url "$line")"
    if printf '%s\n' "$urls" | grep -Fxq -- "$url"; then
      printf '%s\n' "$line" >> "$dst"
      moved=$((moved+1))
    else
      printf '%s\n' "$line" >> "$tmp"; kept=$((kept+1))
    fi
  done < "$src"
  mv "$tmp" "$src"
  ok "✓ Moved $moved $what PR(s) to $(basename "$dst"); $kept entry/entries remain in $(basename "$src")"
  while IFS= read -r url; do [[ -n "$url" ]] && info "  → $url"; done <<<"$urls"
}

cmd_clean_merged() {
  ensure_files
  local total; total=$(count_entries "$UNMERGED_FILE")
  if (( total == 0 )); then warn "No entries in $(basename "$UNMERGED_FILE")"; return 0; fi
  local rows; rows="$(mktemp)"; trap "rm -f '$rows'" RETURN
  run_build_rows "$UNMERGED_FILE" "UNMERGED" "$rows" "Checking $total PR(s)..."
  local urls; urls="$(awk -F'\t' '$8=="MERGED" {print $7}' "$rows")"
  local n; n=$(printf '%s' "$urls" | grep -c . || true)
  if (( n == 0 )); then ok "✓ No merged PRs to move"; return 0; fi
  move_urls "$UNMERGED_FILE" "$MERGED_FILE" "$urls" "merged"
}

cmd_clean_closed() {
  ensure_files
  local total; total=$(count_entries "$UNMERGED_FILE")
  if (( total == 0 )); then warn "No entries in $(basename "$UNMERGED_FILE")"; return 0; fi
  local rows; rows="$(mktemp)"; trap "rm -f '$rows'" RETURN
  run_build_rows "$UNMERGED_FILE" "UNMERGED" "$rows" "Checking $total PR(s)..."
  local urls; urls="$(awk -F'\t' '$8=="CLOSED" {print $7}' "$rows")"
  local n; n=$(printf '%s' "$urls" | grep -c . || true)
  if (( n == 0 )); then ok "✓ No closed-not-merged PRs to move"; return 0; fi
  move_urls "$UNMERGED_FILE" "$CLOSED_FILE" "$urls" "closed"
}

add_one() {
  local url="$1"
  url="${url#"${url%%[![:space:]]*}"}"; url="${url%"${url##*[![:space:]]}"}"
  if [[ ! "$url" =~ ^(https?://github\.com/[^/]+/[^/]+/pull/[0-9]+) ]]; then
    err "✗ Not a GitHub PR URL: $url"; return 1
  fi
  url="${BASH_REMATCH[1]}"
  if grep -Fxq "$url" "$UNMERGED_FILE" 2>/dev/null \
     || grep -Fxq "$url" "$MERGED_FILE" 2>/dev/null \
     || grep -Fxq "$url" "$CLOSED_FILE" 2>/dev/null; then
    warn "⚠ Already tracked: $url"; return 1
  fi
  printf '%s\n' "$url" >> "$UNMERGED_FILE"
  ok "✓ Added: $url"
}

cmd_add() {
  ensure_files
  banner "➕ prowl" "Add PR(s) to $(basename "$UNMERGED_FILE")"

  local added=0

  if [[ $# -ge 1 ]]; then
    if add_one "$1"; then
      added=1
    fi
    (( added > 0 )) && { info ""; cmd_review; }
    return
  fi

  info "Enter PR URLs. Empty input finishes."
  local url count=0
  while true; do
    url="$(prompt_input 'PR URL (empty to finish)')"
    url="${url#"${url%%[![:space:]]*}"}"; url="${url%"${url##*[![:space:]]}"}"
    [[ -z "$url" ]] && break
    if add_one "$url"; then
      count=$((count+1))
    fi
  done
  if (( count == 0 )); then
    info "No PRs added."
  else
    ok "✓ Added $count PR(s) to $(basename "$UNMERGED_FILE")"
    info ""
    cmd_review
  fi
}

# ---------------- check-dependencies ----------------

# Print a colour-aware status line: STATUS  NAME  detail.
dep_print() {
  local status="$1" name="$2" detail="$3"
  local tag color
  case "$status" in
    OK)       tag="[OK]      "; color="$GREEN"  ;;
    MISSING)  tag="[MISSING] "; color="$RED"    ;;
    OPTIONAL) tag="[OPTIONAL]"; color="$YELLOW" ;;
    *)        tag="[?]       "; color="$GREY"   ;;
  esac
  if (( HAS_GUM )); then
    printf '%s  %-22s %s\n' \
      "$(gum style --foreground "$color" --bold "$tag")" \
      "$name" \
      "$(gum style --foreground "$GREY" "$detail")"
  else
    printf '%s  %-22s %s\n' "$tag" "$name" "$detail"
  fi
}

# Try to locate at least one binary out of a list. Echoes the found binary or "".
which_first() {
  local b
  for b in "$@"; do
    if command -v "$b" >/dev/null 2>&1; then printf '%s' "$b"; return 0; fi
  done
  return 1
}

cmd_check_dependencies() {
  banner "🔧 prowl · dependency check"

  local missing_required=0

  # ---- Required ----
  if (( HAS_GUM )); then
    gum style --bold --foreground "$CYAN" "Required"
  else
    printf '\nRequired\n'
  fi

  local bin path
  for bin in bash awk gh jq; do
    if path="$(command -v "$bin" 2>/dev/null)" && [[ -n "$path" ]]; then
      dep_print OK "$bin" "$path"
    else
      dep_print MISSING "$bin" "install '$bin' and retry"
      missing_required=$((missing_required+1))
    fi
  done

  # Bash version (3.2+ supported; 4+ recommended).
  if [[ -n "${BASH_VERSION:-}" ]]; then
    local bash_major="${BASH_VERSION%%.*}"
    if (( bash_major < 3 )); then
      dep_print MISSING "bash>=3.2" "found $BASH_VERSION"
      missing_required=$((missing_required+1))
    elif (( bash_major < 4 )); then
      dep_print OPTIONAL "bash>=4" "found $BASH_VERSION (works, but bash 4+ recommended; 'brew install bash' on macOS)"
    else
      dep_print OK "bash>=4" "$BASH_VERSION"
    fi
  fi

  # gh auth status (informational; gh works without auth for public repos but private fetch will fail).
  if command -v gh >/dev/null 2>&1; then
    if gh auth status >/dev/null 2>&1; then
      dep_print OK "gh auth" "authenticated"
    else
      dep_print OPTIONAL "gh auth" "not authenticated; run 'gh auth login' for private repos"
    fi
  fi

  # ---- Optional ----
  if (( HAS_GUM )); then
    printf '\n'; gum style --bold --foreground "$CYAN" "Optional (degrades gracefully)"
  else
    printf '\nOptional (degrades gracefully)\n'
  fi

  if command -v gum >/dev/null 2>&1; then
    dep_print OK "gum" "$(command -v gum) — pretty UI enabled"
  else
    dep_print OPTIONAL "gum" "plain-text UI fallback (https://github.com/charmbracelet/gum)"
  fi

  if command -v fzf >/dev/null 2>&1; then
    dep_print OK "fzf" "$(command -v fzf) — interactive table enabled"
  else
    dep_print OPTIONAL "fzf" "static boxed table fallback (https://github.com/junegunn/fzf)"
  fi

  # Browser opener
  if path="$(which_first open xdg-open)"; then
    dep_print OK "browser opener" "$path"
  elif [[ -n "${BROWSER:-}" ]]; then
    dep_print OK "browser opener" "\$BROWSER=$BROWSER"
  else
    dep_print OPTIONAL "browser opener" "no 'open'/'xdg-open' nor \$BROWSER; PR links won't auto-open"
  fi

  # Clipboard
  if path="$(which_first pbcopy wl-copy xclip xsel)"; then
    dep_print OK "clipboard" "$path"
  else
    dep_print OPTIONAL "clipboard" "no pbcopy/wl-copy/xclip/xsel; 'c' copy bind disabled"
  fi

  # ---- Data dir ----
  if (( HAS_GUM )); then
    printf '\n'; gum style --bold --foreground "$CYAN" "Data directory"
  else
    printf '\nData directory\n'
  fi
  if [[ -d "$DATA_DIR" ]]; then
    if [[ -w "$DATA_DIR" ]]; then
      dep_print OK "$DATA_DIR" "writable"
    else
      dep_print MISSING "$DATA_DIR" "directory exists but is not writable"
      missing_required=$((missing_required+1))
    fi
  else
    dep_print OPTIONAL "$DATA_DIR" "does not exist (will be created on first use)"
  fi

  printf '\n'
  if (( missing_required > 0 )); then
    err "✗ $missing_required required dependency/dependencies missing"
    return 1
  fi
  ok "✓ All required dependencies present"
}

# ---------------- main ----------------

# Strip global flags (--data-dir <path>, --data-dir=<path>) from positional args.
# Mutates a global POSITIONAL array.
parse_global_flags() {
  POSITIONAL=()
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --data-dir)
        [[ $# -ge 2 ]] || { err "--data-dir requires a path"; exit 2; }
        DATA_DIR="$2"; shift 2 ;;
      --data-dir=*)
        DATA_DIR="${1#--data-dir=}"; shift ;;
      --)
        shift; POSITIONAL+=("$@"); break ;;
      *)
        POSITIONAL+=("$1"); shift ;;
    esac
  done
  resolve_files
}

main() {
  parse_global_flags "$@"
  set -- "${POSITIONAL[@]+"${POSITIONAL[@]}"}"

  # check-dependencies and help must run even when required tools are absent.
  if [[ ${1:-} == "check-dependencies" || ${1:-} == "-h" || ${1:-} == "--help" || ${1:-} == "help" ]]; then
    case "$1" in
      check-dependencies) shift; cmd_check_dependencies "$@"; return ;;
      *) usage; return ;;
    esac
  fi

  need gh
  need jq
  need awk
  if [[ $# -eq 0 ]]; then
    cmd_review
    return
  fi
  case "$1" in
    review)         shift; cmd_review "$@" ;;
    add)            shift; cmd_add "$@" ;;
    clean-merged)   shift; cmd_clean_merged "$@" ;;
    clean-closed)   shift; cmd_clean_closed "$@" ;;
    _emit-rows)     shift; cmd_emit_rows "$@" ;;
    *) err "Unknown command: $1"; usage; exit 1 ;;
  esac
}

main "$@"
