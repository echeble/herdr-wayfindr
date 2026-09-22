package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"

	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/store"
)

func TestChevronLineIsCentred(t *testing.T) {
	line := chevronLine(chevronDown, true, 41)

	if !strings.Contains(line, chevronDown) {
		t.Fatalf("line = %q, want the chevron in it", line)
	}

	// The glyph sits in the middle cell, give or take the odd column.
	if at := lipgloss.Width(strings.SplitN(line, chevronDown, 2)[0]); at != 20 {
		t.Errorf("chevron at column %d of 41, want 20", at)
	}
}

func TestChevronLineIsBlankWhenThereIsNowhereToGo(t *testing.T) {
	if got := chevronLine(chevronUp, false, 40); got != "" {
		t.Errorf("line = %q, want it blank", got)
	}
}

func TestListReportsWhatIsOffScreen(t *testing.T) {
	// Twelve features, one row each plus a header, in a pane that cannot hold
	// them.
	m := gridModel(12, 40, 12)
	m.view = viewList

	above, below := m.scrollable()
	if above || !below {
		t.Fatalf("at the top: above=%v below=%v, want false/true", above, below)
	}

	m.cursor = len(m.rows) - 1
	m.clampCursor()

	if above, below = m.scrollable(); !above || below {
		t.Fatalf("at the bottom: above=%v below=%v, want true/false", above, below)
	}

	// Somewhere in the middle, both ways.
	m.cursor = len(m.rows) / 2
	m.clampCursor()

	if above, _ := m.scrollable(); !above {
		t.Error("scrolled into the middle, something should be above")
	}
}

func TestListThatFitsReportsNoScroll(t *testing.T) {
	m := gridModel(2, 40, 40)
	m.view = viewList

	if above, below := m.scrollable(); above || below {
		t.Fatalf("above=%v below=%v, want nothing off screen", above, below)
	}
}

func TestGridReportsWhatIsOffScreen(t *testing.T) {
	m := gridModel(12, 40, 16)

	if g := m.grid(); len(g.lines) <= g.visible {
		t.Skipf("a %d-row grid shows all 12 cards", g.visible)
	}

	above, below := m.scrollable()
	if above || !below {
		t.Fatalf("at the top: above=%v below=%v, want false/true", above, below)
	}

	m.cardCursor = 11
	m.clampCard()

	if above, below = m.scrollable(); !above || below {
		t.Fatalf("at the end: above=%v below=%v, want true/false", above, below)
	}
}

func TestGridThatFitsReportsNoScroll(t *testing.T) {
	m := gridModel(2, 40, 40)

	if above, below := m.scrollable(); above || below {
		t.Fatalf("above=%v below=%v, want nothing off screen", above, below)
	}
}

func TestNothingScrollsBeforeTheFirstCollection(t *testing.T) {
	// An unloaded pane has no rows, and must not claim there is more to see.
	m := Model{width: 40, height: 20, collapsed: map[string]bool{}}

	if above, below := m.scrollable(); above || below {
		t.Fatalf("above=%v below=%v, want nothing while loading", above, below)
	}
}

// chevrons answers which chevrons a rendered frame carries, and checks each one
// is on the right side of the content it is talking about.
func chevrons(m Model) (above, below bool) {
	lines := strings.Split(m.View(), "\n")

	var first, last, up, down = -1, -1, -1, -1

	for i, line := range lines {
		switch {
		case strings.Contains(line, chevronUp):
			up = i
		case strings.Contains(line, chevronDown):
			down = i
		case i >= m.titleRows() && i < len(lines)-1 && strings.TrimSpace(line) != "":
			if first < 0 {
				first = i
			}

			last = i
		}
	}

	if up >= 0 && first >= 0 && up > first {
		panic("the up chevron is below the content")
	}

	if down >= 0 && last >= 0 && down < last {
		panic("the down chevron is above the content")
	}

	return up >= 0, down >= 0
}

func TestChevronsAppearAboveAndBelowTheContent(t *testing.T) {
	m := gridModel(12, 40, 16)

	above, below := chevrons(m)
	if above || !below {
		t.Fatalf("at the top: above=%v below=%v, want only the one below", above, below)
	}

	m.cardCursor = 11
	m.clampCard()

	if above, below = chevrons(m); !above || below {
		t.Fatalf("at the end: above=%v below=%v, want only the one above", above, below)
	}
}

func TestChevronsAppearInTheList(t *testing.T) {
	m := gridModel(12, 40, 12)
	m.view = viewList

	if _, below := chevrons(m); !below {
		t.Errorf("expected a chevron below the list in:\n%s", m.View())
	}
}

