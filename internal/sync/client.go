package sync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/simontheguitarist/tick/internal/protocol"
	"github.com/simontheguitarist/tick/internal/store"
)

// ErrUnauthorized means the server rejected the token — retrying won't help.
var ErrUnauthorized = errors.New("server rejected the token — re-run `tick sync setup`")

// Result summarizes one exchange.
type Result struct {
	Pushed int
	Pulled int
	Seq    int64
}

// Client runs sync exchanges for one store directory.
type Client struct {
	Dir  store.Dir
	Cfg  *Config
	HTTP *http.Client // injectable for tests; timeouts come from the ctx
}

func NewClient(d store.Dir, cfg *Config) *Client {
	return &Client{Dir: d, Cfg: cfg, HTTP: http.DefaultClient}
}

// Sync performs one exchange: snapshot outbox (outside the lock), one POST,
// merge the response (inside the lock). full re-pushes and re-pulls everything.
func (c *Client) Sync(ctx context.Context, full bool) (Result, error) {
	if full {
		if _, err := c.Dir.Update(func(s *store.Store) error {
			MarkAllDirty(s)
			s.Sync.Since = 0
			return nil
		}); err != nil {
			return Result{}, err
		}
	}
	snap, err := c.Dir.Load()
	if err != nil {
		return Result{}, err
	}
	req := BuildRequest(snap, c.Cfg.Device, snap.Sync.Since)

	resp, err := c.post(ctx, req)
	if err != nil {
		return Result{}, err
	}

	res := Result{Pushed: len(req.Projects) + len(req.Steps), Seq: resp.Seq}
	if res.Pushed == 0 && len(resp.Projects)+len(resp.Steps) == 0 && resp.Seq == snap.Sync.Since {
		return res, nil // nothing to ack or apply: skip the locked fsync'd rewrite
	}
	if _, err := c.Dir.Update(func(s *store.Store) error {
		res.Pulled = Apply(s, req, *resp)
		return nil
	}); err != nil {
		return res, err
	}
	return res, nil
}

func (c *Client) post(ctx context.Context, req protocol.SyncRequest) (*protocol.SyncResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Cfg.URL+"/v1/sync", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Authorization", "Bearer "+c.Cfg.Token)
	res, err := c.HTTP.Do(hr)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	default:
		msg, _ := io.ReadAll(io.LimitReader(res.Body, 512))
		return nil, fmt.Errorf("server said %d: %s", res.StatusCode, bytes.TrimSpace(msg))
	}
	var out protocol.SyncResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// autoDeadline is how long an implicit sync may stall a CLI command; failures
// pause auto-sync for autoBackoff so an unreachable server doesn't tax every
// invocation.
const (
	autoDeadline = 1 * time.Second
	autoBackoff  = 30 * time.Second
)

// Auto runs a quick, silent, best-effort sync around a CLI command. It is a
// no-op when sync isn't configured, TICK_NO_SYNC=1, or a recent failure set a
// backoff. Outcome lands in sync.json for `tick sync status` — never on the
// terminal, so offline work stays noise-free.
func Auto() { RunOnce(autoDeadline) }

// RunOnce performs one configured sync with the given deadline and records
// the outcome. ran=false means it didn't even try: sync not set up,
// TICK_NO_SYNC=1, or still backing off after a failure.
func RunOnce(timeout time.Duration) (res Result, ran bool, err error) {
	if os.Getenv("TICK_NO_SYNC") == "1" {
		return Result{}, false, nil
	}
	d, err := store.DefaultDir()
	if err != nil {
		return Result{}, false, err
	}
	cfg, err := LoadConfig(d)
	if cfg == nil || err != nil {
		return Result{}, false, err
	}
	if time.Now().Before(cfg.BackoffUntil) {
		return Result{}, false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	res, err = NewClient(d, cfg).Sync(ctx, false)
	Record(d, cfg, err)
	return res, true, err
}

// Record notes a sync outcome in the config (best effort). Successes are
// written at most once a minute — every CLI command syncs, and a disk write
// whose only payload is a timestamp isn't worth paying each time.
func Record(d store.Dir, cfg *Config, err error) {
	now := time.Now()
	if err != nil {
		cfg.LastError = err.Error()
		cfg.BackoffUntil = now.Add(autoBackoff)
		_ = cfg.Save(d)
		return
	}
	transition := cfg.LastError != "" || !cfg.BackoffUntil.IsZero()
	stale := now.Sub(cfg.LastOK) > time.Minute
	cfg.LastOK, cfg.LastError, cfg.BackoffUntil = now, "", time.Time{}
	if transition || stale {
		_ = cfg.Save(d)
	}
}
