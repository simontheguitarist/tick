// Package store holds tick's persistent state: a single global JSON file under
// the config dir, keyed by absolute project path. Mutations go through Update,
// which takes an exclusive file lock and writes atomically.
package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"
)

const schemaVersion = 1

// Step is a single next-step in a project.
type Step struct {
	Text      string     `json:"text"`
	Done      bool       `json:"done"`
	Important bool       `json:"important,omitempty"` // flagged as higher priority (visual only)
	Created   time.Time  `json:"created"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
}

// Project is an ordered list of steps for one path on disk.
type Project struct {
	Path     string    `json:"path"`            // absolute, symlink-resolved; also the map key
	Name     string    `json:"name"`            // display only (filepath.Base of Path)
	Group    string    `json:"group,omitempty"` // optional overview section ("" = ungrouped)
	Steps    []Step    `json:"steps"`
	Modified time.Time `json:"modified"`
}

// OpenCount returns the number of not-yet-done steps.
func (p *Project) OpenCount() int {
	n := 0
	for i := range p.Steps {
		if !p.Steps[i].Done {
			n++
		}
	}
	return n
}

// AddStep appends a new open step and bumps Modified.
func (p *Project) AddStep(text string) {
	now := time.Now()
	p.Steps = append(p.Steps, Step{Text: text, Created: now})
	p.Modified = now
}

// Store is the whole on-disk document.
type Store struct {
	Version  int                 `json:"version"`
	Projects map[string]*Project `json:"projects"`
}

// Ensure returns the project for path, creating an empty one if absent.
func (s *Store) Ensure(path, name string) *Project {
	if p := s.Projects[path]; p != nil {
		return p
	}
	p := &Project{Path: path, Name: name, Steps: []Step{}}
	s.Projects[path] = p
	return p
}

// SortedProjects returns the projects ordered by display name (path as tiebreak).
func (s *Store) SortedProjects() []*Project {
	out := make([]*Project, 0, len(s.Projects))
	for _, p := range s.Projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Path < out[j].Path
	})
	return out
}

// Load reads the store read-only, returning an empty store if none exists yet.
func Load() (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	return load(dir)
}

// Update performs a locked read-modify-write: it takes an exclusive lock, loads
// the freshest state from disk, applies fn, then writes it back atomically. Both
// the CLI verbs and the TUI mutate exclusively through this so concurrent
// invocations can't lose writes. It returns the post-mutation store.
func Update(fn func(*Store) error) (*Store, error) {
	dir, err := configDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	lf, err := os.OpenFile(filepath.Join(dir, "store.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	defer lf.Close()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX); err != nil {
		return nil, err
	}
	defer syscall.Flock(int(lf.Fd()), syscall.LOCK_UN)

	s, err := load(dir)
	if err != nil {
		return nil, err
	}
	if err := fn(s); err != nil {
		return nil, err
	}
	if err := save(dir, s); err != nil {
		return nil, err
	}
	return s, nil
}

func load(dir string) (*Store, error) {
	data, err := os.ReadFile(filepath.Join(dir, "store.json"))
	if errors.Is(err, os.ErrNotExist) {
		return &Store{Version: schemaVersion, Projects: map[string]*Project{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Projects == nil {
		s.Projects = map[string]*Project{}
	}
	if s.Version == 0 {
		s.Version = schemaVersion
	}
	return &s, nil
}

// save writes the store atomically: a temp file in the same directory, synced,
// then renamed over store.json. rename(2) on one filesystem is atomic, so a
// crash mid-write never leaves a half-written store.
func save(dir string, s *Store) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "store-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, filepath.Join(dir, "store.json"))
}
