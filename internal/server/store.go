package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"syscall"

	"github.com/simontheguitarist/tick/internal/protocol"
)

// storedProject / storedStep are wire entities plus the device that wrote the
// winning version (the LWW tie-break, and useful when reading tick.json by hand).
type storedProject struct {
	protocol.Project
	Device string `json:"device,omitempty"`
}

type storedStep struct {
	protocol.Step
	Device string `json:"device,omitempty"`
}

type document struct {
	Seq      int64                     `json:"seq"`
	Projects map[string]*storedProject `json:"projects"`
	Steps    map[string]*storedStep    `json:"steps"`
}

// Store is the server's whole persistence layer: one JSON document guarded by
// a mutex, written with the same temp-file-and-rename dance as the CLI store.
// Tombstones are kept forever — the data is a single person's step list, so
// durability beats compaction. SQLite would buy nothing here.
type Store struct {
	mu   sync.Mutex
	dir  string
	lock *os.File
	doc  document
}

// Open loads (or initializes) the document in dir and takes a non-blocking
// exclusive flock so a second tickd on the same volume fails fast instead of
// silently interleaving writes.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	lf, err := os.OpenFile(filepath.Join(dir, "tickd.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lf.Close()
		return nil, fmt.Errorf("another tickd is already serving %s: %w", dir, err)
	}
	s := &Store{dir: dir, lock: lf}
	data, err := os.ReadFile(s.path())
	switch {
	case errors.Is(err, os.ErrNotExist):
		s.doc = document{Projects: map[string]*storedProject{}, Steps: map[string]*storedStep{}}
	case err != nil:
		lf.Close()
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.doc); err != nil {
			lf.Close()
			return nil, fmt.Errorf("%s is corrupt: %w", s.path(), err)
		}
		if s.doc.Projects == nil {
			s.doc.Projects = map[string]*storedProject{}
		}
		if s.doc.Steps == nil {
			s.doc.Steps = map[string]*storedStep{}
		}
	}
	return s, nil
}

// Close releases the instance lock.
func (s *Store) Close() error {
	syscall.Flock(int(s.lock.Fd()), syscall.LOCK_UN)
	return s.lock.Close()
}

func (s *Store) path() string { return filepath.Join(s.dir, "tick.json") }

// Apply runs one sync exchange: last-writer-wins ingest of the pushed
// entities, then a snapshot of everything the caller hasn't seen. Timestamps
// must already be canonical (the handler guarantees it), so LWW is a plain
// string compare; ties break on device id, deterministically on every replica.
func (s *Store) Apply(req protocol.SyncRequest) (protocol.SyncResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	touchedP := make([]string, 0, len(req.Projects))
	touchedS := make([]string, 0, len(req.Steps))
	changed := false

	for _, in := range req.Projects {
		touchedP = append(touchedP, in.ID)
		ex := s.doc.Projects[in.ID]
		if ex != nil && !wins(in.Updated, req.Device, ex.Updated, ex.Device) {
			continue
		}
		s.doc.Seq++
		in.Seq = s.doc.Seq
		s.doc.Projects[in.ID] = &storedProject{Project: in, Device: req.Device}
		changed = true
	}
	for _, in := range req.Steps {
		touchedS = append(touchedS, in.ID)
		ex := s.doc.Steps[in.ID]
		if ex != nil && !wins(in.Updated, req.Device, ex.Updated, ex.Device) {
			continue
		}
		s.doc.Seq++
		in.Seq = s.doc.Seq
		s.doc.Steps[in.ID] = &storedStep{Step: in, Device: req.Device}
		changed = true
	}

	if changed {
		if err := s.save(); err != nil {
			return protocol.SyncResponse{}, err
		}
	}

	resp := protocol.SyncResponse{Seq: s.doc.Seq}
	seenP := map[string]bool{}
	for _, p := range s.doc.Projects {
		if p.Seq > req.Since {
			resp.Projects = append(resp.Projects, p.Project)
			seenP[p.ID] = true
		}
	}
	for _, id := range touchedP { // rejected pushes: return the winner regardless of cursor
		if p := s.doc.Projects[id]; p != nil && !seenP[id] {
			resp.Projects = append(resp.Projects, p.Project)
			seenP[id] = true
		}
	}
	seenS := map[string]bool{}
	for _, st := range s.doc.Steps {
		if st.Seq > req.Since {
			resp.Steps = append(resp.Steps, st.Step)
			seenS[st.ID] = true
		}
	}
	for _, id := range touchedS {
		if st := s.doc.Steps[id]; st != nil && !seenS[id] {
			resp.Steps = append(resp.Steps, st.Step)
			seenS[id] = true
		}
	}
	sort.Slice(resp.Projects, func(i, j int) bool { return resp.Projects[i].Seq < resp.Projects[j].Seq })
	sort.Slice(resp.Steps, func(i, j int) bool { return resp.Steps[i].Seq < resp.Steps[j].Seq })
	return resp, nil
}

// wins decides last-writer-wins between an incoming and an existing record.
// Canonical fixed-width timestamps make the string compare a time compare.
// A same-device equal-time re-push does not win, keeping retries idempotent.
func wins(inUpdated, inDevice, exUpdated, exDevice string) bool {
	if inUpdated != exUpdated {
		return inUpdated > exUpdated
	}
	return inDevice > exDevice
}

// save writes the document atomically (temp file, fsync, rename) — the same
// crash-safety dance as the CLI's store.
func (s *Store) save() error {
	data, err := json.MarshalIndent(&s.doc, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, "tick-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
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
	return os.Rename(tmpName, s.path())
}
