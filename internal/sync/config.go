// Package sync connects the local store to a tickd server: it builds the
// outbox from dirty entities and tombstones, runs the single-request exchange,
// and merges the response back under the store lock. Network I/O always
// happens OUTSIDE the store lock; only the merge runs inside it.
package sync

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/simontheguitarist/tick/internal/store"
)

// Config is ~/.config/tick/sync.json — kept separate from store.json so the
// token lives in its own 0600 file (external tools read store.json) and
// `tick sync off` is a single file deletion. The pull cursor is NOT here: it
// sits inside store.json so cursor and data commit in one atomic rename.
type Config struct {
	URL          string    `json:"url"`
	Token        string    `json:"token"`
	Device       string    `json:"device"`
	LastOK       time.Time `json:"last_ok,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	BackoffUntil time.Time `json:"backoff_until,omitempty"` // auto-sync stays quiet until then
}

func configPath(d store.Dir) string { return filepath.Join(string(d), "sync.json") }

// LoadConfig returns (nil, nil) when sync was never set up.
func LoadConfig(d store.Dir) (*Config, error) {
	data, err := os.ReadFile(configPath(d))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.URL == "" || c.Token == "" || c.Device == "" {
		return nil, errors.New("sync.json is incomplete — run `tick sync setup <url>` again")
	}
	return &c, nil
}

// Save writes the config with owner-only permissions, atomically.
func (c *Config) Save(d store.Dir) error {
	if err := os.MkdirAll(string(d), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := configPath(d) + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, configPath(d))
}

// Off removes the config; local data stays untouched.
func Off(d store.Dir) error {
	err := os.Remove(configPath(d))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
