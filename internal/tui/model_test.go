package tui

import (
	"reflect"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/simontheguitarist/tick/internal/store"
)

// keyFromString builds a KeyPressMsg whose String() matches the handlers.
func keyFromString(s string) tea.KeyPressMsg {
	switch s {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "backspace":
		return tea.KeyPressMsg{Code: tea.KeyBackspace}
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "left":
		return tea.KeyPressMsg{Code: tea.KeyLeft}
	case "right":
		return tea.KeyPressMsg{Code: tea.KeyRight}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "delete":
		return tea.KeyPressMsg{Code: tea.KeyDelete}
	case "ctrl+a":
		return tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl}
	case "ctrl+e":
		return tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	case "alt+left":
		return tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt}
	case "alt+right":
		return tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}
	default:
		r := []rune(s)[0]
		return tea.KeyPressMsg{Code: r, Text: s}
	}
}

func send(t *testing.T, m model, msg tea.Msg) (model, tea.Cmd) {
	t.Helper()
	nm, cmd := m.Update(msg)
	return nm.(model), cmd
}

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		m, _ = send(t, m, keyFromString(k))
	}
	return m
}

func typeText(t *testing.T, m model, s string) model {
	t.Helper()
	for _, r := range s {
		if r == ' ' {
			m = press(t, m, "space")
			continue
		}
		m = press(t, m, string(r))
	}
	return m
}

// newTestModel sets up an isolated config dir and a fresh untracked CWD.
func newTestModel(t *testing.T) model {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Chdir(t.TempDir())
	m, err := newModel()
	if err != nil {
		t.Fatal(err)
	}
	m.mode = modeOverview
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 80, Height: 24})
	return m
}

func TestTrackCurrentDirSwitchesToSteps(t *testing.T) {
	m := newTestModel(t)

	items := m.ovItems()
	if len(items) != 1 || items[0].kind != ovTrackCurrent {
		t.Fatalf("expected a single track-current row, got %+v", items)
	}

	m = press(t, m, "enter") // select "track current dir"
	if m.mode != modeSteps {
		t.Fatalf("expected to switch to steps mode, got %v", m.mode)
	}
	if m.projKey != m.curPath {
		t.Fatalf("projKey %q != curPath %q", m.projKey, m.curPath)
	}
	if _, ok := m.st.Projects[m.curPath]; !ok {
		t.Fatal("current dir was not tracked in the store")
	}
}

func TestAddStepThroughInput(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter") // track current dir -> steps

	m = press(t, m, "a") // open add input
	if m.input == nil {
		t.Fatal("expected an active input after pressing a")
	}
	m = typeText(t, m, "buy milk")
	m = press(t, m, "enter") // commit

	p := m.st.Projects[m.curPath]
	if p == nil || len(p.Steps) != 1 || p.Steps[0].Text != "buy milk" {
		t.Fatalf("step not added correctly: %+v", p)
	}
	if m.input != nil {
		t.Fatal("input should close after commit")
	}
}

func TestCrossOffFiresConfettiThenRemoves(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter")      // -> steps
	m = press(t, m, "a")          // add
	m = typeText(t, m, "ship it") //
	m = press(t, m, "enter")      // commit

	if got := len(m.visibleFullIndices()); got != 1 {
		t.Fatalf("expected 1 visible open step, got %d", got)
	}

	// Cross off.
	m, cmd := send(t, m, keyFromString("x"))
	if cmd == nil {
		t.Fatal("cross-off should return an animate command")
	}
	if !m.conf.active() {
		t.Fatal("expected confetti particles after cross-off")
	}
	if m.anim == nil {
		t.Fatal("expected an animation snapshot holding the row in place")
	}
	if p := m.st.Projects[m.curPath]; !p.Steps[0].Done {
		t.Fatal("step should be persisted as done immediately")
	}
	// While animating, the just-crossed row is still shown (no jump). Strip ANSI
	// first: strikethrough styling wraps each rune in its own escape sequence.
	if view := ansi.Strip(m.stepsView()); !strings.Contains(view, "ship it") {
		t.Fatalf("crossed step should remain visible during the burst, got:\n%s", view)
	}

	// Drive frames until the burst settles.
	frames := 0
	for m.conf.active() {
		m, _ = send(t, m, frameMsg{})
		frames++
		if frames > 5000 {
			t.Fatal("animation never finished")
		}
	}
	if m.anim != nil {
		t.Fatal("snapshot should clear once the burst ends")
	}
	if got := len(m.visibleFullIndices()); got != 0 {
		t.Fatalf("done step should drop from the open list, still see %d", got)
	}
}

