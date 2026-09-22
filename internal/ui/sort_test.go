package ui

import (
	"strings"
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// sortable holds three features whose alphabetical, status and agent orders all
// differ, so a mode that did nothing would be visible.
func sortable() (collect.World, map[string]group.Assignment) {
	seq := func(w collect.Worktree, n int64) collect.Worktree {
		w.AgentSeq = n

		return w
	}

	w := world(
		// alpha: 1st, status: idle, agent: oldest
		seq(wt("/w/a", "cds", "AAA-1", "w1", herdr.StatusIdle), 10),
		// alpha: 2nd, status: blocked, agent: middle
		seq(wt("/w/b", "bnpl", "BBB-2", "w2", herdr.StatusBlocked), 50),
		// alpha: 3rd, status: working, agent: newest
		seq(wt("/w/c", "k8s", "CCC-3", "w3", herdr.StatusWorking), 90),
		// and one nothing claimed, which sinks whatever the mode
		seq(wt("/w/z", "misc", "master", "w4", herdr.StatusBlocked), 99),
	)

	return w, assign(map[string]group.Assignment{
		"/w/a": {Name: "AAA-1", Source: group.SourceJira},
		"/w/b": {Name: "BBB-2", Source: group.SourceJira},
		"/w/c": {Name: "CCC-3", Source: group.SourceJira},
		"/w/z": {Source: group.SourceNone},
	})
}

func headerNames(rows []row) []string {
	var out []string

	for _, r := range rows {
		if r.kind == rowGroup {
			out = append(out, r.name)
		}
	}

	return out
}

// orderings is every step of the s cycle, in order.
func orderings() []ordering {
	var (
		out []ordering
		o   ordering
	)

	for range len(sortModes) * 2 {
		out = append(out, o)
		o = o.next()
	}

	return out
}

func TestSortModesOrderTheFeatures(t *testing.T) {
	w, assignments := sortable()

	cases := []struct {
		order ordering
		want  []string
	}{
		{ordering{}, []string{"AAA-1", "BBB-2", "CCC-3", ungroupedLabel}},
		{ordering{desc: true}, []string{"CCC-3", "BBB-2", "AAA-1", ungroupedLabel}},
		// Ascending is the useful end of each mode: blocked first, then working,
		// then idle.
		{ordering{mode: sortStatus}, []string{"BBB-2", "CCC-3", "AAA-1", ungroupedLabel}},
		{ordering{mode: sortStatus, desc: true}, []string{"AAA-1", "CCC-3", "BBB-2", ungroupedLabel}},
		// Highest state-change sequence first.
		{ordering{mode: sortAgent}, []string{"CCC-3", "BBB-2", "AAA-1", ungroupedLabel}},
		{ordering{mode: sortAgent, desc: true}, []string{"AAA-1", "BBB-2", "CCC-3", ungroupedLabel}},
	}

	for _, tc := range cases {
		t.Run(tc.order.String(), func(t *testing.T) {
			got := headerNames(buildRows(w, assignments, map[string]bool{}, tc.order))

			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("order = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestDescendingIsTheExactReverse(t *testing.T) {
	w, assignments := sortable()

	for _, mode := range sortModes {
		asc := headerNames(buildRows(w, assignments, map[string]bool{}, ordering{mode: mode}))
		desc := headerNames(buildRows(w, assignments, map[string]bool{}, ordering{mode: mode, desc: true}))

		// Ungrouped is pinned last in both, so only the features above it flip.
		asc, desc = asc[:len(asc)-1], desc[:len(desc)-1]

		for i := range asc {
			if asc[i] != desc[len(desc)-1-i] {
				t.Errorf("%s: asc %v is not the reverse of desc %v", mode, asc, desc)

				break
			}
		}
	}
}

func TestUngroupedSinksInEveryOrdering(t *testing.T) {
	// Its member is blocked and the most recently active of the lot, so both
	// non-alphabetical modes would put it first if it were an ordinary feature —
	// and reversing the sort is not a reason to bury the real ones under it.
	w, assignments := sortable()

	for _, order := range orderings() {
		got := headerNames(buildRows(w, assignments, map[string]bool{}, order))

		if last := got[len(got)-1]; last != ungroupedLabel {
			t.Errorf("%s: last feature = %q, want %q", order, last, ungroupedLabel)
		}
	}
}

func TestSortModesOrderTheWorktreesInsideAFeature(t *testing.T) {
	seq := func(w collect.Worktree, n int64) collect.Worktree {
		w.AgentSeq = n

		return w
	}

	w := world(
		seq(wt("/w/a", "aaa", "CRD-1", "w1", herdr.StatusIdle), 10),
		seq(wt("/w/b", "bbb", "CRD-1", "w2", herdr.StatusBlocked), 20),
		seq(wt("/w/c", "ccc", "CRD-1", "w3", herdr.StatusWorking), 90),
	)

	assignments := assign(map[string]group.Assignment{
		"/w/a": {Name: "CRD-1", Source: group.SourceJira},
		"/w/b": {Name: "CRD-1", Source: group.SourceJira},
		"/w/c": {Name: "CRD-1", Source: group.SourceJira},
	})

	members := func(order ordering) []string {
		var out []string

		for _, r := range buildRows(w, assignments, map[string]bool{}, order) {
			if r.kind == rowWorktree {
				out = append(out, r.worktree.RepoName)
			}
		}

		return out
	}

	cases := []struct {
		order ordering
		want  string
	}{
		{ordering{}, "aaa,bbb,ccc"},
		{ordering{desc: true}, "ccc,bbb,aaa"},
		{ordering{mode: sortStatus}, "bbb,ccc,aaa"},
		{ordering{mode: sortStatus, desc: true}, "aaa,ccc,bbb"},
		{ordering{mode: sortAgent}, "ccc,bbb,aaa"},
		{ordering{mode: sortAgent, desc: true}, "aaa,bbb,ccc"},
	}

	for _, tc := range cases {
		if got := strings.Join(members(tc.order), ","); got != tc.want {
			t.Errorf("%s: members = %s, want %s", tc.order, got, tc.want)
		}
	}
}

func TestSortIsStableBetweenRefreshes(t *testing.T) {
	// Two features nothing distinguishes must not swap places, or the list
	// reshuffles itself every time r is pressed.
	w := world(
		wt("/w/a", "aaa", "AAA-1", "w1", herdr.StatusIdle),
		wt("/w/b", "bbb", "BBB-2", "w2", herdr.StatusIdle),
	)

	assignments := assign(map[string]group.Assignment{
		"/w/a": {Name: "AAA-1", Source: group.SourceJira},
		"/w/b": {Name: "BBB-2", Source: group.SourceJira},
	})

	for _, order := range orderings() {
		first := strings.Join(headerNames(buildRows(w, assignments, map[string]bool{}, order)), ",")

		for range 5 {
			again := strings.Join(headerNames(buildRows(w, assignments, map[string]bool{}, order)), ",")
			if again != first {
				t.Fatalf("%s: order changed from %s to %s between builds", order, first, again)
			}
		}
	}
}

func TestSortCycleWalksBothDirectionsOfEachMode(t *testing.T) {
	var got []string

	o := ordering{}
	for range len(sortModes) * 2 {
		got = append(got, o.String())
		o = o.next()
	}

	want := []string{
		"alpha asc", "alpha desc",
		"status asc", "status desc",
		"agent asc", "agent desc",
	}

	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("cycle = %v, want %v", got, want)
	}

	// And one more press comes back round to the start.
	if o != (ordering{}) {
		t.Errorf("after a full cycle: %s, want alpha asc", o)
	}
}

func TestParseOrderingFallsBackToTheDefault(t *testing.T) {
	for _, order := range orderings() {
		if got := parseOrdering(order.String()); got != order {
			t.Errorf("%s parsed as %s", order, got)
		}
	}

	for _, name := range []string{"", "nonsense", "Alpha", "alpha sideways"} {
		if got := parseOrdering(name); got != (ordering{}) {
			t.Errorf("%q parsed as %s, want alpha asc", name, got)
		}
	}
}

func TestCycleSortRebuildsBothViews(t *testing.T) {
	w, assignments := sortable()

	m := cardsOver(w, assignments, 40, 30)
	m.view = viewList

	// Two presses: alpha asc → alpha desc → status asc.
	sorted := press(press(m, "s"), "s")
	if want := (ordering{mode: sortStatus}); sorted.order != want {
		t.Fatalf("order = %s, want %s", sorted.order, want)
	}

	if headerNames(sorted.rows)[0] != "BBB-2" {
		t.Errorf("list = %v, want the blocked feature first", headerNames(sorted.rows))
	}

	// The grid is built from the same buckets, so it has to agree.
	if sorted.cards[0].name != "BBB-2" {
		t.Errorf("cards = %+v, want the blocked feature first", sorted.cards[0].name)
	}
}

func TestCycleSortReturnsToTheTop(t *testing.T) {
	// The cursor was on a row that is somewhere else now; the point of
	// re-sorting is that the first entry is the interesting one.
	w, assignments := sortable()

	m := cardsOver(w, assignments, 40, 30)
	m.view = viewList
	m.cursor = 4
	m.cardCursor = 2

	sorted := press(m, "s")
	if sorted.cursor != 0 || sorted.cardCursor != 0 {
		t.Fatalf("cursors = %d/%d, want both at the top", sorted.cursor, sorted.cardCursor)
	}
}

func TestFooterNamesTheCurrentSort(t *testing.T) {
	w, assignments := sortable()

	m := cardsOver(w, assignments, 80, 30)
	m.view = viewList

	// Both halves: the mode and the direction, so a reversed list says so.
	for _, order := range orderings() {
		m.order = order

		if !strings.Contains(m.footer(80), order.String()) {
			t.Errorf("the footer should name the %s sort: %q", order, m.footer(80))
		}
	}
}
