// Command herdr-wayfindr is the Wayfindr Herdr plugin.
//
// It runs in four modes:
//
//	(no flags)                 the sidebar pane — a grouped list of every
//	                           worktree Herdr knows about, across workspaces
//	--report-tokens            stamp each workspace's group as a Herdr metadata
//	                           token and exit; used by [[startup]] and [[events]]
//	--tag [name]               tag the focused workspace's worktree into a group
//	--rename <name> [label]    rename a group's header without touching its tag
//	                           or how it was matched; no label clears the rename
//	--list                     print the grouping as plain text and exit
//	--resolve-plugin-config    print the normalised config as JSON for pane.sh
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/echeble/herdr-wayfindr/internal/collect"
	"github.com/echeble/herdr-wayfindr/internal/config"
	"github.com/echeble/herdr-wayfindr/internal/group"
	"github.com/echeble/herdr-wayfindr/internal/herdr"
	"github.com/echeble/herdr-wayfindr/internal/store"
	"github.com/echeble/herdr-wayfindr/internal/tokens"
	"github.com/echeble/herdr-wayfindr/internal/ui"
)

const oneShotTimeout = 15 * time.Second

// listTimeout is longer because --list asks GitHub about every branch, which is
// a round trip each.
const listTimeout = 90 * time.Second

func main() {
	var (
		resolveConfig = flag.Bool("resolve-plugin-config", false, "print the normalised plugin config as JSON and exit")
		reportTokens  = flag.Bool("report-tokens", false, "stamp workspace group tokens and exit")
		tag           = flag.Bool("tag", false, "tag the focused workspace's worktree into a group")
		rename        = flag.Bool("rename", false, "rename a group's header (first arg: its resolved name, rest: the new label; omit the label to clear it)")
		list          = flag.Bool("list", false, "print the grouping as plain text and exit")
		workspace     = flag.String("workspace", "", "with --tag, the workspace to tag instead of the focused one")
	)

	flag.Parse()

	if err := run(opts{
		resolveConfig: *resolveConfig,
		reportTokens:  *reportTokens,
		tag:           *tag,
		rename:        *rename,
		list:          *list,
		workspace:     *workspace,
		args:          flag.Args(),
	}); err != nil {
		fmt.Fprintln(os.Stderr, "herdr-wayfindr: "+err.Error())
		os.Exit(1)
	}
}

type opts struct {
	resolveConfig bool
	reportTokens  bool
	tag           bool
	rename        bool
	list          bool
	workspace     string
	args          []string
}

func run(o opts) error {
	cfg := config.Load()

	// The config mode must answer without touching Herdr: pane.sh calls it
	// before it knows whether a pane can be opened at all.
	if o.resolveConfig {
		return config.WriteResolved(os.Stdout, cfg)
	}

	switch {
	case o.reportTokens:
		return runReportTokens(cfg)
	case o.tag:
		return runTag(cfg, o.workspace, strings.Join(o.args, " "))
	case o.rename:
		return runRename(cfg, o.args)
	case o.list:
		return runList(cfg)
	default:
		return runPane(cfg)
	}
}