// setupSteps tracks the current dir and adds the given steps, landing on the
// steps screen with the cursor at the top.
func setupSteps(t *testing.T, steps ...string) model {
	t.Helper()
	m := newTestModel(t)
	m = press(t, m, "enter") // track current dir -> steps
	for _, s := range steps {
		m = press(t, m, "a")
		m = typeText(t, m, s)
		m = press(t, m, "enter")
	}
	return m
}

func stepTexts(m model) []string {
	p := m.st.Projects[m.curPath]
	out := make([]string, len(p.Steps))
	for i, s := range p.Steps {
		out[i] = s.Text
	}
	return out
}

func TestReorderWithShiftDownAndUp(t *testing.T) {
	m := setupSteps(t, "first", "second", "third")
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"first", "second", "third"}) {
		t.Fatalf("setup order wrong: %v", got)
	}

	// Cursor on "first"; move it down twice -> "second","third","first".
	m = press(t, m, "shift+down")
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"second", "first", "third"}) {
		t.Fatalf("after one shift+down: %v", got)
	}
	if m.stCursor != 1 {
		t.Fatalf("cursor should follow moved step to 1, got %d", m.stCursor)
	}
	m = press(t, m, "J") // ASCII fallback for move-down
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"second", "third", "first"}) {
		t.Fatalf("after J: %v", got)
	}

	// Move it back up to the top.
	m = press(t, m, "shift+up", "K")
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"first", "second", "third"}) {
		t.Fatalf("after moving back up: %v", got)
	}
	if m.stCursor != 0 {
		t.Fatalf("cursor should be at 0, got %d", m.stCursor)
	}
}

func TestReorderClampsAtEdges(t *testing.T) {
	m := setupSteps(t, "a", "b")
	m = press(t, m, "shift+up") // already at top -> no-op
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"a", "b"}) {
		t.Fatalf("shift+up at top changed order: %v", got)
	}
	if m.stCursor != 0 {
		t.Fatalf("cursor moved at top edge: %d", m.stCursor)
	}
}

func TestDeleteThenUndoRestoresAtPosition(t *testing.T) {
	m := setupSteps(t, "alpha", "beta", "gamma")
	m = press(t, m, "down") // cursor on "beta" (index 1)
	m = press(t, m, "d")    // delete beta
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"alpha", "gamma"}) {
		t.Fatalf("after delete: %v", got)
	}
	if m.status == "" || len(m.undo) != 1 {
		t.Fatalf("expected an undo entry and status, got status=%q undo=%d", m.status, len(m.undo))
	}

	m = press(t, m, "u") // undo
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"alpha", "beta", "gamma"}) {
		t.Fatalf("after undo, beta should be back at index 1: %v", got)
	}
	if len(m.undo) != 0 {
		t.Fatalf("undo stack should be empty, got %d", len(m.undo))
	}
	if m.stCursor != 1 {
		t.Fatalf("cursor should land on the restored step (1), got %d", m.stCursor)
	}
}

func TestUndoWithEmptyStackIsSafe(t *testing.T) {
	m := setupSteps(t, "only")
	m = press(t, m, "u")
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"only"}) {
		t.Fatalf("undo with nothing to undo changed state: %v", got)
	}
	if m.status != "nothing to undo" {
		t.Fatalf("expected 'nothing to undo', got %q", m.status)
	}
}

func TestToggleReopensDoneStep(t *testing.T) {
	m := setupSteps(t, "task")
	// Cross it off (advance the burst to completion).
	m, _ = send(t, m, keyFromString("x"))
	for m.conf.active() {
		m, _ = send(t, m, frameMsg{})
	}
	if !m.st.Projects[m.curPath].Steps[0].Done {
		t.Fatal("step should be done after cross-off")
	}
	// Show done, then press x on it to reopen — no confetti this time.
	m = press(t, m, "h")
	next, cmd := send(t, m, keyFromString("x"))
	if cmd != nil {
		t.Fatal("reopening a done step should not start an animation")
	}
	m = next
	if m.st.Projects[m.curPath].Steps[0].Done {
		t.Fatal("step should be reopened")
	}
}

