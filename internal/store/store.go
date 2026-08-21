// Package store holds tick's persistent state: a single JSON document under
// the config dir. Mutations go through Update, which takes an exclusive file
// lock and writes atomically.
//
// Schema v2 gives every project and step a UUID, a fractional-index rank
// (steps are kept sorted by it), an `updated` stamp and a `dirty` flag, plus a
// tombstone list for deletions — together the whole state a two-way sync
// needs, while keeping the v1 reader-visible shape (a `projects` map with
// `text`/`done`/`done_at` steps) intact for external consumers.
package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"syscall"
	"time"

	"github.com/simontheguitarist/tick/internal/rank"
)

const schemaVersion = 2

// Step is a single next-step in a project.
type Step struct {
	ID        string     `json:"id"`
	Text      string     `json:"text"`
	Done      bool       `json:"done"`
	Important bool       `json:"important,omitempty"` // flagged as higher priority (visual only)
	Created   time.Time  `json:"created"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
	Rank      string     `json:"rank"`            // fractional index; slice order == rank order
	Updated   time.Time  `json:"updated"`         // last mutation, drives last-writer-wins
	Dirty     bool       `json:"dirty,omitempty"` // local-only: not yet pushed
}

// Project is an ordered list of steps. Path is where it lives on this machine;
// a project created on another device has Path == "" until it is linked.
type Project struct {
	ID      string    `json:"id"`
	Path    string    `json:"path,omitempty"` // absolute, symlink-resolved; also the map key when set
	Name    string    `json:"name"`
	Group   string    `json:"group,omitempty"` // optional overview section ("" = ungrouped)
	Steps   []Step    `json:"steps"`           // live steps only, sorted by (Rank, ID)
	Updated time.Time `json:"updated"`         // bumps on project-level fields only, never on step edits
	Dirty   bool      `json:"dirty,omitempty"`
}

// Tombstone records a deletion that still has to reach the server.
type Tombstone struct {
	Kind      string    `json:"kind"` // "step" | "project"
	ID        string    `json:"id"`
	ProjectID string    `json:"project_id,omitempty"`
	Updated   time.Time `json:"updated"`
}

// SyncState is the pull cursor. It lives inside store.json so cursor and data
// are always written in the same atomic rename — a crash can never persist one
// without the other.
type SyncState struct {
	Since int64 `json:"since"`
}

// Store is the whole on-disk document. The map key is the project's Path when
// linked to a directory on this machine, else its ID.
type Store struct {
	Version    int                 `json:"version"`
	Projects   map[string]*Project `json:"projects"`
	Tombstones []Tombstone         `json:"tombstones,omitempty"`
	Sync       SyncState           `json:"sync"`

	migrated bool // set by load when the file was a v1 document
}

// Key returns p's map key: the local path when linked, else the ID.
func Key(p *Project) string {
	if p.Path != "" {
		return p.Path
	}
	return p.ID
}

// Touch stamps a step as locally modified and pending push.
func Touch(st *Step) {
	st.Updated = Bump(st.Updated)
	st.Dirty = true
}

// TouchProject stamps a project as locally modified and pending push. Only
// project-level fields (name, group, path, deletion) touch the project; step
// edits deliberately don't, so `tick add` here can't clobber a group change
// made on the phone.
func TouchProject(p *Project) {
	p.Updated = Bump(p.Updated)
	p.Dirty = true
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

// AddStep appends a new open step after the current last one.
func (p *Project) AddStep(text string) *Step {
	last := ""
	if len(p.Steps) > 0 {
		last = p.Steps[len(p.Steps)-1].Rank
	}
	now := Now()
	p.Steps = append(p.Steps, Step{
		ID: NewID(), Text: text, Created: now,
		Rank: rank.After(last), Updated: now, Dirty: true,
	})
	return &p.Steps[len(p.Steps)-1]
}

// SortSteps restores the (Rank, ID) invariant after ranks changed or steps
// arrived from elsewhere.
func (p *Project) SortSteps() {
	sort.SliceStable(p.Steps, func(i, j int) bool {
		if p.Steps[i].Rank != p.Steps[j].Rank {
			return p.Steps[i].Rank < p.Steps[j].Rank
		}
		return p.Steps[i].ID < p.Steps[j].ID
	})
}

// StepIndex returns the position of the step with the given id, or -1.
func (p *Project) StepIndex(id string) int {
	for i := range p.Steps {
		if p.Steps[i].ID == id {
			return i
		}
	}
	return -1
}

// Rebalance re-keys every step with evenly spread ranks once any rank has
// grown past rank.MaxLen. All steps become dirty; concurrent remote moves are
// resolved by last-writer-wins like any other edit.
func (p *Project) Rebalance() {
	for i := range p.Steps {
		if len(p.Steps[i].Rank) > rank.MaxLen {
			n := len(p.Steps)
			for j := range p.Steps {
				p.Steps[j].Rank = rank.Initial(j, n)
				Touch(&p.Steps[j])
			}
			return
		}
	}
}

// Ensure returns the project for path, creating it if absent. If exactly one
// unlinked project (created on another device) has the same name, it adopts
// the path instead — running `tick` inside a matching directory links the two.
func (s *Store) Ensure(path, name string) *Project {
	if p := s.Projects[path]; p != nil {
		return p
	}
	var match *Project
	matches := 0
	for _, p := range s.Projects {
		if p.Path == "" && p.Name == name {
			match, matches = p, matches+1
		}
	}
	if matches == 1 {
		s.Link(match, path)
		return match
	}
	p := &Project{ID: NewID(), Path: path, Name: name, Steps: []Step{}, Updated: Now(), Dirty: true}
	s.Projects[path] = p
	return p
}

// Link attaches a project to a local path and re-keys it in the map.
func (s *Store) Link(p *Project, path string) {
	delete(s.Projects, Key(p))
	p.Path = path
	TouchProject(p)
	s.Projects[path] = p
}

// ByID finds a project by its UUID.
func (s *Store) ByID(id string) *Project {
	for _, p := range s.Projects {
		if p.ID == id {
			return p
		}
	}
	return nil
}

// Untrack removes a whole project, leaving tombstones for it and every step so
// the deletion propagates.
func (s *Store) Untrack(p *Project) {
	now := Bump(p.Updated)
	for i := range p.Steps {
		s.Tombstones = append(s.Tombstones, Tombstone{Kind: "step", ID: p.Steps[i].ID, ProjectID: p.ID, Updated: now})
	}
	s.Tombstones = append(s.Tombstones, Tombstone{Kind: "project", ID: p.ID, Updated: now})
	delete(s.Projects, Key(p))
}

// RemoveStep deletes the step at index i, leaving a tombstone. It returns the
// removed step so callers can offer undo.
func (s *Store) RemoveStep(p *Project, i int) Step {
	st := p.Steps[i]
	s.Tombstones = append(s.Tombstones, Tombstone{Kind: "step", ID: st.ID, ProjectID: p.ID, Updated: Bump(st.Updated)})
	p.Steps = append(p.Steps[:i], p.Steps[i+1:]...)
	return st
}

// RestoreStep undoes a RemoveStep: the pending tombstone is dropped and the
// step re-enters with a fresh stamp, so even a tombstone that already reached
// the server is overridden by the newer write.
func (s *Store) RestoreStep(p *Project, st Step) *Step {
	for i := range s.Tombstones {
		if s.Tombstones[i].Kind == "step" && s.Tombstones[i].ID == st.ID {
			s.Tombstones = append(s.Tombstones[:i], s.Tombstones[i+1:]...)
			break
		}
	}
	Touch(&st)
	p.Steps = append(p.Steps, st)
	p.SortSteps()
	return &p.Steps[p.StepIndex(st.ID)]
}

// SortedProjects returns the projects ordered by display name (id as tiebreak).
func (s *Store) SortedProjects() []*Project {
	out := make([]*Project, 0, len(s.Projects))
	for _, p := range s.Projects {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Dir is a store location on disk. The Dir-based API exists so tests (and the
// two-device sync end-to-end test) can run several stores in one process; the
// package-level Load/Update use the default config dir.
type Dir string

// DefaultDir resolves the config dir (~/.config/tick, honoring XDG_CONFIG_HOME).
func DefaultDir() (Dir, error) {
	d, err := configDir()
	return Dir(d), err
}

// Load reads the store read-only, returning an empty store if none exists yet.
func Load() (*Store, error) {
	d, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return d.Load()
}

// Update performs a locked read-modify-write on the default store.
func Update(fn func(*Store) error) (*Store, error) {
	d, err := DefaultDir()
	if err != nil {
		return nil, err
	}
	return d.Update(fn)
}

// Load reads the store in d. A v1 file is migrated — and immediately persisted
// through Update, because migration mints random IDs: without the write-back,
// two consecutive reads would see two different identities for the same step.
func (d Dir) Load() (*Store, error) {
	s, err := load(string(d))
	if err != nil {
		return nil, err
	}
	if s.migrated {
		return d.Update(func(*Store) error { return nil })
	}
	return s, nil
}

// Update performs a locked read-modify-write: it takes an exclusive lock,
// loads the freshest state from disk, applies fn, then writes it back
// atomically. Both the CLI verbs and the TUI mutate exclusively through this
// so concurrent invocations can't lose writes. It returns the post-mutation
// store. Do no network I/O inside fn — the lock blocks every other tick.
func (d Dir) Update(fn func(*Store) error) (*Store, error) {
	dir := string(d)
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
	if s.migrated {
		backupV1(dir)
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
		return emptyStore(), nil
	}
	if err != nil {
		return nil, err
	}
	var probe struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("store.json is not valid JSON: %w", err)
	}
	if probe.Version > schemaVersion {
		return nil, fmt.Errorf("store.json is schema v%d, written by a newer tick — upgrade tick instead of risking data loss", probe.Version)
	}
	if probe.Version <= 1 {
		return migrateV1(data)
	}
	var s Store
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	if s.Projects == nil {
		s.Projects = map[string]*Project{}
	}
	for _, p := range s.Projects {
		p.SortSteps()
	}
	return &s, nil
}

func emptyStore() *Store {
	return &Store{Version: schemaVersion, Projects: map[string]*Project{}}
}

// backupV1 keeps a one-time copy of the pre-migration file next to the store.
func backupV1(dir string) {
	dst := filepath.Join(dir, "store.v1.json")
	if _, err := os.Stat(dst); err == nil {
		return
	}
	if data, err := os.ReadFile(filepath.Join(dir, "store.json")); err == nil {
		_ = os.WriteFile(dst, data, 0o600)
	}
}

// save writes the store atomically: a temp file in the same directory, synced,
// then renamed over store.json. rename(2) on one filesystem is atomic, so a
// crash mid-write never leaves a half-written store.
func save(dir string, s *Store) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	s.Version = schemaVersion
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
