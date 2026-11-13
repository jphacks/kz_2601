package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

const (
	watchWait  = 10 * time.Second       // a change must arrive within this
	watchQuiet = 300 * time.Millisecond // no signal expected within this
)

func watcherOf(t *testing.T, s Store) Watcher {
	t.Helper()
	w, ok := s.(Watcher)
	if !ok {
		t.Fatalf("%T is not a Watcher", s)
	}
	return w
}

// waitVersion reads until v arrives (a listener may repeat the current value).
func waitVersion(t *testing.T, ch <-chan int, v int) {
	t.Helper()
	timeout := time.After(watchWait)
	for {
		select {
		case got, ok := <-ch:
			if !ok {
				t.Fatalf("closed before version %d", v)
			}
			if got == v {
				return
			}
			if got > v {
				t.Fatalf("got version %d, want %d", got, v)
			}
		case <-timeout:
			t.Fatalf("no version %d within %s", v, watchWait)
		}
	}
}

func waitClosed[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	timeout := time.After(watchWait)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-timeout:
			t.Fatalf("not closed within %s after cancel", watchWait)
		}
	}
}

func TestStoreWatchApp(t *testing.T) {
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		w := watcherOf(t, s)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		uid, appID := "wa-user"+sfx, "WATCH"+sfx
		initial := json.RawMessage(`{}`)

		if _, err := w.WatchApp(ctx, uid, appID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("unknown app: want ErrNotFound, got %v", err)
		}
		if _, _, err := s.EnsureApp(ctx, uid, appID, initial, "x"); err != nil {
			t.Fatal(err)
		}
		if _, err := w.WatchApp(ctx, "wa-other"+sfx, appID); !errors.Is(err, ErrNotFound) {
			t.Fatalf("other user's tree: want ErrNotFound, got %v", err)
		}

		ch, err := w.WatchApp(ctx, uid, appID)
		if err != nil {
			t.Fatal(err)
		}
		select {
		case v := <-ch:
			if v != 1 {
				t.Fatalf("initial version %d", v)
			}
		case <-time.After(watchWait):
			t.Fatal("no initial version")
		}
		for v := 2; v <= 3; v++ {
			if _, err := s.CommitVersion(ctx, uid, appID, Commit{BaseVersion: v - 1, DSL: initial}); err != nil {
				t.Fatal(err)
			}
			waitVersion(t, ch, v)
		}

		// a watcher that starts later sees the current version first
		ch2, err := w.WatchApp(ctx, uid, appID)
		if err != nil {
			t.Fatal(err)
		}
		waitVersion(t, ch2, 3)

		cancel()
		waitClosed(t, ch)
		waitClosed(t, ch2)
	})
}

func TestStoreWatchRecords(t *testing.T) {
	t.Parallel() // waits out real quiet timers
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		w := watcherOf(t, s)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rs := PublicRecords("WATCHREC"+sfx, "post")
		other := PublicRecords(rs.AppID, "other")
		if _, _, err := s.CreateRecord(ctx, rs, Record{ID: "old", Entity: rs.Entity, Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}

		ch, err := w.WatchRecords(ctx, rs)
		if err != nil {
			t.Fatal(err)
		}
		quiet := func(what string) {
			t.Helper()
			select {
			case _, ok := <-ch:
				t.Fatalf("%s: unexpected signal (open=%v)", what, ok)
			case <-time.After(watchQuiet):
			}
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
			// a burst may leave one more pending signal: drain it
			select {
			case <-ch:
			case <-time.After(watchQuiet):
			}
		}

		quiet("the initial state is not a change")
		for _, step := range []struct {
			name string
			do   func() error
		}{
			{"create", func() error {
				_, _, err := s.CreateRecord(ctx, rs, Record{ID: "r1", Entity: rs.Entity, Data: map[string]any{"v": 1}})
				return err
			}},
			{"update", func() error {
				_, err := s.UpdateRecord(ctx, rs, "r1", map[string]any{"v": 2})
				return err
			}},
			{"mark", func() error {
				_, err := s.SetMark(ctx, rs, "r1", "like", "wr-user"+sfx, true)
				return err
			}},
			{"delete", func() error { return s.DeleteRecord(ctx, rs, "r1") }},
		} {
			if err := step.do(); err != nil {
				t.Fatalf("%s: %v", step.name, err)
			}
			signal(step.name)
		}

		if _, _, err := s.CreateRecord(ctx, other, Record{ID: "x", Entity: other.Entity, Data: map[string]any{}}); err != nil {
			t.Fatal(err)
		}
		quiet("another entity's change")

		cancel()
		waitClosed(t, ch)
	})
}

// WatchRecords only sees the watchRecordsWindow most recently updated records:
// a mark or a delete on an older record is not signalled, an update brings the
// record back into the window (#553: an intentional limit, same in both stores).
func TestStoreWatchRecordsWindow(t *testing.T) {
	t.Parallel() // waits out real quiet timers
	bothStores(t, func(t *testing.T, s Store, sfx string) {
		w := watcherOf(t, s)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		rs := PublicRecords("WATCHWIN"+sfx, "post")
		uid := "ww-user" + sfx
		// "a-old" sorts below "r..." on an updatedAt tie, so it is the oldest either way
		ids := []string{"a-old"}
		for i := range watchRecordsWindow + 1 {
			ids = append(ids, fmt.Sprintf("r%03d", i))
		}
		for _, id := range ids {
			if _, _, err := s.CreateRecord(ctx, rs, Record{ID: id, Entity: rs.Entity, Data: map[string]any{}}); err != nil {
				t.Fatal(err)
			}
		}

		ch, err := w.WatchRecords(ctx, rs)
		if err != nil {
			t.Fatal(err)
		}
		quiet := func(what string) {
			t.Helper()
			select {
			case _, ok := <-ch:
				t.Fatalf("%s: unexpected signal (open=%v)", what, ok)
			case <-time.After(watchQuiet):
			}
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
			select {
			case <-ch:
			case <-time.After(watchQuiet):
			}
		}
		step := func(name string, err error) {
			t.Helper()
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}

		quiet("the initial state is not a change")
		_, err = s.SetMark(ctx, rs, "a-old", "like", uid, true)
		step("mark outside the window", err)
		quiet("mark outside the window")
		step("delete outside the window", s.DeleteRecord(ctx, rs, "a-old"))
		quiet("delete outside the window")

		_, err = s.SetMark(ctx, rs, "r100", "like", uid, true)
		step("mark inside the window", err)
		signal("mark inside the window")

		// r000 is now the oldest of 101: outside until an update brings it back
		_, err = s.SetMark(ctx, rs, "r000", "like", uid, true)
		step("mark on the oldest", err)
		quiet("mark on the oldest")
		_, err = s.UpdateRecord(ctx, rs, "r000", map[string]any{"v": 1})
		step("update the oldest", err)
		signal("update the oldest")
		_, err = s.SetMark(ctx, rs, "r000", "like", uid, false)
		step("mark after the update", err)
		signal("mark after the update")
	})
}
