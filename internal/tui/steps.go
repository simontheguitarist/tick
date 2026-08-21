package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/simontheguitarist/tick/internal/protocol"
	"github.com/simontheguitarist/tick/internal/rank"
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
	p := m.st.Projects[m.projKey]
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
	p := m.st.Projects[m.projKey]
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
	if p := m.st.Projects[m.projKey]; p != nil {
		return p.Name
	}
	if m.projKey != "" {
		return filepath.Base(m.projKey)
	}
	return m.curName
}

func (m model) stepsView() string {
	var b strings.Builder
	p := m.st.Projects[m.projKey]
	open := 0
	if p != nil {
		open = p.OpenCount()
	}

	// Branded header (ASCII banner + project name/path/count, or compact on
	// narrow terminals) followed by a blank line. Keep in sync with stepsTopRows().
	sub := displayPath(m.projKey)
	if p != nil && p.Path == "" {
		sub = "not linked — created on another device"
	}
	b.WriteString(m.header(m.projectName(), sub, fmt.Sprintf("%d open", open)))
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
	if m.syncInfo != "" {
		b.WriteString("  " + dimStyle.Render("⇅ "+m.syncInfo) + "\n")
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
		return m.maybeQuit()
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
		m = m.moveSelected(idxs, true)
		return m, m.scheduleSyncCmd()
	case "shift+down", "J": // move the selected step down
		m = m.moveSelected(idxs, false)
		return m, m.scheduleSyncCmd()
	case "t": // bump the selected step to priority #1
		m = m.moveToTop(idxs)
		return m, m.scheduleSyncCmd()
	case "i": // toggle importance (visual flag; does not reorder)
		m = m.toggleImportant(idxs)
		return m, m.scheduleSyncCmd()
	case "y": // copy the project's path so you can cd to it elsewhere
		if p := m.st.Projects[m.projKey]; p == nil || p.Path == "" {
			m.status = "no local path — not linked to a directory yet"
			return m, nil
		}
		m.status = "copied path"
		return m, tea.SetClipboard(m.projKey)
	case "h":
		m.showDone = !m.showDone
		return m.clampStepCursor(), nil
	case "a":
		m.input = &textInput{prompt: addPrompt}
		return m, nil
	case "e":
		if full, ok := m.selectedFull(idxs); ok {
			st := m.st.Projects[m.projKey].Steps[full]
			m.editTarget = st.ID
			txt := []rune(st.Text)
			m.input = &textInput{prompt: editPrompt, value: txt, pos: len(txt)}
		}
		return m, nil
	case "d":
		m = m.deleteSelected(idxs)
		return m, m.scheduleSyncCmd()
	case "u":
		m = m.undoLast()
		return m, m.scheduleSyncCmd()
	case "x", "enter", "space":
		next, cmd := m.toggleSelected(idxs)
		if nm, ok := next.(model); ok {
			return nm, tea.Batch(cmd, nm.scheduleSyncCmd())
		}
		return next, cmd
	}
	return m, nil
}

// toggleSelected crosses an open step off (with confetti) or reopens a done one.
func (m model) toggleSelected(idxs []int) (tea.Model, tea.Cmd) {
	full, ok := m.selectedFull(idxs)
	if !ok {
		return m, nil
	}
	if m.st.Projects[m.projKey].Steps[full].Done {
		return m.reopenSelected(idxs), nil
	}
	return m.crossOff(idxs)
}

// moveSelected moves the selected step past its visible neighbor (up or down)
// by re-ranking just that step — the sync-friendly form of a swap.
func (m model) moveSelected(idxs []int, up bool) model {
	c := m.stCursor
	if c < 0 || c >= len(idxs) {
		return m
	}
	nb := c + 1
	if up {
		nb = c - 1
	}
	if nb < 0 || nb >= len(idxs) {
		return m // already at an edge
	}
	p0 := m.st.Projects[m.projKey]
	if p0 == nil {
		return m
	}
	movedID, neighborID := p0.Steps[idxs[c]].ID, p0.Steps[idxs[nb]].ID
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projKey]
		if p == nil {
			return nil
		}
		p.Rebalance() // duplicate ranks leave no gap to move into
		mi := p.StepIndex(movedID)
		if mi < 0 {
			return nil
		}
		// Same algorithm as the app's Store.move: take the moved step out,
		// find the landing slot next to the neighbor, rank between the two
		// steps that will surround it there.
		rest := slices.Clone(p.Steps)
		rest = slices.DeleteFunc(rest, func(st store.Step) bool { return st.ID == movedID })
		dest := slices.IndexFunc(rest, func(st store.Step) bool { return st.ID == neighborID })
		if dest < 0 {
			return nil
		}
		if !up {
			dest++
		}
		lo, hi := "", ""
		if dest > 0 {
			lo = rest[dest-1].Rank
		}
		if dest < len(rest) {
			hi = rest[dest].Rank
		}
		p.Steps[mi].Rank = rank.Mid(lo, hi)
		store.Touch(&p.Steps[mi])
		p.SortSteps()
		p.Rebalance()
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m.cursorToStep(movedID)
}

