// Package config loads the plugin's own config.toml and normalises it, so the
// Go binary and herdr/pane.sh agree on one set of values. pane.sh reads the
// same settings through the binary's --resolve-plugin-config flag rather than
// parsing TOML in bash.
package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/BurntSushi/toml"
)

// PluginID is the manifest id. It namespaces the metadata tokens this plugin
// reports and is the prefix of every action a user binds a key to.
const PluginID = "echeble.wayfindr"

// Defaults. JiraPattern finds a ticket key anywhere in a branch name, which is
// how one feature is already spelled across repositories: WAYF-20748,
// worktree/WAYF-20099 and worktree-WAYF-20226_remove_fields all name their
// ticket, and anchoring to the start would scatter them into separate groups.
// The leading guard keeps the key from starting mid-token.
//
// It is case-insensitive because the case a ticket is typed in is an accident
// of how the worktree was made — wayf-20748 and WAYF-20748 are one feature — and
// the name is upper-cased once matched so the group has a single spelling. The
// trailing guard is what makes that safe: without it a generated name like
// worktree/rapid-forest-349e would read as the ticket FOREST-349, because the
// only thing that used to rule it out was its being lower case.
const (
	DefaultJiraPattern = `(?i)(?:^|[^a-z0-9])([a-z][a-z0-9]+-[0-9]+)(?:[^a-z0-9]|$)`
	DefaultTokenName   = "group"
	DefaultPlacement   = "split"
	DefaultDirection   = "left"
	DefaultWidth       = 40
	DefaultView        = ViewList
)

// The two presentations. ViewList is the grouped list; ViewCards is a grid of
// feature buttons that opens onto a grid of that feature's worktrees. The
// setting picks which one the pane opens as; v switches between them live.
const (
	ViewList  = "list"
	ViewCards = "cards"
)

// widthBounds keep a configured width usable: narrower than this and the rows
// are unreadable, wider and it stops being a sidebar.
const (
	minWidth = 20
	maxWidth = 120
)

// Config is the normalised settings the rest of the plugin reads.
type Config struct {
	Grouping Grouping `json:"grouping" toml:"grouping"`
	Pane     Pane     `json:"pane" toml:"pane"`
	Tokens   Tokens   `json:"tokens" toml:"tokens"`
	PR       PR       `json:"pr" toml:"pr"`

	// Warnings records settings that were rejected and replaced by their
	// default, so a typo degrades the grouping instead of blanking the pane.
	Warnings []string `json:"warnings" toml:"-"`
}

type Grouping struct {
	// JiraPattern extracts a ticket key from a branch name. Its first capture
	// group, or the whole match when it has none, becomes the group name.
	JiraPattern string `json:"jira_pattern" toml:"jira_pattern"`
	// BranchPatterns are tried after JiraPattern. The first one with a
	// non-empty capture named "group" wins.
	BranchPatterns []string `json:"branch_patterns" toml:"branch_patterns"`
	// UseWorktreeName groups checkouts that share a directory name across
	// different repositories.
	UseWorktreeName bool `json:"use_worktree_name" toml:"use_worktree_name"`
	// PrincipalBranches names the primary branches (e.g. main, master) that
	// should not appear in Ungrouped or be tagged.
	PrincipalBranches []string `json:"principal_branches" toml:"principal_branches"`
	// HidePrincipalBranches controls whether principal branches (main, master,
	// or detected default) are omitted from the Ungrouped bucket when unmatched.
	HidePrincipalBranches bool `json:"hide_principal_branches" toml:"hide_principal_branches"`
}

type Pane struct {
	Placement string `json:"placement" toml:"placement"`
	Direction string `json:"direction" toml:"direction"`
	// View is the presentation the pane opens as: "list" or "cards". It is a
	// starting point rather than a lock — v switches for the life of the pane.
	View     string `json:"view" toml:"view"`
	AutoOpen bool   `json:"auto_open" toml:"auto_open"`
	// Mouse asks the terminal for click and wheel events. It is worth a switch
	// because a pane that captures the mouse no longer lets the terminal select
	// text in it the ordinary way.
	Mouse bool `json:"mouse" toml:"mouse"`
	// RightClick opens a small context menu (open, tag, pin, fold) on the row or
	// card under the pointer. It is its own switch, separate from Mouse, because
	// a right click is also how some terminals paste — worth giving back on its
	// own without losing left-click selection and the wheel too.
	RightClick bool `json:"right_click" toml:"right_click"`
	// Singleton keeps one sidebar in the whole session: opening it in a
	// workspace closes the ones left behind in the others. Herdr gives a plugin
	// no session-wide surface, so this is the nearest thing to a sidebar that
	// follows you.
	Singleton bool `json:"singleton" toml:"singleton"`
	// Width is the column count a split sidebar resizes itself to. Herdr's
	// plugin pane API only takes a width for a popup, so a split has to move
	// its own divider. 0 leaves the split wherever Herdr put it.
	Width int `json:"width" toml:"width"`
}

