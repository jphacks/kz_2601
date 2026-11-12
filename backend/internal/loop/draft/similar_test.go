package draft

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/triage"
)

func draftOf(t *testing.T, s loop.Store, key string) loop.Draft {
	t.Helper()
	ds, _ := s.ListDrafts(context.Background(), loop.DraftQuery{Status: loop.DraftPending})
	for _, d := range ds {
		if d.ClusterKey == key {
			return d
		}
	}
	t.Fatalf("no draft for %s", key)
	return loop.Draft{}
}

func TestBuildListsSimilarClusters(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	triaged(t, s, 2, "photo-add-crash", "bug", "写真を足すと落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	triaged(t, s, 1, "dark-mode", "feature", "暗い色にしたい", "暗い配色の切り替えの要望。", loop.SourceStoreReview)
	clock := now
	b := &Builder{Store: s, Now: func() time.Time { return clock }, Embedder: triage.FakeEmbedder{Dim: 256}}
	if res, err := b.Run(ctx); err != nil || res.Created != 2 {
		t.Fatalf("first: %+v %v", res, err)
	}
	if d := draftOf(t, s, "photo-add-crash"); strings.Contains(d.Body, "同じかもしれない束") {
		t.Fatalf("nothing near yet:\n%s", d.Body)
	}

	// a new cluster with another name for the same problem
	triaged(t, s, 1, "photo-crash", "bug", "写真で終了", "写真を追加するとアプリが終了します。", loop.SourceStoreReview)
	clock = now.Add(time.Hour)
	if res, err := b.Run(ctx); err != nil || res.Created != 1 || res.Held != 0 {
		t.Fatalf("second: %+v %v", res, err)
	}
	d := draftOf(t, s, "photo-crash")
	want := "## 同じかもしれない束\n\n- photo-add-crash(2 件)\n"
	if !strings.Contains(d.Body, want) || strings.Contains(d.Body, "dark-mode") {
		t.Fatalf("body:\n%s", d.Body)
	}
	if i, j := strings.Index(d.Body, want), strings.LastIndex(d.Body, "\n---\n"); i < 0 || i > j {
		t.Fatalf("section after the footer:\n%s", d.Body)
	}
	if r := Check(d.Title, d.Body, []string{"写真で終了"}); len(r) > 0 {
		t.Fatalf("exit check: %v", r)
	}
	// nothing is merged
	if d.Count != 1 || len(d.ItemIDs) != 1 {
		t.Fatalf("merged: %+v", d)
	}
	// an extended draft keeps its list
	triaged(t, s, 1, "photo-crash", "bug", "また写真で終了", "写真を追加するとアプリが終了します。", loop.SourceInApp)
	clock = now.Add(2 * time.Hour)
	if res, err := b.Run(ctx); err != nil || res.Extended != 1 {
		t.Fatalf("third: %+v %v", res, err)
	}
	if d := draftOf(t, s, "photo-crash"); !strings.Contains(d.Body, want) || d.Count != 2 {
		t.Fatalf("extended:\n%s", d.Body)
	}
}

// Without an Embedder (LOOP_EMBED off) the body has no section.
func TestBuildWithoutEmbedder(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	triaged(t, s, 1, "photo-add-crash", "bug", "写真を足すと落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	triaged(t, s, 1, "photo-crash", "bug", "写真で終了", "写真を追加するとアプリが終了します。", loop.SourceStoreReview)
	b := &Builder{Store: s, Now: func() time.Time { return now }}
	if _, err := b.Run(ctx); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"photo-add-crash", "photo-crash"} {
		if d := draftOf(t, s, k); strings.Contains(d.Body, "同じかもしれない束") {
			t.Fatalf("%s:\n%s", k, d.Body)
		}
	}
}
