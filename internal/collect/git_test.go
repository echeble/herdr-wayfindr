package collect

import (
	"context"
	"os/exec"
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

func herdrAgent(status string) herdr.Agent {
	return herdr.Agent{AgentStatus: status}
}

func TestParsePorcelainV2(t *testing.T) {
	out := `# branch.oid abc123
# branch.head WAYF-20748
# branch.upstream origin/WAYF-20748
# branch.ab +3 -1
1 M. N... 100644 100644 100644 aaa bbb src/staged.go
1 .M N... 100644 100644 100644 ccc ddd src/unstaged.go
1 MM N... 100644 100644 100644 eee fff src/both.go
2 R. N... 100644 100644 100644 ggg hhh R100 new.go	old.go
u UU N... 100644 100644 100644 100644 iii jjj kkk src/conflict.go
? untracked.txt
? another.txt
`

	got := parsePorcelainV2(out)

	if !got.Known {
		t.Fatal("status should be known")
	}

	if got.Branch != "WAYF-20748" {
		t.Errorf("branch = %q, want WAYF-20748", got.Branch)
	}

	if got.Ahead != 3 || got.Behind != 1 {
		t.Errorf("ahead/behind = %d/%d, want 3/1", got.Ahead, got.Behind)
	}

	// M., MM and the rename all have a staged half.
	if got.Staged != 3 {
		t.Errorf("staged = %d, want 3", got.Staged)
	}

	// .M and MM have an unstaged half.
	if got.Unstaged != 2 {
		t.Errorf("unstaged = %d, want 2", got.Unstaged)
	}

	if got.Conflicts != 1 {
		t.Errorf("conflicts = %d, want 1", got.Conflicts)
	}

	if got.Untracked != 2 {
		t.Errorf("untracked = %d, want 2", got.Untracked)
	}

	if !got.Dirty() {
		t.Error("a tree with tracked changes should be dirty")
	}
}

func TestParsePorcelainV2CleanTree(t *testing.T) {
	got := parsePorcelainV2("# branch.oid abc\n# branch.head master\n")

	if !got.Known {
		t.Fatal("status should be known")
	}

	if got.Dirty() {
		t.Error("a tree with no entries should not be dirty")
	}

	if got.Branch != "master" {
		t.Errorf("branch = %q, want master", got.Branch)
	}
}

func TestParsePorcelainV2UntrackedOnlyIsNotDirty(t *testing.T) {
	// Build output and editor droppings would otherwise mark most of these
	// worktrees permanently modified.
	got := parsePorcelainV2("# branch.head master\n? target/foo.class\n")

	if got.Dirty() {
		t.Error("untracked files alone should not count as dirty")
	}

	if got.Untracked != 1 {
		t.Errorf("untracked = %d, want 1", got.Untracked)
	}
}

func TestParsePorcelainV2DetachedHead(t *testing.T) {
	got := parsePorcelainV2("# branch.head (detached)\n")

	if got.Branch != "" {
		t.Errorf("branch = %q, want empty for a detached head", got.Branch)
	}
}

func TestParsePorcelainV2NoUpstream(t *testing.T) {
	// A branch with no upstream has no branch.ab line at all.
	got := parsePorcelainV2("# branch.head WAYF-1\n1 M. N... 1 1 1 a b f.go\n")

	if got.Ahead != 0 || got.Behind != 0 {
		t.Errorf("ahead/behind = %d/%d, want 0/0", got.Ahead, got.Behind)
	}

	if got.Staged != 1 {
		t.Errorf("staged = %d, want 1", got.Staged)
	}
}

func TestGitStatusUnknownWhenGitFails(t *testing.T) {
	// The zero value must read as "no information", never as "clean".
	var zero GitStatus

	if zero.Known {
		t.Error("the zero GitStatus should not be Known")
	}

	if zero.Dirty() {
		t.Error("the zero GitStatus should not be Dirty")
	}
}

func TestAgentGroupSummaryPrefersBlocked(t *testing.T) {
	cases := []struct {
		name     string
		statuses []string
		want     string
	}{
		{"blocked beats working", []string{"idle", "working", "blocked"}, "blocked"},
		{"working beats done", []string{"done", "working"}, "working"},
		{"done beats idle", []string{"idle", "done"}, "done"},
		{"idle beats unknown", []string{"unknown", "idle"}, "idle"},
		{"no agents", nil, "unknown"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var g agentGroup
			for _, status := range tc.statuses {
				g = append(g, herdrAgent(status))
			}

			got, tally := g.summary()
			if got != tc.want {
				t.Errorf("summary = %q, want %q", got, tc.want)
			}

			if tally.Total() != len(tc.statuses) {
				t.Errorf("total = %d, want %d", tally.Total(), len(tc.statuses))
			}

			// Every agent is counted under the state it is actually in, not
			// under the one the summary picked.
			for _, status := range tc.statuses {
				want := 0
				for _, other := range tc.statuses {
					if other == status {
						want++
					}
				}

				if got := tally.Count(status); got != want {
					t.Errorf("count(%q) = %d, want %d", status, got, want)
				}
			}
		})
	}
}

func TestRepoDefaultBranch(t *testing.T) {
	if got := repoDefaultBranch(context.Background(), ""); got != "" {
		t.Errorf("empty repo root = %q, want empty", got)
	}

	dir := t.TempDir()
	cmdInit := exec.Command("git", "init", "-b", "trunk", dir)
	if err := cmdInit.Run(); err != nil {
		t.Skipf("git init with -b not supported: %v", err)
	}

	got := repoDefaultBranch(context.Background(), dir)
	// On init with trunk without remote, init.defaultBranch might not be in config
	// but let's test configuring init.defaultBranch
	cmdConfig := exec.Command("git", "-C", dir, "config", "init.defaultBranch", "main")
	_ = cmdConfig.Run()

	got = repoDefaultBranch(context.Background(), dir)
	if got != "main" {
		t.Errorf("default branch = %q, want main", got)
	}
}

func TestWorktreeIsPrincipalBranch(t *testing.T) {
	cases := []struct {
		branch      string
		isPrincipal bool
		want        bool
	}{
		{"master", false, true},
		{"main", false, true},
		{"MASTER", false, true},
		{"Main", false, true},
		{"trunk", true, true},
		{"trunk", false, false},
		{"feature/abc", false, false},
		{"", false, false},
	}

	for _, tc := range cases {
		wt := Worktree{Branch: tc.branch, IsPrincipal: tc.isPrincipal}
		if got := wt.IsPrincipalBranch(); got != tc.want {
			t.Errorf("Worktree(branch=%q, isPrincipal=%v).IsPrincipalBranch() = %v, want %v",
				tc.branch, tc.isPrincipal, got, tc.want)
		}
	}
}
