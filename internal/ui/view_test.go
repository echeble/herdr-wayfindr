package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

func TestTruncateRespectsWidth(t *testing.T) {
	// A sidebar is narrow, and a long branch name must not push the pane into
	// horizontal scroll.
	long := "  ◌ ca-bnpl-decision-engine · worktree-CRD-20226_remove_consumer_plan_deprecated_fields"

	for _, width := range []int{10, 26, 40} {
		got := truncate(long, width)
		if lipgloss.Width(got) > width {
			t.Errorf("width %d: rendered %d cells", width, lipgloss.Width(got))
		}
	}
}

func TestTruncateLeavesShortStringsAlone(t *testing.T) {
	if got := truncate("CRD-1", 40); got != "CRD-1" {
		t.Fatalf("got %q", got)
	}
}

func TestPadFixesLineCount(t *testing.T) {
	// The footer is pinned to the bottom, so the body always owns exactly the
	// rows between the title and it.
	if got := len(strings.Split(pad("a\nb", 5), "\n")); got != 5 {
		t.Errorf("short block padded to %d lines, want 5", got)
	}

	if got := len(strings.Split(pad("a\nb\nc\nd", 2), "\n")); got != 2 {
		t.Errorf("long block trimmed to %d lines, want 2", got)
	}
}

func TestPadRightFillsToWidth(t *testing.T) {
	if got := lipgloss.Width(padRight("ab", 6)); got != 6 {
		t.Fatalf("padded width = %d, want 6", got)
	}
}

