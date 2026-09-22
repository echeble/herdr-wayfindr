package collect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

// prTimeout bounds one PR GraphQL query over the network.
const prTimeout = 10 * time.Second

// prCandidates is how many pull requests are asked for per branch. A branch can
// carry more than one over its life — a closed attempt and then the real one —
// and the most interesting state wins, so asking for exactly one would report
// whichever GitHub happened to list first.
const prCandidates = 5

type graphQLRequest struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables"`
}

const prGraphQLQuery = `query($owner: String!, $repo: String!, $head: String!) {
  repository(owner: $owner, name: $repo) {
    pullRequests(headRefName: $head, first: 5, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number
        state
        url
      }
    }
  }
}`

type graphQLResponse struct {
	Data struct {
		Repository *struct {
			PullRequests struct {
				Nodes []struct {
					Number int    `json:"number"`
					State  string `json:"state"`
					URL    string `json:"url"`
				} `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

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
// repos — WAYF-20226 exists in three — so the repository has to be part of it.
func prKey(slug, branch string) string { return slug + "\x00" + branch }

// repoSlug turns a repository's origin remote into the OWNER/REPO form for
// github.com or HOST/OWNER/REPO for a GitHub Enterprise host. A remote that is
// not GitHub yields "", and that repository is then never asked about — there is
// no point spending failed network calls on a GitLab or local checkout.
func repoSlug(ctx context.Context, root string) string {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "config", "--get", "remote.origin.url")
	cmd.Env = hardenedEnv()

	out, err := cmd.Output()
	if err != nil {
		return ""
	}

	return parseRemote(string(out))
}

// splitSlug decomposes a slug into host, owner, and repository name.
// For github.com, slug is "owner/repo" and host defaults to "github.com".
// For enterprise, slug is "host/owner/repo".
func splitSlug(slug string) (host, owner, repo string, err error) {
	parts := strings.Split(slug, "/")
	switch len(parts) {
	case 2:
		if parts[0] == "" || parts[1] == "" {
			return "", "", "", fmt.Errorf("invalid repository slug %q", slug)
		}
		return "github.com", parts[0], parts[1], nil
	case 3:
		if parts[0] == "" || parts[1] == "" || parts[2] == "" {
			return "", "", "", fmt.Errorf("invalid repository slug %q", slug)
		}
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("invalid repository slug %q", slug)
	}
}

// graphQLEndpoint returns the GraphQL API URL for the given host.
func graphQLEndpoint(host string) string {
	if host == "github.com" {
		return "https://api.github.com/graphql"
	}
	return "https://" + host + "/api/graphql"
}

// gitCredential queries `git credential fill` for a given host (e.g. "github.com").
// It disables interactive terminal prompts so it never blocks waiting for input.
func gitCredential(ctx context.Context, host string) (string, error) {
	gitBin := "git"
	if p, ok := toolPath("git"); ok {
		gitBin = p
	}

	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(callCtx, gitBin, "credential", "fill")
	cmd.Env = append(hardenedEnv(), "GIT_TERMINAL_PROMPT=0")
	cmd.Stdin = strings.NewReader(fmt.Sprintf("protocol=https\nhost=%s\n\n", host))

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git credential fill %s: %w", host, err)
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "password=") {
			pass := strings.TrimPrefix(line, "password=")
			if pass != "" {
				return pass, nil
			}
		}
	}

	return "", fmt.Errorf("no password found for host %s", host)
}

// resolveToken finds an authentication token for the given host.
// It checks standard environment variables (GITHUB_TOKEN, GH_TOKEN),
// and falls back to querying `git credential fill`.
func resolveToken(ctx context.Context, host string) (string, error) {
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		return tok, nil
	}
	if tok := os.Getenv("GH_TOKEN"); tok != "" {
		return tok, nil
	}
	return gitCredential(ctx, host)
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
			rest = rest[at+1:]
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

// lookupPR asks GitHub about one branch via the GraphQL API.
// It reports PRNone — not an error — when the branch simply has no pull request,
// which is the ordinary state of work in progress.
func lookupPR(ctx context.Context, client *http.Client, token, host, owner, repo, branch string) (PR, error) {
	if client == nil {
		client = http.DefaultClient
	}

	endpoint := graphQLEndpoint(host)

	reqBody := graphQLRequest{
		Query: prGraphQLQuery,
		Variables: map[string]any{
			"owner": owner,
			"repo":  repo,
			"head":  branch,
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return PR{}, err
	}

	callCtx, cancel := context.WithTimeout(ctx, prTimeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return PR{}, err
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("User-Agent", "wayfindr")
	if token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return PR{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Message string `json:"message"`
		}
		respBody, _ := io.ReadAll(resp.Body)
		if json.Unmarshal(respBody, &errResp) == nil && errResp.Message != "" {
			return PR{}, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, errResp.Message)
		}
		return PR{}, fmt.Errorf("GitHub API %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var data graphQLResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return PR{}, err
	}

	if data.Data.Repository == nil {
		if len(data.Errors) > 0 {
			return PR{}, errors.New(data.Errors[0].Message)
		}
		return PR{State: PRNone}, nil
	}

	best := PR{State: PRNone}

	for _, item := range data.Data.Repository.PullRequests.Nodes {
		state := parsePRState(item.State)
		if state <= best.State {
			continue
		}

		best = PR{State: state, Number: item.Number, URL: item.URL}
	}

	return best, nil
}

// LookupPR asks GitHub about one branch for a given slug.
func LookupPR(ctx context.Context, client *http.Client, token, slug, branch string) (PR, error) {
	host, owner, repo, err := splitSlug(slug)
	if err != nil {
		return PR{}, err
	}
	return lookupPR(ctx, client, token, host, owner, repo, branch)
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
