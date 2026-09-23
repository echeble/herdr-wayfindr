package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// chromeRows is what the pane gives up to the footer and the line kept clear
// under it. The title is counted apart, in Model.titleRows, because it is only
// drawn when it has something to say.
const chromeRows = 2

// markerColumn is the last cell of a group header's fold marker. A click there
// folds; a click past it selects.
const markerColumn = 1

// breadcrumbText is the label inside the button that leads back out of a
// feature — arrow and word together, not just the arrow, so the hand has
// something to aim at wider than a single column.
const breadcrumbText = "‹ Features"

// breadcrumbPad is the space either side of breadcrumbText inside its button,
// so the fill reads as a button's own padding rather than a tight highlight
// on the letters.
const breadcrumbPad = 1

// breadcrumbColumn is the last cell of the drill-down's button, fill
// included. A click at or before it goes back up to the feature grid.
var breadcrumbColumn = lipgloss.Width(breadcrumbText) + 2*breadcrumbPad - 1

// The scroll chevrons, and what the pair of them costs the body in rows.
const (
	chevronUp   = "⌃"
	chevronDown = "⌄"
	chevronRows = 2
)

// defaultWidth is what the pane is assumed to be before the first window event
// arrives, so a frame drawn in that gap is still laid out sensibly.
const defaultWidth = 30

// wheelRows is how far one wheel notch moves the cursor.
const wheelRows = 3

var (
	titleStyle   = lipgloss.NewStyle().Bold(true)
	derivedStyle = lipgloss.NewStyle().Faint(true)
	closedStyle  = lipgloss.NewStyle().Faint(true)
	selfStyle    = lipgloss.NewStyle().Faint(true)
	cursorStyle  = lipgloss.NewStyle().Reverse(true)
	footerStyle  = lipgloss.NewStyle().Faint(true)
	// keyStyle lifts a key out of the faint footer run so it reads as something
	// to press. 215 is a warm peach-orange, bold so the key itself — not just
	// its colour — is what catches the eye against the faint label beside it.
	keyStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("215"))
	// breadcrumbStyle fills the back button with the same peach a key is drawn
	// in, dark text on top of it, so it reads as a thing to press rather than
	// coloured text sitting on the pane's own background.
	breadcrumbStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("215"))
	errorStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
	dirtyStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("3"))
	// scrollStyle marks what is off screen. It fills the same peach a
	// keybinding is drawn in — the one colour in the footer already trained to
	// mean "press this" — dark text on top of it the same way the breadcrumb
	// button is, so a chevron reads as something to notice rather than one more
	// dim line lost among the pane's own idle grey.
	scrollStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("215"))
	// menuBorderColour and menuSelectedStyle match Herdr's own pane menu — the
	// one "Send right-clicks to pane" sits in — rather than the list's plain
	// reverse-video cursor, so the right-click menu reads as one more instance
	// of the same context menu Herdr already draws, not a component of its own.
	menuBorderColour  = lipgloss.Color("63")
	menuSelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("63"))
)

// dashedBorder is a rounded box drawn in dash runes rather than a solid line
// — the weight an idle card sits in, so a glance across the grid finds the
// one solid, selected frame among the rest without any of them needing colour
// spent on them just to make that true.
var dashedBorder = lipgloss.Border{
	Top:         "┄",
	Bottom:      "┄",
	Left:        "┆",
	Right:       "┆",
	TopLeft:     "╭",
	TopRight:    "╮",
	BottomLeft:  "╰",
	BottomRight: "╯",
}

// statusGlyph maps an agent state to one column. Blocked is the only one worth
// a shape that stands out at a glance.
func statusGlyph(status string, open bool) string {
	if !open {
		return "◌"
	}

	switch status {
	case herdr.StatusBlocked:
		return "◆"
	case herdr.StatusWorking:
		return "●"
	case herdr.StatusDone:
		return "✔"
	case herdr.StatusIdle:
		return "○"
	default:
		return "·"
	}
}

func (m Model) listHeight() int {
	h := m.height - chromeRows - m.titleRows()
	if h < 1 {
		return 1
	}

	return h
}

