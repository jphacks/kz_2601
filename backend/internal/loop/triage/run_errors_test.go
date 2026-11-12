package triage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
)

func bugRows(ids []string, _ string) string {
	rs := []map[string]any{}
	for _, id := range ids {
		rs = append(rs, row(id, "bug", "photo-crash", "写真の追加で落ちる。"))
	}
	return rows(rs...)
}

// The model is told each pending cluster once, sorted, and only pending ones.
func TestTriageSendsKnownClusters(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	seed(t, s, "落ちる")
	now := time.Now().UTC()
	for i, d := range []loop.Draft{
		{ID: "d1", ClusterKey: "zoom-lag", Status: loop.DraftPending},
		{ID: "d2", ClusterKey: "photo-crash", Status: loop.DraftPending},
		{ID: "d3", ClusterKey: "photo-crash", Status: loop.DraftPending},
		{ID: "d4", ClusterKey: "", Status: loop.DraftPending},
		{ID: "d5", ClusterKey: "old-fixed", Status: loop.DraftPublished},
	} {
		d.CreatedAt = now.Add(time.Duration(i) * time.Second)
		_ = s.PutDraft(ctx, d)
	}
	m := &fakeModel{respond: bugRows}
	if _, err := (&Triager{Store: s, Model: m, nonce: "abc"}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	req := m.requests[0]
	if !strings.Contains(req, "- photo-crash\n- zoom-lag\n") || strings.Count(req, "photo-crash") != 1 || strings.Contains(req, "old-fixed") {
		t.Fatalf("clusters in request:\n%s", req)
	}
	if strings.Contains(req, "(none)") {
		t.Fatal("said none although clusters exist")
	}
}

// failingStore fails one Store method and counts updates.
type failingStore struct {
	loop.Store
	listItems, listDrafts error
	update                error
}

func (f *failingStore) ListItems(ctx context.Context, q loop.ItemQuery) ([]loop.Item, error) {
	if f.listItems != nil {
		return nil, f.listItems
	}
	return f.Store.ListItems(ctx, q)
}

func (f *failingStore) ListDrafts(ctx context.Context, q loop.DraftQuery) ([]loop.Draft, error) {
	if f.listDrafts != nil {
		return nil, f.listDrafts
	}
	return f.Store.ListDrafts(ctx, q)
}

func (f *failingStore) UpdateItem(ctx context.Context, it loop.Item) error {
	if f.update != nil {
		return f.update
	}
	return f.Store.UpdateItem(ctx, it)
}

func TestTriageStoreErrorsStopBeforeTheCall(t *testing.T) {
	down := errors.New("store down")
	for name, fs := range map[string]*failingStore{
		"items":  {listItems: down},
		"drafts": {listDrafts: down},
	} {
		fs.Store = loop.NewMemory()
		seed(t, fs.Store, "落ちる")
		m := &fakeModel{respond: bugRows}
		if _, err := (&Triager{Store: fs, Model: m}).Run(context.Background()); !errors.Is(err, down) {
			t.Errorf("%s: %v", name, err)
		}
		if m.calls != 0 {
			t.Errorf("%s: the model was called", name)
		}
	}
}

func TestTriageUpdateErrors(t *testing.T) {
	ctx := context.Background()
	// an item erased mid-run (account deletion, purge) is skipped, not an error
	fs := &failingStore{Store: loop.NewMemory(), update: loop.ErrNotFound}
	seed(t, fs.Store, "落ちる")
	var seen []string
	tr := &Triager{Store: fs, Model: &fakeModel{respond: bugRows}, OnItem: func(_ context.Context, it loop.Item) error {
		seen = append(seen, it.Status)
		return nil
	}}
	if res, err := tr.Run(ctx); err != nil || res.Triaged != 1 || len(seen) != 1 {
		t.Fatalf("%+v %v %v", res, err, seen)
	}

	// any other write error stops the run
	down := errors.New("store down")
	fs = &failingStore{Store: loop.NewMemory(), update: down}
	seed(t, fs.Store, "落ちる")
	if _, err := (&Triager{Store: fs, Model: &fakeModel{respond: bugRows}}).Run(ctx); !errors.Is(err, down) {
		t.Fatalf("write error: %v", err)
	}
	// and so does a held write (rejected output)
	fs = &failingStore{Store: loop.NewMemory(), update: down}
	seed(t, fs.Store, "落ちる")
	bad := &fakeModel{respond: func([]string, string) string { return "not json" }}
	if _, err := (&Triager{Store: fs, Model: bad}).Run(ctx); !errors.Is(err, down) {
		t.Fatalf("hold write error: %v", err)
	}
}

func TestTriageHookErrorsStop(t *testing.T) {
	ctx := context.Background()
	stop := errors.New("budget")

	s := loop.NewMemory()
	items := seed(t, s, "落ちる", "固まる")
	m := &fakeModel{respond: bugRows}
	var got Usage
	tr := &Triager{Store: s, Model: m, Batch: 1, AfterCall: func(_ context.Context, u Usage, callErr error) error {
		got = u
		return stop
	}}
	res, err := tr.Run(ctx)
	if !errors.Is(err, stop) || m.calls != 1 || res.Calls != 1 || got.Total() != 150 {
		t.Fatalf("after call: %+v %v calls=%d usage=%+v", res, err, m.calls, got)
	}
	// the response of a refused call is not used
	for _, it := range items {
		if g := get(t, s, it.ID); g.Status != loop.ItemNew {
			t.Fatalf("item changed after AfterCall refused: %+v", g)
		}
	}

	// AfterCall sees the call error too, so a failed call is still accounted
	var callErr error
	m = &fakeModel{err: errors.New("529")}
	_, err = (&Triager{Store: s, Model: m, AfterCall: func(_ context.Context, _ Usage, e error) error {
		callErr = e
		return nil
	}}).Run(ctx)
	if err == nil || callErr == nil {
		t.Fatalf("call error not passed on: %v %v", err, callErr)
	}

	s = loop.NewMemory()
	seed(t, s, "落ちる", "固まる")
	n := 0
	_, err = (&Triager{Store: s, Model: &fakeModel{respond: bugRows}, OnItem: func(context.Context, loop.Item) error {
		n++
		return stop
	}}).Run(ctx)
	if !errors.Is(err, stop) || n != 1 {
		t.Fatalf("on item: %v n=%d", err, n)
	}
}

// Items not allowed for AI never reach the model, and a run with nothing to
// send makes no call.
func TestTriageNothingToSend(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	items := seed(t, s, "落ちる")
	it := items[0]
	it.AIAllowed = false
	_ = s.UpdateItem(ctx, it)
	m := &fakeModel{respond: bugRows}
	res, err := (&Triager{Store: s, Model: m}).Run(ctx)
	if err != nil || m.calls != 0 || res.Calls != 0 || res.Items != 1 {
		t.Fatalf("%+v %v calls=%d", res, err, m.calls)
	}
	if g := get(t, s, it.ID); g.Status != loop.ItemNew {
		t.Fatalf("%+v", g)
	}
}

func TestNormalizeText(t *testing.T) {
	for in, want := range map[string]string{
		"落ちる!":                 "落ちる",
		"ＡＢＣ　ｄｅｆ":              "abcdef",
		"[メール] に送れない。":         "[メール]に送れない",
		"a​b":                  "ab",
		strings.Repeat(" ", 5): "",
	} {
		if got := NormalizeText(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
}
