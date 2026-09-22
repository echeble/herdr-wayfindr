package ui

import (
	"strings"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// ungroupedLabel heads the bucket for worktrees no rule claimed. It sorts last.
const ungroupedLabel = "Ungrouped"

type rowKind int

const (
	rowGroup rowKind = iota
	rowWorktree
)

// row is one printed line. The list is flattened rather than nested so the
// cursor is a single index and collapsing is just a rebuild.
type row struct {
	kind rowKind

	// group fields
	name string
	// label is the header's rename override, when it has one. It is never set
	// by buildRows itself — see applyLabels — so a row built straight from the
	// resolver, in a test or otherwise, always falls back to name.
	label     string
	source    group.Source
	members   int
	openCount int
	status    string
	// agents is every member's tally summed, so the header can say how many
	// agents are in the state it is showing rather than only naming it.
	agents    collect.AgentTally
	collapsed bool
	// pr is the feature's pull-request state: merged only when every worktree
	// under it is, so the done marker means the feature has landed everywhere
	// it touches.
	pr collect.PRState

	// worktree fields
	worktree *collect.Worktree
}

// displayName is what a header actually shows: the rename override when there
// is one, the group's own resolved name otherwise. Everything that keys off a
// group's identity instead — folding, colour, the feature a card drills into,
// the branch-already-says-this check on a member row — reads name directly,
// never this, so a rename cannot disturb any of them.
func (r row) displayName() string {
	if r.label != "" {
		return r.label
	}

	return r.name
}

// applyLabels stamps the rename override onto every header already built, so
// a rename never re-touches how rows were assigned, ordered, or folded — it
// runs over them afterward, purely to fill in what each one is titled.
func applyLabels(rows []row, labels map[string]string) []row {
	for i := range rows {
		if rows[i].kind != rowGroup {
			continue
		}

		if label := strings.TrimSpace(labels[rows[i].name]); label != "" {
			rows[i].label = label
		}
	}

	return rows
}

type groupBucket struct {
	name    string
	source  group.Source
	members []*collect.Worktree
}

// buildRows turns a world plus its group assignments into the flat row list.
func buildRows(world collect.World, assignments map[string]group.Assignment, collapsed map[string]bool, order ordering) []row {
	var rows []row

	for _, b := range buildBuckets(world, assignments, order) {
		isCollapsed := collapsed[b.name]

		rows = append(rows, headerRow(b, isCollapsed))

		if isCollapsed {
			continue
		}

		for _, wt := range b.members {
			rows = append(rows, row{kind: rowWorktree, name: b.name, worktree: wt})
		}
	}

	return rows
}

// buildBuckets groups a world's worktrees by the feature they were assigned to,
// in the order they are drawn: whatever the sort mode says, with the ungrouped
// bucket last.
//
// The list and the card grid are both built from it, so the two always agree on
// what a feature is, which worktrees are in it, and where it sits.
func buildBuckets(world collect.World, assignments map[string]group.Assignment, order ordering) []*groupBucket {
	buckets := map[string]*groupBucket{}

	for i := range world.Worktrees {
		wt := &world.Worktrees[i]

		assignment := assignments[wt.Path]

		name := assignment.Name
		if name == "" {
			name = ungroupedLabel
		}

		b := buckets[name]
		if b == nil {
			b = &groupBucket{name: name, source: assignment.Source}
			buckets[name] = b
		}

		// A group holding both an explicit tag and a derived match is an
		// explicit group: the user named it.
		if assignment.Source == group.SourceExplicit {
			b.source = group.SourceExplicit
		}

		b.members = append(b.members, wt)
	}

	ordered := make([]*groupBucket, 0, len(buckets))
	for _, b := range buckets {
		ordered = append(ordered, b)
	}

	sortBuckets(ordered, order)

	for _, b := range ordered {
		sortMembers(b.members, order)
	}

	return ordered
}

// headerRow is the feature itself: the list draws it as a group line and the
// grid draws it as a card, from the same summary of its members.
func headerRow(b *groupBucket, collapsed bool) row {
	header := row{
		kind:      rowGroup,
		name:      b.name,
		source:    b.source,
		members:   len(b.members),
		status:    aggregateStatus(b.members),
		collapsed: collapsed,
		pr:        groupPRState(b.members),
	}

	for _, wt := range b.members {
		if wt.Open() {
			header.openCount++
		}

		// A closed checkout has no workspace and so no agents; merging it in is
		// a no-op rather than a case worth guarding.
		header.agents = header.agents.Merge(wt.Agents)
	}

	return header
}

// groupPRState is the feature's pull-request state, from its members'.
func groupPRState(members []*collect.Worktree) collect.PRState {
	states := make([]collect.PRState, 0, len(members))
	for _, wt := range members {
		states = append(states, wt.PR.State)
	}

	return collect.AggregatePR(states)
}

// aggregateStatus answers the state a group header shows: the most urgent one
// among its members, so a blocked agent anywhere is visible with the group
// collapsed.
func aggregateStatus(members []*collect.Worktree) string {
	rank := map[string]int{
		herdr.StatusBlocked: 5,
		herdr.StatusWorking: 4,
		herdr.StatusDone:    3,
		herdr.StatusIdle:    2,
		herdr.StatusUnknown: 1,
	}

	best := herdr.StatusUnknown

	for _, wt := range members {
		if rank[wt.AgentStatus] > rank[best] {
			best = wt.AgentStatus
		}
	}

	return best
}
