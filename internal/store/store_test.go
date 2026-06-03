package store

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
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
