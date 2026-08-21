package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// drain runs a cmd (possibly a batch) and returns every message it yields.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drain(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

func TestSyncOfflineOutcomeShowsQuietly(t *testing.T) {
	m := newTestModel(t)
	m.syncEnabled = true
	next, _ := m.Update(syncDoneMsg{ran: true, err: errors.New("dial tcp: nope")})
	m = next.(model)
	if m.syncInfo != "offline — changes queued" {
		t.Fatalf("syncInfo = %q", m.syncInfo)
	}
	if m.err != nil {
		t.Fatal("a failed sync must never surface as a hard error")
	}
}

func TestSyncPendingRunsOnceMore(t *testing.T) {
	m := newTestModel(t)
	m.syncEnabled = true
	m.syncing = true
	// A debounce fires while a sync is in flight: remember, don't double-run.
	next, cmd := m.Update(syncDebounceMsg{seq: m.syncSeq})
	m = next.(model)
	if cmd != nil || !m.syncPending {
		t.Fatalf("in-flight debounce mishandled: cmd=%v pending=%v", cmd, m.syncPending)
	}
	// When the in-flight sync lands, the pending one starts.
	next, cmd = m.Update(syncDoneMsg{ran: true})
	m = next.(model)
	if cmd == nil || !m.syncing || m.syncPending {
		t.Fatalf("pending sync not started: cmd=%v syncing=%v", cmd, m.syncing)
	}
}

func TestStaleDebounceIgnored(t *testing.T) {
	m := newTestModel(t)
	m.syncEnabled = true
	m.syncSeq = 5
	if _, cmd := m.Update(syncDebounceMsg{seq: 3}); cmd != nil {
		t.Fatal("stale debounce must not trigger a sync")
	}
}

func TestQuitFlushesThenQuits(t *testing.T) {
	m := newTestModel(t)
	m.syncEnabled = true
	next, cmd := m.handleOverviewKey(keyFromString("q"))
	m = next.(model)
	if !m.quitting || cmd == nil {
		t.Fatalf("q with sync enabled must start the flush, got quitting=%v", m.quitting)
	}
	// The flush completes -> quit.
	_, cmd = m.Update(syncDoneMsg{ran: true, err: errors.New("still offline")})
	msgs := drain(cmd)
	for _, msg := range msgs {
		if _, ok := msg.(tea.QuitMsg); ok {
			return
		}
	}
	t.Fatalf("no QuitMsg after flush, got %T", msgs)
}

func TestQuitWithoutSyncQuitsImmediately(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.handleOverviewKey(keyFromString("q"))
	for _, msg := range drain(cmd) {
		if _, ok := msg.(tea.QuitMsg); ok {
			return
		}
	}
	t.Fatal("q without sync must quit at once")
}
