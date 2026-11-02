package store

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestMemoryFeedback(t *testing.T) { testFeedback(t, NewMemory(), "") }

// Runs against the emulator only (FIRESTORE_EMULATOR_HOST=...); never staging/prod.
func TestFirestoreFeedback(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set")
	}
	f, err := NewFirestore(context.Background(), "demo-kz2601", "(default)")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	testFeedback(t, f, time.Now().Format("150405.000000"))
}

type feedbackStore interface {
	Feedbacks
	FeedbackReader
	FeedbackPurger
}

func testFeedback(t *testing.T, s feedbackStore, sfx string) {
	ctx := context.Background()
	uid := "fb-user" + sfx
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 1, 2, 3}
	first, err := s.CreateFeedback(ctx, Feedback{UID: uid, Kind: FeedbackBug, Message: "m1", ConsentVersion: "v1",
		AppID: "A" + sfx, DSLVersion: 3, DSL: []byte(`{"initialScreen":"home"}`), Screenshot: jpeg,
		Device: &FeedbackDevice{AppVersion: "1.0", Build: "12", OS: "iOS 26.1", Model: "iPhone17,1"}}, 2)
	if err != nil || first.Status != FeedbackReceived || first.Day != FeedbackDay(time.Now()) || first.Screenshot != nil {
		t.Fatalf("create: %+v %v", first, err)
	}
	if _, err := s.CreateFeedback(ctx, Feedback{UID: uid, Message: "m2", ConsentVersion: "v1"}, 2); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFeedback(ctx, Feedback{UID: uid, Message: "m3", ConsentVersion: "v1"}, 2); !errors.Is(err, ErrFeedbackQuota) {
		t.Fatalf("3rd today: want ErrFeedbackQuota, got %v", err)
	}
	got, err := s.GetFeedback(ctx, first.ID)
	if err != nil || string(got.Screenshot) != string(jpeg) || string(got.DSL) != `{"initialScreen":"home"}` || got.Device.Model != "iPhone17,1" {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, err := s.ListFeedback(ctx, FeedbackQuery{Since: time.Now().Add(-time.Hour), Limit: 10})
	if err != nil || len(list) < 2 || list[0].Message != "m2" || list[0].Screenshot != nil || list[1].DSL != nil {
		t.Fatalf("list newest first without bodies: %+v %v", list, err)
	}
	if _, err := s.GetFeedback(ctx, "fb_missing"+sfx); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	// retention: nothing is older than an hour ago from the future's point of view
	if n, err := s.PurgeFeedbackBefore(ctx, time.Now().Add(-time.Hour), 0, false); err != nil || n != 0 {
		t.Fatalf("nothing expired yet: %d %v", n, err)
	}
	if n, err := s.PurgeFeedbackBefore(ctx, time.Now().Add(time.Minute), 0, true); err != nil || n < 2 {
		t.Fatalf("dry run counts: %d %v", n, err)
	}
	if _, err := s.GetFeedback(ctx, first.ID); err != nil {
		t.Fatalf("dry run must keep: %v", err)
	}
}
