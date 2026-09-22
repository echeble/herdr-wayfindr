package group

import (
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/config"
)

func defaultGrouping() config.Grouping {
	return config.Default().Grouping
}

func TestResolveTicketKeyFromBranch(t *testing.T) {
	cases := []struct {
		name   string
		branch string
		want   string
	}{
		{"bare key", "WAYF-20748", "WAYF-20748"},
		{"key with suffix", "DECIS-1234-signum-guard", "DECIS-1234"},
		{"underscore suffix", "WAYF-20616_fix_rule_133", "WAYF-20616"},
		// The case a ticket was typed in is an accident of how the worktree was
		// made, so it is matched either way and upper-cased into one group.
		{"lowercase key", "wayf-20748", "WAYF-20748"},
		{"mixed case key", "Wayf-20748", "WAYF-20748"},
		{"lowercase key mid-branch", "whatever-fix-no-wayf-20226", "WAYF-20226"},
		// The prefixed forms matter most: the same ticket is spelled three
		// ways across these repos and all three have to land in one group.
		{"slash prefix", "worktree/WAYF-20099", "WAYF-20099"},
		{"dash prefix", "worktree-WAYF-20226_remove_fields", "WAYF-20226"},
		{"no number is not a key", "WAYF-extract-sim-files", ""},
		// Herdr's own generated names are the reason the key has a guard at
		// both ends: matched case-insensitively and without one, forest-349
		// inside rapid-forest-349e would read as a ticket.
		{"generated worktree name", "worktree/rapid-forest-349e", ""},
		{"generated worktree name, hex tail", "worktree/rapid-harbor-50e6", ""},
		{"no key at all", "bnpl_server_module", ""},
		{"detached checkout", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(defaultGrouping(), nil)
			got := r.Resolve([]Input{{Path: "/w/a", Branch: tc.branch, RepoRoot: "/r/a"}})

			if got["/w/a"].Name != tc.want {
				t.Fatalf("branch %q: group = %q, want %q", tc.branch, got["/w/a"].Name, tc.want)
			}

			if tc.want != "" && got["/w/a"].Source != SourceJira {
				t.Fatalf("branch %q: source = %v, want SourceJira", tc.branch, got["/w/a"].Source)
			}
		})
	}
}

func TestResolveTicketKeyFromWorkspaceName(t *testing.T) {
	cases := []struct {
		name      string
		branch    string
		workspace string
		want      string
	}{
		// The point of the rule: the checkout is on master or detached, and
		// the only place the ticket is written down is the workspace.
		{"master in a ticket workspace", "master", "wayf-20764", "WAYF-20764"},
		{"detached in a ticket workspace", "", "WAYF-20764", "WAYF-20764"},
		{"suffixed workspace name", "master", "wayf-20226_remove_fields", "WAYF-20226"},
		{"no key anywhere", "master", "wayfindr", ""},
		{"no workspace open", "master", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewResolver(defaultGrouping(), nil)
			got := r.Resolve([]Input{{
				Path:      "/w/a",
				Branch:    tc.branch,
				Workspace: tc.workspace,
				RepoRoot:  "/r/a",
			}})

			if got["/w/a"].Name != tc.want {
				t.Fatalf("workspace %q: group = %q, want %q", tc.workspace, got["/w/a"].Name, tc.want)
			}
		})
	}
}

func TestBranchKeyBeatsTheWorkspaceName(t *testing.T) {
	// A workspace can be renamed under a worktree; the branch is the
	// worktree's own answer, so it is asked first.
	r := NewResolver(defaultGrouping(), nil)

	got := r.Resolve([]Input{{
		Path:      "/w/a",
		Branch:    "WAYF-20748",
		Workspace: "wayf-20764",
		RepoRoot:  "/r/a",
	}})

	if got["/w/a"].Name != "WAYF-20748" {
		t.Fatalf("group = %q, want WAYF-20748: the branch names the feature", got["/w/a"].Name)
	}
}

func TestWorkspaceNameLosesToAnExplicitTag(t *testing.T) {
	r := NewResolver(defaultGrouping(), map[string]string{"/w/a": "bnpl-rollout"})

	got := r.Resolve([]Input{{Path: "/w/a", Branch: "master", Workspace: "wayf-20764", RepoRoot: "/r/a"}})

	if got["/w/a"].Source != SourceExplicit {
		t.Fatalf("source = %v, want SourceExplicit", got["/w/a"].Source)
	}
}

