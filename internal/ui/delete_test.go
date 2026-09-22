package ui

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
	"github.com/echeble/herdr-wayfindr/internal/store"
)

type mockHerdrClient struct {
	calls               []string
	worktreeRemoveCalls []herdr.WorktreeRemoveParams
}

func (m *mockHerdrClient) Call(ctx context.Context, method string, params any, result any) error {
	m.calls = append(m.calls, method)
	if method == herdr.MethodWorktreeRemove {
		if p, ok := params.(herdr.WorktreeRemoveParams); ok {
			m.worktreeRemoveCalls = append(m.worktreeRemoveCalls, p)
		}
	}

	return nil
}

func TestDeleteAllMenuItemPresence(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle),
		wt("/w/c", "mainrepo", "main", "w3", herdr.StatusIdle),
	)

	assignments := map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/c": {Name: "", Source: group.SourceNone}, // Ungrouped
	}

	cfg := config.Default()
	cfg.Grouping.HidePrincipalBranches = false
	cfg.Pane.RightClick = true

	m := Model{
		cfg:         cfg,
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}, false),
		width:       40,
		height:      20,
	}

	// 1. Right click on feature group header ("WAYF-1"): should include "Delete All"
	m.cursor = 0 // WAYF-1 header
	items := m.menuItems()
	hasDeleteAll := false
	for _, it := range items {
		if it.label == "Delete All" {
			hasDeleteAll = true
		}
	}
	if !hasDeleteAll {
		t.Fatalf("expected feature group header to have Delete All, got items: %+v", items)
	}

	// 2. Right click on Ungrouped header: should NOT include "Delete All"
	var ungroupedIndex int
	for idx, r := range m.rows {
		if r.kind == rowGroup && r.name == ungroupedLabel {
			ungroupedIndex = idx
			break
		}
	}
	m.cursor = ungroupedIndex
	ungroupedItems := m.menuItems()
	for _, it := range ungroupedItems {
		if it.label == "Delete All" {
			t.Fatalf("Ungrouped should NOT have Delete All menu item, got items: %+v", ungroupedItems)
		}
	}

	// 3. Right click on a worktree row: should NOT include "Delete All"
	m.cursor = 1 // member row
	wtItems := m.menuItems()
	for _, it := range wtItems {
		if it.label == "Delete All" {
			t.Fatalf("worktree row should NOT have Delete All menu item, got items: %+v", wtItems)
		}
	}
}

func TestDeleteAllCardsViewPresence(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle),
	)

	assignments := map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
	}

	cfg := config.Default()
	cfg.Pane.RightClick = true

	m := Model{
		cfg:         cfg,
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		view:        viewCards,
		cards:       buildRows(w, assignments, map[string]bool{}, ordering{}),
		width:       40,
		height:      20,
	}

	// Feature card at top-level cards view
	m.cardCursor = 0
	items := m.menuItems()
	hasDeleteAll := false
	for _, it := range items {
		if it.label == "Delete All" {
			hasDeleteAll = true
		}
	}
	if !hasDeleteAll {
		t.Fatalf("cards view feature card should have Delete All, got: %+v", items)
	}
}

func TestStartDeleteAllStrictAbortOnPrincipalBranch(t *testing.T) {
	// A group that contains a main/principal branch MUST strictly abort deletion
	w := collect.World{
		Worktrees: []collect.Worktree{
			{
				Path:        "/w/a",
				RepoRoot:    "/w/repo-a",
				Branch:      "feature/WAYF-1",
				IsLinked:    true,
				IsPrincipal: false,
			},
			{
				Path:        "/w/b",
				RepoRoot:    "/w/repo-b",
				Branch:      "main", // Principal branch!
				IsLinked:    true,
				IsPrincipal: true,
			},
		},
	}

	assignments := map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
	}

	m := Model{
		cfg:         config.Default(),
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		cursor:      0, // Header
	}

	after := m.startDeleteAll()

	if after.mode == modeConfirmDelete {
		t.Fatal("startDeleteAll should NOT enter modeConfirmDelete when a member is a principal branch")
	}

	if after.errNote == "" {
		t.Fatal("startDeleteAll should set errNote when principal branch detected")
	}

	if !strings.Contains(after.errNote, "protected branch") && !strings.Contains(after.errNote, "main") {
		t.Fatalf("errNote should mention protected branch/main, got: %q", after.errNote)
	}
}

