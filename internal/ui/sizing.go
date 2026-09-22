package ui

import "github.com/echeble/herdr-wayfindr/internal/herdr"

// minPaneWidth is the narrowest either side of the split may be left. Asking
// for a sidebar wider than the window would otherwise squeeze the pane you
// actually work in down to nothing.
const minPaneWidth = 12

// Resize is the split adjustment that makes this pane a given number of cells
// wide.
type Resize struct {
	Path  []bool
	Ratio float64
}

// ratioForWidth works out how to move one divider so that paneID ends up
// target cells wide.
//
// Herdr's plugin pane API has no width for a split placement — only a popup
// takes one — so a sidebar that wants a fixed width has to move the split
// itself. That means finding the split holding this pane, measuring how much
// room that split has, and converting cells into the ratio the API speaks.
//
// It answers false when there is nothing sensible to do: a lone pane, a
// stacked split, a missing rectangle, or a target that will not fit.
func ratioForWidth(root *herdr.Node, rects map[string]herdr.Rect, paneID string, target int) (Resize, bool) {
	branches, found := pathTo(root, paneID)
	if !found || len(branches) == 0 {
		return Resize{}, false
	}

	// The last branch is the step into this pane, so the split that holds it
	// is one level up.
	splitPath := branches[:len(branches)-1]
	side := branches[len(branches)-1]

	split := nodeAt(root, splitPath)
	if !split.IsSplit() || !split.Horizontal() {
		return Resize{}, false
	}

	self, ok := rects[paneID]
	if !ok || self.Width <= 0 {
		return Resize{}, false
	}

	sibling := split.First
	if !side {
		sibling = split.Second
	}

	siblingWidth, ok := subtreeWidth(sibling, rects)
	if !ok {
		return Resize{}, false
	}

	// Herdr draws no divider column: the two children fill the split exactly.
	total := self.Width + siblingWidth
	if total < 2*minPaneWidth {
		return Resize{}, false
	}

	if target < minPaneWidth {
		target = minPaneWidth
	}

	if target > total-minPaneWidth {
		target = total - minPaneWidth
	}

	// The ratio is always the first child's share.
	share := target
	if side {
		share = total - target
	}

	return Resize{Path: splitPath, Ratio: float64(share) / float64(total)}, true
}

// pathTo records the branches taken from root to reach paneID: false into the
// first child, true into the second.
func pathTo(node *herdr.Node, paneID string) ([]bool, bool) {
	if node == nil {
		return nil, false
	}

	if node.PaneID == paneID && node.Type == "pane" {
		return []bool{}, true
	}

	if !node.IsSplit() {
		return nil, false
	}

	if path, ok := pathTo(node.First, paneID); ok {
		return append([]bool{false}, path...), true
	}

	if path, ok := pathTo(node.Second, paneID); ok {
		return append([]bool{true}, path...), true
	}

	return nil, false
}

func nodeAt(node *herdr.Node, path []bool) *herdr.Node {
	for _, second := range path {
		if !node.IsSplit() {
			return nil
		}

		if second {
			node = node.Second
		} else {
			node = node.First
		}
	}

	return node
}

// subtreeWidth measures how wide a subtree is on screen, from the rectangles of
// the panes it contains. A subtree can be a single pane or a whole nested
// split, and either way its width is the span its panes cover.
func subtreeWidth(node *herdr.Node, rects map[string]herdr.Rect) (int, bool) {
	ids := paneIDs(node)
	if len(ids) == 0 {
		return 0, false
	}

	left, right := 0, 0
	seen := false

	for _, id := range ids {
		rect, ok := rects[id]
		if !ok {
			continue
		}

		if !seen {
			left, right = rect.X, rect.Right()
			seen = true

			continue
		}

		left = min(left, rect.X)
		right = max(right, rect.Right())
	}

	if !seen || right <= left {
		return 0, false
	}

	return right - left, true
}

func paneIDs(node *herdr.Node) []string {
	if node == nil {
		return nil
	}

	if node.Type == "pane" {
		if node.PaneID == "" {
			return nil
		}

		return []string{node.PaneID}
	}

	return append(paneIDs(node.First), paneIDs(node.Second)...)
}
