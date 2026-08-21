// Package tui implements tick's interactive terminal UI: a global overview of
// projects and a per-project steps screen, with a confetti pop on cross-off.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/simontheguitarist/tick/internal/store"
)

type mode int

const (
	modeOverview mode = iota
	modeSteps
)

// Header geometry. Below wideHeaderMinWidth we fall back to a compact one-line
// header so the ASCII banner never wraps on narrow terminals.
const (
	bannerRows         = 6  // height of the ASCII art
	bannerWidth        = 29 // cell width of the ASCII art
	wideHeaderMinWidth = 56
)

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a864fd"))
	cursorStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#29cdff"))
	doneStyle     = lipgloss.NewStyle().Faint(true).Strikethrough(true)
	checkStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#78ff44"))
	dimStyle      = lipgloss.NewStyle().Faint(true)
	countStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#fdff6a"))
	trackStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#78ff44"))
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("#ff718d"))
	helpStyle     = lipgloss.NewStyle().Faint(true)
	projNameStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#29cdff"))
	pathStyle     = lipgloss.NewStyle().Faint(true)
	ruleStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#44475a"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#8be9fd"))

	importantStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ff718d"))
	importantTextStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ffb86c"))
	numStyle           = lipgloss.NewStyle().Faint(true)
	groupHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#a864fd"))
)

// undoEntry records a deleted step so it can be restored at its original spot
// (the step carries its own id and rank, so position needs no bookkeeping).
type undoEntry struct {
	projKey  string
	projName string
	step     store.Step
}

// bannerArt is "tick" in an ANSI-shadow block font (6 rows × 31 cells).
var bannerArt = []string{
	"████████╗██╗  ██████╗██╗  ██╗",
	"╚══██╔══╝██║ ██╔════╝██║ ██╔╝",
	"   ██║   ██║ ██║     █████╔╝ ",
	"   ██║   ██║ ██║     ██╔═██╗ ",
	"   ██║   ██║ ╚██████╗██║  ██╗",
	"   ╚═╝   ╚═╝  ╚═════╝╚═╝  ╚═╝",
}

// bannerPalette colors each banner row, a gradient drawn from the confetti hues.
var bannerPalette = []string{"#a864fd", "#8f7bff", "#29cdff", "#78ff44", "#fdff6a", "#ff718d"}

// headerHeight is the number of lines header() emits, including the rule.
func (m model) headerHeight() int {
	if m.width >= wideHeaderMinWidth {
		return bannerRows + 1 // banner + rule
	}
	return 3 // logo + subtitle + rule
}

// stepsTopRows is the number of lines above the first step row (header + one
// blank). The confetti origin row is derived from this, so it must match
// exactly what stepsView() renders.
func (m model) stepsTopRows() int { return m.headerHeight() + 1 }

// header renders the branded header. On wide terminals it's the ASCII banner
// with project metadata to its right; on narrow ones, a compact one-liner.
func (m model) header(context, subtitle, right string) string {
	if m.width < wideHeaderMinWidth {
		return m.compactHeader(context, subtitle, right)
	}
	return m.bannerHeader(context, subtitle, right)
}

