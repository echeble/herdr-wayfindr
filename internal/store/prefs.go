package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const prefsFileName = "ui.json"

// PrefsVersion is written into the file so a future format change can be
// detected rather than guessed at.
const PrefsVersion = 1

type prefsDocument struct {
	Version int    `json:"version"`
	View    string `json:"view"`
	Sort    string `json:"sort"`
}

// Prefs remembers the choices made inside the pane — which presentation it was
// last in, and which order it was last sorted by.
//
// They live apart from config.toml on purpose. The config file is what the user
// writes; this is what the pane writes back, and a plugin that edited the
// former would fight whoever was editing it by hand. config.toml seeds a fresh
// install, and from the first keystroke this file is the answer.
type Prefs struct {
	path string

	mu   sync.RWMutex
	view string
	sort string
}

// OpenPrefs reads the preference file from dir. A missing file is an empty set
// of preferences, not an error; an unreadable or malformed one is reported but
// still yields a usable Prefs, so a corrupt file costs the memory of the last
// choice rather than the pane.
func OpenPrefs(dir string) (*Prefs, error) {
	p := &Prefs{}

	if dir == "" {
		return p, fmt.Errorf("no state directory: the view and sort choices will not be remembered")
	}

	p.path = filepath.Join(dir, prefsFileName)

	data, err := os.ReadFile(p.path)
	if err != nil {
		if os.IsNotExist(err) {
			return p, nil
		}

		return p, fmt.Errorf("read %s: %w", p.path, err)
	}

	var doc prefsDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		return p, fmt.Errorf("parse %s: %w", p.path, err)
	}

	p.view, p.sort = doc.View, doc.Sort

	return p, nil
}

// View answers the remembered presentation, or "" when none was ever chosen —
// which is what tells the caller to fall back to the configured default.
func (p *Prefs) View() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.view
}

// Sort answers the remembered order, or "" when none was ever chosen.
func (p *Prefs) Sort() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.sort
}

func (p *Prefs) SetView(view string) error {
	p.mu.Lock()
	p.view = view
	p.mu.Unlock()

	return p.save()
}

func (p *Prefs) SetSort(sort string) error {
	p.mu.Lock()
	p.sort = sort
	p.mu.Unlock()

	return p.save()
}

func (p *Prefs) save() error {
	if p.path == "" {
		return fmt.Errorf("no state directory: the choice was not remembered")
	}

	p.mu.RLock()
	doc := prefsDocument{Version: PrefsVersion, View: p.view, Sort: p.sort}
	p.mu.RUnlock()

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode preferences: %w", err)
	}

	return writeAtomic(p.path, append(data, '\n'))
}
