package herdr

import "context"

// MethodPaneInputSet routes an input a pane's own chrome would otherwise
// intercept — right click among them — either to Herdr's own handling or to
// the pane itself. It is the socket API behind the pane menu's own "Send
// right-clicks to pane" entry.
const MethodPaneInputSet = "pane.input.set"

// PaneRightClickTarget matches Herdr's own enum: whether a right click opens
// Herdr's pane menu (rename, split, zoom, …) or is forwarded to the pane to
// draw one of its own.
type PaneRightClickTarget string

const (
	RightClickHerdr PaneRightClickTarget = "herdr"
	RightClickPane  PaneRightClickTarget = "pane"
)

type paneInputSetParams struct {
	PaneID     string               `json:"pane_id"`
	RightClick PaneRightClickTarget `json:"right_click"`
}

// SetPaneRightClick asks Herdr to forward this pane's right clicks to it
// instead of opening Herdr's own pane menu, so a config that promises a
// context menu is the menu a click actually shows.
func SetPaneRightClick(ctx context.Context, c Client, paneID string, toPane bool) error {
	target := RightClickHerdr
	if toPane {
		target = RightClickPane
	}

	return c.Call(ctx, MethodPaneInputSet, paneInputSetParams{PaneID: paneID, RightClick: target}, nil)
}
