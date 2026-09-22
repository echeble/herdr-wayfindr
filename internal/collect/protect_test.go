package collect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIsProtectedWorktree(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		wt        *Worktree
		cfgPrinc  []string
		protected bool
	}{
		{
			name:      "nil worktree",
			wt:        nil,
			protected: true,
		},
		{
			name: "empty paths",
			wt: &Worktree{
				Path:     "",
				RepoRoot: "",
				Branch:   "feature/foo",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "primary checkout isLinked false",
			wt: &Worktree{
				Path:     "/repo/primary",
				RepoRoot: "/repo/primary",
				Branch:   "feature/foo",
				IsLinked: false,
			},
			protected: true,
		},
		{
			name: "primary checkout path equals repo root",
			wt: &Worktree{
				Path:     "/repo/primary",
				RepoRoot: "/repo/primary",
				Branch:   "feature/foo",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "flagged isPrincipal true",
			wt: &Worktree{
				Path:        "/repo/wt-1",
				RepoRoot:    "/repo/primary",
				Branch:      "some-branch",
				IsLinked:    true,
				IsPrincipal: true,
			},
			protected: true,
		},
		{
			name: "main branch",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "main",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "master branch uppercase",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "MASTER",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "trunk branch",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "trunk",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "develop branch",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "Develop",
				IsLinked: true,
			},
			protected: true,
		},
		{
			name: "configured principal branch",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "release-v1",
				IsLinked: true,
			},
			cfgPrinc:  []string{"release-v1"},
			protected: true,
		},
		{
			name: "safe linked feature worktree",
			wt: &Worktree{
				Path:     "/repo/wt-1",
				RepoRoot: "/repo/primary",
				Branch:   "feature/WAYF-123",
				IsLinked: true,
			},
			protected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotProtected, reason := IsProtectedWorktree(ctx, tc.wt, tc.cfgPrinc)
			if gotProtected != tc.protected {
				t.Fatalf("IsProtectedWorktree() = %v (reason: %q), want %v", gotProtected, reason, tc.protected)
			}
		})
	}
}

func TestRemoveGitWorktree(t *testing.T) {
	ctx := context.Background()

	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	wtDir := filepath.Join(tmpDir, "wt-feature")

	runCmd := func(dir string, name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s %v failed: %v\nOutput: %s", name, args, err, out)
		}
	}

	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatal(err)
	}

	runCmd(repoDir, "git", "init", "-b", "main")
	runCmd(repoDir, "git", "config", "user.email", "test@test.com")
	runCmd(repoDir, "git", "config", "user.name", "Test")

	dummyFile := filepath.Join(repoDir, "file.txt")
	if err := os.WriteFile(dummyFile, []byte("init"), 0o644); err != nil {
		t.Fatal(err)
	}

	runCmd(repoDir, "git", "add", "file.txt")
	runCmd(repoDir, "git", "commit", "-m", "init")

	// Add linked worktree
	runCmd(repoDir, "git", "worktree", "add", "-b", "feature-test", wtDir)

	wt := &Worktree{
		Path:     wtDir,
		RepoRoot: repoDir,
		Branch:   "feature-test",
		IsLinked: true,
	}

	// Verify not protected
	if protected, reason := IsProtectedWorktree(ctx, wt, nil); protected {
		t.Fatalf("expected feature worktree to be deletable, got protected: %s", reason)
	}

	// Verify main worktree IS protected
	mainWt := &Worktree{
		Path:     repoDir,
		RepoRoot: repoDir,
		Branch:   "main",
		IsLinked: false,
	}
	if protected, _ := IsProtectedWorktree(ctx, mainWt, nil); !protected {
		t.Fatal("expected main worktree to be protected")
	}

	// Now remove the linked worktree
	if err := RemoveGitWorktree(ctx, repoDir, wtDir, true); err != nil {
		t.Fatalf("RemoveGitWorktree failed: %v", err)
	}

	// Verify directory is gone
	if _, err := os.Stat(wtDir); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be removed, but it still exists", wtDir)
	}
}
