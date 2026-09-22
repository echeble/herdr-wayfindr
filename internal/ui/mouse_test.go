package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// clickable builds a model holding one group of two worktrees, so the rows are
// header, member, member. Right click defaults on, matching the config
// default, so a test has to opt out rather than every other test opting in.
func clickable(height int) Model {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle),
	)

	assignments := map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
	}

	return Model{
		cfg:         config.Config{Pane: config.Pane{RightClick: true}},
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		width:       40,
		height:      height,
	}
}

func click(m Model, x, y int) Model {
	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      y,
	})

	return next.(Model)
}

func rightClick(m Model, x, y int) Model {
	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      x,
		Y:      y,
	})

	return next.(Model)
}

func TestClickSelectsTheRowUnderIt(t *testing.T) {
	m := clickable(20)

	// The list has no title line of its own, so the first row is the pane's
	// first line. contentTop is asked rather than assumed: it is what the hit
	// testing itself undoes.
	for row := range 3 {
		if got := click(m, 10, m.contentTop()+row).cursor; got != row {
			t.Errorf("click on row %d selected row %d", row, got)
		}
	}
}

func TestClickAccountsForScrolling(t *testing.T) {
	m := clickable(20)
	m.top = 1

	if got := click(m, 10, m.contentTop()).cursor; got != 1 {
		t.Fatalf("cursor = %d, want 1: the first visible line is row m.top", got)
	}
}

func TestClickOutsideTheListIsIgnored(t *testing.T) {
	m := clickable(20)
	m.cursor = 2

	// Past the last row, and above the pane entirely.
	for _, line := range []int{m.contentTop() + 3, 40, -1} {
		if got := click(m, 10, line).cursor; got != 2 {
			t.Errorf("click on line %d moved the cursor to %d, want it left at 2", line, got)
		}
	}
}

func TestClickOnTheFoldMarkerFolds(t *testing.T) {
	m := clickable(20)

	folded := click(m, 0, m.contentTop())
	if !folded.collapsed["WAYF-1"] {
		t.Fatal("a click on the marker column should fold the group")
	}

	// And unfold it again.
	if click(folded, 0, m.contentTop()).collapsed["WAYF-1"] {
		t.Fatal("a second click on the marker should unfold the group")
	}
}

func TestClickOnAGroupNameOnlySelects(t *testing.T) {
	m := clickable(20)

	after := click(m, 6, m.contentTop())
	if after.collapsed["WAYF-1"] {
		t.Fatal("clicking the name should select the group, not fold it")
	}

	if after.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", after.cursor)
	}
}

func TestClickOnAWorktreeIgnoresTheMarkerColumn(t *testing.T) {
	// Worktree rows are indented past the marker column and have nothing to
	// fold, so a click there is a plain selection.
	m := clickable(20)

	after := click(m, 0, m.contentTop()+1)
	if after.cursor != 1 || len(after.rows) != 3 {
		t.Fatalf("cursor = %d with %d rows, want 1 with 3", after.cursor, len(after.rows))
	}
}

func TestWheelMovesTheCursor(t *testing.T) {
	m := clickable(20)

	wheel := func(m Model, button tea.MouseButton) Model {
		next, _ := m.updateMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: button})

		return next.(Model)
	}

	down := wheel(m, tea.MouseButtonWheelDown)
	if down.cursor != wheelRows-1 {
		// Three rows in the list, so a three-row notch clamps to the last one.
		t.Fatalf("cursor after a wheel notch = %d, want %d", down.cursor, wheelRows-1)
	}

	if up := wheel(down, tea.MouseButtonWheelUp); up.cursor != 0 {
		t.Fatalf("cursor after scrolling back up = %d, want 0", up.cursor)
	}
}

func TestMouseIsIgnoredWhileTagging(t *testing.T) {
	m := clickable(20)
	m.mode = modeTag
	m.cursor = 1

	if got := click(m, 10, 1).cursor; got != 1 {
		t.Fatalf("cursor = %d, want it left at 1 while the tag prompt is up", got)
	}
}

