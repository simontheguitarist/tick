package cli

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/simontheguitarist/tick/internal/store"
	tsync "github.com/simontheguitarist/tick/internal/sync"
)

const syncTimeout = 15 * time.Second // explicit `tick sync` may take its time

// SyncRun executes an explicit sync and reports what moved.
func SyncRun(full bool) error {
	d, cfg, err := requireSync()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	res, err := tsync.NewClient(d, cfg).Sync(ctx, full)
	tsync.Record(d, cfg, err)
	if err != nil {
		return err
	}
	fmt.Printf("synced with %s — pushed %d, pulled %d\n", cfg.URL, res.Pushed, res.Pulled)
	return nil
}

// SyncSetup connects this machine to a server: it validates the token with a
// first full sync (uploading the whole local store) and only then saves the
// config. The token is prompted, never a CLI argument — arguments land in
// shell history.
func SyncSetup(rawURL string) error {
	u, err := normalizeURL(rawURL)
	if err != nil {
		return err
	}
	token, err := readToken()
	if err != nil {
		return err
	}
	d, err := store.DefaultDir()
	if err != nil {
		return err
	}
	host, _ := os.Hostname()
	host = strings.ToLower(strings.SplitN(host, ".", 2)[0])
	if host == "" {
		host = "device"
	}
	cfg := &tsync.Config{URL: u, Token: token, Device: host + "-" + store.NewID()[:8]}

	ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
	defer cancel()
	res, err := tsync.NewClient(d, cfg).Sync(ctx, true)
	if err != nil {
		return fmt.Errorf("setup aborted, nothing saved: %w", err)
	}
	tsync.Record(d, cfg, nil)
	fmt.Printf("connected to %s as %s — pushed %d, pulled %d\n", u, cfg.Device, res.Pushed, res.Pulled)
	fmt.Println("pair your phone with:  tick sync qr")
	return nil
}

// SyncStatus prints the connection, the last outcome and what's still queued.
func SyncStatus() error {
	d, err := store.DefaultDir()
	if err != nil {
		return err
	}
	cfg, err := tsync.LoadConfig(d)
	if err != nil {
		return err
	}
	if cfg == nil {
		fmt.Println("sync is off — set it up with `tick sync setup <url>`")
		return nil
	}
	s, err := d.Load()
	if err != nil {
		return err
	}
	dirty := len(s.Tombstones)
	for _, p := range s.Projects {
		if p.Dirty {
			dirty++
		}
		for i := range p.Steps {
			if p.Steps[i].Dirty {
				dirty++
			}
		}
	}
	fmt.Printf("server   %s\n", cfg.URL)
	fmt.Printf("device   %s\n", cfg.Device)
	if !cfg.LastOK.IsZero() {
		fmt.Printf("last ok  %s\n", cfg.LastOK.Format("2006-01-02 15:04:05"))
	}
	if cfg.LastError != "" {
		fmt.Printf("problem  %s\n", cfg.LastError)
	}
	if time.Now().Before(cfg.BackoffUntil) {
		fmt.Printf("paused   auto-sync resumes %s\n", cfg.BackoffUntil.Format("15:04:05"))
	}
	fmt.Printf("queued   %d change(s) waiting to push\n", dirty)
	fmt.Printf("cursor   %d\n", s.Sync.Since)
	return nil
}

// SyncOff forgets the server and token; local data stays.
func SyncOff() error {
	d, err := store.DefaultDir()
	if err != nil {
		return err
	}
	if err := tsync.Off(d); err != nil {
		return err
	}
	fmt.Println("sync is off — your steps stay on this machine")
	return nil
}

// SyncQR renders the pairing QR (server URL + token) for the iPhone app.
func SyncQR() error {
	_, cfg, err := requireSync()
	if err != nil {
		return err
	}
	payload := "tick://pair?url=" + url.QueryEscape(cfg.URL) + "&token=" + url.QueryEscape(cfg.Token)
	fmt.Println("Scan with the Tick iPhone app (or your camera). The code contains")
	fmt.Println("your sync token — share the screen like you'd share the token.")
	fmt.Println()
	art, err := qrArt(payload)
	if err != nil {
		return err
	}
	fmt.Print(art)
	fmt.Println()
	fmt.Println("no camera handy? paste this into the app instead:")
	fmt.Println("  " + payload)
	return nil
}

func requireSync() (store.Dir, *tsync.Config, error) {
	d, err := store.DefaultDir()
	if err != nil {
		return "", nil, err
	}
	cfg, err := tsync.LoadConfig(d)
	if err != nil {
		return "", nil, err
	}
	if cfg == nil {
		return "", nil, fmt.Errorf("sync is not set up — run `tick sync setup <url>` first")
	}
	return d, cfg, nil
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("usage: tick sync setup <url>")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return "", fmt.Errorf("that doesn't look like a server URL: %q", raw)
	}
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String(), nil
}

// readToken takes the token from $TICK_TOKEN (for scripts) or prompts without
// echo on a terminal, falling back to a plain stdin line when piped.
func readToken() (string, error) {
	if t := strings.TrimSpace(os.Getenv("TICK_TOKEN")); t != "" {
		return t, nil
	}
	fmt.Fprint(os.Stderr, "token (from the server's TICK_TOKEN): ")
	if term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, nil
		}
		return "", fmt.Errorf("no token entered")
	}
	sc := bufio.NewScanner(os.Stdin)
	if !sc.Scan() {
		return "", fmt.Errorf("no token on stdin")
	}
	if t := strings.TrimSpace(sc.Text()); t != "" {
		return t, nil
	}
	return "", fmt.Errorf("no token entered")
}
