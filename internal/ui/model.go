// Package ui draws the feature sidebar: every worktree Herdr knows about,
// grouped by feature, with Enter to jump to one.
package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
	"github.com/echeble/herdr-wayfindr/internal/store"
	"github.com/echeble/herdr-wayfindr/internal/tokens"
)

// callTime bounds one collection. It is generous because a full refresh runs a
// worktree.list per repository and a git status per checkout.
const callTime = 20 * time.Second

// prPhaseTime bounds the pull-request pass, which is a network round trip per
// branch. Forty branches through eight workers is seconds, not milliseconds,
// and a rate-limited GitHub is slower still.
const prPhaseTime = 90 * time.Second

// maxSizeAttempts bounds the self-resize. Opening on the left is an open plus a
// swap, and the swap hands the pane its neighbour's width, so one attempt is
// not always enough. The cap is what stops a width the split cannot give —
// a window too narrow for it, say — from being chased forever.
const maxSizeAttempts = 4

// mode is what the pane is doing, as opposed to how it draws: browsing, or
// typing a tag into the footer.
type mode int

const (
	modeBrowse mode = iota
	modeTag
	modeRename
	modeMenu
	modeConfirmDelete
)

// view is how the pane draws: a grouped list, or a grid of cards. Both are
// built from the same world and act on the same rows; only the geometry and
// what a click means differ.
type view int

const (
	viewList view = iota
	viewCards
)

type worldMsg struct {
	world collect.World
	err   error
}

// prMsg is the second phase of a refresh: the same world with pull-request
// state filled in. It carries the generation it was started from, so an answer
// that arrives after the user has pressed r again is dropped rather than
// painting stale rows over fresh ones.
type prMsg struct {
	world collect.World
	gen   int
}

type actionMsg struct {
	note string
	err  error
}

type deleteResultMsg struct {
	group   string
	deleted int
	errors  []string
}

type resizedMsg struct{ err error }

type rightClickRoutedMsg struct{ err error }

// Model is the Bubble Tea model for the sidebar.
type Model struct {
	collector *collect.Collector
	client    herdr.Client
	store     *store.Store
	prefs     *store.Prefs
	reporter  *tokens.Reporter
	cfg       config.Config

	world       collect.World
	assignments map[string]group.Assignment
	rows        []row
	collapsed   map[string]bool

	cursor int
	top    int
	width  int
	height int

	// The card grid keeps its own cursor so switching views back and forth
	// leaves each one where you left it. feature is the drill-down: empty for
	// the grid of features, otherwise the feature whose worktrees are on screen.
	view       view
	feature    string
	cards      []row
	cardCursor int
	cardTop    int

	// cardArmed says the selected card was put there by a click, so the next
	// click on it is the press rather than another selection. Any other way the
	// cursor moves — a key, a resize, a rebuild — clears it in clampCard, so a
	// card the cursor merely happens to sit on still takes two clicks.
	cardArmed bool

	// order is the sort both views are built in. Like view, it is remembered
	// between panes rather than reset every time one opens.
	order ordering

	mode         mode
	input        string
	tagTarget    string
	renameTarget string

	deleteTargetGroup string
	deleteTargets     []*collect.Worktree

	// menuCursor is the highlighted item in the right-click menu. It always
	// starts at 0: the menu is built fresh from whatever was clicked, and there
	// is no previous menu for a remembered position to belong to.
	menuCursor int

	// menuX and menuY are where the menu opened: the click's own screen
	// position, not the row or card it landed on. It stays put at that corner
	// for the life of the menu — hovering to a different item moves the
	// highlight, not the box itself.
	menuX, menuY int

	loaded  bool
	busy    bool
	note    string
	errNote string

	// gen counts refreshes, so a slow pull-request pass can tell whether the
	// world it was built from is still the one on screen.
	gen int

	// selfWorkspace and selfPane identify this pane. The row for the workspace
	// the pane sits in is marked, and the pane sizes itself by its own id.
	selfWorkspace string
	selfPane      string

	// sizeAttempts counts self-resizes, and sizedAt records the width the last
	// one started from. Together they stop a resize and the window event it
	// causes from chasing each other.
	sizeAttempts int
	sizedAt      int
}

// New builds the model. It performs no I/O: the first frame must paint before
// anything is fetched, because Herdr shows a blank pane until a process draws.
func New(client herdr.Client, collector *collect.Collector, tagStore *store.Store, prefs *store.Prefs, cfg config.Config) Model {
	m := Model{
		collector: collector,
		client:    client,
		store:     tagStore,
		prefs:     prefs,
		cfg:       cfg,
		// Ungrouped starts folded. It is the biggest bucket and the least
		// interesting one — every checkout no rule claimed — so it would
		// otherwise push the real groups off the top of the pane. Space opens it.
		collapsed:     map[string]bool{ungroupedLabel: true},
		assignments:   map[string]group.Assignment{},
		selfWorkspace: os.Getenv("HERDR_WORKSPACE_ID"),
		selfPane:      os.Getenv("HERDR_PANE_ID"),
		sizedAt:       -1,
	}

	// config.toml seeds a pane that has never been told otherwise; from the
	// first v or s, what the user last chose is what the pane opens as.
	view := cfg.Pane.View

	if prefs != nil {
		if remembered := prefs.View(); remembered != "" {
			view = remembered
		}

		m.order = parseOrdering(prefs.Sort())
	}

	if view == config.ViewCards {
		m.view = viewCards
	}

	if cfg.Tokens.Enabled {
		m.reporter = tokens.NewReporter(client, cfg.Tokens.Name)
	}

	if len(cfg.Warnings) > 0 {
		m.errNote = cfg.Warnings[0]
	}

	return m
}

// WithError seeds the footer with a startup problem — a tag file that could not
// be read, say — without stopping the pane from drawing.
func (m Model) WithError(err error) Model {
	if err != nil {
		m.errNote = err.Error()
	}

	return m
}