func TestUntrackProjectFromOverview(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter") // track current dir -> steps
	m = press(t, m, "esc")   // back to overview

	items := m.ovItems()
	if len(items) != 1 || items[0].kind != ovProject {
		t.Fatalf("after tracking, expected one project row, got %+v", items)
	}

	m = press(t, m, "d") // ask to untrack
	if m.confirm == nil {
		t.Fatal("expected a confirm prompt")
	}
	m = press(t, m, "y") // confirm
	if m.confirm != nil {
		t.Fatal("confirm should clear after answering")
	}
	if len(m.st.Projects) != 0 {
		t.Fatalf("project should be untracked, still have %d", len(m.st.Projects))
	}
}

func TestGroupedOverviewNavigation(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter") // track cwd -> steps
	m = press(t, m, "esc")   // -> overview (one ungrouped project: cwd)

	if _, err := store.Update(func(s *store.Store) error {
		s.Ensure("/tmp/proj-work", "proj-work").Group = "work"
		s.Ensure("/tmp/proj-home", "proj-home").Group = "personal"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.refreshFromDisk()
	m = m.clampOvCursor()

	items := m.ovItems()
	var kinds []ovKind
	for _, it := range items {
		kinds = append(kinds, it.kind)
	}
	want := []ovKind{ovHeader, ovProject, ovHeader, ovProject, ovHeader, ovProject}
	if !reflect.DeepEqual(kinds, want) {
		t.Fatalf("grouped shape = %v, want %v (%+v)", kinds, want, items)
	}
	if items[0].label != "personal" || items[2].label != "work" || items[4].label != "ungrouped" {
		t.Fatalf("header order: %q %q %q", items[0].label, items[2].label, items[4].label)
	}

	// Walking down lands only on selectable project rows.
	for i := 0; i < len(items)+2; i++ {
		if m.ovItems()[m.ovCursor].kind == ovHeader {
			t.Fatalf("cursor on a header at step %d (idx %d)", i, m.ovCursor)
		}
		if m.selectedProject(m.ovItems()) == nil {
			t.Fatalf("selectedProject nil mid-nav at step %d", i)
		}
		m = press(t, m, "down")
	}

	// Forcing the cursor onto a header and clamping moves it off.
	m.ovCursor = 0 // the PERSONAL header
	m = m.clampOvCursor()
	if m.ovItems()[m.ovCursor].kind == ovHeader {
		t.Fatalf("clampOvCursor left cursor on a header: %d", m.ovCursor)
	}
}

func TestAssignGroupFlow(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter") // track cwd -> steps
	m = press(t, m, "esc")   // -> overview, one ungrouped project

	m = press(t, m, "g") // open the group prompt on the project
	if m.input == nil || m.input.prompt != groupPrompt {
		t.Fatalf("g should open the group prompt, got %+v", m.input)
	}
	m = typeText(t, m, "work")
	m = press(t, m, "enter")
	if got := m.st.Projects[m.curPath].Group; got != "work" {
		t.Fatalf("group not set, got %q", got)
	}
	if items := m.ovItems(); items[0].kind != ovHeader || items[0].label != "work" {
		t.Fatalf("expected a WORK header first, got %+v", items)
	}
	if m.ovItems()[m.ovCursor].kind == ovHeader {
		t.Fatal("cursor left on the header after grouping")
	}

	// Re-open (prefilled with "work") and clear it with a blank submit.
	m = press(t, m, "g")
	m = press(t, m, "ctrl+a")
	for i := 0; i < 4; i++ {
		m = press(t, m, "delete")
	}
	m = press(t, m, "enter")
	if got := m.st.Projects[m.curPath].Group; got != "" {
		t.Fatalf("group should be cleared, got %q", got)
	}
}

func TestCopyPathFromOverview(t *testing.T) {
	m := newTestModel(t)
	m = press(t, m, "enter") // track cwd
	m = press(t, m, "esc")   // -> overview on the project
	next, cmd := send(t, m, keyFromString("y"))
	if cmd == nil {
		t.Fatal("overview copy path should return a command")
	}
	if next.status != "copied path" {
		t.Fatalf("expected 'copied path', got %q", next.status)
	}
	if !strings.Contains(ansi.Strip(next.overviewView()), "copied path") {
		t.Fatal("overview should render the status line")
	}
	next = press(t, next, "down") // any key clears it
	if next.status != "" {
		t.Fatalf("status should clear on next key, got %q", next.status)
	}
}

func TestViewsRenderWithoutPanic(t *testing.T) {
	m := newTestModel(t)
	if v := m.View(); v.Content == "" {
		t.Fatal("overview view empty")
	}
	m = press(t, m, "enter") // -> steps (empty)
	if v := m.View(); !strings.Contains(v.Content, "no steps yet") {
		t.Fatal("empty steps view missing hint")
	}
	if !v(m).AltScreen {
		t.Fatal("expected alt-screen to be enabled")
	}
}

// v is a tiny helper so the AltScreen assertion above reads cleanly.
func v(m model) tea.View { return m.View() }

// TestKeyStrings guards that constructed key messages produce the .String()
// values the handlers switch on — cheap insurance against v2 naming drift.
func TestKeyStrings(t *testing.T) {
	want := map[string]string{
		"left": "left", "right": "right", "home": "home", "end": "end",
		"delete": "delete", "ctrl+a": "ctrl+a", "ctrl+e": "ctrl+e",
		"alt+left": "alt+left", "alt+right": "alt+right",
	}
	for in, exp := range want {
		if got := keyFromString(in).String(); got != exp {
			t.Errorf("keyFromString(%q).String() = %q, want %q", in, got, exp)
		}
	}
}

func TestTextInputInsertAndCursor(t *testing.T) {
	ti := &textInput{}
	ti.insert("abc")
	if ti.string() != "abc" || ti.pos != 3 {
		t.Fatalf("insert: %q pos=%d", ti.string(), ti.pos)
	}
	ti.left()
	ti.left()
	ti.insert("X") // a[X]bc, cursor after X
	if ti.string() != "aXbc" || ti.pos != 2 {
		t.Fatalf("mid insert: %q pos=%d", ti.string(), ti.pos)
	}
	ti.insert("Y") // aX[Y]bc — second mid-insert must not corrupt the tail
	if ti.string() != "aXYbc" {
		t.Fatalf("second insert aliased the backing array: %q", ti.string())
	}
}

func TestTextInputDeleteAndBounds(t *testing.T) {
	ti := &textInput{value: []rune("hello"), pos: 5}
	ti.backspace() // hell
	if ti.string() != "hell" || ti.pos != 4 {
		t.Fatalf("backspace: %q pos=%d", ti.string(), ti.pos)
	}
	ti.home()
	ti.deleteFwd() // ell
	if ti.string() != "ell" || ti.pos != 0 {
		t.Fatalf("deleteFwd at home: %q pos=%d", ti.string(), ti.pos)
	}
	ti.backspace() // no-op at start
	if ti.string() != "ell" || ti.pos != 0 {
		t.Fatalf("backspace at start should no-op: %q pos=%d", ti.string(), ti.pos)
	}
	ti.end()
	ti.right() // no-op at end (clamps)
	ti.deleteFwd()
	if ti.string() != "ell" || ti.pos != 3 {
		t.Fatalf("end-of-line ops should clamp: %q pos=%d", ti.string(), ti.pos)
	}
}

func TestTextInputWordJump(t *testing.T) {
	ti := &textInput{value: []rune("foo  bar baz"), pos: 12}
	for _, want := range []int{9, 5, 0} { // baz, bar, foo starts
		ti.wordLeft()
		if ti.pos != want {
			t.Fatalf("wordLeft: pos=%d want %d", ti.pos, want)
		}
	}
	for _, want := range []int{3, 8} { // foo, bar ends
		ti.wordRight()
		if ti.pos != want {
			t.Fatalf("wordRight: pos=%d want %d", ti.pos, want)
		}
	}
}

func TestTextInputViewShowsCursorMidString(t *testing.T) {
	ti := &textInput{prompt: "add:", value: []rune("cat"), pos: 1}
	if got := ansi.Strip(ti.view(80)); !strings.Contains(got, "add: cat") {
		t.Fatalf("view should render full text in order: %q", got)
	}
}

func TestTextInputScrollsWhenTextOutgrowsWindow(t *testing.T) {
	const width = 40
	val := "rewrite the sync layer so the phone and the mac finally agree"
	ti := &textInput{prompt: "add:", value: []rune(val), pos: len([]rune(val))}

	got := ansi.Strip(ti.view(width))
	if w := ansi.StringWidth(got); w > width {
		t.Fatalf("input is %d cells wide in a %d-cell window: %q", w, width, got)
	}
	if !strings.HasSuffix(got, "agree▌") {
		t.Fatalf("the window should follow the cursor to the end: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("the clipped head should be marked: %q", got)
	}

	ti.home() // jumping back scrolls the window with it
	got = ansi.Strip(ti.view(width))
	if w := ansi.StringWidth(got); w > width {
		t.Fatalf("input is %d cells wide after home: %q", w, got)
	}
	if !strings.HasPrefix(strings.TrimLeft(got, " "), "add: rewrite the") {
		t.Fatalf("home should show the head of the value: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Fatalf("the clipped tail should be marked: %q", got)
	}
}

func TestTextInputWindowFollowsBackspace(t *testing.T) {
	const width = 30
	val := strings.Repeat("ab ", 20)
	ti := &textInput{prompt: "add:", value: []rune(val), pos: len([]rune(val))}
	ti.view(width) // scroll out to the end
	if ti.off == 0 {
		t.Fatal("the window should have scrolled off the start")
	}
	for i := 0; i < len([]rune(val)); i++ {
		ti.backspace()
		ti.view(width)
	}
	if ti.off != 0 {
		t.Fatalf("the window should scroll back as the value shrinks, off=%d", ti.off)
	}
}

func TestPasteInsertsAtCursorAndSanitizes(t *testing.T) {
	m := setupSteps(t)
	m = press(t, m, "a")
	m = typeText(t, m, "hi")
	m, _ = send(t, m, tea.PasteMsg{Content: "there"})
	if m.input.string() != "hithere" {
		t.Fatalf("paste at cursor: %q", m.input.string())
	}
	m, _ = send(t, m, tea.PasteMsg{Content: "a\nb\tc"})
	if strings.ContainsAny(m.input.string(), "\n\t") {
		t.Fatalf("paste should sanitize newlines/tabs: %q", m.input.string())
	}
}

func TestPasteWithoutInputIsNoop(t *testing.T) {
	m := setupSteps(t, "one")
	before := stepTexts(m)
	m, _ = send(t, m, tea.PasteMsg{Content: "junk"})
	if got := stepTexts(m); !reflect.DeepEqual(got, before) {
		t.Fatalf("paste without an active input changed state: %v", got)
	}
}

func TestCopyPathFromSteps(t *testing.T) {
	m := setupSteps(t, "one")
	next, cmd := send(t, m, keyFromString("y"))
	if cmd == nil {
		t.Fatal("copy path should return a SetClipboard command")
	}
	if next.status != "copied path" {
		t.Fatalf("expected 'copied path' status, got %q", next.status)
	}
	// status clears on the next key
	next = press(t, next, "down")
	if next.status != "" {
		t.Fatalf("status should clear on next key, got %q", next.status)
	}
}

func TestOpenStepsAreNumbered(t *testing.T) {
	m := setupSteps(t, "alpha", "beta", "gamma")
	view := ansi.Strip(m.stepsView())
	for _, want := range []string{"1. alpha", "2. beta", "3. gamma"} {
		if !strings.Contains(view, want) {
			t.Fatalf("numbered list missing %q in:\n%s", want, view)
		}
	}
	// Cross off the top step and let the burst settle; the rest renumber.
	m, _ = send(t, m, keyFromString("x"))
	for m.conf.active() {
		m, _ = send(t, m, frameMsg{})
	}
	view = ansi.Strip(m.stepsView())
	for _, want := range []string{"1. beta", "2. gamma"} {
		if !strings.Contains(view, want) {
			t.Fatalf("after cross-off, renumber missing %q in:\n%s", want, view)
		}
	}
}

func TestToggleImportant(t *testing.T) {
	m := setupSteps(t, "ship the thing", "later")
	m = press(t, m, "i") // mark step 1 important
	if !m.st.Projects[m.curPath].Steps[0].Important {
		t.Fatal("step 0 should be important after pressing i")
	}
	if m.st.Projects[m.curPath].Steps[1].Important {
		t.Fatal("only the selected step should be important")
	}
	view := ansi.Strip(m.stepsView())
	if !strings.Contains(view, "! 1. ship the thing") {
		t.Fatalf("important step should render a !:\n%s", view)
	}
	// Importance must not reorder.
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"ship the thing", "later"}) {
		t.Fatalf("importance reordered the list: %v", got)
	}
	m = press(t, m, "i") // toggle off
	if m.st.Projects[m.curPath].Steps[0].Important {
		t.Fatal("pressing i again should clear importance")
	}
}

func TestMoveToTop(t *testing.T) {
	m := setupSteps(t, "a", "b", "c")
	m = press(t, m, "down", "down") // cursor on "c"
	m = press(t, m, "t")            // bump to #1
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("move-to-top: %v", got)
	}
	if m.stCursor != 0 {
		t.Fatalf("cursor should follow to the top, got %d", m.stCursor)
	}
	m = press(t, m, "t") // already at top -> no-op
	if got := stepTexts(m); !reflect.DeepEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("move-to-top at top changed order: %v", got)
	}
}