// titleRows is how many lines the title takes, and so the offset between a
// screen line and a row index. It is none at the top level: Herdr already
// labels the pane "Features" in its own chrome, and a second copy of that
// inside the pane says nothing while costing a row of a sidebar that rarely
// has one to spare. Inside a feature the line earns its place — it is the
// trail back out.
func (m Model) titleRows() int {
	if m.view == viewCards && m.feature != "" {
		return 1
	}

	return 0
}

func (m Model) View() string {
	width := m.width
	if width <= 0 {
		width = defaultWidth
	}

	var b strings.Builder

	if m.titleRows() > 0 {
		b.WriteString(m.title(width))
		b.WriteString("\n")
	}

	if m.view == viewCards {
		b.WriteString(m.cardBody(width))
	} else {
		b.WriteString(m.body(width))
	}

	b.WriteString("\n")
	b.WriteString(m.footer(width))

	out := b.String()

	if m.mode == modeMenu {
		out = m.overlayMenu(out)
	}

	return out
}

// title is the trail back out of a feature. It is the only thing the title line
// is ever spent on — see Model.titleRows — and a filled button on one side
// against the feature's own name run out to the other reads as two different
// things: a place to go back to, and the place you are now.
func (m Model) title(width int) string {
	pad := strings.Repeat(" ", breadcrumbPad)
	button := breadcrumbStyle.Render(pad + breadcrumbText + pad)
	name := groupStyleFor(m.feature).Render(m.labelFor(m.feature))

	gap := width - lipgloss.Width(button) - lipgloss.Width(name)
	if gap < 1 {
		gap = 1
	}

	return truncate(button+strings.Repeat(" ", gap)+name, width)
}

// withChevrons frames the content with the scroll hints and hands back a block
// of exactly listHeight lines, so the footer stays pinned wherever the pane is
// scrolled to.
//
// Both lines are reserved together the moment anything overflows, and kept
// whichever way it can actually scroll. Reserving only the one that is in use
// would move every row up or down a line each time you reached an end, which
// reads as the pane twitching.
func (m Model) withChevrons(content string, width int) string {
	if !m.overflows() {
		return pad(content, m.listHeight())
	}

	above, below := m.scrollable()

	// The chevrons hug the content rather than the pane: a card row rarely
	// divides the body exactly, and a down chevron pinned to the last line of
	// the pane would sit marooned below a band of empty space instead of under
	// the cards it is talking about.
	lines := []string{chevronLine(chevronUp, above, width)}
	lines = append(lines, strings.Split(content, "\n")...)
	lines = append(lines, chevronLine(chevronDown, below, width))

	return pad(strings.Join(lines, "\n"), m.listHeight())
}

// chevronLine centres one chevron on its own line, or leaves the line blank
// when the pane does not go that way. Centred rather than tucked into a corner
// because it is about the pane as a whole, and the middle is where the eye
// already is. The glyph gets a space of padding either side, the same amount
// the breadcrumb button pads its own text, so the fill reads as a small pill
// rather than a single cell of colour.
func chevronLine(glyph string, show bool, width int) string {
	if !show {
		return ""
	}

	pad := strings.Repeat(" ", breadcrumbPad)

	return centre(scrollStyle.Render(pad+glyph+pad), width)
}

func centre(s string, width int) string {
	gap := (width - lipgloss.Width(s)) / 2
	if gap <= 0 {
		return truncate(s, width)
	}

	return strings.Repeat(" ", gap) + s
}

// body is the list itself, or the placeholder shown before the first collection
// lands. Something is always drawn: Herdr leaves a pane blank until its process
// paints, so an empty first frame reads as a broken plugin.
func (m Model) body(width int) string {
	visible := m.contentHeight()

	if !m.loaded {
		return pad(derivedStyle.Render("loading…"), m.listHeight())
	}

	if len(m.rows) == 0 {
		return pad(derivedStyle.Render("no worktrees found"), m.listHeight())
	}

	var lines []string

	end := min(m.top+visible, len(m.rows))
	for i := m.top; i < end; i++ {
		lines = append(lines, m.renderRow(m.rows[i], i == m.cursor, width))
	}

	return m.withChevrons(strings.Join(lines, "\n"), width)
}

