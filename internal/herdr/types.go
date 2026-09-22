// Package herdr speaks the Herdr socket API: the NDJSON protocol on
// $HERDR_SOCKET_PATH that mirrors the herdr CLI one method per subcommand.
package herdr

import (
	"encoding/json"
	"fmt"
)

// Methods used by the plugin. The socket names them with dots; the CLI spells
// the same calls with spaces (session.snapshot is `herdr api snapshot`).
const (
	MethodSessionSnapshot         = "session.snapshot"
	MethodWorktreeList            = "worktree.list"
	MethodWorktreeOpen            = "worktree.open"
	MethodWorkspaceFocus          = "workspace.focus"
	MethodWorkspaceReportMetadata = "workspace.report_metadata"
)

// AgentStatus values Herdr reports for a pane running a known agent.
const (
	StatusBlocked = "blocked"
	StatusWorking = "working"
	StatusDone    = "done"
	StatusIdle    = "idle"
	StatusUnknown = "unknown"
)

// Snapshot is the whole session in one answer: every workspace, pane and agent.
// The plugin polls this rather than subscribing — see collect.Collector.
type Snapshot struct {
	Version            string      `json:"version"`
	Protocol           int         `json:"protocol"`
	Workspaces         []Workspace `json:"workspaces"`
	Panes              []Pane      `json:"panes"`
	Agents             []Agent     `json:"agents"`
	FocusedWorkspaceID string      `json:"focused_workspace_id"`
	FocusedTabID       string      `json:"focused_tab_id"`
	FocusedPaneID      string      `json:"focused_pane_id"`
}

// Workspace is one entry in Herdr's own sidebar. Worktree is nil for a
// workspace whose cwd is not inside a git repository.
type Workspace struct {
	WorkspaceID string             `json:"workspace_id"`
	Number      int                `json:"number"`
	Label       string             `json:"label"`
	Focused     bool               `json:"focused"`
	PaneCount   int                `json:"pane_count"`
	TabCount    int                `json:"tab_count"`
	ActiveTabID string             `json:"active_tab_id"`
	AgentStatus string             `json:"agent_status"`
	Tokens      map[string]string  `json:"tokens"`
	Worktree    *WorkspaceWorktree `json:"worktree"`
}

// WorkspaceWorktree is the git checkout a workspace is bound to. RepoRoot is
// the primary checkout and the only cwd worktree.list accepts for this repo.
type WorkspaceWorktree struct {
	RepoKey          string `json:"repo_key"`
	RepoName         string `json:"repo_name"`
	RepoRoot         string `json:"repo_root"`
	CheckoutPath     string `json:"checkout_path"`
	IsLinkedWorktree bool   `json:"is_linked_worktree"`
}

// Worktree is a git checkout as worktree.list reports it. It carries no id and
// no writable metadata, so Path is the only stable handle a plugin can key on.
// OpenWorkspaceID is nil for a checkout that exists on disk but is not open in
// Herdr — including worktrees other tools created.
type Worktree struct {
	Path             string  `json:"path"`
	Branch           *string `json:"branch"`
	Label            string  `json:"label"`
	IsBare           bool    `json:"is_bare"`
	IsDetached       bool    `json:"is_detached"`
	IsPrunable       bool    `json:"is_prunable"`
	IsLinkedWorktree bool    `json:"is_linked_worktree"`
	OpenWorkspaceID  *string `json:"open_workspace_id"`
}

// BranchName is the branch or an empty string when the checkout is detached.
func (w Worktree) BranchName() string {
	if w.Branch == nil {
		return ""
	}

	return *w.Branch
}

type Pane struct {
	PaneID      string `json:"pane_id"`
	WorkspaceID string `json:"workspace_id"`
	TabID       string `json:"tab_id"`
	Cwd         string `json:"cwd"`
	Label       string `json:"label"`
	Focused     bool   `json:"focused"`
	AgentStatus string `json:"agent_status"`
	Revision    int64  `json:"revision"`
}

// Agent is a pane Herdr recognised as running a coding agent. Unlike
// Workspace.AgentStatus — which reads "unknown" in workspace.list — these carry
// the live state, so the sidebar derives its status from here.
type Agent struct {
	PaneID                string `json:"pane_id"`
	WorkspaceID           string `json:"workspace_id"`
	TabID                 string `json:"tab_id"`
	Agent                 string `json:"agent"`
	AgentStatus           string `json:"agent_status"`
	Cwd                   string `json:"cwd"`
	ForegroundCwd         string `json:"foreground_cwd"`
	TerminalTitleStripped string `json:"terminal_title_stripped"`
	Focused               bool   `json:"focused"`
	StateChangeSeq        int64  `json:"state_change_seq"`
}

// --- request params ---

type emptyParams struct{}

type WorktreeListParams struct {
	Cwd             string `json:"cwd"`
	TrustRepository bool   `json:"trust_repository,omitempty"`
}

type WorktreeOpenParams struct {
	Cwd   string `json:"cwd"`
	Path  string `json:"path"`
	Focus bool   `json:"focus"`
}

type WorkspaceFocusParams struct {
	WorkspaceID string `json:"workspace_id"`
}

// WorkspaceReportMetadataParams carries the tokens Herdr's own sidebar renders
// as $name. Tokens are display-only: they never reach session.json, so they
// must be restamped after a server restart. A nil value clears one.
type WorkspaceReportMetadataParams struct {
	WorkspaceID string             `json:"workspace_id"`
	Source      string             `json:"source"`
	Tokens      map[string]*string `json:"tokens"`
}

// --- response envelopes ---

type frame struct {
	Result json.RawMessage `json:"result"`
	Error  *APIError       `json:"error"`
}

// APIError is the {"code","message"} object Herdr answers with instead of a
// sentence. Callers log the code and show the user their own wording.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

type snapshotResult struct {
	Snapshot Snapshot `json:"snapshot"`
}

type worktreeListResult struct {
	Source    WorktreeSource `json:"source"`
	Worktrees []Worktree     `json:"worktrees"`
}

type WorktreeSource struct {
	RepoKey            string `json:"repo_key"`
	RepoName           string `json:"repo_name"`
	RepoRoot           string `json:"repo_root"`
	SourceCheckoutPath string `json:"source_checkout_path"`
	SourceWorkspaceID  string `json:"source_workspace_id"`
}

type worktreeOpenResult struct {
	Workspace Workspace `json:"workspace"`
}
