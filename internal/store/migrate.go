package store

import (
	"encoding/json"
	"time"

	"github.com/simontheguitarist/tick/internal/rank"
)

// v1 mirrors the original schema exactly as it was written, so a pre-sync
// store parses without loss. Kept forever: migration must work on any old file.
type v1Step struct {
	Text      string     `json:"text"`
	Done      bool       `json:"done"`
	Important bool       `json:"important,omitempty"`
	Created   time.Time  `json:"created"`
	DoneAt    *time.Time `json:"done_at,omitempty"`
}

type v1Project struct {
	Path     string    `json:"path"`
	Name     string    `json:"name"`
	Group    string    `json:"group,omitempty"`
	Steps    []v1Step  `json:"steps"`
	Modified time.Time `json:"modified"`
}

type v1Store struct {
	Version  int                   `json:"version"`
	Projects map[string]*v1Project `json:"projects"`
}

// migrateV1 lifts a v1 document to v2: UUIDs for identity, evenly spread ranks
// preserving the manual order, and `updated` stamps derived from what v1
// recorded. Created/DoneAt values pass through untouched (their exact string
// forms matter to external readers of store.json). Nothing is marked dirty —
// `tick sync setup` marks everything for the first push.
func migrateV1(data []byte) (*Store, error) {
	var old v1Store
	if err := json.Unmarshal(data, &old); err != nil {
		return nil, err
	}
	s := emptyStore()
	s.migrated = true
	for key, op := range old.Projects {
		if op == nil {
			continue
		}
		up := op.Modified
		for _, ost := range op.Steps { // covers v1's zero-Modified hole (Ensure never stamped it)
			if ost.Created.After(up) {
				up = ost.Created
			}
		}
		if up.IsZero() {
			up = Now()
		}
		p := &Project{
			ID: NewID(), Path: key, Name: op.Name, Group: op.Group,
			Steps: make([]Step, 0, len(op.Steps)), Updated: up.Truncate(time.Millisecond),
		}
		n := len(op.Steps)
		for i, ost := range op.Steps {
			u := ost.Created
			if ost.DoneAt != nil {
				u = *ost.DoneAt
			}
			p.Steps = append(p.Steps, Step{
				ID: NewID(), Text: ost.Text, Done: ost.Done, Important: ost.Important,
				Created: ost.Created, DoneAt: ost.DoneAt,
				Rank: rank.Initial(i, n), Updated: u.Truncate(time.Millisecond),
			})
		}
		s.Projects[key] = p
	}
	return s, nil
}