// Init loads once. There is no timer: a background poll meant a worktree.list
// per repository and a git status per checkout every few seconds, which is
// enough work to be felt in the panes around it. The list refreshes on r, and
// after anything this pane does itself.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.refresh(true), m.claimRightClick())
}

// claimRightClick asks Herdr to forward this pane's right clicks to it
// instead of opening Herdr's own pane menu (rename, split, zoom, …). Without
// it pane.right_click's own menu never gets a click to answer: Herdr's chrome
// takes every right click first, and only forwards them to a pane that has
// asked for it.
func (m *Model) claimRightClick() tea.Cmd {
	if m.selfPane == "" || !m.cfg.Pane.RightClick {
		return nil
	}

	client := m.client
	paneID := m.selfPane

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		return rightClickRoutedMsg{err: herdr.SetPaneRightClick(ctx, client, paneID, true)}
	}
}

// refresh collects the world off the update loop. Every Herdr and git call
// happens here, never in View.
func (m Model) refresh(withGit bool) tea.Cmd {
	collector := m.collector
	if collector == nil {
		return nil
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		world, err := collector.Collect(ctx, withGit)

		return worldMsg{world: world, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampCursor()

		// A resize changes how many cards fit across, so the scroll position has
		// to be worked out again from the new geometry.
		m.clampCard()

		return m, m.selfSize()

	case resizedMsg:
		// A failed resize is not worth a word to the user: the pane is simply
		// whatever width the split gave it.
		return m, nil

	case rightClickRoutedMsg:
		// A failed claim is not worth a word either: a right click simply keeps
		// opening Herdr's own pane menu, which is a working fallback, if this
		// asked and did not get an answer.
		return m, nil

	case worldMsg:
		return m.applyWorld(msg)

	case prMsg:
		// A world from before the last refresh is not worth painting.
		if msg.gen != m.gen {
			return m, nil
		}

		m.world = msg.world
		m.note = ""

		if len(msg.world.Errors) > 0 {
			m.errNote = msg.world.Errors[len(msg.world.Errors)-1]
		}

		m.regroup()

		return m, nil

	case actionMsg:
		m.busy = false

		if msg.err != nil {
			m.errNote = msg.err.Error()

			return m, nil
		}

		m.note = msg.note
		m.errNote = ""

		return m, nil

	case deleteResultMsg:
		m.busy = false

		if len(msg.errors) > 0 {
			m.errNote = fmt.Sprintf("deleted %d of %d: %s", msg.deleted, msg.deleted+len(msg.errors), strings.Join(msg.errors, "; "))
		} else {
			m.note = fmt.Sprintf("deleted %s in %s", plural(msg.deleted, "worktree"), msg.group)
			m.errNote = ""
		}

		return m, m.refresh(true)

	case tea.MouseMsg:
		return m.updateMouse(msg)

	case tea.KeyMsg:
		if m.mode == modeTag {
			return m.updateTagPrompt(msg)
		}

		if m.mode == modeRename {
			return m.updateRenamePrompt(msg)
		}

		if m.mode == modeMenu {
			return m.updateMenu(msg)
		}

		if m.mode == modeConfirmDelete {
			return m.updateConfirmDelete(msg)
		}

		if m.view == viewCards {
			return m.updateCards(msg)
		}

		return m.updateList(msg)
	}

	return m, nil
}

// selfSize asks Herdr to move the divider so the pane is the configured width.
// Only a split needs this: an overlay or a tab owns the whole area, and Herdr's
// plugin pane API takes a width for neither.
func (m *Model) selfSize() tea.Cmd {
	if m.selfPane == "" || m.cfg.Pane.Width <= 0 || m.width <= 0 {
		return nil
	}

	if m.width == m.cfg.Pane.Width || m.sizeAttempts >= maxSizeAttempts {
		return nil
	}

	// A window event carrying the width the last attempt started from is that
	// attempt having achieved nothing; trying again from here would loop.
	if m.width == m.sizedAt {
		return nil
	}

	// Record the attempt before making it: the resize provokes another window
	// event, and this is what keeps that event from starting a third.
	m.sizedAt = m.width
	m.sizeAttempts++

	client := m.client
	paneID := m.selfPane
	target := m.cfg.Pane.Width

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		layout, err := herdr.ExportLayout(ctx, client, paneID)
		if err != nil {
			return resizedMsg{err: err}
		}

		geometry, err := herdr.PaneGeometry(ctx, client, paneID)
		if err != nil {
			return resizedMsg{err: err}
		}

		resize, ok := ratioForWidth(layout.Root, geometry.Rects(), paneID, target)
		if !ok {
			return resizedMsg{}
		}

		return resizedMsg{err: herdr.SetSplitRatio(ctx, client, paneID, resize.Path, resize.Ratio)}
	}
}

func (m Model) applyWorld(msg worldMsg) (tea.Model, tea.Cmd) {
	m.loaded = true
	m.busy = false

	if msg.err != nil {
		// Keep the last good picture on screen; a failed refresh is not worth
		// blanking the list for.
		m.errNote = msg.err.Error()

		return m, nil
	}

	m.world = msg.world
	m.errNote = ""
	m.note = ""

	if len(msg.world.Errors) > 0 {
		m.errNote = msg.world.Errors[0]
	}

	// Every applied world is a new generation, so a pull-request pass still in
	// flight from the previous one knows to drop its answer.
	m.gen++

	m.regroup()

	var cmds []tea.Cmd

	if m.reporter != nil {
		cmds = append(cmds, m.reportTokens())
	}

	if m.cfg.PR.Enabled && len(m.world.Worktrees) > 0 {
		m.note = "checking pull requests…"
		cmds = append(cmds, m.collectPR(m.gen))
	}

	return m, tea.Batch(cmds...)
}

