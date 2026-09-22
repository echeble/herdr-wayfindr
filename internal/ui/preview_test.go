package ui

import (
	"fmt"
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// TestPreview is not an assertion, it is a way to look at the pane. Run it with
// -run TestPreview -v to see the grid and the list at a few pane widths.
func TestPreview(t *testing.T) {
	pr := func(w collect.Worktree, state collect.PRState) collect.Worktree {
		w.PR = collect.PR{State: state}

		return w
	}

	dirty := func(w collect.Worktree, staged, unstaged, ahead int) collect.Worktree {
		w.Git = collect.GitStatus{Known: true, Staged: staged, Unstaged: unstaged, Ahead: ahead}

		return w
	}

	seq := func(w collect.Worktree, n int64) collect.Worktree {
		w.AgentSeq = n

		return w
	}

	w := world(
		seq(pr(dirty(crew(wt("/w/a", "marketplace-billing-service", "WAYF-20748", "w1", herdr.StatusWorking), herdr.StatusWorking), 1, 2, 3), collect.PROpen), 40),
		seq(pr(wt("/w/b", "marketplace-catalog-service", "worktree-WAYF-20748", "w2", herdr.StatusBlocked), collect.PRMerged), 10),
		pr(wt("/w/c", "k8s-template", "WAYF-20748", "", herdr.StatusUnknown), collect.PRNone),
		seq(pr(wt("/w/d", "credit-report-srvc", "WAYF-20099", "w4", herdr.StatusDone), collect.PRMerged), 90),
		seq(wt("/w/e", "wayfindr", "0.8.0", "w5", herdr.StatusIdle), 70),
	)

	assignments := assign(map[string]group.Assignment{
		"/w/a": {Name: "WAYF-20748", Source: group.SourceJira},
		"/w/b": {Name: "WAYF-20748", Source: group.SourceJira},
		"/w/c": {Name: "WAYF-20748", Source: group.SourceJira},
		"/w/d": {Name: "WAYF-20099", Source: group.SourceJira},
		"/w/e": {Source: group.SourceNone},
	})

	for _, width := range []int{40, 62} {
		m := cardsOver(w, assignments, width, 24)
		m.selfWorkspace = "w1"

		fmt.Printf("\n=== cards, %d columns ===\n%s\n", width, m.View())

		inside := press(press(m, "right"), "enter")
		fmt.Printf("\n=== inside WAYF-20748, %d columns ===\n%s\n", width, inside.View())
	}

	list := cardsOver(w, assignments, 40, 24)
	list.view = viewList
	list.selfWorkspace = "w1"

	fmt.Printf("\n=== list, 40 columns ===\n%s\n", list.View())

	order := ordering{}
	for range len(sortModes) * 2 {
		sorted := cardsOver(w, assignments, 40, 24)
		sorted.view = viewList
		sorted.order = order
		sorted.rebuild()

		fmt.Printf("\n=== list sorted by %s ===\n%s\n", order, sorted.View())

		order = order.next()
	}

	// A grid with more features than fit, to show the scroll mark.
	scrolled := gridModel(12, 40, 16)
	fmt.Printf("\n=== cards, scrollable ===\n%s\n", scrolled.View())

	scrolled.cardCursor = 11
	scrolled.clampCard()
	fmt.Printf("\n=== cards, scrolled to the end ===\n%s\n", scrolled.View())
}