// bannerHeader draws the ASCII logo on the left and the context/path/count to
// its right, finished with a full-width rule.
func (m model) bannerHeader(context, subtitle, right string) string {
	const metaCol = bannerWidth + 3 // 1 left margin + banner + 2 gap
	meta := map[int]string{}
	if context != "" {
		meta[1] = dimStyle.Render("› ") + projNameStyle.Render(context)
		meta[2] = pathStyle.Render(truncTail(subtitle, m.width-metaCol-1))
		meta[3] = countStyle.Render(right)
	} else {
		meta[2] = pathStyle.Render(truncTail(subtitle, m.width-metaCol-1))
		meta[3] = countStyle.Render(right)
	}

	var b strings.Builder
	for i, art := range bannerArt {
		line := " " + lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(bannerPalette[i])).Render(art)
		if mt, ok := meta[i]; ok {
			line += "  " + mt
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(ruleStyle.Render(strings.Repeat("─", m.width)) + "\n")
	return b.String()
}

// compactHeader is the one-line fallback for narrow terminals: a "✓ tick" logo
// (optionally "› context"), the count flush right, a subtitle line, then a rule.
func (m model) compactHeader(context, subtitle, right string) string {
	w := m.width
	if w < 20 {
		w = 20
	}
	left := " " + checkStyle.Render("✓ ") + titleStyle.Render("tick")
	if context != "" {
		left += dimStyle.Render(" › ") + projNameStyle.Render(context)
	}
	gap := w - lipgloss.Width(left) - lipgloss.Width(right) - 1
	if gap < 1 {
		gap = 1
	}
	var b strings.Builder
	b.WriteString(left + strings.Repeat(" ", gap) + countStyle.Render(right) + "\n")
	b.WriteString("   " + pathStyle.Render(truncTail(subtitle, w-3)) + "\n")
	b.WriteString(ruleStyle.Render(strings.Repeat("─", w)) + "\n")
	return b.String()
}

// truncTail clamps s to max cells, keeping the tail (so a long path still shows
// the project directory at the end) behind a leading ellipsis.
func truncTail(s string, max int) string {
	if max <= 1 {
		return s
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return "…" + string(r[len(r)-(max-1):])
}

// displayPath abbreviates the home directory to ~ for a tidier path line.
func displayPath(p string) string {
	if p == "" {
		return ""
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if p == home {
			return "~"
		}
		if strings.HasPrefix(p, home+string(os.PathSeparator)) {
			return "~" + p[len(home):]
		}
	}
	return p
}

type model struct {
	st   *store.Store
	mode mode

	width, height int

	curPath, curName string // the working directory's project

	ovCursor int // overview selection

	projKey string // selected project's store key (its path, or its id when unlinked)
	stCursor int    // steps selection
	showDone bool

	input        *textInput    // non-nil while adding/editing/grouping
	editTarget   int           // full step index being edited (when input is an edit)
	assignTarget string        // project path being assigned a group (when input is a group prompt)
	confirm      *confirmState // non-nil while confirming an untrack
	conf         *system       // confetti
	anim         *animState    // snapshot held while a burst plays

	undo   []undoEntry // stack of reversible deletions (most recent last)
	status string      // transient one-line message (e.g. "deleted X — u to undo")

	err error
}

const (
	addPrompt   = "add:"
	editPrompt  = "edit:"
	groupPrompt = "group:"
)

func newModel() (model, error) {
	st, err := store.Load()
	if err != nil {
		return model{}, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	root := store.ProjectRoot(cwd)
	return model{
		st:      st,
		width:   80,
		height:  24,
		curPath: root,
		curName: filepath.Base(root),
		conf:    &system{w: 80, h: 24},
	}, nil
}

// RunAuto launches the TUI for `tick` with no args: the current project's steps
// if it's tracked, otherwise the global overview.
func RunAuto() error {
	m, err := newModel()
	if err != nil {
		return err
	}
	if _, ok := m.st.Projects[m.curPath]; ok {
		m.mode = modeSteps
		m.projKey = m.curPath
	} else {
		m.mode = modeOverview
		m = m.clampOvCursor()
	}
	return run(m)
}

// RunOverview launches the TUI directly on the global overview (`tick ui`).
func RunOverview() error {
	m, err := newModel()
	if err != nil {
		return err
	}
	m.mode = modeOverview
	m = m.clampOvCursor()
	return run(m)
}

func run(m model) error {
	_, err := tea.NewProgram(m).Run()
	return err
}

func (m model) Init() tea.Cmd { return nil }

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.conf.w, m.conf.h = msg.Width, msg.Height
		return m, nil

	case frameMsg:
		m.conf.step()
		if m.conf.active() {
			return m, animate()
		}
		// Burst finished: drop the snapshot so the live (post-removal) list shows.
		m.anim = nil
		m = m.clampStepCursor()
		return m, nil

	case tea.PasteMsg:
		if m.input != nil {
			m.input.insert(sanitize(msg.Content))
		}
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if msg.String() == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.input != nil:
		return m.handleInputKey(msg)
	case m.confirm != nil:
		return m.handleConfirmKey(msg)
	case m.mode == modeOverview:
		return m.handleOverviewKey(msg)
	default:
		return m.handleStepsKey(msg)
	}
}

func (m model) View() tea.View {
	var content string
	switch m.mode {
	case modeOverview:
		content = m.overviewView()
	default:
		content = m.stepsView()
	}

	if m.conf.active() {
		lines := strings.Split(content, "\n")
		for len(lines) < m.height { // pad so confetti can rain into empty space
			lines = append(lines, "")
		}
		lines = m.conf.overlay(lines)
		content = strings.Join(lines, "\n")
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}

// --- small text input with a movable cursor ---

type textInput struct {
	prompt string
	value  []rune
	pos    int // cursor offset into value, 0..len(value)
}

// insert places s at the cursor and advances past it. It rebuilds the slice so
// successive inserts never alias a shared backing array.
func (t *textInput) insert(s string) {
	r := []rune(s)
	if len(r) == 0 {
		return
	}
	next := make([]rune, 0, len(t.value)+len(r))
	next = append(next, t.value[:t.pos]...)
	next = append(next, r...)
	next = append(next, t.value[t.pos:]...)
	t.value, t.pos = next, t.pos+len(r)
}

// backspace deletes the rune before the cursor.
func (t *textInput) backspace() {
	if t.pos > 0 {
		t.value = append(t.value[:t.pos-1], t.value[t.pos:]...)
		t.pos--
	}
}

// deleteFwd deletes the rune under the cursor.
func (t *textInput) deleteFwd() {
	if t.pos < len(t.value) {
		t.value = append(t.value[:t.pos], t.value[t.pos+1:]...)
	}
}

func (t *textInput) left() {
	if t.pos > 0 {
		t.pos--
	}
}

func (t *textInput) right() {
	if t.pos < len(t.value) {
		t.pos++
	}
}

func (t *textInput) home() { t.pos = 0 }
func (t *textInput) end()  { t.pos = len(t.value) }

// wordLeft jumps to the start of the previous word: skip spaces, then the word.
func (t *textInput) wordLeft() {
	for t.pos > 0 && unicode.IsSpace(t.value[t.pos-1]) {
		t.pos--
	}
	for t.pos > 0 && !unicode.IsSpace(t.value[t.pos-1]) {
		t.pos--
	}
}

// wordRight jumps to the end of the next word: skip spaces, then the word.
func (t *textInput) wordRight() {
	for t.pos < len(t.value) && unicode.IsSpace(t.value[t.pos]) {
		t.pos++
	}
	for t.pos < len(t.value) && !unicode.IsSpace(t.value[t.pos]) {
		t.pos++
	}
}

func (t *textInput) string() string { return string(t.value) }

func (t *textInput) view() string {
	before := string(t.value[:t.pos])
	var cur, after string
	if t.pos < len(t.value) {
		cur = cursorStyle.Reverse(true).Render(string(t.value[t.pos]))
		after = string(t.value[t.pos+1:])
	} else {
		cur = cursorStyle.Render("▌")
	}
	return "  " + cursorStyle.Render(t.prompt+" ") + before + cur + after
}

// sanitize flattens newlines and tabs from pasted text to spaces so a multi-line
// clipboard can't inject line breaks into a single-line step.
func sanitize(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		return r
	}, s)
}

