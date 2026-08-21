package sync

import (
	"os"
	"time"

	"github.com/simontheguitarist/tick/internal/protocol"
	"github.com/simontheguitarist/tick/internal/store"
)

// BuildRequest snapshots the outbox: every dirty project and step plus all
// pending tombstones. Runs on a Load()ed snapshot, outside the store lock.
func BuildRequest(s *store.Store, device string, since int64) protocol.SyncRequest {
	req := protocol.SyncRequest{Device: device, Since: since}
	for _, p := range s.Projects {
		if p.Dirty {
			req.Projects = append(req.Projects, wireProject(p))
		}
		for i := range p.Steps {
			if p.Steps[i].Dirty {
				req.Steps = append(req.Steps, wireStep(&p.Steps[i], p.ID))
			}
		}
	}
	for _, t := range s.Tombstones {
		switch t.Kind {
		case "project":
			req.Projects = append(req.Projects, protocol.Project{
				ID: t.ID, Deleted: true, Updated: protocol.FormatTime(t.Updated)})
		default:
			req.Steps = append(req.Steps, protocol.Step{
				ID: t.ID, ProjectID: t.ProjectID, Deleted: true, Updated: protocol.FormatTime(t.Updated)})
		}
	}
	return req
}

// Apply merges a server response into the store. It must run inside
// store.Update. Rules (identical in the iOS app, see docs/protocol.md):
//
//   - a pushed entity whose local `updated` is unchanged since the push is
//     acked: dirty cleared / tombstone dropped (an edit that raced the push
//     stays dirty for the next round);
//   - a remote entity replaces the local one iff remote.updated is newer —
//     or equal while the local copy is clean, because the server breaks
//     equal-stamp ties by device id and echoes the winner (a clean local
//     copy with the same stamp but different content is the tie's loser);
//     deletions replace by removing;
//   - a remote insert is skipped when a *newer* local tombstone exists —
//     otherwise the local deletion would resurrect;
//   - a project's local Path is never overwritten; a remote path is adopted
//     only when local has none and the directory exists on this machine;
//   - finally the cursor advances to resp.Seq.
func Apply(s *store.Store, req protocol.SyncRequest, resp protocol.SyncResponse) (pulled int) {
	ackProjects := map[string]string{}
	for _, p := range req.Projects {
		ackProjects[p.ID] = p.Updated
	}
	ackSteps := map[string]string{}
	for _, st := range req.Steps {
		ackSteps[st.ID] = st.Updated
	}

	// 1. Clear dirty flags for unchanged pushed entities.
	for _, p := range s.Projects {
		if u, ok := ackProjects[p.ID]; ok && u == protocol.FormatTime(p.Updated) {
			p.Dirty = false
		}
		for i := range p.Steps {
			if u, ok := ackSteps[p.Steps[i].ID]; ok && u == protocol.FormatTime(p.Steps[i].Updated) {
				p.Steps[i].Dirty = false
			}
		}
	}
	// 2. Pushed tombstones are settled either way: accepted, or outvoted by a
	// newer live version that arrives in this very response.
	kept := s.Tombstones[:0]
	for _, t := range s.Tombstones {
		acked := false
		if u, ok := ackProjects[t.ID]; ok && t.Kind == "project" && u == protocol.FormatTime(t.Updated) {
			acked = true
		}
		if u, ok := ackSteps[t.ID]; ok && t.Kind == "step" && u == protocol.FormatTime(t.Updated) {
			acked = true
		}
		if !acked {
			kept = append(kept, t)
		}
	}
	s.Tombstones = kept

	// Remaining tombstones still guard against resurrection below.
	pendingDel := map[string]time.Time{}
	for _, t := range s.Tombstones {
		pendingDel[t.Kind+"/"+t.ID] = t.Updated
	}

	// 3. Projects first, so steps always find their project.
	for _, rp := range resp.Projects {
		local := s.ByID(rp.ID)
		rt, err := protocol.ParseTime(rp.Updated)
		if err != nil {
			continue
		}
		rt = rt.In(time.Local)
		if rp.Deleted {
			if local != nil && (rt.After(local.Updated) || (rt.Equal(local.Updated) && !local.Dirty)) {
				delete(s.Projects, store.Key(local))
				pulled++
			}
			continue
		}
		switch {
		case local == nil:
			if del, ok := pendingDel["project/"+rp.ID]; ok && !rt.After(del) {
				continue // our own deletion is newer — don't resurrect
			}
			p := &store.Project{ID: rp.ID, Name: rp.Name, Group: rp.Group,
				Steps: []store.Step{}, Updated: rt}
			if rp.Path != "" && dirExists(rp.Path) && s.Projects[rp.Path] == nil {
				p.Path = rp.Path
			}
			s.Projects[store.Key(p)] = p
			pulled++
		case rt.After(local.Updated) || (rt.Equal(local.Updated) && !local.Dirty && !sameProject(local, rp)):
			local.Name, local.Group, local.Updated, local.Dirty = rp.Name, rp.Group, rt, false
			if local.Path == "" && rp.Path != "" && dirExists(rp.Path) && s.Projects[rp.Path] == nil {
				delete(s.Projects, store.Key(local))
				local.Path = rp.Path
				s.Projects[rp.Path] = local
			}
			pulled++
		}
	}

	// 4. Steps.
	resort := map[*store.Project]bool{}
	for _, rs := range resp.Steps {
		p := s.ByID(rs.ProjectID)
		rt, err := protocol.ParseTime(rs.Updated)
		if err != nil {
			continue
		}
		rt = rt.In(time.Local)
		if rs.Deleted {
			if p == nil {
				continue
			}
			if i := p.StepIndex(rs.ID); i >= 0 && (rt.After(p.Steps[i].Updated) || (rt.Equal(p.Steps[i].Updated) && !p.Steps[i].Dirty)) {
				p.Steps = append(p.Steps[:i], p.Steps[i+1:]...)
				pulled++
			}
			continue
		}
		if p == nil {
			continue // project unknown here (deleted locally, or invalid) — the guard tombstone will win
		}
		st, ok := fromWireStep(rs, rt)
		if !ok {
			continue
		}
		if i := p.StepIndex(rs.ID); i >= 0 {
			cur := &p.Steps[i]
			if rt.After(cur.Updated) || (rt.Equal(cur.Updated) && !cur.Dirty && !sameStep(cur, p.ID, rs)) {
				p.Steps[i] = st
				resort[p] = true
				pulled++
			}
			continue
		}
		if del, ok := pendingDel["step/"+rs.ID]; ok && !rt.After(del) {
			continue
		}
		p.Steps = append(p.Steps, st)
		resort[p] = true
		pulled++
	}
	for p := range resort {
		p.SortSteps()
	}

	s.Sync.Since = resp.Seq
	return pulled
}

