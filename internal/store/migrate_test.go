package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/simontheguitarist/tick/internal/rank"
)

// seedV1 copies the v1 fixture into a fresh temp store dir.
func seedV1(t *testing.T) string {
	t.Helper()
	cfg := useTempStore(t)
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "store.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestMigrateV1(t *testing.T) {
	cfg := seedV1(t)

	s, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Projects) != 3 {
		t.Fatalf("projects = %d, want 3", len(s.Projects))
	}
	bs := s.Projects["/Users/skn/Documents/1_Projects/blitzsheet"]
	if bs == nil || bs.ID == "" || bs.Path != "/Users/skn/Documents/1_Projects/blitzsheet" {
		t.Fatalf("blitzsheet project mangled: %+v", bs)
	}
	if len(bs.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(bs.Steps))
	}
	seen := map[string]bool{}
	prevRank := ""
	for i, st := range bs.Steps {
		if st.ID == "" || seen[st.ID] {
			t.Fatalf("step %d: bad or duplicate id %q", i, st.ID)
		}
		seen[st.ID] = true
		if !rank.IsValid(st.Rank) || (prevRank != "" && st.Rank <= prevRank) {
			t.Fatalf("step %d: rank %q not valid/increasing after %q", i, st.Rank, prevRank)
		}
		prevRank = st.Rank
		if st.Updated.IsZero() {
			t.Fatalf("step %d: zero updated", i)
		}
		if st.Dirty {
			t.Fatalf("step %d: migration must not mark dirty", i)
		}
	}
	// Manual order and the done/important flags survive.
	if bs.Steps[0].Text != "fix langfuse not recording (root) - also switch to main branch & pull" ||
		!bs.Steps[0].Done || !bs.Steps[1].Important || bs.Steps[2].Done {
		t.Fatalf("step content mangled: %+v", bs.Steps)
	}
	// The zero-Modified project got a real timestamp.
	if fresh := s.Projects["/Users/skn/tmp/fresh"]; fresh == nil || fresh.Updated.IsZero() {
		t.Fatalf("zero-modified project not repaired: %+v", fresh)
	}

	// The migration was persisted (Load write-back) and a backup kept.
	if _, err := os.Stat(filepath.Join(cfg, "store.v1.json")); err != nil {
		t.Fatalf("no v1 backup: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(cfg, "store.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"version": 2`) {
		t.Fatal("migration not persisted as v2")
	}
	// done_at strings must be byte-identical to the v1 file (external readers
	// derive local dates from them).
	for _, ts := range []string{"2026-06-10T12:53:29.391918+02:00", "2026-06-12T08:10:26.170567+02:00"} {
		if !strings.Contains(string(raw), ts) {
			t.Fatalf("done_at %s not preserved byte-identical", ts)
		}
	}
}

func TestMigrationIDsAreStableAcrossLoads(t *testing.T) {
	seedV1(t)
	a, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	pa := a.Projects["/Users/skn/Documents/1_Projects/blitzsheet"]
	pb := b.Projects["/Users/skn/Documents/1_Projects/blitzsheet"]
	if pa.ID != pb.ID || pa.Steps[0].ID != pb.Steps[0].ID {
		t.Fatal("ids changed between loads — migration was not persisted")
	}
}

func TestNewerSchemaRefused(t *testing.T) {
	cfg := useTempStore(t)
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "store.json"), []byte(`{"version": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "newer tick") {
		t.Fatalf("want refusal for newer schema, got %v", err)
	}
}
