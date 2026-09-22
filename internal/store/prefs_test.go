package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrefsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	p, err := OpenPrefs(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if p.View() != "" || p.Sort() != "" {
		t.Fatalf("a fresh file answered %q/%q, want empty so the config default applies", p.View(), p.Sort())
	}

	if err := p.SetView("cards"); err != nil {
		t.Fatalf("set view: %v", err)
	}

	if err := p.SetSort("status"); err != nil {
		t.Fatalf("set sort: %v", err)
	}

	reopened, err := OpenPrefs(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	if reopened.View() != "cards" || reopened.Sort() != "status" {
		t.Fatalf("reopened as %q/%q, want cards/status", reopened.View(), reopened.Sort())
	}
}

func TestPrefsMissingFileIsNotAnError(t *testing.T) {
	p, err := OpenPrefs(t.TempDir())
	if err != nil {
		t.Fatalf("a missing preference file should be an empty one, got %v", err)
	}

	if p.View() != "" {
		t.Fatalf("view = %q, want empty", p.View())
	}
}

func TestPrefsCorruptFileStillYieldsAUsableSet(t *testing.T) {
	// A broken file costs the memory of the last choice, never the pane.
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, prefsFileName), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := OpenPrefs(dir)
	if err == nil {
		t.Error("a malformed file should be reported")
	}

	if p == nil {
		t.Fatal("a malformed file should still yield a usable set of preferences")
	}

	if err := p.SetView("list"); err != nil {
		t.Fatalf("it should still be writable: %v", err)
	}
}

func TestPrefsWithoutAStateDirectoryDoesNotCrash(t *testing.T) {
	p, err := OpenPrefs("")
	if err == nil {
		t.Error("no state directory should be reported")
	}

	if err := p.SetView("cards"); err == nil {
		t.Error("saving with nowhere to save should say so")
	}

	// The choice still holds for this session: it is only the remembering that
	// failed, and undoing what the user just asked for would be the worse half
	// of a bad situation.
	if p.View() != "cards" {
		t.Errorf("view = %q, want the choice to apply even though it was not saved", p.View())
	}

	// A set of preferences that never loaded starts empty, which is what makes
	// the configured default apply rather than a phantom remembered one.
	fresh, _ := OpenPrefs("")
	if fresh.View() != "" || fresh.Sort() != "" {
		t.Errorf("a fresh set answered %q/%q, want empty", fresh.View(), fresh.Sort())
	}
}

func TestPrefsAreSeparateFromTheTagFile(t *testing.T) {
	// The two are written by different things — the user's hand and the pane —
	// so one being rewritten must never disturb the other.
	dir := t.TempDir()

	tags, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := tags.Set("/w/a", "WAYF-1"); err != nil {
		t.Fatal(err)
	}

	p, err := OpenPrefs(dir)
	if err != nil {
		t.Fatal(err)
	}

	if err := p.SetView("cards"); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("the tag file should survive a preference write: %v", err)
	}

	if reopened.Tags()["/w/a"] != "WAYF-1" {
		t.Fatalf("tags = %v, want the tag intact", reopened.Tags())
	}
}
