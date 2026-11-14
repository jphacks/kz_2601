package calls

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// TestMemoryCreate: a created session reads back, the same id twice is an
// error, and a session of another app is not in the room listing.
func TestMemoryCreate(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC)
	st := NewMemory()

	blocks := []Block{{HoldID: "h1", Amount: 30}}
	s := Session{
		ID: "s1", UID: "u1", AppID: "app1", Room: RoomName("app1", "lobby"), Video: true,
		Blocks: blocks, CreatedAt: now, DeleteAt: now.Add(Retention),
	}
	if err := st.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := st.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.ID != "s1" || got.UID != "u1" || got.AppID != "app1" || got.Room != "app:app1:lobby" || !got.Video {
		t.Errorf("read back %+v", got)
	}
	if !got.CreatedAt.Equal(now) || !got.DeleteAt.Equal(now.Add(Retention)) {
		t.Errorf("times: created %v, deleteAt %v", got.CreatedAt, got.DeleteAt)
	}
	if len(got.Blocks) != 1 || got.Blocks[0] != (Block{HoldID: "h1", Amount: 30}) {
		t.Errorf("blocks %+v", got.Blocks)
	}
	if got.Settled || got.Minutes != 0 || got.Charges != nil {
		t.Errorf("a new session must not be settled: %+v", got)
	}

	// The store keeps a copy: neither the caller's slice nor the read value
	// may reach into it.
	blocks[0].Amount = 999
	got.Blocks[0].HoldID = "stomped"
	again, err := st.Get(ctx, "s1")
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if again.Blocks[0] != (Block{HoldID: "h1", Amount: 30}) {
		t.Errorf("blocks were shared with the caller: %+v", again.Blocks)
	}

	// The same id a second time is an error and leaves the first one as it is.
	dup := Session{ID: "s1", UID: "u2", AppID: "app1", Room: RoomName("app1", "lobby"), CreatedAt: now.Add(time.Minute)}
	err = st.Create(ctx, dup)
	if err == nil {
		t.Fatal("the same id twice must be an error")
	}
	if errors.Is(err, ErrNotFound) {
		t.Errorf("a duplicate is not ErrNotFound: %v", err)
	}
	if kept, _ := st.Get(ctx, "s1"); kept.UID != "u1" {
		t.Errorf("the first session must stay: %+v", kept)
	}

	// Another app's session lives in another room, so it is not listed with
	// the first app's (RoomName namespaces the room by app).
	other := Session{
		ID: "s2", UID: "u1", AppID: "app2", Room: RoomName("app2", "lobby"), CreatedAt: now.Add(time.Minute),
		DeleteAt: now.Add(Retention),
	}
	if err := st.Create(ctx, other); err != nil {
		t.Fatalf("create other app: %v", err)
	}
	if other.Room == s.Room {
		t.Fatalf("the two apps must not share a room name: %s", s.Room)
	}
	for _, c := range []struct {
		room string
		want string
	}{
		{s.Room, "s1"},
		{other.Room, "s2"},
		{RoomName("app3", "lobby"), ""},
	} {
		open, err := st.Open(ctx, c.room)
		if err != nil {
			t.Fatalf("open %s: %v", c.room, err)
		}
		var ids []string
		for _, o := range open {
			ids = append(ids, o.ID)
		}
		if got := strings.Join(ids, ","); got != c.want {
			t.Errorf("open %s = %q, want %q", c.room, got, c.want)
		}
	}
}
