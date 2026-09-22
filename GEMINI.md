# Wayfindr: Project Context & AI Agent Guide

Wayfindr is a Go-based [Herdr](https://herdr.dev) plugin that collects Git worktrees across distinct workspaces and repositories and groups them by **feature** rather than by repository. It displays them in a sidebar pane with two view modes (`list` and `cards`), color-codes features stably, tracks agent statuses and Git status, and checks GitHub pull requests without background polling.

---

## 1. Architecture & Directory Map

```text
wayfindr/
├── cmd/
│   └── herdr-wayfindr/
│       └── main.go           # CLI entrypoint; routes between TUI pane and one-shot flags
├── internal/
│   ├── collect/              # Gathers world state: worktree listing, git porcelain v2, gh PR checks
│   │   ├── collect.go        # Fan-out collector across distinct repo roots; agent state aggregation
│   │   ├── git.go            # git status --porcelain=v2 --branch parsing; hardened PATH lookup
│   │   └── pr.go             # gh pr list inspection; AggregatePR logic; merged PR cache
│   ├── config/               # TOML config loader (~/.config/herdr/plugins/config/echeble.wayfindr/)
│   │   └── config.go         # Defaults, fallback validation, JSON serialization for pane.sh
│   ├── group/                # Pure business logic: feature grouping resolver
│   │   └── resolve.go        # Rule precedence: Tag -> Ticket -> Pattern -> Shared Name -> Ungrouped
│   ├── herdr/                # Client for Herdr NDJSON socket ($HERDR_SOCKET_PATH)
│   │   ├── client.go         # SocketClient; single request/response per connection
│   │   ├── types.go          # Snapshot, Workspace, Worktree, Pane, Agent structs
│   │   ├── layout.go         # Pane layout inspection, split tree parsing, set_split_ratio
│   │   └── input.go          # pane.input.set routing (claims right-click from Herdr chrome)
│   ├── store/                # JSON persistence in ~/.local/state/herdr/plugins/echeble.wayfindr/
│   │   ├── store.go          # groups.json: explicit tags (path -> tag) and header renames (name -> label)
│   │   └── prefs.go          # ui.json: last active view ("list"|"cards") and sort order
│   ├── tokens/               # Herdr sidebar token reporting
│   │   └── report.go         # Reports $group token to workspaces; deduplicates socket calls
│   └── ui/                   # Bubble Tea TUI implementation
│       ├── model.go          # Root Model, Update loop, async commands, cursor & clamp management
│       ├── view.go           # Render loop, list rendering, header/footer styling, context menu overlay
│       ├── rows.go           # Row representation, bucket building, header construction
│       ├── cards.go          # Dynamic card grid layout, name-based sizing, two-level drill-down
│       ├── sizing.go         # Split ratio calculation for auto-resizing pane to pane.width
│       ├── sort.go           # Sort orders: alpha, status, agent (each asc & desc)
│       └── palette.go        # FNV-32a color hashing for stable feature hue, GitHub PR styles
├── herdr/
│   └── pane.sh               # Bash lifecycle script: singleton check, left-edge placement swap, hooks
├── herdr-plugin.toml         # Manifest declaring panes, actions (toggle, open, close, restamp), and events
├── config.example.toml       # Documented example settings
└── go.mod / go.sum           # Dependencies: Bubble Tea, Lip Gloss
```

---

## 2. Core Invariants & Architectural Rules

1. **No Timers / Background Polling**:
   - Never add recurring timer ticks or background loops. Subprocess fan-out (`git status` and `worktree.list` across tens of repositories) is too heavy.
   - Refreshes happen **only** on pane open, user keystroke `r`, or local UI mutations (`t`, `p`, `R`).
   - Do not subscribe to Herdr's event backlog (`events.subscribe`), which causes high backlog playback lag.
2. **Paint Before Working**:
   - Herdr displays a blank pane until the process emits its first frame.
   - Bubble Tea `Init()` renders an initial loading state immediately, before dispatching async fetch commands.
3. **Decoupled Asynchronous PR Check**:
   - Pull-request lookups (`gh pr list`) are executed as a secondary pass (`CollectPR`) after initial paint.
   - Merged PR status is immutable; once merged, it is permanently cached in memory for the life of the pane session.
   - Non-GitHub remotes are detected early via `remote.origin.url` and skipped.
4. **Self-Sizing Split Panes**:
   - Herdr's `plugin pane open` split placement does not accept a `--width` argument.
   - The pane reads its own layout via `layout.export` + `pane.layout`, identifies its split node, computes the ratio needed for `pane.width` cells, and invokes `layout.set_split_ratio`.
   - Max 4 sizing attempts (`maxSizeAttempts`) prevent infinite resize loops.
5. **Left-Edge Placement Hack**:
   - Herdr only splits `right` or `down`.
   - To dock on the left, `pane.sh` splits the leftmost pane to the right and immediately calls `herdr pane swap` to place the sidebar on the outer edge, followed by ratio readjustment.
6. **Process Identity via `argv0`**:
   - Panes are identified by `argv0` basename (`herdr-wayfindr`), **never** by process title `name` (which agents and terminals frequently rewrite).
   - One-shot CLI flags (`--resolve-plugin-config`, `--report-tokens`, `--tag`) are excluded from singleton checks in `pane.sh`.
7. **State Separation**:
   - User config (`config.toml`) is strictly read-only for the plugin.
   - Runtime tags and group renames live in `groups.json`.
   - User UI choices (`view`, `sort`) live in `ui.json`.
   - Both files are written atomically via temporary files and rename replacement (`writeAtomic`).
   - Herdr sidebar metadata tokens (`$group`) are ephemeral in Herdr; they must be restamped on startup and on workspace/worktree events.

---

## 3. Grouping Precedence Engine

Defined in `internal/group/resolve.go`, the resolver evaluates each worktree using the first match:
1. **Explicit Tag** (`SourceExplicit`): Assigned manually via `t` key or `--tag`. Keyed by absolute worktree path.
2. **Ticket Key** (`SourceJira`): Regex pattern matching ticket identifiers (e.g. `WAYF-20748`, case-insensitive). Checked first against the branch name; if absent or detached, checked against the Herdr workspace label. The matched key is converted to uppercase.
3. **Branch Pattern** (`SourceBranchPattern`): User-configured regexes with a `(?P<group>...)` named capture group evaluated against the branch.
4. **Shared Name** (`SourceWorktreeName`): Directory basename shared by worktrees across **two or more distinct repositories** (`repo_root`).
5. **Ungrouped** (`SourceNone`): Any unmatched worktree. Displays under `Ungrouped`, defaults to collapsed/folded, and always sorts to the bottom regardless of sort order or direction.

Header renames via `R` or `--rename` override only the display label without altering the underlying group key or member assignment.

---

## 4. UI & Interaction Specification

- **Dual Presentations**:
  - `list`: 40-column tree view with collapsible feature groups.
  - `cards`: Sized to card contents. Displays top-level feature cards; pressing a card drills down into that feature's worktree cards.
- **Card Sizing**: Cards dynamically adjust row capacity based on the widest name in the row (`desiredWidth`), preventing truncated repo names like `credit-decision-srvc` vs `credit-decision-engine`.
- **Card Activation**: Requires two clicks (first click selects/aims, second click on the selected card opens) to prevent mis-clicks during reflows.
- **Sorting (`s`)**: Cycles `alpha asc` -> `alpha desc` -> `status asc` -> `status desc` -> `agent asc` -> `agent desc`. Both features and worktree members are sorted by the active rule.
- **Right-Click Context Menu**:
  - Claimed from Herdr chrome via `herdr.SetPaneRightClick(..., toPane=true)`.
  - Displays `Open`, `Tag`, `Pin`, `Fold`/`Unfold`, and `Rename`.
  - While menu is open, mouse tracking switches from cell motion to all motion (`tea.EnableMouseAllMotion`) for responsive hover selection.
- **Feature Colors**: Computed via FNV-32a hash of the group name to select from a 12-color curated palette, ensuring stable color identity across sessions.

---

## 5. CLI Modes & Developer Runbook

### Build & Test
```bash
# Build binary
go build -o bin/herdr-wayfindr ./cmd/herdr-wayfindr

# Run entire test suite
go test ./...

# Link plugin to local Herdr installation
herdr plugin link ~/wayfindr
```

### CLI Flag Operations
- `herdr-wayfindr` (no flags): Launches TUI sidebar pane.
- `herdr-wayfindr --list`: Dumps plain-text grouping, sources, and PR status.
- `herdr-wayfindr --report-tokens`: Evaluates and pushes `$group` token metadata to Herdr.
- `herdr-wayfindr --tag [name]`: Sets or clears group tag on the focused workspace (or `--workspace <id>`).
- `herdr-wayfindr --rename <name> [label]`: Renames a group header (or clears if label omitted).
- `herdr-wayfindr --resolve-plugin-config`: Prints normalized JSON config for `pane.sh`.
