package collect

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// Protected branch names that must never be removed under any circumstance.
var canonicalProtectedBranches = []string{
	"main",
	"master",
	"trunk",
	"develop",
	"development",
	"default",
	"head",
}

// IsProtectedWorktree verifies whether a worktree is a primary checkout, a
// principal branch, a default branch, or matches any configured or canonical
// principal branch names.
//
// It reports true and a descriptive reason if the worktree must be preserved,
// or false and "" if it is safe to delete.
func IsProtectedWorktree(ctx context.Context, wt *Worktree, configPrincipal []string) (bool, string) {
	if wt == nil {
		return true, "nil worktree"
	}

	if wt.Path == "" || wt.RepoRoot == "" {
		return true, "missing worktree path or repository root"
	}

	cleanPath := filepath.Clean(wt.Path)
	cleanRoot := filepath.Clean(wt.RepoRoot)

	// Primary repository checkout protection: the root working tree can never
	// be removed without corrupting or deleting the repository itself.
	if !wt.IsLinked || cleanPath == cleanRoot {
		return true, fmt.Sprintf("primary repository checkout (%s)", wt.RepoRoot)
	}

	// Flagged principal branch check
	if wt.IsPrincipal || wt.IsPrincipalBranch() {
		return true, fmt.Sprintf("principal branch %s", wt.Branch)
	}

	// Canonical protected names check
	cleanBranch := strings.ToLower(strings.TrimSpace(wt.Branch))
	for _, canon := range canonicalProtectedBranches {
		if cleanBranch == canon {
			return true, fmt.Sprintf("protected canonical branch %s", wt.Branch)
		}
	}

	// User-configured principal branches
	for _, p := range configPrincipal {
		if strings.EqualFold(wt.Branch, p) {
			return true, fmt.Sprintf("configured principal branch %s", wt.Branch)
		}
	}

	// Repository default branch (e.g. from origin/HEAD or init.defaultBranch)
	if defaultBranch := repoDefaultBranch(ctx, wt.RepoRoot); defaultBranch != "" {
		if strings.EqualFold(wt.Branch, defaultBranch) {
			return true, fmt.Sprintf("repository default branch %s", wt.Branch)
		}
	}

	// Deep check against git's own worktree list: the first entry is always the main worktree.
	cmd := exec.CommandContext(ctx, "git",
		"-C", wt.RepoRoot,
		"--no-optional-locks",
		"worktree", "list", "--porcelain",
	)
	cmd.Env = hardenedEnv()

	if out, err := cmd.Output(); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(out)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "worktree ") {
				firstWt := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
				if filepath.Clean(firstWt) == cleanPath {
					return true, fmt.Sprintf("git main working tree (%s)", firstWt)
				}

				break
			}
		}
	}

	return false, ""
}

// RemoveGitWorktree removes a linked worktree from disk via git.
func RemoveGitWorktree(ctx context.Context, repoRoot, worktreePath string, force bool) error {
	args := []string{"-C", repoRoot, "--no-optional-locks", "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Env = hardenedEnv()

	if out, err := cmd.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("git worktree remove: %s", msg)
		}

		return fmt.Errorf("git worktree remove: %w", err)
	}

	return nil
}
