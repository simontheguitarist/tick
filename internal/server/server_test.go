package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/simontheguitarist/tick/internal/protocol"
)

const testToken = "sekrit"

func newTestServer(t *testing.T) (*httptest.Server, *Store) {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := New(st, testToken, "https://example.com/landing", slog.New(slog.DiscardHandler))
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

func postSync(t *testing.T, srv *httptest.Server, token string, req protocol.SyncRequest) (*http.Response, protocol.SyncResponse) {
	t.Helper()
	body, _ := json.Marshal(req)
	hr, _ := http.NewRequest("POST", srv.URL+"/v1/sync", bytes.NewReader(body))
	if token != "" {
		hr.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	var out protocol.SyncResponse
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
	}
	res.Body.Close()
	return res, out
}

func step(id, proj, text, rank, updated string) protocol.Step {
	return protocol.Step{ID: id, ProjectID: proj, Text: text, Rank: rank, Created: updated, Updated: updated}
}

func TestAuth(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, tok := range []string{"", "wrong", "sekri"} {
		if res, _ := postSync(t, srv, tok, protocol.SyncRequest{Device: "a"}); res.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q: status %d, want 401", tok, res.StatusCode)
		}
	}
	if res, _ := postSync(t, srv, testToken, protocol.SyncRequest{Device: "a"}); res.StatusCode != http.StatusOK {
		t.Fatalf("valid token refused: %d", res.StatusCode)
	}
}

func TestLandingRedirectAndHealthz(t *testing.T) {
	srv, _ := newTestServer(t)
	c := srv.Client()
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	res, err := c.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "https://example.com/landing" {
		t.Fatalf("landing: %d -> %q", res.StatusCode, res.Header.Get("Location"))
	}
	res, err = c.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %d", res.StatusCode)
	}
}

func TestValidationRejects(t *testing.T) {
	srv, _ := newTestServer(t)
	cases := []protocol.SyncRequest{
		{},                          // no device
		{Device: "a", Steps: []protocol.Step{{ID: "s1", Updated: "2026-01-01T00:00:00.000Z"}}},                                      // no project_id
		{Device: "a", Steps: []protocol.Step{step("s1", "p1", "x", "V", "not-a-time")}},                                             // bad time
		{Device: "a", Steps: []protocol.Step{step("s1", "p1", "x", "V0", "2026-01-01T00:00:00.000Z")}},                              // invalid rank
		{Device: "a", Steps: []protocol.Step{step("s1", "p1", "", "V", "2026-01-01T00:00:00.000Z")}},                                // empty text
		{Device: "a", Steps: []protocol.Step{step("s1", "p1", strings.Repeat("x", 5000), "V", "2026-01-01T00:00:00.000Z")}},         // text too long
		{Device: "a", Projects: []protocol.Project{{ID: "p1", Updated: "2026-01-01T00:00:00.000Z"}}},                                // live project, no name
	}
	for i, req := range cases {
		if res, _ := postSync(t, srv, testToken, req); res.StatusCode != http.StatusBadRequest {
			t.Fatalf("case %d: status %d, want 400", i, res.StatusCode)
		}
	}
	// Tombstones need no text/rank/name.
	req := protocol.SyncRequest{Device: "a",
		Projects: []protocol.Project{{ID: "p1", Deleted: true, Updated: "2026-01-01T00:00:00.000Z"}},
		Steps:    []protocol.Step{{ID: "s1", ProjectID: "p1", Deleted: true, Updated: "2026-01-01T00:00:00.000Z"}}}
	if res, _ := postSync(t, srv, testToken, req); res.StatusCode != http.StatusOK {
		t.Fatalf("tombstone push refused: %d", res.StatusCode)
	}
}

