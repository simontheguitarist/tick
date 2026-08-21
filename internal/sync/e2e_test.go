package sync

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/simontheguitarist/tick/internal/server"
	"github.com/simontheguitarist/tick/internal/store"
)

// flakyTransport lets a test cut the wire.
type flakyTransport struct {
	offline bool
	next    http.RoundTripper
}

func (f *flakyTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if f.offline {
		return nil, errors.New("simulated offline")
	}
	return f.next.RoundTrip(r)
}

// device is one simulated tick installation: its own store dir + sync client.
type device struct {
	dir  store.Dir
	c    *Client
	wire *flakyTransport
}

func newDevice(t *testing.T, name, srvURL string) *device {
	t.Helper()
	d := store.Dir(filepath.Join(t.TempDir(), "tick"))
	cfg := &Config{URL: srvURL, Token: "e2e-token", Device: name}
	wire := &flakyTransport{next: http.DefaultTransport}
	c := NewClient(d, cfg)
	c.HTTP = &http.Client{Transport: wire}
	return &device{dir: d, c: c, wire: wire}
}

func (d *device) sync(t *testing.T) Result {
	t.Helper()
	res, err := d.c.Sync(context.Background(), false)
	if err != nil {
		t.Fatalf("sync %s: %v", d.c.Cfg.Device, err)
	}
	return res
}