func TestStartDeleteAllStrictAbortOnPrimaryCheckout(t *testing.T) {
	// A group that contains a primary checkout (!IsLinked or Path == RepoRoot)
	w := collect.World{
		Worktrees: []collect.Worktree{
			{
				Path:        "/w/repo-root",
				RepoRoot:    "/w/repo-root",
				Branch:      "feature/WAYF-1",
				IsLinked:    false, // Primary checkout!
				IsPrincipal: false,
			},
		},
	}

	assignments := map[string]group.Assignment{
		"/w/repo-root": {Name: "WAYF-1", Source: group.SourceJira},
	}

	m := Model{
		cfg:         config.Default(),
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		cursor:      0,
	}

	after := m.startDeleteAll()

	if after.mode == modeConfirmDelete {
		t.Fatal("startDeleteAll should NOT enter modeConfirmDelete on primary checkout")
	}

	if after.errNote == "" {
		t.Fatal("startDeleteAll should set errNote on primary checkout")
	}
}

func TestStartDeleteAllSafeGroupEntersConfirmation(t *testing.T) {
	w := collect.World{
		Worktrees: []collect.Worktree{
			{
				Path:     "/w/wt1",
				RepoRoot: "/w/repo-a",
				Branch:   "feature/safe-1",
				IsLinked: true,
			},
			{
				Path:     "/w/wt2",
				RepoRoot: "/w/repo-b",
				Branch:   "feature/safe-2",
				IsLinked: true,
			},
		},
	}

	assignments := map[string]group.Assignment{
		"/w/wt1": {Name: "SAFE-FEAT", Source: group.SourceJira},
		"/w/wt2": {Name: "SAFE-FEAT", Source: group.SourceJira},
	}

	m := Model{
		cfg:         config.Default(),
		world:       w,
		assignments: assignments,
		collapsed:   map[string]bool{},
		rows:        buildRows(w, assignments, map[string]bool{}, ordering{}),
		cursor:      0,
	}

	after := m.startDeleteAll()

	if after.mode != modeConfirmDelete {
		t.Fatalf("startDeleteAll mode = %v, want modeConfirmDelete", after.mode)
	}

	if after.deleteTargetGroup != "SAFE-FEAT" {
		t.Fatalf("deleteTargetGroup = %q, want SAFE-FEAT", after.deleteTargetGroup)
	}

	if len(after.deleteTargets) != 2 {
		t.Fatalf("deleteTargets len = %d, want 2", len(after.deleteTargets))
	}
}

func TestConfirmDeleteCancel(t *testing.T) {
	m := Model{
		mode:              modeConfirmDelete,
		deleteTargetGroup: "SAFE-FEAT",
		deleteTargets: []*collect.Worktree{
			{Path: "/w/wt1", RepoRoot: "/w/repo-a", Branch: "feature/1", IsLinked: true},
		},
	}

	// Pressing n cancels
	next, cmd := m.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	after := next.(Model)
	if after.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after n", after.mode)
	}
	if cmd != nil {
		t.Fatal("expected nil cmd after cancel")
	}

	// Pressing esc cancels
	m.mode = modeConfirmDelete
	nextEsc, _ := m.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyEscape})
	if nextEsc.(Model).mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after esc", nextEsc.(Model).mode)
	}
}

