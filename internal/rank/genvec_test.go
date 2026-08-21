package rank

// TestGenerateVectors regenerates the golden-vector fixture shared with the
// Swift port when RANK_GENVEC names an output path. Normal test runs skip it.

import (
	"encoding/json"
	"math/rand"
	"os"
	"testing"
)

func TestGenerateVectors(t *testing.T) {
	out := os.Getenv("RANK_GENVEC")
	if out == "" {
		t.Skip("set RANK_GENVEC=<path> to (re)generate the shared fixture")
	}
	type midV struct{ A, B, Want string }
	type afterV struct{ Last, Want string }
	type beforeV struct{ First, Want string }
	type initV struct {
		I, N int
		Want string
	}
	vectors := struct {
		Mid     []midV    `json:"mid"`
		After   []afterV  `json:"after"`
		Before  []beforeV `json:"before"`
		Initial []initV   `json:"initial"`
	}{}
	for _, c := range [][2]string{{"", ""}, {"", "V"}, {"V", ""}, {"V", "W"}, {"V", "VV"},
		{"", "1"}, {"A", "B5"}, {"B", "B05"}, {"03", "05"}, {"V", "V5"}, {"Az", "B"}, {"zz", ""}} {
		vectors.Mid = append(vectors.Mid, midV{c[0], c[1], Mid(c[0], c[1])})
	}
	// A deterministic random walk pins hundreds of interior cases.
	rng := rand.New(rand.NewSource(42))
	keys := []string{}
	for i := 0; i < 300; i++ {
		pos := rng.Intn(len(keys) + 1)
		var a, b string
		if pos > 0 {
			a = keys[pos-1]
		}
		if pos < len(keys) {
			b = keys[pos]
		}
		k := Mid(a, b)
		vectors.Mid = append(vectors.Mid, midV{a, b, k})
		keys = append(keys[:pos], append([]string{k}, keys[pos:]...)...)
	}
	for _, l := range []string{"", "V", "W", "Az", "z", "zz", "A1", "0V"} {
		vectors.After = append(vectors.After, afterV{l, After(l)})
	}
	for _, f := range []string{"", "V", "1", "2", "0V", "AB"} {
		vectors.Before = append(vectors.Before, beforeV{f, Before(f)})
	}
	for _, n := range []int{1, 2, 3, 10, 63} {
		for i := 0; i < n; i++ {
			vectors.Initial = append(vectors.Initial, initV{i, n, Initial(i, n)})
		}
	}
	f, err := os.Create(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", " ")
	if err := enc.Encode(vectors); err != nil {
		t.Fatal(err)
	}
}
