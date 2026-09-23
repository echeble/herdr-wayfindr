#!/usr/bin/env bash
#
# Lifecycle for the Wayfindr panes.
#
#   pane.sh toggle      open a sidebar pane, or close every one if any is open
#   pane.sh open        open a sidebar pane, no-op if one is open
#   pane.sh close       close every sidebar pane, no-op if none
#   pane.sh auto-open   workspace-birth hook, gated by pane.auto_open
#   pane.sh on-worktree restamp the sidebar tokens, then auto-open
#
# on-worktree is what the worktree.* event hooks call. Both jobs live behind one
# command so a single [[events]] entry per event name can do both, rather than
# relying on Herdr running two hooks for the same event.
#
# A sidebar pane is any pane running herdr-wayfindr in its foreground
# process group, read live per pane. There is no state file, so the answer
# survives a Herdr restart and also recognises a pane opened from a saved
# layout rather than by this script.

set -euo pipefail

# Herdr runs plugin commands with a minimal PATH, so jq and git are not on it.
PATH="/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:${PATH:-}"
export PATH

H="${HERDR_BIN_PATH:-herdr}"
PLUGIN_ID="${HERDR_PLUGIN_ID:-echeble.herdr-wayfindr}"
PLUGIN_ROOT="${HERDR_PLUGIN_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
ENTRYPOINT="sidebar"
BIN_NAME="herdr-wayfindr"
BIN="$PLUGIN_ROOT/bin/$BIN_NAME"

mode="${1:-toggle}"

die() {
  echo "herdr-wayfindr: $1" >&2
  exit 1
}

# quiet_exit ends an event-driven run without noise. Event stderr lands in
# `herdr plugin log list`, where a refusal to act is not a problem worth
# recording.
quiet_exit() {
  if [ "$mode" = auto-open ]; then
    exit 0
  fi

  die "$1"
}

# on-worktree is auto-open with a token restamp in front of it. Tokens are
# display-only state Herdr forgets, so every worktree event refreshes them.
if [ "$mode" = on-worktree ]; then
  mode=auto-open
  restamp=1
else
  restamp=0
fi

# --- configuration -----------------------------------------------------------
#
# The binary owns config parsing; bash reads the normalised JSON. One contract,
# no TOML in shell.

placement="split"
direction="right"
auto_open="false"
singleton="true"

if [ -x "$BIN" ]; then
  if config_json=$("$BIN" --resolve-plugin-config 2>/dev/null); then
    placement=$(printf '%s' "$config_json" | jq -r '.pane.placement // "split"')
    direction=$(printf '%s' "$config_json" | jq -r '.pane.direction // "right"')
    auto_open=$(printf '%s' "$config_json" | jq -r 'if .pane.auto_open then "true" else "false" end')
    singleton=$(printf '%s' "$config_json" | jq -r 'if .pane.singleton then "true" else "false" end')
  fi
fi

# Restamp before the auto-open gate: the sidebar tokens must refresh whether or
# not the user wants a pane opened for them.
if [ "$restamp" -eq 1 ] && [ -x "$BIN" ]; then
  "$BIN" --report-tokens >/dev/null 2>&1 || true
fi

if [ "$mode" = auto-open ] && [ "$auto_open" != "true" ]; then
  exit 0
fi

# --- context -----------------------------------------------------------------

ws="${HERDR_WORKSPACE_ID:-}"
pane="${HERDR_PANE_ID:-}"
cwd=""

if [ -n "${HERDR_PLUGIN_CONTEXT_JSON:-}" ]; then
  # `plugin action invoke` resolves context from the focused workspace wherever
  # it is run and ignores the calling pane's own HERDR_* variables, so the
  # context JSON wins over them.
  ctx_ws=$(printf '%s' "$HERDR_PLUGIN_CONTEXT_JSON" | jq -r '.workspace_id // empty')
  if [ -n "$ctx_ws" ]; then
    ws="$ctx_ws"
  fi

  ctx_pane=$(printf '%s' "$HERDR_PLUGIN_CONTEXT_JSON" | jq -r '.focused_pane_id // empty')
  if [ -n "$ctx_pane" ]; then
    pane="$ctx_pane"
  fi

  cwd=$(printf '%s' "$HERDR_PLUGIN_CONTEXT_JSON" | jq -r '.worktree.checkout_path // .workspace_cwd // empty')
fi

