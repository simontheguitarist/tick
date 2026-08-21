// Command tickd is tick's sync server: one binary, one JSON document on one
// volume, one bearer token. See docs/protocol.md for the wire contract.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/simontheguitarist/tick/internal/server"
)

var version = "dev"

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	token := os.Getenv("TICK_TOKEN")
	if token == "" {
		// No default and no auto-generation: an unauthenticated sync server
		// is an open door, and a generated secret buried in logs gets lost.
		log.Error("TICK_TOKEN is not set — generate one with `openssl rand -hex 32` and set it")
		os.Exit(1)
	}
	dataDir := envOr("TICK_DATA_DIR", "/data")
	addr := envOr("TICK_ADDR", ":8080")
	landing := envOr("TICK_LANDING_URL", "https://github.com/simontheguitarist/tick")

	st, err := server.Open(dataDir)
	if err != nil {
		log.Error("open store", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	srv := &http.Server{
		Addr:              addr,
		Handler:           server.New(st, token, landing, log),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
		close(done)
	}()

	log.Info("tickd listening", "addr", addr, "version", version, "data", dataDir)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
	<-done
	log.Info("tickd stopped")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