// collectPR runs the pull-request pass over the world already on screen. It is
// a second command rather than part of the collection because it takes seconds:
// the list is drawn first, and the merged markers appear when they appear.
func (m Model) collectPR(gen int) tea.Cmd {
	collector := m.collector
	world := m.world

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), prPhaseTime)
		defer cancel()

		return prMsg{world: collector.CollectPR(ctx, world), gen: gen}
	}
}

// tags answers the explicit group tags, tolerating a model built without a
// store. Nothing in the pane is worth a crash for want of a tag file.
func (m Model) tags() map[string]string {
	if m.store == nil {
		return nil
	}

	return m.store.Tags()
}

// setTag writes one, and says so rather than crashing when there is nowhere to
// write it.
func (m Model) setTag(path, name string) error {
	if m.store == nil {
		return errors.New("no state directory: the tag was not saved")
	}

	return m.store.Set(path, name)
}

// labels answers the group label overrides, tolerating a model built without a
// store.
func (m Model) labels() map[string]string {
	if m.store == nil {
		return nil
	}

	return m.store.Labels()
}

// setLabel writes one, and says so rather than crashing when there is nowhere
// to write it.
func (m Model) setLabel(name, label string) error {
	if m.store == nil {
		return errors.New("no state directory: the rename was not saved")
	}

	return m.store.SetLabel(name, label)
}

// labelFor is what a group's own resolved name is shown as: the rename
// override when the user gave it one, the name itself otherwise. It exists
// for the places that only hold the name as a bare string — the breadcrumb
// title, an action's own note — rather than a row that already carries its
// label; row.displayName is the equivalent for a row in hand.
func (m Model) labelFor(name string) string {
	if label := strings.TrimSpace(m.labels()[name]); label != "" {
		return label
	}

	return name
}

// regroup re-runs the resolver over the current world and rebuilds the rows.
func (m *Model) regroup() {
	resolver := group.NewResolver(m.cfg.Grouping, m.tags())

	inputs := make([]group.Input, 0, len(m.world.Worktrees))
	for _, wt := range m.world.Worktrees {
		inputs = append(inputs, group.Input{
			Path:        wt.Path,
			Branch:      wt.Branch,
			Workspace:   wt.WorkspaceLabel,
			RepoRoot:    wt.RepoRoot,
			IsPrincipal: wt.IsPrincipalBranch(),
		})
	}

	m.assignments = resolver.Resolve(inputs)
	m.rebuild()
}

// rebuild redraws both views from the assignments already in hand. Sorting and
// folding change the shape of the list, not which feature anything belongs to,
// so neither needs the resolver run again; a rename does not even need this
// much — see startRename — but rebuild is also what a refresh, a sort, and a
// fold fall back on, so it always stamps the current labels on regardless.
func (m *Model) rebuild() {
	m.rows = applyLabels(buildRows(m.world, m.assignments, m.collapsed, m.order, m.cfg.Grouping.HidePrincipalBranches), m.labels())
	m.clampCursor()
	m.rebuildCards()
}

// rebuildCards refreshes the card screen from the world in hand. A feature that
// was open and is no longer there — its last worktree removed, or renamed into
// another group by a refresh — drops the pane back to the feature grid rather
// than leaving it on a screen with nothing in it.
func (m *Model) rebuildCards() {
	if m.feature != "" {
		if cards := worktreeCards(m.world, m.assignments, m.feature, m.order, m.cfg.Grouping.HidePrincipalBranches); len(cards) > 0 {
			m.cards = cards
			m.clampCard()

			return
		}

		m.feature = ""
	}

	m.cards = applyLabels(featureCards(m.world, m.assignments, m.order, m.cfg.Grouping.HidePrincipalBranches), m.labels())
	m.clampCard()
}

func (m Model) reportTokens() tea.Cmd {
	reporter := m.reporter
	world := m.world
	// Herdr's own sidebar only ever sees a flat name, so the rename has to be
	// folded into the token here — the grouping itself stays exactly what the
	// resolver produced.
	assignments := group.WithLabels(m.assignments, m.labels())

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		// Token failures are not worth interrupting the user over: the pane
		// still shows the grouping, only Herdr's own sidebar misses it.
		_ = reporter.Report(ctx, world, assignments)

		return nil
	}
}

// updateShared handles the keys that mean the same thing whichever view is up,
// so neither view can quietly lose one.
func (m Model) updateShared(msg tea.KeyMsg) (Model, tea.Cmd, bool) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit, true

	case "v":
		return m.toggleView(), nil, true

	case "s":
		return m.cycleSort(), nil, true

	case "t":
		return m.startTag(), nil, true

	case "R":
		return m.startRename(), nil, true

	case "p":
		next, cmd := m.pin()

		return next, cmd, true

	case "r":
		m.busy = true
		m.note = "refreshing…"

		return m, m.refresh(true), true
	}

	return m, nil, false
}

func (m Model) updateList(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.updateShared(msg); handled {
		return next, cmd
	}

	switch msg.String() {
	case "esc":
		return m, tea.Quit

	case "j", "down":
		m.move(1)

		return m, nil

	case "k", "up":
		m.move(-1)

		return m, nil

	case "g", "home":
		m.cursor = 0
		m.clampCursor()

		return m, nil

	case "G", "end":
		m.cursor = len(m.rows) - 1
		m.clampCursor()

		return m, nil

	case " ", "tab":
		return m.toggleCollapse(), nil

	case "enter":
		m.busy = true

		return m, m.jump()
	}

	return m, nil
}

