package store

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// probe records the ids the visible func was asked about, so a test knows
// the watcher has reached a change without sleeping.
type probe struct {
	mu   sync.Mutex
	seen map[string]bool
	tick chan struct{}
}

func newProbe() *probe { return &probe{seen: map[string]bool{}, tick: make(chan struct{}, 1)} }

// visible: ids starting "h-" are hidden from the watcher.
func (p *probe) visible(r *Record) bool {
	p.mu.Lock()
	p.seen[r.ID] = true
	p.mu.Unlock()
	offer(p.tick, struct{}{})
	return !strings.HasPrefix(r.ID, "h-")
}

// wait returns once visible has been called with id (watchWait is only a guard).
func (p *probe) wait(t *testing.T, id string) {
	t.Helper()
	timeout := time.After(watchWait)
	for {
		p.mu.Lock()
		ok := p.seen[id]
		p.mu.Unlock()
		if ok {
			return
		}
		select {
		case <-p.tick:
		case <-timeout:
			t.Fatalf("watcher never looked at %q within %s", id, watchWait)
		}
	}
}

func visibleWatcherOf(t *testing.T, s Store) VisibleWatcher {
	t.Helper()
	w, ok := s.(VisibleWatcher)
	if !ok {
		t.Fatalf("%T is not a VisibleWatcher", s)
	}
	return w
}

// #271: a change of a visible record signals; changes of hidden records do
// not; cancelling ctx closes the channel.
func TestStoreWatchVisibleRecords(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		w := visibleWatcherOf(t, s)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rs := PublicRecords("WATCHVIS"+sfx, "msg")
		uid := "wv-user" + sfx
		create := func(id string) error {
			_, _, err := s.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Data: map[string]any{"v": 1}})
			return err
		}
		if err := create("v-old"); err != nil {
			t.Fatal(err)
		}

		p := newProbe()
		ch, err := w.WatchVisibleRecords(ctx, rs, p.visible)
		if err != nil {
			t.Fatal(err)
		}
		signal := func(what string) {
			t.Helper()
			select {
			case _, ok := <-ch:
				if !ok {
					t.Fatalf("%s: closed", what)
				}
			case <-time.After(watchWait):
				t.Fatalf("%s: no signal within %s", what, watchWait)
			}
		}

		for _, step := range []struct {
			name string
			do   func() error
		}{
			{"create visible", func() error { return create("v-1") }},
			{"update visible", func() error {
				_, err := s.UpdateRecord(ctx, rs, "v-1", map[string]any{"v": 2})
				return err
			}},
			{"mark visible", func() error {
				_, err := s.SetMark(ctx, rs, "v-1", "like", uid, true)
				return err
			}},
			{"delete visible", func() error { return s.DeleteRecord(ctx, rs, "v-1") }},
		} {
			if err := step.do(); err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
			signal(step.name)
		}

		// barrier: once the watcher has looked at h-0, every visible change
		// above is done; drop a signal one of them may have left.
		if err := create("h-0"); err != nil {
			t.Fatal(err)
		}
		p.wait(t, "h-0")
		select {
		case <-ch:
		default:
		}

		for _, step := range []struct {
			name string
			do   func() error
		}{
			{"update hidden", func() error {
				_, err := s.UpdateRecord(ctx, rs, "h-0", map[string]any{"v": 2})
				return err
			}},
			{"mark hidden", func() error {
				_, err := s.SetMark(ctx, rs, "h-0", "like", uid, true)
				return err
			}},
			{"delete hidden", func() error { return s.DeleteRecord(ctx, rs, "h-0") }},
			{"create hidden", func() error { return create("h-end") }},
		} {
			if err := step.do(); err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
		}
		// the watcher has reached the last hidden change; after cancel the
		// channel closes only when its goroutine is done, so draining it
		// counts every signal the hidden changes could have sent.
		p.wait(t, "h-end")
		cancel()
		n := 0
		timeout := time.After(watchWait)
		for open := true; open; {
			select {
			case _, ok := <-ch:
				if ok {
					n++
				}
				open = ok
			case <-timeout:
				t.Fatalf("not closed within %s after cancel", watchWait)
			}
		}
		if n != 0 {
			t.Fatalf("hidden changes sent %d signal(s)", n)
		}
	})
}

