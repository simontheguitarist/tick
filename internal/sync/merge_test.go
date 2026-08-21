package sync

import (
	"testing"
	"time"

	"github.com/simontheguitarist/tick/internal/protocol"
	"github.com/simontheguitarist/tick/internal/store"
)

func ts(s string) time.Time {
	t, err := protocol.ParseTime(s)
	if err != nil {
		panic(err)
	}
	return t
}

func baseStore() *store.Store {
	s := &store.Store{Version: 2, Projects: map[string]*store.Project{}}
	p := &store.Project{ID: "p1", Path: "/local/proj", Name: "proj",
		Updated: ts("2026-01-01T00:00:00.000Z"), Steps: []store.Step{{
			ID: "s1", Text: "hello", Rank: "V",
			Created: ts("2026-01-01T00:00:00.000Z"), Updated: ts("2026-01-01T00:00:00.000Z"),
		}}}
	s.Projects[p.Path] = p
	return s
}

func TestApplyRemoteNewerWins(t *testing.T) {
	s := baseStore()
	resp := protocol.SyncResponse{Seq: 7, Steps: []protocol.Step{{
		ID: "s1", ProjectID: "p1", Text: "edited elsewhere", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: "2026-01-02T00:00:00.000Z", Done: true, DoneAt: "2026-01-02T00:00:00.000Z",
	}}}
	if n := Apply(s, protocol.SyncRequest{}, resp); n != 1 {
		t.Fatalf("pulled %d, want 1", n)
	}
	st := s.Projects["/local/proj"].Steps[0]
	if st.Text != "edited elsewhere" || !st.Done || st.DoneAt == nil || st.Dirty {
		t.Fatalf("remote edit not applied: %+v", st)
	}
	if s.Sync.Since != 7 {
		t.Fatalf("cursor not advanced: %d", s.Sync.Since)
	}
}

func TestApplyLocalNewerKept(t *testing.T) {
	s := baseStore()
	p := s.Projects["/local/proj"]
	p.Steps[0].Text = "local newer"
	p.Steps[0].Updated = ts("2026-01-03T00:00:00.000Z")
	p.Steps[0].Dirty = true
	resp := protocol.SyncResponse{Seq: 3, Steps: []protocol.Step{{
		ID: "s1", ProjectID: "p1", Text: "stale remote", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: "2026-01-02T00:00:00.000Z",
	}}}
	Apply(s, protocol.SyncRequest{}, resp)
	if st := p.Steps[0]; st.Text != "local newer" || !st.Dirty {
		t.Fatalf("local newer edit lost: %+v", st)
	}
}

func TestApplyDirtyStaysWhenEditedMidFlight(t *testing.T) {
	s := baseStore()
	p := s.Projects["/local/proj"]
	// The push carried updated=T1, but the user edited again before the
	// response landed: local updated is now T2 — dirty must survive.
	req := protocol.SyncRequest{Steps: []protocol.Step{{ID: "s1", Updated: "2026-01-01T00:00:00.000Z"}}}
	p.Steps[0].Updated = ts("2026-01-05T00:00:00.000Z")
	p.Steps[0].Dirty = true
	Apply(s, req, protocol.SyncResponse{Seq: 1})
	if !p.Steps[0].Dirty {
		t.Fatal("mid-flight edit lost its dirty flag")
	}
	// And an unchanged push is acked.
	req2 := protocol.SyncRequest{Steps: []protocol.Step{{ID: "s1", Updated: "2026-01-05T00:00:00.000Z"}}}
	Apply(s, req2, protocol.SyncResponse{Seq: 2})
	if p.Steps[0].Dirty {
		t.Fatal("unchanged push not acked")
	}
}

func TestApplyTombstoneAckAndGuard(t *testing.T) {
	s := baseStore()
	p := s.Projects["/local/proj"]
	removed := s.RemoveStep(p, 0)
	delAt := s.Tombstones[0].Updated

	// Pushed tombstone gets acked away.
	req := protocol.SyncRequest{Steps: []protocol.Step{{ID: removed.ID, ProjectID: "p1", Deleted: true, Updated: protocol.FormatTime(delAt)}}}
	Apply(s, req, protocol.SyncResponse{Seq: 1})
	if len(s.Tombstones) != 0 {
		t.Fatalf("acked tombstone kept: %+v", s.Tombstones)
	}

	// A pending (un-pushed) tombstone must block resurrection by an older live copy.
	p.Steps = []store.Step{{ID: "s2", Text: "x", Rank: "V", Created: ts("2026-01-01T00:00:00.000Z"), Updated: ts("2026-01-01T00:00:00.000Z")}}
	s.RemoveStep(p, 0)
	resp := protocol.SyncResponse{Seq: 2, Steps: []protocol.Step{{
		ID: "s2", ProjectID: "p1", Text: "zombie", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: "2026-01-01T00:00:00.000Z",
	}}}
	Apply(s, protocol.SyncRequest{}, resp)
	if p.StepIndex("s2") >= 0 {
		t.Fatal("older live copy resurrected a deleted step")
	}
	// But a NEWER live copy (deleted here, then done elsewhere later) wins.
	resp2 := protocol.SyncResponse{Seq: 3, Steps: []protocol.Step{{
		ID: "s2", ProjectID: "p1", Text: "done later elsewhere", Rank: "V", Done: true,
		Created: "2026-01-01T00:00:00.000Z", Updated: "2126-01-01T00:00:00.000Z",
	}}}
	Apply(s, protocol.SyncRequest{}, resp2)
	if p.StepIndex("s2") < 0 {
		t.Fatal("newer live copy should override the local deletion")
	}
}

