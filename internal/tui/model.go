// Package tui implements tick's interactive terminal UI: a global overview of
// projects and a per-project steps screen, with a confetti pop on cross-off.
package tui

import (
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/simontheguitarist/tick/internal/store"
	tsync "github.com/simontheguitarist/tick/internal/sync"
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
		meta[1] = dimStyle.Render("› ") + projNameStyle.Render(ansi.Truncate(context, m.width-metaCol-3, "…"))
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
	// The count is the fixed part of the line; the name yields to it.
	if maxLeft := w - lipgloss.Width(right) - 2; maxLeft > 0 && lipgloss.Width(left) > maxLeft {
		left = ansi.Truncate(left, maxLeft, "…")
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

// wrapText breaks s into lines of at most width cells, at word boundaries where
// it can and mid-word for words too long to fit one.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	return strings.Split(ansi.Wrap(s, width, ""), "\n")
}

// indented renders s wrapped to the window width, two cells in from the left
// margin, styling each line on its own so a break never lands inside an escape
// sequence. Used for the footer lines (help, sync, status, errors).
func (m model) indented(style lipgloss.Style, s string) string {
	var b strings.Builder
	for _, ln := range wrapText(s, m.width-2) {
		b.WriteString("  " + style.Render(ln) + "\n")
	}
	return b.String()
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

	projKey  string // selected project's store key (its path, or its id when unlinked)
	projID   string // the same project's id — survives a re-key when it gets linked
	stCursor int    // steps selection
	showDone bool

	input        *textInput    // non-nil while adding/editing/grouping
	editTarget   string        // id of the step being edited (when input is an edit); an id, not an index, so a background pull can't retarget the edit
	assignTarget string        // project path being assigned a group (when input is a group prompt)
	confirm      *confirmState // non-nil while confirming an untrack
	conf         *system       // confetti
	anim         *animState    // snapshot held while a burst plays

	undo   []undoEntry // stack of reversible deletions (most recent last)
	status string      // transient one-line message (e.g. "deleted X — u to undo")

	// Async sync state. Sync runs as tea.Cmds so the UI never blocks on the
	// network; the store lock is only held for the merge.
	syncEnabled bool
	syncing     bool   // a sync cmd is in flight
	syncPending bool   // a debounce fired mid-flight; run once more after
	syncSeq     int    // invalidates stale debounce timers
	syncInfo    string // persistent footer note ("synced 15:04" / offline)
	quitting    bool   // waiting for the final flush before tea.Quit
	refreshWait bool   // a pull landed during the confetti; refresh after

	err error
}

// Messages for the async sync loop.
type (
	syncDebounceMsg struct{ seq int }
	syncPeriodicMsg struct{}
	syncDoneMsg     struct {
		res tsync.Result
		ran bool
		err error
	}
)

const (
	syncDebounce = 400 * time.Millisecond
	syncPeriod   = 30 * time.Second
	syncBudget   = 5 * time.Second
	quitBudget   = 2 * time.Second
)

func runSyncCmd(timeout time.Duration) tea.Cmd {
	return func() tea.Msg {
		res, ran, err := tsync.RunOnce(timeout)
		return syncDoneMsg{res: res, ran: ran, err: err}
	}
}

func periodicSyncCmd() tea.Cmd {
	return tea.Tick(syncPeriod, func(time.Time) tea.Msg { return syncPeriodicMsg{} })
}

// scheduleSyncCmd arms the post-mutation debounce: rapid edits collapse into
// one sync 400ms after the last.
func (m *model) scheduleSyncCmd() tea.Cmd {
	if !m.syncEnabled {
		return nil
	}
	m.syncSeq++
	seq := m.syncSeq
	return tea.Tick(syncDebounce, func(time.Time) tea.Msg { return syncDebounceMsg{seq: seq} })
}

// maybeQuit flushes pending changes (bounded) before leaving, so crossing
// something off and immediately quitting still reaches the phone.
func (m model) maybeQuit() (tea.Model, tea.Cmd) {
	if !m.syncEnabled || m.quitting {
		return m, tea.Quit
	}
	m.quitting = true
	m.syncInfo = "syncing…"
	return m, runSyncCmd(quitBudget)
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
	syncEnabled := false
	if os.Getenv("TICK_NO_SYNC") != "1" {
		if d, err := store.DefaultDir(); err == nil {
			if cfg, err := tsync.LoadConfig(d); err == nil && cfg != nil {
				syncEnabled = true
			}
		}
	}
	return model{
		st:          st,
		width:       80,
		height:      24,
		curPath:     root,
		curName:     filepath.Base(root),
		conf:        &system{w: 80, h: 24},
		syncEnabled: syncEnabled,
	}, nil
}

// RunAuto launches the TUI for `tick` with no args: the current project's steps
// if it's tracked, otherwise the global overview.
func RunAuto() error {
	m, err := newModel()
	if err != nil {
		return err
	}
	if p, ok := m.st.Projects[m.curPath]; ok {
		m.mode = modeSteps
		m.projKey, m.projID = m.curPath, p.ID
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

func (m model) Init() tea.Cmd {
	if !m.syncEnabled {
		return nil
	}
	return tea.Batch(runSyncCmd(syncBudget), periodicSyncCmd())
}

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
		if m.refreshWait { // a sync pull landed mid-burst
			m.refreshWait = false
			m.refreshFromDisk()
		}
		m = m.clampStepCursor()
		return m, nil

	case syncPeriodicMsg:
		cmds := []tea.Cmd{periodicSyncCmd()}
		if m.syncEnabled && !m.syncing && !m.quitting {
			m.syncing = true
			cmds = append(cmds, runSyncCmd(syncBudget))
		}
		return m, tea.Batch(cmds...)

	case syncDebounceMsg:
		if msg.seq != m.syncSeq || !m.syncEnabled {
			return m, nil
		}
		if m.syncing {
			m.syncPending = true
			return m, nil
		}
		m.syncing = true
		return m, runSyncCmd(syncBudget)

	case syncDoneMsg:
		m.syncing = false
		if msg.ran {
			if msg.err != nil {
				m.syncInfo = "offline — changes queued"
			} else {
				m.syncInfo = "synced " + time.Now().Format("15:04")
				if msg.res.Pulled > 0 {
					if m.anim != nil {
						m.refreshWait = true // don't yank rows mid-confetti
					} else {
						m.refreshFromDisk()
						m = m.clampStepCursor().clampOvCursor()
					}
				}
			}
		}
		if m.quitting {
			return m, tea.Quit
		}
		if m.syncPending {
			m.syncPending = false
			m.syncing = true
			return m, runSyncCmd(syncBudget)
		}
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

// --- small text input with a movable cursor and a horizontal scroll window ---

type textInput struct {
	prompt string
	value  []rune
	pos    int // cursor offset into value, 0..len(value)
	off    int // first visible rune: the text slides left once it outgrows the window
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

// inputMinRoom is the smallest text window we shrink to; below that the line is
// allowed to overflow rather than collapse to nothing.
const inputMinRoom = 4

// cells is the display width of a run of runes.
func cells(r []rune) int { return ansi.StringWidth(string(r)) }

// cursorCells is the width of the cell the cursor occupies: the rune under it,
// or one cell for the block cursor sitting past the last rune.
func (t *textInput) cursorCells() int {
	if t.pos >= len(t.value) {
		return 1
	}
	if w := cells(t.value[t.pos : t.pos+1]); w > 0 {
		return w
	}
	return 1
}

// scroll slides the window so the cursor always stays inside room cells: out to
// the right as you type past the edge, and back to the left as soon as the whole
// tail fits again (after a deletion, or after jumping back with home/left).
func (t *textInput) scroll(room int) {
	if t.off > t.pos {
		t.off = t.pos
	}
	for t.off < t.pos && cells(t.value[t.off:t.pos])+t.cursorCells() > room {
		t.off++
	}
	// +1 keeps a cell free for the block cursor that follows the last rune.
	for t.off > 0 && cells(t.value[t.off-1:])+1 <= room {
		t.off--
	}
}

// visibleEnd is the exclusive rune index where the window ends: as many runes
// past off as fit in room cells.
func (t *textInput) visibleEnd(room int) int {
	w, i := 0, t.off
	for ; i < len(t.value); i++ {
		cw := cells(t.value[i : i+1])
		if w+cw > room {
			break
		}
		w += cw
	}
	return i
}

func (t *textInput) view(width int) string {
	// Left margin, prompt, the 1-cell gap after it (which doubles as the
	// clipped-head marker, so the text column never moves), a 1-cell slot for
	// the clipped-tail marker, and one spare column so the cursor never lands
	// in the last cell of the line.
	room := width - 2 - ansi.StringWidth(t.prompt) - 1 - 1 - 1
	if room < inputMinRoom {
		room = inputMinRoom
	}
	t.scroll(room)
	end := t.visibleEnd(room)
	if t.pos < len(t.value) && end <= t.pos {
		end = t.pos + 1 // the cell under the cursor is always shown
	}

	lead := " "
	if t.off > 0 {
		lead = dimStyle.Render("…")
	}
	trail := ""
	if end < len(t.value) {
		trail = dimStyle.Render("…")
	}

	before := string(t.value[t.off:t.pos])
	var cur, after string
	if t.pos < len(t.value) {
		cur = cursorStyle.Reverse(true).Render(string(t.value[t.pos]))
		after = string(t.value[t.pos+1 : end])
	} else {
		cur = cursorStyle.Render("▌")
	}
	return "  " + cursorStyle.Render(t.prompt) + lead + before + cur + after + trail
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
		editID := m.editTarget
		path := m.assignTarget
		m.input = nil
		m.assignTarget = ""
		switch prompt {
		case addPrompt:
			if val == "" {
				return m, nil
			}
			m = m.commitAdd(val)
			return m, m.scheduleSyncCmd()
		case editPrompt:
			if val == "" {
				return m, nil
			}
			m = m.commitEdit(editID, val)
			return m, m.scheduleSyncCmd()
		case groupPrompt:
			m = m.commitGroup(path, val) // blank clears the group
			return m, m.scheduleSyncCmd()
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
		return m.clampOvCursor(), m.scheduleSyncCmd()
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
