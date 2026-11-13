package loop

import (
	"context"
	"errors"
	"testing"
	"time"
)

// A dry run reports what the real run then does: an item that is both
// expired (or orphaned) and owned by a deleted account is counted once.
func TestSweepDryRunMatchesRealRun(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 12, 0, 0, 0, 0, time.UTC)
	build := func() *Memory {
		s := NewMemory()
		put := func(sid string, exp time.Time) {
			it := Item{ID: ItemID(SourceInApp, sid), Source: SourceInApp, SourceID: sid, UID: "leaver", Status: ItemNew, ExpiresAt: exp}
			if _, err := s.PutItem(ctx, it); err != nil {
				t.Fatal(err)
			}
		}
		put("fb_live", now.Add(time.Hour))
		put("fb_gone", now.Add(time.Hour))     // orphan of a deleted account
		put("fb_old", now.Add(-time.Hour))     // expired, of a deleted account
		put("fb_oldgone", now.Add(-time.Hour)) // expired and orphaned
		_, _ = s.PutNotice(ctx, Notice{ID: NoticeID("d", "leaver"), UID: "leaver", ExpiresAt: now.Add(time.Hour)})
		return s
	}
	src := existing{"fb_live": true, "fb_old": true}
	users := deletedUsers{"leaver": true}

	dry, err := Sweep(ctx, build(), src, users, now, true)
	if err != nil {
		t.Fatal(err)
	}
	real, err := Sweep(ctx, build(), src, users, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if dry != real {
		t.Fatalf("dry run %+v, real run %+v", dry, real)
	}
	if want := (SweepResult{Expired: 2, Orphans: 1, Deleted: 2}); real != want {
		t.Fatalf("%+v want %+v", real, want)
	}
}

func TestSweepWithoutCheckers(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	s := NewMemory()
	_, _ = s.PutItem(ctx, Item{ID: "lin_1", Source: SourceInApp, SourceID: "fb_1", UID: "u1", ExpiresAt: now.Add(time.Hour)})
	res, err := Sweep(ctx, s, nil, nil, now, false)
	if err != nil || res != (SweepResult{}) {
		t.Fatalf("%+v %v", res, err)
	}
	if _, err := s.GetItem(ctx, "lin_1"); err != nil {
		t.Fatal("deleted without a checker saying so")
	}
}

// failStore fails one Store method.
type failStore struct {
	Store
	purge, list, byUID, delItems, delNotices bool
}

var errStore = errors.New("store down")

func (f failStore) PurgeExpired(ctx context.Context, now time.Time, limit int, dry bool) (int, error) {
	if f.purge {
		return 0, errStore
	}
	return f.Store.PurgeExpired(ctx, now, limit, dry)
}

func (f failStore) ListItems(ctx context.Context, q ItemQuery) ([]Item, error) {
	if f.list {
		return nil, errStore
	}
	return f.Store.ListItems(ctx, q)
}

func (f failStore) ItemsByUID(ctx context.Context, uid string) ([]string, []string, error) {
	if f.byUID {
		return nil, nil, errStore
	}
	return f.Store.ItemsByUID(ctx, uid)
}

func (f failStore) DeleteItems(ctx context.Context, ids []string) error {
	if f.delItems {
		return errStore
	}
	return f.Store.DeleteItems(ctx, ids)
}

func (f failStore) DeleteNotices(ctx context.Context, ids []string) error {
	if f.delNotices {
		return errStore
	}
	return f.Store.DeleteNotices(ctx, ids)
}

type failingSource struct{}

func (failingSource) FeedbackExists(context.Context, []string) (map[string]bool, error) {
	return nil, errStore
}

type failingUsers struct{}

func (failingUsers) UserDeleted(context.Context, string) (bool, error) { return false, errStore }

// Any error stops the sweep and is returned; nothing is guessed.
func TestSweepErrors(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	seed := func() *Memory {
		s := NewMemory()
		_, _ = s.PutItem(ctx, Item{ID: "lin_gone", Source: SourceInApp, SourceID: "fb_gone", UID: "u1", ExpiresAt: now.Add(time.Hour)})
		_, _ = s.PutNotice(ctx, Notice{ID: "ntc_1", UID: "u1", ExpiresAt: now.Add(time.Hour)})
		return s
	}
	gone := existing{}
	users := deletedUsers{"u1": true}
	cases := []struct {
		name  string
		store Store
		src   SourceChecker
		users UserChecker
	}{
		{"purge", failStore{Store: seed(), purge: true}, gone, users},
		{"list", failStore{Store: seed(), list: true}, gone, users},
		{"source", seed(), failingSource{}, users},
		{"orphan delete", failStore{Store: seed(), delItems: true}, gone, users},
		{"user", seed(), nil, failingUsers{}},
		{"by uid", failStore{Store: seed(), byUID: true}, nil, users},
		{"user items delete", failStore{Store: seed(), delItems: true}, nil, users},
		{"user notices delete", failStore{Store: seed(), delNotices: true}, nil, users},
	}
	for _, c := range cases {
		if _, err := Sweep(ctx, c.store, c.src, c.users, now, false); !errors.Is(err, errStore) {
			t.Errorf("%s: %v", c.name, err)
		}
	}
	// a dry run never deletes, so delete failures cannot surface
	if _, err := Sweep(ctx, failStore{Store: seed(), delItems: true, delNotices: true}, gone, users, now, true); err != nil {
		t.Fatalf("dry run deleted: %v", err)
	}
}
