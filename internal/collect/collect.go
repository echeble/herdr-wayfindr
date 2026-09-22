// Package collect assembles the cross-workspace picture the sidebar draws:
// every worktree Herdr knows about, in every repository, with the workspace and
// agent state attached.
package collect

import (
	"context"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// gitTimeout bounds one `git status` call. A checkout on a stalled network
// mount must not hold up the whole refresh.
const gitTimeout = 3 * time.Second

// repoWorkers caps the parallel fan-out. worktree.list is one round trip per
// repository and there is no call that answers them all at once.
const repoWorkers = 8

// Worktree is one row of the sidebar: a git checkout, plus whatever Herdr and
// git know about it.
type Worktree struct {
	Path     string
	Branch   string
	RepoName string
	RepoRoot string
	IsLinked bool

	// WorkspaceID is empty when the checkout exists on disk but no workspace
	// has it open — including worktrees other tools created.
	WorkspaceID    string
	WorkspaceLabel string
	Focused        bool

	// AgentStatus is the strongest state among the agents running in this
	// worktree's workspace, or "unknown" when none is.
	AgentStatus string
	// Agents is how many of them are in each state. AgentStatus alone says what
	// the strongest one is doing and nothing about how much company it has, and
	// the sidebar spends a line saying "2 agents working" rather than "working".
	Agents AgentTally
	// AgentSeq is the highest state-change sequence among those agents, and 0
	// when none is running. Herdr timestamps nothing, so this counter is the
	// only handle on *when* something last happened: it rises every time an
	// agent changes state, which makes it an ordering and not a clock. It
	// restarts with the Herdr server.
	AgentSeq int64

	Git GitStatus

	// PR is GitHub's answer for this worktree's branch. It stays PRUnknown
	// when the lookup is off, the remote is not GitHub, or gh could not answer.
	PR PR
}

// Open reports whether Herdr currently has this checkout open as a workspace.
func (w Worktree) Open() bool { return w.WorkspaceID != "" }

// AgentTally is how many agents sit in each state: one workspace's, or a whole
// feature's once its worktrees' are merged.
//
// It exists because the state a row is labelled with is the strongest one among
// its agents — see agentGroup.summary — which says nothing about how many are
// in it. "blocked" with two others still working is a different picture from
// "blocked" alone, and the tally is what lets a row say which.
type AgentTally struct {
	Blocked int
	Working int
	Done    int
	Idle    int
	Unknown int
}

// Plus is this tally with one more agent in the given state. An unrecognised
// state counts as unknown rather than being dropped, so Total never undercounts
// the agents actually running.
func (t AgentTally) Plus(status string) AgentTally {
	switch status {
	case herdr.StatusBlocked:
		t.Blocked++
	case herdr.StatusWorking:
		t.Working++
	case herdr.StatusDone:
		t.Done++
	case herdr.StatusIdle:
		t.Idle++
	default:
		t.Unknown++
	}

	return t
}

// Merge sums two tallies, which is how a feature's is built from its members'.
func (t AgentTally) Merge(other AgentTally) AgentTally {
	t.Blocked += other.Blocked
	t.Working += other.Working
	t.Done += other.Done
	t.Idle += other.Idle
	t.Unknown += other.Unknown

	return t
}

// Count answers how many agents are in one state.
func (t AgentTally) Count(status string) int {
	switch status {
	case herdr.StatusBlocked:
		return t.Blocked
	case herdr.StatusWorking:
		return t.Working
	case herdr.StatusDone:
		return t.Done
	case herdr.StatusIdle:
		return t.Idle
	default:
		return t.Unknown
	}
}

// Total is every agent, whatever state it is in.
func (t AgentTally) Total() int {
	return t.Blocked + t.Working + t.Done + t.Idle + t.Unknown
}

// World is one complete observation.
type World struct {
	Worktrees          []Worktree
	FocusedWorkspaceID string
	// Errors are per-repository failures. A repository that cannot be listed
	// drops its rows; it must not blank the rest of the sidebar.
	Errors []string
}

// Collector fetches the world and caches the expensive parts between refreshes.
type Collector struct {
	client herdr.Client
	withPR bool

	mu       sync.Mutex
	gitCache map[string]GitStatus
	// prCache holds only merged results. A merged pull request is the one
	// answer that cannot change back, so it is worth remembering for the life
	// of the pane; every other state is asked again.
	prCache map[string]PR
	// slugCache maps a repository root to its GitHub slug, "" meaning "asked,
	// and it is not GitHub". Presence is what distinguishes that from "not yet
	// asked", so it is read with the two-value form.
	slugCache map[string]string
}

func New(client herdr.Client) *Collector {
	return &Collector{
		client:    client,
		gitCache:  map[string]GitStatus{},
		prCache:   map[string]PR{},
		slugCache: map[string]string{},
	}
}

// WithPR turns the pull-request lookup on, which is what CollectPR does at all.
// It is off by default because it costs a `gh` call per branch.
func (c *Collector) WithPR(on bool) *Collector {
	c.withPR = on

	return c
}

// Collect takes a fresh snapshot, fans out over every repository it names, and
// attaches git and agent state. full is false for the one-shot modes, which
// want the grouping and nothing that costs a subprocess per worktree: the
// cached git status is reused instead of re-read. Pull requests are a separate
// pass — see CollectPR.
func (c *Collector) Collect(ctx context.Context, full bool) (World, error) {
	snapshot, err := herdr.SessionSnapshot(ctx, c.client)
	if err != nil {
		return World{}, err
	}

	world := World{FocusedWorkspaceID: snapshot.FocusedWorkspaceID}

	worktrees, errs := c.listAllWorktrees(ctx, snapshot)
	world.Errors = errs

	byWorkspace := indexWorkspaces(snapshot)
	agents := indexAgents(snapshot)

	for i := range worktrees {
		wt := &worktrees[i]

		if ws, ok := byWorkspace[wt.WorkspaceID]; ok && wt.WorkspaceID != "" {
			wt.WorkspaceLabel = ws.Label
			wt.Focused = ws.WorkspaceID == snapshot.FocusedWorkspaceID

			status, tally := agents[wt.WorkspaceID].summary()
			wt.AgentStatus = status
			wt.Agents = tally
			wt.AgentSeq = agents[wt.WorkspaceID].lastSeq()
		} else {
			wt.AgentStatus = herdr.StatusUnknown
		}
	}

	c.attachGit(ctx, worktrees, full)

	sortWorktrees(worktrees)

	world.Worktrees = worktrees

	return world, nil
}

// listAllWorktrees walks every distinct repository named by the snapshot and
// unions their worktrees. Enumeration has to work this way: worktree.list is
// per-repository and refuses a cwd outside a git work tree.
func (c *Collector) listAllWorktrees(ctx context.Context, snapshot herdr.Snapshot) ([]Worktree, []string) {
	roots := distinctRepoRoots(snapshot)

	type result struct {
		root      string
		repoName  string
		worktrees []herdr.Worktree
		err       error
	}

	jobs := make(chan string)
	results := make(chan result)

	var wg sync.WaitGroup

	workers := min(repoWorkers, len(roots))
	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for root := range jobs {
				list, source, err := herdr.ListWorktrees(ctx, c.client, root)
				results <- result{root: root, repoName: source.RepoName, worktrees: list, err: err}
			}
		}()
	}

	go func() {
		for _, root := range roots {
			jobs <- root
		}

		close(jobs)
		wg.Wait()
		close(results)
	}()

	var (
		out  []Worktree
		errs []string
	)

	// A checkout can be reported by more than one repository only if the repos
	// share a git dir, but dedupe by path anyway — a duplicated row would be
	// counted twice in its group.
	seen := map[string]bool{}

	for res := range results {
		if res.err != nil {
			errs = append(errs, res.root+": "+res.err.Error())

			continue
		}

		for _, wt := range res.worktrees {
			if seen[wt.Path] {
				continue
			}

			seen[wt.Path] = true

			row := Worktree{
				Path:     wt.Path,
				Branch:   wt.BranchName(),
				RepoName: nonEmpty(res.repoName, wt.Label),
				RepoRoot: res.root,
				IsLinked: wt.IsLinkedWorktree,
			}

			if wt.OpenWorkspaceID != nil {
				row.WorkspaceID = *wt.OpenWorkspaceID
			}

			out = append(out, row)
		}
	}

	sort.Strings(errs)

	return out, errs
}

