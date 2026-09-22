package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetAndReload(t *testing.T) {
	dir := t.TempDir()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := s.Set("/w/a", "WAYF-1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	if got := reopened.Tags()["/w/a"]; got != "WAYF-1" {
		t.Fatalf("tag did not survive a reopen: %q", got)
	}
}

func TestSetEmptyClearsTheTag(t *testing.T) {
	// Clearing has to remove the entry, not store an empty name: the resolver
	// treats a blank tag as "fall through to derivation".
	s, _ := Open(t.TempDir())

	if err := s.Set("/w/a", "WAYF-1"); err != nil {
		t.Fatalf("set: %v", err)
	}

	if err := s.Set("/w/a", "   "); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if _, ok := s.Tags()["/w/a"]; ok {
		t.Fatal("a blank name should have removed the entry")
	}
}

func TestTagsReturnsACopy(t *testing.T) {
	// The resolver runs on another goroutine with this map.
	s, _ := Open(t.TempDir())
	_ = s.Set("/w/a", "WAYF-1")

	tags := s.Tags()
	tags["/w/a"] = "mutated"

	if s.Tags()["/w/a"] != "WAYF-1" {
		t.Fatal("Tags handed out the live map")
	}
}

func TestMissingFileIsAnEmptyStore(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatalf("a missing file should not be an error: %v", err)
	}

	if len(s.Tags()) != 0 {
		t.Fatal("expected no tags")
	}
}

func TestCorruptFileStillYieldsAUsableStore(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, fileName), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	s, err := Open(dir)
	if err == nil {
		t.Fatal("a corrupt file should be reported")
	}

	// Reported, but still usable: a bad file costs the tags, not the pane.
	if s == nil {
		t.Fatal("store should not be nil")
	}

	if err := s.Set("/w/a", "WAYF-1"); err != nil {
		t.Fatalf("store should still be writable: %v", err)
	}
}

func TestPruneDropsOnlyVanishedPaths(t *testing.T) {
	dir := t.TempDir()
	live := filepath.Join(dir, "live-checkout")

	if err := os.Mkdir(live, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	s, _ := Open(dir)
	_ = s.Set(live, "WAYF-1")
	_ = s.Set("/w/gone", "WAYF-2")
	// On disk but not open in Herdr: it must keep its tag.
	_ = s.Set(dir, "WAYF-3")

	dropped, err := s.Prune(map[string]bool{})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if dropped != 1 {
		t.Fatalf("dropped %d entries, want 1", dropped)
	}

	tags := s.Tags()
	if _, ok := tags["/w/gone"]; ok {
		t.Error("a path that is gone from disk should have been pruned")
	}

	if tags[live] != "WAYF-1" {
		t.Error("a path still on disk should have kept its tag")
	}

	if tags[dir] != "WAYF-3" {
		t.Error("a checkout not open in Herdr should have kept its tag")
	}
}

func TestPruneKeepsPathsHerdrStillReports(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Set("/w/reported", "WAYF-1")

	dropped, err := s.Prune(map[string]bool{"/w/reported": true})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}

	if dropped != 0 {
		t.Fatalf("dropped %d entries, want 0", dropped)
	}
}

func TestNamesAreDistinctAndSorted(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.Set("/w/a", "beta")
	_ = s.Set("/w/b", "alpha")
	_ = s.Set("/w/c", "beta")

	names := s.Names()
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Fatalf("names = %v, want [alpha beta]", names)
	}
}

func TestSaveLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()

	s, _ := Open(dir)
	_ = s.Set("/w/a", "WAYF-1")
	_ = s.Set("/w/b", "WAYF-2")

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}

	if len(entries) != 1 || entries[0].Name() != fileName {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}

		t.Fatalf("expected only %s, found %v", fileName, names)
	}
}

func TestSetLabelAndReload(t *testing.T) {
	dir := t.TempDir()

	s, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if err := s.SetLabel("WAYF-1", "Checkout redesign"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}

	if got := reopened.Labels()["WAYF-1"]; got != "Checkout redesign" {
		t.Fatalf("label did not survive a reopen: %q", got)
	}
}

func TestSetLabelEmptyClearsTheRename(t *testing.T) {
	s, _ := Open(t.TempDir())

	if err := s.SetLabel("WAYF-1", "Checkout redesign"); err != nil {
		t.Fatalf("set label: %v", err)
	}

	if err := s.SetLabel("WAYF-1", "   "); err != nil {
		t.Fatalf("clear: %v", err)
	}

	if _, ok := s.Labels()["WAYF-1"]; ok {
		t.Fatal("a blank label should have removed the entry")
	}
}

func TestLabelsAndTagsDoNotInterfere(t *testing.T) {
	// A rename and a tag are two different concerns living in the same file;
	// setting one must not disturb the other.
	s, _ := Open(t.TempDir())

	_ = s.Set("/w/a", "WAYF-1")
	_ = s.SetLabel("WAYF-1", "Checkout redesign")

	if s.Tags()["/w/a"] != "WAYF-1" {
		t.Fatal("the tag should be unchanged by setting a label")
	}

	if s.Labels()["WAYF-1"] != "Checkout redesign" {
		t.Fatal("the label should be unchanged by setting a tag")
	}
}

func TestLabelsReturnsACopy(t *testing.T) {
	s, _ := Open(t.TempDir())
	_ = s.SetLabel("WAYF-1", "Checkout redesign")

	labels := s.Labels()
	labels["WAYF-1"] = "mutated"

	if s.Labels()["WAYF-1"] != "Checkout redesign" {
		t.Fatal("Labels handed out the live map")
	}
}

func TestOpenWithoutAStateDirectory(t *testing.T) {
	s, err := Open("")
	if err == nil {
		t.Fatal("an empty directory should be reported")
	}

	if s == nil {
		t.Fatal("store should not be nil")
	}

	if err := s.Set("/w/a", "WAYF-1"); err == nil {
		t.Fatal("saving without a directory should fail rather than pretend")
	}
}
