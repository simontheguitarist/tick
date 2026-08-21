// Package server is tickd's HTTP layer: bearer-token auth, request validation
// and the single /v1/sync endpoint, in front of the JSON-document Store.
package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/simontheguitarist/tick/internal/protocol"
	"github.com/simontheguitarist/tick/internal/rank"
)

// Field limits: generous for humans, tight enough that a leaked token can't
// turn the store into a blob dump.
const (
	maxBody  = 4 << 20
	maxID    = 64
	maxText  = 4096
	maxName  = 256
	maxGroup = 128
	maxPath  = 1024
	maxRank  = 128
)

// New assembles the handler. token authenticates /v1/*; landingURL is where a
// browser hitting the bare domain is sent (the GitHub repo now, the App Store
// listing later).
func New(st *Store, token, landingURL string, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, landingURL, http.StatusFound)
	})
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "ok")
	})
	mux.Handle("POST /v1/sync", requireToken(token, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleSync(st, w, r)
	})))
	return logged(log, mux)
}

// requireToken guards a handler with a constant-time bearer-token check.
func requireToken(token string, next http.Handler) http.Handler {
	want := sha256.Sum256([]byte(token))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		h := sha256.Sum256([]byte(got))
		if !ok || subtle.ConstantTimeCompare(h[:], want[:]) != 1 {
			jsonError(w, http.StatusUnauthorized, "missing or wrong token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func handleSync(st *Store, w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	var req protocol.SyncRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonError(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		return
	}
	if req.Device == "" || len(req.Device) > maxID {
		jsonError(w, http.StatusBadRequest, "device id missing or too long")
		return
	}
	if err := validate(&req); err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := st.Apply(req)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, "persist failed: "+err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// validate checks limits and canonicalizes every timestamp in place, so the
// store only ever compares fixed-width UTC strings.
func validate(req *protocol.SyncRequest) error {
	for i := range req.Projects {
		p := &req.Projects[i]
		if p.ID == "" || len(p.ID) > maxID {
			return fmt.Errorf("project %d: bad id", i)
		}
		if !p.Deleted && (p.Name == "" || len(p.Name) > maxName) {
			return fmt.Errorf("project %s: name missing or too long", p.ID)
		}
		if len(p.Group) > maxGroup || len(p.Path) > maxPath {
			return fmt.Errorf("project %s: group or path too long", p.ID)
		}
		var err error
		if p.Updated, err = protocol.Canonical(p.Updated); err != nil {
			return fmt.Errorf("project %s: bad updated: %v", p.ID, err)
		}
	}
	for i := range req.Steps {
		s := &req.Steps[i]
		if s.ID == "" || len(s.ID) > maxID {
			return fmt.Errorf("step %d: bad id", i)
		}
		if s.ProjectID == "" || len(s.ProjectID) > maxID {
			return fmt.Errorf("step %s: bad project_id", s.ID)
		}
		if !s.Deleted {
			if s.Text == "" || len(s.Text) > maxText {
				return fmt.Errorf("step %s: text missing or too long", s.ID)
			}
			if len(s.Rank) > maxRank || !rank.IsValid(s.Rank) {
				return fmt.Errorf("step %s: invalid rank %q", s.ID, s.Rank)
			}
		}
		var err error
		if s.Updated, err = protocol.Canonical(s.Updated); err != nil {
			return fmt.Errorf("step %s: bad updated: %v", s.ID, err)
		}
		for name, v := range map[string]*string{"created": &s.Created, "done_at": &s.DoneAt} {
			if *v == "" {
				continue
			}
			if *v, err = protocol.Canonical(*v); err != nil {
				return fmt.Errorf("step %s: bad %s: %v", s.ID, name, err)
			}
		}
	}
	return nil
}

func jsonError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// logged emits one structured line per request.
func logged(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		log.Info("request",
			"method", r.Method, "path", r.URL.Path, "status", rec.status,
			"dur_ms", time.Since(start).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
