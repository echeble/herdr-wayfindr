package ui

import (
	"strings"
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

func world(worktrees ...collect.Worktree) collect.World {
	return collect.World{Worktrees: worktrees}
}

func wt(path, repo, branch, workspace, status string) collect.Worktree {
	w := collect.Worktree{
		Path:        path,
		RepoName:    repo,
		Branch:      branch,
		WorkspaceID: workspace,
		AgentStatus: status,
	}

	// An open workspace in a known state has an agent in it to be in that
	// state; a closed checkout has none, which is what leaves its tally zero.
	// Collect builds the status from the tally, so a fixture where the two
	// disagree is not a shape the pane ever sees.
	if workspace != "" && status != "" && status != herdr.StatusUnknown {
		w.Agents = w.Agents.Plus(status)
	}

	return w
}

// crew adds agents to a worktree beyond the one wt gives it, for the cases
// where how many are in a state is the thing under test.
func crew(w collect.Worktree, statuses ...string) collect.Worktree {
	for _, status := range statuses {
		w.Agents = w.Agents.Plus(status)
	}

	return w
}

func assign(pairs map[string]group.Assignment) map[string]group.Assignment { return pairs }

func TestBuildRowsGroupsAndOrders(t *testing.T) {
	w := world(
		wt("/w/b", "bnpl", "WAYF-2", "w2", herdr.StatusIdle),
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/z", "k8s", "master", "", herdr.StatusUnknown),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-2", Source: group.SourceJira},
		"/w/z": {Source: group.SourceNone},
	}), map[string]bool{}, ordering{})

	var headers []string

	for _, r := range rows {
		if r.kind == rowGroup {
			headers = append(headers, r.name)
		}
	}

	want := []string{"WAYF-1", "WAYF-2", ungroupedLabel}
	if strings.Join(headers, ",") != strings.Join(want, ",") {
		t.Fatalf("headers = %v, want %v", headers, want)
	}
}

func TestBuildRowsPutsUngroupedLast(t *testing.T) {
	// Even when its name would sort first alphabetically.
	w := world(
		wt("/w/a", "cds", "ZZZ-1", "w1", herdr.StatusIdle),
		wt("/w/z", "k8s", "master", "", herdr.StatusUnknown),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "ZZZ-1", Source: group.SourceJira},
		"/w/z": {Source: group.SourceNone},
	}), map[string]bool{}, ordering{})

	last := rows[len(rows)-1]
	if last.kind != rowWorktree || last.worktree.Path != "/w/z" {
		t.Fatalf("ungrouped members should be last, got %+v", last)
	}
}

func TestBuildRowsCollapseHidesMembers(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle),
	)

	assignments := assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
	})

	expanded := buildRows(w, assignments, map[string]bool{}, ordering{})
	if len(expanded) != 3 {
		t.Fatalf("expanded rows = %d, want 3", len(expanded))
	}

	collapsed := buildRows(w, assignments, map[string]bool{"WAYF-1": true}, ordering{})
	if len(collapsed) != 1 {
		t.Fatalf("collapsed rows = %d, want 1", len(collapsed))
	}

	if !collapsed[0].collapsed {
		t.Error("the header should know it is collapsed")
	}

	// The count has to survive collapsing — it is the only thing left to read.
	if collapsed[0].members != 2 {
		t.Errorf("members = %d, want 2", collapsed[0].members)
	}
}

func TestBuildRowsCountsOpenMembers(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "", herdr.StatusUnknown),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
	}), map[string]bool{}, ordering{})

	if rows[0].openCount != 1 || rows[0].members != 2 {
		t.Fatalf("open/members = %d/%d, want 1/2", rows[0].openCount, rows[0].members)
	}
}

func TestBuildRowsHeaderShowsMostUrgentStatus(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusBlocked),
		wt("/w/c", "k8s", "WAYF-1", "w3", herdr.StatusWorking),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1"},
		"/w/b": {Name: "WAYF-1"},
		"/w/c": {Name: "WAYF-1"},
	}), map[string]bool{}, ordering{})

	if rows[0].status != herdr.StatusBlocked {
		t.Fatalf("header status = %q, want blocked so it is visible while folded", rows[0].status)
	}
}

func TestBuildRowsExplicitTagWinsTheHeaderSource(t *testing.T) {
	// One member tagged by hand makes the whole group an explicit one, so it
	// is not rendered as a guess.
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "other", "w2", herdr.StatusIdle),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "release", Source: group.SourceJira},
		"/w/b": {Name: "release", Source: group.SourceExplicit},
	}), map[string]bool{}, ordering{})

	if rows[0].source != group.SourceExplicit {
		t.Fatalf("header source = %v, want SourceExplicit", rows[0].source)
	}
}

