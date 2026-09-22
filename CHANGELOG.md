# Changelog

All notable changes to Wayfindr are recorded here, one entry per plugin
version (`herdr-plugin.toml`'s `version` field), newest first.

## [0.16.0] - 2026-09-22

### Added

- Configurable `hide_principal_branches` (default `true`) and `principal_branches` list in `[grouping]` config.

### Changed

- Exclude repository principal branches (`main`, `master`, and detected default branches) from the Ungrouped folder and cards when unmatched.
- Prevent tagging and pinning principal branches via TUI (`t`/`p`/context menu) and CLI (`--tag`).
- Ignore explicit tags on principal branches during feature resolution.
- Prevent shared worktree name grouping from creating feature groups for principal branches (`master`/`main`).

## [0.15.0] - 2026-09-17

### Added

- A recorded demo per capability, in both the list and card views — sixteen
  animated SVGs under `docs/assets`, each a real recording of the pane
  (keystrokes and mouse events written into a pty, captured one frame per
  redraw), not a reconstruction.
- A collapsible Demos section in the README explaining how to drive each one.

## [0.14.0] - 2026-09-17

### Changed

- The pull-request marker reads "in progress" behind `⧗` instead of `↗`
  open — an arrow read as a link.
- `⧗` is U+29D7, not the double-width `⌛` emoji, which would have shifted
  every row it marks.
- Agent lines count agents by the state they name ("2 agents working"), so
  "1 agent blocked" still holds while others are working.
- README gains a "Reading a row" legend for every glyph in both views.

## [0.13.0] - 2026-09-16

### Added

- `R` renames a group's header without touching its tag or grouping.

### Changed

- Idle cards get a dashed rounded border instead of a solid one.
- The selected card fills solid with its own colour, text reversed to black
  for contrast.
- Scroll chevrons fill with the same peach as a keybinding.

## [0.12.0] - 2026-09-16

### Changed

- Keybindings render bold and peach.
- `‹ Features` becomes a real clickable button instead of static text.

## [0.11.0] - 2026-09-16

### Added

- A right-click context menu, hover-navigable and styled like the rest of
  Herdr.

### Fixed

- README corrections.

## [0.10.0] - 2026-09-16

### Changed

- Renamed the plugin from Feature Organizer to Wayfindr, across the plugin
  id and display name, the binary and Go module path, the install and
  config-dir commands in the README, and the keybinding example. Naming
  only — no behavior change.

## [0.9.0] - 2026-09-16

### Added

- Cards are a two-click surface: the first click on a card selects it, and
  only a second click on that same selected card opens it — a mis-aimed
  click on a small, reflowing grid can no longer open a worktree by
  accident.

### Changed

- Ticket keys match case-insensitively and are grouped under one
  upper-cased name, guarded at each end so a generated name like
  `rapid-forest-349e` isn't mistaken for the ticket `FOREST-349`.
- A worktree with no ticket key in its branch is matched against its Herdr
  workspace's name instead, since a workspace can be renamed under it.
- The pane no longer repeats "Features" as its own title, since Herdr's
  chrome already labels it; the title line now appears only inside a
  feature, carrying the trail back out.

## [0.8.0] - 2026-09-15

### Added

- A card grid alongside the list — one screen of cards per feature, then
  one per worktree in it — where a click is the press rather than a
  selection waiting on `enter`.
- `v` switches between list and card views; `s` cycles the sort order
  (alpha, status, agent — each ascending then descending). Both choices are
  remembered in a new `ui.json` beside `groups.json`.
- Cards size themselves to their names, so two repositories that would
  otherwise clip identically (`marketplace-billing-service` vs
  `credit-decision-engine`) stay distinguishable.

### Fixed

- Idle card frames dim to grey so the selected card is the only coloured
  one.
- `regroup` no longer dereferences a nil store.

## [0.7.0] - 2026-09-15

### Changed

- The footer legend ends with a coloured `r` refresh key, replacing the
  `r · HH:MM:SS` timestamp and the last-collection-time plumbing behind it.

## [0.6.0] - 2026-09-15

### Added

- Initial release, as Feature Organizer: groups worktrees across
  workspaces by explicit tag, ticket key, branch pattern, or shared
  checkout name, in a Herdr sidebar pane.