func TestStepRowWrapsWithHangingIndent(t *testing.T) {
	const width = 40
	r := displayRow{text: "rewrite the sync layer so the phone and the mac finally agree", num: 7}
	lines := r.render("> ", width)
	if len(lines) < 2 {
		t.Fatalf("long text should wrap, got %d line(s): %q", len(lines), lines)
	}
	for _, ln := range lines {
		if w := ansi.StringWidth(ln); w > width {
			t.Fatalf("line is %d cells wide in a %d-cell window: %q", w, width, ansi.Strip(ln))
		}
	}
	col := r.textCol()
	for _, ln := range lines[1:] {
		plain := ansi.Strip(ln)
		if got := len(plain) - len(strings.TrimLeft(plain, " ")); got != col {
			t.Fatalf("continuation should hang at column %d, got %d: %q", col, got, plain)
		}
	}
	var parts []string
	for _, ln := range lines {
		parts = append(parts, strings.TrimSpace(ansi.Strip(ln)))
	}
	if got := strings.Join(parts, " "); !strings.HasSuffix(got, r.text) {
		t.Fatalf("the wrap changed the text: %q", got)
	}
}

// Nothing the views draw may spill past the window: the terminal would hard-wrap
// it mid-word and the confetti row math would drift.
func TestViewsFitNarrowWindows(t *testing.T) {
	long := "rewrite the sync layer so the phone and the mac finally agree on ranks"
	for _, width := range []int{24, 34, 40, 55, 56, 80} {
		m := setupSteps(t, long, "short one")
		m.status = "deleted " + strconv.Quote(long) + " — press u to undo"
		m.syncInfo = "offline — changes queued"
		m, _ = send(t, m, tea.WindowSizeMsg{Width: width, Height: 24})

		views := map[string]string{"steps": m.stepsView(), "overview": m.overviewView()}
		mi := press(t, m, "a") // the add input, with a value longer than the window
		mi = typeText(t, mi, long)
		views["steps+input"] = mi.stepsView()
		empty := press(t, newTestModel(t), "enter") // tracked project, no steps yet
		empty, _ = send(t, empty, tea.WindowSizeMsg{Width: width, Height: 24})
		views["steps+empty"] = empty.stepsView()

		for name, v := range views {
			for i, ln := range strings.Split(v, "\n") {
				if w := ansi.StringWidth(ln); w > width {
					t.Fatalf("%s at width %d: line %d is %d cells: %q", name, width, i, w, ansi.Strip(ln))
				}
			}
		}
	}
}