func (m Model) renderRow(r row, selected bool, width int) string {
	var line string

	if r.kind == rowGroup {
		line = m.renderGroup(r)
	} else {
		line = m.renderWorktree(r, width)
	}

	line = truncate(line, width)

	if selected {
		// Reverse the whole row width so the cursor reads as a bar rather than
		// a ragged highlight.
		return cursorStyle.Render(padRight(line, width))
	}

	return line
}

func (m Model) renderGroup(r row) string {
	marker := "▾"
	if r.collapsed {
		marker = "▸"
	}

	gStyle := groupStyleFor(r.name)
	dot := statusGlyphStyled(r.status, r.openCount > 0)
	head := fmt.Sprintf("%s %s %s", gStyle.Render(marker), dot, gStyle.Render(r.displayName()))

	if badge := prBadge(r.pr); badge != "" {
		head += " " + badge
	}

	return head
}

func (m Model) renderWorktree(r row, width int) string {
	wt := r.worktree

	name := wt.RepoName
	if name == "" {
		name = wt.Path
	}

	line := fmt.Sprintf("  %s %s", statusGlyphStyled(wt.AgentStatus, wt.Open()), name)

	// The branch is only worth a column when it is not already the group's
	// name — which it is for every ticket-derived group.
	if wt.Branch != "" && !strings.EqualFold(wt.Branch, r.name) {
		line += derivedStyle.Render(" · " + wt.Branch)
	}

	if suffix := statusSuffix(*wt); suffix != "" {
		line += " " + dirtyStyle.Render(suffix)
	}

	if wt.WorkspaceID != "" && wt.WorkspaceID == m.selfWorkspace {
		line += selfStyle.Render(" ·here")
	}

	if !wt.Open() {
		line = closedStyle.Render(stripANSI(line))
	}

	// The row mark is what makes the group's own state legible: with one member
	// marked differently it is obvious which repository is holding the feature
	// open. It is appended past a truncation rather than before one — a long
	// branch name must lose its tail, not the mark, and at 40 columns plenty of
	// rows are long.
	if style, ok := prStyleFor(wt.PR.State); ok {
		mark := " " + prMark(wt.PR.State)

		if room := width - lipgloss.Width(mark); room > 0 {
			line = truncate(line, room) + style.Render(mark)
		}
	}

	return line
}

// statusSuffix is the compact git summary: ahead/behind first, then a mark for
// a dirty tree. Untracked files alone do not earn a mark.
func statusSuffix(wt collect.Worktree) string {
	if !wt.Git.Known {
		return ""
	}

	var parts []string

	if wt.Git.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("↑%d", wt.Git.Ahead))
	}

	if wt.Git.Behind > 0 {
		parts = append(parts, fmt.Sprintf("↓%d", wt.Git.Behind))
	}

	if wt.Git.Conflicts > 0 {
		parts = append(parts, fmt.Sprintf("!%d", wt.Git.Conflicts))
	} else if wt.Git.Dirty() {
		parts = append(parts, fmt.Sprintf("✱%d", wt.Git.Staged+wt.Git.Unstaged))
	}

	return strings.Join(parts, " ")
}

// cardBody draws the grid. Like the list it always fills the body exactly, so
// the footer stays pinned whatever is or is not on screen.
func (m Model) cardBody(width int) string {
	if !m.loaded {
		return pad(derivedStyle.Render("loading…"), m.listHeight())
	}

	if len(m.cards) == 0 {
		return pad(derivedStyle.Render("no worktrees found"), m.listHeight())
	}

	g := m.grid()

	var lines []string

	for i := m.cardTop; i < m.cardTop+g.visible && i < len(g.lines); i++ {
		lines = append(lines, m.cardRow(g.lines[i], g.height))
	}

	return m.withChevrons(strings.Join(lines, "\n"), width)
}

// cardRow lays one row of the grid side by side. The gap is its own block
// rather than a margin on the cards: a margin on the last card would push the
// row a column past the pane and wrap it.
func (m Model) cardRow(line gridLine, height int) string {
	var parts []string

	for c := range line.count {
		if c > 0 {
			parts = append(parts, strings.Repeat(" ", cardGap))
		}

		index := line.start + c
		parts = append(parts, m.renderCard(m.cards[index], index == m.cardCursor, line.width, height))
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, parts...)
}