type Tokens struct {
	Enabled bool `json:"enabled" toml:"enabled"`
	// Name is the token key, rendered as $name in [ui.sidebar.spaces] rows.
	Name string `json:"name" toml:"name"`
}

// PR controls the pull-request lookup behind the "merged" marker. It costs one
// `gh pr list` per branch on every full refresh, so it is worth a switch.
type PR struct {
	Enabled bool `json:"enabled" toml:"enabled"`
}

// tokenKeyPattern is Herdr's own constraint on a metadata token key.
var tokenKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,32}$`)

var (
	validPlacements = map[string]bool{"split": true, "tab": true, "overlay": true, "zoomed": true}
	// Which edge of the workspace the sidebar sits against. Herdr's own split
	// only opens right or down; "left" is that plus a swap, which pane.sh does.
	validDirections = map[string]bool{"left": true, "right": true}
	validViews      = map[string]bool{ViewList: true, ViewCards: true}
)

// Default returns the configuration used when no config.toml exists.
func Default() Config {
	return Config{
		Grouping: Grouping{
			JiraPattern:           DefaultJiraPattern,
			BranchPatterns:        nil,
			UseWorktreeName:       true,
			PrincipalBranches:     []string{"main", "master"},
			HidePrincipalBranches: true,
		},
		Pane: Pane{
			Placement:  DefaultPlacement,
			Direction:  DefaultDirection,
			View:       DefaultView,
			AutoOpen:   false,
			Width:      DefaultWidth,
			Mouse:      true,
			RightClick: true,
			Singleton:  true,
		},
		Tokens: Tokens{
			Enabled: true,
			Name:    DefaultTokenName,
		},
		PR: PR{
			Enabled: true,
		},
	}
}

// Dir answers the directory Herdr set aside for this plugin's user config.
// HERDR_PLUGIN_CONFIG_DIR is set for plugin commands and panes; the fallback is
// the documented default so a binary run by hand still finds the same file.
func Dir() string {
	if dir := os.Getenv("HERDR_PLUGIN_CONFIG_DIR"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".config", "herdr", "plugins", "config", PluginID)
}

// StateDir answers where the plugin keeps state it owns — the explicit group
// tags. Herdr never writes here.
func StateDir() string {
	if dir := os.Getenv("HERDR_PLUGIN_STATE_DIR"); dir != "" {
		return dir
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	return filepath.Join(home, ".local", "state", "herdr", "plugins", PluginID)
}

// Load reads config.toml from the plugin config dir. A missing file is not an
// error: it yields the defaults. An unreadable or malformed one yields the
// defaults plus a warning, because a broken config must never stop the pane
// from painting.
func Load() Config {
	dir := Dir()
	if dir == "" {
		return Default()
	}

	return LoadFile(filepath.Join(dir, "config.toml"))
}

// LoadFile reads one config file. Exported for tests.
func LoadFile(path string) Config {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("read %s: %v", path, err))
		}

		return cfg
	}

	var raw Config

	md, err := toml.Decode(string(data), &raw)
	if err != nil {
		cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("parse %s: %v", path, err))

		return cfg
	}

	return merge(cfg, raw, md)
}

// merge folds the file's values over the defaults, dropping any that cannot be
// honoured. Booleans are taken only when the file actually defines them —
// otherwise an absent key would read as false and quietly turn a default-on
// setting off. An empty string likewise means "not set".
func merge(cfg, raw Config, md toml.MetaData) Config {
	if raw.Grouping.JiraPattern != "" {
		if _, err := regexp.Compile(raw.Grouping.JiraPattern); err != nil {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("grouping.jira_pattern is not a valid regexp, using the default: %v", err))
		} else {
			cfg.Grouping.JiraPattern = raw.Grouping.JiraPattern
		}
	}

	for _, pattern := range raw.Grouping.BranchPatterns {
		compiled, err := regexp.Compile(pattern)
		if err != nil {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("grouping.branch_patterns entry %q is not a valid regexp, ignoring it", pattern))

			continue
		}

		// A pattern without the capture can never name a group, so it would
		// silently do nothing. Say so instead.
		if !hasGroupCapture(compiled) {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("grouping.branch_patterns entry %q has no (?P<group>...) capture, ignoring it", pattern))

			continue
		}

		cfg.Grouping.BranchPatterns = append(cfg.Grouping.BranchPatterns, pattern)
	}

	if md.IsDefined("grouping", "use_worktree_name") {
		cfg.Grouping.UseWorktreeName = raw.Grouping.UseWorktreeName
	}

	if md.IsDefined("grouping", "principal_branches") {
		cfg.Grouping.PrincipalBranches = raw.Grouping.PrincipalBranches
	}

	if md.IsDefined("grouping", "hide_principal_branches") {
		cfg.Grouping.HidePrincipalBranches = raw.Grouping.HidePrincipalBranches
	}

	if raw.Pane.Placement != "" {
		if !validPlacements[raw.Pane.Placement] {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("pane.placement %q is not one of split, tab, overlay, zoomed; using %s", raw.Pane.Placement, DefaultPlacement))
		} else {
			cfg.Pane.Placement = raw.Pane.Placement
		}
	}

	if raw.Pane.Direction != "" {
		if !validDirections[raw.Pane.Direction] {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("pane.direction %q is not left or right; using %s", raw.Pane.Direction, DefaultDirection))
		} else {
			cfg.Pane.Direction = raw.Pane.Direction
		}
	}

	if raw.Pane.View != "" {
		if !validViews[raw.Pane.View] {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("pane.view %q is not list or cards; using %s", raw.Pane.View, DefaultView))
		} else {
			cfg.Pane.View = raw.Pane.View
		}
	}

	if md.IsDefined("pane", "auto_open") {
		cfg.Pane.AutoOpen = raw.Pane.AutoOpen
	}

	if md.IsDefined("pane", "mouse") {
		cfg.Pane.Mouse = raw.Pane.Mouse
	}

	if md.IsDefined("pane", "right_click") {
		cfg.Pane.RightClick = raw.Pane.RightClick
	}

	if md.IsDefined("pane", "singleton") {
		cfg.Pane.Singleton = raw.Pane.Singleton
	}

	if md.IsDefined("pane", "width") {
		switch {
		case raw.Pane.Width == 0:
			// An explicit zero is "leave the split alone", not a mistake.
			cfg.Pane.Width = 0
		case raw.Pane.Width < minWidth || raw.Pane.Width > maxWidth:
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("pane.width %d is outside %d-%d; using %d", raw.Pane.Width, minWidth, maxWidth, DefaultWidth))
		default:
			cfg.Pane.Width = raw.Pane.Width
		}
	}

	if md.IsDefined("pr", "enabled") {
		cfg.PR.Enabled = raw.PR.Enabled
	}

	if md.IsDefined("tokens", "enabled") {
		cfg.Tokens.Enabled = raw.Tokens.Enabled
	}

	if raw.Tokens.Name != "" {
		if !tokenKeyPattern.MatchString(raw.Tokens.Name) {
			cfg.Warnings = append(cfg.Warnings, fmt.Sprintf("tokens.name %q is not a valid Herdr token key; using %s", raw.Tokens.Name, DefaultTokenName))
		} else {
			cfg.Tokens.Name = raw.Tokens.Name
		}
	}

	return cfg
}

func hasGroupCapture(re *regexp.Regexp) bool {
	for _, name := range re.SubexpNames() {
		if name == "group" {
			return true
		}
	}

	return false
}

// WriteResolved prints the normalised config as JSON. herdr/pane.sh reads its
// placement, direction and auto_open from here, so bash never parses TOML and
// the two halves cannot drift apart.
func WriteResolved(w io.Writer, cfg Config) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")

	return enc.Encode(cfg)
}