func TestReleaseDoesNotSelect(t *testing.T) {
	m := clickable(20)
	m.cursor = 2

	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      10,
		Y:      1,
	})

	if got := next.(Model).cursor; got != 2 {
		t.Fatalf("cursor = %d, want 2: only the press acts", got)
	}
}

// A click must never fire on an empty list — rowAt is the only guard between a
// stray coordinate and an index out of range.
func TestClickOnAnEmptyListIsSafe(t *testing.T) {
	m := Model{collapsed: map[string]bool{}, width: 40, height: 20, world: collect.World{}}

	if got := click(m, 0, 1).cursor; got != 0 {
		t.Fatalf("cursor = %d, want 0", got)
	}
}

func TestRightClickSelectsAndOpensTheMenu(t *testing.T) {
	m := clickable(20)

	after := rightClick(m, 10, m.contentTop()+1)
	if after.cursor != 1 {
		t.Fatalf("cursor = %d, want 1: a right click selects the row under it", after.cursor)
	}

	if after.mode != modeMenu {
		t.Fatal("a right click on a row should open the menu")
	}
}

func TestRightClickIsIgnoredWhenConfigDisablesIt(t *testing.T) {
	m := clickable(20)
	m.cfg.Pane.RightClick = false

	after := rightClick(m, 10, m.contentTop())
	if after.mode != modeBrowse {
		t.Fatal("pane.right_click = false should leave the menu closed")
	}
}

func TestClickOutsideTheMenuDismissesIt(t *testing.T) {
	m := clickable(20)
	m.mode = modeMenu
	m.cursor = 1

	// Row 0 is above where the menu opens (anchored to row 1), so it is
	// outside the box whatever its exact size.
	after := click(m, 10, 0)
	if after.mode != modeBrowse {
		t.Fatal("a click outside the box should dismiss the menu")
	}

	// It only dismisses; it does not also act as an ordinary click.
	if after.cursor != 1 {
		t.Fatalf("cursor = %d, want it left at 1", after.cursor)
	}
}

func TestClickOnAMenuItemRunsIt(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	items := m.menuItems() // Open, Rename, Fold for a group header in the list.
	bx, by, _, _ := m.menuRect(items)

	// Fold is item index 2, two rows below the top border.
	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      bx + 1,
		Y:      by + 3,
	})
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatal("clicking a menu item should close the menu")
	}

	if !after.collapsed["WAYF-1"] {
		t.Fatal("clicking Fold should fold the group")
	}
}

func TestClickOnTheMenuBoxBorderDoesNothing(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	bx, by, _, _ := m.menuRect(m.menuItems())

	// The box's own top-left corner: inside its rectangle, but not an item.
	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      bx,
		Y:      by,
	})
	after := next.(Model)

	if after.mode != modeMenu {
		t.Fatal("clicking the menu's own border should neither close it nor run an item")
	}
}

func TestWheelMovesTheMenuHighlightWithoutClosingIt(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	next, _ := m.updateMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	after := next.(Model)

	if after.mode != modeMenu {
		t.Fatal("a wheel notch should not close the menu")
	}

	if after.menuCursor != 1 {
		t.Fatalf("menuCursor = %d, want 1 after a wheel-down notch", after.menuCursor)
	}

	if up, _ := after.updateMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp}); up.(Model).menuCursor != 0 {
		t.Fatalf("menuCursor after scrolling back up = %d, want 0", up.(Model).menuCursor)
	}
}

// TestRightClickWhileTheMenuIsUpRetargetsIt guards the bug this once was:
// right-clicking a different row or card while the menu was already up just
// closed it instead of reopening on the new target, which read as the menu
// having appeared in the wrong place — it was really the previous, correctly
// placed one lingering on screen.
func TestRightClickWhileTheMenuIsUpRetargetsIt(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()) // opens on the group header, row 0

	next, cmd := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      10,
		Y:      m.contentTop() + 1, // a worktree row, not the header
	})
	after := next.(Model)

	if after.mode != modeMenu {
		t.Fatal("a right click on a new target while the menu is up should retarget it, not close it")
	}

	if after.cursor != 1 {
		t.Fatalf("cursor = %d, want 1: the menu should have moved to the new row", after.cursor)
	}

	if cmd == nil {
		t.Fatal("retargeting the menu should keep hover tracking on")
	}
}