func (m Model) renderCard(r row, selected bool, width, height int) string {
	inner := max(1, width-cardFrameSize)
	inside := max(1, height-cardFrameSize)

	body := strings.Join(m.cardLines(r, inner, inside), "\n")

	return cardFrame(r, selected).Width(inner).Height(inside).Render(body)
}

// cardFrame is the box a card is drawn in.
// An idle card is framed in dashed grey; a selected one is highlighted with a heavy
// solid border in its feature's colour, so the focused card stands out sharply
// while keeping its interior status colours, agent dots, and PR marks intact.
func cardFrame(r row, selected bool) lipgloss.Style {
	if !selected {
		return lipgloss.NewStyle().
			Border(dashedBorder).
			BorderForeground(idleFrameColour)
	}

	color := lipgloss.Color("215")
	if r.name != ungroupedLabel {
		color = groupColor(r.name)
	}

	return lipgloss.NewStyle().
		Border(lipgloss.ThickBorder()).
		BorderForeground(color)
}

func (m Model) cardLines(r row, width, height int) []string {
	if r.kind == rowGroup {
		return featureCardLines(r, width, height)
	}

	return m.worktreeCardLines(r, width, height)
}

// featureCardLines is the face of a feature: what it is called, how much of it
// there is, how much of it is open, and where its pull requests stand.
func featureCardLines(r row, width, height int) []string {
	head := statusGlyphStyled(r.status, r.openCount > 0) + " " + groupStyleFor(r.name).Render(cardName(r))

	details := []string{
		plural(r.members, "worktree"),
		derivedStyle.Render(fmt.Sprintf("%d open", r.openCount)),
	}

	// A card has room for the word behind the glyph, which a 40-column row
	// never did — and "blocked" is the one thing you want a feature to be able
	// to tell you from across the pane.
	if word := agentWord(r.status, r.agents, r.openCount > 0); word != "" {
		details = append(details, statusWordStyle(r.status).Render(word))
	}

	return cardBlock(head, details, prFoot(r.pr), width, height)
}

// worktreeCardLines is the face of one checkout: the repository it is, the
// branch it is on, and what git and GitHub make of it.
func (m Model) worktreeCardLines(r row, width, height int) []string {
	wt := r.worktree

	// cardName is what the card was sized to fit, so it has to be what the card
	// then draws.
	head := statusGlyphStyled(wt.AgentStatus, wt.Open()) + " " + cardName(r)
	if !wt.Open() {
		head = closedStyle.Render(stripANSI(head))
	}

	var details []string

	// As in the list, the branch is only worth a line when it is not already
	// the name of the feature you drilled into to get here.
	if wt.Branch != "" && !strings.EqualFold(wt.Branch, r.name) {
		details = append(details, derivedStyle.Render(wt.Branch))
	}

	// The count is part of the word now — "2 agents working" — so the trailing
	// "· N agents" this used to carry would say the same thing twice.
	if word := agentWord(wt.AgentStatus, wt.Agents, wt.Open()); word != "" {
		details = append(details, statusWordStyle(wt.AgentStatus).Render(word))
	}

	if suffix := statusSuffix(*wt); suffix != "" {
		details = append(details, dirtyStyle.Render(suffix))
	}

	switch {
	case !wt.Open():
		details = append(details, closedStyle.Render("closed"))
	case wt.WorkspaceID == m.selfWorkspace && wt.WorkspaceID != "":
		details = append(details, selfStyle.Render("·here"))
	}

	return cardBlock(head, details, prFoot(wt.PR.State), width, height)
}

// agentWord is the agent state spelled out with the weight behind it — "2
// agents working" rather than a bare "working" that could be one agent or six.
// The count is of agents in the state being named, not of every agent present:
// a workspace labelled blocked because one agent is blocked says "1 agent
// blocked" even with two others still working, because that is the one the
// label is about.
//
// A closed checkout has no agent state worth printing — "closed" is the state —
// and neither does one Herdr could not answer for.
//
// A tally that cannot account for the state falls back to the bare word. That
// is not an expected shape — a status is derived from the agents counted here —
// but a row that says "working" is a better failure than one that says
// "0 agents working".
func agentWord(status string, agents collect.AgentTally, open bool) string {
	if !open || status == "" || status == herdr.StatusUnknown {
		return ""
	}

	count := agents.Count(status)
	if count == 0 {
		return status
	}

	return plural(count, "agent") + " " + status
}