// attachGit fills in the working-tree summary, refreshing it only on the slow
// tick. Cached entries survive a tick where git is skipped so rows do not blink
// between "clean" and "unknown".
func (c *Collector) attachGit(ctx context.Context, worktrees []Worktree, refresh bool) {
	if !refresh {
		c.mu.Lock()
		for i := range worktrees {
			worktrees[i].Git = c.gitCache[worktrees[i].Path]
		}
		c.mu.Unlock()

		return
	}

	type entry struct {
		path   string
		status GitStatus
	}

	jobs := make(chan string)
	results := make(chan entry)

	var wg sync.WaitGroup

	workers := min(repoWorkers, len(worktrees))
	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for path := range jobs {
				callCtx, cancel := context.WithTimeout(ctx, gitTimeout)
				results <- entry{path: path, status: gitStatus(callCtx, path)}

				cancel()
			}
		}()
	}

	go func() {
		for i := range worktrees {
			jobs <- worktrees[i].Path
		}

		close(jobs)
		wg.Wait()
		close(results)
	}()

	fresh := make(map[string]GitStatus, len(worktrees))
	for res := range results {
		fresh[res.path] = res.status
	}

	c.mu.Lock()
	// Replacing the map rather than merging drops entries for worktrees that
	// are gone, so the cache cannot grow without bound.
	c.gitCache = fresh
	c.mu.Unlock()

	for i := range worktrees {
		worktrees[i].Git = fresh[worktrees[i].Path]
	}
}