// updateCards is the grid's keyboard: the arrows walk it in two dimensions,
// enter presses the card under the cursor, and esc comes back up a level before
// it closes the pane.
func (m Model) updateCards(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if next, cmd, handled := m.updateShared(msg); handled {
		return next, cmd
	}

	switch msg.String() {
	case "esc", "backspace":
		if m.feature != "" {
			return m.leaveFeature(), nil
		}

		return m, tea.Quit

	case "h", "left":
		m.moveCard(-1)

	case "l", "right":
		m.moveCard(1)

	case "k", "up":
		m.moveCardLine(-1)

	case "j", "down":
		m.moveCardLine(1)

	case "g", "home":
		m.cardCursor = 0
		m.clampCard()

	case "G", "end":
		m.cardCursor = len(m.cards) - 1
		m.clampCard()

	case "enter", " ":
		return m.activateCard()
	}

	return m, nil
}

// toggleView swaps the presentation, and remembers it: the next pane opens the
// way you left this one.
func (m Model) toggleView() Model {
	if m.view == viewCards {
		m.view = viewList
		m.clampCursor()
	} else {
		m.view = viewCards
		m.rebuildCards()
	}

	m.remember(func() error { return m.prefs.SetView(m.viewName()) })

	return m
}

// cycleSort moves to the next order and rebuilds both views in it. The mode is
// named in the footer rather than announced, so the pane says what it is
// sorted by for as long as it is sorted that way.
func (m Model) cycleSort() Model {
	m.order = m.order.next()
	m.rebuild()

	// The cursor is on whatever row happens to be in that position now, which
	// is rarely what it was on. Going back to the top is the honest answer: the
	// point of re-sorting is that the first entry is the interesting one.
	m.cursor, m.top = 0, 0
	m.cardCursor, m.cardTop = 0, 0
	m.clampCursor()
	m.clampCard()

	m.remember(func() error { return m.prefs.SetSort(m.order.String()) })

	return m
}

func (m Model) viewName() string {
	if m.view == viewCards {
		return config.ViewCards
	}

	return config.ViewList
}

// remember persists a choice, reporting a failure in the footer rather than
// swallowing it — a preference that silently does not stick is worse than one
// that says why.
func (m *Model) remember(save func() error) {
	if m.prefs == nil {
		return
	}

	if err := save(); err != nil {
		m.errNote = err.Error()
	}
}

// activateCard is what pressing a card does: a feature opens onto its
// worktrees, a worktree opens the checkout itself.
func (m Model) activateCard() (tea.Model, tea.Cmd) {
	cur, ok := m.currentCard()
	if !ok {
		return m, nil
	}

	if cur.kind == rowGroup {
		return m.enterFeature(cur.name), nil
	}

	m.busy = true

	return m, m.jump()
}

func (m Model) enterFeature(name string) Model {
	m.feature = name
	m.cardCursor, m.cardTop = 0, 0
	m.rebuildCards()

	return m
}

// leaveFeature goes back to the grid of features, landing on the one just left
// rather than at the top — coming back out of a feature should put you where
// you went in.
func (m Model) leaveFeature() Model {
	name := m.feature

	m.feature = ""
	m.cardTop = 0
	m.rebuildCards()
	m.cardCursor = indexOfFeature(m.cards, name)
	m.clampCard()

	return m
}

// updateMouse turns a click or a wheel notch into the same movements the keys
// make. It is deliberately not a jump: selecting is what a click does
// everywhere else, and enter is one keystroke away.
func (m Model) updateMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeMenu {
		switch msg.Action {
		case tea.MouseActionMotion:
			// Hovering the pointer is the mouse's own way of moving the
			// highlight, the same as an arrow key or the wheel.
			return m.hoverMenu(msg.X, msg.Y), nil

		case tea.MouseActionRelease:
			// The right click that opened the menu still has its own release
			// to come; that release is not a second click and must not act
			// as one.
			return m, nil
		}

		switch msg.Button {
		case tea.MouseButtonLeft:
			return m.clickMenu(msg.X, msg.Y)

		case tea.MouseButtonWheelUp:
			m.menuCursor = max(0, m.menuCursor-1)

		case tea.MouseButtonWheelDown:
			m.menuCursor = min(len(m.menuItems())-1, m.menuCursor+1)

		case tea.MouseButtonRight:
			// A right click while the menu is up retargets it to whatever is
			// under the pointer now, the same as opening it fresh — not a
			// click away that merely closes the wrong one.
			if next, ok := m.openMenuAt(msg.X, msg.Y); ok {
				return next, tea.EnableMouseAllMotion
			}

			return m.closeMenu()

		default:
			// Anything else closes the menu without acting, the same way
			// clicking away from a menu does everywhere else.
			return m.closeMenu()
		}

		return m, nil
	}

	if m.mode == modeConfirmDelete && msg.Action == tea.MouseActionPress {
		m.mode = modeBrowse
		m.deleteTargets = nil
		m.deleteTargetGroup = ""

		return m, nil
	}

	// The tag prompt owns the pane while it is up; a stray click must not move
	// the row it is about to write to.
	if m.mode != modeBrowse || msg.Action != tea.MouseActionPress {
		return m, nil
	}

	if m.view == viewCards {
		return m.updateCardMouse(msg)
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.move(-wheelRows)

	case tea.MouseButtonWheelDown:
		m.move(wheelRows)

	case tea.MouseButtonLeft:
		index, ok := m.rowAt(msg.Y)
		if !ok {
			return m, nil
		}

		m.cursor = index
		m.clampCursor()

		// The fold marker is its own target: clicking the ▾ opens or closes a
		// group, clicking its name only selects it.
		if msg.X <= markerColumn && m.rows[index].kind == rowGroup {
			return m.toggleCollapse(), nil
		}

	case tea.MouseButtonRight:
		if !m.cfg.Pane.RightClick {
			return m, nil
		}

		if next, ok := m.openMenuAt(msg.X, msg.Y); ok {
			return next, tea.EnableMouseAllMotion
		}
	}

	return m, nil
}

