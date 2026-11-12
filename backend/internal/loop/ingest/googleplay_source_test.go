package ingest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

// fakePlay serves fixed pages by token and records the tokens asked for.
type fakePlay struct {
	pages  map[string]GooglePlayPage
	tokens []string
}

func (f *fakePlay) ListReviews(_ context.Context, pkg, token string) (GooglePlayPage, error) {
	f.tokens = append(f.tokens, pkg+"/"+token)
	p, ok := f.pages[token]
	if !ok {
		return GooglePlayPage{}, fmt.Errorf("unknown token %q", token)
	}
	return p, nil
}

func playReview(id string, at time.Time, text string) GooglePlayReview {
	return GooglePlayReview{ID: id, Text: text, Rating: 3, AppVersion: "1.2.0", AndroidAPI: 34, CreatedAt: at}
}

func TestGooglePlayReviewsPages(t *testing.T) {
	ctx := context.Background()
	// the testdata pages chained: normal (2025-11-12) then replied (2025-11-11)
	f := &fakePlay{pages: map[string]GooglePlayPage{"": readPlay(t, "normal"), "next-page-1": readPlay(t, "replied")}}
	g := GooglePlayReviews{Client: f, Package: "jp.example.app"}
	raws, err := g.Fetch(ctx, time.Time{})
	if err != nil || len(raws) != 2 || raws[0].SourceID != "gp:AOqpTOE-normal-1" || raws[1].SourceID != "gp:AOqpTOE-replied-1" {
		t.Fatalf("all: %+v %v", raws, err)
	}
	if strings.Join(f.tokens, ",") != "jp.example.app/,jp.example.app/next-page-1" {
		t.Fatalf("tokens %v", f.tokens)
	}
	// a since between the two stops before the second page's older review
	f.tokens = nil
	raws, err = g.Fetch(ctx, time.Date(2025, 11, 11, 12, 0, 0, 0, time.UTC))
	if err != nil || len(raws) != 1 || raws[0].Source != SourceGooglePlay || raws[0].OSMajor != "Android API 34" {
		t.Fatalf("since: %+v %v", raws, err)
	}
	// the page cap bounds one run
	f.tokens = nil
	if raws, err := (GooglePlayReviews{Client: f, Package: "p", MaxPages: 1}).Fetch(ctx, time.Time{}); err != nil || len(raws) != 1 || len(f.tokens) != 1 {
		t.Fatalf("cap: %+v %v %v", raws, err, f.tokens)
	}
	// a token that does not move stops; API errors surface
	stuck := &fakePlay{pages: map[string]GooglePlayPage{"": {NextPageToken: "x"}, "x": {NextPageToken: "x"}}}
	if _, err := (GooglePlayReviews{Client: stuck, Package: "p"}).Fetch(ctx, time.Time{}); err == nil {
		t.Fatal("stuck token accepted")
	}
	if _, err := (GooglePlayReviews{Client: &fakePlay{}, Package: "p"}).Fetch(ctx, time.Time{}); err == nil {
		t.Fatal("api error swallowed")
	}
}

// The Runner keeps the place in cursor-google_play_review, re-reads only
// from it, and masks the text on the way in.
func TestGooglePlayReviewsCursor(t *testing.T) {
	ctx := context.Background()
	t1 := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	t2 := t1.Add(48 * time.Hour)
	f := &fakePlay{pages: map[string]GooglePlayPage{"": {Reviews: []GooglePlayReview{
		playReview("gp:2", t1.Add(time.Hour), "保存できない 090-1234-5678"),
		playReview("gp:1", t1, "落ちる"),
	}}}}
	ls := loop.NewMemory()
	now := t1.Add(2 * time.Hour)
	r := &Runner{Store: ls, Policy: PolicyFromEnv(env{}.get), Now: func() time.Time { return now }}
	g := GooglePlayReviews{Client: f, Package: "p"}
	if st, err := r.Run(ctx, g); err != nil || st.Created != 2 || st.Held != 2 { // not in the default LOOP_AI_SOURCES
		t.Fatalf("first: %+v %v", st, err)
	}
	if c, _ := ls.Cursor(ctx, SourceGooglePlay); !c.Equal(t1.Add(time.Hour)) {
		t.Fatalf("cursor %v", c)
	}
	items, _ := ls.ListItems(ctx, loop.ItemQuery{})
	for _, it := range items {
		if strings.Contains(it.Text, "090-1234-5678") {
			t.Fatalf("unmasked: %q", it.Text)
		}
	}
	// next run: a newer review on top; the older ones are within the overlap and skipped
	f.pages[""] = GooglePlayPage{Reviews: append([]GooglePlayReview{playReview("gp:3", t2, "通知が来ない")}, f.pages[""].Reviews...)}
	now = t2.Add(time.Hour)
	if st, err := r.Run(ctx, g); err != nil || st.Created != 1 || st.Skipped != 2 {
		t.Fatalf("second: %+v %v", st, err)
	}
	if c, _ := ls.Cursor(ctx, SourceGooglePlay); !c.Equal(t2) {
		t.Fatalf("cursor %v", c)
	}
}

func TestGooglePlayConfigFromEnv(t *testing.T) {
	if _, err := GooglePlayConfigFromEnv(env{}.get); !errors.Is(err, ErrGooglePlayOff) {
		t.Fatalf("off by default: %v", err)
	}
	if _, err := GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on"}.get); err == nil || !strings.Contains(err.Error(), EnvGooglePlayPackage) {
		t.Fatalf("on without a package: %v", err)
	}
	g, err := GooglePlayConfigFromEnv(env{EnvGooglePlayOn: "on", EnvGooglePlayPackage: " jp.example.app "}.get)
	if err != nil || g.Package != "jp.example.app" || g.Client == nil || g.Source() != SourceGooglePlay {
		t.Fatalf("on: %+v %v", g, err)
	}
	if PolicyFromEnv(env{}.get).AISources[SourceGooglePlay] {
		t.Fatal("google play must not be in the default LOOP_AI_SOURCES")
	}
}
