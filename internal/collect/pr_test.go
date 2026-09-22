package collect

import "testing"

func TestParseRemote(t *testing.T) {
	cases := []struct {
		remote string
		want   string
	}{
		{"git@github.com:Credify/marketplace-billing-service.git", "Credify/marketplace-billing-service"},
		{"git@github.com:Credify/marketplace-billing-service", "Credify/marketplace-billing-service"},
		{"https://github.com/Credify/qa-automation.git", "Credify/qa-automation"},
		{"https://github.com/Credify/qa-automation/", "Credify/qa-automation"},
		{"ssh://git@github.com/uplift-inc/bnpl-decision-engine.git", "uplift-inc/bnpl-decision-engine"},
		{"ssh://git@github.com:22/uplift-inc/bnpl-decision-engine.git", "uplift-inc/bnpl-decision-engine"},
		{"https://user:token@github.com/Credify/k8s-template.git", "Credify/k8s-template"},
		// A trailing newline is what `git config --get` actually hands back.
		{"git@github.com:Credify/marketplace-billing-service.git\n", "Credify/marketplace-billing-service"},
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
	if prKey("Credify/a", "WAYF-20226") == prKey("Credify/b", "WAYF-20226") {
		t.Fatal("prKey must distinguish the same branch in different repositories")
	}
}
