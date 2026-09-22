package ui

import (
	"sort"
	"strings"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// sortMode is what features are ordered by. It orders the worktrees inside a
// feature by the same rule, so the answer to "what should I look at first" is
// the same at both levels.
type sortMode int

const (
	sortAlpha sortMode = iota
	sortStatus
	sortAgent
)

// sortModes is the cycle s walks, in order.
var sortModes = []sortMode{sortAlpha, sortStatus, sortAgent}

func (s sortMode) String() string {
	switch s {
	case sortStatus:
		return "status"
	case sortAgent:
		return "agent"
	default:
		return "alpha"
	}
}

func (s sortMode) next() sortMode {
	for i, mode := range sortModes {
		if mode == s {
			return sortModes[(i+1)%len(sortModes)]
		}
	}

	return sortAlpha
}

// ordering is a mode and a direction: what s cycles through and what the pane
// remembers between sessions.
//
// Ascending is each mode's useful end rather than its numerically smaller one —
// A first, the most urgent agent first, the most recent first — because that is
// what you want when you pick a mode at all. Descending is the exact reverse of
// it, so the two together answer "which is furthest along" as well as "which
// needs me now".
type ordering struct {
	mode sortMode
	desc bool
}

// next steps through both directions of a mode before moving on to the next
// one: alpha asc, alpha desc, status asc, and so on back round.
func (o ordering) next() ordering {
	if !o.desc {
		return ordering{mode: o.mode, desc: true}
	}

	return ordering{mode: o.mode.next()}
}

func (o ordering) String() string {
	if o.desc {
		return o.mode.String() + " desc"
	}

	return o.mode.String() + " asc"
}

// parseOrdering reads a remembered ordering. Anything it does not recognise
// falls back to the default rather than being reported, the same way a bad
// config value does — a preference file is not worth an error in the footer.
func parseOrdering(name string) ordering {
	mode, direction, _ := strings.Cut(name, " ")

	for _, candidate := range sortModes {
		if candidate.String() == mode {
			return ordering{mode: candidate, desc: direction == "desc"}
		}
	}

	return ordering{}
}

// statusRank orders the agent states by how much they want the user. It is the
// same ranking the group headers aggregate with, so a feature sorts where its
// most urgent member puts it.
var statusRank = map[string]int{
	herdr.StatusBlocked: 5,
	herdr.StatusWorking: 4,
	herdr.StatusDone:    3,
	herdr.StatusIdle:    2,
	herdr.StatusUnknown: 1,
}

// sortBuckets orders the features. Ungrouped sinks to the bottom whatever the
// mode and whatever the direction: it is the bucket you look at last by
// definition, and reversing the sort is not a reason to bury the real features
// under it.
func sortBuckets(buckets []*groupBucket, order ordering) {
	sort.SliceStable(buckets, func(i, j int) bool {
		a, b := buckets[i], buckets[j]

		if (a.name == ungroupedLabel) != (b.name == ungroupedLabel) {
			return b.name == ungroupedLabel
		}

		// Descending is the same comparison with the pair swapped rather than
		// its result negated: negating would make equal entries compare true
		// both ways round, which is not an ordering at all.
		if order.desc {
			a, b = b, a
		}

		switch order.mode {
		case sortStatus:
			if ra, rb := bucketStatusRank(a), bucketStatusRank(b); ra != rb {
				return ra > rb
			}

		case sortAgent:
			if sa, sb := bucketSeq(a), bucketSeq(b); sa != sb {
				return sa > sb
			}
		}

		// Alphabetical is the mode, and the tie-break for every other mode: two
		// features nothing distinguishes must not swap places between refreshes.
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
}

// sortMembers orders the worktrees inside one feature. The collector already
// hands them over by repository, branch and path, so ascending alpha is the
// order they arrive in and the stable base every other ordering works from.
func sortMembers(members []*collect.Worktree, order ordering) {
	if order == (ordering{}) {
		return
	}

	sort.SliceStable(members, func(i, j int) bool {
		a, b := members[i], members[j]

		if order.desc {
			a, b = b, a
		}

		switch order.mode {
		case sortStatus:
			if ra, rb := statusRank[a.AgentStatus], statusRank[b.AgentStatus]; ra != rb {
				return ra > rb
			}

		case sortAgent:
			if a.AgentSeq != b.AgentSeq {
				return a.AgentSeq > b.AgentSeq
			}
		}

		return repoKey(a) < repoKey(b)
	})
}

// repoKey is the collector's own ordering — repository, then branch, then path
// — as one comparable string, so a reversed or tied sort still has something
// stable underneath it.
func repoKey(w *collect.Worktree) string {
	return w.RepoName + "\x00" + w.Branch + "\x00" + w.Path
}

func bucketStatusRank(b *groupBucket) int {
	return statusRank[aggregateStatus(b.members)]
}

// bucketSeq is how recently anything happened anywhere in a feature.
func bucketSeq(b *groupBucket) int64 {
	var last int64

	for _, wt := range b.members {
		if wt.AgentSeq > last {
			last = wt.AgentSeq
		}
	}

	return last
}
