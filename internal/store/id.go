package store

import (
	"crypto/rand"
	"fmt"
	"time"
)

// NewID returns a lowercase canonical UUIDv4. The stdlib has no UUID type and
// this is the whole of what we need from one, so no dependency.
func NewID() string {
	var b [16]byte
	rand.Read(b[:]) // cannot fail (crypto/rand panics instead, per Go 1.24+)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// Now is the store's clock: local time truncated to milliseconds, matching the
// wire precision so a value survives a round-trip through the server unchanged.
func Now() time.Time { return time.Now().Truncate(time.Millisecond) }

// Bump returns a timestamp strictly after prev — normally just Now, but nudged
// forward by 1ms when the clock hasn't advanced (or moved backwards), so
// last-writer-wins can never see two edits of one entity with equal times from
// the same device.
func Bump(prev time.Time) time.Time {
	if now := Now(); now.After(prev) {
		return now
	}
	return prev.Add(time.Millisecond)
}