func TestLWWAndRejectedEcho(t *testing.T) {
	srv, _ := newTestServer(t)
	// Device a pushes v2 of a step.
	_, r1 := postSync(t, srv, testToken, protocol.SyncRequest{Device: "a",
		Steps: []protocol.Step{step("s1", "p1", "newer", "V", "2026-01-02T00:00:00.000Z")}})
	if len(r1.Steps) != 1 || r1.Steps[0].Text != "newer" {
		t.Fatalf("push not echoed: %+v", r1)
	}
	// Device b, already at the current cursor, pushes an OLDER version.
	_, r2 := postSync(t, srv, testToken, protocol.SyncRequest{Device: "b", Since: r1.Seq,
		Steps: []protocol.Step{step("s1", "p1", "older", "W", "2026-01-01T00:00:00.000Z")}})
	// The rejected push must come back with the server's winning version even
	// though its seq <= since.
	if len(r2.Steps) != 1 || r2.Steps[0].Text != "newer" {
		t.Fatalf("rejected push not answered with the winner: %+v", r2)
	}
	if r2.Seq != r1.Seq {
		t.Fatalf("rejected push bumped seq: %d -> %d", r1.Seq, r2.Seq)
	}
	// Equal timestamps: device id breaks the tie deterministically (b > a).
	_, r3 := postSync(t, srv, testToken, protocol.SyncRequest{Device: "b", Since: 0,
		Steps: []protocol.Step{step("s1", "p1", "tie-b", "X", "2026-01-02T00:00:00.000Z")}})
	if r3.Steps[len(r3.Steps)-1].Text != "tie-b" {
		t.Fatalf("tie not broken by device id: %+v", r3)
	}
}

func TestIdempotentRepush(t *testing.T) {
	srv, _ := newTestServer(t)
	s := step("s1", "p1", "hello", "V", "2026-01-01T00:00:00.000Z")
	_, r1 := postSync(t, srv, testToken, protocol.SyncRequest{Device: "a", Steps: []protocol.Step{s}})
	_, r2 := postSync(t, srv, testToken, protocol.SyncRequest{Device: "a", Steps: []protocol.Step{s}})
	if r2.Seq != r1.Seq {
		t.Fatalf("identical re-push bumped seq: %d -> %d", r1.Seq, r2.Seq)
	}
}

func TestOffsetTimesAreCanonicalized(t *testing.T) {
	srv, _ := newTestServer(t)
	// The CLI's local zone is +02:00; on the wire the client sends UTC, but the
	// server must survive a lenient client too.
	s := step("s1", "p1", "local", "V", "2026-06-10T12:53:29.391918+02:00")
	_, r := postSync(t, srv, testToken, protocol.SyncRequest{Device: "a", Steps: []protocol.Step{s}})
	if got := r.Steps[0].Updated; got != "2026-06-10T10:53:29.391Z" {
		t.Fatalf("not canonicalized: %q", got)
	}
}

func TestConcurrentApplies(t *testing.T) {
	srv, st := newTestServer(t)
	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("s%02d", i)
			postSync(t, srv, testToken, protocol.SyncRequest{Device: "a",
				Steps: []protocol.Step{step(id, "p1", "t"+id, "V", "2026-01-01T00:00:00.000Z")}})
		}(i)
	}
	wg.Wait()
	st.mu.Lock()
	defer st.mu.Unlock()
	if len(st.doc.Steps) != n || st.doc.Seq != n {
		t.Fatalf("steps=%d seq=%d, want %d/%d", len(st.doc.Steps), st.doc.Seq, n, n)
	}
}

func TestRestartReloads(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Apply(protocol.SyncRequest{Device: "a",
		Steps: []protocol.Step{step("s1", "p1", "persist me", "V", "2026-01-01T00:00:00.000Z")}}); err != nil {
		t.Fatal(err)
	}
	st.Close()

	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	resp, err := st2.Apply(protocol.SyncRequest{Device: "b", Since: 0})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Steps) != 1 || resp.Steps[0].Text != "persist me" {
		t.Fatalf("data lost across restart: %+v", resp)
	}
}

func TestDoubleOpenRefused(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if _, err := Open(dir); err == nil {
		t.Fatal("second Open on the same dir must fail")
	}
}

func TestBodyLimit(t *testing.T) {
	srv, _ := newTestServer(t)
	huge := bytes.Repeat([]byte("x"), maxBody+1)
	hr, _ := http.NewRequest("POST", srv.URL+"/v1/sync", bytes.NewReader(huge))
	hr.Header.Set("Authorization", "Bearer "+testToken)
	res, err := srv.Client().Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("oversized body: %d, want 400", res.StatusCode)
	}
}