// prFoot is the pull-request line a card ends on. A card has room for the word
// as well as the mark, which the 40-column list did not.
func prFoot(state collect.PRState) string {
	style, ok := prStyleFor(state)
	if !ok {
		return ""
	}

	return style.Render(prMark(state) + " " + state.String())
}

// cardBlock lays a card's text into exactly height lines of width cells: the
// head on top, details under it, and the footer pinned to the bottom right.
//
// Everything is inset by a column so the text never touches the frame, and a
// card too short for all of it gives up the breathing room first and the
// details after — the head and the pull-request state are what a card is for.
func cardBlock(head string, details []string, foot string, width, height int) []string {
	height = max(1, height)
	text := max(0, width-2)

	reserve := 0
	if foot != "" {
		reserve = 1
	}

	room := max(0, height-1-reserve)
	if len(details) > room {
		details = details[:room]
	}

	lines := make([]string, 0, height)
	lines = append(lines, inset(head, text))

	if len(details) < room {
		lines = append(lines, blank(width))
	}

	for _, d := range details {
		lines = append(lines, inset(d, text))
	}

	for len(lines) < height-reserve {
		lines = append(lines, blank(width))
	}

	if reserve == 1 {
		lines = append(lines, insetRight(foot, text))
	}

	return lines[:min(len(lines), height)]
}

// inset draws one line of card text: a column of margin, the text, and padding
// out to the full card width so the frame stays straight.
func inset(s string, width int) string {
	return " " + padRight(clip(s, width), width) + " "
}

func insetRight(s string, width int) string {
	return " " + padLeft(clip(s, width), width) + " "
}

// clip cuts card text to a width and marks the cut.
//
// A card is narrow enough that repository names routinely lose their tails, and
// a name cut without a mark reads as a different, shorter name —
// marketplace-billing-service and credit-decision-engine both become "credit-decisi".
// The list does not need this: its rows are the full pane wide.
func clip(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}

	if width == 1 {
		return "…"
	}

	return truncate(s, width-1) + "…"
}

func padLeft(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return strings.Repeat(" ", gap) + s
	}

	return s
}

func blank(width int) string {
	return strings.Repeat(" ", max(0, width))
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}

	return fmt.Sprintf("%d %ss", n, noun)
}

func (m Model) footer(width int) string {
	if m.mode == modeMenu {
		hint := keyStyle.Render("↑↓") + footerStyle.Render(" move · ") +
			keyStyle.Render("enter") + footerStyle.Render(" select · ") +
			keyStyle.Render("esc") + footerStyle.Render(" cancel")

		return truncate(hint, width)
	}

	if m.mode == modeTag {
		return truncate("tag: "+m.input+"▏", width)
	}

	if m.mode == modeRename {
		return truncate("rename: "+m.input+"▏", width)
	}

	if m.mode == modeConfirmDelete {
		prompt := errorStyle.Render(fmt.Sprintf("delete %s in %q?", plural(len(m.deleteTargets), "worktree"), m.deleteTargetGroup)) +
			footerStyle.Render(" ") + keyStyle.Render("y") + footerStyle.Render("es · ") +
			keyStyle.Render("n") + footerStyle.Render("o")

		return truncate(prompt, width)
	}

	if m.errNote != "" {
		return truncate(errorStyle.Render(m.errNote), width)
	}

	if m.note != "" {
		return truncate(footerStyle.Render(m.note), width)
	}

	// Nothing refreshes on its own, so the key that does always stays in the
	// legend, down to the narrowest form. Each variant is assembled from
	// already-styled runs rather than wrapped in one Render, because the key's
	// own reset would end a surrounding faint style at that point.
	reload := keyStyle.Render("r") + footerStyle.Render(" refresh")

	// Offer the keys that fit; a 40-column sidebar gets the short form.
	hints := m.hints(reload, m.sortHint())

	for _, hint := range hints {
		if lipgloss.Width(hint) <= width {
			return hint
		}
	}

	return truncate(hints[len(hints)-1], width)
}