func (m model) handleInputKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		val := strings.TrimSpace(m.input.string())
		prompt := m.input.prompt
		idx := m.editTarget
		path := m.assignTarget
		m.input = nil
		m.assignTarget = ""
		switch prompt {
		case addPrompt:
			if val == "" {
				return m, nil
			}
			return m.commitAdd(val), nil
		case editPrompt:
			if val == "" {
				return m, nil
			}
			return m.commitEdit(idx, val), nil
		case groupPrompt:
			return m.commitGroup(path, val), nil // blank clears the group
		}
		return m, nil
	case "esc":
		m.input = nil
		m.assignTarget = ""
		return m, nil
	case "left":
		m.input.left()
		return m, nil
	case "right":
		m.input.right()
		return m, nil
	case "home", "ctrl+a":
		m.input.home()
		return m, nil
	case "end", "ctrl+e":
		m.input.end()
		return m, nil
	case "alt+left":
		m.input.wordLeft()
		return m, nil
	case "alt+right":
		m.input.wordRight()
		return m, nil
	case "backspace":
		m.input.backspace()
		return m, nil
	case "delete":
		m.input.deleteFwd()
		return m, nil
	}
	if txt := msg.Key().Text; txt != "" {
		m.input.insert(txt)
	}
	return m, nil
}

// --- confirm prompt (untrack) ---

type confirmState struct {
	prompt      string
	untrackPath string
	untrackName string
}

func (m model) handleConfirmKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(msg.String()) {
	case "y":
		path := m.confirm.untrackPath
		m.confirm = nil
		if s, err := store.Update(func(s *store.Store) error {
			if p := s.Projects[path]; p != nil {
				s.Untrack(p)
			}
			return nil
		}); err != nil {
			m.err = err
		} else {
			m.st = s
		}
		return m.clampOvCursor(), nil
	default: // n, esc, enter, anything else -> cancel
		m.confirm = nil
		return m, nil
	}
}

func (m model) clampStepCursor() model {
	n := len(m.visibleFullIndices())
	if m.stCursor >= n {
		m.stCursor = n - 1
	}
	if m.stCursor < 0 {
		m.stCursor = 0
	}
	return m
}