func TestApplyPathNeverOverwritten(t *testing.T) {
	s := baseStore()
	resp := protocol.SyncResponse{Seq: 1, Projects: []protocol.Project{{
		ID: "p1", Name: "renamed", Path: "/somewhere/else", Updated: "2026-02-01T00:00:00.000Z",
	}}}
	Apply(s, protocol.SyncRequest{}, resp)
	p := s.ByID("p1")
	if p == nil || p.Path != "/local/proj" || s.Projects["/local/proj"] != p {
		t.Fatalf("local path clobbered: %+v", p)
	}
	if p.Name != "renamed" {
		t.Fatalf("project-level fields not applied: %+v", p)
	}
}

func TestApplyUnknownProjectStepSkipped(t *testing.T) {
	s := baseStore()
	resp := protocol.SyncResponse{Seq: 1, Steps: []protocol.Step{{
		ID: "sX", ProjectID: "ghost", Text: "orphan", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: "2026-01-01T00:00:00.000Z",
	}}}
	if n := Apply(s, protocol.SyncRequest{}, resp); n != 0 {
		t.Fatalf("orphan step applied: %d", n)
	}
}

func TestApplyRemoteProjectAndStepsInserted(t *testing.T) {
	s := baseStore()
	resp := protocol.SyncResponse{Seq: 2,
		Projects: []protocol.Project{{ID: "p2", Name: "phone-project", Group: "ideas", Updated: "2026-01-01T00:00:00.000Z"}},
		Steps: []protocol.Step{{ID: "n1", ProjectID: "p2", Text: "made on the phone", Rank: "V",
			Created: "2026-01-01T00:00:00.000Z", Updated: "2026-01-01T00:00:00.000Z"}}}
	Apply(s, protocol.SyncRequest{}, resp)
	p := s.ByID("p2")
	if p == nil || p.Path != "" || s.Projects["p2"] != p { // keyed by id while unlinked
		t.Fatalf("remote project not inserted/keyed by id: %+v", p)
	}
	if len(p.Steps) != 1 || p.Steps[0].Text != "made on the phone" || p.Steps[0].Dirty {
		t.Fatalf("remote step not inserted clean: %+v", p.Steps)
	}
	// Times must be stored in the local zone for store.json's external readers.
	if _, off := p.Steps[0].Created.Zone(); off != timeLocalOffset(p.Steps[0].Created) {
		t.Fatalf("created not in local zone")
	}
}

func timeLocalOffset(t time.Time) int {
	_, off := t.In(time.Local).Zone()
	return off
}

func TestApplyProjectTombstone(t *testing.T) {
	s := baseStore()
	resp := protocol.SyncResponse{Seq: 1, Projects: []protocol.Project{{
		ID: "p1", Deleted: true, Updated: "2026-02-01T00:00:00.000Z"}}}
	Apply(s, protocol.SyncRequest{}, resp)
	if len(s.Projects) != 0 {
		t.Fatalf("untracked-elsewhere project survived: %+v", s.Projects)
	}
}

func TestApplyEqualStampTieFollowsServer(t *testing.T) {
	// Both devices edited in the same ms; the server picked the other device
	// (tie-break by device id) and echoes its version. Our copy is clean
	// (already acked) with the same stamp but different text: we must adopt.
	s := baseStore()
	p := s.Projects["/local/proj"]
	p.Steps[0].Text = "ours"
	p.Steps[0].Dirty = false
	resp := protocol.SyncResponse{Seq: 2, Steps: []protocol.Step{{
		ID: "s1", ProjectID: "p1", Text: "theirs", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: protocol.FormatTime(p.Steps[0].Updated),
	}}}
	Apply(s, protocol.SyncRequest{}, resp)
	if p.Steps[0].Text != "theirs" {
		t.Fatalf("tie-loser must converge to the server's winner, got %q", p.Steps[0].Text)
	}
	// An echo of our own identical record is a no-op (no churn).
	before := p.Steps[0]
	Apply(s, protocol.SyncRequest{}, protocol.SyncResponse{Seq: 3, Steps: []protocol.Step{{
		ID: "s1", ProjectID: "p1", Text: "theirs", Rank: "V",
		Created: "2026-01-01T00:00:00.000Z", Updated: protocol.FormatTime(p.Steps[0].Updated),
	}}})
	if p.Steps[0] != before {
		t.Fatal("identical echo must not rewrite the local record")
	}
}
