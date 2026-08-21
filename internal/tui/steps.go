package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/simontheguitarist/tick/internal/store"
)

// displayRow is one rendered step line.
type displayRow struct {
	text      string
	done      bool
	important bool
	crossed   bool // the step being celebrated right now (rendered as just-done)
	num       int  // 1-based priority position among open steps (0 = none, e.g. done rows)
}

func (r displayRow) render() string {
	if r.crossed || r.done {
		return checkStyle.Render("✓ ") + doneStyle.Render(r.text)
	}
	flag := "  " // constant-width slot so importance never shifts columns
	text := r.text
	if r.important {
		flag = importantStyle.Render("! ")
		text = importantTextStyle.Render(text)
	}
	return flag + numStyle.Render(stepNumLabel(r.num)) + text
}

// stepNumLabel formats the priority position shown left of an open step.
func stepNumLabel(num int) string { return fmt.Sprintf("%d. ", num) }

// stepPrefixWidth is the cell width of everything left of a step's text — the
// "> " cursor column, the importance slot, and the number label. The confetti
// origin and the renderer share it so the burst always centers on the text.
func stepPrefixWidth(num int) int {
	return 2 /* "> " */ + 2 /* importance slot */ + ansi.StringWidth(stepNumLabel(num))
}

// animState is a snapshot of the visible rows, held while a burst plays so the
// list doesn't jump the instant a step is crossed off.
type animState struct {
	rows []displayRow
}

// visibleFullIndices maps the on-screen step order to indices in the full
// Steps slice, honoring the show-done toggle.
func (m model) visibleFullIndices() []int {
	p := m.st.Projects[m.projPath]
	if p == nil {
		return nil
	}
	var idx []int
	for i := range p.Steps {
		if p.Steps[i].Done && !m.showDone {
			continue
		}
		idx = append(idx, i)
	}
	return idx
}

func (m model) liveStepRows() []displayRow {
	p := m.st.Projects[m.projPath]
	if p == nil {
		return nil
	}
	var rows []displayRow
	open := 0
	for _, st := range p.Steps {
		if st.Done && !m.showDone {
			continue
		}
		r := displayRow{text: st.Text, done: st.Done, important: st.Important}
		if !st.Done {
			open++
			r.num = open
		}
		rows = append(rows, r)
	}
	return rows
}

func (m model) projectName() string {
	if p := m.st.Projects[m.projPath]; p != nil {
		return p.Name
	}
	if m.projPath != "" {
		return filepath.Base(m.projPath)
	}
	return m.curName
}

func (m model) stepsView() string {
	var b strings.Builder
	p := m.st.Projects[m.projPath]
	open := 0
	if p != nil {
		open = p.OpenCount()
	}

	// Branded header (ASCII banner + project name/path/count, or compact on
	// narrow terminals) followed by a blank line. Keep in sync with stepsTopRows().
	b.WriteString(m.header(m.projectName(), displayPath(m.projPath), fmt.Sprintf("%d open", open)))
	b.WriteString("\n")

	rows := m.liveStepRows()
	if m.anim != nil {
		rows = m.anim.rows
	}
	if len(rows) == 0 {
		b.WriteString("  " + dimStyle.Render("no steps yet — press ") + cursorStyle.Render("a") + dimStyle.Render(" to add one") + "\n")
	}
	for i, r := range rows {
		cursor := "  "
		if i == m.stCursor && m.anim == nil && m.input == nil {
			cursor = cursorStyle.Render("> ")
		}
		b.WriteString(cursor + r.render() + "\n")
	}

	b.WriteString("\n")
	if m.input != nil {
		b.WriteString(m.input.view() + "\n")
		b.WriteString("  " + helpStyle.Render("enter save · esc cancel") + "\n")
	} else {
		b.WriteString("  " + helpStyle.Render("x done · a add · e edit · i ! · t top · ⇧↑/↓ move · d del · u undo · y copy · h show-done · esc back · q quit") + "\n")
	}
	if m.status != "" {
		b.WriteString("  " + statusStyle.Render(m.status) + "\n")
	}
	if m.err != nil {
		b.WriteString("  " + errStyle.Render(m.err.Error()) + "\n")
	}
	return b.String()
}

