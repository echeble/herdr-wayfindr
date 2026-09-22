// Package group decides which feature a worktree belongs to. It is pure: no
// I/O, no Herdr calls, no git — everything it needs arrives as Input — which is
// what makes the precedence rules cheap to test.
package group

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/echeble/herdr-wayfindr/internal/config"
)

// Source records which rule produced a group name. The pane dims anything but
// Explicit, so it is always clear what was guessed and what was stated.
type Source int

const (
	// SourceNone means no rule matched; the worktree falls in the ungrouped
	// bucket.
	SourceNone Source = iota
	// SourceExplicit is a tag the user set. It always wins.
	SourceExplicit
	// SourceJira is a ticket key extracted from the branch name, or from the
	// Herdr workspace's name when the branch carries none.
	SourceJira
	// SourceBranchPattern is a user-supplied regexp over the branch name.
	SourceBranchPattern
	// SourceWorktreeName is a checkout directory name shared by at least two
	// different repositories.
	SourceWorktreeName
)

func (s Source) String() string {
	switch s {
	case SourceExplicit:
		return "tag"
	case SourceJira:
		return "ticket"
	case SourceBranchPattern:
		return "branch"
	case SourceWorktreeName:
		return "name"
	default:
		return ""
	}
}

// Input is one worktree as the resolver needs to see it.
type Input struct {
	// Path is the checkout path. It is the worktree's only stable identity —
	// Herdr gives worktrees no id — so it keys both the explicit tags and the
	// result.
	Path string
	// Branch is empty for a detached checkout.
	Branch string
	// Workspace is the Herdr workspace's name, empty for a checkout no
	// workspace has open. It is searched for a ticket key after the branch:
	// the workspace a worktree was made for is often named for the ticket even
	// when the checkout sits on master or is detached.
	Workspace string
	// RepoRoot distinguishes repositories for the shared-name rule.
	RepoRoot string
}

// Assignment is the group a worktree landed in and the rule that put it there.
type Assignment struct {
	Name   string
	Source Source
}

// Resolver applies the grouping rules in precedence order.
type Resolver struct {
	jira     *regexp.Regexp
	branch   []*regexp.Regexp
	useName  bool
	explicit map[string]string
}

// NewResolver compiles the configured patterns. config.Load has already
// rejected anything that does not compile or lacks the named capture, so a
// pattern that still fails here is simply skipped.
func NewResolver(cfg config.Grouping, explicit map[string]string) *Resolver {
	r := &Resolver{useName: cfg.UseWorktreeName, explicit: explicit}

	if cfg.JiraPattern != "" {
		if compiled, err := regexp.Compile(cfg.JiraPattern); err == nil {
			r.jira = compiled
		}
	}

	for _, pattern := range cfg.BranchPatterns {
		if compiled, err := regexp.Compile(pattern); err == nil {
			r.branch = append(r.branch, compiled)
		}
	}

	return r
}

// Resolve assigns every input to a group, keyed by Path. It takes the whole set
// at once because the shared-name rule is a property of the set, not of one
// worktree: a directory name only forms a group when two different
// repositories use it.
func (r *Resolver) Resolve(inputs []Input) map[string]Assignment {
	out := make(map[string]Assignment, len(inputs))

	var unresolved []Input

	for _, in := range inputs {
		if name, ok := r.perWorktree(in); ok {
			out[in.Path] = name

			continue
		}

		unresolved = append(unresolved, in)
	}

	for path, assignment := range r.bySharedName(unresolved) {
		out[path] = assignment
	}

	for _, in := range unresolved {
		if _, done := out[in.Path]; !done {
			out[in.Path] = Assignment{Source: SourceNone}
		}
	}

	return out
}

// perWorktree applies the rules that need only this worktree: an explicit tag,
// then the ticket key, then the user's branch patterns.
func (r *Resolver) perWorktree(in Input) (Assignment, bool) {
	if name := strings.TrimSpace(r.explicit[in.Path]); name != "" {
		return Assignment{Name: name, Source: SourceExplicit}, true
	}

	// The branch is asked first and the workspace name second: the branch is
	// the worktree's own answer, while the workspace is the window it happens
	// to be open in, which a rename can change under it.
	if r.jira != nil {
		for _, text := range []string{in.Branch, in.Workspace} {
			if text == "" {
				continue
			}

			if name := firstCapture(r.jira, text); name != "" {
				return Assignment{Name: strings.ToUpper(name), Source: SourceJira}, true
			}
		}
	}

	if in.Branch == "" {
		return Assignment{}, false
	}

	for _, pattern := range r.branch {
		if name := namedCapture(pattern, in.Branch, "group"); name != "" {
			return Assignment{Name: name, Source: SourceBranchPattern}, true
		}
	}

	return Assignment{}, false
}

// bySharedName groups leftovers whose checkout directory name is shared by at
// least two distinct repositories. Without the two-repo test every unmatched
// worktree would become a group of one, which is noise rather than grouping.
func (r *Resolver) bySharedName(inputs []Input) map[string]Assignment {
	out := map[string]Assignment{}

	if !r.useName {
		return out
	}

	type bucket struct {
		display string
		repos   map[string]bool
		paths   []string
	}

	buckets := map[string]*bucket{}

	for _, in := range inputs {
		name := filepath.Base(in.Path)
		if name == "" || name == "." || name == string(filepath.Separator) {
			continue
		}

		key := strings.ToLower(name)

		b := buckets[key]
		if b == nil {
			// The first spelling seen wins the display name, so the group
			// header keeps a real directory's casing.
			b = &bucket{display: name, repos: map[string]bool{}}
			buckets[key] = b
		}

		b.repos[in.RepoRoot] = true
		b.paths = append(b.paths, in.Path)
	}

	for _, b := range buckets {
		if len(b.repos) < 2 {
			continue
		}

		for _, path := range b.paths {
			out[path] = Assignment{Name: b.display, Source: SourceWorktreeName}
		}
	}

	return out
}

// WithLabels answers a copy of assignments with each Name replaced by its
// label override, where one exists — the same renamed header the pane itself
// would show. Grouping has already happened by the time this runs: nothing
// here changes which worktree landed in which assignment, only what a
// consumer that only ever sees the flat Name — a sidebar token, a plain-text
// report — prints for it.
func WithLabels(assignments map[string]Assignment, labels map[string]string) map[string]Assignment {
	if len(labels) == 0 {
		return assignments
	}

	out := make(map[string]Assignment, len(assignments))

	for path, a := range assignments {
		if label := strings.TrimSpace(labels[a.Name]); label != "" {
			a.Name = label
		}

		out[path] = a
	}

	return out
}

// firstCapture answers the first capture group, or the whole match when the
// pattern has none — so a bare pattern like [A-Z]+-[0-9]+ works without the
// user having to remember parentheses.
func firstCapture(re *regexp.Regexp, s string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}

	if len(m) > 1 && m[1] != "" {
		return strings.TrimSpace(m[1])
	}

	return strings.TrimSpace(m[0])
}

func namedCapture(re *regexp.Regexp, s, name string) string {
	m := re.FindStringSubmatch(s)
	if m == nil {
		return ""
	}

	for i, capture := range re.SubexpNames() {
		if capture == name && i < len(m) {
			return strings.TrimSpace(m[i])
		}
	}

	return ""
}
