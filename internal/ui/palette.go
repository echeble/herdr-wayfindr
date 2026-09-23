package ui

import (
	"hash/fnv"

	"github.com/charmbracelet/lipgloss"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
)

// groupPalette is the set of colours group headers are drawn in. They are
// mid-tone 256-colour values chosen to stay legible on both a light and a dark
// background, and to be distinguishable from each other at a glance — which is
// the whole point of colouring a feature.
var groupPalette = []lipgloss.Color{
	lipgloss.Color("39"),  // azure
	lipgloss.Color("42"),  // sea green
	lipgloss.Color("170"), // orchid
	lipgloss.Color("214"), // amber
	lipgloss.Color("210"), // salmon
	lipgloss.Color("44"),  // teal
	lipgloss.Color("141"), // violet
	lipgloss.Color("113"), // moss
	lipgloss.Color("205"), // pink
	lipgloss.Color("109"), // steel
	lipgloss.Color("180"), // tan
	lipgloss.Color("81"),  // sky
}

// idleFrameColour is what an unselected card's frame is drawn in.
//
// 244 is the exact middle of the 256-colour greyscale ramp — rgb(128,128,128) —
// so it recedes equally against a light background and a dark one. It is a
// palette index above 15 for the same reason the pull-request colours are: a
// terminal theme cannot remap it into something with a different meaning.
const idleFrameColour = lipgloss.Color("244")

// Semantic agent status styles.
// Blocked is coral/red (bold for immediate urgency across the pane).
// Working is warm amber/yellow.
// Done is cyan/teal.
// Idle is calm green.
// Closed and unknown are faint.
var (
	statusBlockedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("203"))
	statusWorkingStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	statusDoneStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("79"))
	statusIdleStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("114"))
	statusClosedStyle  = lipgloss.NewStyle().Faint(true)
	statusUnknownStyle = lipgloss.NewStyle().Faint(true)
)

// statusGlyphStyled maps an agent state to a colored one-cell glyph.
func statusGlyphStyled(status string, open bool) string {
	if !open {
		return statusClosedStyle.Render("◌")
	}

	switch status {
	case herdr.StatusBlocked:
		return statusBlockedStyle.Render("◆")
	case herdr.StatusWorking:
		return statusWorkingStyle.Render("●")
	case herdr.StatusDone:
		return statusDoneStyle.Render("✔")
	case herdr.StatusIdle:
		return statusIdleStyle.Render("○")
	default:
		return statusUnknownStyle.Render("·")
	}
}

// statusWordStyle colors the word description (e.g. "1 agent blocked")
// according to the state it describes.
func statusWordStyle(status string) lipgloss.Style {
	switch status {
	case herdr.StatusBlocked:
		return statusBlockedStyle
	case herdr.StatusWorking:
		return statusWorkingStyle
	case herdr.StatusDone:
		return statusDoneStyle
	case herdr.StatusIdle:
		return statusIdleStyle
	default:
		return derivedStyle
	}
}

// The pull-request colours are GitHub's own, so the sidebar reads the way the
// pull request page does: purple for merged, green for open, red for closed.
// They are 256-colour approximations of #8250df, #1a7f37 and #cf222e, given as
// palette indices above 15 so a terminal theme cannot remap them into some
// other meaning.
var (
	prMergedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	prOpenStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("34"))
	prClosedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("160"))
)

// prBadge formats a pull-request state with glyph and label for group headers.
func prBadge(state collect.PRState) string {
	style, ok := prStyleFor(state)
	if !ok {
		return ""
	}

	return style.Render(prMark(state) + " " + state.String())
}

// prStyleFor answers how a pull-request state is drawn, and whether it is drawn
// at all: a branch with no pull request, and one nobody could ask about, are
// both simply absent from the display.
func prStyleFor(state collect.PRState) (lipgloss.Style, bool) {
	switch state {
	case collect.PRMerged:
		return prMergedStyle, true
	case collect.PROpen:
		return prOpenStyle, true
	case collect.PRClosed:
		return prClosedStyle, true
	default:
		return lipgloss.Style{}, false
	}
}

// prMark is the one-cell form used on a worktree row, where there is no room
// for a word. ✔ is the merge, ✕ the close, and ⧗ the work still in progress.
//
// ⧗ is U+29D7 BLACK HOURGLASS, not U+231B ⌛: the emoji hourglass is East Asian
// Wide, so it would take two cells and shift every row carrying it out of line
// with the rest, and it arrives coloured by the font rather than by the palette
// above. U+29D7 is a plain one-cell symbol.
//
// It was ↗ before, which read as a link — a thing to click rather than a state
// to be in.
func prMark(state collect.PRState) string {
	switch state {
	case collect.PRMerged:
		return "✔"
	case collect.PROpen:
		return "⧗"
	case collect.PRClosed:
		return "✕"
	default:
		return ""
	}
}

// groupColor picks a group's colour from a hash of its name.
//
// Hashing rather than assigning colours in list order is deliberate: a feature
// keeps the same colour every time the pane opens, and keeps it when other
// groups appear or disappear around it. A colour you cannot learn is just
// decoration.
func groupColor(name string) lipgloss.Color {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))

	return groupPalette[int(h.Sum32()%uint32(len(groupPalette)))]
}

// groupStyleFor is the style a group header is drawn in. The ungrouped bucket
// is deliberately colourless: it is the absence of a feature, not one more.
func groupStyleFor(name string) lipgloss.Style {
	if name == ungroupedLabel {
		return lipgloss.NewStyle().Bold(true).Faint(true)
	}

	return lipgloss.NewStyle().Bold(true).Foreground(groupColor(name))
}