// CollectPR fills in the pull-request state of a world that has already been
// collected, and answers a new one — the argument is left untouched, so the
// caller can keep drawing the picture it has while this runs.
//
// It is a second pass rather than part of Collect because it is by far the
// slowest thing the plugin does: a `gh` round trip per branch, some seconds in
// total. The list has to be on screen before that starts.
func (c *Collector) CollectPR(ctx context.Context, world World) World {
	if !c.withPR || len(world.Worktrees) == 0 {
		return world
	}

	worktrees := slices.Clone(world.Worktrees)

	errs := c.attachPR(ctx, worktrees)

	world.Worktrees = worktrees
	world.Errors = append(slices.Clone(world.Errors), errs...)

	return world
}

// attachPR asks GitHub about every branch and returns whatever went wrong as
// footer-sized notes. A worktree whose pull request cannot be determined keeps
// PRUnknown, which renders as nothing: this feature may be silent, never wrong.
func (c *Collector) attachPR(ctx context.Context, worktrees []Worktree) []string {
	if !c.withPR || len(worktrees) == 0 {
		return nil
	}

	if _, ok := toolPath("gh"); !ok {
		return []string{"gh is not installed; pull-request state is unavailable"}
	}

	type entry struct {
		index int
		pr    PR
		err   error
	}

	jobs := make(chan int)
	results := make(chan entry)

	var wg sync.WaitGroup

	workers := min(repoWorkers, len(worktrees))
	for range workers {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for i := range jobs {
				slug := c.slugFor(ctx, worktrees[i].RepoRoot)
				if slug == "" || worktrees[i].Branch == "" {
					continue
				}

				if pr, ok := c.cachedPR(slug, worktrees[i].Branch); ok {
					results <- entry{index: i, pr: pr}

					continue
				}

				pr, err := lookupPR(ctx, slug, worktrees[i].Branch)
				results <- entry{index: i, pr: pr, err: err}
			}
		}()
	}

	go func() {
		for i := range worktrees {
			jobs <- i
		}

		close(jobs)
		wg.Wait()
		close(results)
	}()

	var failure string

	for res := range results {
		if res.err != nil {
			// One note, not forty: a broken gh fails identically for every
			// branch, and the footer has room for one line.
			if failure == "" {
				failure = "gh: " + res.err.Error()
			}

			continue
		}

		worktrees[res.index].PR = res.pr

		if res.pr.Merged() {
			c.rememberPR(worktrees[res.index].RepoRoot, worktrees[res.index].Branch, res.pr)
		}
	}

	if failure != "" {
		return []string{failure}
	}

	return nil
}