func TestResolveGroupsAcrossRepositories(t *testing.T) {
	r := NewResolver(defaultGrouping(), nil)

	got := r.Resolve([]Input{
		{Path: "/w/cds/wayf-20748", Branch: "WAYF-20748", RepoRoot: "/r/credit-decision-srvc"},
		{Path: "/w/bnpl/wayf-20748", Branch: "WAYF-20748-engine", RepoRoot: "/r/bnpl-decision-engine"},
		{Path: "/w/qa/wayf-20748", Branch: "worktree/WAYF-20748", RepoRoot: "/r/qa-automation"},
		{Path: "/w/k8s/other", Branch: "WAYF-19000", RepoRoot: "/r/k8s-template"},
	})

	for _, path := range []string{"/w/cds/wayf-20748", "/w/bnpl/wayf-20748", "/w/qa/wayf-20748"} {
		if got[path].Name != "WAYF-20748" {
			t.Fatalf("%s did not land in WAYF-20748: %+v", path, got[path])
		}
	}

	if got["/w/k8s/other"].Name != "WAYF-19000" {
		t.Fatalf("unrelated ticket was folded in: %+v", got["/w/k8s/other"])
	}
}

func TestResolveExplicitTagWinsOverDerivation(t *testing.T) {
	explicit := map[string]string{"/w/cds/wayf-20748": "bnpl-rollout"}
	r := NewResolver(defaultGrouping(), explicit)

	got := r.Resolve([]Input{{Path: "/w/cds/wayf-20748", Branch: "WAYF-20748", RepoRoot: "/r/cds"}})

	if got["/w/cds/wayf-20748"].Name != "bnpl-rollout" {
		t.Fatalf("explicit tag lost to the ticket key: %+v", got["/w/cds/wayf-20748"])
	}

	if got["/w/cds/wayf-20748"].Source != SourceExplicit {
		t.Fatalf("source = %v, want SourceExplicit", got["/w/cds/wayf-20748"].Source)
	}
}

func TestResolveBlankExplicitTagIsIgnored(t *testing.T) {
	// An emptied tag must fall through to derivation rather than pin the
	// worktree to a nameless group.
	r := NewResolver(defaultGrouping(), map[string]string{"/w/a": "   "})

	got := r.Resolve([]Input{{Path: "/w/a", Branch: "WAYF-1", RepoRoot: "/r/a"}})

	if got["/w/a"].Source != SourceJira {
		t.Fatalf("blank tag was honoured: %+v", got["/w/a"])
	}
}

func TestResolveBranchPatternAfterTicketKey(t *testing.T) {
	cfg := defaultGrouping()
	cfg.BranchPatterns = []string{`^feature/(?P<group>[^/]+)/`}

	r := NewResolver(cfg, nil)

	got := r.Resolve([]Input{
		{Path: "/w/a", Branch: "feature/checkout-v2/api", RepoRoot: "/r/a"},
		{Path: "/w/b", Branch: "WAYF-42", RepoRoot: "/r/b"},
	})

	if got["/w/a"].Name != "checkout-v2" || got["/w/a"].Source != SourceBranchPattern {
		t.Fatalf("branch pattern did not match: %+v", got["/w/a"])
	}

	if got["/w/b"].Source != SourceJira {
		t.Fatalf("ticket key should still win where it matches: %+v", got["/w/b"])
	}
}

func TestResolveSharedNameNeedsTwoRepositories(t *testing.T) {
	r := NewResolver(defaultGrouping(), nil)

	got := r.Resolve([]Input{
		{Path: "/w/cds/spike", Branch: "spike", RepoRoot: "/r/cds"},
		{Path: "/w/bnpl/spike", Branch: "spike", RepoRoot: "/r/bnpl"},
		{Path: "/w/k8s/lonely", Branch: "lonely", RepoRoot: "/r/k8s"},
	})

	if got["/w/cds/spike"].Name != "spike" || got["/w/cds/spike"].Source != SourceWorktreeName {
		t.Fatalf("shared name did not group: %+v", got["/w/cds/spike"])
	}

	if got["/w/k8s/lonely"].Source != SourceNone {
		t.Fatalf("a name used by one repo became a group: %+v", got["/w/k8s/lonely"])
	}
}

