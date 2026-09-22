package ui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// cardModel holds two features: CRD-1 with two worktrees, one of them closed,
// and CRD-2 with one.
func cardModel(width, height int) Model {
	w := world(
		wt("/w/a", "cds", "CRD-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "CRD-1", "", herdr.StatusUnknown),
		wt("/w/c", "k8s", "CRD-2", "w3", herdr.StatusBlocked),
	)

	return cardsOver(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "CRD-1", Source: group.SourceJira},
		"/w/b": {Name: "CRD-1", Source: group.SourceJira},
		"/w/c": {Name: "CRD-2", Source: group.SourceJira},
	}), width, height)
}

// gridModel holds one worktree in each of n features, which is what a grid with
// something to scroll looks like.
func gridModel(features, width, height int) Model {
	var worktrees []collect.Worktree

	assignments := map[string]group.Assignment{}

	for i := range features {
		name := fmt.Sprintf("CRD-%02d", i)
		path := "/w/" + name

		worktrees = append(worktrees, wt(path, "repo-"+name, name, "ws-"+name, herdr.StatusIdle))
		assignments[path] = group.Assignment{Name: name, Source: group.SourceJira}
	}

	return cardsOver(world(worktrees...), assignments, width, height)
}

func cardsOver(w collect.World, assignments map[string]group.Assignment, width, height int) Model {
	m := Model{
		view:        viewCards,
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		width:       width,
		height:      height,
		loaded:      true,
	}

	m.rows = buildRows(w, assignments, m.collapsed, m.order)
	m.rebuildCards()

	return m
}

func press(m Model, key string) Model {
	msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}

	switch key {
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "left":
		msg = tea.KeyMsg{Type: tea.KeyLeft}
	case "right":
		msg = tea.KeyMsg{Type: tea.KeyRight}
	case "down":
		msg = tea.KeyMsg{Type: tea.KeyDown}
	case "up":
		msg = tea.KeyMsg{Type: tea.KeyUp}
	}

	next, _ := m.Update(msg)

	return next.(Model)
}

// named builds n feature cards whose names are all the given length, which is
// what decides how they pack.
func named(n, length int) []row {
	cards := make([]row, 0, n)

	for i := range n {
		name := fmt.Sprintf("%0*d", length, i)
		cards = append(cards, row{kind: rowGroup, name: name[:length]})
	}

	return cards
}

func TestLayoutGridStaysInsideThePane(t *testing.T) {
	// A card row wider than the pane would wrap, and one wrapped row pushes
	// every card below it out of place.
	for width := 8; width <= 200; width++ {
		for _, length := range []int{1, 8, 20, 60} {
			g := layoutGrid(named(9, length), width, 30)

			for _, line := range g.lines {
				if line.count < 1 {
					t.Fatalf("width %d, names of %d: a row with %d cards", width, length, line.count)
				}

				if used := line.count*line.width + cardGap*(line.count-1); used > width {
					t.Errorf("width %d, names of %d: %d cards of %d use %d cells", width, length, line.count, line.width, used)
				}
			}
		}
	}
}

func TestLayoutGridHoldsEveryCardExactlyOnce(t *testing.T) {
	for width := 8; width <= 120; width++ {
		g := layoutGrid(named(9, 20), width, 30)

		next := 0
		for _, line := range g.lines {
			if line.start != next {
				t.Fatalf("width %d: row starts at %d, want %d", width, line.start, next)
			}

			next += line.count
		}

		if next != 9 {
			t.Errorf("width %d: rows hold %d cards, want 9", width, next)
		}
	}
}

func TestLayoutGridFitsTheNameWhenItCan(t *testing.T) {
	// The whole point: a card wide enough to say which repository it is.
	for _, length := range []int{6, 14, 20, 30} {
		g := layoutGrid(named(4, length), 60, 30)

		for _, line := range g.lines {
			if line.width < length+nameOverhead {
				t.Errorf("names of %d: cards are %d wide, want at least %d", length, line.width, length+nameOverhead)
			}
		}
	}
}