// updateCardMouse aims before it fires: the first click on a card selects it,
// and only a second click on that same, already selected card opens it. A card
// is a button, but it is a small one in a grid that reflows, and opening a
// worktree on a mis-aimed click is not something the pane can take back.
func (m Model) updateCardMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.moveCardLine(-1)

	case tea.MouseButtonWheelDown:
		m.moveCardLine(1)

	case tea.MouseButtonLeft:
		// The back button is the way back for a hand that never leaves the
		// mouse — a click anywhere on its fill, not just the "‹" itself.
		if m.feature != "" && msg.Y < m.titleRows() && msg.X <= breadcrumbColumn {
			return m.leaveFeature(), nil
		}

		index, ok := m.cardAt(msg.X, msg.Y)
		if !ok {
			return m, nil
		}

		armed := m.cardArmed && index == m.cardCursor

		m.cardCursor = index
		m.clampCard()

		if !armed {
			m.cardArmed = true

			return m, nil
		}

		return m.activateCard()

	case tea.MouseButtonRight:
		if !m.cfg.Pane.RightClick {
			return m, nil
		}

		if next, ok := m.openMenuAt(msg.X, msg.Y); ok {
			return next, tea.EnableMouseAllMotion
		}
	}

	return m, nil
}

// openMenuAt finds the row or card at (x, y), whichever view is up, and opens
// the menu on it. It answers ok=false on a miss — no row or card under the
// pointer — rather than opening on whatever the cursor happened to be on
// already, so a caller can decide what a miss means for it.
func (m Model) openMenuAt(x, y int) (Model, bool) {
	if m.view == viewCards {
		index, ok := m.cardAt(x, y)
		if !ok {
			return m, false
		}

		m.cardCursor = index
		m.clampCard()
	} else {
		index, ok := m.rowAt(y)
		if !ok {
			return m, false
		}

		m.cursor = index
		m.clampCursor()
	}

	m.mode = modeMenu
	m.menuCursor = 0
	m.menuX, m.menuY = x, y

	return m, true
}

// menuAction is what a right-click menu item does, resolved through
// runMenuItem rather than a closure stored on the item — the actions
// themselves already exist as ordinary Model methods, so this only has to
// name which one.
type menuAction int

const (
	menuOpen menuAction = iota
	menuTag
	menuPin
	menuFold
	menuRename
	menuDeleteAll
)

// menuItem is one line of the right-click menu.
type menuItem struct {
	label  string
	action menuAction
}

// menuItems is the right-click menu for whatever is currently selected: tag
// and pin only make sense for a worktree, and fold only for a group header in
// the list, where there is a marker to fold. It is computed fresh each time
// rather than stored, because nothing that would change it can happen while
// the menu owns the pane.
func (m Model) menuItems() []menuItem {
	cur, ok := m.current()
	if !ok {
		return nil
	}

	items := []menuItem{{label: "Open", action: menuOpen}}

	switch cur.kind {
	case rowWorktree:
		if cur.worktree != nil && (!m.cfg.Grouping.HidePrincipalBranches || !cur.worktree.IsPrincipalBranch()) {
			items = append(items,
				menuItem{label: "Tag", action: menuTag},
				menuItem{label: "Pin", action: menuPin},
			)
		}

	case rowGroup:
		items = append(items, menuItem{label: "Rename", action: menuRename})

		if m.view != viewCards {
			label := "Fold"
			if cur.collapsed {
				label = "Unfold"
			}

			items = append(items, menuItem{label: label, action: menuFold})
		}

		if cur.name != ungroupedLabel && cur.members > 0 {
			items = append(items, menuItem{label: "Delete All", action: menuDeleteAll})
		}
	}

	return items
}

// updateMenu is the right-click menu's keyboard: up/down move the highlight,
// enter runs the highlighted item, and esc backs out without acting.
func (m Model) updateMenu(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.menuItems()

	switch msg.String() {
	case "esc", "ctrl+c":
		return m.closeMenu()

	case "k", "up":
		m.menuCursor = max(0, m.menuCursor-1)

		return m, nil

	case "j", "down":
		m.menuCursor = min(len(items)-1, m.menuCursor+1)

		return m, nil

	case "enter", " ":
		if m.menuCursor < 0 || m.menuCursor >= len(items) {
			return m.closeMenu()
		}

		return m.runMenuItem(items[m.menuCursor].action)
	}

	return m, nil
}

// closeMenu leaves the menu and turns hover tracking back off. Reporting
// every idle mouse move is not worth the traffic through the pty outside a
// menu that can actually use it — see runPane's own reasoning for opening in
// cell motion rather than all motion.
func (m Model) closeMenu() (tea.Model, tea.Cmd) {
	m.mode = modeBrowse

	return m, tea.EnableMouseCellMotion
}

// hoverMenu is what moving the mouse does while the menu is up: the item
// under the pointer becomes the highlighted one, the same one enter would
// run. Hovering off the box leaves the highlight where it was — the pointer
// wandering away from the menu is not a click away from it.
func (m Model) hoverMenu(x, y int) Model {
	items := m.menuItems()

	bx, by, bw, bh := m.menuRect(items)
	if x < bx || x >= bx+bw || y < by || y >= by+bh {
		return m
	}

	// Row 0 of the box is its own top border; the items start on row 1.
	if index := y - by - 1; index >= 0 && index < len(items) {
		m.menuCursor = index
	}

	return m
}

// runMenuItem is what enter, or a click on an item, does: the same action the
// item's label promises, through the ordinary Model method that key already
// runs in browse mode.
func (m Model) runMenuItem(action menuAction) (tea.Model, tea.Cmd) {
	m.mode = modeBrowse

	var (
		next tea.Model = m
		cmd  tea.Cmd
	)

	switch action {
	case menuOpen:
		next, cmd = m.openTarget()

	case menuTag:
		next = m.startTag()

	case menuPin:
		next, cmd = m.pin()

	case menuFold:
		next = m.toggleCollapse()

	case menuRename:
		next = m.startRename()

	case menuDeleteAll:
		next = m.startDeleteAll()
	}

	return next, tea.Batch(cmd, tea.EnableMouseCellMotion)
}

