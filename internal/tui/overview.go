package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"tick/internal/store"
)

// ovKind distinguishes the three row types in the overview.
type ovKind int

const (
	ovHeader       ovKind = iota // a non-selectable group section header
	ovTrackCurrent               // the "track current dir" affordance
	ovProject                    // a tracked project
)

// ovItem is one row in the overview.
type ovItem struct {
	kind  ovKind
	label string         // group name when kind == ovHeader
	proj  *store.Project // set when kind == ovProject
}

// ovItems builds the overview rows: a synthetic "track current dir" row first
// (only when the current dir isn't tracked), then projects grouped into sections
// (alphabetical), with ungrouped projects last. When no groups exist at all, no
// headers are emitted, so the list looks exactly like a flat overview.
func (m model) ovItems() []ovItem {
	var items []ovItem
	if _, ok := m.st.Projects[m.curPath]; !ok {
		items = append(items, ovItem{kind: ovTrackCurrent})
	}

	groups := map[string][]*store.Project{}
	var order []string
	for _, p := range m.st.SortedProjects() {
		if p.Group != "" {
			if _, seen := groups[p.Group]; !seen {
				order = append(order, p.Group)
			}
		}
		groups[p.Group] = append(groups[p.Group], p)
	}
	sort.Strings(order)

	for _, g := range order {
		items = append(items, ovItem{kind: ovHeader, label: g})
		for _, p := range groups[g] {
			items = append(items, ovItem{kind: ovProject, proj: p})
		}
	}
	if ung := groups[""]; len(ung) > 0 {
		if len(order) > 0 { // only label "ungrouped" when there's something to contrast with
			items = append(items, ovItem{kind: ovHeader, label: "ungrouped"})
		}
		for _, p := range ung {
			items = append(items, ovItem{kind: ovProject, proj: p})
		}
	}
	return items
}

// firstSelectable returns the index of the first non-header row scanning from
// `from` in direction `dir` (+1/-1), or -1 if there is none.
func (m model) firstSelectable(items []ovItem, from, dir int) int {
	for i := from; i >= 0 && i < len(items); i += dir {
		if items[i].kind != ovHeader {
			return i
		}
	}
	return -1
}

// selectedProject returns the project under the cursor, or nil for header /
// track-current rows.
func (m model) selectedProject(items []ovItem) *store.Project {
	if m.ovCursor < 0 || m.ovCursor >= len(items) {
		return nil
	}
	if it := items[m.ovCursor]; it.kind == ovProject {
		return it.proj
	}
	return nil
}

// clampOvCursor keeps the cursor in range and off section headers.
func (m model) clampOvCursor() model {
	items := m.ovItems()
	if len(items) == 0 {
		m.ovCursor = 0
		return m
	}
	if m.ovCursor < 0 {
		m.ovCursor = 0
	} else if m.ovCursor >= len(items) {
		m.ovCursor = len(items) - 1
	}
	if items[m.ovCursor].kind == ovHeader {
		if i := m.firstSelectable(items, m.ovCursor, +1); i >= 0 {
			m.ovCursor = i
		} else if i := m.firstSelectable(items, m.ovCursor, -1); i >= 0 {
			m.ovCursor = i
		}
	}
	return m
}