// slugFor answers a repository's GitHub slug, asking git only once per root.
func (c *Collector) slugFor(ctx context.Context, root string) string {
	if root == "" {
		return ""
	}

	c.mu.Lock()
	slug, known := c.slugCache[root]
	c.mu.Unlock()

	if known {
		return slug
	}

	slug = repoSlug(ctx, root)

	c.mu.Lock()
	c.slugCache[root] = slug
	c.mu.Unlock()

	return slug
}

func (c *Collector) cachedPR(slug, branch string) (PR, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	pr, ok := c.prCache[prKey(slug, branch)]

	return pr, ok
}

func (c *Collector) rememberPR(root, branch string, pr PR) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.prCache[prKey(c.slugCache[root], branch)] = pr
}

func distinctRepoRoots(snapshot herdr.Snapshot) []string {
	seen := map[string]bool{}

	var roots []string

	for _, ws := range snapshot.Workspaces {
		if ws.Worktree == nil || ws.Worktree.RepoRoot == "" || seen[ws.Worktree.RepoRoot] {
			continue
		}

		seen[ws.Worktree.RepoRoot] = true

		roots = append(roots, ws.Worktree.RepoRoot)
	}

	sort.Strings(roots)

	return roots
}

func indexWorkspaces(snapshot herdr.Snapshot) map[string]herdr.Workspace {
	out := make(map[string]herdr.Workspace, len(snapshot.Workspaces))
	for _, ws := range snapshot.Workspaces {
		out[ws.WorkspaceID] = ws
	}

	return out
}

type agentGroup []herdr.Agent

// summary answers the state that should represent a whole workspace, and the
// tally behind it. Blocked wins because it is the one state that wants the user
// now; unknown loses to everything.
func (g agentGroup) summary() (string, AgentTally) {
	var tally AgentTally

	if len(g) == 0 {
		return herdr.StatusUnknown, tally
	}

	rank := map[string]int{
		herdr.StatusBlocked: 5,
		herdr.StatusWorking: 4,
		herdr.StatusDone:    3,
		herdr.StatusIdle:    2,
		herdr.StatusUnknown: 1,
	}

	best := herdr.StatusUnknown
	for _, agent := range g {
		tally = tally.Plus(agent.AgentStatus)

		if rank[agent.AgentStatus] > rank[best] {
			best = agent.AgentStatus
		}
	}

	return best, tally
}

// lastSeq answers how recently anything happened in this workspace: the highest
// state-change sequence among its agents. A workspace with no agent answers 0,
// which sorts it below every workspace that has one.
func (g agentGroup) lastSeq() int64 {
	var last int64

	for _, agent := range g {
		if agent.StateChangeSeq > last {
			last = agent.StateChangeSeq
		}
	}

	return last
}

func indexAgents(snapshot herdr.Snapshot) map[string]agentGroup {
	out := map[string]agentGroup{}
	for _, agent := range snapshot.Agents {
		out[agent.WorkspaceID] = append(out[agent.WorkspaceID], agent)
	}

	return out
}

// sortWorktrees gives the list a stable order within a group: repository first,
// then branch, then path.
func sortWorktrees(worktrees []Worktree) {
	sort.Slice(worktrees, func(i, j int) bool {
		a, b := worktrees[i], worktrees[j]

		if a.RepoName != b.RepoName {
			return a.RepoName < b.RepoName
		}

		if a.Branch != b.Branch {
			return a.Branch < b.Branch
		}

		return a.Path < b.Path
	})
}

func nonEmpty(first, second string) string {
	if first != "" {
		return first
	}

	return second
}