// runList prints what the pane would show. It is the way to check the grouping
// from a shell, and the way to see it at all where a TUI is inconvenient.
func runList(cfg config.Config) error {
	client, err := herdr.New()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), listTimeout)
	defer cancel()

	// --list is the shell-side view of the pane, so it pays for the full
	// picture: git status and the pull-request state behind the merged marker.
	world, assignments, tagStore, err := resolveWorld(ctx, client, cfg, true)
	if err != nil {
		return err
	}

	// A rename only changes what a group is printed as, never how these
	// worktrees were sorted into it — that already happened above.
	if tagStore != nil {
		assignments = group.WithLabels(assignments, tagStore.Labels())
	}

	for _, line := range world.Errors {
		fmt.Fprintln(os.Stderr, "herdr-wayfindr: "+line)
	}

	byGroup := map[string][]collect.Worktree{}

	for _, wt := range world.Worktrees {
		name := assignments[wt.Path].Name
		if name == "" {
			if cfg.Grouping.HidePrincipalBranches && wt.IsPrincipalBranch() {
				continue
			}

			name = "Ungrouped"
		}

		byGroup[name] = append(byGroup[name], wt)
	}

	names := make([]string, 0, len(byGroup))
	for name := range byGroup {
		names = append(names, name)
	}

	sort.Slice(names, func(i, j int) bool {
		if (names[i] == "Ungrouped") != (names[j] == "Ungrouped") {
			return names[j] == "Ungrouped"
		}

		return strings.ToLower(names[i]) < strings.ToLower(names[j])
	})

	for _, name := range names {
		members := byGroup[name]

		source := ""
		for _, wt := range members {
			if s := assignments[wt.Path].Source.String(); s != "" {
				source = "  (" + s + ")"

				break
			}
		}

		state := ""
		if pr := groupPRState(members); pr != collect.PRUnknown {
			state = "  " + pr.String()
		}

		fmt.Printf("%s  %d%s%s\n", name, len(members), source, state)

		for _, wt := range members {
			state := "closed"
			if wt.Open() {
				state = wt.WorkspaceID + " " + wt.AgentStatus
			}

			fmt.Printf("    %-28s %-28s %-12s %s\n", wt.RepoName, wt.Branch, state, wt.PR.State)
		}
	}

	return nil
}

// groupPRState is the plain-text half of the pane's feature state.
func groupPRState(members []collect.Worktree) collect.PRState {
	states := make([]collect.PRState, 0, len(members))
	for _, wt := range members {
		states = append(states, wt.PR.State)
	}

	return collect.AggregatePR(states)
}

// runPane draws the sidebar. Nothing is fetched before the program starts:
// Bubble Tea paints the first frame from Init, and Herdr leaves a pane blank
// until its process draws, so a slow or failing first collection must not be
// what the user sees.
func runPane(cfg config.Config) error {
	client, err := herdr.New()
	if err != nil {
		return err
	}

	tagStore, storeErr := store.Open(config.StateDir())

	// A preference file that cannot be read costs the memory of the last view
	// and sort, not the pane: the defaults still open something usable.
	prefs, prefsErr := store.OpenPrefs(config.StateDir())

	collector := collect.New(client).
		WithPR(cfg.PR.Enabled).
		WithPrincipalBranches(cfg.Grouping.PrincipalBranches)

	model := ui.New(client, collector, tagStore, prefs, cfg)

	if storeErr != nil {
		model = model.WithError(storeErr)
	} else if prefsErr != nil {
		model = model.WithError(prefsErr)
	}

	options := []tea.ProgramOption{tea.WithAltScreen()}

	// Cell motion rather than all motion: browsing only cares about clicks and
	// the wheel, and reporting every idle mouse move would be traffic through
	// the pty for nothing. The right-click menu is the one thing hover is
	// worth it for, so the model itself switches to all motion — via
	// tea.EnableMouseAllMotion — for as long as one is open, and back to cell
	// motion the moment it closes.
	if cfg.Pane.Mouse {
		options = append(options, tea.WithMouseCellMotion())
	}

	program := tea.NewProgram(model, options...)

	_, err = program.Run()

	return err
}

// runReportTokens is the one-shot the [[startup]] and [[events]] hooks call.
// Tokens are display-only and do not survive a Herdr restart, so this runs
// again every time the server comes up.
func runReportTokens(cfg config.Config) error {
	if !cfg.Tokens.Enabled {
		return nil
	}

	client, err := herdr.New()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), oneShotTimeout)
	defer cancel()

	world, assignments, tagStore, err := resolveWorld(ctx, client, cfg, false)
	if err != nil {
		return err
	}

	// Herdr's own sidebar only ever sees a flat name, so the rename has to be
	// folded into the token here — the grouping itself stays exactly what the
	// resolver produced above.
	if tagStore != nil {
		assignments = group.WithLabels(assignments, tagStore.Labels())
	}

	reporter := tokens.NewReporter(client, cfg.Tokens.Name)
	if errs := reporter.Report(ctx, world, assignments); len(errs) > 0 {
		return errs[0]
	}

	return nil
}