func (m model) handleStepsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.anim != nil { // ignore input while a burst is playing
		return m, nil
	}
	m.err = nil
	m.status = ""
	idxs := m.visibleFullIndices()

	switch msg.String() {
	case "q":
		return m, tea.Quit
	case "esc", "g":
		m.mode = modeOverview
		m.refreshFromDisk()
		return m.clampOvCursor(), nil
	case "up", "k":
		if m.stCursor > 0 {
			m.stCursor--
		}
		return m, nil
	case "down", "j":
		if m.stCursor < len(idxs)-1 {
			m.stCursor++
		}
		return m, nil
	case "shift+up", "K": // move the selected step up
		return m.moveSelected(idxs, true), nil
	case "shift+down", "J": // move the selected step down
		return m.moveSelected(idxs, false), nil
	case "t": // bump the selected step to priority #1
		return m.moveToTop(idxs), nil
	case "i": // toggle importance (visual flag; does not reorder)
		return m.toggleImportant(idxs), nil
	case "y": // copy the project's path so you can cd to it elsewhere
		m.status = "copied path"
		return m, tea.SetClipboard(m.projPath)
	case "h":
		m.showDone = !m.showDone
		return m.clampStepCursor(), nil
	case "a":
		m.input = &textInput{prompt: addPrompt}
		return m, nil
	case "e":
		if full, ok := m.selectedFull(idxs); ok {
			m.editTarget = full
			txt := []rune(m.st.Projects[m.projPath].Steps[full].Text)
			m.input = &textInput{prompt: editPrompt, value: txt, pos: len(txt)}
		}
		return m, nil
	case "d":
		return m.deleteSelected(idxs), nil
	case "u":
		return m.undoLast(), nil
	case "x", "enter", "space":
		return m.toggleSelected(idxs)
	}
	return m, nil
}

// toggleSelected crosses an open step off (with confetti) or reopens a done one.
func (m model) toggleSelected(idxs []int) (tea.Model, tea.Cmd) {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m, nil
	}
	if m.st.Projects[m.projPath].Steps[full].Done {
		return m.reopenSelected(idxs), nil
	}
	return m.crossOff(idxs)
}

// moveSelected swaps the selected step with its visible neighbor (up or down),
// reordering the project's next steps.
func (m model) moveSelected(idxs []int, up bool) model {
	c := m.stCursor
	if c < 0 || c >= len(idxs) {
		return m
	}
	swapWith := c + 1
	if up {
		swapWith = c - 1
	}
	if swapWith < 0 || swapWith >= len(idxs) {
		return m // already at an edge
	}
	a, b := idxs[c], idxs[swapWith]
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projPath]
		if p == nil || a >= len(p.Steps) || b >= len(p.Steps) {
			return nil
		}
		p.Steps[a], p.Steps[b] = p.Steps[b], p.Steps[a]
		p.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.stCursor = swapWith // follow the moved step
	return m
}

// moveToTop bumps the selected step to priority #1 (full index 0), preserving
// the relative order of the rest.
func (m model) moveToTop(idxs []int) model {
	full, ok := m.selectedFull(idxs)
	if !ok || full == 0 {
		return m
	}
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projPath]
		if p == nil || full >= len(p.Steps) {
			return nil
		}
		moved := p.Steps[full]
		p.Steps = append(p.Steps[:full], p.Steps[full+1:]...)
		p.Steps = append([]store.Step{moved}, p.Steps...)
		p.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.stCursor = 0
	return m.clampStepCursor()
}

// toggleImportant flips the importance flag on the selected step. It's a visual
// accent only — the manual order is the priority, so nothing is reordered.
func (m model) toggleImportant(idxs []int) model {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m
	}
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projPath]
		if p == nil || full < 0 || full >= len(p.Steps) {
			return nil
		}
		p.Steps[full].Important = !p.Steps[full].Important
		p.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

func (m model) selectedFull(idxs []int) (int, bool) {
	if m.stCursor < 0 || m.stCursor >= len(idxs) {
		return 0, false
	}
	return idxs[m.stCursor], true
}