if [ "$mode" = auto-open ] && [ -n "${HERDR_PLUGIN_EVENT_JSON:-}" ]; then
  # worktree.opened also fires for a workspace that is already live. That is a
  # focus request, not a workspace birth: never resurrect a pane the user closed.
  if printf '%s' "$HERDR_PLUGIN_EVENT_JSON" | jq -e '.data.already_open == true' >/dev/null 2>&1; then
    exit 0
  fi

  # An event fires with no focused pane, so the payload names the workspace.
  ws=$(printf '%s' "$HERDR_PLUGIN_EVENT_JSON" |
    jq -r '.data.workspace.workspace_id // .data.worktree.open_workspace_id // empty')
  cwd=$(printf '%s' "$HERDR_PLUGIN_EVENT_JSON" |
    jq -r '.data.workspace.worktree.checkout_path // .data.worktree.path // empty')
  pane=""
fi

[ -n "$ws" ] || quiet_exit "no workspace in context"

# --- finding our panes -------------------------------------------------------

panes_json=$("$H" pane list --workspace "$ws" 2>/dev/null) ||
  quiet_exit "could not list panes in $ws"

pane_ids=$(printf '%s' "$panes_json" | jq -r '.result.panes[].pane_id')

# is_ours answers 0 for one of our panes, 1 for someone else's, 2 when the
# probe could not be read. An unreadable probe is never treated as "not ours":
# that would let toggle stack duplicates and let close report false success.
is_ours() {
  local target="$1" info count

  info=$("$H" pane process-info --pane "$target" 2>/dev/null) || return 2

  # Identity is the argv0/argv[0] basename. `name` is the rewritable process
  # title — a live agent pane reports its own version string there.
  # The one-shot modes share the binary name but are not panes, so they are
  # excluded here and must stay excluded in the Go flag set too.
  count=$(printf '%s' "$info" | jq -r --arg bin "$BIN_NAME" '
    def base: split("/") | last;
    [.result.process_info.foreground_processes[]
      | select((((.argv0 // "") | base) == $bin)
          or ((((.argv // [])[0] // "") | base) == $bin))
      | select(((.argv // []) | index("--resolve-plugin-config")) == null)
      | select(((.argv // []) | index("--report-tokens")) == null)
      | select(((.argv // []) | index("--tag")) == null)]
    | length' 2>/dev/null) || return 2

  [ -n "$count" ] || return 2
  [ "$count" -gt 0 ] 2>/dev/null || return 1

  return 0
}

ours=""
unreadable=0

for candidate in $pane_ids; do
  set +e
  is_ours "$candidate"
  verdict=$?
  set -e

  case "$verdict" in
    0) ours="$ours $candidate" ;;
    2) unreadable=1 ;;
  esac
done

ours=$(printf '%s' "$ours" | tr -s ' ' | sed 's/^ //')

# --- actions -----------------------------------------------------------------

# edge_pane answers the pane against the left or right edge of the current
# tab's layout, which is the one to split so the sidebar ends up on that edge.
# Ties on x are broken by y so the choice is stable.
edge_pane() {
  local anchor="$1" order

  order='sort_by(.rect.x, .rect.y) | first'
  if [ "$direction" = "right" ]; then
    order='sort_by(.rect.x, .rect.y) | last'
  fi

  "$H" pane layout --pane "$anchor" 2>/dev/null |
    jq -r "[.result.layout.panes[]] | $order | .pane_id // empty"
}

open_pane() {
  local focus_flag="$1"
  local -a args=()
  local target=""

  case "$placement" in
    split | zoomed)
      # split and zoomed need a pane to split from; the workspace alone is
      # rejected. Fall back to the workspace's first pane when the action did
      # not arrive with one, which is what an event hook looks like.
      if [ -z "$pane" ]; then
        pane=$(printf '%s' "$panes_json" | jq -r '.result.panes[0].pane_id // empty')
      fi

      [ -n "$pane" ] || quiet_exit "no pane to split in $ws"

      # Split the pane on the edge we want, not whichever one happens to be
      # focused, or the sidebar lands in the middle of the layout.
      target=$(edge_pane "$pane")
      [ -n "$target" ] || target="$pane"

      args=(--placement "$placement" --target-pane "$target")

      if [ "$placement" = "split" ]; then
        # Herdr only splits right or down. A left sidebar is a right split
        # followed by a swap, below.
        args+=(--direction right)
      fi
      ;;
    tab)
      args=(--placement tab --workspace "$ws")
      ;;
    *)
      args=(--placement overlay)
      ;;
  esac

  if [ -n "$cwd" ]; then
    args+=(--cwd "$cwd")
  fi

  local open_json new
  open_json=$("$H" plugin pane open --plugin "$PLUGIN_ID" --entrypoint "$ENTRYPOINT" \
    "${args[@]}" "$focus_flag" 2>/dev/null) || quiet_exit "could not open the sidebar"

  new=$(printf '%s' "$open_json" | jq -r '.result.plugin_pane.pane.pane_id // empty')
  [ -n "$new" ] || quiet_exit "Herdr did not report a pane id"

  # The split opened to the right of the edge pane; swapping the two puts the
  # sidebar on the left edge, between Herdr's own sidebar and your work. The
  # swap trades widths as well as places, and the pane resizes itself again
  # once the window event from this lands.
  if [ "$placement" = "split" ] && [ "$direction" = "left" ] && [ -n "$target" ]; then
    "$H" pane swap --source-pane "$new" --target-pane "$target" >/dev/null 2>&1 ||
      true
  fi

  if [ "$placement" = "tab" ]; then
    local tab
    tab=$(printf '%s' "$open_json" | jq -r '.result.plugin_pane.pane.tab_id // empty')

    # Cosmetic only — a rename that fails is not worth failing the open over.
    if [ -n "$tab" ]; then
      "$H" tab rename "$tab" "Features" >/dev/null 2>&1 || true
    fi
  fi

  # Last, so a failed open never costs the sidebar you already had.
  sweep "$new"
}

# retire closes every sidebar pane in the session except the ones named in
# $1, a space-separated list. `pane list` without --workspace answers the whole
# session, and probing all of it costs one socket round trip per pane — cheap
# enough to stay exact rather than guess from a pane's rewritable label.
#
# A pane that cannot be probed is left alone. Closing someone else's pane is a
# far worse outcome than leaving a second sidebar open.
retire() {
  local keep="$1" ids verdict

  ids=$("$H" pane list 2>/dev/null | jq -r '.result.panes[].pane_id') || return 0

  for candidate in $ids; do
    case " $keep " in
      *" $candidate "*) continue ;;
    esac

    set +e
    is_ours "$candidate"
    verdict=$?
    set -e

    if [ "$verdict" -ne 0 ]; then
      continue
    fi

    "$H" pane close "$candidate" >/dev/null 2>&1 || true
  done
}

# sweep is retire behind the setting, keeping the panes named in $1.
#
# Only an open the user asked for sweeps. An event-driven auto-open does not: a
# worktree created elsewhere must not take the sidebar out of the workspace you
# are looking at.
sweep() {
  if [ "$singleton" = "true" ] && [ "$mode" != auto-open ]; then
    retire "$1"
  fi
}

close_panes() {
  local failed="" err status

  for target in $ours; do
    set +e
    err=$("$H" pane close "$target" 2>&1 >/dev/null)
    status=$?
    set -e

    if [ "$status" -eq 0 ]; then
      continue
    fi

    case "$err" in
      # It exited on its own first. Same end state, so not a failure.
      *pane_not_found*) ;;
      *) failed="$failed $target" ;;
    esac
  done

  [ -z "$failed" ] || die "could not close:$failed"
}

case "$mode" in
  open | auto-open)
    if [ -n "$ours" ]; then
      sweep "$ours"

      exit 0
    fi

    [ "$unreadable" -eq 0 ] || quiet_exit "could not read every pane; refusing to risk a duplicate"

    # A manual open takes focus; a workspace-birth hook must not steal it.
    if [ "$mode" = auto-open ]; then
      open_pane --no-focus
    else
      open_pane --focus
    fi
    ;;

  close)
    # With one sidebar in the session, "close" means the one — wherever it is.
    sweep ""

    [ -n "$ours" ] || exit 0

    close_panes
    ;;

  toggle)
    [ "$unreadable" -eq 0 ] || die "could not read every pane; refusing to toggle blindly"

    if [ -n "$ours" ]; then
      close_panes
      sweep ""
    else
      open_pane --focus
    fi
    ;;

  *)
    die "unknown mode: $mode (expected popup, toggle, open, close, auto-open or on-worktree)"
    ;;
esac