func TestRightClickOnEmptySpaceWhileTheMenuIsUpClosesIt(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      10,
		Y:      40, // well past the last row
	})

	if next.(Model).mode != modeBrowse {
		t.Fatal("a right click that hits nothing should close the menu")
	}
}

// TestReleasingTheOpeningClickDoesNotDismissTheMenu guards the bug this once
// was: right-click delivers a press and then its own release as two separate
// messages, and that release must not read as a second click away from the
// menu the press just opened.
func TestReleasingTheOpeningClickDoesNotDismissTheMenu(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonRight,
		X:      10,
		Y:      m.contentTop(),
	})

	if next.(Model).mode != modeMenu {
		t.Fatal("releasing the button that opened the menu should not close it")
	}
}

func TestMenuEscCancelsWithoutActing(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1)

	next, cmd := m.updateMenu(tea.KeyMsg{Type: tea.KeyEsc})
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatal("esc should close the menu")
	}

	if after.note != "" || after.errNote != "" || after.tagTarget != "" {
		t.Fatalf("esc should not run any menu action, got note=%q errNote=%q tagTarget=%q",
			after.note, after.errNote, after.tagTarget)
	}

	// A cmd is still expected here: closing the menu turns hover tracking
	// back off, and that is the one thing esc still does.
	if cmd == nil {
		t.Fatal("esc should still turn hover tracking back off")
	}
}

// down moves the menu highlight, mirroring what an arrow key or j/k sends.
func down(m Model) Model {
	next, _ := m.updateMenu(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})

	return next.(Model)
}

func enter(m Model) (tea.Model, tea.Cmd) {
	return m.updateMenu(tea.KeyMsg{Type: tea.KeyEnter})
}

func TestMenuItemsForAWorktree(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1)

	var labels []string
	for _, item := range m.menuItems() {
		labels = append(labels, item.label)
	}

	want := []string{"Open", "Tag", "Pin"}
	if len(labels) != len(want) {
		t.Fatalf("items = %v, want %v", labels, want)
	}

	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("items = %v, want %v", labels, want)
		}
	}
}

func TestMenuItemsForAGroupHeaderInTheList(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	var labels []string
	for _, item := range m.menuItems() {
		labels = append(labels, item.label)
	}

	want := []string{"Open", "Rename", "Fold", "Delete All"}
	if len(labels) != len(want) {
		t.Fatalf("items = %v, want %v", labels, want)
	}

	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("items = %v, want %v", labels, want)
		}
	}
}

func TestMenuTagOpensTheTagPromptForAWorktree(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1)

	// Open, Tag, Pin: Tag is one down from where the menu opens.
	next, _ := enter(down(m))
	after := next.(Model)

	if after.mode != modeTag {
		t.Fatalf("mode = %v, want modeTag", after.mode)
	}

	if after.tagTarget != "/w/a" {
		t.Fatalf("tagTarget = %q, want /w/a", after.tagTarget)
	}
}

func TestMenuFoldTogglesAGroupInTheList(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	// Open, Rename, Fold: Fold is two down from where the menu opens.
	next, _ := enter(down(down(m)))
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatal("fold should close the menu")
	}

	if !after.collapsed["WAYF-1"] {
		t.Fatal("selecting fold on a group header should fold it")
	}
}

func TestMenuHighlightDoesNotWalkPastTheLastItem(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	// Four items for a group header in the list — the highlight must
	// clamp rather than run off the end.
	after := down(down(down(down(down(m)))))

	if after.menuCursor != 3 {
		t.Fatalf("menuCursor = %d, want it clamped to 3", after.menuCursor)
	}
}