func TestBuildRowsSumsAGroupsAgents(t *testing.T) {
	// A feature's agent line counts every agent across its worktrees, not the
	// worktrees that have one: two checkouts with two agents each is four.
	w := world(
		crew(wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusWorking), herdr.StatusWorking),
		crew(wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusBlocked), herdr.StatusWorking),
		// A closed checkout brings nothing to the count.
		wt("/w/c", "k8s", "WAYF-1", "", herdr.StatusUnknown),
	)

	rows := buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/c": {Name: "WAYF-1", Source: group.SourceJira},
	}), map[string]bool{}, ordering{})

	header := rows[0]

	if got := header.agents.Total(); got != 4 {
		t.Errorf("total agents = %d, want 4", got)
	}

	if got := header.agents.Working; got != 3 {
		t.Errorf("working = %d, want 3", got)
	}

	// The header is labelled blocked — the most urgent state among its members
	// — and so reports the one agent actually in it.
	if header.status != herdr.StatusBlocked {
		t.Fatalf("header status = %q, want blocked", header.status)
	}

	if got := agentWord(header.status, header.agents, header.openCount > 0); got != "1 agent blocked" {
		t.Errorf("agent word = %q, want %q", got, "1 agent blocked")
	}
}

func TestBuildRowsGivesAGroupItsPullRequestState(t *testing.T) {
	withPR := func(w collect.Worktree, state collect.PRState) collect.Worktree {
		w.PR = collect.PR{State: state}

		return w
	}

	cases := []struct {
		name   string
		states []collect.PRState
		want   collect.PRState
	}{
		// Merged needs every member: the marker means the feature has landed.
		{"every member merged", []collect.PRState{collect.PRMerged, collect.PRMerged}, collect.PRMerged},
		{"one still open", []collect.PRState{collect.PRMerged, collect.PROpen}, collect.PROpen},
		{"one has no pull request", []collect.PRState{collect.PRMerged, collect.PRNone}, collect.PRUnknown},
		{"one could not be looked up", []collect.PRState{collect.PRMerged, collect.PRUnknown}, collect.PRUnknown},
		{"one closed unmerged", []collect.PRState{collect.PRMerged, collect.PRClosed}, collect.PRClosed},
		// An open pull request outranks an abandoned one: what matters about an
		// unfinished feature is whether anything is still in review.
		{"open beats closed", []collect.PRState{collect.PRClosed, collect.PROpen}, collect.PROpen},
		{"nothing to show", []collect.PRState{collect.PRNone, collect.PRNone}, collect.PRUnknown},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := world(
				withPR(wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle), tc.states[0]),
				withPR(wt("/w/b", "bnpl", "WAYF-1", "w2", herdr.StatusIdle), tc.states[1]),
			)

			rows := buildRows(w, assign(map[string]group.Assignment{
				"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
				"/w/b": {Name: "WAYF-1", Source: group.SourceJira},
			}), map[string]bool{}, ordering{})

			if rows[0].pr != tc.want {
				t.Fatalf("group state = %v, want %v", rows[0].pr, tc.want)
			}
		})
	}
}

func TestBuildRowsEmptyWorld(t *testing.T) {
	if rows := buildRows(collect.World{}, nil, map[string]bool{}, ordering{}); len(rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(rows))
	}
}

func TestDisplayNameFallsBackToTheResolvedName(t *testing.T) {
	// A row built straight from the resolver — in a test, or before applyLabels
	// has run — has no override yet, and has to read exactly as it always did.
	r := row{kind: rowGroup, name: "WAYF-1"}

	if got := r.displayName(); got != "WAYF-1" {
		t.Fatalf("displayName = %q, want WAYF-1", got)
	}
}

func TestDisplayNamePrefersTheLabel(t *testing.T) {
	r := row{kind: rowGroup, name: "WAYF-1", label: "Checkout redesign"}

	if got := r.displayName(); got != "Checkout redesign" {
		t.Fatalf("displayName = %q, want the override", got)
	}
}

func TestApplyLabelsOnlyTouchesMatchingHeaders(t *testing.T) {
	w := world(
		wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bnpl", "WAYF-2", "w2", herdr.StatusIdle),
	)

	rows := applyLabels(buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-2", Source: group.SourceJira},
	}), map[string]bool{}, ordering{}), map[string]string{"WAYF-1": "Checkout redesign"})

	for _, r := range rows {
		if r.kind != rowGroup {
			continue
		}

		switch r.name {
		case "WAYF-1":
			if r.displayName() != "Checkout redesign" {
				t.Errorf("WAYF-1 header = %q, want the override", r.displayName())
			}
		case "WAYF-2":
			if r.displayName() != "WAYF-2" {
				t.Errorf("WAYF-2 header = %q, want it unaffected by another group's rename", r.displayName())
			}
		}
	}
}

func TestApplyLabelsLeavesWorktreeRowsAlone(t *testing.T) {
	w := world(wt("/w/a", "cds", "WAYF-1", "w1", herdr.StatusIdle))

	rows := applyLabels(buildRows(w, assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-1", Source: group.SourceJira},
	}), map[string]bool{}, ordering{}), map[string]string{"WAYF-1": "Checkout redesign"})

	for _, r := range rows {
		if r.kind == rowWorktree && r.label != "" {
			t.Errorf("a worktree row should never carry a label, got %q", r.label)
		}
	}
}
