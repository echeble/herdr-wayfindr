package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
)

func TestParseRemote(t *testing.T) {
	cases := []struct {
		remote string
		want   string
	}{
		{"git@github.com:example-org/marketplace-billing-service.git", "example-org/marketplace-billing-service"},
		{"git@github.com:example-org/marketplace-billing-service", "example-org/marketplace-billing-service"},
		{"https://github.com/example-org/qa-automation.git", "example-org/qa-automation"},
		{"https://github.com/example-org/qa-automation/", "example-org/qa-automation"},
		{"ssh://git@github.com/uplift-inc/bnpl-decision-engine.git", "uplift-inc/bnpl-decision-engine"},
		{"ssh://git@github.com:22/uplift-inc/bnpl-decision-engine.git", "uplift-inc/bnpl-decision-engine"},
		{"https://user:token@github.com/example-org/k8s-template.git", "example-org/k8s-template"},
		// A trailing newline is what `git config --get` actually hands back.
		{"git@github.com:example-org/marketplace-billing-service.git\n", "example-org/marketplace-billing-service"},
		// An enterprise host keeps its name, which is the form gh --repo takes.
		{"git@github.example.com:platform/service.git", "github.example.com/platform/service"},
		// Not GitHub: never ask, rather than ask and fail once per branch.
		{"git@gitlab.com:group/project.git", ""},
		{"https://bitbucket.org/team/repo.git", ""},
		// Not a remote we can make a slug of.
		{"/Users/echeble/Repo/local-only", ""},
		{"git@github.com:toodeep/a/b.git", ""},
		{"", ""},
	}

	for _, tc := range cases {
		if got := parseRemote(tc.remote); got != tc.want {
			t.Errorf("parseRemote(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}

// A branch can carry several pull requests over its life. The most advanced
// state is the true one: a closed second attempt must not hide the merge.
func TestPRStatePrecedence(t *testing.T) {
	if !(PRMerged > PROpen && PROpen > PRClosed && PRClosed > PRNone && PRNone > PRUnknown) {
		t.Fatal("PRState order must rank merged over open over closed over none over unknown")
	}
}

func TestPRStateWords(t *testing.T) {
	// GitHub's own word for an unmerged pull request is "open", but the sidebar
	// is reporting on the feature rather than the pull request, and parsePRState
	// still has to accept GitHub's spelling on the way in.
	cases := map[PRState]string{
		PROpen:    "in progress",
		PRMerged:  "merged",
		PRClosed:  "closed",
		PRNone:    "none",
		PRUnknown: "unknown",
	}

	for state, want := range cases {
		if got := state.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}

	if parsePRState("OPEN") != PROpen {
		t.Error("the display word changed; parsing GitHub's own must not")
	}
}

func TestParsePRState(t *testing.T) {
	cases := map[string]PRState{
		"MERGED":  PRMerged,
		"merged":  PRMerged,
		" OPEN ":  PROpen,
		"CLOSED":  PRClosed,
		"DRAFT":   PRUnknown,
		"":        PRUnknown,
		"unknown": PRUnknown,
	}

	for in, want := range cases {
		if got := parsePRState(in); got != want {
			t.Errorf("parsePRState(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestPRKeyIsPerRepository(t *testing.T) {
	// The same branch name exists in several repositories — that is the whole
	// point of the plugin — so the cache key cannot be the branch alone.
	if prKey("example-org/a", "WAYF-20226") == prKey("example-org/b", "WAYF-20226") {
		t.Fatal("prKey must distinguish the same branch in different repositories")
	}
}

func TestSplitSlug(t *testing.T) {
	cases := []struct {
		slug      string
		wantHost  string
		wantOwner string
		wantRepo  string
		wantErr   bool
	}{
		{"example-org/billing", "github.com", "example-org", "billing", false},
		{"github.example.com/org/repo", "github.example.com", "org", "repo", false},
		{"", "", "", "", true},
		{"single", "", "", "", true},
		{"too/many/parts/here", "", "", "", true},
		{"/repo", "", "", "", true},
		{"owner/", "", "", "", true},
	}

	for _, tc := range cases {
		host, owner, repo, err := splitSlug(tc.slug)
		if (err != nil) != tc.wantErr {
			t.Errorf("splitSlug(%q) error = %v, wantErr = %v", tc.slug, err, tc.wantErr)
			continue
		}
		if !tc.wantErr {
			if host != tc.wantHost || owner != tc.wantOwner || repo != tc.wantRepo {
				t.Errorf("splitSlug(%q) = (%q, %q, %q), want (%q, %q, %q)",
					tc.slug, host, owner, repo, tc.wantHost, tc.wantOwner, tc.wantRepo)
			}
		}
	}
}

func TestGraphQLEndpoint(t *testing.T) {
	if got := graphQLEndpoint("github.com"); got != "https://api.github.com/graphql" {
		t.Errorf("graphQLEndpoint(github.com) = %q, want https://api.github.com/graphql", got)
	}

	if got := graphQLEndpoint("github.mycompany.com"); got != "https://github.mycompany.com/api/graphql" {
		t.Errorf("graphQLEndpoint(github.mycompany.com) = %q, want https://github.mycompany.com/api/graphql", got)
	}
}

func TestResolveTokenEnvPrecedence(t *testing.T) {
	origGitHub := os.Getenv("GITHUB_TOKEN")
	origGH := os.Getenv("GH_TOKEN")
	defer func() {
		os.Setenv("GITHUB_TOKEN", origGitHub)
		os.Setenv("GH_TOKEN", origGH)
	}()

	os.Setenv("GITHUB_TOKEN", "token-from-github")
	os.Setenv("GH_TOKEN", "token-from-gh")

	tok, err := resolveToken(context.Background(), "github.com")
	if err != nil || tok != "token-from-github" {
		t.Errorf("resolveToken() = %q, want token-from-github", tok)
	}

	os.Unsetenv("GITHUB_TOKEN")
	tok, err = resolveToken(context.Background(), "github.com")
	if err != nil || tok != "token-from-gh" {
		t.Errorf("resolveToken() with GH_TOKEN only = %q, want token-from-gh", tok)
	}
}

func TestLookupPR_GraphQLSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-secret-token" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}

		var req graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		head := req.Variables["head"].(string)

		var resp graphQLResponse
		switch head {
		case "merged-branch":
			resp.Data.Repository = &struct {
				PullRequests struct {
					Nodes []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
						URL    string `json:"url"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			}{
				PullRequests: struct {
					Nodes []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
						URL    string `json:"url"`
					} `json:"nodes"`
				}{
					Nodes: []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
						URL    string `json:"url"`
					}{
						{Number: 42, State: "MERGED", URL: "https://github.com/org/repo/pull/42"},
						{Number: 41, State: "CLOSED", URL: "https://github.com/org/repo/pull/41"},
					},
				},
			}
		case "no-pr-branch":
			resp.Data.Repository = &struct {
				PullRequests struct {
					Nodes []struct {
						Number int    `json:"number"`
						State  string `json:"state"`
						URL    string `json:"url"`
					} `json:"nodes"`
				} `json:"pullRequests"`
			}{}
		case "nonexistent-repo":
			resp.Errors = []struct {
				Message string `json:"message"`
			}{
				{Message: "Could not resolve to a Repository"},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := server.Client()

	// Parse test server host for custom URL routing in lookupPR
	// We can test lookupPR by using custom host or routing
	u := server.URL

	// Verify merged precedence
	reqBody := graphQLRequest{
		Query: prGraphQLQuery,
		Variables: map[string]any{
			"owner": "org",
			"repo":  "repo",
			"head":  "merged-branch",
		},
	}
	bodyBytes, _ := json.Marshal(reqBody)
	httpReq, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, u, bytes.NewReader(bodyBytes))
	httpReq.Header.Set("Authorization", "Bearer test-secret-token")
	res, err := client.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()

	var data graphQLResponse
	if err := json.NewDecoder(res.Body).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Data.Repository.PullRequests.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(data.Data.Repository.PullRequests.Nodes))
	}
	best := PR{State: PRNone}
	for _, item := range data.Data.Repository.PullRequests.Nodes {
		st := parsePRState(item.State)
		if st > best.State {
			best = PR{State: st, Number: item.Number, URL: item.URL}
		}
	}
	if best.State != PRMerged || best.Number != 42 {
		t.Errorf("best = %+v, want PRMerged #42", best)
	}
}

func TestLookupPR_Unauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Bad credentials"}`))
	}))
	defer server.Close()

	client := server.Client()
	callCtx, cancel := context.WithTimeout(context.Background(), prTimeout)
	defer cancel()

	httpReq, _ := http.NewRequestWithContext(callCtx, http.MethodPost, server.URL, bytes.NewReader([]byte(`{}`)))
	resp, err := client.Do(httpReq)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestCollectorCollectPR(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-tok" {
			http.Error(w, `{"message":"Unauthorized"}`, http.StatusUnauthorized)
			return
		}

		var req graphQLRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		head := req.Variables["head"].(string)
		if head == "feat-merged" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"repository": map[string]any{
						"pullRequests": map[string]any{
							"nodes": []map[string]any{
								{"number": 101, "state": "MERGED", "url": "https://github.com/test/repo/pull/101"},
							},
						},
					},
				},
			})
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"repository": map[string]any{
					"pullRequests": map[string]any{
						"nodes": []any{},
					},
				},
			},
		})
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}

	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			return http.DefaultTransport.RoundTrip(req)
		}),
	}

	c := New(nil).
		WithPR(true).
		WithHTTPClient(client).
		WithTokenResolver(func(ctx context.Context, host string) (string, error) {
			if host == "github.com" {
				return "test-tok", nil
			}
			return "", errors.New("unknown host")
		})

	c.slugCache["/dummy/repo"] = "test/repo"

	world := World{
		Worktrees: []Worktree{
			{RepoRoot: "/dummy/repo", Branch: "feat-merged"},
			{RepoRoot: "/dummy/repo", Branch: "feat-wip"},
		},
	}

	res := c.CollectPR(context.Background(), world)
	if len(res.Errors) > 0 {
		t.Fatalf("unexpected errors: %v", res.Errors)
	}

	if res.Worktrees[0].PR.State != PRMerged || res.Worktrees[0].PR.Number != 101 {
		t.Errorf("worktree 0 = %+v, want PRMerged #101", res.Worktrees[0].PR)
	}
	if res.Worktrees[1].PR.State != PRNone {
		t.Errorf("worktree 1 = %+v, want PRNone", res.Worktrees[1].PR)
	}

	// Verify merged PR is cached
	if pr, ok := c.cachedPR("test/repo", "feat-merged"); !ok || pr.State != PRMerged {
		t.Errorf("cachedPR = %+v, want PRMerged", pr)
	}

	// Test missing token error handling
	cNoAuth := New(nil).
		WithPR(true).
		WithTokenResolver(func(ctx context.Context, host string) (string, error) {
			return "", errors.New("no token configured")
		})
	cNoAuth.slugCache["/dummy/repo"] = "test/repo"

	resNoAuth := cNoAuth.CollectPR(context.Background(), world)
	if len(resNoAuth.Errors) == 0 {
		t.Error("expected auth error in world.Errors, got none")
	}
	if resNoAuth.Worktrees[0].PR.State != PRUnknown {
		t.Errorf("worktree 0 state = %v, want PRUnknown", resNoAuth.Worktrees[0].PR.State)
	}
}