func TestLayoutGridPacksShortNamesSideBySide(t *testing.T) {
	// A 40-column sidebar still gets its two-across grid when the names are
	// short — this must not have cost the dense case.
	g := layoutGrid(named(4, 6), 40, 30)

	if g.lines[0].count != 2 {
		t.Fatalf("row holds %d cards, want 2", g.lines[0].count)
	}

	if ratio := float64(g.lines[0].width) / float64(g.height*cellAspect); ratio < 0.8 || ratio > 1.25 {
		t.Errorf("card %dx%d is not square: ratio %.2f", g.lines[0].width, g.height, ratio)
	}
}

func TestLayoutGridMovesALongNameOntoItsOwnRow(t *testing.T) {
	// Two twenty-character names cannot share forty columns, so the second one
	// moves down rather than both clipping to "credit-deci…".
	g := layoutGrid(named(2, 20), 40, 30)

	if len(g.lines) != 2 {
		t.Fatalf("rows = %d, want 2", len(g.lines))
	}

	for i, line := range g.lines {
		if line.count != 1 {
			t.Errorf("row %d holds %d cards, want 1", i, line.count)
		}

		if line.width != 40 {
			t.Errorf("row %d card is %d wide, want the full 40", i, line.width)
		}
	}
}

func TestLayoutGridRowTakesTheWidestNameInIt(t *testing.T) {
	// One long name among short ones costs its row the space, rather than being
	// clipped to fit its neighbours.
	cards := []row{
		{kind: rowGroup, name: "AA"},
		{kind: rowGroup, name: "a-name-of-twenty-two-x"},
		{kind: rowGroup, name: "BB"},
	}

	g := layoutGrid(cards, 40, 30)

	first := g.lines[0]
	if first.count != 1 || first.start != 0 {
		t.Fatalf("first row = %d cards from %d, want 1 from 0", first.count, first.start)
	}
}

func TestLayoutGridNarrowPaneKeepsOneCardPerRow(t *testing.T) {
	g := layoutGrid(named(4, 20), 10, 30)

	for _, line := range g.lines {
		if line.count != 1 || line.width > 10 {
			t.Fatalf("row = %d cards of %d, want 1 no wider than the pane", line.count, line.width)
		}
	}
}

func TestLayoutGridShortPaneShrinksTheCard(t *testing.T) {
	// Better a squat card than one that is never drawn whole.
	g := layoutGrid(named(4, 8), 40, 6)

	if g.height > 6 || g.visible < 1 {
		t.Fatalf("grid = %d rows of %d in 6 lines", g.visible, g.height)
	}
}

func TestCardNameFallsBackToTheFolder(t *testing.T) {
	// A checkout git could not name is still somewhere, and the folder is what
	// the user knows it by — not the whole path, which would size the card to
	// the width of the screen.
	r := row{kind: rowWorktree, worktree: &collect.Worktree{Path: "/Users/someone/Repo/wayfindr"}}

	if got := cardName(r); got != "wayfindr" {
		t.Fatalf("name = %q, want the folder", got)
	}
}

func TestFeatureCardsAreOnePerFeature(t *testing.T) {
	m := cardModel(40, 30)

	if len(m.cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(m.cards))
	}

	if m.cards[0].name != "CRD-1" || m.cards[0].members != 2 || m.cards[0].openCount != 1 {
		t.Fatalf("first card = %+v, want CRD-1 with 2 members and 1 open", m.cards[0])
	}
}

func TestWorktreeCardsAreTheFeaturesMembers(t *testing.T) {
	m := cardModel(40, 30)

	cards := worktreeCards(m.world, m.assignments, "CRD-1", ordering{})
	if len(cards) != 2 {
		t.Fatalf("cards = %d, want 2", len(cards))
	}

	for _, c := range cards {
		if c.kind != rowWorktree || c.worktree == nil {
			t.Fatalf("card = %+v, want a worktree", c)
		}
	}

	if got := worktreeCards(m.world, m.assignments, "nothing-here", ordering{}); got != nil {
		t.Fatalf("an unknown feature answered %d cards, want none", len(got))
	}
}

func TestEnterOnAFeatureCardOpensItsWorktrees(t *testing.T) {
	m := press(cardModel(40, 30), "enter")

	if m.feature != "CRD-1" {
		t.Fatalf("feature = %q, want CRD-1", m.feature)
	}

	if len(m.cards) != 2 || m.cards[0].kind != rowWorktree {
		t.Fatalf("cards = %d of kind %v, want 2 worktrees", len(m.cards), m.cards[0].kind)
	}
}