// moveToTop bumps the selected step to priority #1, preserving the relative
// order of the rest.
func (m model) moveToTop(idxs []int) model {
	full, ok := m.selectedFull(idxs)
	if !ok || full == 0 {
		return m
	}
	p0 := m.st.Projects[m.projKey]
	if p0 == nil || full >= len(p0.Steps) {
		return m
	}
	movedID := p0.Steps[full].ID
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projKey]
		if p == nil {
			return nil
		}
		p.Rebalance()
		mi := p.StepIndex(movedID)
		if mi <= 0 {
			return nil
		}
		p.Steps[mi].Rank = rank.Before(p.Steps[0].Rank)
		store.Touch(&p.Steps[mi])
		p.SortSteps()
		p.Rebalance()
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
	p0 := m.st.Projects[m.projKey]
	if p0 == nil || full >= len(p0.Steps) {
		return m
	}
	id := p0.Steps[full].ID
	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projKey]
		if p == nil {
			return nil
		}
		if i := p.StepIndex(id); i >= 0 {
			p.Steps[i].Important = !p.Steps[i].Important
			store.Touch(&p.Steps[i])
		}
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
	p := m.st.Projects[m.projKey]
	if p.Steps[full].Done {
		return m, nil // already done
	}
	text := p.Steps[full].Text
	id := p.Steps[full].ID

	snap := m.liveStepRows()
	if m.stCursor < len(snap) {
		snap[m.stCursor].crossed = true
	}

	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projKey]
		if pp == nil {
			return nil
		}
		if i := pp.StepIndex(id); i >= 0 && !pp.Steps[i].Done {
			now := store.Now()
			pp.Steps[i].Done = true
			pp.Steps[i].DoneAt = &now
			store.Touch(&pp.Steps[i])
		}
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
		p := s.Projects[m.projKey]
		if p == nil && filepath.IsAbs(m.projKey) {
			p = s.Ensure(m.projKey, m.projectName())
		}
		if p == nil {
			return fmt.Errorf("project vanished — go back to the overview")
		}
		p.AddStep(text)
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

func (m model) commitEdit(id, text string) model {
	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projKey]
		if pp == nil {
			return nil
		}
		if i := pp.StepIndex(id); i >= 0 {
			pp.Steps[i].Text = protocol.Clamp(text, protocol.MaxText)
			store.Touch(&pp.Steps[i])
		}
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
	pp := m.st.Projects[m.projKey]
	if pp == nil || full >= len(pp.Steps) {
		return m
	}
	deleted := pp.Steps[full] // capture before removal so we can undo

	s, err := store.Update(func(s *store.Store) error {
		p := s.Projects[m.projKey]
		if p == nil {
			return nil
		}
		if i := p.StepIndex(deleted.ID); i >= 0 {
			s.RemoveStep(p, i)
		}
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.undo = append(m.undo, undoEntry{
		projKey: m.projKey, projName: m.projectName(), step: deleted,
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
		p := s.Projects[e.projKey]
		if p == nil && filepath.IsAbs(e.projKey) {
			p = s.Ensure(e.projKey, e.projName) // project may have been untracked meanwhile
		}
		if p == nil {
			return fmt.Errorf("that project is gone — nothing restored")
		}
		s.RestoreStep(p, e.step)
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	m.status = fmt.Sprintf("restored %q", clip(e.step.Text, 36))
	// If we're viewing the project it was restored into, move the cursor to it.
	if e.projKey == m.projKey {
		return m.cursorToStep(e.step.ID)
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
	p0 := m.st.Projects[m.projKey]
	if p0 == nil || full >= len(p0.Steps) {
		return m
	}
	id := p0.Steps[full].ID
	s, err := store.Update(func(s *store.Store) error {
		pp := s.Projects[m.projKey]
		if pp == nil {
			return nil
		}
		if i := pp.StepIndex(id); i >= 0 {
			pp.Steps[i].Done = false
			pp.Steps[i].DoneAt = nil
			store.Touch(&pp.Steps[i])
		}
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	return m
}

// refreshFromDisk reloads the store so the overview reflects any external
// edits. The open project is re-found by id: linking it (here or on a pull)
// re-keys it from id to path.
func (m *model) refreshFromDisk() {
	if s, err := store.Load(); err == nil {
		m.st = s
		if m.projID != "" && m.st.Projects[m.projKey] == nil {
			if p := m.st.ByID(m.projID); p != nil {
				m.projKey = store.Key(p)
			}
		}
	}
}

// cursorToStep points the cursor at the step with the given id, if visible.
func (m model) cursorToStep(id string) model {
	p := m.st.Projects[m.projKey]
	if p == nil {
		return m.clampStepCursor()
	}
	for vi, f := range m.visibleFullIndices() {
		if p.Steps[f].ID == id {
			m.stCursor = vi
			return m
		}
	}
	return m.clampStepCursor()
}