// sortHint names the order the pane is in rather than offering a generic "s
// sort". The mode has to be legible at rest: a keystroke that silently reorders
// the list leaves you wondering what you are looking at.
func (m Model) sortHint() string {
	return keyStyle.Render("s") + " " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("141")).Render(m.order.String())
}

// overlayCutHorizon is a column count no rendered line is ever close to
// reaching. ansi.Cut wants a right bound to mean "to the end of the line";
// this stands in for infinity there.
const overlayCutHorizon = 1 << 20

// overlayMenu draws the right-click menu as a bordered box on top of the
// pane's own content, its top-left corner at the pointer itself. Only the
// columns the box itself occupies are touched: whatever was to either side of
// it on a covered line survives — another card sharing that row, on either
// side of this one — spliced back on with ansi.Cut rather than truncated
// away, so a styled neighbour keeps its own colours intact instead of losing
// them to a plain re-pad.
func (m Model) overlayMenu(base string) string {
	items := m.menuItems()
	if len(items) == 0 {
		return base
	}

	x, top, boxWidth, _ := m.menuRect(items)
	box := strings.Split(m.menuBox(items), "\n")

	lines := strings.Split(base, "\n")

	for i, boxLine := range box {
		target := top + i
		if target < 0 || target >= len(lines) {
			continue
		}

		// padRight squares the prefix up to x: a line shorter than that — an
		// empty one below the last row, say — must not pull the box's own
		// left edge in with it.
		before := padRight(ansi.Cut(lines[target], 0, x), x)
		after := ansi.Cut(lines[target], x+boxWidth, overlayCutHorizon)

		lines[target] = before + boxLine + after
	}

	return strings.Join(lines, "\n")
}

// menuRect is the on-screen rectangle the menu box occupies. It exists so
// drawing the overlay and hit-testing a click against it can never disagree
// about where the box actually is.
func (m Model) menuRect(items []menuItem) (x, y, w, h int) {
	if len(items) == 0 {
		return 0, 0, 0, 0
	}

	width := m.width
	if width <= 0 {
		width = defaultWidth
	}

	box := strings.Split(m.menuBox(items), "\n")
	h = len(box)

	for _, line := range box {
		w = max(w, lipgloss.Width(line))
	}

	x = min(m.menuX, max(0, width-w))
	y = max(0, min(m.menuY, max(0, m.frameLines()-h)))

	return x, y, w, h
}

// frameLines is the number of lines View() draws in total: the title, when
// there is one, the body and the footer. Menu placement clamps against it
// rather than against a rendered frame, so a click can be tested before one
// exists.
func (m Model) frameLines() int {
	return m.titleRows() + m.listHeight() + 1
}

// menuBox renders the menu's items into a bordered block, the highlighted one
// reversed the same way the list's own cursor is.
func (m Model) menuBox(items []menuItem) string {
	inner := 8 // Wide enough for "Unfold", the longest label, plus its margins.

	for _, item := range items {
		inner = max(inner, lipgloss.Width(item.label)+2)
	}

	lines := make([]string, len(items))

	for i, item := range items {
		text := padRight(" "+item.label, inner)

		if i == m.menuCursor {
			text = menuSelectedStyle.Render(text)
		}

		lines[i] = text
	}

	return lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		BorderForeground(menuBorderColour).
		Render(strings.Join(lines, "\n"))
}