func TestEscComesBackToTheFeatureYouWereIn(t *testing.T) {
	m := cardModel(40, 30)
	m.cardCursor = 1

	inside := press(m, "enter")
	if inside.feature != "CRD-2" {
		t.Fatalf("feature = %q, want CRD-2", inside.feature)
	}

	back := press(inside, "esc")
	if back.feature != "" {
		t.Fatalf("feature = %q, want the grid", back.feature)
	}

	if back.cardCursor != 1 {
		t.Fatalf("cursor = %d, want 1: coming out should land on the feature you went into", back.cardCursor)
	}
}

func TestEscOnTheFeatureGridClosesThePane(t *testing.T) {
	_, cmd := cardModel(40, 30).Update(tea.KeyMsg{Type: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("esc at the top level should still close the pane")
	}
}

func TestFirstClickOnACardOnlySelectsIt(t *testing.T) {
	m := cardModel(40, 30)
	m.cardCursor = 1

	first := click(m, 5, 2)
	if first.feature != "" {
		t.Fatalf("feature = %q, want the grid: the first click only selects", first.feature)
	}

	if first.cardCursor != 0 {
		t.Fatalf("cursor = %d, want 0: the click should have selected the card under it", first.cardCursor)
	}
}

func TestSecondClickOnTheSelectedCardOpensIt(t *testing.T) {
	m := click(click(cardModel(40, 30), 5, 2), 5, 2)

	if m.feature != "CRD-1" {
		t.Fatalf("feature = %q, want CRD-1 opened by the second click", m.feature)
	}
}

func TestClickingAnotherCardSelectsRatherThanOpens(t *testing.T) {
	// Aiming somewhere else is a new aim, not the press of the first one.
	m := gridModel(12, 40, 30)

	g := m.grid()
	span := g.lines[0].width + cardGap

	second := click(click(m, 0, m.contentTop()), span, m.contentTop())
	if second.feature != "" {
		t.Fatalf("feature = %q, want the grid: a click on a different card only selects", second.feature)
	}

	if second.cardCursor != 1 {
		t.Fatalf("cursor = %d, want 1: the second click should have moved the selection", second.cardCursor)
	}
}

func TestKeyboardSelectionIsNotArmedForAClick(t *testing.T) {
	// The cursor sitting on a card is not the same as having clicked it: a card
	// the keys walked onto still takes a click to select and a click to open.
	m := press(gridModel(12, 40, 30), "right")

	if m.cardCursor != 1 {
		t.Fatalf("cursor = %d, want 1", m.cardCursor)
	}

	g := m.grid()
	span := g.lines[0].width + cardGap

	// The card the cursor walked onto, clicked once.
	if after := click(m, span, m.contentTop()); after.feature != "" {
		t.Fatalf("feature = %q, want the grid", after.feature)
	}
}

func TestClickBetweenTwoCardsIsIgnored(t *testing.T) {
	m := cardModel(40, 30)

	g := m.grid()
	if g.lines[0].count < 2 {
		t.Skip("these names pack one to a row, so there is no gap to click in")
	}

	if _, ok := m.cardAt(g.lines[0].width, m.contentTop()); ok {
		t.Fatal("the gap column between two cards is not a card")
	}

	// The feature grid spends no line on a title, so line 0 is a card there.
	// Inside a feature the same line is the trail back out, and must not be.
	inside := press(m, "enter")
	if _, ok := inside.cardAt(5, 0); ok {
		t.Fatal("the breadcrumb line is not a card")
	}
}

func TestClickPastTheLastCardIsIgnored(t *testing.T) {
	m := cardModel(40, 30)

	g := m.grid()

	// The first line below the last row of cards.
	if _, ok := m.cardAt(5, m.contentTop()+len(g.lines)*g.height); ok {
		t.Fatal("empty space below the last card should not resolve to one")
	}
}

func TestClickOnTheBreadcrumbGoesBack(t *testing.T) {
	inside := press(cardModel(40, 30), "enter")

	if back := click(inside, breadcrumbColumn, 0); back.feature != "" {
		t.Fatalf("feature = %q, want the grid", back.feature)
	}
}

func TestCardAtAccountsForScrolling(t *testing.T) {
	m := gridModel(12, 40, 30)
	m.cardTop = 2

	// Two columns, so the first visible card is the one starting row 2. The
	// content begins below the title and the chevron line reserved above it.
	index, ok := m.cardAt(0, m.contentTop())
	if !ok || index != 4 {
		t.Fatalf("card = %d (%v), want 4: the first visible row is m.cardTop", index, ok)
	}
}

func TestGridMovesInTwoDimensions(t *testing.T) {
	m := gridModel(12, 40, 30)

	if right := press(m, "right"); right.cardCursor != 1 {
		t.Fatalf("right = %d, want 1", right.cardCursor)
	}

	// Down is a whole row of cards, however many that row happens to hold.
	down := press(m, "down")
	if want := m.grid().lines[0].count; down.cardCursor != want {
		t.Fatalf("down = %d, want %d", down.cardCursor, want)
	}

	if up := press(down, "up"); up.cardCursor != 0 {
		t.Fatalf("up = %d, want 0", up.cardCursor)
	}
}

func TestMovingDownOntoAShorterRowClampsAlongIt(t *testing.T) {
	// Rows hold different numbers of cards now, so a column that exists on one
	// row may not exist on the next. Landing on its last card beats landing
	// nowhere.
	cards := []row{
		{kind: rowGroup, name: "aa"},
		{kind: rowGroup, name: "bb"},
		{kind: rowGroup, name: "a-name-of-twenty-two-x"},
	}

	m := cardModel(40, 30)
	m.cards = cards
	m.cardCursor = 1
	m.clampCard()

	g := m.grid()
	if g.lines[0].count != 2 || g.lines[1].count != 1 {
		t.Skipf("rows packed %d then %d, not the 2/1 this checks", g.lines[0].count, g.lines[1].count)
	}

	m.moveCardLine(1)

	if m.cardCursor != 2 {
		t.Fatalf("cursor = %d, want 2: the only card on the row below", m.cardCursor)
	}
}

func TestGridCursorStaysOnACard(t *testing.T) {
	m := gridModel(3, 40, 30)

	for _, key := range []string{"left", "up", "up"} {
		m = press(m, key)
	}

	if m.cardCursor != 0 {
		t.Fatalf("cursor = %d, want 0 after walking off the top", m.cardCursor)
	}

	for range 10 {
		m = press(m, "right")
	}

	if m.cardCursor != 2 {
		t.Fatalf("cursor = %d, want 2 after walking off the end", m.cardCursor)
	}
}

func TestGridScrollsByCardRow(t *testing.T) {
	m := gridModel(12, 40, 30)

	g := m.grid()
	if len(g.lines) <= g.visible {
		t.Skipf("a %d-row grid shows all 12 cards, nothing to scroll", g.visible)
	}

	m.cardCursor = 11
	m.clampCard()

	if want := g.lineOf(11) - g.visible + 1; m.cardTop != want {
		t.Fatalf("top = %d, want %d: the last card has to be on screen", m.cardTop, want)
	}
}

func TestRefreshingAwayTheOpenFeatureFallsBackToTheGrid(t *testing.T) {
	m := press(cardModel(40, 30), "enter")

	// The feature's worktrees are gone — removed, or regrouped by a refresh.
	m.world = world(wt("/w/c", "k8s", "CRD-2", "w3", herdr.StatusBlocked))
	m.assignments = assign(map[string]group.Assignment{"/w/c": {Name: "CRD-2", Source: group.SourceJira}})
	m.rebuildCards()

	if m.feature != "" {
		t.Fatalf("feature = %q, want the grid: its worktrees are gone", m.feature)
	}

	if len(m.cards) != 1 || m.cards[0].name != "CRD-2" {
		t.Fatalf("cards = %+v, want the one remaining feature", m.cards)
	}
}

func TestToggleViewSwitchesBothWays(t *testing.T) {
	m := cardModel(40, 30)

	list := press(m, "v")
	if list.view != viewList {
		t.Fatal("v should switch a card grid to the list")
	}

	if back := press(list, "v"); back.view != viewCards {
		t.Fatal("v should switch the list back to the card grid")
	}
}

func TestToggleViewKeepsEachCursor(t *testing.T) {
	m := cardModel(40, 30)
	m.cursor = 2
	m.cardCursor = 1

	round := press(press(m, "v"), "v")
	if round.cursor != 2 || round.cardCursor != 1 {
		t.Fatalf("cursors = %d/%d, want 2/1: each view keeps its own place", round.cursor, round.cardCursor)
	}
}

func TestIdleCardFramesAreDimmed(t *testing.T) {
	// The selected card has to win the pane at a glance, and it cannot do that
	// against eleven other coloured frames.
	feature := row{kind: rowGroup, name: "CRD-1"}

	idle := cardFrame(feature, false).GetBorderTopForeground()
	if idle != lipgloss.TerminalColor(idleFrameColour) {
		t.Errorf("idle frame = %v, want the grey %v", idle, idleFrameColour)
	}

	if selected := cardFrame(feature, true).GetBorderTopForeground(); selected == idle {
		t.Fatal("the selected frame should not be the same colour as an idle one")
	}

	// And the feature's own colour is still on the card, in its title.
	if !strings.Contains(strings.Join(featureCardLines(feature, 15, 5), ""), "CRD-1") {
		t.Error("the card title should still name the feature")
	}
}

func TestIdleCardFramesAreDimmedForEveryFeature(t *testing.T) {
	// Including the ungrouped bucket, which has no colour of its own to dim.
	for _, name := range []string{"CRD-1", "CRD-2", "release", ungroupedLabel} {
		r := row{kind: rowGroup, name: name}

		if got := cardFrame(r, false).GetBorderTopForeground(); got != lipgloss.TerminalColor(idleFrameColour) {
			t.Errorf("%s: idle frame = %v, want the grey %v", name, got, idleFrameColour)
		}
	}
}

func TestSelectedCardKeepsTheThickerFrame(t *testing.T) {
	// The weight carries the selection as well as the colour, so it still reads
	// where a terminal has no colour at all.
	r := row{kind: rowGroup, name: "CRD-1"}

	if cardFrame(r, true).GetBorderStyle().Top == cardFrame(r, false).GetBorderStyle().Top {
		t.Fatal("the selected card should keep its heavier border")
	}
}

func TestSelectedCardFillsWithItsOwnColour(t *testing.T) {
	// A selected card is a solid block of its feature's colour, not just a
	// coloured outline around an otherwise plain one.
	r := row{kind: rowGroup, name: "CRD-1"}
	style := cardFrame(r, true)

	if bg := style.GetBackground(); bg != lipgloss.TerminalColor(groupColor("CRD-1")) {
		t.Errorf("selected background = %v, want the feature's own colour %v", bg, groupColor("CRD-1"))
	}

	if border := style.GetBorderTopForeground(); border != style.GetBackground() {
		t.Errorf("border colour = %v, want it to match the fill %v", border, style.GetBackground())
	}

	// Idle cards spend no background at all: only the selected one fills.
	if idle := cardFrame(r, false).GetBackground(); idle != (lipgloss.NoColor{}) {
		t.Errorf("idle background = %v, want none", idle)
	}
}

func TestSelectedCardTextIsBlackOnEveryColour(t *testing.T) {
	// The text is stripped of its own colour before this style ever sees it —
	// see renderCard — so a single readable foreground has to carry every
	// feature's card, not just some of them.
	for _, name := range []string{"CRD-1", "CRD-2", "release", ungroupedLabel} {
		r := row{kind: rowGroup, name: name}

		if fg := cardFrame(r, true).GetForeground(); fg != lipgloss.Color("0") {
			t.Errorf("%s: selected foreground = %v, want black", name, fg)
		}
	}
}

func TestSelectedUngroupedCardTakesThePeachAccent(t *testing.T) {
	// Ungrouped is colourless everywhere else — it is the absence of a
	// feature, not one more — but selected it still has to fill with
	// something, and peach is the one hue already spent on "you can act on
	// this" rather than on any feature of its own.
	r := row{kind: rowGroup, name: ungroupedLabel}

	if bg := cardFrame(r, true).GetBackground(); bg != lipgloss.Color("215") {
		t.Errorf("selected ungrouped background = %v, want the peach accent", bg)
	}
}

func TestRenderCardKeepsTheTextReadableEitherWay(t *testing.T) {
	// The selected card runs its body through stripANSI before the frame's own
	// fill and foreground take over — see renderCard — so the visible text has
	// to survive that round trip exactly, whichever card is selected.
	m := cardModel(40, 30)

	if out := m.renderCard(m.cards[0], true, 20, 8); !strings.Contains(out, "CRD-1") {
		t.Errorf("selected card lost its own name, got:\n%s", out)
	}

	if out := m.renderCard(m.cards[1], false, 20, 8); !strings.Contains(out, "CRD-2") {
		t.Errorf("idle card lost its own name, got:\n%s", out)
	}
}

func TestCardBlockFillsExactly(t *testing.T) {
	for _, h := range []int{1, 2, 3, 5, 7} {
		lines := cardBlock("head", []string{"one", "two"}, "⧗ in progress", 17, h)

		if len(lines) != h {
			t.Errorf("height %d: %d lines", h, len(lines))

			continue
		}

		for i, line := range lines {
			if got := lipgloss.Width(line); got != 17 {
				t.Errorf("height %d line %d: %d cells, want 17", h, i, got)
			}
		}
	}
}

func TestCardBlockPinsTheFooterToTheBottom(t *testing.T) {
	lines := cardBlock("head", []string{"one"}, "✔ merged", 20, 6)

	if !strings.Contains(lines[len(lines)-1], "merged") {
		t.Fatalf("last line = %q, want the pull-request state", lines[len(lines)-1])
	}

	// And right-aligned, so it reads as the card's own footer.
	if strings.HasPrefix(strings.TrimPrefix(lines[len(lines)-1], " "), "✔") {
		t.Fatalf("last line = %q, want it right-aligned", lines[len(lines)-1])
	}
}

func TestCardBlockGivesUpDetailsBeforeTheFooter(t *testing.T) {
	// A card too short for everything keeps what it is for: what this is, and
	// where its pull request stands.
	lines := cardBlock("head", []string{"one", "two", "three"}, "✔ merged", 20, 2)

	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2", len(lines))
	}

	if !strings.Contains(lines[0], "head") || !strings.Contains(lines[1], "merged") {
		t.Fatalf("lines = %q, want the head and the footer", lines)
	}
}