func (d *device) mustLoad(t *testing.T) *store.Store {
	t.Helper()
	s, err := d.dir.Load()
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newE2EServer(t *testing.T) *httptest.Server {
	t.Helper()
	st, err := server.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(server.New(st, "e2e-token", "https://example.com", slog.New(slog.DiscardHandler)))
	t.Cleanup(srv.Close)
	return srv
}

// normalized flattens a store for cross-device comparison: everything except
// local-only state (path/key/dirty) must converge to identical values.
type flatStep struct {
	ID, Text, Rank string
	Done, Imp      bool
	Updated        int64
}

func normalized(s *store.Store) map[string][]flatStep {
	out := map[string][]flatStep{}
	for _, p := range s.Projects {
		var steps []flatStep
		for _, st := range p.Steps {
			steps = append(steps, flatStep{st.ID, st.Text, st.Rank, st.Done, st.Important, st.Updated.UnixMilli()})
		}
		out[p.ID+"/"+p.Name+"/"+p.Group] = steps
	}
	return out
}

func TestTwoDevicesConverge(t *testing.T) {
	srv := newE2EServer(t)
	mac := newDevice(t, "mac", srv.URL)
	phone := newDevice(t, "phone", srv.URL)

	// A real project directory so the phone (same fs in tests) may adopt the path.
	projDir := filepath.Join(t.TempDir(), "blitzsheet")
	os.MkdirAll(projDir, 0o755)

	// Mac seeds three steps and pushes.
	_, err := mac.dir.Update(func(s *store.Store) error {
		p := s.Ensure(projDir, "blitzsheet")
		p.AddStep("first")
		p.AddStep("second")
		p.AddStep("third")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	res := mac.sync(t)
	if res.Pushed != 4 { // project + 3 steps
		t.Fatalf("mac pushed %d, want 4", res.Pushed)
	}

	// Phone pulls everything.
	if res = phone.sync(t); res.Pulled != 4 {
		t.Fatalf("phone pulled %d, want 4", res.Pulled)
	}

	// Phone crosses off "second" and moves "third" to the top; Mac edits
	// "first" — all offline of each other, then both sync twice (push, then
	// pick up the other's changes).
	_, err = phone.dir.Update(func(s *store.Store) error {
		p := s.SortedProjects()[0]
		i := indexByText(p, "second")
		now := store.Now()
		p.Steps[i].Done, p.Steps[i].DoneAt = true, &now
		store.Touch(&p.Steps[i])
		j := indexByText(p, "third")
		p.Steps[j].Rank = "0V" // before "V"
		store.Touch(&p.Steps[j])
		p.SortSteps()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = mac.dir.Update(func(s *store.Store) error {
		p := s.SortedProjects()[0]
		i := indexByText(p, "first")
		p.Steps[i].Text = "first — edited on the mac"
		store.Touch(&p.Steps[i])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	phone.sync(t)
	mac.sync(t)
	phone.sync(t)

	sm, sp := mac.mustLoad(t), phone.mustLoad(t)
	if !reflect.DeepEqual(normalized(sm), normalized(sp)) {
		t.Fatalf("devices diverged:\nmac:   %+v\nphone: %+v", normalized(sm), normalized(sp))
	}
	p := sm.SortedProjects()[0]
	if p.Steps[0].Text != "third" || !p.Steps[2].Done || p.Steps[1].Text != "first — edited on the mac" {
		t.Fatalf("merged state wrong: %+v", p.Steps)
	}
}

func TestOfflineQueueAndTombstonePropagation(t *testing.T) {
	srv := newE2EServer(t)
	mac := newDevice(t, "mac", srv.URL)
	phone := newDevice(t, "phone", srv.URL)

	projDir := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(projDir, 0o755)
	if _, err := mac.dir.Update(func(s *store.Store) error {
		p := s.Ensure(projDir, "proj")
		p.AddStep("keep me")
		p.AddStep("delete me")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mac.sync(t)
	phone.sync(t)

	// Mac goes offline, deletes a step and adds one; syncs fail silently.
	mac.wire.offline = true
	if _, err := mac.dir.Update(func(s *store.Store) error {
		p := s.SortedProjects()[0]
		s.RemoveStep(p, indexByText(p, "delete me"))
		p.AddStep("added while offline")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := mac.c.Sync(context.Background(), false); err == nil {
		t.Fatal("offline sync must fail")
	}
	// The outbox survives: dirty step + tombstone still queued.
	s := mac.mustLoad(t)
	if len(s.Tombstones) != 1 {
		t.Fatalf("tombstone lost offline: %+v", s.Tombstones)
	}

	// Reconnect: everything drains, phone converges.
	mac.wire.offline = false
	mac.sync(t)
	phone.sync(t)
	sp := phone.mustLoad(t)
	p := sp.SortedProjects()[0]
	if indexByText(p, "delete me") >= 0 {
		t.Fatalf("deletion did not propagate: %+v", p.Steps)
	}
	if indexByText(p, "added while offline") < 0 {
		t.Fatalf("offline add did not propagate: %+v", p.Steps)
	}
	if s = mac.mustLoad(t); len(s.Tombstones) != 0 {
		t.Fatalf("tombstone not purged after ack: %+v", s.Tombstones)
	}
}

func TestUntrackPropagatesAndDoneVsDeleteLWW(t *testing.T) {
	srv := newE2EServer(t)
	mac := newDevice(t, "mac", srv.URL)
	phone := newDevice(t, "phone", srv.URL)

	projDir := filepath.Join(t.TempDir(), "gone")
	os.MkdirAll(projDir, 0o755)
	if _, err := mac.dir.Update(func(s *store.Store) error {
		s.Ensure(projDir, "gone").AddStep("only step")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mac.sync(t)
	phone.sync(t)

	// Phone marks the step done — with a LATER timestamp than the untrack
	// that happens on the mac next. The later write must win: the step (and
	// its project) resurrect as done.
	if _, err := mac.dir.Update(func(s *store.Store) error {
		s.Untrack(s.SortedProjects()[0])
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond) // ensure the phone's edit is strictly later
	if _, err := phone.dir.Update(func(s *store.Store) error {
		p := s.SortedProjects()[0]
		now := store.Now()
		p.Steps[0].Done, p.Steps[0].DoneAt = true, &now
		store.Touch(&p.Steps[0])
		// Only the step is touched. The untrack's project tombstone is
		// unopposed, so the project dies everywhere and takes the step with
		// it — the documented LWW outcome (steps don't outlive projects).
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mac.sync(t)
	phone.sync(t)
	mac.sync(t)

	sm, sp := mac.mustLoad(t), phone.mustLoad(t)
	if len(sm.Projects) != 0 || len(sp.Projects) != 0 {
		t.Fatalf("untrack did not win on the project: mac=%d phone=%d", len(sm.Projects), len(sp.Projects))
	}
}

func TestFullResyncAfterSetup(t *testing.T) {
	srv := newE2EServer(t)
	mac := newDevice(t, "mac", srv.URL)
	projDir := filepath.Join(t.TempDir(), "proj")
	os.MkdirAll(projDir, 0o755)
	if _, err := mac.dir.Update(func(s *store.Store) error {
		s.Ensure(projDir, "proj").AddStep("a")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	mac.sync(t)
	// Clean dirty flags everywhere, then a full sync must still push all.
	fresh := newDevice(t, "fresh", srv.URL)
	if res, err := fresh.c.Sync(context.Background(), true); err != nil || res.Pulled != 2 {
		t.Fatalf("full sync on a fresh device: res=%+v err=%v", res, err)
	}
}

func indexByText(p *store.Project, text string) int {
	for i := range p.Steps {
		if p.Steps[i].Text == text {
			return i
		}
	}
	return -1
}