// hints are the footer legends for the current view, widest first. The keys
// that switch view and order are in every one that has room for them: an
// alternative nobody finds out about is worth nothing.
func (m Model) hints(reload, order string) []string {
	badge := keyStyle.Render("[list]")
	if m.view == viewCards {
		badge = keyStyle.Render("[cards]")
	}

	if m.view != viewCards {
		return []string{
			badge + " " + keyStyle.Render("j/k") + footerStyle.Render(" move · ") + keyStyle.Render("enter") + footerStyle.Render(" jump · ") + keyStyle.Render("space") + footerStyle.Render(" fold · ") + keyStyle.Render("t") + footerStyle.Render(" tag · ") + keyStyle.Render("p") + footerStyle.Render(" pin · ") + keyStyle.Render("R") + footerStyle.Render(" rename · ") + keyStyle.Render("v") + footerStyle.Render(" cards · ") + keyStyle.Render("q") + footerStyle.Render(" close · ") + order + footerStyle.Render(" · ") + reload,
			badge + " " + keyStyle.Render("enter") + footerStyle.Render(" jump · ") + keyStyle.Render("t") + footerStyle.Render(" tag · ") + keyStyle.Render("v") + footerStyle.Render(" cards · ") + order + footerStyle.Render(" · ") + reload,
			badge + " " + order + footerStyle.Render(" · ") + keyStyle.Render("v") + footerStyle.Render(" cards · ") + reload,
			badge + " " + order + footerStyle.Render(" · ") + reload,
			order + footerStyle.Render(" · ") + reload,
			reload,
		}
	}

	if m.feature != "" {
		// Inside a feature the sort is the one the grid behind it is in, and it
		// is not what you are deciding here: the way back matters more. Rename
		// belongs to the feature itself, not to the worktrees inside it, so it
		// is not offered here either — back out to the grid for that.
		return []string{
			badge + " " + keyStyle.Render("←/→") + footerStyle.Render(" move · ") + keyStyle.Render("enter") + footerStyle.Render(" open · ") + keyStyle.Render("esc") + footerStyle.Render(" back · ") + keyStyle.Render("t") + footerStyle.Render(" tag · ") + keyStyle.Render("v") + footerStyle.Render(" list · ") + reload,
			badge + " " + keyStyle.Render("enter") + footerStyle.Render(" open · ") + keyStyle.Render("esc") + footerStyle.Render(" back · ") + keyStyle.Render("v") + footerStyle.Render(" list · ") + reload,
			badge + " " + keyStyle.Render("enter") + footerStyle.Render(" open · ") + keyStyle.Render("esc") + footerStyle.Render(" back · ") + reload,
			badge + " " + keyStyle.Render("esc") + footerStyle.Render(" back · ") + reload,
			keyStyle.Render("esc") + footerStyle.Render(" back · ") + reload,
			reload,
		}
	}

	return []string{
		badge + " " + keyStyle.Render("←/→") + footerStyle.Render(" move · ") + keyStyle.Render("enter") + footerStyle.Render(" open · ") + keyStyle.Render("R") + footerStyle.Render(" rename · ") + keyStyle.Render("v") + footerStyle.Render(" list · ") + keyStyle.Render("q") + footerStyle.Render(" close · ") + order + footerStyle.Render(" · ") + reload,
		badge + " " + keyStyle.Render("enter") + footerStyle.Render(" open · ") + keyStyle.Render("v") + footerStyle.Render(" list · ") + order + footerStyle.Render(" · ") + reload,
		badge + " " + order + footerStyle.Render(" · ") + keyStyle.Render("v") + footerStyle.Render(" list · ") + reload,
		badge + " " + order + footerStyle.Render(" · ") + reload,
		order + footerStyle.Render(" · ") + reload,
		reload,
	}
}

// pad grows a block to exactly n lines, so the footer stays pinned to the
// bottom instead of walking up the pane as the list shrinks.
func pad(s string, n int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < n {
		lines = append(lines, "")
	}

	return strings.Join(lines[:n], "\n")
}

func padRight(s string, width int) string {
	if gap := width - lipgloss.Width(s); gap > 0 {
		return s + strings.Repeat(" ", gap)
	}

	return s
}

// truncate cuts a styled string to a display width, counting cells rather than
// bytes so a glyph or a CJK path does not overflow the pane.
func truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}

	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}

// stripANSI removes styling so a whole row can be re-styled as faint. Nested
// styles would otherwise end at the first inner reset.
func stripANSI(s string) string {
	var (
		b        strings.Builder
		inEscape bool
	)

	for _, r := range s {
		switch {
		case inEscape:
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEscape = false
			}
		case r == '\x1b':
			inEscape = true
		default:
			b.WriteRune(r)
		}
	}

	return b.String()
}
