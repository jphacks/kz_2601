package store

import (
	"context"
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