// clickMenu is what a left click does while the menu is up: a click on one of
// its items runs it, a click elsewhere in the box — its border, say — does
// nothing, and a click outside the box closes the menu, the same as clicking
// away from a menu does everywhere else.
func (m Model) clickMenu(x, y int) (tea.Model, tea.Cmd) {
	items := m.menuItems()

	bx, by, bw, bh := m.menuRect(items)
	if x < bx || x >= bx+bw || y < by || y >= by+bh {
		return m.closeMenu()
	}

	// Row 0 of the box is its own top border; the items start on row 1.
	index := y - by - 1
	if index < 0 || index >= len(items) {
		return m, nil
	}

	m.menuCursor = index

	return m.runMenuItem(items[index].action)
}

// openTarget is what the menu's Open does: the same jump or drill-down enter
// already performs in each view, so the menu never invents a second way to
// open something.
func (m Model) openTarget() (tea.Model, tea.Cmd) {
	if m.view == viewCards {
		return m.activateCard()
	}

	m.busy = true

	return m, m.jump()
}

// cardAt answers which card covers a screen cell, or that none does — the gap
// between two cards, the title line, the space past the end of a short row and
// the empty space below the last one all have to miss.
func (m Model) cardAt(x, y int) (int, bool) {
	g := m.grid()

	line := y - m.contentTop()
	if line < 0 || line >= g.visible*g.height {
		return 0, false
	}

	index := m.cardTop + line/g.height
	if index >= len(g.lines) {
		return 0, false
	}

	span := g.lines[index].width + cardGap
	if x < 0 || x%span >= g.lines[index].width {
		return 0, false
	}

	return g.at(index, x/span)
}

// grid is the card layout for the pane as it stands. It is derived rather than
// stored so a resize cannot leave the drawing and the hit-testing disagreeing.
func (m Model) grid() grid {
	return layoutGrid(m.cards, m.paneWidth(), m.contentHeight())
}

// moveCard steps along the cards in order, which walks off the end of a row
// onto the start of the next one — rows hold different numbers of cards now, so
// counting along them is the only movement that always lands somewhere.
func (m *Model) moveCard(delta int) {
	if len(m.cards) == 0 {
		return
	}

	m.cardCursor += delta
	m.clampCard()
}

// moveCardLine moves a row at a time, keeping your place along it. Rows are
// packed to different lengths, so the position is clamped to the shorter one
// rather than skipped past.
func (m *Model) moveCardLine(delta int) {
	if len(m.cards) == 0 {
		return
	}

	g := m.grid()

	line := g.lineOf(m.cardCursor)
	column := m.cardCursor - g.lines[line].start

	target := max(0, min(line+delta, len(g.lines)-1))
	if index, ok := g.at(target, min(column, g.lines[target].count-1)); ok {
		m.cardCursor = index
	}

	m.clampCard()
}

// clampCard keeps the grid cursor on a card and that card on screen. The unit
// of scrolling is a card row: a grid that scrolled by the line would cut every
// card in half.
func (m *Model) clampCard() {
	// Every way the grid cursor moves ends here, which makes it the one place
	// that can tell a card is no longer the card the last click landed on.
	m.cardArmed = false

	if len(m.cards) == 0 {
		m.cardCursor, m.cardTop = 0, 0

		return
	}

	m.cardCursor = max(0, min(m.cardCursor, len(m.cards)-1))

	g := m.grid()
	cursorLine := g.lineOf(m.cardCursor)

	if cursorLine < m.cardTop {
		m.cardTop = cursorLine
	}

	if cursorLine >= m.cardTop+g.visible {
		m.cardTop = cursorLine - g.visible + 1
	}

	m.cardTop = max(0, min(m.cardTop, max(0, len(g.lines)-g.visible)))
}

// scrollable answers whether anything is off the top or the bottom of the pane,
// for whichever view is up. Nothing else says so: the list scrolls without a
// bar and the grid scrolls a card row at a time, so a feature below the fold is
// simply invisible.
func (m Model) scrollable() (above, below bool) {
	if !m.overflows() {
		return false, false
	}

	if m.view == viewCards {
		g := m.grid()

		return m.cardTop > 0, m.cardTop+g.visible < len(g.lines)
	}

	return m.top > 0, m.top+m.contentHeight() < len(m.rows)
}

// overflows answers whether there is more than the body can hold, which is what
// decides whether the chevron lines are there at all.
//
// It measures against the *whole* body rather than against contentHeight, and
// that is the point: contentHeight subtracts the chevron lines, so asking it
// here would be asking a question whose answer changes the question. Measuring
// the unreserved body breaks the circle, and safely — content that overflows
// the full body overflows the smaller one too.
func (m Model) overflows() bool {
	if !m.loaded {
		return false
	}

	body := m.listHeight()

	if m.view == viewCards {
		g := layoutGrid(m.cards, m.paneWidth(), body)

		return len(g.lines) > g.visible
	}

	return len(m.rows) > body
}

// contentHeight is the room the list or the grid itself gets: the body, less
// the two lines the chevrons take when there is more than fits. Both are
// reserved together and kept whichever way the pane can scroll, so the content
// does not jump a line every time you reach an end.
func (m Model) contentHeight() int {
	height := m.listHeight()

	if m.overflows() {
		height -= chevronRows
	}

	return max(1, height)
}

// contentTop is the screen line the first row or card row is drawn on, which is
// one lower when the up chevron has a line reserved above it.
func (m Model) contentTop() int {
	if m.overflows() {
		return m.titleRows() + 1
	}

	return m.titleRows()
}

func (m Model) paneWidth() int {
	if m.width <= 0 {
		return defaultWidth
	}

	return m.width
}

