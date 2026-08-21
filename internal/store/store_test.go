package store

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"
)

// useTempStore points the config dir at a fresh temp dir for the test.
func useTempStore(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	return filepath.Join(dir, "tick")
}

func TestUpdateAndLoadRoundTrip(t *testing.T) {
	useTempStore(t)

	_, err := Update(func(s *Store) error {
		p := s.Ensure("/proj/a", "a")
		p.AddStep("first")
		p.AddStep("second")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	p := s.Projects["/proj/a"]
	if p == nil {
		t.Fatal("project not persisted")
	}
	if len(p.Steps) != 2 || p.Steps[0].Text != "first" || p.Steps[1].Text != "second" {
		t.Fatalf("steps not persisted in order: %+v", p.Steps)
	}
	if p.OpenCount() != 2 {
		t.Fatalf("OpenCount = %d, want 2", p.OpenCount())
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	useTempStore(t)
	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if s.Projects == nil || len(s.Projects) != 0 {
		t.Fatalf("expected empty store, got %+v", s)
	}
}

func TestSaveIsAtomicLeavesNoTemp(t *testing.T) {
	cfg := useTempStore(t)
	if _, err := Update(func(s *Store) error { s.Ensure("/x", "x").AddStep("a"); return nil }); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}
}

func TestConcurrentUpdatesDoNotLoseWrites(t *testing.T) {
	useTempStore(t)
	const n = 25
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			_, err := Update(func(s *Store) error {
				s.Ensure("/proj", "proj").AddStep("step-" + strconv.Itoa(i))
				return nil
			})
			if err != nil {
				t.Errorf("update %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := len(s.Projects["/proj"].Steps); got != n {
		t.Fatalf("lost writes: got %d steps, want %d", got, n)
	}
}

func TestRestoreStepOutranksItsTombstone(t *testing.T) {
	useTempStore(t)
	s := &Store{Version: 2, Projects: map[string]*Project{}}
	p := s.Ensure("/proj", "proj")
	st := p.AddStep("x")
	// A stamp from the future (phone clock ahead) — the classic trap.
	st.Updated = Now().Add(5 * time.Second)
	removed := s.RemoveStep(p, 0)
	tomb := s.Tombstones[0].Updated
	if !removed.Updated.Equal(tomb) {
		t.Fatalf("returned copy must carry the tombstone stamp")
	}
	restored := s.RestoreStep(p, removed)
	if !restored.Updated.After(tomb) {
		t.Fatalf("restored stamp %v must be after tombstone %v", restored.Updated, tomb)
	}
	if len(s.Tombstones) != 0 {
		t.Fatal("pending tombstone not dropped by restore")
	}
}

func TestRestoreStepKeepsNewerLiveCopy(t *testing.T) {
	useTempStore(t)
	s := &Store{Version: 2, Projects: map[string]*Project{}}
	p := s.Ensure("/proj", "proj")
	p.AddStep("original")
	removed := s.RemoveStep(p, 0)
	// Meanwhile a newer version came back from another device.
	live := removed
	live.Text = "edited elsewhere"
	live.Updated = removed.Updated.Add(time.Hour)
	p.Steps = append(p.Steps, live)
	s.RestoreStep(p, removed)
	if len(p.Steps) != 1 || p.Steps[0].Text != "edited elsewhere" {
		t.Fatalf("undo must not duplicate or overwrite a resurrected step: %+v", p.Steps)
	}
}

func TestUntrackStampsEachStepFromItsOwnClock(t *testing.T) {
	useTempStore(t)
	s := &Store{Version: 2, Projects: map[string]*Project{}}
	p := s.Ensure("/proj", "proj")
	a := p.AddStep("a")
	a.Updated = Now().Add(10 * time.Second) // edited on a device whose clock is ahead
	s.Untrack(p)
	for _, tb := range s.Tombstones {
		if tb.Kind == "step" && tb.ID == a.ID && !tb.Updated.After(Now().Add(10*time.Second)) {
			t.Fatalf("step tombstone %v must outrank the step's own stamp", tb.Updated)
		}
	}
}

func TestRebalanceOnDuplicateRanks(t *testing.T) {
	useTempStore(t)
	s := &Store{Version: 2, Projects: map[string]*Project{}}
	p := s.Ensure("/proj", "proj")
	p.AddStep("a")
	p.AddStep("b")
	p.Steps[1].Rank = p.Steps[0].Rank // two devices appended offline
	p.SortSteps()                     // ties order by id — whatever that yields is the order to keep
	before := []string{p.Steps[0].Text, p.Steps[1].Text}
	if !p.NeedsRebalance() {
		t.Fatal("duplicate ranks must trigger a rebalance")
	}
	p.Rebalance()
	if p.Steps[0].Rank >= p.Steps[1].Rank || p.Steps[0].Text != before[0] || p.Steps[1].Text != before[1] {
		t.Fatalf("rebalance must separate ranks and keep the (rank,id) order %v: %+v", before, p.Steps)
	}
}

func TestAddStepClampsText(t *testing.T) {
	useTempStore(t)
	s := &Store{Version: 2, Projects: map[string]*Project{}}
	p := s.Ensure("/proj", "proj")
	st := p.AddStep(strings.Repeat("é", 5000))
	if len(st.Text) > 4096 || !utf8.ValidString(st.Text) {
		t.Fatalf("text not clamped on a rune boundary: %d bytes", len(st.Text))
	}
}
