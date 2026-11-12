package draft

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func TestFiledIssues(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	put := func(d loop.Draft) {
		t.Helper()
		d.Status = loop.DraftPublished
		if err := s.PutDraft(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
	// photo: issue #10, first posted by an old draft without PublishedAt
	// (reviewed at day 1), later extended by a comment at day 5
	put(loop.Draft{ID: "p1", ClusterKey: "photo", IssueRepo: "o/r", IssueNumber: 10, CreatedAt: base, ReviewedAt: base.Add(24 * time.Hour)})
	put(loop.Draft{ID: "p2", ClusterKey: "photo", IssueRepo: "o/r", IssueNumber: 10, CreatedAt: base.Add(4 * 24 * time.Hour), PublishedAt: base.Add(5 * 24 * time.Hour)})
	// zoom: issue #11 fixed, so a return is a new issue
	put(loop.Draft{ID: "z1", ClusterKey: "zoom", IssueRepo: "o/r", IssueNumber: 11, CreatedAt: base, FixedAt: base.Add(time.Hour)})
	put(loop.Draft{ID: "z2", ClusterKey: "zoom", IssueRepo: "o/r", IssueNumber: 11, CreatedAt: base.Add(time.Hour)})
	// dark: issue #12 seen closed
	put(loop.Draft{ID: "k1", ClusterKey: "dark", IssueRepo: "o/r", IssueNumber: 12, CreatedAt: base, ClosedAt: base.Add(time.Hour)})
	// approved in a dry run (no issue) and a draft without a cluster
	put(loop.Draft{ID: "n1", ClusterKey: "lag", CreatedAt: base})
	put(loop.Draft{ID: "e1", IssueRepo: "o/r", IssueNumber: 13, CreatedAt: base})
	// only the createdAt is known
	put(loop.Draft{ID: "c1", ClusterKey: "font", IssueRepo: "o/r", IssueNumber: 14, CreatedAt: base.Add(2 * time.Hour)})

	got, err := FiledIssues(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
	// newest draft of the issue; At is when the issue itself was first posted
	if p := got["photo"]; p.Number != 10 || p.Repo != "o/r" || p.DraftID != "p2" || !p.At.Equal(base.Add(24*time.Hour)) {
		t.Fatalf("photo %+v", p)
	}
	if f := got["font"]; f.Number != 14 || !f.At.Equal(base.Add(2*time.Hour)) {
		t.Fatalf("font %+v", f)
	}
}

// errStore fails one Store method.
type errStore struct {
	loop.Store
	get, update, listDrafts error
}

func (e errStore) GetItem(ctx context.Context, id string) (*loop.Item, error) {
	if e.get != nil {
		return nil, e.get
	}
	return e.Store.GetItem(ctx, id)
}

func (e errStore) UpdateItem(ctx context.Context, it loop.Item) error {
	if e.update != nil {
		return e.update
	}
	return e.Store.UpdateItem(ctx, it)
}

func (e errStore) ListDrafts(ctx context.Context, q loop.DraftQuery) ([]loop.Draft, error) {
	if e.listDrafts != nil {
		return nil, e.listDrafts
	}
	return e.Store.ListDrafts(ctx, q)
}

func TestLoadItems(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	its := triaged(t, s, 2, "photo", "bug", "落ちる", "落ちる。", loop.SourceInApp)
	// an expired or erased id is skipped
	got, err := LoadItems(ctx, s, []string{its[0].ID, "lin_erased", its[1].ID})
	if err != nil || len(got) != 2 || got[0].ID != its[0].ID || got[1].ID != its[1].ID {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err = LoadItems(ctx, s, nil); err != nil || len(got) != 0 {
		t.Fatalf("nil ids: %+v %v", got, err)
	}
	// any other read error is returned, not skipped
	down := errors.New("store down")
	if _, err := LoadItems(ctx, errStore{Store: s, get: down}, []string{its[0].ID}); !errors.Is(err, down) {
		t.Fatalf("read error: %v", err)
	}
}

func TestBuildStopsOnErrors(t *testing.T) {
	ctx := context.Background()
	down := errors.New("store down")

	s := loop.NewMemory()
	triaged(t, s, 1, "photo", "bug", "写真を追加すると落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	if _, err := (&Builder{Store: errStore{Store: s, listDrafts: down}}).Run(ctx); !errors.Is(err, down) {
		t.Fatalf("list drafts: %v", err)
	}
	if ds, _ := s.ListDrafts(ctx, loop.DraftQuery{}); len(ds) != 0 {
		t.Fatalf("draft written although the run failed: %+v", ds)
	}

	if _, err := (&Builder{Store: errStore{Store: s, update: down}, Now: func() time.Time { return now }}).Run(ctx); !errors.Is(err, down) {
		t.Fatalf("item update: %v", err)
	}

	s = loop.NewMemory()
	triaged(t, s, 2, "photo", "bug", "写真を追加すると落ちる", "写真を追加するとアプリが終了する。", loop.SourceInApp)
	n := 0
	stop := errors.New("audit down")
	_, err := (&Builder{Store: s, Now: func() time.Time { return now }, OnItem: func(context.Context, loop.Item) error {
		n++
		return stop
	}}).Run(ctx)
	if !errors.Is(err, stop) || n != 1 {
		t.Fatalf("on item: %v n=%d", err, n)
	}
}