func TestHoveringAMenuItemHighlightsIt(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1) // a worktree row: Open, Tag, Pin

	bx, by, _, _ := m.menuRect(m.menuItems())

	// Tag is item index 1, one row below the top border.
	next, cmd := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      bx + 1,
		Y:      by + 2,
	})
	after := next.(Model)

	if after.menuCursor != 1 {
		t.Fatalf("menuCursor = %d, want 1 after hovering the second item", after.menuCursor)
	}

	if after.mode != modeMenu {
		t.Fatal("hovering should not close the menu")
	}

	if cmd != nil {
		t.Fatal("hovering should not run an item")
	}
}

func TestHoveringOutsideTheBoxLeavesTheHighlightAlone(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop()+1)
	m.menuCursor = 0

	next, _ := m.updateMouse(tea.MouseMsg{Action: tea.MouseActionMotion, X: 0, Y: 0})
	after := next.(Model)

	if after.menuCursor != 0 {
		t.Fatalf("menuCursor = %d, want it left at 0 while hovering outside the box", after.menuCursor)
	}

	if after.mode != modeMenu {
		t.Fatal("hovering outside the box should not close the menu")
	}
}

func TestOpeningTheMenuTurnsOnHoverTracking(t *testing.T) {
	m := clickable(20)

	_, cmd := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      10,
		Y:      m.contentTop(),
	})

	if cmd == nil {
		t.Fatal("opening the menu should turn hover tracking on")
	}
}

func TestRunningAMenuItemTurnsHoverTrackingBackOff(t *testing.T) {
	m := clickable(20)
	m = rightClick(m, 10, m.contentTop())

	// Fold is item index 2; its own action (toggleCollapse) returns no cmd of
	// its own, so any cmd here has to be the one that closing the menu adds.
	next, cmd := enter(down(down(m)))
	after := next.(Model)

	if !after.collapsed["WAYF-1"] {
		t.Fatal("fold did not run")
	}

	if cmd == nil {
		t.Fatal("running a menu item should turn hover tracking back off")
	}
}

// TestRightClickOnASecondColumnCardAnchorsTheMenuThere guards the bug this
// once was: the menu always opened at the pane's own left edge, so
// right-clicking a card anywhere but the first column put the box nowhere
// near the click. The menu's top-left corner sits at the pointer itself, so
// the fix is checked against the exact column clicked, not just "somewhere
// past the first card".
func TestRightClickOnASecondColumnCardAnchorsTheMenuThere(t *testing.T) {
	m := cardModel(40, 20)
	m.cfg.Pane.RightClick = true

	g := m.grid()
	if g.lines[0].count < 2 {
		t.Fatal("test setup: this width should fit two cards per row")
	}

	span := g.lines[0].width + cardGap
	clickX := span + 1 // well inside the second card, not the first

	next, _ := m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonRight,
		X:      clickX,
		Y:      m.contentTop(),
	})
	after := next.(Model)

	if after.cardCursor != 1 {
		t.Fatalf("cardCursor = %d, want 1: the click landed on the second card", after.cardCursor)
	}

	bx, _, _, _ := after.menuRect(after.menuItems())
	if bx != clickX {
		t.Fatalf("menu x = %d, want it anchored at the click's own column, %d", bx, clickX)
	}
}

// TestRightClickOnACardRetargetsAnAlreadyOpenMenu is the card-grid case of
// the retargeting fix: opening on one card and then right-clicking another
// must move the menu to the new one rather than leaving it — closed or not —
// wherever the first click put it.
func TestRightClickOnACardRetargetsAnAlreadyOpenMenu(t *testing.T) {
	m := cardModel(40, 20)
	m.cfg.Pane.RightClick = true

	g := m.grid()
	if g.lines[0].count < 2 {
		t.Fatal("test setup: this width should fit two cards per row")
	}

	span := g.lines[0].width + cardGap

	// Open on the second card first.
	m = next(m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonRight,
		X: span + 1, Y: m.contentTop(),
	}))

	if m.cardCursor != 1 {
		t.Fatalf("test setup: cardCursor = %d, want 1", m.cardCursor)
	}

	// Right click the first card while that menu is still up.
	m = next(m.updateMouse(tea.MouseMsg{
		Action: tea.MouseActionPress, Button: tea.MouseButtonRight,
		X: 1, Y: m.contentTop(),
	}))

	if m.mode != modeMenu {
		t.Fatal("the menu should still be open, retargeted rather than closed")
	}

	if m.cardCursor != 0 {
		t.Fatalf("cardCursor = %d, want 0: the menu should have moved to the first card", m.cardCursor)
	}

	bx, _, _, _ := m.menuRect(m.menuItems())
	if bx != 1 {
		t.Fatalf("menu x = %d, want it back at the second click's own column, 1", bx)
	}
}