func TestNoChevronsWhenItAllFits(t *testing.T) {
	m := gridModel(2, 40, 40)

	if above, below := chevrons(m); above || below {
		t.Fatalf("above=%v below=%v, want no chevrons at all", above, below)
	}
}

func TestChevronsCostTheContentTwoRows(t *testing.T) {
	// Reserved together the moment anything overflows, and kept whichever way
	// the pane can actually scroll, so the rows do not shift as you scroll.
	m := gridModel(12, 40, 12)
	m.view = viewList

	if got, want := m.contentHeight(), m.listHeight()-chevronRows; got != want {
		t.Fatalf("content height = %d, want %d", got, want)
	}

	m.cursor = len(m.rows) - 1
	m.clampCursor()

	if got, want := m.contentHeight(), m.listHeight()-chevronRows; got != want {
		t.Fatalf("scrolled to the end, content height = %d, want it unchanged at %d", got, want)
	}

	fits := gridModel(2, 40, 40)
	fits.view = viewList

	if got := fits.contentHeight(); got != fits.listHeight() {
		t.Errorf("with nothing off screen, content height = %d, want the whole body %d", got, fits.listHeight())
	}
}

func TestFrameHeightIsTheSameWithAndWithoutChevrons(t *testing.T) {
	// The footer is pinned to the bottom, and the chevrons must not push it.
	fits := strings.Count(gridModel(2, 40, 20).View(), "\n")

	if overflowing := strings.Count(gridModel(40, 40, 20).View(), "\n"); overflowing != fits {
		t.Fatalf("frame height changed from %d to %d lines once the chevrons appeared", fits+1, overflowing+1)
	}
}

func TestNewHonoursTheRememberedView(t *testing.T) {
	// config.toml seeds a pane that has never been told otherwise; after that,
	// the last choice wins.
	dir := t.TempDir()

	prefs, err := store.OpenPrefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Pane.View = config.ViewList

	if m := New(nil, nil, nil, prefs, cfg); m.view != viewList {
		t.Fatal("with nothing remembered, the configured view should win")
	}

	if err := prefs.SetView(config.ViewCards); err != nil {
		t.Fatal(err)
	}

	want := ordering{mode: sortStatus, desc: true}
	if err := prefs.SetSort(want.String()); err != nil {
		t.Fatal(err)
	}

	reopened, err := store.OpenPrefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	m := New(nil, nil, nil, reopened, cfg)
	if m.view != viewCards {
		t.Error("the remembered view should beat the configured one")
	}

	// The direction has to survive the round trip as well as the mode.
	if m.order != want {
		t.Errorf("order = %s, want the remembered %s", m.order, want)
	}
}

func TestNewWithoutPrefsFallsBackToTheConfig(t *testing.T) {
	cfg := config.Default()
	cfg.Pane.View = config.ViewCards

	if m := New(nil, nil, nil, nil, cfg); m.view != viewCards {
		t.Fatal("with no preference file at all, the configured view should apply")
	}
}

func TestTogglingAndSortingAreRemembered(t *testing.T) {
	dir := t.TempDir()

	prefs, err := store.OpenPrefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	w, assignments := sortable()

	m := cardsOver(w, assignments, 40, 30)
	m.view = viewList
	m.prefs = prefs

	m = press(m, "v")
	m = press(m, "s")

	if m.errNote != "" {
		t.Fatalf("remembering reported %q", m.errNote)
	}

	reopened, err := store.OpenPrefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if reopened.View() != config.ViewCards {
		t.Errorf("remembered view = %q, want cards", reopened.View())
	}

	// One s from the default lands on alpha descending.
	if want := (ordering{desc: true}).String(); reopened.Sort() != want {
		t.Errorf("remembered sort = %q, want %q", reopened.Sort(), want)
	}
}

func TestAFailedRememberIsReportedNotSwallowed(t *testing.T) {
	// A preference that silently does not stick is worse than one that says why.
	prefs, _ := store.OpenPrefs("")

	w, assignments := sortable()

	m := cardsOver(w, assignments, 40, 30)
	m.prefs = prefs

	if got := press(m, "v"); got.errNote == "" {
		t.Fatal("a preference that could not be saved should say so in the footer")
	}
}

func TestTogglingWithoutPrefsDoesNotCrash(t *testing.T) {
	w, assignments := sortable()

	m := cardsOver(w, assignments, 40, 30)
	m.prefs = nil

	if got, want := press(press(m, "v"), "s").order, (ordering{desc: true}); got != want {
		t.Fatalf("order = %s, want %s: the sort should work with nowhere to remember it", got, want)
	}
}
