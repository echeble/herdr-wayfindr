package collect

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// prTimeout bounds one `gh pr list`. It is longer than the git timeout because
// this one is a network round trip to GitHub.
const prTimeout = 10 * time.Second

// prCandidates is how many pull requests are asked for per branch. A branch can
// carry more than one over its life — a closed attempt and then the real one —
// and the most interesting state wins, so asking for exactly one would report
// whichever GitHub happened to list first.
const prCandidates = 5

// PRState is what GitHub knows about the branch a worktree sits on.
//
// The order is also the precedence used to pick between several pull requests
// on one branch: a merged one is the answer even if a later attempt was closed.
type PRState int

const (
	// PRUnknown means the branch was never asked about, or the answer failed.
	// It is deliberately distinct from PRNone: "we do not know" must never
	// render as "there is no pull request".
	PRUnknown PRState = iota
	PRNone
	PRClosed
	PROpen
	PRMerged
)

func (s PRState) String() string {
	switch s {
	case PRNone:
		return "none"
	case PRClosed:
		return "closed"
	case PROpen:
		// "open" is GitHub's word for the pull request; "in progress" is what
		// the sidebar is actually telling you about the feature — that work on
		// it is still going — and it does not collide with the other "open" on
		// a card, which counts checkouts Herdr has open as workspaces.
		return "in progress"
	case PRMerged:
		return "merged"
	default:
		return "unknown"
	}
}

// PR is the pull-request summary carried by a worktree row.
type PR struct {
	State  PRState
	Number int
	URL    string
}

// Merged reports whether this worktree's work has landed. A group is done when
// every member answers true.
func (p PR) Merged() bool { return p.State == PRMerged }

// AggregatePR answers the state of a whole feature from its worktrees'.
//
// Merged needs every one of them: the marker says the feature has landed, so a
// single member that has not — or that could not be asked about — withholds it.
// Otherwise the most active state wins, because what you want from a feature
// that is not finished is whether anything is still in review.
func AggregatePR(states []PRState) PRState {
	if len(states) == 0 {
		return PRUnknown
	}

	merged, open, closed := true, false, false

	for _, state := range states {
		switch state {
		case PRMerged:
			continue
		case PROpen:
			open = true
		case PRClosed:
			closed = true
		}

		merged = false
	}

	switch {
	case merged:
		return PRMerged
	case open:
		return PROpen
	case closed:
		return PRClosed
	default:
		return PRUnknown
	}
}

// prKey identifies a branch across repositories. Branch names repeat between
// repos — CRD-20226 exists in three — so the repository has to be part of it.
func prKey(slug, branch string) string { return slug + "\x00" + branch }

// repoSlug turns a repository's origin remote into the OWNER/REPO form that
// `gh --repo` takes, or HOST/OWNER/REPO for a GitHub Enterprise host. A remote
// that is not GitHub yields "", and that repository is then never asked about —
// there is no point spending a failed `gh` call per worktree on a GitLab
// checkout.
func repoSlug(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "config", "--get", "remote.origin.url")
	cmd.Env = hardenedEnv()

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return parseRemote(string(out))
}

// parseRemote reads the two shapes git remotes come in: a URL with a scheme,
// and the scp-like host:path form.
func parseRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimSuffix(raw, "/")
	raw = strings.TrimSuffix(raw, ".git")

	var host, path string

	switch {
	case strings.Contains(raw, "://"):
		rest := raw[strings.Index(raw, "://")+3:]

		// Strip any user[:password]@ in front of the host.
		if at := strings.LastIndex(rest, "@"); at >= 0 {
			rest = rest[at+1:]
		}

		slash := strings.Index(rest, "/")
		if slash < 0 {
			return ""
		}

		host, path = rest[:slash], rest[slash+1:]

	case strings.Contains(raw, ":"):
		rest := raw
		if at := strings.LastIndex(raw, "@"); at >= 0 {
			rest = raw[at+1:]
		}

		colon := strings.Index(rest, ":")
		host, path = rest[:colon], rest[colon+1:]

	default:
		return ""
	}

	// ssh://git@host:22/owner/repo carries a port the slug must not keep.
	if colon := strings.Index(host, ":"); colon >= 0 {
		host = host[:colon]
	}

	path = strings.Trim(path, "/")

	if host == "" || path == "" || strings.Count(path, "/") != 1 {
		return ""
	}

	if !strings.Contains(host, "github") {
		return ""
	}

	if host == "github.com" {
		return path
	}

	return host + "/" + path
}

// lookupPR asks GitHub about one branch. It reports PRNone — not an error —
// when the branch simply has no pull request, which is the ordinary state of
// work in progress.
func lookupPR(ctx context.Context, slug, branch string) (PR, error) {
	gh, ok := toolPath("gh")
	if !ok {
		return PR{}, errors.New("gh is not installed")
	}

	callCtx, cancel := context.WithTimeout(ctx, prTimeout)
	defer cancel()

	cmd := exec.CommandContext(callCtx, gh, "pr", "list",
		"--repo", slug,
		"--head", branch,
		"--state", "all",
		"--limit", strconv.Itoa(prCandidates),
		"--json", "number,state,url",
	)
	cmd.Env = hardenedEnv()

	out, err := cmd.Output()
	if err != nil {
		return PR{}, ghError(err)
	}

	var list []struct {
		Number int    `json:"number"`
		State  string `json:"state"`
		URL    string `json:"url"`
	}

	if err := json.Unmarshal(out, &list); err != nil {
		return PR{}, err
	}

	best := PR{State: PRNone}

	for _, item := range list {
		state := parsePRState(item.State)
		if state <= best.State {
			continue
		}

		best = PR{State: state, Number: item.Number, URL: item.URL}
	}

	return best, nil
}

func parsePRState(s string) PRState {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "MERGED":
		return PRMerged
	case "OPEN":
		return PROpen
	case "CLOSED":
		return PRClosed
	default:
		return PRUnknown
	}
}

// ghError turns an exec failure into the one line gh actually printed, so a
// missing login reads as "gh auth login" rather than "exit status 4".
func ghError(err error) error {
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		for _, line := range strings.Split(string(exit.Stderr), "\n") {
			if line = strings.TrimSpace(line); line != "" {
				return errors.New(line)
			}
		}
	}

	return err
}
