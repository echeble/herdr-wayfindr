# Wayfindr

![Go](https://img.shields.io/badge/go-1.24%2B-00ADD8.svg)
![herdr 0.9.0+](https://img.shields.io/badge/herdr-0.9.0%2B-8a2be2)
![wayfindr v0.19.0](https://img.shields.io/badge/wayfindr-v0.19.0-blue)
![platforms: linux, macOS](https://img.shields.io/badge/platforms-linux%2C%20macOS-informational)

**Group worktrees across every workspace, not just within one repository.** A [Herdr](https://herdr.dev)
plugin: it lists every worktree Herdr knows about, groups them by feature instead of by repo, and
lets you jump between them from a sidebar pane.

<details>
<summary><strong>Interface</strong></summary>

**List** — one row per worktree, grouped by feature, colour-coded, sorted the way you left it.

```
▾ ○ WAYF-20226 in progress
  ◌ marketplace-catalog-service · worktree-WAYF-20226_remove_deprecat ✔
  ◌ marketplace-billing-service ✱4
  · marketplace-billing-service · worktree-WAYF-20226 ⧗
▾ ○ WAYF-20748 merged
  · marketplace-api-gateway ✔
  · marketplace-billing-service ↑2 ✔
▸ ○ Ungrouped
```

**Cards** — two screens of buttons: a card per feature, then a card per worktree in it.

```
‹ Features                     WAYF-20748
┏━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┓
┃ ● marketplace-billing-service        ┃
┃                                      ┃
┃ 2 agents working                     ┃
┃ ↑3 ✱3                                ┃
┃ ·here                                ┃
┃                        ⧗ in progress ┃
┗━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━┛
╭──────────────────────────────────────╮
│ ◆ marketplace-catalog-service        │
│                                      │
│ worktree-WAYF-20748                  │
│ 1 agent blocked                      │
│                                      │
│                             ✔ merged │
╰──────────────────────────────────────╯
```

It lists **every worktree Herdr knows about**, in every repository — including checkouts that
exist on disk but are not open in Herdr, and ones other tools created. Those render dimmed with a
`◌`; press Enter and the plugin opens the worktree as a workspace before focusing it. Each group
gets its own colour, derived from a hash of its name — so a feature keeps the same colour every
time you open the panel.

</details>

## Why wayfindr?

- **Grouped by feature, not by repo.** A ticket that spans a service, an engine, the k8s chart, and
  the test suite scatters across as many repo groups in Herdr's own sidebar; wayfindr puts them
  back together under one heading.
- **Two views, one state.** `v` switches between a 40-column list and a touch-friendly card grid —
  the same sort, the same groups, the way you last left it.
- **Sized to the name, not a fixed grid.** A card asks for the width its name needs; rows pack as
  many cards as the pane can hold at that width, so `marketplace-billing-service` never clips to
  `credit-deci…` next to a card that had room to spare.
- **One sort, everywhere.** `s` cycles `alpha`, `status`, `agent`, each ascending then descending —
  at both the feature level and the worktree level, so "what should I look at first" has one answer.
- **Pull-request state without polling.** A feature is marked `merged` only when every worktree
  under it has a merged PR; `in progress` and `closed` fill in the rest. Runs as a second pass
  after the first paint, and a merge is remembered for the life of the pane.
- **Agents counted, not just named.** A card says `2 agents working` rather than `working`, and
  counts the agents in the state it is naming — a feature labelled blocked because one agent is
  says `1 agent blocked` even with two others still going.
- **One sidebar, not one per workspace.** Opening it closes the one left behind elsewhere — there is
  one at a time and it follows you.
- **Nothing refreshes on a timer.** No background poll, no subprocess churn between keystrokes —
  the list is collected on open and on `r`, full stop.

## How it works

A single binary — `herdr-wayfindr` — runs as a Herdr plugin pane, calls `worktree.list` once per
distinct repository, groups the results, and draws either presentation:

```text
┌─────────┐
│ sidebar │   Bubble Tea pane — list or cards, v switches
└────┬────┘
     │
┌────┴───────┐
│herdr-      │   binary — collect, group, sort worktrees
│wayfindr    │
└────┬───────┘
     │
┌────┴────┬─────────┬────────┐
│  herdr  │   git   │ GitHub │
└─────────┴─────────┴────────┘
```

- **Collection** calls Herdr's `worktree.list` once per distinct `repo_root` found in the session —
  never a global scan — then applies the grouping rules in order (explicit tag, ticket key, branch
  pattern, shared name) and the current sort.
- **Drawing** happens once on open and again on `r`; there is no background loop and no event
  subscription (see [Notes for anyone hacking on it](#notes-for-anyone-hacking-on-it)).
- **Pull-request state** is a second pass behind the first paint: a GitHub GraphQL query per branch, with
  merged answers cached for the life of the pane.

## Two views

The list above is one of two presentations. `v` switches between them, and the
pane opens the way you last left it.

**Cards** trades the tree for two screens of buttons. The first is one card per
feature; pressing one opens a card per worktree in it; pressing one of those
opens the checkout. With the mouse a card takes two clicks: the first selects
it, and a second click on that same selected card is the press.

**Cards are sized to their names.** A card that cannot show which repository it
is has failed at the one job it has — `marketplace-billing-service` and
`credit-decision-engine` both clip to `credit-deci…` — so a card asks for the
width its name needs, and a row takes as many cards as it can while still
showing the longest name among them. The rest move down. Above, two
twenty-plus-character names cannot share forty columns, so each gets a row;
short names still pack two or three across exactly as before.

A row's cards are all drawn at the width of the widest name in it, and sized by
how many cards of that width the pane *would* hold rather than how many the row
ended up with — so a last row of one sits at the same width as the full rows
above it instead of stretching across the pane.

Card height is uniform and follows the pane, not the row: about half the width
of a nominal card, which is square because a terminal cell is about twice as
tall as it is wide. Rows of different heights would scroll unevenly and be
harder to point at.

Idle frames are grey; the selected card is the only coloured one, drawn in its
feature's colour and a heavier border. The colour is not lost on the others —
each card's title already carries it — and spending it on every frame only made
the selected one compete with eleven others. The weight of the border says the
same thing again, so the selection still reads where there is no colour at all.

Cards have room the 40-column list does not, so they spend it: the agent line
behind the status glyph, and `in progress` or `merged` spelled out where a row
gets one cell. **Reading a row** below is the whole legend for both views. A
branch or a name too long even for a full-width card still ends in a `…`, so a
clipped one never reads as a shorter name. A worktree git cannot name is titled
by its folder rather than its whole path — the path would size the card to the
width of the screen. The title line becomes the trail back — the `‹ Features` button clicks
back, as does `esc`.

Which to use is a question of what you are doing. The list answers "what is the
state of everything"; the cards answer "take me to the thing I am working on"
in two presses without reading a tree.

## Reading a row

Every worktree row is the same six parts in the same order. A card is those
same parts unpacked onto their own lines, with room to spell out the ones the
list can only afford a glyph for.

```
  ● marketplace-billing-service · worktree-WAYF-20748 ↑3 ✱3 ·here ⧗
  1 2                             3                   4     5     6
```

1. **the agent** — what is running in this worktree's workspace
2. **the repository** — or the folder name, when git cannot name it
3. **the branch** — only when it is not already the feature's own name, which
   it is for every ticket-derived group, so the column is not spent repeating
   the heading you are sitting under
4. **git** — how this checkout stands against its remote and its own index
5. **`·here`** — this is the worktree the sidebar itself is running in
6. **the pull request** — where the branch stands on GitHub

**The leading glyph is the agent state.** On a feature it is the most urgent
state among that feature's worktrees, so a blocked agent anywhere is visible
with the group folded.

| | |
|---|---|
| `◆` | blocked — an agent wants you now |
| `●` | working |
| `✔` | done |
| `○` | idle |
| `·` | open in Herdr, but no agent state to report |
| `◌` | not open in Herdr at all — a checkout on disk, or one another tool made |

A `◌` row is dimmed whole. Pressing Enter on it opens the worktree as a
workspace first, then focuses it.

**The trailing mark is the pull request**, the same three states the feature
line spells out in words — `✔` merged, `⧗` in progress, `✕` closed. A branch
with no pull request, and one that could not be asked about, are both simply
absent rather than marked.

> `✔` means two different things depending on where it sits. Leading, it is an
> agent that finished; trailing, it is a pull request that merged. They are
> never in the same column, and a row can carry both at once — an agent done
> with work that has also landed.

**Between them is git**, and nothing at all when the tree is clean or git could
not be read — a quiet row is a row with nothing to say.

| | |
|---|---|
| `↑3` | 3 commits ahead of the remote |
| `↓2` | 2 behind |
| `✱3` | 3 changed paths, staged and unstaged together |
| `!2` | 2 conflicted paths — this replaces `✱` rather than joining it |

Untracked files alone earn no mark: a build directory nobody has told git about
is not a dirty tree worth flagging on every refresh.

**A group line** is the same shape one level up — `▾` open or `▸` folded, the
most urgent agent state under it, the feature's name, and its pull-request state
spelled out. A feature with nothing open in Herdr takes the `◌` for the same
reason a worktree does.

**A card** spends the room the 40-column list does not have:

```
╭──────────────────────────────────────╮
│ ◆ marketplace-catalog-service        │   the same leading glyph
│                                      │
│ worktree-WAYF-20748                  │   the branch
│ 1 agent blocked                      │   the agent line, counted
│ ↑3 ✱3                                │   the same git summary
│                                      │
│                        ⧗ in progress │   the pull request, in words
╰──────────────────────────────────────╯
```

**The agent line counts.** `2 agents working` rather than a bare `working`,
which could be one agent or six. The count is of agents **in the state being
named**, not of every agent present: a worktree labelled blocked because one of
its three agents is blocked reads `1 agent blocked`, because that one is what
the label is about. A feature card sums this across every worktree under it.

A feature card also carries `N worktrees` and `N open` — how many checkouts the
feature spans, and how many of those Herdr currently has open as workspaces.
That `open` counts workspaces and has nothing to do with the pull-request state
below it, which is why the pull request says `in progress` rather than `open`.

## Install

```bash
herdr plugin install Credify/llm-context/plugins/wayfindr
```

Open the sidebar:

```bash
herdr plugin action invoke toggle --plugin echeble.wayfindr
```

<details>
<summary><strong>Set up a keybinding to toggle the sidebar from anywhere</strong></summary>

Not configured automatically — add it to your Herdr config at
`~/.config/herdr/config.toml`. The command is `<plugin_id>.<action_id>`:

```toml
[[keys.command]]
key = "cmd+shift+f"
type = "plugin_action"
command = "echeble.wayfindr.toggle"
description = "toggle the feature sidebar"
```

`f` for features. `cmd+shift+f` sits outside Herdr's prefix-key namespace, so
it doesn't collide with any `prefix+...` binding — `herdr --default-config`
lists every binding if you want to verify it's free on your own config before
binding it.

**The group on Herdr's own sidebar rows.** The plugin publishes each
workspace's group as a `group` metadata token; add `$group` to the row template
and every space shows which feature it belongs to, without the pane open:

```toml
[ui.sidebar.spaces]
rows = [["state_icon", "workspace"], ["branch", "git_status"], ["$group"]]
```

Then `herdr server reload-config`.

</details>

<details>
<summary><strong>Requirements and local development</strong></summary>

- Herdr 0.9.0 or newer (socket protocol as shipped in that release)
- Go 1.24+ to build
- `jq` and `git` on the system (macOS and Linux)
- `GITHUB_TOKEN`, `GH_TOKEN`, or a `git credential` helper for the `merged` marker — without it everything else still
  works and no group is ever marked

`plugin link` does not run the build step, so build first:

```bash
git clone <this repo> ~/wayfindr
cd ~/wayfindr
go build -o bin/herdr-wayfindr ./cmd/herdr-wayfindr
herdr plugin link ~/wayfindr
```

Check it registered cleanly — `warnings` should be empty:

```bash
herdr plugin list --plugin echeble.wayfindr --json
```

</details>

## Quickstart

1. Install the plugin and, optionally, bind a key to `echeble.wayfindr.toggle`.
2. Open it — the pane appears as a 40-column split against the left edge of
   the workspace, listing every worktree Herdr knows about, grouped by feature.
3. Press `s` to cycle the sort, `space` to fold or unfold a group, `v` to
   switch to cards.
4. Press `Enter` on a worktree to jump to it, opening it as a workspace first
   if it isn't one already.
5. Press `t` to tag a worktree into a group by hand, or `p` to pin a derived
   group so it stops depending on the branch name.
6. Press `R` on a group header to rename it — the header's own text only, not
   the tag or the rule that put its worktrees there.

## Demos

Real recordings of the pane — keys and mouse events written into a pty, one
frame per redraw, not a reconstruction. Each capability is shown in both
presentations, because `v` is a switch between two views of the same state and
not two different tools.

<details>
<summary><strong>Reading the sidebar</strong></summary>

Nothing to press: this is what the pane looks like the moment it opens. Every
worktree Herdr knows about, in every repository, grouped by the feature it
belongs to — and the ones no workspace has open, dimmed with a `◌`.

The leading glyph is the agent state (`◆` blocked, `●` working, `✔` done, `○`
idle, `·` open with nothing to report). Then the repository, the branch when it
is not already the feature's name, the git summary (`✱2` — two changed paths),
`·here` if it is the worktree the pane itself is running in, and the pull
request at the end (`⧗` in progress, `✔` merged, `✕` closed). A feature reads
merged only once every worktree under it has.

`Ungrouped` starts folded — `space` on it looks inside.

| List | Cards |
|---|---|
| <img src="docs/assets/01-grouping.svg" width="420" alt="The list at rest, grouped by feature"> | <img src="docs/assets/01-grouping-cards.svg" width="420" alt="The same grouping as a grid of feature cards"> |

</details>

<details>
<summary><strong>Moving around</strong></summary>

`j` / `k` move a row at a time; `g` and `G` jump to the top and bottom. `space`
folds or unfolds the group you are on. The arrow keys work everywhere `hjkl`
do.

In cards, `h` / `l` step between cards in a row and `j` / `k` between rows, and
`space` presses the card rather than folding anything.

| List | Cards |
|---|---|
| <img src="docs/assets/02-navigation.svg" width="420" alt="Moving through the list with j, k, g, G and space"> | <img src="docs/assets/02-navigation-cards.svg" width="420" alt="Moving around the card grid with h, l, j, k"> |

</details>

<details>
<summary><strong>Sorting</strong></summary>

`s` cycles six orders — alpha, status and agent, each ascending then descending
— and the footer names the one you are in. The choice is remembered, so the
pane opens in the order you left it rather than the one in the config.

| List | Cards |
|---|---|
| <img src="docs/assets/03-sorting.svg" width="420" alt="s cycling the six sort orders in the list"> | <img src="docs/assets/03-sorting-cards.svg" width="420" alt="The same six orders reordering the grid"> |

</details>

<details>
<summary><strong>Switching between the two views</strong></summary>

`v` trades the tree for a grid of feature cards. Pressing one opens a card per
worktree in it; pressing one of those opens the checkout. `esc` — or the
`‹ Features` button — backs out a level, and `v` returns to the list.

Cards have room the 40-column list does not, so they spend it: the branch on
its own line, the agent line counted (`1 agent blocked`, not a bare `blocked`),
and the pull request spelled out.

<img src="docs/assets/04-cards.svg" width="420" alt="v switching to cards, then entering a feature">

</details>

<details>
<summary><strong>Mouse</strong></summary>

A left click selects the row under the pointer — it never jumps, because
selecting is what a click does everywhere else and `enter` is one keystroke
away. A click on a group's `▾` folds or unfolds it; a click on the name beside
it only selects. The wheel scrolls three rows a notch.

In cards a click aims and a second one fires: the first click on a card selects
it, and clicking that same card again is the press.

The pane asks Herdr to forward the mouse, which costs the terminal its own
text selection — `pane.mouse = false` hands it back.

<img src="docs/assets/05-mouse.svg" width="420" alt="Clicking a row, folding from the marker, and the wheel">

</details>

<details>
<summary><strong>The right-click menu</strong></summary>

Right click selects what is under the pointer and opens a small boxed menu
there. The items are the actions already bound in browse mode, never a second
way to do anything: **Open** for either kind, **Tag** and **Pin** on a
worktree, **Rename** on a group header or feature card, and **Fold** or
**Unfold** beside it for a group header in the list — a card grid has nothing
to fold, so the entry is absent there.

Moving the pointer highlights an item; `↑`/`↓`, `j`/`k` and the wheel do the
same. `enter` or a left click runs it, `esc` or a click outside cancels.
Right-clicking a different row while a menu is open moves the menu rather than
just closing it.

Herdr's own pane chrome claims right clicks by default, so the pane asks for
them on startup. `pane.right_click = false` leaves them to the terminal.

| List | Cards |
|---|---|
| <img src="docs/assets/06-context-menu.svg" width="420" alt="Right-click menus on a worktree row and a group header"> | <img src="docs/assets/06-context-menu-cards.svg" width="420" alt="Right-click menus on a feature card and a worktree card"> |

</details>

<details>
<summary><strong>Tagging a worktree — <code>t</code></strong></summary>

Select a worktree, press `t`, type a name and press `enter`. The worktree
leaves whatever group the rules derived for it and joins the one you named,
forming it if it did not exist. An empty name clears the tag and lets the
derived group take it back.

An explicit tag beats every other rule and is keyed on the checkout path, so it
survives a branch rename, a Herdr restart, and the pane being closed.

From the cards, press into a feature first — `t` acts on a worktree, and the
feature cards are a level above one.

| List | Cards |
|---|---|
| <img src="docs/assets/07-tag.svg" width="420" alt="Tagging a worktree from the list"> | <img src="docs/assets/07-tag-cards.svg" width="420" alt="Tagging a worktree from inside a feature's cards"> |

</details>

<details>
<summary><strong>Pinning a derived group — <code>p</code></strong></summary>

A derived group is a guess the rules made from a branch name, a workspace name
or a shared folder. `p` on a worktree writes that guess down as an explicit tag
on that checkout, so it stops depending on the guess: rename the branch, move
the folder, or let the other repository that shared its name drift away, and
the worktree stays where you put it.

`p` needs a worktree selected — on a group header it answers
`select a worktree to pin`.

| List | Cards |
|---|---|
| <img src="docs/assets/08-pin.svg" width="420" alt="Pinning a derived group from the list"> | <img src="docs/assets/08-pin-cards.svg" width="420" alt="Pinning from inside a feature's cards"> |

</details>

<details>
<summary><strong>Renaming a group header — <code>R</code></strong></summary>

`R` on a group header, or on a feature card, renames what the heading says.
That is all it changes: the worktrees under it keep whatever tag or rule put
them there, so a renamed `WAYF-20748` still collects the same three checkouts.
An empty name puts the derived heading back.

| List | Cards |
|---|---|
| <img src="docs/assets/09-rename.svg" width="420" alt="Renaming a group header in the list"> | <img src="docs/assets/09-rename-cards.svg" width="420" alt="Renaming a feature card in the grid"> |

</details>

## Everyday controls

| Key | List | Cards |
|-----|------|-------|
| `j` / `k` | move | up / down a row of cards |
| `h` / `l` | | previous / next card |
| `enter` | jump to the worktree | open the feature, or the worktree |
| `v` | switch to cards | switch to the list |
| `s` | cycle the sort | ← |
| `space` | fold / unfold a group | press the card |
| `t` / `p` | tag / pin a group | ← |
| `R` | rename a group header | ← |
| `r` | refresh now | ← |
| `q` / `esc` | close the pane | back out of a feature, then close |

<details>
<summary><strong>Full keyboard, mouse, sorting, and pull-request reference</strong></summary>

### Keys

| Key | List | Cards |
|-----|------|-------|
| `j` / `k` | move | up / down a row of cards |
| `h` / `l` | | previous / next card |
| `g` / `G` | top / bottom | first / last card |
| `space` | fold or unfold a group — `Ungrouped` starts folded | press the card |
| `enter` | jump to the worktree — opening it as a workspace first if needed | open the feature, or the worktree |
| `esc` | close the pane | back out of a feature, then close the pane |
| `v` | switch to cards | switch to the list |
| `s` | cycle the sort: alpha, status, agent, each ascending then descending | ← |
| `t` | tag the selected worktree into a group (empty clears the tag) | ← |
| `p` | pin the derived group as an explicit tag | ← |
| `R` | rename the selected group's header (empty resets it) | ← |
| `r` | refresh now | ← |
| `q` | close the pane | ← |

The arrow keys work everywhere `hjkl` do. On a group header in the list, `enter`
jumps to the first member that is already open; on a feature card it opens the
feature instead.

### Mouse

In the **list**, left click selects the row under the pointer, a click on a
group's `▾` folds or unfolds it, and the wheel scrolls. A click never jumps —
selecting is what a click does everywhere else, and `enter` is one keystroke
away.

In **cards** a click aims and a second one fires: the first click on a card
selects it, and clicking that same selected card again is the press — on a
feature it opens that feature, on a worktree it opens the checkout. Anything
that moves the selection another way — a key, a resize, a re-sort — puts the aim
back, so a card the cursor merely happens to sit on still takes two clicks.
A click on the `‹ Features` button goes back straight away, and the wheel scrolls a
row of cards at a time.

Right click selects the row or card under the pointer and opens a small boxed
menu with its top-left corner at the pointer itself: Open for either kind, Tag
and Pin for a worktree, Rename for a group header or feature card, and Fold or
Unfold besides it for a group header in the list. Moving
the pointer over an item highlights it, `↑`/`↓`, `j`/`k` and the wheel do the
same, `enter` or a left click on an item runs it, and `esc` or a click outside
the box cancels. It is the same actions already bound in browse mode, not a
second way to do anything. Right-clicking a different row or card while a menu
is already open moves it there instead of just closing the old one — a second
click to get to what you actually meant to open is never needed.

Hovering only works while the menu is open: the pane reports every mouse move
for as long as one is up, and goes back to reporting just clicks and the
wheel the moment it closes, so browsing the list never pays for traffic it
has no use for.

Herdr's own pane chrome intercepts right clicks by default and shows its own
menu — rename, split, zoom, close — so the pane claims them for itself once,
on startup, the same thing that menu's own "Send right-clicks to pane" entry
does by hand. If the claim did not reach Herdr for some reason, that entry is
the fallback.

Herdr forwards the mouse to a pane that asks for it, which this pane does. The
cost is that the terminal can no longer select text in it the ordinary way, so
`pane.mouse = false` hands the mouse back. Right click is its own switch,
`pane.right_click`, for a terminal whose own right click already does
something — paste, say — that this pane would otherwise take over.

### Sorting

`s` cycles the order, in both views at once and at both levels — the features
and the worktrees inside them are sorted by the same rule, so the answer to
"what should I look at first" is the same wherever you are. The footer names the
current order rather than offering a generic `s sort`: a keystroke that
silently reorders the pane leaves you wondering what you are looking at.

Each mode has both directions, and `s` walks them in turn — `alpha asc`,
`alpha desc`, `status asc`, `status desc`, `agent asc`, `agent desc`, and back
round.

| Order | ascending | descending |
|-------|-----------|------------|
| `alpha` | by name, A first | Z first |
| `status` | most urgent agent first: blocked, working, done, idle | calmest first |
| `agent` | most recently active first | longest untouched first |

**Ascending is each mode's useful end rather than its numerically smaller one.**
That is what you want when you pick a mode at all — you reach for `status`
because something is blocked, not because something is idle. Descending is the
exact reverse of it, which is how you ask the other question: what is furthest
along, what has gone quiet.

Alphabetical is also the tie-break under every other mode, so two features
nothing distinguishes never swap places between refreshes.

`Ungrouped` sinks to the bottom in every mode and both directions. It is the
bucket you look at last by definition, and reversing the sort is not a reason to
bury the real features under it.

**`agent` is an ordering, not a clock.** Herdr timestamps nothing, so this rides
on the `state_change_seq` counter each agent carries — it rises every time one
changes state, which is enough to rank them and nothing more. A feature with no
agent running sorts below every feature that has one, and the whole ordering
restarts with the Herdr server. It costs nothing: the number is already in the
snapshot the pane collects anyway.

### What is off screen

A chevron appears above the list or grid when there is more above it, and below
when there is more below — centred, because it is about the pane as a whole and
the middle is where the eye already is.

```
                   ⌃
╭─────────────────╮ ┏━━━━━━━━━━━━━━━━━┓
│ ○ WAYF-20099    │ ┃ ○ WAYF-20748    ┃
╰─────────────────╯ ┗━━━━━━━━━━━━━━━━━┛
                   ⌄
```

It is the only thing that says so — the list scrolls without a bar, and the grid
scrolls a card row at a time, so a feature below the fold is otherwise simply
invisible.

Both lines are reserved the moment anything overflows and kept whichever way the
pane can actually scroll, so reaching an end never shifts every row by one. The
chevrons hug the content rather than the edges of the pane: a card row rarely
divides the body exactly, and one pinned to the last line would sit marooned
below a band of empty space instead of under the cards it is talking about.

### Pull-request state

Each group carries the state of its pull requests, in GitHub's own colours:

| | | |
|---|---|---|
| `merged` | purple | **every** worktree under it has a merged pull request — the feature has landed in each repository it touches, and all of those checkouts can go |
| `in progress` | green | at least one is still in review |
| `closed` | red | what is left was closed without merging |

Rows carry the same state one cell wide: `✔` merged, `⧗` still in progress, `✕`
closed. That is what makes the group's word legible — when a feature is not
merged, the marks show which repository is holding it.

The hourglass is `U+29D7`, not the emoji `⌛` `U+231B`: the emoji one is
double-width, so it would shift every row carrying it out of line with the rest,
and it arrives coloured by the font rather than by the palette. Fonts without
`U+29D7` fall back for it exactly as they already do for `✱`.

`merged` needs every member, and one the plugin could not ask about is enough
to withhold it — a branch with no pull request, a repository that is not on
GitHub, or an unauthenticated session. The marker means the work has landed; it
must never mean "probably". Where nothing is known, nothing is drawn.

This costs a GraphQL query per branch, so it runs as a second pass *after* the
list is on screen rather than holding the first paint: the rows appear, the
footer says `checking pull requests…`, and the states arrive a few seconds
later. Merged answers are remembered for the life of the pane — a merge cannot
un-merge — so a later `r` only re-asks about the branches still in flight. Turn
the whole thing off with `pr.enabled = false`.

### Where it sits

The sidebar is a split pane against the **left edge** of the workspace, between
Herdr's own sidebar and your work, 40 columns wide.

Both of those take some doing, because Herdr gives a plugin neither. `plugin
pane open` splits only `right` or `down`, so the script splits the *leftmost*
pane of the current tab and then swaps the two, which puts the sidebar on the
edge. And a split placement accepts no width — only a popup does — so on
startup the pane reads its own layout tree, finds the split holding it,
converts columns into a ratio against that split's width, and moves the
divider. The swap trades widths as well as places, so the sizing runs again
after it; that is bounded, not a loop.

It works from a nested layout too: with two panes already side by side it
adjusts the divider that actually holds the sidebar and leaves the others alone.

`pane.direction = "right"` puts it on the other edge instead, and
`pane.width = 0` leaves the split wherever Herdr put it.

**One sidebar, not one per workspace.** A pane belongs to a workspace, so
toggling it in four workspaces would leave four of them running. Instead,
opening the sidebar **closes the ones left behind elsewhere**: there is one at
a time and it follows you. `close` closes it wherever it is.

That sweep is exact rather than quick — it reads every pane in the session and
identifies ours by `argv0`, the same test used everywhere else, because a
pane's label can be rewritten and closing someone else's pane is much worse
than leaving a second sidebar open. A pane that cannot be probed is left alone.
All of it costs about a third of a second across forty-odd panes.

An `auto_open` from a worktree event never sweeps: a worktree being created
somewhere else must not take the sidebar out of the workspace you are looking
at. `pane.singleton = false` turns the whole thing off and lets them
accumulate.

### Refreshing

**Nothing refreshes on a timer.** The list is collected when the pane opens and
whenever you press `r`; the footer keeps `r refresh` in its legend at every
width, with the key itself coloured, so the one that brings the list up to date
is always in reach.

That is deliberate. A background poll meant a `worktree.list` per repository and
a `git status` per checkout every few seconds — around thirty subprocesses on a
machine with this many worktrees — which was enough work to be felt in the panes
around it. Tagging and pinning regroup in place without any collection at all.

The pull-request pass behind the `merged` markers runs after each collection,
off the update loop, and is the one part of a refresh you can watch arrive.

</details>

## Grouping

The first rule that matches wins.

| # | Rule | Group name |
|---|------|------------|
| 1 | **Explicit tag** — set with `t` in the pane | whatever you typed |
| 2 | **Ticket key** anywhere in the branch, then in the Herdr workspace's name | the key, uppercased |
| 3 | **Branch pattern** — your regexps, first with a `group` capture | the capture |
| 4 | **Shared name** — a checkout directory name used by two or more *different* repositories | the directory name |

Anything left over lands in `Ungrouped`, which always sorts last and opens
folded — it is usually the biggest bucket and the least interesting one. `space`
unfolds it.

Rule 2 deliberately looks for the key anywhere rather than only at the start,
because the same ticket gets spelled several ways: `WAYF-20099`,
`worktree/WAYF-20099` and `worktree-WAYF-20099_fix_thing` are one group. Case is
one of those ways, so `wayf-20099` is the same group as `WAYF-20099` — the name is
uppercased once matched. What keeps that from over-matching is the guard at each
end of the key: `rapid-forest-349e` is a generated worktree name, not the ticket
`FOREST-349`.

The workspace name is the second place rule 2 looks, and only when the branch
holds no key: a worktree on `master`, or a detached one, in a workspace you
named `wayf-20764` still belongs to that feature. The branch is asked first
because it is the worktree's own answer — a workspace can be renamed under it.
A checkout no workspace has open simply has no name to read.

Rule 4 requires two different repositories on purpose. Without that test every
unmatched worktree would become a group of one, which is noise rather than
grouping.

The pane does not print which rule produced a group — at 40 columns the name is
what you scan for. `--list` names it. Press `p` on a row to pin its derived
group as an explicit tag, so it stops depending on the branch name.

**Renaming a group is a separate thing from grouping it.** `R` on a header only
changes the text it is shown under — not the tag, not the ticket key, not which
worktrees land in it. A group renamed "Checkout redesign" is still, underneath,
whatever rule 1–4 produced (`WAYF-1`, say), and still bucketed by that key: a new
worktree on the same branch or with the same tag joins it exactly as before,
now under the friendlier header. Clearing the rename (an empty entry) goes
straight back to the resolved name.

## Configuration

Optional. Copy [`config.example.toml`](config.example.toml) to the path
`herdr plugin config-dir echeble.wayfindr` prints, then edit. Every
setting has a default, so no file means the defaults.

The interesting ones:

- `grouping.jira_pattern` — the ticket key, case-insensitive by default. Replace
  it to match your own scheme; a pattern of your own is used exactly as written,
  so add `(?i)` yourself if you want it to ignore case.
- `grouping.branch_patterns` — extra regexps tried after the ticket key. Each
  needs a capture named `group`, e.g. `"^feature/(?P<group>[^/]+)/"`. These read
  the branch only, not the workspace name.
- `grouping.use_worktree_name` — turn rule 4 off.
- `pane.view` — which presentation a pane that has never been switched opens as:
  `list` (default) or `cards`. Once `v` has been pressed, the remembered choice
  wins and this only seeds a fresh install.
- `pane.placement` — `split` (default), `tab`, `overlay` or `zoomed`.
- `pane.direction` — which edge the sidebar sits against: `left` (default) or
  `right`.
- `pane.width` — columns the sidebar resizes itself to, 40 by default. `0`
  leaves the split wherever Herdr put it.
- `pane.auto_open` — open the sidebar automatically in a workspace born from a
  new worktree. Off by default.
- `pane.mouse` — click to select and wheel to scroll. On by default.
- `pane.right_click` — the row or card under the pointer gets a small context
  menu. On by default, and independent of `pane.mouse`.
- `pane.singleton` — opening the sidebar closes the ones left in other
  workspaces. On by default.
- `pr.enabled` — the pull-request state and the GitHub queries behind it. On by
  default.
- `tokens.enabled` / `tokens.name` — the `$group` token on Herdr's sidebar.

A setting that cannot be honoured — a regexp that does not compile, a pattern
with no `group` capture, an unknown placement — falls back to its default and
reports itself in the pane footer rather than failing.

## Command line

The binary is useful outside the pane. It finds the session socket the same way
Herdr's own client does, so these work from any pane or shell:

```bash
herdr-wayfindr --list                        # the grouping as plain text, with the rule and pull-request state per row
herdr-wayfindr --tag WAYF-20748               # tag the focused workspace's worktree
herdr-wayfindr --tag --workspace wT WAYF-1    # tag a specific workspace
herdr-wayfindr --tag                         # clear that tag
herdr-wayfindr --rename WAYF-1 Checkout redesign  # rename a group's header; WAYF-1 is its --list name
herdr-wayfindr --rename WAYF-1                    # clear that rename
herdr-wayfindr --report-tokens               # restamp the $group tokens now
```

`herdr plugin action invoke restamp --plugin echeble.wayfindr` does the
last one from anywhere.

## Where state lives

Two files, both under `herdr plugin config-dir`'s sibling state directory
(`~/.local/state/herdr/plugins/echeble.wayfindr/`):

- `groups.json` — the explicit tags, keyed by checkout path, because Herdr gives
  worktrees no id and no writable metadata, plus the group renames, keyed by
  the resolved group name they retitle. Tags for a checkout that has vanished
  from disk are pruned automatically; a checkout that is merely closed keeps
  its tag. Renames are not pruned — a group with no worktree in it today keeps
  its header for whenever one lands there again.
- `ui.json` — the view and sort last chosen in the pane.

The second is deliberately not `config.toml`. That file is what you write; this
is what the pane writes back, and a plugin that edited the one you were editing
by hand would be fighting you. `pane.view` seeds a fresh install and `ui.json`
answers from the first keystroke onward. Either file being missing or corrupt
costs what it remembers, never the pane.

Sidebar tokens are **not** persisted — Herdr treats them as display-only state
and forgets them on restart. The plugin restamps them from its `[[startup]]`
hook and on every worktree and workspace event.

## Notes for anyone hacking on it

- **Paint before you work.** Herdr shows a blank pane until the process draws,
  so the first frame goes up before anything is fetched.
- **There is no background loop, and no event subscription either.** Besides the
  cost (see Refreshing), `events.subscribe` replays a large backlog before
  delivering anything live, with no way to skip it and no stream position, so
  changes can arrive seconds late. Collection happens on open and on `r`.
- **A split pane sizes itself.** `plugin pane open` takes `--width` for no
  placement, and `pane resize` speaks in ratio deltas, so the pane reads
  `layout.export` plus `pane.layout`, finds the split holding it, converts
  columns to a ratio against that split's own width, and calls
  `layout.set_split_ratio`. The path is the boolean branch list from the root.
- **Left means open-then-swap.** `--direction` accepts only `right` and `down`.
  `pane swap` exchanges two panes' positions *and their sizes*, so the sizing
  has to run a second time afterwards; it is capped at four attempts so a width
  the window cannot give is abandoned rather than chased.
- **Enumeration is a fan-out.** `worktree.list` is per-repository and refuses a
  cwd outside a git work tree, so the plugin takes the distinct `repo_root`
  values out of the snapshot and calls it once per repo.
- **Pane identity is `argv0`, never `name`.** `name` is the rewritable process
  title, truncated to 15 characters here. `herdr/pane.sh` keys on the `argv0`
  basename and excludes the one-shot flags, which must stay excluded on both
  sides.
- **Close with `herdr pane close`.** `herdr plugin pane close` only knows panes
  in the in-memory registry and refuses a still-live pane after a restart.
