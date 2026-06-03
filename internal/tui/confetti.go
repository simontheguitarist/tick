package tui

import (
	"image/color"
	"math/rand"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/harmonica"
	"github.com/charmbracelet/x/ansi"
)

const framesPerSec = 30

// frameMsg drives the animation clock.
type frameMsg struct{}

// animate schedules the next frame. We only keep returning it while particles
// are alive, so the program goes idle (zero CPU) once a burst finishes.
func animate() tea.Cmd {
	return tea.Tick(time.Second/framesPerSec, func(time.Time) tea.Msg { return frameMsg{} })
}

var confettiColors = []color.Color{
	lipgloss.Color("#a864fd"),
	lipgloss.Color("#29cdff"),
	lipgloss.Color("#78ff44"),
	lipgloss.Color("#ff718d"),
	lipgloss.Color("#fdff6a"),
	lipgloss.Color("#ffa94d"),
}

var confettiGlyphs = []string{"█", "▓", "▒", "░", "▀", "▄", "●", "◆", "✦", "✶", "*"}

type particle struct {
	glyph   string
	color   color.Color
	physics *harmonica.Projectile
	age     int
}

// maxParticleAge bounds the burst to ~1s at 30fps. A hard cap matters: steps sit
// near the top of the screen, so without it any upward particle would arc off
// the top and stay "alive" (invisible) for seconds, stalling the UI.
const maxParticleAge = 30

// system is a little particle simulation rendered over the TUI.
type system struct {
	particles []*particle
	w, h      int
}

func (s *system) active() bool { return len(s.particles) > 0 }

// burst spawns a celebratory pop centered at (x, y) in screen cells. Velocity is
// biased outward and downward (with just a touch of upward "pop") so the confetti
// rains down over the list — staying on screen, since there's little room above.
func (s *system) burst(x, y float64) {
	const n = 60
	for i := 0; i < n; i++ {
		vx := (rand.Float64() - 0.5) * 30 // wide horizontal spread
		vy := rand.Float64()*16 - 5       // -5..+11: small pop up, mostly raining down
		s.particles = append(s.particles, &particle{
			glyph: confettiGlyphs[rand.Intn(len(confettiGlyphs))],
			color: confettiColors[rand.Intn(len(confettiColors))],
			physics: harmonica.NewProjectile(
				harmonica.FPS(framesPerSec),
				harmonica.Point{X: x, Y: y},
				harmonica.Vector{X: vx, Y: vy},
				harmonica.TerminalGravity,
			),
		})
	}
}

// step advances the simulation one frame, dropping particles that age out or
// leave the screen. The age cap guarantees the burst always finishes promptly.
func (s *system) step() {
	alive := s.particles[:0]
	for _, p := range s.particles {
		p.age++
		pos := p.physics.Position()
		if p.age > maxParticleAge || pos.Y > float64(s.h)+1 || pos.X < -2 || pos.X > float64(s.w)+2 {
			continue
		}
		p.physics.Update()
		alive = append(alive, p)
	}
	s.particles = alive
}

type stamp struct {
	x int
	g string // an already-styled, single-cell glyph
}

// overlay composites the live particles onto the rendered screen lines. Glyphs
// are grouped by row and each affected row is rebuilt EXACTLY ONCE from the
// original line — never feeding a modified line back in. That matters: on a
// per-character-styled line (e.g. the struck "✓ done" row), repeatedly cutting
// and rejoining re-emits ANSI state and grows the string exponentially.
func (s *system) overlay(lines []string) []string {
	byRow := map[int][]stamp{}
	for _, p := range s.particles {
		pos := p.physics.Position()
		x, y := int(pos.X), int(pos.Y)
		if x < 0 || x >= s.w || y < 0 || y >= len(lines) {
			continue
		}
		g := lipgloss.NewStyle().Foreground(p.color).Render(p.glyph)
		byRow[y] = append(byRow[y], stamp{x, g})
	}
	for y, stamps := range byRow {
		lines[y] = stampRow(lines[y], stamps)
	}
	return lines
}

// stampRow rebuilds line once, placing each glyph at its visual column. It uses
// ansi.Cut (which adds no style-preservation prefixes), so the output cannot bloat.
func stampRow(line string, stamps []stamp) string {
	sort.Slice(stamps, func(i, j int) bool { return stamps[i].x < stamps[j].x })
	w := ansi.StringWidth(line)
	var b strings.Builder
	col := 0 // next visual column still to emit from the original line
	for _, st := range stamps {
		if st.x < col {
			continue // this column is already covered by an earlier glyph
		}
		if st.x <= w {
			b.WriteString(ansi.Cut(line, col, st.x))
		} else {
			if col < w {
				b.WriteString(ansi.Cut(line, col, w))
			}
			b.WriteString(strings.Repeat(" ", st.x-maxi(col, w)))
		}
		b.WriteString(st.g)
		col = st.x + 1
	}
	if col < w {
		b.WriteString(ansi.Cut(line, col, w))
	}
	return b.String()
}

func maxi(a, b int) int {
	if a > b {
		return a
	}
	return b
}
