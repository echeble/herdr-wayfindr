package ui

import (
	"path/filepath"

	"github.com/charmbracelet/lipgloss"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/group"
)

// The card grid is the list's alternative: one big button per feature, which
// opens onto one button per worktree inside it. Two screens instead of one
// scrolling tree — you pick the feature you are working on, then the checkout
// you want to be in, and a click is the whole interaction.
//
// Cards carry the same row values the list draws, so a feature means the same
// thing in both views and every action written against a row keeps working.

const (
	// minCardWidth is the narrowest a card may be. Below it the frame costs
	// more than the text it leaves room for.
	minCardWidth = 16
	// cardGap is the column between two cards in a row. There is no row gap:
	// vertical space is what a sidebar has least of, and the frames touching is
	// what makes the grid read as a grid.
	cardGap = 1
	// cardFrameSize is what the border takes from a card's width and height.
	cardFrameSize = 2
)

// A card is square when it is about twice as wide as it is tall, because a
// terminal cell is about twice as tall as it is wide. The bounds keep that from
// producing a card with no room for text, or one so tall that two of them fill
// the pane.
const (
	minCardHeight = 4
	maxCardHeight = 9
	cellAspect    = 2
)

// nameOverhead is what a card spends before its name: the two frame columns,
// the inset either side of the text, and the status glyph with its space.
const nameOverhead = cardFrameSize + 2 + 2

// grid is how the cards are packed into the pane: a uniform height, and a row
// at a time of however many cards fit at the width that row needs.
//
// Rows are packed rather than columned because a name that does not fit is a
// card that does not say which repository it is — marketplace-billing-service and
// credit-decision-engine both clip to "credit-deci…". So a row takes as many
// cards as it can while still showing the longest name among them, and the rest
// move down.
type grid struct {
	// height is one card, border included, and is the same for every row: rows
	// of different heights would scroll unevenly and be harder to point at.
	height int
	// rows that fit in the body at once.
	visible int
	lines   []gridLine
}

// gridLine is one row of the grid. Every card in it is the same width — a row
// of ragged cards reads as a mistake — and together they fill the pane.
type gridLine struct {
	start int
	count int
	width int
}

// layoutGrid packs cards into the pane.
//
// Each card asks for the width its name needs, bounded by the pane; a row then
// takes as many as fit at the widest ask among them and divides the pane
// between them. Short names still give the two-across grid a narrow sidebar had
// before, and a long one now gets a row to itself rather than an ellipsis.
func layoutGrid(cards []row, width, height int) grid {
	g := grid{height: cardHeightFor(width, height)}
	g.visible = max(1, height/g.height)

	for i := 0; i < len(cards); {
		line := packLine(cards, i, width)
		g.lines = append(g.lines, line)
		i += line.count
	}

	return g
}

// packLine answers how many of the cards from start fit on one row, and how
// wide each of them is drawn.
func packLine(cards []row, start, width int) gridLine {
	count, widest := 0, desiredWidth(cards[start], width)

	for i := start; i < len(cards); i++ {
		want := max(widest, desiredWidth(cards[i], width))

		// Every card on a row is drawn at the widest ask, so adding one costs
		// the row that width for each card already on it as well.
		if count+1 > fitCount(want, width) {
			break
		}

		count, widest = count+1, want
	}

	// A pane too narrow for even one card still gets one: better a card wider
	// than the pane can show than a row with nothing on it.
	count = max(1, count)

	// The width comes from how many cards of this size the pane *would* hold,
	// not from how many this row ended up with. A last row of one would
	// otherwise stretch across the whole pane and look nothing like the full
	// rows above it — whereas a row of one long name genuinely is full.
	per := fitCount(widest, width)

	return gridLine{
		start: start,
		count: count,
		width: max(1, (width-cardGap*(per-1))/per),
	}
}

// fitCount is how many cards of a given width fit across the pane.
func fitCount(cardWidth, paneWidth int) int {
	return max(1, (paneWidth+cardGap)/(cardWidth+cardGap))
}

// desiredWidth is the width a card would like: enough for its name, never less
// than minCardWidth — which is also what keeps "3 worktrees" and "✔ merged"
// intact — and never more than the pane.
func desiredWidth(r row, paneWidth int) int {
	want := lipgloss.Width(cardName(r)) + nameOverhead

	return max(minCardWidth, min(want, max(1, paneWidth)))
}

// cardName is the text a card is sized to show: the feature, or the repository
// the worktree belongs to, or failing that the folder it lives in.
func cardName(r row) string {
	if r.kind == rowGroup {
		return r.displayName()
	}

	if r.worktree == nil {
		return r.name
	}

	if r.worktree.RepoName != "" {
		return r.worktree.RepoName
	}

	return filepath.Base(r.worktree.Path)
}

// cardHeightFor is one card's height, from the width a card would have if every
// card were the same size. Tying it to each row's own width instead would make
// a row of one tall and a row of three short, for no gain: past about twenty
// columns the square shape is capped anyway.
func cardHeightFor(width, height int) int {
	cols := max(1, (width+cardGap)/(minCardWidth+cardGap))

	// Never round a card up to minCardWidth: in a pane too narrow for even one,
	// a card wider than the pane would wrap and break the whole grid.
	nominal := max(1, (width-cardGap*(cols-1))/cols)

	cardHeight := min(max(nominal/cellAspect, minCardHeight), maxCardHeight)

	// A card taller than the body would never be drawn whole, so in a short
	// pane the card gives up its shape rather than its visibility. Three rows
	// is the floor: two frame lines and something between them.
	if height > 0 {
		cardHeight = max(3, min(cardHeight, height))
	}

	return cardHeight
}

// lineOf answers which row a card is drawn on.
func (g grid) lineOf(index int) int {
	for i, line := range g.lines {
		if index < line.start+line.count {
			return i
		}
	}

	return max(0, len(g.lines)-1)
}

// at answers the card drawn at a row and a position along it.
func (g grid) at(line, column int) (int, bool) {
	if line < 0 || line >= len(g.lines) {
		return 0, false
	}

	l := g.lines[line]
	if column < 0 || column >= l.count {
		return 0, false
	}

	return l.start + column, true
}

// featureCards is the top screen of the grid: one card per feature, in the same
// order the list puts them in.
func featureCards(world collect.World, assignments map[string]group.Assignment, order ordering) []row {
	buckets := buildBuckets(world, assignments, order)

	cards := make([]row, 0, len(buckets))
	for _, b := range buckets {
		// A card is never folded — drilling in is what a feature card does
		// instead.
		cards = append(cards, headerRow(b, false))
	}

	return cards
}

// worktreeCards is the screen a feature opens onto: one card per checkout in
// it. An empty answer means the feature is gone — refreshed away while it was
// open — and the caller falls back to the feature grid.
func worktreeCards(world collect.World, assignments map[string]group.Assignment, feature string, order ordering) []row {
	for _, b := range buildBuckets(world, assignments, order) {
		if b.name != feature {
			continue
		}

		cards := make([]row, 0, len(b.members))
		for _, wt := range b.members {
			cards = append(cards, row{kind: rowWorktree, name: b.name, worktree: wt})
		}

		return cards
	}

	return nil
}

// indexOfFeature finds a feature's card, so leaving a drill-down lands back on
// the feature you were just in rather than at the top of the grid.
func indexOfFeature(cards []row, name string) int {
	for i, c := range cards {
		if c.kind == rowGroup && c.name == name {
			return i
		}
	}

	return 0
}