func TestResolveSharedNameIgnoresTwoCheckoutsInOneRepo(t *testing.T) {
	// Two checkouts of the same repo cannot share a directory name in
	// practice, but a case-only difference could sneak past — and grouping a
	// repo with itself is never what the user meant.
	r := NewResolver(defaultGrouping(), nil)

	got := r.Resolve([]Input{
		{Path: "/w/cds/Spike", Branch: "a", RepoRoot: "/r/cds"},
		{Path: "/w/cds2/spike", Branch: "b", RepoRoot: "/r/cds"},
	})

	if got["/w/cds/Spike"].Source != SourceNone {
		t.Fatalf("same-repo checkouts were grouped by name: %+v", got)
	}
}

func TestResolveSharedNameIsCaseInsensitive(t *testing.T) {
	r := NewResolver(defaultGrouping(), nil)

	got := r.Resolve([]Input{
		{Path: "/w/cds/Spike", Branch: "a", RepoRoot: "/r/cds"},
		{Path: "/w/bnpl/spike", Branch: "b", RepoRoot: "/r/bnpl"},
	})

	if got["/w/cds/Spike"].Name == "" || got["/w/bnpl/spike"].Name == "" {
		t.Fatalf("case difference split the group: %+v", got)
	}

	if got["/w/cds/Spike"].Name != got["/w/bnpl/spike"].Name {
		t.Fatalf("group names disagree: %q vs %q", got["/w/cds/Spike"].Name, got["/w/bnpl/spike"].Name)
	}
}

func TestResolveSharedNameDisabled(t *testing.T) {
	cfg := defaultGrouping()
	cfg.UseWorktreeName = false

	r := NewResolver(cfg, nil)

	got := r.Resolve([]Input{
		{Path: "/w/cds/spike", Branch: "spike", RepoRoot: "/r/cds"},
		{Path: "/w/bnpl/spike", Branch: "spike", RepoRoot: "/r/bnpl"},
	})

	if got["/w/cds/spike"].Source != SourceNone {
		t.Fatalf("use_worktree_name = false still grouped by name: %+v", got)
	}
}

func TestResolveAssignsEveryInput(t *testing.T) {
	// The pane indexes the result by path; a missing entry would drop a row.
	r := NewResolver(defaultGrouping(), nil)

	inputs := []Input{
		{Path: "/w/a", Branch: "WAYF-1", RepoRoot: "/r/a"},
		{Path: "/w/b", Branch: "", RepoRoot: "/r/b"},
		{Path: "/w/c", Branch: "junk", RepoRoot: "/r/c"},
	}

	got := r.Resolve(inputs)

	if len(got) != len(inputs) {
		t.Fatalf("resolved %d of %d inputs: %+v", len(got), len(inputs), got)
	}
}

func TestResolvePatternWithoutCaptureUsesWholeMatch(t *testing.T) {
	cfg := defaultGrouping()
	cfg.JiraPattern = `^[A-Z]+-[0-9]+`

	r := NewResolver(cfg, nil)

	got := r.Resolve([]Input{{Path: "/w/a", Branch: "WAYF-20748-thing", RepoRoot: "/r/a"}})

	if got["/w/a"].Name != "WAYF-20748" {
		t.Fatalf("whole-match fallback failed: %q", got["/w/a"].Name)
	}
}

func TestWithLabelsRenamesOnlyWhatHasAnOverride(t *testing.T) {
	assignments := map[string]Assignment{
		"/w/a": {Name: "WAYF-1", Source: SourceJira},
		"/w/b": {Name: "WAYF-2", Source: SourceJira},
	}

	got := WithLabels(assignments, map[string]string{"WAYF-1": "Checkout redesign"})

	if got["/w/a"].Name != "Checkout redesign" {
		t.Fatalf("renamed assignment = %q, want the override", got["/w/a"].Name)
	}

	if got["/w/a"].Source != SourceJira {
		t.Fatalf("renaming should not touch Source, got %v", got["/w/a"].Source)
	}

	if got["/w/b"].Name != "WAYF-2" {
		t.Fatalf("an assignment with no override should be unchanged, got %q", got["/w/b"].Name)
	}
}

func TestWithLabelsNoOverridesAnswersTheSameMap(t *testing.T) {
	assignments := map[string]Assignment{"/w/a": {Name: "WAYF-1", Source: SourceJira}}

	got := WithLabels(assignments, nil)

	if got["/w/a"].Name != "WAYF-1" {
		t.Fatalf("assignment = %q, want it untouched", got["/w/a"].Name)
	}
}