// next unwraps updateMouse's tea.Model return back into a Model, for tests
// that chain several calls together.
func next(model tea.Model, _ tea.Cmd) Model {
	return model.(Model)
}

func TestPrincipalBranchWorktreeMenuHasNoTagOrPin(t *testing.T) {
	w := world(
		wt("/w/m", "cds", "master", "w1", herdr.StatusIdle),
	)
	assignments := map[string]group.Assignment{
		"/w/m": {Name: "CRD-1", Source: group.SourceJira},
	}

	cfg := config.Default()
	cfg.Pane.RightClick = true

	m := Model{
		cfg:         cfg,
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		width:       40,
		height:      20,
		cursor:      1,
	}

	items := m.menuItems()
	for _, item := range items {
		if item.label == "Tag" || item.label == "Pin" {
			t.Fatalf("menu on principal branch should not contain %q when HidePrincipalBranches is true", item.label)
		}
	}
	if len(items) != 1 || items[0].label != "Open" {
		t.Fatalf("menu on principal branch should only have Open, got %+v", items)
	}

	// When HidePrincipalBranches is false, Tag and Pin should be allowed
	m.cfg.Grouping.HidePrincipalBranches = false
	itemsAllowed := m.menuItems()
	hasTag, hasPin := false, false
	for _, item := range itemsAllowed {
		if item.label == "Tag" {
			hasTag = true
		}
		if item.label == "Pin" {
			hasPin = true
		}
	}
	if !hasTag || !hasPin {
		t.Fatalf("menu should contain Tag and Pin when HidePrincipalBranches is false, got %+v", itemsAllowed)
	}
}

func TestStartTagRejectsPrincipalBranch(t *testing.T) {
	w := world(
		wt("/w/m", "cds", "master", "w1", herdr.StatusIdle),
	)
	assignments := map[string]group.Assignment{
		"/w/m": {Name: "CRD-1", Source: group.SourceJira},
	}

	m := Model{
		cfg:         config.Default(),
		world:       w,
		assignments: assignments,
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		width:       40,
		height:      20,
		cursor:      1,
	}

	after := m.startTag()

	if after.mode == modeTag {
		t.Fatal("startTag on principal branch should not enter modeTag when HidePrincipalBranches is true")
	}
	if after.errNote == "" {
		t.Fatal("startTag on principal branch should set an error note")
	}

	// When HidePrincipalBranches is false, startTag should open modeTag
	m.cfg.Grouping.HidePrincipalBranches = false
	afterAllowed := m.startTag()
	if afterAllowed.mode != modeTag {
		t.Fatalf("startTag should enter modeTag when HidePrincipalBranches is false, got mode=%v", afterAllowed.mode)
	}
}

func TestPinRejectsPrincipalBranch(t *testing.T) {
	w := world(
		wt("/w/m", "cds", "master", "w1", herdr.StatusIdle),
	)
	assignments := map[string]group.Assignment{
		"/w/m": {Name: "CRD-1", Source: group.SourceJira},
	}

	m := Model{
		cfg:         config.Default(),
		world:       w,
		assignments: assignments,
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		width:       40,
		height:      20,
		cursor:      1,
	}

	after, _ := m.pin()

	if after.errNote == "" {
		t.Fatal("pin on principal branch should set an error note when HidePrincipalBranches is true")
	}
}