// crossOff marks the selected step done, persists it immediately, and fires the
// confetti burst over its row. The visible list is snapshotted so it doesn't
// jump until the burst finishes (animate-then-remove).
func (m model) crossOff(idxs []int) (tea.Model, tea.Cmd) {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m, nil
	}
	p := m.st.Projects[m.projPath]
	if p.Steps[full].Done {
		return m, nil // already done
	}
	text := p.Steps[full].Text

	snap := m.liveStepRows()
	if m.stCursor < len(snap) {
		snap[m.stCursor].crossed = true
	}

	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projPath]
		if pp == nil || full >= len(pp.Steps) {
			return nil
		}
		now := time.Now()
		pp.Steps[full].Done = true
		pp.Steps[full].DoneAt = &now
		pp.Modified = now
		return nil
	})
	if err != nil {
		m.err = err
		return m, nil
	}
	m.st = s

	row := m.stepsTopRows() + m.stCursor
	num := 0
	if m.stCursor < len(snap) {
		num = snap[m.stCursor].num
	}
	originX := float64(stepPrefixWidth(num) + ansi.StringWidth(text)/2)
	if max := float64(m.width - 1); originX > max {
		originX = max
	}
	m.conf.w, m.conf.h = m.width, m.height
	m.conf.burst(originX, float64(row))
	m.anim = &animState{rows: snap}
	return m, animate()
}

func (m model) commitAdd(text string) model {
	s, err := store.Update(func(s *store.Store) error {
		s.Ensure(m.projPath, m.projectName()).AddStep(text)
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

func (m model) commitEdit(full int, text string) model {
	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projPath]
		if pp == nil || full < 0 || full >= len(pp.Steps) {
			return nil
		}
		pp.Steps[full].Text = text
		pp.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

func (m model) deleteSelected(idxs []int) model {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m
	}
	pp := m.st.Projects[m.projPath]
	if pp == nil || full >= len(pp.Steps) {
		return m
	}
	deleted := pp.Steps[full] // capture before removal so we can undo

	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projPath]
		if p == nil || full >= len(p.Steps) {
			return nil
		}
		p.Steps = append(p.Steps[:full], p.Steps[full+1:]...)
		p.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.undo = append(m.undo, undoEntry{
		projPath: m.projPath, projName: m.projectName(), index: full, step: deleted,
	})
	m.status = fmt.Sprintf("deleted %q — press u to undo", clip(deleted.Text, 36))
	return m.clampStepCursor()
}

// undoLast restores the most recently deleted step at its original position.
func (m model) undoLast() model {
	if len(m.undo) == 0 {
		m.status = "nothing to undo"
		return m
	}
	e := m.undo[len(m.undo)-1]
	m.undo = m.undo[:len(m.undo)-1]

	s, err := store.Update(func(s *store.Store) error {
		p := s.Ensure(e.projPath, e.projName) // project may have been emptied/removed
		i := e.index
		if i > len(p.Steps) {
			i = len(p.Steps)
		}
		p.Steps = append(p.Steps[:i], append([]store.Step{e.step}, p.Steps[i:]...)...)
		p.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.status = fmt.Sprintf("restored %q", clip(e.step.Text, 36))
	// If we're viewing the project it was restored into, move the cursor to it.
	if e.projPath == m.projPath {
		for vi, f := range m.visibleFullIndices() {
			if f == e.index {
				m.stCursor = vi
				break
			}
		}
	}
	return m.clampStepCursor()
}

// clip shortens s to max runes with a trailing ellipsis, for status messages.
func clip(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}

func (m model) reopenSelected(idxs []int) model {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m
	}
	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projPath]
		if pp == nil || full >= len(pp.Steps) {
			return nil
		}
		pp.Steps[full].Done = false
		pp.Steps[full].DoneAt = nil
		pp.Modified = time.Now()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

// refreshFromDisk reloads the store so the overview reflects any external edits.
func (m *model) refreshFromDisk() {
	if s, err := store.Load(); err == nil {
		m.st = s
	}
}
