package tui

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestStampRowPlacesGlyphs(t *testing.T) {
	got := ansi.Strip(stampRow("hello", []stamp{{0, "X"}}))
	if got != "Xello" {
		t.Fatalf("stamp at 0 = %q, want %q", got, "Xello")
	}
	got = ansi.Strip(stampRow("hello", []stamp{{2, "X"}, {4, "Y"}}))
	if got != "heXlY" {
		t.Fatalf("stamp at 2,4 = %q, want %q", got, "heXlY")
	}
}

func TestStampRowPastEndPads(t *testing.T) {
	got := ansi.Strip(stampRow("hi", []stamp{{5, "X"}}))
	if got != "hi   X" {
		t.Fatalf("stamp past end (stripped) = %q, want %q", got, "hi   X")
	}
}

func TestStampRowPreservesWidthOnStyledLine(t *testing.T) {
	line := lipgloss.NewStyle().Foreground(lipgloss.Color("#29cdff")).Render("hello world")
	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#ff718d")).Render("X")
	got := stampRow(line, []stamp{{6, g}})
	// The visible width must be unchanged (particles never shift the layout).
	if ansi.StringWidth(got) != 11 {
		t.Fatalf("width changed: got %d, want 11", ansi.StringWidth(got))
	}
}

// TestStampRowDoesNotBloat is the regression guard for the exponential-growth
// hang: stamping many glyphs onto a heavily per-character-styled line must
// rebuild from the original each time and stay bounded in size.
func TestStampRowDoesNotBloat(t *testing.T) {
	struck := lipgloss.NewStyle().Faint(true).Strikethrough(true).Render("ship the beta done")
	base := len(struck)
	stamps := make([]stamp, 0, 40)
	g := lipgloss.NewStyle().Foreground(lipgloss.Color("#78ff44")).Render("█")
	for i := 0; i < 40; i++ {
		stamps = append(stamps, stamp{x: i % 12, g: g})
	}
	out := stampRow(struck, stamps)
	if len(out) > base*20 {
		t.Fatalf("stampRow output bloated: %d bytes from a %d-byte line", len(out), base)
	}
}

func TestBurstIsBoundedAndVisible(t *testing.T) {
	// Origin near the TOP of the screen (row 2), like a real steps list. This is
	// the exact case that must not stall the UI.
	s := &system{w: 100, h: 30}
	s.burst(12, 2)
	if !s.active() {
		t.Fatal("expected particles after burst")
	}

	rendered := false
	frames := 0
	for s.active() {
		// Count frames where at least one particle is visible on screen.
		for _, p := range s.particles {
			pos := p.physics.Position()
			if pos.Y >= 0 && pos.Y < float64(s.h) && pos.X >= 0 && pos.X < float64(s.w) {
				rendered = true
				break
			}
		}
		s.step()
		frames++
		if frames > maxParticleAge+2 {
			t.Fatalf("burst ran %d frames — exceeds the %d-frame cap; UI would stall", frames, maxParticleAge)
		}
	}
	if !rendered {
		t.Fatal("confetti never appeared on screen — burst flew off and stayed invisible")
	}
}
