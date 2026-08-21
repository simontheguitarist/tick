package rank

import (
	"math/rand"
	"sort"
	"testing"
)

// golden pins Mid for the exact vectors shared with the Swift port
// (docs/protocol.md and ios/TickTests/Fixtures/rank_vectors.json).
func TestMidGoldenVectors(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "", "V"},
		{"", "V", "F"},
		{"V", "", "k"},
		{"V", "W", "VV"},
		{"V", "VV", "VF"},
		{"", "1", "0V"},
		{"A", "B5", "B"},
		{"B", "B05", "B02"},
		{"03", "05", "04"},
		{"V", "V5", "V2"},
		{"Az", "B", "AzV"},
		{"zz", "", "zzV"},
	}
	for _, c := range cases {
		if got := Mid(c.a, c.b); got != c.want {
			t.Errorf("Mid(%q,%q) = %q, want %q", c.a, c.b, got, c.want)
		}
	}
}

func TestAfterBefore(t *testing.T) {
	if After("") != "V" || Before("") != "V" {
		t.Fatal("empty-list anchors changed")
	}
	if got := After("V"); got != "W" {
		t.Errorf("After(V) = %q", got)
	}
	if got := After("Az"); got != "AzV" {
		t.Errorf("After(Az) = %q", got)
	}
	if got := Before("V"); got != "U" {
		t.Errorf("Before(V) = %q", got)
	}
	if got := Before("1"); got != "0V" {
		t.Errorf("Before(1) = %q", got)
	}
}

func TestInitialSortedAndValid(t *testing.T) {
	for _, n := range []int{1, 2, 5, 63, 500, 5000} {
		prev := ""
		for i := 0; i < n; i++ {
			r := Initial(i, n)
			if !IsValid(r) {
				t.Fatalf("Initial(%d,%d) = %q invalid", i, n, r)
			}
			if prev != "" && r <= prev {
				t.Fatalf("Initial not increasing at %d/%d: %q <= %q", i, n, r, prev)
			}
			prev = r
		}
	}
}

// TestPropertyRandomOps hammers Mid/After/Before with 10k random insertions
// and asserts strict order plus validity throughout.
func TestPropertyRandomOps(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	keys := []string{}
	for i := 0; i < 10000; i++ {
		pos := rng.Intn(len(keys) + 1)
		var k string
		switch {
		case len(keys) == 0:
			k = Mid("", "")
		case pos == 0:
			k = Before(keys[0])
		case pos == len(keys):
			k = After(keys[len(keys)-1])
		default:
			k = Mid(keys[pos-1], keys[pos])
		}
		if !IsValid(k) {
			t.Fatalf("op %d: invalid key %q", i, k)
		}
		keys = append(keys[:pos], append([]string{k}, keys[pos:]...)...)
		if !sort.StringsAreSorted(keys) {
			t.Fatalf("op %d: order broken around %q", i, k)
		}
		for j := 1; j < len(keys); j++ {
			if keys[j] == keys[j-1] {
				t.Fatalf("op %d: duplicate key %q", i, k)
			}
		}
	}
}

func TestMidDegradesOnBadInput(t *testing.T) {
	// a >= b is a caller bug; Mid must still return a valid key.
	if got := Mid("W", "V"); !IsValid(got) {
		t.Fatalf("degraded key invalid: %q", got)
	}
}