// #2771: the fingerprint of the watch window (id, updatedAt, mark counts):
// hidden records are out, a visible mark or update moves it, and the record
// past the window does not count.
func TestVisibleState(t *testing.T) {
	m := NewMemory()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { at = at.Add(time.Minute); return at }
	ctx := t.Context()
	rs := PublicRecords("VISSTATE", "msg")
	vis := func(r *Record) bool { return !strings.HasPrefix(r.ID, "h-") }
	create := func(id string) {
		t.Helper()
		if _, _, err := m.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Data: map[string]any{"v": 1}}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	mark := func(id string, on bool) {
		t.Helper()
		if _, err := m.SetMark(ctx, rs, id, "like", "u-1", on); err != nil {
			t.Fatalf("mark %s: %v", id, err)
		}
	}
	update := func(id string) {
		t.Helper()
		if _, err := m.UpdateRecord(ctx, rs, id, map[string]any{"v": 2}); err != nil {
			t.Fatalf("update %s: %v", id, err)
		}
	}

	create("v-1")
	create("h-1")
	base := m.visibleState(rs, vis)
	if !strings.Contains(base, "v-1@") || strings.Contains(base, "h-1@") {
		t.Fatalf("only the visible record is in the print: %q", base)
	}

	// a hidden record's update and mark leave the print as it was
	update("h-1")
	mark("h-1", true)
	if got := m.visibleState(rs, vis); got != base {
		t.Errorf("hidden change moved the print:\n got %q\nwant %q", got, base)
	}

	// a mark on a visible record moves it, and its count is in the print
	mark("v-1", true)
	marked := m.visibleState(rs, vis)
	if marked == base {
		t.Errorf("a visible mark did not move the print: %q", marked)
	}
	if !strings.Contains(marked, "#like=1") {
		t.Errorf("the mark count is in the print: %q", marked)
	}
	mark("v-1", false)
	if got := m.visibleState(rs, vis); got == marked || !strings.Contains(got, "#like=0") {
		t.Errorf("undoing the mark moves the print back to a count of 0: %q", got)
	}

	// an update moves it too (updatedAt), and a delete empties it
	before := m.visibleState(rs, vis)
	update("v-1")
	if got := m.visibleState(rs, vis); got == before {
		t.Errorf("an update of a visible record did not move the print: %q", got)
	}
	if err := m.DeleteRecord(ctx, rs, "v-1"); err != nil {
		t.Fatal(err)
	}
	if got := m.visibleState(rs, vis); got != "" {
		t.Errorf("with only hidden records the print is empty: %q", got)
	}

	// a record set that is not there at all (no such app) has no print
	if got := m.visibleState(PrivateRecords("no-user", "no-app", "msg"), vis); got != "" {
		t.Errorf("an unreadable record set has no print: %q", got)
	}
}

// #2771: only the newest watchRecordsWindow records are in the print.
func TestVisibleStateWatchWindow(t *testing.T) {
	m := NewMemory()
	at := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.now = func() time.Time { at = at.Add(time.Minute); return at }
	ctx := t.Context()
	rs := PublicRecords("VISWINDOW", "msg")
	vis := func(*Record) bool { return true }
	for i := 0; i <= watchRecordsWindow; i++ { // one more than the window
		id := fmt.Sprintf("v-%03d", i)
		if _, _, err := m.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Data: map[string]any{"v": i}}); err != nil {
			t.Fatalf("create %s: %v", id, err)
		}
	}
	state := m.visibleState(rs, vis)
	if n := len(strings.Split(state, "|")); n != watchRecordsWindow {
		t.Errorf("the print has %d records, want %d", n, watchRecordsWindow)
	}
	if strings.Contains(state, "v-000@") {
		t.Error("the oldest record is past the window and out of the print")
	}
	if !strings.Contains(state, fmt.Sprintf("v-%03d@", watchRecordsWindow)) {
		t.Error("the newest record is in the print")
	}

	// a mark on the record past the window does not move the print
	// (SetMark keeps updatedAt, so it stays out)
	if _, err := m.SetMark(ctx, rs, "v-000", "like", "u-1", true); err != nil {
		t.Fatal(err)
	}
	if got := m.visibleState(rs, vis); got != state {
		t.Error("a change past the window moved the print")
	}
}
