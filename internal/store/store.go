// Package store persists the explicit group tags and the group label
// overrides that rename a header without touching them. Herdr has no storage
// API for plugins and worktrees carry no writable metadata, so this file is
// the only place either kind of hand-made state can live.
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const fileName = "groups.json"

// Version is written into the file so a future format change can be detected
// rather than guessed at.
const Version = 1

type document struct {
	Version int               `json:"version"`
	Tags    map[string]string `json:"tags"`
	// Labels renames a group's header without touching what put a worktree in
	// it: the key is the group's own resolved name — a tag, a ticket, a branch
	// match, or a shared directory name — not a checkout path, so one rename
	// covers every worktree already in the group and any more that join it
	// later.
	Labels map[string]string `json:"labels"`
}

// Store holds the tag map and the group label overrides, and writes both back
// atomically together. Tags are keyed by checkout path, which is a worktree's
// only stable identity; labels are keyed by the group name they retitle.
type Store struct {
	path string

	mu     sync.RWMutex
	tags   map[string]string
	labels map[string]string
}

// Open reads the tag file from dir, creating nothing. A missing file is an
// empty store; an unreadable or malformed one is reported but still yields a
// usable store, so a corrupt file costs the tags rather than the pane.
func Open(dir string) (*Store, error) {
	s := &Store{tags: map[string]string{}, labels: map[string]string{}}

	if dir == "" {
		return s, fmt.Errorf("no state directory: explicit tags will not be saved")
	}

	s.path = filepath.Join(dir, fileName)

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}

		return s, fmt.Errorf("read %s: %w", s.path, err)
	}

	var doc document
	if err := json.Unmarshal(data, &doc); err != nil {
		return s, fmt.Errorf("parse %s: %w", s.path, err)
	}

	for path, name := range doc.Tags {
		if name = strings.TrimSpace(name); name != "" {
			s.tags[path] = name
		}
	}

	for name, label := range doc.Labels {
		if label = strings.TrimSpace(label); label != "" {
			s.labels[name] = label
		}
	}

	return s, nil
}

// Tags answers a copy of the tag map, safe to hand to a resolver running on
// another goroutine.
func (s *Store) Tags() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]string, len(s.tags))
	for path, name := range s.tags {
		out[path] = name
	}

	return out
}

// Set tags a checkout. An empty name clears the tag, which lets the worktree
// fall back to a derived group rather than pinning it to a nameless one.
func (s *Store) Set(path, name string) error {
	s.mu.Lock()

	name = strings.TrimSpace(name)
	if name == "" {
		delete(s.tags, path)
	} else {
		s.tags[path] = name
	}

	s.mu.Unlock()

	return s.save()
}

// Labels answers a copy of the group label overrides, keyed by the resolved
// group name each one retitles.
func (s *Store) Labels() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make(map[string]string, len(s.labels))
	for name, label := range s.labels {
		out[name] = label
	}

	return out
}

// SetLabel renames a group's header without touching what put a worktree in
// it: name is the group's own resolved key — a tag, a ticket, a branch match,
// or a shared directory name — and only the label shown in its place changes.
// An empty label clears the override, so the header goes back to the name
// itself.
func (s *Store) SetLabel(name, label string) error {
	s.mu.Lock()

	name = strings.TrimSpace(name)
	label = strings.TrimSpace(label)

	if label == "" {
		delete(s.labels, name)
	} else {
		s.labels[name] = label
	}

	s.mu.Unlock()

	return s.save()
}

// Prune drops tags whose checkout no longer exists. Removing a worktree is not
// something the plugin is told about reliably, so this runs on the slow tick.
// It returns the number of entries dropped.
func (s *Store) Prune(livePaths map[string]bool) (int, error) {
	s.mu.Lock()

	var dropped int

	for path := range s.tags {
		if livePaths[path] {
			continue
		}

		// Only forget a path git no longer knows AND that is gone from disk:
		// a repo that simply is not open in Herdr must keep its tag.
		if _, err := os.Stat(path); err == nil {
			continue
		}

		delete(s.tags, path)
		dropped++
	}

	s.mu.Unlock()

	if dropped == 0 {
		return 0, nil
	}

	return dropped, s.save()
}

// Names answers the distinct tag names in use, sorted — the completion list the
// tag prompt offers.
func (s *Store) Names() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	seen := map[string]bool{}

	var out []string

	for _, name := range s.tags {
		if seen[name] {
			continue
		}

		seen[name] = true

		out = append(out, name)
	}

	sort.Strings(out)

	return out
}

// save writes the whole document out atomically.
func (s *Store) save() error {
	if s.path == "" {
		return fmt.Errorf("no state directory: tags were not saved")
	}

	s.mu.RLock()
	doc := document{Version: Version, Tags: map[string]string{}, Labels: map[string]string{}}

	for path, name := range s.tags {
		doc.Tags[path] = name
	}

	for name, label := range s.labels {
		doc.Labels[name] = label
	}
	s.mu.RUnlock()

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return fmt.Errorf("encode tags: %w", err)
	}

	return writeAtomic(s.path, append(data, '\n'))
}

// writeAtomic writes through a temporary file in the same directory and renames
// it over the old one, so an interrupted write cannot leave a half-parsed file
// behind. Both state files go through it.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create temp file in %s: %w", dir, err)
	}

	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)

		return fmt.Errorf("write %s: %w", tmpName, err)
	}

	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("close %s: %w", tmpName, err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)

		return fmt.Errorf("replace %s: %w", path, err)
	}

	return nil
}
