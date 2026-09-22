// Package tokens publishes each workspace's group to Herdr's own sidebar.
//
// This is the one way a plugin contributes to the built-in sidebar: there is no
// sidebar section in the manifest. Tokens reported here render wherever the
// user's [ui.sidebar.spaces] row template names them with a $ prefix. They are
// display-only and never reach session.json, so they have to be restamped after
// a Herdr restart — which is what the [[startup]] hook is for.
package tokens

import (
	"context"
	"strings"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// maxValue keeps a pathological group name from crowding out the rest of a
// sidebar row.
const maxValue = 48

// Reporter stamps group tokens and remembers what it last sent, so a steady
// state costs no socket calls and leaves no trail in `herdr plugin log list`.
type Reporter struct {
	client herdr.Client
	name   string
	last   map[string]string
}

func NewReporter(client herdr.Client, tokenName string) *Reporter {
	return &Reporter{client: client, name: tokenName, last: map[string]string{}}
}

// Report publishes one token per open workspace and clears it where the group
// went away. Errors are collected rather than returned one at a time: a single
// closing workspace must not stop the rest from being stamped.
func (r *Reporter) Report(ctx context.Context, world collect.World, assignments map[string]group.Assignment) []error {
	var errs []error

	wanted := map[string]string{}

	for _, wt := range world.Worktrees {
		if !wt.Open() {
			continue
		}

		value := sanitize(assignments[wt.Path].Name)
		if value == "" {
			continue
		}

		wanted[wt.WorkspaceID] = value
	}

	for workspaceID, value := range wanted {
		if r.last[workspaceID] == value {
			continue
		}

		if err := r.send(ctx, workspaceID, &value); err != nil {
			errs = append(errs, err)

			continue
		}

		r.last[workspaceID] = value
	}

	for workspaceID := range r.last {
		if _, ok := wanted[workspaceID]; ok {
			continue
		}

		// A nil value clears the token. Clearing a workspace that has already
		// closed answers an error, which is harmless — forget it either way.
		if err := r.send(ctx, workspaceID, nil); err != nil {
			errs = append(errs, err)
		}

		delete(r.last, workspaceID)
	}

	return errs
}

func (r *Reporter) send(ctx context.Context, workspaceID string, value *string) error {
	return herdr.ReportWorkspaceTokens(ctx, r.client, config.PluginID, workspaceID, map[string]*string{
		r.name: value,
	})
}

// sanitize keeps a group name to one short single-line label. Herdr renders the
// value inline in a sidebar row, where a newline or a tab would break the row.
func sanitize(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}

	name = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}

		return r
	}, name)

	name = strings.Join(strings.Fields(name), " ")

	if len(name) > maxValue {
		name = strings.TrimSpace(name[:maxValue-1]) + "…"
	}

	return name
}
