package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestMemoryReleaseSnapshot_S7Backend008(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	m := NewMemory()
	dsl := json.RawMessage(`{"initialScreen":"home"}`)
	created, ok, err := m.CreateReleaseRequest(ctx, ReleaseRequest{AppID: "A1", OwnerUID: "u1", Name: "N", Version: 2, DSL: dsl, Purpose: "p", Contact: "c@example.com"})
	if err != nil || !ok {
		t.Fatalf("create: %v %v", ok, err)
	}
	if created.DSL != nil {
		t.Fatal("the create result must not carry the DSL")
	}

	t.Run("returns the stored request with its DSL", func(t *testing.T) {
		t.Parallel()
		snap, err := m.ReleaseSnapshot(created.ID)
		if err != nil {
			t.Fatal(err)
		}
		if string(snap.DSL) != string(dsl) || snap.AppID != "A1" || snap.OwnerUID != "u1" || snap.Version != 2 || snap.Status != ReleaseReceived {
			t.Fatalf("snapshot %+v", snap)
		}
	})

	t.Run("a copy never changes the stored one", func(t *testing.T) {
		t.Parallel()
		m2 := NewMemory()
		rr, _, err := m2.CreateReleaseRequest(ctx, ReleaseRequest{AppID: "A2", OwnerUID: "u2", DSL: json.RawMessage(`{"a":1}`)})
		if err != nil {
			t.Fatal(err)
		}
		first, err := m2.ReleaseSnapshot(rr.ID)
		if err != nil {
			t.Fatal(err)
		}
		first.DSL[2] = 'X' // the bytes of the copy
		first.Name, first.Status = "changed", "hacked"
		second, err := m2.ReleaseSnapshot(rr.ID)
		if err != nil {
			t.Fatal(err)
		}
		if string(second.DSL) != `{"a":1}` || second.Name != "" || second.Status != ReleaseReceived {
			t.Fatalf("stored request was changed through the copy: %+v %s", second, second.DSL)
		}
	})

	t.Run("an unknown id is ErrNotFound", func(t *testing.T) {
		t.Parallel()
		if _, err := m.ReleaseSnapshot("nope"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
		if _, err := NewMemory().ReleaseSnapshot(""); !errors.Is(err, ErrNotFound) {
			t.Fatalf("empty store: %v", err)
		}
	})
}

func TestMemorySetFeedbackCreatedAt_S7Backend008(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("backdates createdAt and the day (Japan time)", func(t *testing.T) {
		t.Parallel()
		m := NewMemory()
		fb, err := m.CreateFeedback(ctx, Feedback{UID: "u1"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		// 16:00 UTC on the 1st is 01:00 on the 2nd in Japan.
		old := time.Date(2026, 10, 1, 16, 0, 0, 0, time.UTC)
		if err := m.SetFeedbackCreatedAt(fb.ID, old); err != nil {
			t.Fatal(err)
		}
		got, err := m.ListFeedback(ctx, FeedbackQuery{})
		if err != nil || len(got) != 1 {
			t.Fatalf("%v %v", got, err)
		}
		if !got[0].CreatedAt.Equal(old) || got[0].Day != "2026-10-02" {
			t.Fatalf("createdAt %v day %q", got[0].CreatedAt, got[0].Day)
		}
	})

	t.Run("a backdated document is out of a later Since", func(t *testing.T) {
		t.Parallel()
		m := NewMemory()
		fb, err := m.CreateFeedback(ctx, Feedback{UID: "u1"}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if err := m.SetFeedbackCreatedAt(fb.ID, time.Now().Add(-48*time.Hour)); err != nil {
			t.Fatal(err)
		}
		got, err := m.ListFeedback(ctx, FeedbackQuery{Since: time.Now().Add(-time.Hour)})
		if err != nil || len(got) != 0 {
			t.Fatalf("%v %v", got, err)
		}
	})

	t.Run("an unknown id is ErrNotFound", func(t *testing.T) {
		t.Parallel()
		if err := NewMemory().SetFeedbackCreatedAt("nope", time.Now()); !errors.Is(err, ErrNotFound) {
			t.Fatalf("got %v", err)
		}
	})
}
