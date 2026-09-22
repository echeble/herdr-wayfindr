package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
	"github.com/echeble/herdr-wayfindr/internal/store"
)

// renameModel builds a two-feature list-view model backed by a real, temporary
// store, so a rename can be checked all the way through to disk and back
// rather than only through the in-memory row it produces.
func renameModel(t *testing.T) Model {
	t.Helper()

	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle),
		wt("/w/c", "k8s", "WAYF-2", "w3", herdr.StatusIdle),
	)

	assignments := assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/c": {Name: "WAYF-2", Source: group.SourceJira},
	})

	m := Model{
		world:       w,
		assignments: assignments,
		store:       s,
		collapsed:   map[string]bool{},
		loaded:      true,
	}

	m.rebuild()

	return m
}

func TestStartRenameRequiresAGroupSelected(t *testing.T) {
	m := renameModel(t)
	m.cursor = 1 // a worktree row, not a header

	next := m.startRename()

	if next.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse: renaming a worktree row makes no sense", next.mode)
	}

	if next.errNote == "" {
		t.Fatal("expected an error note when nothing groupable is selected")
	}
}

func TestStartRenamePrefillsAnExistingLabel(t *testing.T) {
	m := renameModel(t)

	if err := m.store.SetLabel("WAYF-1", "Checkout redesign"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	next := m.startRename() // cursor starts on row 0, the WAYF-1 header

	if next.mode != modeRename {
		t.Fatalf("mode = %v, want modeRename", next.mode)
	}

	if next.renameTarget != "WAYF-1" {
		t.Fatalf("renameTarget = %q, want WAYF-1", next.renameTarget)
	}

	if next.input != "Checkout redesign" {
		t.Fatalf("input = %q, want the existing label prefilled", next.input)
	}
}

func TestRenameRelabelsTheHeaderWithoutRegrouping(t *testing.T) {
	m := renameModel(t)

	renaming := m.startRename()
	renaming.input = "Checkout redesign"

	next, _ := renaming.updateRenamePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after enter", after.mode)
	}

	if got := after.rows[0].displayName(); got != "Checkout redesign" {
		t.Fatalf("header = %q, want the renamed label", got)
	}

	// The identity the pane keys everything off has to be exactly what it was
	// before the rename: the tag, the branch match, the grouping — nothing
	// about how the worktrees were assigned should have moved.
	if after.rows[0].name != "WAYF-1" {
		t.Fatalf("row name = %q, want it unchanged at WAYF-1", after.rows[0].name)
	}

	if a := after.assignments["/w/a"]; a.Name != "WAYF-1" || a.Source != group.SourceJira {
		t.Fatalf("assignment changed by a rename: %+v", a)
	}

	if got := after.store.Labels()["WAYF-1"]; got != "Checkout redesign" {
		t.Fatalf("label not persisted to the store: %q", got)
	}
}

func TestRenameEscLeavesTheHeaderUnchanged(t *testing.T) {
	m := renameModel(t)

	renaming := m.startRename()
	renaming.input = "Something"

	next, _ := renaming.updateRenamePrompt(tea.KeyMsg{Type: tea.KeyEsc})
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse", after.mode)
	}

	if got := after.rows[0].displayName(); got != "WAYF-1" {
		t.Fatalf("header = %q, want it unchanged", got)
	}

	if len(after.store.Labels()) != 0 {
		t.Fatal("esc should not have written anything to the store")
	}
}

func TestRenameEmptyClearsAnExistingLabel(t *testing.T) {
	m := renameModel(t)

	if err := m.store.SetLabel("WAYF-1", "Checkout redesign"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	renaming := m.startRename()
	renaming.input = ""

	next, _ := renaming.updateRenamePrompt(tea.KeyMsg{Type: tea.KeyEnter})
	after := next.(Model)

	if got := after.rows[0].displayName(); got != "WAYF-1" {
		t.Fatalf("header = %q, want it reset to the resolved name", got)
	}

	if _, ok := after.store.Labels()["WAYF-1"]; ok {
		t.Fatal("an empty rename should have cleared the stored label")
	}
}
