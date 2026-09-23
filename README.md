# Wayfindr

![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8.svg)
![herdr 0.9.0+](https://img.shields.io/badge/herdr-0.9.0%2B-8a2be2)
![wayfindr v0.20.0](https://img.shields.io/badge/wayfindr-v0.20.0-blue)
![platforms: linux, macOS](https://img.shields.io/badge/platforms-linux%2C%20macOS-informational)

**Group Git worktrees across distinct workspaces and repositories by feature rather than by repository.** A [Herdr](https://herdr.dev) plugin that brings scattered microservice checkouts, ticket branches, and agent statuses together into a cohesive sidebar pane.

👉 **Interactive Simulator & Documentation:** [`docs/index.html`](docs/index.html) (or visit [echeble.github.io/wayfindr](https://echeble.github.io/wayfindr/))

---

## Highlights

- **Grouped by Feature, Not Repository**: Microservices, k8s charts, and integration test suites scattered across repos are automatically assembled under single feature headings.
- **Dual Presentations (`v`)**: Switch instantly between a compact 40-column tree (`list`) and a touch/mouse-friendly card grid (`cards`) with dynamic row capacity based on repository name width.
- **Stable Feature Color Identity**: Group colors are computed via FNV-32a hashing so features retain their visual identity across sessions.
- **Agent & Git Status Awareness**: Displays agent counts and statuses (`blocked`, `working`, `done`, `idle`) alongside porcelain v2 ahead/behind and dirty git summaries.
- **Decoupled PR Lookups**: Pull request status (`merged`, `in progress`, `closed`) is checked asynchronously via GitHub GraphQL queries without blocking the initial paint.
- **Zero Background Polling**: No background timers or subprocess fan-out loops. World state is gathered only on pane open, manual refresh (`r`), or local UI mutations (`t`, `p`, `R`).
- **Singleton Sidebar**: Opening the sidebar moves focus to it and closes any orphaned sidebars in other workspaces.

---

## Quickstart

### 1. Install Plugin

```bash
herdr plugin install echeble/wayfindr
```

Open the sidebar pane:

```bash
herdr plugin action invoke toggle --plugin echeble.wayfindr
```

### 2. Configure Global Keybinding

Add to `~/.config/herdr/config.toml` to toggle the feature sidebar from anywhere with `cmd+shift+f`:

```toml
[[keys.command]]
key = "cmd+shift+f"
type = "plugin_action"
command = "echeble.wayfindr.toggle"
description = "toggle the feature sidebar"
```

### 3. Display Groups in Herdr's Native Sidebar

Wayfindr publishes each workspace's active group as a `$group` metadata token. Add it to your Herdr space row template:

```toml
[ui.sidebar.spaces]
rows = [["state_icon", "workspace"], ["branch", "git_status"], ["$group"]]
```

Then reload configuration with `herdr server reload-config`.

---

## Interface & Controls

### Reading Rows & Cards

```text
  ● marketplace-billing-service · worktree-WAYF-20748 ↑3 ✱3 ·here ⧗
  1 2                             3                   4     5     6
```

1. **Agent State**: `◆` blocked, `●` working, `✔` done, `○` idle, `·` open with no agent, `◌` checkout not open in Herdr.
2. **Repository**: Name of the repository checkout.
3. **Branch**: Active Git branch (hidden when identical to feature header).
4. **Git Status**: `↑N` ahead, `↓N` behind, `✱N` modified paths, `!N` merge conflicts.
5. **`·here`**: Indicates the checkout the current pane is running in.
6. **Pull Request**: `✔` merged, `⧗` in progress (review), `✕` closed.

### Keybindings & Mouse

| Key / Action | List View | Cards View |
|:---|:---|:---|
| `j` / `k` or `↓` / `↑` | Move cursor down / up | Move down / up one card row |
| `h` / `l` or `←` / `→` | — | Select previous / next card |
| `g` / `G` | Jump to top / bottom | Jump to first / last card |
| `enter` | Jump to worktree (opens workspace if needed) | Open feature (level 1) or jump to worktree (level 2) |
| `space` | Fold / unfold focused group | Press / activate focused card |
| `v` | Switch to Cards view | Switch to List view |
| `s` | Cycle sort order: `alpha`, `status`, `agent` (asc / desc) | Cycle sort order |
| `t` | Tag selected worktree into a group (empty clears tag) | ← |
| `p` | Pin derived group as an explicit tag | ← |
| `R` | Rename group display label (empty resets to derived) | ← |
| `r` | Refresh world state immediately | ← |
| `esc` / `q` | Close sidebar pane | Back out to level 1, then close pane |
| **Left Click** | Select row; click `▾`/`▸` to fold/unfold | First click selects/aims, second click opens |
| **Right Click** | Context menu: **Open**, **Tag**, **Pin**, **Fold/Unfold**, **Delete All** | Context menu: **Open**, **Tag**, **Pin**, **Rename**, **Delete All** |

---

## Grouping Precedence

Features are resolved deterministically using the first matching rule:

| Order | Rule | Match Target | Group Name |
|:---:|:---|:---|:---|
| **1** | **Explicit Tag** (`SourceExplicit`) | Path-keyed tag set via `t` or CLI `--tag` | User-assigned tag |
| **2** | **Ticket Key** (`SourceJira`) | Regex matching ticket pattern (`WAYF-20748`) in branch or workspace label | Matched key (uppercased) |
| **3** | **Branch Pattern** (`SourceBranchPattern`) | User-configured regexes with `(?P<group>...)` named capture group | Captured group name |
| **4** | **Shared Name** (`SourceWorktreeName`) | Directory basename shared across **2 or more distinct repositories** | Shared folder name |
| **5** | **Ungrouped** (`SourceNone`) | Fallback for any unmatched worktree | Sinks to bottom, starts folded |

> **Header Renames (`R`)**: Pressing `R` renames only the visual header label without altering underlying member assignment or group keys.

---

## Configuration

Optional. To customize defaults, copy [`config.example.toml`](config.example.toml) to `~/.config/herdr/plugins/config/echeble.wayfindr/config.toml`:

```toml
[pane]
view = "list"          # "list" or "cards"
width = 40             # Columns to resize sidebar split to
theme = "auto"         # "auto", "ink" (dark), or "paper" (light)
mouse = true           # Enable mouse selection and scrolling
right_click = true     # Claim right-click for Wayfindr context menu
singleton = true       # Keep only one active Wayfindr pane across workspaces

[grouping]
jira_pattern = '(?i)\b([A-Z]{2,10}-\d+)\b'
hide_principal_branches = true
principal_branches = ["main", "master"]

[pr]
enabled = true         # Asynchronous GitHub PR status check via GraphQL
```

---

## Headless CLI Operations

Wayfindr functions cleanly outside the TUI pane via Herdr's socket API:

```bash
herdr-wayfindr --list                        # Dump plain-text grouping, rules, and PR state
herdr-wayfindr --tag WAYF-20748              # Tag focused workspace's checkout
herdr-wayfindr --tag --workspace <id> WAYF-1 # Tag specific workspace checkout
herdr-wayfindr --tag                         # Clear tag on focused checkout
herdr-wayfindr --rename WAYF-20748 "Billing" # Retitle visual group header
herdr-wayfindr --rename WAYF-20748           # Reset header back to derived key
herdr-wayfindr --report-tokens               # Push $group metadata tokens to Herdr workspaces
```

---

## Building from Source

```bash
# Clone repository
git clone https://github.com/echeble/wayfindr ~/wayfindr
cd ~/wayfindr

# Build CLI & plugin binary
go build -o bin/herdr-wayfindr ./cmd/herdr-wayfindr

# Run unit tests
go test ./...

# Link plugin to local Herdr installation
herdr plugin link ~/wayfindr
herdr plugin list --plugin echeble.wayfindr --json
```

---

## License

MIT © [echeble](https://github.com/echeble)
