package fixed

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jphacks/kz_2601/backend/internal/loop"
	"github.com/jphacks/kz_2601/backend/internal/loop/github"
	"github.com/jphacks/kz_2601/backend/internal/runner/notify"
	"github.com/jphacks/kz_2601/backend/internal/store"
)

var now = time.Date(2026, 12, 1, 3, 0, 0, 0, time.UTC)

type fakeIssues map[int]github.Fix

func (f fakeIssues) IssueFix(_ context.Context, _ string, n int) (github.Fix, error) {
	return f[n], nil
}

type fakePusher struct{ sent []string }

func (f *fakePusher) Push(_ context.Context, uid, title, body string) (bool, error) {
	f.sent = append(f.sent, uid+"|"+body)
	return true, nil
}

type consent map[string]bool

func (c consent) PushAllowed(_ context.Context, uid string) (bool, error) { return c[uid], nil }

type deleted map[string]bool

func (d deleted) UserDeleted(_ context.Context, uid string) (bool, error) { return d[uid], nil }

// published puts a published draft for issue n with in-app senders uids
// (and one store review, which has no sender to tell).
func published(t *testing.T, s loop.Store, id string, n int, uids ...string) {
	t.Helper()
	ctx := context.Background()
	ids := []string{}
	for i, uid := range append(uids, "") {
		src := loop.SourceInApp
		if uid == "" {
			src = loop.SourceStoreReview
		}
		sid := fmt.Sprintf("%s-%d", id, i)
		it := loop.Item{ID: loop.ItemID(src, sid), Source: src, SourceID: sid, UID: uid, Text: "x", Status: loop.ItemDrafted, DraftID: id, ExpiresAt: now.Add(time.Hour)}
		if _, err := s.PutItem(ctx, it); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	d := loop.Draft{ID: id, Title: "[フィードバック] 不具合: 写真の追加で終了する", Status: loop.DraftPublished, IssueRepo: "owner/repo", IssueNumber: n,
		ItemIDs: ids, CreatedAt: now, ExpiresAt: now.Add(loop.Retention)}
	if err := s.PutDraft(ctx, d); err != nil {
		t.Fatal(err)
	}
}

func TestNotifyFixedSenders(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	published(t, s, "drf_a", 10, "u1", "u2", "u1", "gone") // u1 twice: one notice
	published(t, s, "drf_b", 11, "u3")                     // not fixed yet
	pusher := &fakePusher{}
	events := []loop.Event{}
	n := &Notifier{Store: s, Now: func() time.Time { return now },
		Issues: fakeIssues{10: {Fixed: true, PR: 55, At: now.Add(-time.Hour)}},
		Users:  deleted{"gone": true}, Pusher: pusher, Consent: consent{"u2": true},
		OnEvent: func(_ context.Context, e loop.Event) error { events = append(events, e); return nil }}
	res, err := n.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if res.Checked != 2 || len(res.Fixed) != 1 || res.Notices != 2 || res.Pushes != 1 || res.Skipped != 1 {
		t.Fatalf("%+v", res)
	}
	for _, uid := range []string{"u1", "u2"} {
		ns, _ := s.ListNotices(ctx, uid, 10)
		if len(ns) != 1 || ns[0].Title != NoticeTitle || !strings.Contains(ns[0].Body, "「写真の追加で終了する」") {
			t.Fatalf("%s notices %+v", uid, ns)
		}
	}
	if ns, _ := s.ListNotices(ctx, "gone", 10); len(ns) != 0 {
		t.Fatal("deleted account notified")
	}
	if len(pusher.sent) != 1 || !strings.HasPrefix(pusher.sent[0], "u2|") {
		t.Fatalf("pushes %v (only u2 consented)", pusher.sent)
	}
	d, _ := s.GetDraft(ctx, "drf_a")
	if d.FixedPR != 55 || d.NotifiedAt.IsZero() {
		t.Fatalf("draft %+v", d)
	}
	if events[0].Action != "fixed" || events[0].PR != 55 || events[0].Stage != loop.StageNotify {
		t.Fatalf("events %+v", events)
	}

	// a second run tells nobody twice
	res, err = n.Run(ctx)
	if err != nil || res.Notices != 0 || len(pusher.sent) != 1 {
		t.Fatalf("re-run %+v %v", res, err)
	}
	if !strings.Contains(ReleaseNotes([]Fixed{{Title: d.Title, Issue: 10}}), "- 写真の追加で終了する(#10)") {
		t.Fatal("release notes")
	}
}

func TestPushRateLimit(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	published(t, s, "drf_a", 10, "u1")
	published(t, s, "drf_b", 11, "u1")
	pusher := &fakePusher{}
	n := &Notifier{Store: s, Now: func() time.Time { return now }, Pusher: pusher, Consent: consent{"u1": true},
		Issues: fakeIssues{10: {Fixed: true, PR: 1}, 11: {Fixed: true, PR: 2}}}
	res, err := n.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// two notices in the app, one push in the week
	if res.Notices != 2 || len(pusher.sent) != 1 {
		t.Fatalf("%+v pushes %v", res, pusher.sent)
	}
}

func TestDryRunAndNoPushWithoutPusher(t *testing.T) {
	ctx := context.Background()
	s := loop.NewMemory()
	published(t, s, "drf_a", 10, "u1")
	n := &Notifier{Store: s, Now: func() time.Time { return now }, Issues: fakeIssues{10: {Fixed: true, PR: 1}}, DryRun: true}
	res, err := n.Run(ctx)
	if err != nil || len(res.Fixed) != 1 || res.Notices != 0 {
		t.Fatalf("dry run %+v %v", res, err)
	}
	if d, _ := s.GetDraft(ctx, "drf_a"); !d.FixedAt.IsZero() {
		t.Fatal("dry run wrote")
	}
	n.DryRun = false
	res, _ = n.Run(ctx)
	ns, _ := s.ListNotices(ctx, "u1", 10)
	if res.Notices != 1 || len(ns) != 1 || ns[0].Pushed {
		t.Fatalf("in-app only: %+v %+v", res, ns)
	}
}

type fakeSender struct{ tokens []string }

func (f *fakeSender) Send(_ context.Context, p notify.Push) (notify.Result, error) {
	if p.Token == "bad" {
		return notify.Result{Status: 410, Unregistered: true}, errors.New("gone")
	}
	f.tokens = append(f.tokens, p.Token)
	return notify.Result{Status: 200}, nil
}

type fakeDevices []store.Device

func (f fakeDevices) ListDevices(context.Context, string) ([]store.Device, error) { return f, nil }

func TestAPNsPusher(t *testing.T) {
	snd := &fakeSender{}
	p := APNsPusher{Sender: snd, Environment: store.DeviceProduction, Devices: fakeDevices{
		{Token: "t1", Environment: store.DeviceProduction}, {Token: "t2", Environment: store.DeviceSandbox}, {Token: "bad", Environment: store.DeviceProduction}}}
	sent, err := p.Push(context.Background(), "u1", "t", "b")
	if err != nil || !sent || len(snd.tokens) != 1 || snd.tokens[0] != "t1" {
		t.Fatalf("%v %v %v", sent, err, snd.tokens)
	}
	sent, err = APNsPusher{Sender: snd, Devices: fakeDevices{}}.Push(context.Background(), "u1", "t", "b")
	if sent || err != nil {
		t.Fatalf("no devices: %v %v", sent, err)
	}
}

func TestStoreConsent(t *testing.T) {
	ctx := context.Background()
	m := store.NewMemory()
	c := StoreConsent{C: m}
	if ok, _ := c.PushAllowed(ctx, "u1"); ok {
		t.Fatal("allowed with no consent")
	}
	if _, err := m.SetNotifyConsent(ctx, "u1", ConsentAppID, true); err != nil {
		t.Fatal(err)
	}
	if ok, _ := c.PushAllowed(ctx, "u1"); !ok {
		t.Fatal("consent ignored")
	}
}