func TestOverviewRowKeepsCountOnTheNameLine(t *testing.T) {
	m := setupSteps(t, "one")
	m = press(t, m, "esc") // back to the overview
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 34, Height: 24})
	view := ansi.Strip(m.overviewView())
	var found bool
	for _, ln := range strings.Split(view, "\n") {
		if strings.Contains(ln, "1 open") {
			found = true
			if !strings.Contains(ln, "(here)") {
				t.Fatalf("count and marker should share the name's line: %q", ln)
			}
		}
	}
	if !found {
		t.Fatalf("overview should show the open count:\n%s", view)
	}
}

// The burst is drawn at an absolute screen row, so a wrapped row above the
// crossed-off one has to push it down.
func TestConfettiRowFollowsWrappedRowsAbove(t *testing.T) {
	long := "rewrite the sync layer so the phone and the mac finally agree on ranks"
	m := setupSteps(t, long, "second")
	m, _ = send(t, m, tea.WindowSizeMsg{Width: 40, Height: 24})
	m = press(t, m, "down") // cursor on "second"

	above := len(m.liveStepRows()[0].render("  ", m.width))
	if above < 2 {
		t.Fatalf("the first row should wrap at width 40, got %d line(s)", above)
	}
	want := m.stepsTopRows() + above

	m, _ = send(t, m, keyFromString("x"))
	if len(m.conf.particles) == 0 {
		t.Fatal("crossing off should fire a burst")
	}
	if got := int(m.conf.particles[0].physics.Position().Y); got != want {
		t.Fatalf("burst row %d, want %d", got, want)
	}
	lines := strings.Split(ansi.Strip(m.stepsView()), "\n")
	if want >= len(lines) || !strings.Contains(lines[want], "second") {
		t.Fatalf("row %d of the view is not the crossed step:\n%s", want, strings.Join(lines, "\n"))
	}
}
