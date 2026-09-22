package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
)

// socketPathEnv names the socket Herdr made for this session. Herdr sets it on
// every plugin command and every pane it opens.
const socketPathEnv = "HERDR_SOCKET_PATH"

// sessionEnv names a session when the socket path itself is not given.
const sessionEnv = "HERDR_SESSION"

// Client is the slice of the Herdr socket API this plugin uses.
type Client interface {
	Call(ctx context.Context, method string, params any, result any) error
}

// SocketClient speaks NDJSON to the Herdr socket. Herdr answers one request per
// connection and then closes it, so each call dials its own and there is
// nothing to reconnect.
type SocketClient struct {
	path string
	seq  atomic.Uint64
}

var _ Client = (*SocketClient)(nil)

// New builds a client for this session's socket. It performs no I/O; the first
// connection is made by the first call.
func New() (*SocketClient, error) {
	path, err := SocketPath()
	if err != nil {
		return nil, err
	}

	return &SocketClient{path: path}, nil
}

// SocketPath resolves the socket the way Herdr's own client does: the explicit
// path first, then the named session, then the default session. The fallbacks
// matter because the binary is also useful from a plain shell pane, where only
// a plugin command would have HERDR_SOCKET_PATH set.
func SocketPath() (string, error) {
	if path := os.Getenv(socketPathEnv); path != "" {
		return path, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("%s is not set and the home directory is unknown: %w", socketPathEnv, err)
	}

	base := filepath.Join(home, ".config", "herdr")

	if session := os.Getenv(sessionEnv); session != "" {
		return filepath.Join(base, "sessions", session, "herdr.sock"), nil
	}

	return filepath.Join(base, "herdr.sock"), nil
}

// Call sends one request and reads the single response Herdr answers with.
func (c *SocketClient) Call(ctx context.Context, method string, params any, result any) error {
	// Herdr rejects a request whose params are absent, so an empty object
	// stands in for "no arguments".
	if params == nil {
		params = emptyParams{}
	}

	var dialer net.Dialer

	conn, err := dialer.DialContext(ctx, "unix", c.path)
	if err != nil {
		return fmt.Errorf("connect to herdr socket %s: %w", c.path, err)
	}
	defer conn.Close()

	// Unblock a call whose context is cancelled while it waits on the socket.
	stopCancel := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopCancel()

	req := struct {
		ID     string `json:"id"`
		Method string `json:"method"`
		Params any    `json:"params"`
	}{
		ID:     fmt.Sprintf("wayfindr-%d", c.seq.Add(1)),
		Method: method,
		Params: params,
	}

	if err := json.NewEncoder(conn).Encode(req); err != nil {
		return withContextErr(ctx, fmt.Errorf("send %s: %w", method, err))
	}

	line, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil && len(line) == 0 {
		return withContextErr(ctx, fmt.Errorf("read %s response: %w", method, err))
	}

	var f frame
	if err := json.Unmarshal(line, &f); err != nil {
		return fmt.Errorf("decode %s response: %w", method, err)
	}

	if f.Error != nil {
		return fmt.Errorf("%s: %w", method, f.Error)
	}

	if result != nil && len(f.Result) > 0 {
		if err := json.Unmarshal(f.Result, result); err != nil {
			return fmt.Errorf("decode %s result: %w", method, err)
		}
	}

	return nil
}

// withContextErr reports cancellation as such rather than as the socket error
// that closing the connection produced.
func withContextErr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return ctxErr
	}

	return err
}

// SessionSnapshot fetches every workspace, pane and agent in one call.
func SessionSnapshot(ctx context.Context, c Client) (Snapshot, error) {
	var res snapshotResult
	if err := c.Call(ctx, MethodSessionSnapshot, emptyParams{}, &res); err != nil {
		return Snapshot{}, err
	}

	return res.Snapshot, nil
}

// ListWorktrees answers every checkout of the repository containing cwd,
// including ones no workspace has open and ones other tools created. It is
// per-repository: outside a git work tree it fails with not_git_worktree, so
// callers pass a known repo root rather than an arbitrary directory.
func ListWorktrees(ctx context.Context, c Client, repoRoot string) ([]Worktree, WorktreeSource, error) {
	var res worktreeListResult
	if err := c.Call(ctx, MethodWorktreeList, WorktreeListParams{Cwd: repoRoot}, &res); err != nil {
		return nil, WorktreeSource{}, err
	}

	return res.Worktrees, res.Source, nil
}

// OpenWorktree adopts an existing checkout as a workspace and answers it.
func OpenWorktree(ctx context.Context, c Client, repoRoot, path string, focus bool) (Workspace, error) {
	var res worktreeOpenResult

	params := WorktreeOpenParams{Cwd: repoRoot, Path: path, Focus: focus}
	if err := c.Call(ctx, MethodWorktreeOpen, params, &res); err != nil {
		return Workspace{}, err
	}

	return res.Workspace, nil
}

// FocusWorkspace brings a workspace to the front.
func FocusWorkspace(ctx context.Context, c Client, workspaceID string) error {
	return c.Call(ctx, MethodWorkspaceFocus, WorkspaceFocusParams{WorkspaceID: workspaceID}, nil)
}

// ReportWorkspaceTokens publishes display tokens for one workspace under this
// plugin's source namespace. Herdr renders them in its own sidebar wherever the
// user's row template names them with a $ prefix.
func ReportWorkspaceTokens(ctx context.Context, c Client, source, workspaceID string, tokens map[string]*string) error {
	params := WorkspaceReportMetadataParams{
		WorkspaceID: workspaceID,
		Source:      source,
		Tokens:      tokens,
	}

	return c.Call(ctx, MethodWorkspaceReportMetadata, params, nil)
}