func TestCardViewDrawsEveryFeature(t *testing.T) {
	out := cardModel(40, 30).View()

	for _, want := range []string{"CRD-1", "CRD-2", "2 worktrees", "1 open"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
}

func TestCardViewShowsTheBreadcrumbInsideAFeature(t *testing.T) {
	out := press(cardModel(40, 30), "enter").View()

	if !strings.Contains(out, "‹") || !strings.Contains(out, "CRD-1") {
		t.Errorf("expected a breadcrumb back to the grid in:\n%s", out)
	}

	// The worktrees of the feature, not the features.
	if !strings.Contains(out, "cds") || !strings.Contains(out, "bnpl") {
		t.Errorf("expected the feature's worktrees in:\n%s", out)
	}
}

func TestCardViewHeightIsStable(t *testing.T) {
	// The footer is pinned to the bottom in both views, and a grid that changed
	// height as it filled would walk it up the pane.
	empty := Model{view: viewCards, width: 40, height: 20, collapsed: map[string]bool{}}

	loading := strings.Count(empty.View(), "\n")

	if filled := strings.Count(cardModel(40, 20).View(), "\n"); filled != loading {
		t.Fatalf("frame height changed from %d to %d lines", loading+1, filled+1)
	}
}

func TestCardViewNeverOverflowsThePane(t *testing.T) {
	// Every line of the frame has to fit the pane, or the grid wraps.
	for _, width := range []int{20, 34, 40, 61, 100} {
		for _, line := range strings.Split(cardModel(width, 24).View(), "\n") {
			if got := lipgloss.Width(line); got > width {
				t.Errorf("width %d: a line is %d cells wide: %q", width, got, line)
			}
		}
	}
}

func TestCardViewFooterOffersTheListBack(t *testing.T) {
	if !strings.Contains(cardModel(40, 30).footer(40), "list") {
		t.Fatal("the card footer should say how to get back to the list")
	}

	// Inside a feature, the way back up matters more than the way to the list.
	if !strings.Contains(press(cardModel(40, 30), "enter").footer(40), "back") {
		t.Fatal("the drill-down footer should say how to get back to the grid")
	}
}

func TestListFooterOffersTheCards(t *testing.T) {
	m := cardModel(80, 30)
	m.view = viewList

	if !strings.Contains(m.footer(80), "cards") {
		t.Fatal("the list footer should say the card grid is there")
	}
}