func (m model) overviewView() string {
	var b strings.Builder
	n := len(m.st.Projects)
	suffix := "s"
	if n == 1 {
		suffix = ""
	}
	b.WriteString(m.header("", "next steps across your projects", fmt.Sprintf("%d project%s", n, suffix)))
	b.WriteString("\n")

	items := m.ovItems()
	if len(items) == 0 {
		b.WriteString("  " + dimStyle.Render("no projects yet — cd into one and add a step") + "\n")
	}
	for i, it := range items {
		if it.kind == ovHeader {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(" " + groupHeaderStyle.Render(strings.ToUpper(it.label)) + "\n")
			continue
		}
		cursor := "  "
		if i == m.ovCursor && m.confirm == nil && m.input == nil {
			cursor = cursorStyle.Render("> ")
		}
		if it.kind == ovTrackCurrent {
			b.WriteString(cursor + trackStyle.Render("+ Track current dir: ") + trackStyle.Render(m.curName) + "\n")
			continue
		}
		p := it.proj
		marker := ""
		if p.Path == m.curPath {
			marker = dimStyle.Render("  (here)")
		}
		name := fmt.Sprintf("%-24s", p.Name)
		b.WriteString(cursor + name + "  " + countStyle.Render(fmt.Sprintf("%d open", p.OpenCount())) + marker + "\n")
	}

	b.WriteString("\n")
	switch {
	case m.input != nil:
		b.WriteString(m.input.view() + "\n")
		hint := "enter save · esc cancel"
		if g := m.existingGroups(); len(g) > 0 {
			hint += "  ·  existing: " + strings.Join(g, ", ") + " (blank clears)"
		}
		b.WriteString("  " + helpStyle.Render(hint) + "\n")
	case m.confirm != nil:
		b.WriteString("  " + errStyle.Render(m.confirm.prompt) + "\n")
	default:
		b.WriteString("  " + helpStyle.Render("enter open · g group · y copy path · d untrack · q quit") + "\n")
	}
	if m.status != "" {
		b.WriteString("  " + statusStyle.Render(m.status) + "\n")
	}
	if m.err != nil {
		b.WriteString("  " + errStyle.Render(m.err.Error()) + "\n")
	}
	return b.String()
}

func (m model) handleOverviewKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.err = nil
	m.status = ""
	items := m.ovItems()

	switch msg.String() {
	case "q", "esc":
		return m, tea.Quit
	case "up", "k":
		if i := m.firstSelectable(items, m.ovCursor-1, -1); i >= 0 {
			m.ovCursor = i
		}
		return m, nil
	case "down", "j":
		if i := m.firstSelectable(items, m.ovCursor+1, +1); i >= 0 {
			m.ovCursor = i
		}
		return m, nil
	case "enter", "l", "space":
		return m.openSelected(items), nil
	case "g": // assign the selected project to a group
		if p := m.selectedProject(items); p != nil {
			m.assignTarget = p.Path
			cur := []rune(p.Group)
			m.input = &textInput{prompt: groupPrompt, value: cur, pos: len(cur)}
		}
		return m, nil
	case "y": // copy the selected project's path
		if p := m.selectedProject(items); p != nil {
			m.status = "copied path"
			return m, tea.SetClipboard(p.Path)
		}
		return m, nil
	case "d":
		return m.untrackSelected(items), nil
	}
	return m, nil
}

// openSelected enters the steps screen for the selected project, tracking the
// current dir first if the synthetic row is chosen.
func (m model) openSelected(items []ovItem) model {
	if m.ovCursor < 0 || m.ovCursor >= len(items) {
		return m
	}
	switch it := items[m.ovCursor]; it.kind {
	case ovTrackCurrent:
		s, err := store.Update(func(s *store.Store) error {
			s.Ensure(m.curPath, m.curName)
			return nil
		})
		if err != nil {
			m.err = err
			return m
		}
		m.st = s
		m.projPath = m.curPath
	case ovProject:
		m.projPath = it.proj.Path
	default:
		return m // header — not selectable
	}
	m.mode = modeSteps
	m.stCursor = 0
	return m
}

func (m model) untrackSelected(items []ovItem) model {
	p := m.selectedProject(items)
	if p == nil {
		return m
	}
	m.confirm = &confirmState{
		prompt:      fmt.Sprintf("Untrack %q and delete its %d step(s)?  y / n", p.Name, len(p.Steps)),
		untrackPath: p.Path,
		untrackName: p.Name,
	}
	return m
}

// commitGroup assigns (or, with a blank value, clears) the project's group.
func (m model) commitGroup(path, group string) model {
	s, err := store.Update(func(s *store.Store) error {
		if p := s.Projects[path]; p != nil {
			p.Group = group
			p.Modified = time.Now()
		}
		return nil
	})
	if err != nil {
		m.err = err
		return m
	}
	m.st = s
	if group == "" {
		m.status = "moved to ungrouped"
	} else {
		m.status = "moved to " + group
	}
	return m.clampOvCursor() // the project jumped sections; keep the cursor selectable
}

// existingGroups returns the sorted, distinct, non-empty group names in use.
func (m model) existingGroups() []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range m.st.Projects {
		if p.Group != "" && !seen[p.Group] {
			seen[p.Group] = true
			out = append(out, p.Group)
		}
	}
	sort.Strings(out)
	return out
}