func (m Model) currentCard() (row, bool) {
	if m.cardCursor < 0 || m.cardCursor >= len(m.cards) {
		return row{}, false
	}

	return m.cards[m.cardCursor], true
}

// rowAt answers which row is drawn on a screen line. The title, the chevron
// line above the list and the scroll position all sit between a screen line and
// a row index, so all three have to be undone together.
func (m Model) rowAt(y int) (int, bool) {
	line := y - m.contentTop()
	if line < 0 || line >= m.contentHeight() {
		return 0, false
	}

	index := m.top + line
	if index >= len(m.rows) {
		return 0, false
	}

	return index, true
}

func (m *Model) move(delta int) {
	if len(m.rows) == 0 {
		return
	}

	m.cursor += delta
	m.clampCursor()
}

func (m *Model) clampCursor() {
	if len(m.rows) == 0 {
		m.cursor, m.top = 0, 0

		return
	}

	m.cursor = max(0, min(m.cursor, len(m.rows)-1))

	// Keep the cursor inside the viewport without re-centring on every move.
	visible := m.contentHeight()
	if visible <= 0 {
		m.top = 0

		return
	}

	if m.cursor < m.top {
		m.top = m.cursor
	}

	if m.cursor >= m.top+visible {
		m.top = m.cursor - visible + 1
	}

	m.top = max(0, min(m.top, max(0, len(m.rows)-visible)))
}

// current is what the user has picked out, whichever view is up. Everything
// that acts on a selection — jumping, tagging, pinning — goes through it, so
// those actions never had to learn there is more than one way to draw the
// world.
func (m Model) current() (row, bool) {
	if m.view == viewCards {
		return m.currentCard()
	}

	if m.cursor < 0 || m.cursor >= len(m.rows) {
		return row{}, false
	}

	return m.rows[m.cursor], true
}

func (m Model) toggleCollapse() Model {
	cur, ok := m.current()
	if !ok {
		return m
	}

	m.collapsed[cur.name] = !m.collapsed[cur.name]
	m.rebuild()

	return m
}

// jump focuses the selected worktree's workspace, opening the checkout as a
// workspace first when Herdr does not have it open.
func (m Model) jump() tea.Cmd {
	cur, ok := m.current()
	if !ok {
		return nil
	}

	if cur.kind == rowGroup {
		// On a header, jump to the first member that is already open — the
		// cheapest useful reading of "go to this feature".
		for _, r := range m.rows {
			if r.kind == rowWorktree && r.name == cur.name && r.worktree.Open() {
				cur = r

				break
			}
		}

		if cur.kind == rowGroup {
			name := cur.displayName()

			return func() tea.Msg {
				return actionMsg{err: fmt.Errorf("no worktree in %s is open", name)}
			}
		}
	}

	client := m.client
	wt := *cur.worktree

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		if wt.Open() {
			if err := herdr.FocusWorkspace(ctx, client, wt.WorkspaceID); err != nil {
				return actionMsg{err: fmt.Errorf("focus %s: %w", wt.RepoName, err)}
			}

			return actionMsg{note: "focused " + wt.RepoName}
		}

		ws, err := herdr.OpenWorktree(ctx, client, wt.RepoRoot, wt.Path, true)
		if err != nil {
			return actionMsg{err: fmt.Errorf("open %s: %w", wt.RepoName, err)}
		}

		return actionMsg{note: "opened " + nonEmpty(ws.Label, wt.RepoName)}
	}
}

func (m Model) startTag() Model {
	cur, ok := m.current()
	if !ok || cur.kind != rowWorktree {
		m.errNote = "select a worktree to tag"

		return m
	}

	if m.cfg.Grouping.HidePrincipalBranches && cur.worktree != nil && cur.worktree.IsPrincipalBranch() {
		m.errNote = fmt.Sprintf("cannot tag principal branch %s", cur.worktree.Branch)

		return m
	}

	m.mode = modeTag
	m.tagTarget = cur.worktree.Path
	m.input = m.tags()[cur.worktree.Path]
	m.errNote = ""

	return m
}

// startRename opens the footer prompt to retitle the selected group's header.
// It changes nothing about what put a worktree there or what it is tagged as
// — only the text a header and its cards show in place of the resolved name.
func (m Model) startRename() Model {
	cur, ok := m.current()
	if !ok || cur.kind != rowGroup {
		m.errNote = "select a group to rename"

		return m
	}

	m.mode = modeRename
	m.renameTarget = cur.name
	m.input = m.labels()[cur.name]
	m.errNote = ""

	return m
}

// pin turns the currently derived group into an explicit tag, so it stops
// depending on the branch name.
func (m Model) pin() (Model, tea.Cmd) {
	cur, ok := m.current()
	if !ok || cur.kind != rowWorktree {
		m.errNote = "select a worktree to pin"

		return m, nil
	}

	if m.cfg.Grouping.HidePrincipalBranches && cur.worktree != nil && cur.worktree.IsPrincipalBranch() {
		m.errNote = fmt.Sprintf("cannot pin principal branch %s", cur.worktree.Branch)

		return m, nil
	}

	assignment := m.assignments[cur.worktree.Path]
	if assignment.Name == "" {
		m.errNote = "nothing to pin: this worktree has no derived group"

		return m, nil
	}

	if assignment.Source == group.SourceExplicit {
		m.errNote = m.labelFor(assignment.Name) + " is already an explicit tag"

		return m, nil
	}

	if err := m.setTag(cur.worktree.Path, assignment.Name); err != nil {
		m.errNote = err.Error()

		return m, nil
	}

	m.note = "pinned " + m.labelFor(assignment.Name)
	m.errNote = ""

	// Regrouping is local work over state already in hand, so the pane updates
	// without another round of collection.
	m.regroup()

	return m, nil
}

