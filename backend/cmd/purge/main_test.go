package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/store"
	"github.com/jphacks/kz_2601/backend/pkg/dsl"
)

type fakeAuth struct {
	deleted []string
	fail    map[string]bool
}

func (f *fakeAuth) DeleteUser(_ context.Context, uid string) error {
	if f.fail[uid] {
		return errors.New("auth down")
	}
	f.deleted = append(f.deleted, uid)
	return nil
}

func setup(t *testing.T, uids ...string) *store.Memory {
	t.Helper()
	ctx := context.Background()
	m := store.NewMemory()
	initial, _ := json.Marshal(dsl.Initial())
	for _, uid := range uids {
		if _, _, err := m.EnsureApp(ctx, uid, "app-"+uid, initial, "x"); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func opts() options { return options{limit: 50, olderThan: store.PurgeAfter, deleteAuth: true} }

func TestRunPurgesOnlyOldDeletedAccounts(t *testing.T) {
	ctx := context.Background()
	m := setup(t, "gone", "active")
	if err := m.DeactivateUser(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	auth := &fakeAuth{}

	// 29 days later: nothing yet
	if failed := run(ctx, m, auth, opts(), logger, time.Now().Add(29*24*time.Hour)); failed != 0 || len(auth.deleted) != 0 {
		t.Fatalf("too early: failed=%d deleted=%v", failed, auth.deleted)
	}
	if _, err := m.GetApp(ctx, "gone", "app-gone"); err != nil {
		t.Fatalf("kept before 30 days: %v", err)
	}

	later := time.Now().Add(31 * 24 * time.Hour)
	if failed := run(ctx, m, auth, opts(), logger, later); failed != 0 {
		t.Fatalf("failed=%d", failed)
	}
	if len(auth.deleted) != 1 || auth.deleted[0] != "gone" {
		t.Fatalf("auth deleted: %v", auth.deleted)
	}
	if _, err := m.GetApp(ctx, "gone", "app-gone"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("app must be erased: %v", err)
	}
	if _, err := m.GetApp(ctx, "active", "app-active"); err != nil {
		t.Fatalf("active account untouched: %v", err)
	}
	// a second run has nothing left to do
	if failed := run(ctx, m, auth, opts(), logger, later); failed != 0 || len(auth.deleted) != 1 {
		t.Fatalf("rerun: failed=%d deleted=%v", failed, auth.deleted)
	}
	if strings.Contains(logs.String(), `"gone"`) || !strings.Contains(logs.String(), uidHash("gone")) {
		t.Fatalf("logs must carry the hash, not the uid:\n%s", logs.String())
	}
}

func TestRunDryRunAndLimit(t *testing.T) {
	ctx := context.Background()
	m := setup(t, "a", "b", "c")
	for _, uid := range []string{"a", "b", "c"} {
		if err := m.DeactivateUser(ctx, uid); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	later := time.Now().Add(31 * 24 * time.Hour)
	o := opts()
	o.dryRun = true
	if failed := run(ctx, m, nil, o, logger, later); failed != 0 {
		t.Fatal(failed)
	}
	for _, uid := range []string{"a", "b", "c"} {
		if _, err := m.GetApp(ctx, uid, "app-"+uid); err != nil {
			t.Fatalf("dry run must not delete %s: %v", uid, err)
		}
	}
	o = opts()
	o.limit = 2
	if failed := run(ctx, m, nil, o, logger, later); failed != 0 {
		t.Fatal(failed)
	}
	left, _ := m.DeletedAccounts(ctx, later, 0)
	if len(left) != 1 {
		t.Fatalf("limit 2 of 3 leaves 1: %v", left)
	}
}

func TestRunRetriesWhenAuthDeletionFails(t *testing.T) {
	ctx := context.Background()
	m := setup(t, "x")
	if err := m.DeactivateUser(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	later := time.Now().Add(31 * 24 * time.Hour)
	auth := &fakeAuth{fail: map[string]bool{"x": true}}
	if failed := run(ctx, m, auth, opts(), logger, later); failed != 1 {
		t.Fatalf("failed=%d", failed)
	}
	if left, _ := m.DeletedAccounts(ctx, later, 0); len(left) != 1 {
		t.Fatalf("still a candidate: %v", left)
	}
	auth.fail = nil
	if failed := run(ctx, m, auth, opts(), logger, later); failed != 0 || len(auth.deleted) != 1 {
		t.Fatalf("retry: failed=%d deleted=%v", failed, auth.deleted)
	}
	if gone, _ := m.UserDeleted(ctx, "x"); !gone {
		t.Fatal("purged uid stays deleted (410)")
	}
}

func TestRunRefusesShortWindow(t *testing.T) {
	o := opts()
	o.olderThan = time.Hour
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if failed := run(context.Background(), store.NewMemory(), nil, o, logger, time.Now()); failed != 1 {
		t.Fatalf("failed=%d", failed)
	}
}