// runTag tags the worktree of a workspace — the focused one unless workspaceID
// names another. With no name it clears the tag, which lets the worktree fall
// back to a derived group.
func runTag(cfg config.Config, workspaceID, name string) error {
	client, err := herdr.New()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), oneShotTimeout)
	defer cancel()

	world, _, _, err := resolveWorld(ctx, client, cfg, false)
	if err != nil {
		return err
	}

	if workspaceID == "" {
		workspaceID = focusedWorkspaceID(world)
	}

	if workspaceID == "" {
		return errors.New("no focused workspace")
	}

	var targetWt *collect.Worktree

	for i := range world.Worktrees {
		if world.Worktrees[i].WorkspaceID == workspaceID {
			targetWt = &world.Worktrees[i]

			break
		}
	}

	if targetWt == nil {
		return fmt.Errorf("workspace %s is not a git worktree", workspaceID)
	}

	if cfg.Grouping.HidePrincipalBranches && targetWt.IsPrincipalBranch() {
		return fmt.Errorf("cannot tag workspace %s: %s is a principal branch", workspaceID, targetWt.Branch)
	}

	path := targetWt.Path

	tagStore, err := store.Open(config.StateDir())
	if err != nil {
		return err
	}

	if err := tagStore.Set(path, name); err != nil {
		return err
	}

	if strings.TrimSpace(name) == "" {
		fmt.Fprintln(os.Stdout, "cleared the group tag for "+path)
	} else {
		fmt.Fprintln(os.Stdout, "tagged "+path+" as "+name)
	}

	return nil
}

// runRename retitles a group's header without touching what put a worktree in
// it. name is the group's own resolved key — a tag, a ticket, a branch match,
// or a shared directory name, the same text --list prints — not a checkout
// path or a workspace. With no label it clears the rename, which lets the
// header go back to showing name itself.
func runRename(cfg config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("--rename needs a group name — see --list for what to rename")
	}

	name := args[0]
	label := strings.Join(args[1:], " ")

	tagStore, err := store.Open(config.StateDir())
	if err != nil {
		return err
	}

	if err := tagStore.SetLabel(name, label); err != nil {
		return err
	}

	if strings.TrimSpace(label) == "" {
		fmt.Fprintln(os.Stdout, "reset the header for "+name)
	} else {
		fmt.Fprintln(os.Stdout, "renamed "+name+" to "+label)
	}

	return nil
}

// resolveWorld is the shared path of the one-shot modes: collect once, resolve
// groups once.
func resolveWorld(ctx context.Context, client herdr.Client, cfg config.Config, full bool) (collect.World, map[string]group.Assignment, *store.Store, error) {
	// A missing or unreadable tag file costs the explicit tags, not the run.
	tagStore, _ := store.Open(config.StateDir())

	collector := collect.New(client).
		WithPR(full && cfg.PR.Enabled).
		WithPrincipalBranches(cfg.Grouping.PrincipalBranches)

	world, err := collector.Collect(ctx, full)
	if err != nil {
		return collect.World{}, nil, nil, err
	}

	world = collector.CollectPR(ctx, world)

	resolver := group.NewResolver(cfg.Grouping, tagStore.Tags())

	inputs := make([]group.Input, 0, len(world.Worktrees))
	for _, wt := range world.Worktrees {
		inputs = append(inputs, group.Input{
			Path:        wt.Path,
			Branch:      wt.Branch,
			Workspace:   wt.WorkspaceLabel,
			RepoRoot:    wt.RepoRoot,
			IsPrincipal: wt.IsPrincipalBranch(),
		})
	}

	return world, resolver.Resolve(inputs), tagStore, nil
}

// focusedWorkspaceID answers which workspace an action should act on. The
// plugin context comes first: `herdr plugin action invoke` resolves context
// from the focused workspace wherever it is run, and ignores the calling pane's
// own HERDR_* environment.
func focusedWorkspaceID(world collect.World) string {
	if raw := os.Getenv("HERDR_PLUGIN_CONTEXT_JSON"); raw != "" {
		var ctx struct {
			WorkspaceID string `json:"workspace_id"`
		}

		if err := json.Unmarshal([]byte(raw), &ctx); err == nil && ctx.WorkspaceID != "" {
			return ctx.WorkspaceID
		}
	}

	if id := os.Getenv("HERDR_WORKSPACE_ID"); id != "" {
		return id
	}

	return world.FocusedWorkspaceID
}
