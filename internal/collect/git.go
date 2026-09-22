package collect

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// commonBins are prepended to PATH for the tools this plugin shells out to.
// Herdr runs plugin commands and panes with a minimal PATH, so plain "git" is
// not guaranteed to resolve.
var commonBins = []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin"}

// GitStatus is the working-tree summary shown on a worktree row.
type GitStatus struct {
	// Known is false when git could not be run or the directory is gone. The
	// row then shows nothing rather than a confident "clean".
	Known     bool
	Branch    string
	Ahead     int
	Behind    int
	Staged    int
	Unstaged  int
	Conflicts int
	Untracked int
}

// Dirty reports whether the checkout has any tracked change or conflict.
// Untracked files alone do not count as dirty: build output and editor droppings
// would otherwise mark half these worktrees permanently modified.
func (s GitStatus) Dirty() bool {
	return s.Staged+s.Unstaged+s.Conflicts > 0
}

// gitStatus reads one checkout with a single porcelain v2 call, which reports
// the branch, the ahead/behind counts and every changed path at once.
func gitStatus(ctx context.Context, path string) GitStatus {
	cmd := exec.CommandContext(ctx, "git",
		"-C", path,
		"--no-optional-locks", // never take the index lock out from under a running agent
		"status", "--porcelain=v2", "--branch", "--untracked-files=normal",
	)
	cmd.Env = hardenedEnv()

	out, err := cmd.Output()
	if err != nil {
		return GitStatus{}
	}

	return parsePorcelainV2(string(out))
}

func parsePorcelainV2(out string) GitStatus {
	status := GitStatus{Known: true}

	scanner := bufio.NewScanner(strings.NewReader(out))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		switch {
		case strings.HasPrefix(line, "# branch.head "):
			head := strings.TrimPrefix(line, "# branch.head ")
			if head != "(detached)" {
				status.Branch = head
			}

		case strings.HasPrefix(line, "# branch.ab "):
			status.Ahead, status.Behind = parseAheadBehind(strings.TrimPrefix(line, "# branch.ab "))

		case strings.HasPrefix(line, "1 "), strings.HasPrefix(line, "2 "):
			// Field 2 is the two-letter XY code: X is the staged change, Y the
			// unstaged one, "." meaning none.
			fields := strings.Fields(line)
			if len(fields) < 2 || len(fields[1]) < 2 {
				continue
			}

			if fields[1][0] != '.' {
				status.Staged++
			}

			if fields[1][1] != '.' {
				status.Unstaged++
			}

		case strings.HasPrefix(line, "u "):
			status.Conflicts++

		case strings.HasPrefix(line, "? "):
			status.Untracked++
		}
	}

	return status
}

// parseAheadBehind reads the "+3 -1" form of the branch.ab header.
func parseAheadBehind(s string) (ahead, behind int) {
	for _, field := range strings.Fields(s) {
		if len(field) < 2 {
			continue
		}

		n, err := strconv.Atoi(field[1:])
		if err != nil {
			continue
		}

		switch field[0] {
		case '+':
			ahead = n
		case '-':
			behind = n
		}
	}

	return ahead, behind
}

// toolPath resolves an external tool, searching the common binary directories
// before PATH. It exists because exec.Command resolves a bare name against the
// parent process's PATH and not the one in cmd.Env, so hardenedEnv alone cannot
// find a tool that Herdr's minimal PATH leaves out.
func toolPath(name string) (string, bool) {
	for _, dir := range commonBins {
		candidate := filepath.Join(dir, name)

		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			return candidate, true
		}
	}

	if path, err := exec.LookPath(name); err == nil {
		return path, true
	}

	return "", false
}

// hardenedEnv is the process environment with the common binary directories
// prepended to PATH.
func hardenedEnv() []string {
	env := os.Environ()
	path := os.Getenv("PATH")

	prefix := strings.Join(commonBins, ":")
	if path != "" {
		path = prefix + ":" + path
	} else {
		path = prefix
	}

	out := make([]string, 0, len(env)+1)

	for _, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			continue
		}

		out = append(out, kv)
	}

	return append(out, "PATH="+path)
}
