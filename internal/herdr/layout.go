package herdr

import "context"

// Layout methods. A pane can read the split tree it lives in and change the
// ratio of the split that holds it, which is how the sidebar sizes itself: the
// plugin pane API has no width for a split placement.
const (
	MethodPaneLayout          = "pane.layout"
	MethodLayoutExport        = "layout.export"
	MethodLayoutSetSplitRatio = "layout.set_split_ratio"
)

// Node is one node of a tab's layout tree: either a split with two children or
// a leaf pane.
type Node struct {
	Type      string  `json:"type"` // "split" or "pane"
	Direction string  `json:"direction"`
	Ratio     float64 `json:"ratio"`
	First     *Node   `json:"first"`
	Second    *Node   `json:"second"`
	PaneID    string  `json:"pane_id"`
}

// IsSplit reports whether the node divides its area between two children.
func (n *Node) IsSplit() bool {
	return n != nil && n.Type == "split" && n.First != nil && n.Second != nil
}

// Horizontal reports whether the split places its children side by side, which
// is the only orientation a width can be asked of.
func (n *Node) Horizontal() bool {
	return n != nil && n.Direction == "right"
}

type Layout struct {
	WorkspaceID   string `json:"workspace_id"`
	TabID         string `json:"tab_id"`
	FocusedPaneID string `json:"focused_pane_id"`
	Root          *Node  `json:"root"`
}

// Rect is a cell rectangle in the tab's grid.
type Rect struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Right is the first column past the rectangle.
func (r Rect) Right() int { return r.X + r.Width }

// Geometry is where every pane of a tab actually sits. The layout tree carries
// ratios; this carries cells, and the sidebar needs both to turn "40 columns"
// into a ratio.
type Geometry struct {
	Area  Rect `json:"area"`
	Panes []struct {
		PaneID string `json:"pane_id"`
		Rect   Rect   `json:"rect"`
	} `json:"panes"`
}

// Rects indexes the geometry by pane id.
func (g Geometry) Rects() map[string]Rect {
	out := make(map[string]Rect, len(g.Panes))
	for _, p := range g.Panes {
		out[p.PaneID] = p.Rect
	}

	return out
}

type paneLayoutParams struct {
	PaneID string `json:"pane_id"`
}

type layoutExportParams struct {
	PaneID string `json:"pane_id"`
}

// SetSplitRatioParams addresses a split by the branches taken from the root:
// false for the first child, true for the second. An empty path is the root
// split itself.
type SetSplitRatioParams struct {
	PaneID string  `json:"pane_id"`
	Path   []bool  `json:"path"`
	Ratio  float64 `json:"ratio"`
}

type paneLayoutResult struct {
	Layout Geometry `json:"layout"`
}

type layoutExportResult struct {
	Layout Layout `json:"layout"`
}

// PaneGeometry answers where the panes of this pane's tab sit, in cells.
func PaneGeometry(ctx context.Context, c Client, paneID string) (Geometry, error) {
	var res paneLayoutResult
	if err := c.Call(ctx, MethodPaneLayout, paneLayoutParams{PaneID: paneID}, &res); err != nil {
		return Geometry{}, err
	}

	return res.Layout, nil
}

// ExportLayout answers the split tree of this pane's tab.
func ExportLayout(ctx context.Context, c Client, paneID string) (Layout, error) {
	var res layoutExportResult
	if err := c.Call(ctx, MethodLayoutExport, layoutExportParams{PaneID: paneID}, &res); err != nil {
		return Layout{}, err
	}

	return res.Layout, nil
}

// SetSplitRatio moves one split divider. Path is empty for the root split.
func SetSplitRatio(ctx context.Context, c Client, paneID string, path []bool, ratio float64) error {
	// A nil slice encodes as null, which the server rejects; the root split is
	// an empty path, not a missing one.
	if path == nil {
		path = []bool{}
	}

	return c.Call(ctx, MethodLayoutSetSplitRatio, SetSplitRatioParams{PaneID: paneID, Path: path, Ratio: ratio}, nil)
}