func TestConfirmDeleteExecutionWithHerdrClient(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	st, err := store.Open(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Set("/w/wt-open", "SAFE-FEAT"); err != nil {
		t.Fatal(err)
	}

	mock := &mockHerdrClient{}

	wtOpen := &collect.Worktree{
		Path:        "/w/wt-open",
		RepoRoot:    "/w/repo-a",
		Branch:      "feature/safe",
		IsLinked:    true,
		WorkspaceID: "ws-123",
	}

	m := Model{
		mode:              modeConfirmDelete,
		client:            mock,
		store:             st,
		deleteTargetGroup: "SAFE-FEAT",
		deleteTargets:     []*collect.Worktree{wtOpen},
	}

	// Confirm with 'y'
	next, cmd := m.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	after := next.(Model)

	if after.mode != modeBrowse {
		t.Fatalf("mode = %v, want modeBrowse after confirm", after.mode)
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd for deletion")
	}

	// Execute the command
	msg := cmd()
	delMsg, ok := msg.(deleteResultMsg)
	if !ok {
		t.Fatalf("expected deleteResultMsg, got %T", msg)
	}

	if delMsg.deleted != 1 {
		t.Fatalf("deleted = %d, want 1", delMsg.deleted)
	}

	if len(mock.worktreeRemoveCalls) != 1 {
		t.Fatalf("expected 1 worktree.remove call, got %d", len(mock.worktreeRemoveCalls))
	}

	if mock.worktreeRemoveCalls[0].WorkspaceID != "ws-123" {
		t.Fatalf("workspace_id = %q, want ws-123", mock.worktreeRemoveCalls[0].WorkspaceID)
	}

	// Tag should have been cleared in store
	if tag := st.Tags()["/w/wt-open"]; tag != "" {
		t.Fatalf("tag was not cleared, got %q", tag)
	}

	// Update with deleteResultMsg
	nextM, _ := after.Update(delMsg)
	finalM := nextM.(Model)
	if !strings.Contains(finalM.note, "deleted 1 worktree") {
		t.Fatalf("note = %q, want deleted 1 worktree", finalM.note)
	}
	_ = ctx
}

func TestConfirmDeleteExecutionGitRemoval(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	repoDir := filepath.Join(tmpDir, "repo")
	wtDir := filepath.Join(tmpDir, "wt-feature")

	runCmd := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v failed: %v\nOutput: %s", name, args, err, out)
		}
	}

	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runCmd(repoDir, "git", "init", "-b", "main")
	runCmd(repoDir, "git", "config", "user.email", "test@test.com")
	runCmd(repoDir, "git", "config", "user.name", "Test")

	if err := os.WriteFile(filepath.Join(repoDir, "f.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCmd(repoDir, "git", "add", "f.txt")
	runCmd(repoDir, "git", "commit", "-m", "init")

	runCmd(repoDir, "git", "worktree", "add", "-b", "feat-git", wtDir)

	targetWt := &collect.Worktree{
		Path:     wtDir,
		RepoRoot: repoDir,
		Branch:   "feat-git",
		IsLinked: true,
	}

	m := Model{
		mode:              modeConfirmDelete,
		deleteTargetGroup: "FEAT",
		deleteTargets:     []*collect.Worktree{targetWt},
	}

	next, cmd := m.updateConfirmDelete(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next
	if cmd == nil {
		t.Fatal("expected cmd")
	}

	msg := cmd()
	delMsg, ok := msg.(deleteResultMsg)
	if !ok {
		t.Fatalf("expected deleteResultMsg, got %T", msg)
	}
	if delMsg.deleted != 1 {
		t.Fatalf("deleted = %d, want 1, errors: %v", delMsg.deleted, delMsg.errors)
	}

	// Verify worktree directory was actually removed from disk
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Fatalf("expected wtDir to be deleted, stat returned: %v", err)
	}

	// Verify main repo still exists
	if _, err := os.Stat(filepath.Join(repoDir, "f.txt")); err != nil {
		t.Fatalf("expected main repo file to still exist: %v", err)
	}
}
