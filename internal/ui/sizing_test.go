package ui

import (
	"testing"

	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

func pane(id string) *herdr.Node {
	return &herdr.Node{Type: "pane", PaneID: id}
}

func split(direction string, first, second *herdr.Node) *herdr.Node {
	return &herdr.Node{Type: "split", Direction: direction, First: first, Second: second}
}

func rect(x, width int) herdr.Rect {
	return herdr.Rect{X: x, Width: width, Height: 50}
}

func TestRatioForWidthRootSplitSidebarOnTheRight(t *testing.T) {
	// The real shape: one work pane, the sidebar split off to its right.
	root := split("right", pane("w1:p1"), pane("w1:p2"))
	rects := map[string]herdr.Rect{
		"w1:p1": rect(0, 164),
		"w1:p2": rect(164, 40),
	}

	got, ok := ratioForWidth(root, rects, "w1:p2", 40)
	if !ok {
		t.Fatal("expected a resize")
	}

	if len(got.Path) != 0 {
		t.Errorf("path = %v, want the root split", got.Path)
	}

	// 40 of 204 on the right leaves 164/204 for the first child.
	if want := 164.0 / 204.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

func TestRatioForWidthSidebarOnTheLeft(t *testing.T) {
	root := split("right", pane("w1:p2"), pane("w1:p1"))
	rects := map[string]herdr.Rect{
		"w1:p2": rect(0, 100),
		"w1:p1": rect(100, 104),
	}

	got, ok := ratioForWidth(root, rects, "w1:p2", 40)
	if !ok {
		t.Fatal("expected a resize")
	}

	// As the first child the sidebar takes the ratio directly.
	if want := 40.0 / 204.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

func TestRatioForWidthNestedSplit(t *testing.T) {
	// Two stacked work panes on the left, the sidebar on the right of both.
	root := split("right",
		split("down", pane("w1:p1"), pane("w1:p3")),
		pane("w1:p2"),
	)
	rects := map[string]herdr.Rect{
		"w1:p1": {X: 0, Width: 164, Y: 0, Height: 25},
		"w1:p3": {X: 0, Width: 164, Y: 25, Height: 25},
		"w1:p2": {X: 164, Width: 40, Y: 0, Height: 50},
	}

	got, ok := ratioForWidth(root, rects, "w1:p2", 50)
	if !ok {
		t.Fatal("expected a resize")
	}

	if len(got.Path) != 0 {
		t.Errorf("path = %v, want the root split", got.Path)
	}

	// The stacked siblings span 164 cells between them, not 328.
	if want := 154.0 / 204.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

func TestRatioForWidthDeeperPathIsReported(t *testing.T) {
	// The sidebar hangs off a nested split, so the divider to move is not the
	// root one.
	root := split("right",
		pane("w1:p1"),
		split("right", pane("w1:p3"), pane("w1:p2")),
	)
	rects := map[string]herdr.Rect{
		"w1:p1": rect(0, 100),
		"w1:p3": rect(100, 64),
		"w1:p2": rect(164, 40),
	}

	got, ok := ratioForWidth(root, rects, "w1:p2", 30)
	if !ok {
		t.Fatal("expected a resize")
	}

	if len(got.Path) != 1 || got.Path[0] != true {
		t.Fatalf("path = %v, want [true] — the split inside the second child", got.Path)
	}

	// Only the nested split's 104 cells are up for grabs.
	if want := 74.0 / 104.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

func TestRatioForWidthRefusesStackedSplit(t *testing.T) {
	root := split("down", pane("w1:p1"), pane("w1:p2"))
	rects := map[string]herdr.Rect{
		"w1:p1": {X: 0, Width: 204, Y: 0, Height: 25},
		"w1:p2": {X: 0, Width: 204, Y: 25, Height: 25},
	}

	if _, ok := ratioForWidth(root, rects, "w1:p2", 40); ok {
		t.Fatal("a stacked split has no width to set")
	}
}

func TestRatioForWidthRefusesLonePane(t *testing.T) {
	root := pane("w1:p1")
	rects := map[string]herdr.Rect{"w1:p1": rect(0, 204)}

	if _, ok := ratioForWidth(root, rects, "w1:p1", 40); ok {
		t.Fatal("a pane with no split cannot be resized")
	}
}

func TestRatioForWidthRefusesUnknownPane(t *testing.T) {
	root := split("right", pane("w1:p1"), pane("w1:p2"))
	rects := map[string]herdr.Rect{"w1:p1": rect(0, 164), "w1:p2": rect(164, 40)}

	if _, ok := ratioForWidth(root, rects, "w1:p9", 40); ok {
		t.Fatal("expected no resize for a pane outside the tree")
	}
}

func TestRatioForWidthClampsToLeaveRoomForWork(t *testing.T) {
	// A 40-column sidebar in a 60-column window must not squeeze the work pane
	// out of existence.
	root := split("right", pane("w1:p1"), pane("w1:p2"))
	rects := map[string]herdr.Rect{
		"w1:p1": rect(0, 40),
		"w1:p2": rect(40, 20),
	}

	got, ok := ratioForWidth(root, rects, "w1:p2", 55)
	if !ok {
		t.Fatal("expected a clamped resize")
	}

	// 60 total, the work pane keeps at least minPaneWidth.
	if want := float64(minPaneWidth) / 60.0; got.Ratio != want {
		t.Errorf("ratio = %v, want %v", got.Ratio, want)
	}
}

func TestRatioForWidthRefusesWindowTooNarrowToShare(t *testing.T) {
	root := split("right", pane("w1:p1"), pane("w1:p2"))
	rects := map[string]herdr.Rect{
		"w1:p1": rect(0, 10),
		"w1:p2": rect(10, 8),
	}

	if _, ok := ratioForWidth(root, rects, "w1:p2", 40); ok {
		t.Fatal("a window this narrow cannot hold two panes")
	}
}