func (m Model) updateTagPrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeBrowse
		m.input = ""
		m.tagTarget = ""

		return m, nil

	case "enter":
		path, name := m.tagTarget, strings.TrimSpace(m.input)

		m.mode = modeBrowse
		m.input = ""
		m.tagTarget = ""

		if err := m.setTag(path, name); err != nil {
			m.errNote = err.Error()

			return m, nil
		}

		if name == "" {
			m.note = "cleared tag"
		} else {
			m.note = "tagged " + name
		}

		m.errNote = ""
		m.regroup()

		return m, nil

	case "backspace":
		if m.input != "" {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
		}

		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.input += string(msg.Runes)
	} else if msg.String() == " " {
		m.input += " "
	}

	return m, nil
}

func (m Model) updateRenamePrompt(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "ctrl+c":
		m.mode = modeBrowse
		m.input = ""
		m.renameTarget = ""

		return m, nil

	case "enter":
		name, label := m.renameTarget, strings.TrimSpace(m.input)

		m.mode = modeBrowse
		m.input = ""
		m.renameTarget = ""

		if err := m.setLabel(name, label); err != nil {
			m.errNote = err.Error()

			return m, nil
		}

		if label == "" {
			m.note = "reset the name"
		} else {
			m.note = "renamed to " + label
		}

		m.errNote = ""

		// The rename only changes what a header is titled, never what put a
		// worktree in it, so rebuilding what is already on screen is enough —
		// there is nothing here for the resolver to redo.
		m.rebuild()

		return m, nil

	case "backspace":
		if m.input != "" {
			runes := []rune(m.input)
			m.input = string(runes[:len(runes)-1])
		}

		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.input += string(msg.Runes)
	} else if msg.String() == " " {
		m.input += " "
	}

	return m, nil
}

// groupMembers answers every worktree that belongs to the given group name.
func (m Model) groupMembers(groupName string) []*collect.Worktree {
	var members []*collect.Worktree
	for i := range m.world.Worktrees {
		wt := &m.world.Worktrees[i]
		name := m.assignments[wt.Path].Name
		if name == "" {
			name = ungroupedLabel
		}
		if name == groupName {
			members = append(members, wt)
		}
	}

	return members
}

// startDeleteAll evaluates a group for deletion. If any member is a principal,
// master, or main branch, or a primary checkout, deletion is aborted immediately.
// Otherwise, it transitions to modeConfirmDelete to request confirmation.
func (m Model) startDeleteAll() Model {
	cur, ok := m.current()
	if !ok || cur.kind != rowGroup {
		m.errNote = "select a group to delete"

		return m
	}

	if cur.name == ungroupedLabel {
		m.errNote = "cannot delete ungrouped worktrees"

		return m
	}

	members := m.groupMembers(cur.name)
	if len(members) == 0 {
		m.errNote = "no worktrees in " + cur.displayName()

		return m
	}

	ctx, cancel := context.WithTimeout(context.Background(), callTime)
	defer cancel()

	// Strict Abort: Check each member. If ANY is protected or principal, abort entirely.
	for _, wt := range members {
		if protected, reason := collect.IsProtectedWorktree(ctx, wt, m.cfg.Grouping.PrincipalBranches); protected {
			m.errNote = fmt.Sprintf("cannot delete %s: contains protected branch %s (%s)", cur.displayName(), wt.Branch, reason)

			return m
		}
	}

	m.mode = modeConfirmDelete
	m.deleteTargetGroup = cur.name
	m.deleteTargets = members
	m.errNote = ""

	return m
}

func (m Model) updateConfirmDelete(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y", "enter":
		targets := m.deleteTargets
		groupName := m.deleteTargetGroup

		m.mode = modeBrowse
		m.deleteTargets = nil
		m.deleteTargetGroup = ""
		m.note = fmt.Sprintf("deleting %s in %s...", plural(len(targets), "worktree"), groupName)
		m.errNote = ""

		return m, m.deleteGroupWorktrees(targets, groupName)

	case "n", "N", "esc", "ctrl+c":
		m.mode = modeBrowse
		m.deleteTargets = nil
		m.deleteTargetGroup = ""
		m.errNote = ""

		return m, nil
	}

	return m, nil
}

func (m Model) deleteGroupWorktrees(targets []*collect.Worktree, groupName string) tea.Cmd {
	client := m.client
	principalBranches := m.cfg.Grouping.PrincipalBranches
	store := m.store

	wts := make([]collect.Worktree, 0, len(targets))
	for _, wt := range targets {
		if wt != nil {
			wts = append(wts, *wt)
		}
	}

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), callTime)
		defer cancel()

		var (
			deleted int
			errs    []string
		)

		for _, wt := range wts {
			// Defense in depth: Verify protection once again before deleting
			if protected, reason := collect.IsProtectedWorktree(ctx, &wt, principalBranches); protected {
				errs = append(errs, fmt.Sprintf("%s: skipped protected branch (%s)", wt.RepoName, reason))

				continue
			}

			// If open in Herdr and client is available, request removal via Herdr socket
			if wt.Open() && client != nil {
				if err := herdr.RemoveWorktree(ctx, client, wt.WorkspaceID, true); err != nil {
					errs = append(errs, fmt.Sprintf("%s: %v", wt.RepoName, err))

					continue
				}
				deleted++

				if store != nil {
					_ = store.Set(wt.Path, "")
				}

				continue
			}

			// Not open in Herdr, or client unavailable: remove via git worktree remove
			if err := collect.RemoveGitWorktree(ctx, wt.RepoRoot, wt.Path, true); err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", wt.RepoName, err))

				continue
			}
			deleted++

			if store != nil {
				_ = store.Set(wt.Path, "")
			}
		}

		return deleteResultMsg{
			group:   groupName,
			deleted: deleted,
			errors:  errs,
		}
	}
}

func nonEmpty(first, second string) string {
	if first != "" {
		return first
	}

	return second
}