func TestStatusSuffix(t *testing.T) {
	cases := []struct {
		name string
		git  collect.GitStatus
		want string
	}{
		{"unknown reports nothing", collect.GitStatus{}, ""},
		{"clean", collect.GitStatus{Known: true}, ""},
		{"ahead and behind", collect.GitStatus{Known: true, Ahead: 2, Behind: 3}, "↑2 ↓3"},
		{"dirty", collect.GitStatus{Known: true, Staged: 1, Unstaged: 2}, "✱3"},
		{"untracked only", collect.GitStatus{Known: true, Untracked: 9}, ""},
		{"conflicts replace the dirty mark", collect.GitStatus{Known: true, Staged: 1, Conflicts: 2}, "!2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := statusSuffix(collect.Worktree{Git: tc.git})
			if got != tc.want {
				t.Errorf("suffix = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusGlyphClosedWorktree(t *testing.T) {
	// A checkout Herdr does not have open reads as closed whatever its
	// recorded agent status says.
	if statusGlyph(herdr.StatusWorking, false) != statusGlyph(herdr.StatusIdle, false) {
		t.Fatal("a closed worktree should have one glyph regardless of status")
	}
}

func TestAgentWord(t *testing.T) {
	cases := []struct {
		name   string
		status string
		agents collect.AgentTally
		open   bool
		want   string
	}{
		{"one agent is singular", herdr.StatusWorking, collect.AgentTally{Working: 1}, true, "1 agent working"},
		{"several are plural", herdr.StatusWorking, collect.AgentTally{Working: 3}, true, "3 agents working"},
		{
			// The label says blocked, so the count is of blocked agents — the
			// two still working are not what the row is reporting.
			"counts the state named, not the crowd",
			herdr.StatusBlocked,
			collect.AgentTally{Blocked: 1, Working: 2},
			true,
			"1 agent blocked",
		},
		{"idle says so too", herdr.StatusIdle, collect.AgentTally{Idle: 2}, true, "2 agents idle"},
		{"a closed checkout says nothing", herdr.StatusWorking, collect.AgentTally{Working: 1}, false, ""},
		{"unknown says nothing", herdr.StatusUnknown, collect.AgentTally{Unknown: 2}, true, ""},
		{"no status says nothing", "", collect.AgentTally{}, true, ""},
		{"a tally that cannot account for it falls back", herdr.StatusWorking, collect.AgentTally{}, true, "working"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := agentWord(tc.status, tc.agents, tc.open); got != tc.want {
				t.Errorf("agentWord = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPRMarksAreOneCell(t *testing.T) {
	// The row marks are appended past a truncation and the card foot is right
	// aligned, so both assume a mark is exactly one column. An emoji-width
	// glyph — ⌛ U+231B is the tempting one — would be two and shift every row
	// carrying it out of line with the rest.
	for _, state := range []collect.PRState{collect.PRMerged, collect.PROpen, collect.PRClosed} {
		mark := prMark(state)

		if got := lipgloss.Width(mark); got != 1 {
			t.Errorf("%s mark %q is %d cells, want 1", state, mark, got)
		}
	}
}

func TestViewAlwaysDrawsSomething(t *testing.T) {
	// Herdr leaves a pane blank until its process paints, so an empty first
	// frame reads as a broken plugin.
	m := Model{width: 30, height: 10, collapsed: map[string]bool{}}

	out := m.View()
	if strings.TrimSpace(out) == "" {
		t.Fatal("the first frame was empty")
	}

	// And it does not spend a line repeating the name Herdr already puts on
	// the pane.
	if strings.Contains(out, "Features") {
		t.Errorf("the pane titled itself, got:\n%s", out)
	}
}

func TestOnlyTheBreadcrumbTakesTheTitleLine(t *testing.T) {
	grid := cardModel(40, 30)
	if grid.titleRows() != 0 {
		t.Fatal("the feature grid should spend no line on a title")
	}

	inside := press(grid, "enter")
	if inside.titleRows() != 1 {
		t.Fatal("inside a feature the title line carries the trail back out")
	}

	if first := strings.SplitN(inside.View(), "\n", 2)[0]; !strings.Contains(first, "‹") {
		t.Fatalf("first line = %q, want the breadcrumb", first)
	}
}

func TestViewHeightIsStable(t *testing.T) {
	m := Model{width: 40, height: 12, collapsed: map[string]bool{}}

	loading := strings.Count(m.View(), "\n")

	m.loaded = true
	m.world = world(wt("/w/a", "cds", "CRD-1", "w1", herdr.StatusIdle))
	m.assignments = map[string]group.Assignment{"/w/a": {Name: "CRD-1", Source: group.SourceJira}}
	m.rows = buildRows(m.world, m.assignments, m.collapsed, ordering{})

	if listed := strings.Count(m.View(), "\n"); listed != loading {
		t.Fatalf("frame height changed from %d to %d lines", loading+1, listed+1)
	}
}

func TestFooterShowsErrorsOverHints(t *testing.T) {
	m := Model{width: 80, height: 10, collapsed: map[string]bool{}, errNote: "worktree.list failed"}

	if !strings.Contains(m.footer(80), "worktree.list failed") {
		t.Fatal("an error should displace the key hints")
	}
}

func TestMenuOverlayStaysInsideThePane(t *testing.T) {
	m := clickable(8)
	before := len(strings.Split(m.View(), "\n"))

	m = rightClick(m, 10, m.contentTop()+1)
	out := m.View()

	// The overlay replaces lines in place; it must never add or drop one, or
	// the footer would jump around whenever the menu opens and closes.
	lines := strings.Split(out, "\n")
	if len(lines) != before {
		t.Fatalf("frame is %d lines with the menu open, want it still %d", len(lines), before)
	}

	for i, line := range lines {
		if w := lipgloss.Width(line); w > m.width {
			t.Errorf("line %d is %d cells wide, want it within the pane's %d", i, w, m.width)
		}
	}

	if !strings.Contains(out, "Open") || !strings.Contains(out, "Tag") || !strings.Contains(out, "Pin") {
		t.Fatalf("menu items missing from the frame:\n%s", out)
	}
}

// TestMenuOverlayDoesNotStretchToTheFullPaneWidth guards the bug this once
// was: a covered line was padded all the way out to the pane's own width, so
// the menu's footprint read as spanning the whole sidebar instead of just the
// small box it actually draws.
func TestMenuOverlayDoesNotStretchToTheFullPaneWidth(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1)

	x, top, boxWidth, boxHeight := m.menuRect(m.menuItems())

	lines := strings.Split(m.View(), "\n")

	for i := range boxHeight {
		line := lines[top+i]
		if w := lipgloss.Width(line); w > x+boxWidth {
			t.Errorf("line %d is %d cells wide, want at most %d — the box's own footprint, not the pane's",
				top+i, w, x+boxWidth)
		}
	}
}

// TestMenuOverlayAtTheLeftmostColumnStartsAtItsOwnEdge guards the first shape
// of this bug: truncate treats a width of 0 as "no limit" and hands the whole
// line back unchanged, so a menu anchored at column 0 — any click in the
// leftmost column of a row other than the very first — got appended after
// the entire row instead of drawn at its start.
func TestMenuOverlayAtTheLeftmostColumnStartsAtItsOwnEdge(t *testing.T) {
	m := gridModel(6, 60, 30)
	m.cfg.Pane.RightClick = true

	g := m.grid()
	if len(g.lines) < 2 {
		t.Fatal("test setup: need at least two rows")
	}

	// Well inside the first card of the second row, column 0 of its own row —
	// the exact case that broke.
	clickY := m.contentTop() + g.height + 1
	m = rightClick(m, 2, clickY)

	if m.cardCursor != g.lines[1].start {
		t.Fatalf("test setup: cardCursor = %d, want %d (the first card of the second row)",
			m.cardCursor, g.lines[1].start)
	}

	x, top, boxWidth, _ := m.menuRect(m.menuItems())

	line := strings.Split(m.View(), "\n")[top]
	got := ansi.Cut(line, x, x+boxWidth)
	want := strings.Split(m.menuBox(m.menuItems()), "\n")[0]

	if got != want {
		t.Fatalf("the box's own first line should start at column %d:\n got  %q\n want %q", x, got, want)
	}
}

// TestMenuOverlayPreservesWhatIsBesideTheBox guards the second shape of this
// bug: even once the box itself landed at the right column, the overlay
// replaced each covered line outright, which silently deleted anything a
// neighbour drew on that same line past the box's own width — another card's
// top border sharing the row, say.
func TestMenuOverlayPreservesWhatIsBesideTheBox(t *testing.T) {
	m := gridModel(6, 60, 30)
	m.cfg.Pane.RightClick = true

	g := m.grid()
	if len(g.lines) < 2 || g.lines[1].count < 2 {
		t.Fatal("test setup: need a second row with at least two cards in it")
	}

	clickY := m.contentTop() + g.height + 1
	m = rightClick(m, 2, clickY)

	// The baseline is the same selection, without the menu, so the only
	// difference the comparison below can see is the overlay itself — not a
	// border that happens to thicken once a card becomes selected.
	baseline := m
	baseline.mode = modeBrowse
	before := strings.Split(baseline.View(), "\n")

	x, top, boxWidth, boxHeight := m.menuRect(m.menuItems())

	after := strings.Split(m.View(), "\n")

	for i := range boxHeight {
		row := top + i

		wantAfterBox := ansi.Cut(before[row], x+boxWidth, overlayCutHorizon)
		gotAfterBox := ansi.Cut(after[row], x+boxWidth, overlayCutHorizon)

		if gotAfterBox != wantAfterBox {
			t.Errorf("line %d: content beside the box changed\n got  %q\n want %q", row, gotAfterBox, wantAfterBox)
		}
	}
}

func TestMenuOverlayOnlyDrawsWhileTheMenuIsUp(t *testing.T) {
	m := clickable(20)

	if strings.Contains(m.View(), "Open") {
		t.Fatal("the menu should not draw in browse mode")
	}
}

func TestMenuBoxListsEveryItem(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	box := m.menuBox(m.menuItems())
	if !strings.Contains(box, "Open") || !strings.Contains(box, "Fold") {
		t.Fatalf("expected Open and Fold in the box, got:\n%s", box)
	}
}