// MarkAllDirty queues the entire store for push (first sync, or --full).
func MarkAllDirty(s *store.Store) {
	for _, p := range s.Projects {
		p.Dirty = true
		for i := range p.Steps {
			p.Steps[i].Dirty = true
		}
	}
}

func wireProject(p *store.Project) protocol.Project {
	return protocol.Project{
		ID: p.ID, Name: p.Name, Path: p.Path, Group: p.Group,
		Updated: protocol.FormatTime(p.Updated),
	}
}

func wireStep(st *store.Step, projectID string) protocol.Step {
	w := protocol.Step{
		ID: st.ID, ProjectID: projectID, Text: st.Text, Done: st.Done,
		Important: st.Important, Rank: st.Rank,
		Created: protocol.FormatTime(st.Created), Updated: protocol.FormatTime(st.Updated),
	}
	if st.DoneAt != nil {
		w.DoneAt = protocol.FormatTime(*st.DoneAt)
	}
	return w
}

// fromWireStep converts a live remote step, rendering times in local zone so
// store.json stays uniformly local-offset for its external readers.
func fromWireStep(rs protocol.Step, updated time.Time) (store.Step, bool) {
	created, err := protocol.ParseTime(rs.Created)
	if err != nil {
		return store.Step{}, false
	}
	st := store.Step{
		ID: rs.ID, Text: rs.Text, Done: rs.Done, Important: rs.Important,
		Rank: rs.Rank, Created: created.In(time.Local), Updated: updated,
	}
	if rs.DoneAt != "" {
		if t, err := protocol.ParseTime(rs.DoneAt); err == nil {
			lt := t.In(time.Local)
			st.DoneAt = &lt
		}
	}
	return st, true
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// sameStep / sameProject tell an echo of our own push (identical content)
// from a tie the server resolved against us, without ever rewriting our own
// records — that would only churn timestamp precision in store.json.
func sameStep(local *store.Step, projectID string, rs protocol.Step) bool {
	w := wireStep(local, projectID)
	rs.Seq = 0
	return w == rs
}

func sameProject(local *store.Project, rp protocol.Project) bool {
	w := wireProject(local)
	rp.Seq = 0
	return w == rp
}
