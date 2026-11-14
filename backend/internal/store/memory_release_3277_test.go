package store

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

// ReleaseSnapshot returns the stored DSL, and editing the copy leaves the
// stored request alone.
func TestMemoryReleaseSnapshot_3277(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	dsl := json.RawMessage(`{"name":"家計簿"}`)
	rr, created, err := m.CreateReleaseRequest(ctx, ReleaseRequest{AppID: "app1", OwnerUID: "u1", Name: "家計簿", DSL: dsl})
	if err != nil || !created {
		t.Fatalf("create: %v created=%v", err, created)
	}
	if rr.DSL != nil {
		t.Fatalf("create returned DSL %s, want nil", rr.DSL)
	}

	snap, err := m.ReleaseSnapshot(rr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(snap.DSL) != string(dsl) || snap.ID != rr.ID || snap.Status != ReleaseReceived {
		t.Fatalf("snapshot = %+v", snap)
	}

	snap.DSL[0] = 'X'
	snap.Name, snap.Status = "changed", "changed"
	again, err := m.ReleaseSnapshot(rr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if string(again.DSL) != string(dsl) || again.Name != "家計簿" || again.Status != ReleaseReceived {
		t.Fatalf("store changed through the copy: %+v", again)
	}
}

func TestMemoryReleaseSnapshotMissing_3277(t *testing.T) {
	m := NewMemory()
	if _, err := m.ReleaseSnapshot("rr_nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

// SetFeedbackCreatedAt moves CreatedAt and Day together, so ListFeedback's
// Since filter sees the backdated time.
func TestMemorySetFeedbackCreatedAt_3277(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	fb, err := m.CreateFeedback(ctx, Feedback{UID: "u1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	old := time.Date(2026, 1, 2, 20, 0, 0, 0, time.UTC) // 2026-01-03 in JST
	if err := m.SetFeedbackCreatedAt(fb.ID, old); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetFeedback(ctx, fb.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CreatedAt.Equal(old) || got.Day != "2026-01-03" {
		t.Fatalf("got CreatedAt=%s Day=%s", got.CreatedAt, got.Day)
	}
	list, err := m.ListFeedback(ctx, FeedbackQuery{Since: old.Add(time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 0 {
		t.Fatalf("listed %d after backdating, want 0", len(list))
	}
}

func TestMemorySetFeedbackCreatedAtMissing_3277(t *testing.T) {
	m := NewMemory()
	if err := m.SetFeedbackCreatedAt("fb_nope", time.Now()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
