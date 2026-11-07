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

	"github.com/jphacks/kz_2601/backend/internal/entitlement"
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

func opts() options {
	return options{limit: 50, olderThan: store.PurgeAfter, deleteAuth: true, feedbackOlderThan: store.FeedbackRetention, feedbackLimit: 500}
}

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

func TestRunPurgesEntitlements(t *testing.T) {
	ctx := context.Background()
	m := setup(t, "gone", "active")
	if err := m.DeactivateUser(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	ents := entitlement.NewMemory()
	exp := time.Now().Add(400 * 24 * time.Hour)
	for _, uid := range []string{"gone", "active"} {
		p := entitlement.Pro{ProductID: "p", OriginalTransactionID: "otid-" + uid, TransactionID: "t", Environment: "Sandbox", ExpiresAt: exp}
		if _, err := ents.Apply(ctx, uid, p, ""); err != nil {
			t.Fatal(err)
		}
	}
	o := opts()
	o.entitlements = ents
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	if failed := run(ctx, m, &fakeAuth{}, o, logger, time.Now().Add(31*24*time.Hour)); failed != 0 {
		t.Fatalf("failed=%d", failed)
	}
	if p, _ := ents.Get(ctx, "gone"); p != nil {
		t.Fatalf("entitlement kept: %+v", p)
	}
	if owner, _ := ents.Owner(ctx, "otid-gone"); owner != "" {
		t.Fatalf("binding kept: %q", owner)
	}
	if p, _ := ents.Get(ctx, "active"); p == nil {
		t.Fatal("active account's entitlement must stay")
	}
}

func TestRunErasesExpiredFeedback(t *testing.T) {
	ctx := context.Background()
	m := setup(t)
	old, err := m.CreateFeedback(ctx, store.Feedback{UID: "a", Message: "old", ConsentVersion: "v1"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateFeedback(ctx, store.Feedback{UID: "a", Message: "new", ConsentVersion: "v1"}, 0); err != nil {
		t.Fatal(err)
	}
	if err := m.SetFeedbackCreatedAt(old.ID, time.Now().Add(-181*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	dry := opts()
	dry.dryRun = true
	if failed := run(ctx, m, nil, dry, logger, time.Now()); failed != 0 {
		t.Fatalf("dry run failed=%d", failed)
	}
	if items, _ := m.ListFeedback(ctx, store.FeedbackQuery{}); len(items) != 2 {
		t.Fatalf("dry run must keep everything: %d", len(items))
	}
	if failed := run(ctx, m, nil, opts(), logger, time.Now()); failed != 0 {
		t.Fatalf("failed=%d", failed)
	}
	items, _ := m.ListFeedback(ctx, store.FeedbackQuery{})
	if len(items) != 1 || items[0].Message != "new" {
		t.Fatalf("left: %+v", items)
	}
	short := opts()
	short.feedbackOlderThan = time.Hour
	if failed := run(ctx, m, nil, short, logger, time.Now()); failed != 1 {
		t.Fatalf("below 24h must be refused: failed=%d", failed)
	}
}

// The Cloud Logging queries in docs/infra/account-purge.md read these keys:
// renaming or dropping one breaks them silently.
func TestRunAccountPurgedLogKeys(t *testing.T) {
	ctx := context.Background()
	m := setup(t, "gone")
	if err := m.DeactivateUser(ctx, "gone"); err != nil {
		t.Fatal(err)
	}
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	if failed := run(ctx, m, &fakeAuth{}, opts(), logger, time.Now().Add(31*24*time.Hour)); failed != 0 {
		t.Fatalf("failed=%d", failed)
	}
	var keys []string
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		if !strings.Contains(line, `"msg":"purge: account purged"`) {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(line))
		if _, err := dec.Token(); err != nil {
			t.Fatal(err)
		}
		for dec.More() {
			k, err := dec.Token()
			if err != nil {
				t.Fatal(err)
			}
			keys = append(keys, k.(string))
			var v json.RawMessage
			if err := dec.Decode(&v); err != nil {
				t.Fatal(err)
			}
		}
	}
	want := "time level msg account deletedDaysAgo apps keptMirrors shares listings releases owners installs memberships marks postsScrubbed reports blocks feedback matchPairs matchRecords devices notifyConsents follows entitlements authDeleted"
	if got := strings.Join(keys, " "); got != want {
		t.Fatalf("account purged log keys:\n got %s\nwant %s", got, want)
	}
}
