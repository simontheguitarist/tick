// Package protocol defines the wire format shared by the CLI, the server and
// the iOS app: one endpoint, POST /v1/sync. A client sends its locally dirty
// entities plus a pull cursor and receives everything newer than the cursor —
// including the server's current version of anything it pushed that lost
// last-writer-wins, so a device with a slow clock converges instead of
// diverging silently. docs/protocol.md is the human-readable contract.
package protocol

import "time"

// TimeFormat is the canonical wire timestamp: fixed-width UTC with
// milliseconds. Fixed width means lexicographic comparison equals time
// comparison, so the server never has to parse times to run LWW.
const TimeFormat = "2006-01-02T15:04:05.000Z"

// FormatTime renders t in the canonical wire form.
func FormatTime(t time.Time) string { return t.UTC().Truncate(time.Millisecond).Format(TimeFormat) }

// ParseTime accepts any RFC3339 timestamp (offset or Z, 0–9 fraction digits).
func ParseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339Nano, s) }

// Canonical re-renders any parseable timestamp in the canonical wire form.
func Canonical(s string) (string, error) {
	t, err := ParseTime(s)
	if err != nil {
		return "", err
	}
	return FormatTime(t), nil
}

// Project is a project on the wire. A tombstone (Deleted) carries only ID and
// Updated. Path is per-device metadata (a Mac directory); other devices store
// it opaquely and never interpret it.
type Project struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Path    string `json:"path,omitempty"`
	Group   string `json:"group,omitempty"`
	Deleted bool   `json:"deleted,omitempty"`
	Updated string `json:"updated"`
	Seq     int64  `json:"seq,omitempty"` // server-assigned, ignored on push
}

// Step is a step on the wire. A tombstone carries only ID, ProjectID, Updated.
type Step struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Text      string `json:"text,omitempty"`
	Done      bool   `json:"done,omitempty"`
	Important bool   `json:"important,omitempty"`
	Rank      string `json:"rank,omitempty"`
	Created   string `json:"created,omitempty"`
	DoneAt    string `json:"done_at,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
	Updated   string `json:"updated"`
	Seq       int64  `json:"seq,omitempty"`
}

// SyncRequest pushes the device's dirty entities and names its pull cursor.
// Since = 0 requests a full resync.
type SyncRequest struct {
	Device   string    `json:"device"`
	Since    int64     `json:"since"`
	Projects []Project `json:"projects,omitempty"`
	Steps    []Step    `json:"steps,omitempty"`
}

// SyncResponse returns the new cursor plus every entity with seq > Since —
// after the request was applied — and the current version of every pushed
// entity that was rejected as older.
type SyncResponse struct {
	Seq      int64     `json:"seq"`
	Projects []Project `json:"projects,omitempty"`
	Steps    []Step    `json:"steps,omitempty"`
}
